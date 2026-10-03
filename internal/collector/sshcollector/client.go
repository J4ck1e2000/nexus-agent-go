package sshcollector

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

// SSHNodeConfig 描述一次 SSH 连接的目标，仅用于连接配置，
// 永远不会进入远程脚本内容。
type SSHNodeConfig struct {
	Host string
	Port int
	User string
	// HostKey is the persisted OpenSSH public key pin for this node.
	HostKey string
	// Password is transient bootstrap input and is never used in pooled polling.
	Password []byte
}

// poolKey 生成连接池的缓存键 host:port:user。
func poolKey(node SSHNodeConfig) string {
	return node.Host + ":" + strconv.Itoa(node.Port) + ":" + node.User + ":" + node.HostKey
}

// Session 抽象 ssh.Session，便于测试替换。
type Session interface {
	StdinPipe() (io.WriteCloser, error)
	StdoutPipe() (io.Reader, error)
	StderrPipe() (io.Reader, error)
	Start(cmd string) error
	Wait() error
	Close() error
}

// SSHClient 抽象 ssh.Client，便于测试替换。
type SSHClient interface {
	NewSession() (Session, error)
	SendRequest(name string, wantReply bool, payload []byte) (bool, []byte, error)
	Close() error
}

// stdSession 将 *ssh.Session 适配到 Session 接口。
type stdSession struct {
	session *ssh.Session
}

func (s *stdSession) StdinPipe() (io.WriteCloser, error) { return s.session.StdinPipe() }
func (s *stdSession) StdoutPipe() (io.Reader, error)     { return s.session.StdoutPipe() }
func (s *stdSession) StderrPipe() (io.Reader, error)     { return s.session.StderrPipe() }
func (s *stdSession) Start(cmd string) error             { return s.session.Start(cmd) }
func (s *stdSession) Wait() error                        { return s.session.Wait() }
func (s *stdSession) Close() error                       { return s.session.Close() }

// stdClient 将 *ssh.Client 适配到 SSHClient 接口。
type stdClient struct {
	client *ssh.Client
}

func (c *stdClient) NewSession() (Session, error) {
	session, err := c.client.NewSession()
	if err != nil {
		return nil, err
	}
	return &stdSession{session: session}, nil
}

func (c *stdClient) SendRequest(name string, wantReply bool, payload []byte) (bool, []byte, error) {
	return c.client.SendRequest(name, wantReply, payload)
}

func (c *stdClient) Close() error { return c.client.Close() }

// DialFunc 建立一条 SSH 连接；由连接池调用，测试可注入假实现。
type DialFunc func(ctx context.Context, node SSHNodeConfig) (SSHClient, error)

// productionDial 返回真实 SSH 拨号函数：
// TCP 连接受 ctx 与 ConnectTimeout 双重约束，握手超时同样受限，
// 每个节点的固定主机公钥或 known_hosts 校验强制开启（禁止 InsecureIgnoreHostKey）。
func productionDial(config *ssh.ClientConfig) DialFunc {
	return func(ctx context.Context, node SSHNodeConfig) (SSHClient, error) {
		client, err := dialSSHClient(ctx, node, config.Auth, config.HostKeyCallback, config.Timeout)
		if err != nil {
			return nil, err
		}
		return &stdClient{client: client}, nil
	}
}

func dialSSHClient(ctx context.Context, node SSHNodeConfig, defaultAuth []ssh.AuthMethod, fallbackHostKey ssh.HostKeyCallback, timeout time.Duration) (*ssh.Client, error) {
	addr := net.JoinHostPort(node.Host, strconv.Itoa(node.Port))
	hostKeyCallback, err := pinnedHostKeyCallback(node.HostKey, fallbackHostKey)
	if err != nil {
		return nil, err
	}
	auth := defaultAuth
	if len(node.Password) > 0 {
		auth = []ssh.AuthMethod{ssh.Password(string(node.Password))}
	}
	dialer := &net.Dialer{Timeout: timeout}
	netConn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, classifyDialError(err)
	}
	deadline := handshakeDeadline(ctx, timeout)
	if !deadline.IsZero() {
		_ = netConn.SetDeadline(deadline)
	}
	clientConfig := &ssh.ClientConfig{
		User:            node.User,
		Auth:            auth,
		HostKeyCallback: hostKeyCallback,
		Timeout:         timeout,
	}
	conn, chans, reqs, err := ssh.NewClientConn(netConn, addr, clientConfig)
	if err != nil {
		_ = netConn.Close()
		classified := classifyDialError(err)
		if len(node.Password) > 0 && errors.Is(classified, ErrSSHAuthFailed) {
			return nil, fmt.Errorf("%w: %v", ErrSSHPasswordAuthFailed, err)
		}
		return nil, classified
	}
	_ = netConn.SetDeadline(time.Time{})
	return ssh.NewClient(conn, chans, reqs), nil
}

func pinnedHostKeyCallback(pinned string, fallback ssh.HostKeyCallback) (ssh.HostKeyCallback, error) {
	if strings.TrimSpace(pinned) == "" {
		if fallback == nil {
			return nil, fmt.Errorf("%w: no trusted host key is available", ErrSSHHostKeyFailed)
		}
		return fallback, nil
	}
	expected, _, _, _, err := ssh.ParseAuthorizedKey([]byte(pinned))
	if err != nil {
		return nil, fmt.Errorf("%w: stored host key is invalid", ErrSSHHostKeyFailed)
	}
	return func(_ string, _ net.Addr, presented ssh.PublicKey) error {
		if !bytes.Equal(expected.Marshal(), presented.Marshal()) {
			return fmt.Errorf("%w: server host key changed", ErrSSHHostKeyFailed)
		}
		return nil
	}, nil
}

// handshakeDeadline 取 ctx deadline 与 connect timeout 中较早者。
func handshakeDeadline(ctx context.Context, timeout time.Duration) time.Time {
	var deadline time.Time
	if timeout > 0 {
		deadline = time.Now().Add(timeout)
	}
	if d, ok := ctx.Deadline(); ok && (deadline.IsZero() || d.Before(deadline)) {
		deadline = d
	}
	return deadline
}

// classifyDialError 把拨号错误归约为稳定的 SSH 错误哨兵。
func classifyDialError(err error) error {
	if err == nil {
		return fmt.Errorf("%w: unknown error", ErrSSHConnectFailed)
	}

	if errors.Is(err, ErrSSHHostKeyFailed) {
		return err
	}
	var keyErr *knownhosts.KeyError
	if errors.As(err, &keyErr) {
		return fmt.Errorf("%w: %v", ErrSSHHostKeyFailed, err)
	}
	var revokedErr *knownhosts.RevokedError
	if errors.As(err, &revokedErr) {
		return fmt.Errorf("%w: %v", ErrSSHHostKeyFailed, err)
	}

	message := err.Error()
	if strings.Contains(message, "unable to authenticate") {
		return fmt.Errorf("%w: %v", ErrSSHAuthFailed, err)
	}
	return fmt.Errorf("%w: %v", ErrSSHConnectFailed, err)
}
