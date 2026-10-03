package sshcollector

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

// fakeClient 是可编程的 SSHClient 假实现。
type fakeClient struct {
	mu              sync.Mutex
	newSessionCount int
	sessionErr      error
	// sessionFactory 每次 NewSession 生成一个 session；nil 时生成空 session。
	sessionFactory func() *fakeSession
	sendRequestErr error
	closed         bool
}

func (c *fakeClient) NewSession() (Session, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.newSessionCount++
	if c.sessionErr != nil {
		return nil, c.sessionErr
	}
	if c.sessionFactory == nil {
		return &fakeSession{client: c}, nil
	}
	session := c.sessionFactory()
	session.client = c
	return session, nil
}

func (c *fakeClient) SendRequest(name string, wantReply bool, payload []byte) (bool, []byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.sendRequestErr != nil {
		return false, nil, c.sendRequestErr
	}
	return true, nil, nil
}

func (c *fakeClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	return nil
}

func (c *fakeClient) isClosed() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closed
}

// fakeSession 提供可编程的 session：预置 stdout 内容与 Wait 行为。
type fakeSession struct {
	client     *fakeClient
	stdoutData []byte
	waitErr    error
	// blockWait 为 true 时 Wait 阻塞直到 Close 被调用（模拟挂起命令）。
	blockWait bool

	started bool
	closed  bool
}

func (s *fakeSession) StdinPipe() (io.WriteCloser, error) {
	return &nopWriteCloser{}, nil
}

func (s *fakeSession) StdoutPipe() (io.Reader, error) {
	return bytes.NewReader(s.stdoutData), nil
}

func (s *fakeSession) StderrPipe() (io.Reader, error) {
	return bytes.NewReader(nil), nil
}

func (s *fakeSession) Start(cmd string) error {
	if cmd != remoteCommand {
		return errors.New("unexpected remote command: " + cmd)
	}
	s.started = true
	return nil
}

func (s *fakeSession) Wait() error {
	if s.blockWait {
		for {
			if s.closed {
				return errors.New("session closed while waiting")
			}
			time.Sleep(2 * time.Millisecond)
		}
	}
	return s.waitErr
}

func (s *fakeSession) Close() error {
	s.closed = true
	return nil
}

type nopWriteCloser struct{}

func (w *nopWriteCloser) Write(p []byte) (int, error) { return len(p), nil }
func (w *nopWriteCloser) Close() error                { return nil }

// poolHarness 提供可控的连接池与拨号记录。
type poolHarness struct {
	pool      *ConnectionPool
	dialCount int
	mu        sync.Mutex
	client    *fakeClient
	dialErr   error
}

func newPoolHarness(keepAlive time.Duration) *poolHarness {
	h := &poolHarness{client: &fakeClient{}}
	h.pool = NewConnectionPool(func(ctx context.Context, node SSHNodeConfig) (SSHClient, error) {
		h.mu.Lock()
		h.dialCount++
		dialErr := h.dialErr
		h.mu.Unlock()
		if dialErr != nil {
			return nil, dialErr
		}
		return h.client, nil
	}, keepAlive)
	return h
}

func TestConnectionPool_ReusesConnection(t *testing.T) {
	h := newPoolHarness(0)
	node := SSHNodeConfig{Host: "10.0.0.15", Port: 22, User: "renhaokun"}

	first, err := h.pool.Get(context.Background(), node)
	if err != nil {
		t.Fatalf("first Get failed: %v", err)
	}
	second, err := h.pool.Get(context.Background(), node)
	if err != nil {
		t.Fatalf("second Get failed: %v", err)
	}

	if first != SSHClient(h.client) || second != SSHClient(h.client) {
		t.Fatal("pool should return the cached client")
	}
	if h.dialCount != 1 {
		t.Fatalf("dial count = %d, want 1 (connection must be reused)", h.dialCount)
	}
}

func TestConnectionPool_DistinctKeysPerUser(t *testing.T) {
	h := newPoolHarness(0)

	if _, err := h.pool.Get(context.Background(), SSHNodeConfig{Host: "h", Port: 22, User: "a"}); err != nil {
		t.Fatalf("Get a failed: %v", err)
	}
	if _, err := h.pool.Get(context.Background(), SSHNodeConfig{Host: "h", Port: 22, User: "b"}); err != nil {
		t.Fatalf("Get b failed: %v", err)
	}
	if h.dialCount != 2 {
		t.Fatalf("distinct users must not share a connection: %d", h.dialCount)
	}
}

