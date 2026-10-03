package sshcollector

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"

	"nexus-agent-go/internal/model"
)

// EnrollmentResult contains the first metrics sample and the public server key
// that will be pinned with the new node configuration.
type TrustedHostKeyHint struct {
	Marker    string `json:"marker"`
	PublicKey string `json:"public_key"`
}

type EnrollmentResult struct {
	Metrics                model.SystemMetrics
	CollectedAtUnix        int64
	HostKey                string
	HostKeyFingerprint     string
	GatewayKeyBootstrapped bool
}

// EnrollSSH discovers the remote host key, uses the existing Gateway key when
// already authorized, otherwise uses the one-time password to add that public
// key to the selected account, and verifies a key-authenticated collection.
func (c *Collector) EnrollSSH(ctx context.Context, node model.AgentConfig, password []byte, hints []TrustedHostKeyHint) (EnrollmentResult, error) {
	defer clearSecret(password)
	if c == nil || c.signer == nil {
		return EnrollmentResult{}, ErrSSHNotConfigured
	}
	hostKey, err := c.discoverHostKey(ctx, node.SSHHost, node.SSHPort, node.SSHUser, node.SSHHostKey, hints)
	if err != nil {
		return EnrollmentResult{}, err
	}

	hostKeyLine := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(hostKey)))
	node.SSHHostKey = hostKeyLine
	node.SSHHostKeyFingerprint = ssh.FingerprintSHA256(hostKey)
	sshNode := sshNodeConfig(node)

	// Prefer the Gateway public key if it is already authorized. This permits
	// later reconnects and re-enrollment without asking for the password again.
	client, err := dialSSHClient(ctx, sshNode, []ssh.AuthMethod{ssh.PublicKeys(c.signer)}, c.hostKeyCallback, c.options.ConnectTimeout)
	bootstrapped := false
	if err != nil {
		if !errors.Is(err, ErrSSHAuthFailed) || len(password) == 0 {
			return EnrollmentResult{}, err
		}
		sshNode.Password = password
		if err := c.installGatewayPublicKey(ctx, sshNode, c.publicAuthorizedKey()); err != nil {
			return EnrollmentResult{}, err
		}
		sshNode.Password = nil
		bootstrapped = true

		client, err = dialSSHClient(ctx, sshNode, []ssh.AuthMethod{ssh.PublicKeys(c.signer)}, c.hostKeyCallback, c.options.ConnectTimeout)
		if err != nil {
			return EnrollmentResult{}, fmt.Errorf("%w: public-key login failed after bootstrap", ErrSSHBootstrapFailed)
		}
	}
	_ = client.Close()

	metrics, collectedAt, err := c.Collect(ctx, node)
	if err != nil {
		return EnrollmentResult{}, err
	}
	return EnrollmentResult{
		Metrics:                metrics,
		CollectedAtUnix:        collectedAt,
		HostKey:                hostKeyLine,
		HostKeyFingerprint:     node.SSHHostKeyFingerprint,
		GatewayKeyBootstrapped: bootstrapped,
	}, nil
}

func sshNodeConfig(node model.AgentConfig) SSHNodeConfig {
	return SSHNodeConfig{
		Host:    node.SSHHost,
		Port:    node.SSHPort,
		User:    node.SSHUser,
		HostKey: node.SSHHostKey,
	}
}

func (c *Collector) publicAuthorizedKey() string {
	fields := strings.Fields(strings.TrimSpace(string(ssh.MarshalAuthorizedKey(c.signer.PublicKey()))))
	if len(fields) < 2 {
		return ""
	}
	// Disable forwarding and PTY access for this application key.
	return "restrict " + fields[0] + " " + fields[1] + " nexus-agent-go-gateway"
}

func (c *Collector) discoverHostKey(ctx context.Context, host string, port int, user string, existingPin string, hints []TrustedHostKeyHint) (ssh.PublicKey, error) {
	addr := net.JoinHostPort(host, fmt.Sprintf("%d", port))
	timeout := c.options.ConnectTimeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	dialer := &net.Dialer{Timeout: timeout}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, classifyDialError(err)
	}
	defer conn.Close()
	var candidate ssh.PublicKey
	var observedHost string
	var observedAddr net.Addr
	callback := func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		candidate = key
		observedHost = hostname
		observedAddr = remote
		return nil
	}
	if deadline := handshakeDeadline(ctx, timeout); !deadline.IsZero() {
		_ = conn.SetDeadline(deadline)
	}
	config := &ssh.ClientConfig{User: user, HostKeyCallback: callback, Timeout: timeout}
	sshConn, chans, reqs, handshakeErr := ssh.NewClientConn(conn, addr, config)
	if handshakeErr == nil {
		_ = ssh.NewClient(sshConn, chans, reqs).Close()
	}
	if candidate == nil {
		if handshakeErr != nil {
			return nil, classifyDialError(handshakeErr)
		}
		return nil, fmt.Errorf("%w: server did not present a host key", ErrSSHHostKeyFailed)
	}
	if strings.TrimSpace(existingPin) != "" {
		expected, _, _, _, parseErr := ssh.ParseAuthorizedKey([]byte(existingPin))
		if parseErr != nil || !bytes.Equal(expected.Marshal(), candidate.Marshal()) {
			return nil, fmt.Errorf("%w: pinned server host key changed", ErrSSHHostKeyFailed)
		}
		if err := c.rejectRevokedHostKey(observedHost, observedAddr, candidate, hints); err != nil {
			return nil, err
		}
		return candidate, nil
	}
	if err := c.validateHostKey(observedHost, observedAddr, candidate, hints); err != nil {
		return nil, err
	}
	return candidate, nil
}

