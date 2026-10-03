package sshcollector

import (
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
}

// poolKey 生成连接池的缓存键 host:port:user。
func poolKey(node SSHNodeConfig) string {
	return node.Host + ":" + strconv.Itoa(node.Port) + ":" + node.User
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
// known_hosts 校验强制开启（禁止 InsecureIgnoreHostKey）。
func productionDial(config *ssh.ClientConfig) DialFunc {
	return func(ctx context.Context, node SSHNodeConfig) (SSHClient, error) {
		addr := net.JoinHostPort(node.Host, strconv.Itoa(node.Port))

		cfg := *config
		cfg.User = node.User

		timeout := cfg.Timeout
		dialer := &net.Dialer{Timeout: timeout}
		netConn, err := dialer.DialContext(ctx, "tcp", addr)
		if err != nil {
			return nil, classifyDialError(err)
		}

		// 握手（密钥交换 + 认证）也必须有界：取 ctx deadline 与超时的较小者。
		deadline := handshakeDeadline(ctx, timeout)
		if !deadline.IsZero() {
			_ = netConn.SetDeadline(deadline)
		}

		conn, chans, reqs, err := ssh.NewClientConn(netConn, addr, &cfg)
		if err != nil {
			_ = netConn.Close()
			return nil, classifyDialError(err)
		}
		// 长连接恢复无 deadline，避免误杀后续 session。
		_ = netConn.SetDeadline(time.Time{})
		return &stdClient{client: ssh.NewClient(conn, chans, reqs)}, nil
	}
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