func TestConnectionPool_InvalidateForcesRedial(t *testing.T) {
	h := newPoolHarness(0)
	node := SSHNodeConfig{Host: "10.0.0.15", Port: 22, User: "renhaokun"}

	if _, err := h.pool.Get(context.Background(), node); err != nil {
		t.Fatalf("first Get failed: %v", err)
	}

	h.pool.Invalidate(node)
	if !h.client.isClosed() {
		t.Fatal("invalidate should close the old client")
	}

	if _, err := h.pool.Get(context.Background(), node); err != nil {
		t.Fatalf("second Get failed: %v", err)
	}
	if h.dialCount != 2 {
		t.Fatalf("dial count = %d, want 2 after invalidation", h.dialCount)
	}
}

func TestConnectionPool_CloseAllClosesEveryClient(t *testing.T) {
	h := newPoolHarness(0)
	nodes := []SSHNodeConfig{
		{Host: "10.0.0.1", Port: 22, User: "a"},
		{Host: "10.0.0.2", Port: 22, User: "b"},
	}
	for _, node := range nodes {
		if _, err := h.pool.Get(context.Background(), node); err != nil {
			t.Fatalf("Get failed: %v", err)
		}
	}

	h.pool.CloseAll()
	if !h.client.isClosed() {
		t.Fatal("CloseAll should close all clients")
	}

	// 关停后重新 Get 会建立新连接。
	if _, err := h.pool.Get(context.Background(), nodes[0]); err != nil {
		t.Fatalf("Get after CloseAll failed: %v", err)
	}
	if h.dialCount != 3 {
		t.Fatalf("dial count = %d, want 3", h.dialCount)
	}
}

func TestConnectionPool_KeepaliveInvalidatesDeadConnection(t *testing.T) {
	h := newPoolHarness(20 * time.Millisecond)
	h.client.mu.Lock()
	h.client.sendRequestErr = errors.New("connection lost")
	h.client.mu.Unlock()

	node := SSHNodeConfig{Host: "10.0.0.15", Port: 22, User: "renhaokun"}
	if _, err := h.pool.Get(context.Background(), node); err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if h.client.isClosed() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("keepalive should invalidate the dead connection")
}

func TestConnectionPool_DialErrorPropagates(t *testing.T) {
	h := newPoolHarness(0)
	h.mu.Lock()
	h.dialErr = fmt.Errorf("%w: permission denied", ErrSSHAuthFailed)
	h.mu.Unlock()

	_, err := h.pool.Get(context.Background(), SSHNodeConfig{Host: "h", Port: 22, User: "u"})
	if !errors.Is(err, ErrSSHAuthFailed) {
		t.Fatalf("expected auth failed, got %v", err)
	}
}

// ---- executor 级别测试 ----

func newExecutorHarness(sessionFactory func() *fakeSession) (*SSHCommandExecutor, *poolHarness) {
	h := newPoolHarness(0)
	h.client.mu.Lock()
	h.client.sessionFactory = sessionFactory
	h.client.mu.Unlock()
	executor := NewSSHCommandExecutor(h.pool, 2*time.Second, 0)
	return executor, h
}

func fixtureSessionFactory(t *testing.T) func() *fakeSession {
	return func() *fakeSession {
		return &fakeSession{stdoutData: loadFixture(t, "snapshot_a6000.txt")}
	}
}

func TestSSHCommandExecutor_ReusesPooledConnection(t *testing.T) {
	executor, h := newExecutorHarness(fixtureSessionFactory(t))
	node := SSHNodeConfig{Host: "10.0.0.15", Port: 22, User: "renhaokun"}

	for i := 0; i < 3; i++ {
		if _, err := executor.Run(context.Background(), node, remoteMetricsScript); err != nil {
			t.Fatalf("Run %d failed: %v", i, err)
		}
	}
	if h.dialCount != 1 {
		t.Fatalf("dial count = %d, want 1", h.dialCount)
	}
	h.client.mu.Lock()
	sessions := h.client.newSessionCount
	h.client.mu.Unlock()
	if sessions != 3 {
		t.Fatalf("session count = %d, want 3 (new session per run)", sessions)
	}
}