func (c *Collector) validateHostKey(hostname string, remote net.Addr, candidate ssh.PublicKey, hints []TrustedHostKeyHint) error {
	if err := rejectRevokedHint(candidate, hints); err != nil {
		return err
	}

	hasLocalPin := false
	for _, hint := range hints {
		if hint.Marker == "@revoked" || hint.Marker == "@cert-authority" {
			continue
		}
		key, _, _, _, err := ssh.ParseAuthorizedKey([]byte(strings.TrimSpace(hint.PublicKey)))
		if err != nil {
			return fmt.Errorf("%w: local known_hosts entry is malformed", ErrSSHHostKeyFailed)
		}
		hasLocalPin = true
		if bytes.Equal(key.Marshal(), candidate.Marshal()) {
			return nil
		}
	}
	if hasLocalPin {
		return fmt.Errorf("%w: server key differs from the administrator workstation known_hosts", ErrSSHHostKeyFailed)
	}
	if c.hostKeyCallback != nil {
		if err := c.hostKeyCallback(hostname, remote, candidate); err == nil {
			return nil
		} else {
			var keyErr *knownhosts.KeyError
			if errors.As(err, &keyErr) && len(keyErr.Want) == 0 {
				// The Gateway known_hosts has no entry for this host; use TOFU.
				return nil
			}
			return fmt.Errorf("%w: %v", ErrSSHHostKeyFailed, err)
		}
	}
	// No pre-existing trust source: accept the first observed key and persist it with the node.
	return nil
}

func (c *Collector) rejectRevokedHostKey(hostname string, remote net.Addr, candidate ssh.PublicKey, hints []TrustedHostKeyHint) error {
	if err := rejectRevokedHint(candidate, hints); err != nil {
		return err
	}
	if c.hostKeyCallback != nil {
		if err := c.hostKeyCallback(hostname, remote, candidate); err != nil {
			var revokedErr *knownhosts.RevokedError
			if errors.As(err, &revokedErr) {
				return fmt.Errorf("%w: host key is revoked", ErrSSHHostKeyFailed)
			}
		}
	}
	return nil
}

func rejectRevokedHint(candidate ssh.PublicKey, hints []TrustedHostKeyHint) error {
	for _, hint := range hints {
		if hint.Marker != "@revoked" {
			continue
		}
		key, _, _, _, err := ssh.ParseAuthorizedKey([]byte(strings.TrimSpace(hint.PublicKey)))
		if err != nil {
			return fmt.Errorf("%w: local revoked-host entry is malformed", ErrSSHHostKeyFailed)
		}
		if bytes.Equal(key.Marshal(), candidate.Marshal()) {
			return fmt.Errorf("%w: host key is revoked", ErrSSHHostKeyFailed)
		}
	}
	return nil
}

func (c *Collector) installGatewayPublicKey(ctx context.Context, node SSHNodeConfig, publicKey string) error {
	if strings.TrimSpace(publicKey) == "" {
		return fmt.Errorf("%w: Gateway public key is unavailable", ErrSSHBootstrapFailed)
	}
	client, err := dialSSHClient(ctx, node, nil, c.hostKeyCallback, c.options.ConnectTimeout)
	if err != nil {
		if errors.Is(err, ErrSSHPasswordAuthFailed) || errors.Is(err, ErrSSHHostKeyFailed) || errors.Is(err, ErrSSHConnectFailed) {
			return err
		}
		return fmt.Errorf("%w: could not establish the bootstrap connection", ErrSSHBootstrapFailed)
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("%w: open bootstrap session failed", ErrSSHBootstrapFailed)
	}
	defer session.Close()

	script := strings.Join([]string{
		"set -eu",
		"umask 077",
		"mkdir -p \"$HOME/.ssh\"",
		"chmod 700 \"$HOME/.ssh\"",
		"touch \"$HOME/.ssh/authorized_keys\"",
		"chmod 600 \"$HOME/.ssh/authorized_keys\"",
		"key=" + shellQuote(publicKey),
		"if ! grep -F -x \"$key\" \"$HOME/.ssh/authorized_keys\" >/dev/null 2>&1; then",
		"  printf '%s\\n' \"$key\" >> \"$HOME/.ssh/authorized_keys\"",
		"fi",
	}, "\n") + "\n"
	session.Stdin = strings.NewReader(script)

	timeout := c.options.CommandTimeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	done := make(chan error, 1)
	go func() { done <- session.Run(remoteCommand) }()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case err := <-done:
		if err != nil {
			return fmt.Errorf("%w: could not authorize Gateway key", ErrSSHBootstrapFailed)
		}
		return nil
	case <-ctx.Done():
		_ = session.Close()
		return fmt.Errorf("%w: enrollment was cancelled", ErrSSHCommandTimeout)
	case <-timer.C:
		_ = session.Close()
		return fmt.Errorf("%w: authorizing Gateway key timed out", ErrSSHCommandTimeout)
	}
}

func clearSecret(secret []byte) {
	secret = secret[:cap(secret)]
	for i := range secret {
		secret[i] = 0
	}
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