func TestSSHCommandExecutor_SessionFailureInvalidatesConnection(t *testing.T) {
	executor, h := newExecutorHarness(fixtureSessionFactory(t))
	node := SSHNodeConfig{Host: "10.0.0.15", Port: 22, User: "renhaokun"}

	h.client.mu.Lock()
	h.client.sessionErr = errors.New("connection reset by peer")
	h.client.mu.Unlock()

	_, err := executor.Run(context.Background(), node, remoteMetricsScript)
	if !errors.Is(err, ErrSSHConnectFailed) {
		t.Fatalf("expected connect failed, got %v", err)
	}

	// 失效后下一轮重新拨号。
	h.client.mu.Lock()
	h.client.sessionErr = nil
	h.client.mu.Unlock()

	if _, err := executor.Run(context.Background(), node, remoteMetricsScript); err != nil {
		t.Fatalf("Run after invalidation failed: %v", err)
	}
	h.mu.Lock()
	count := h.dialCount
	h.mu.Unlock()
	if count != 2 {
		t.Fatalf("dial count = %d, want 2 (session failure must invalidate)", count)
	}
}

func TestSSHCommandExecutor_EOFInvalidatesConnection(t *testing.T) {
	h := newPoolHarness(0)
	h.client.mu.Lock()
	h.client.sessionFactory = func() *fakeSession {
		return &fakeSession{
			stdoutData: loadFixture(t, "snapshot_a6000.txt"),
			waitErr:    io.EOF,
		}
	}
	h.client.mu.Unlock()
	executor := NewSSHCommandExecutor(h.pool, 2*time.Second, 0)
	node := SSHNodeConfig{Host: "10.0.0.15", Port: 22, User: "renhaokun"}

	_, err := executor.Run(context.Background(), node, remoteMetricsScript)
	if !errors.Is(err, ErrSSHConnectFailed) {
		t.Fatalf("EOF should map to connect failed, got %v", err)
	}

	// 连接被失效：下一次 Get 会重新拨号。
	if _, err := h.pool.Get(context.Background(), node); err != nil {
		t.Fatalf("Get after invalidation failed: %v", err)
	}
	h.mu.Lock()
	count := h.dialCount
	h.mu.Unlock()
	if count != 2 {
		t.Fatalf("dial count = %d, want 2 (EOF must invalidate the connection)", count)
	}
}

func TestSSHCommandExecutor_ExitErrorMapsToCommandFailed(t *testing.T) {
	executor, _ := newExecutorHarness(func() *fakeSession {
		return &fakeSession{
			stdoutData: loadFixture(t, "snapshot_a6000.txt"),
			waitErr:    &ssh.ExitError{},
		}
	})

	_, err := executor.Run(context.Background(), SSHNodeConfig{Host: "h", Port: 22, User: "u"}, remoteMetricsScript)
	if !errors.Is(err, ErrSSHCommandFailed) {
		t.Fatalf("exit error should map to command failed, got %v", err)
	}
}

func TestSSHCommandExecutor_Timeout(t *testing.T) {
	executor, h := newExecutorHarness(func() *fakeSession {
		return &fakeSession{
			stdoutData: loadFixture(t, "snapshot_a6000.txt"),
			blockWait:  true,
		}
	})
	executor = NewSSHCommandExecutor(executor.pool, 50*time.Millisecond, 0)

	start := time.Now()
	_, err := executor.Run(context.Background(), SSHNodeConfig{Host: "h", Port: 22, User: "u"}, remoteMetricsScript)
	if !errors.Is(err, ErrSSHCommandTimeout) {
		t.Fatalf("expected command timeout, got %v", err)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("timeout should trigger quickly, took %s", elapsed)
	}
	// 超时不失效连接（远端命令挂起不代表连接损坏）。
	h.mu.Lock()
	count := h.dialCount
	h.mu.Unlock()
	if count != 1 {
		t.Fatalf("timeout must not invalidate the connection, dial count = %d", count)
	}
}

func TestSSHCommandExecutor_OutputTooLarge(t *testing.T) {
	executor, _ := newExecutorHarness(func() *fakeSession {
		return &fakeSession{stdoutData: make([]byte, 128)}
	})
	executor = NewSSHCommandExecutor(executor.pool, 2*time.Second, 64)

	_, err := executor.Run(context.Background(), SSHNodeConfig{Host: "h", Port: 22, User: "u"}, remoteMetricsScript)
	if !errors.Is(err, ErrSSHOutputTooLarge) {
		t.Fatalf("expected output too large, got %v", err)
	}
}
