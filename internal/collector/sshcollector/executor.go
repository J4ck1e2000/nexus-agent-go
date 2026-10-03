package sshcollector

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"time"

	"golang.org/x/crypto/ssh"
)

// maxStderrCaptureBytes 限制内部捕获的 stderr 长度，只用于调试日志。
const maxStderrCaptureBytes = 4 * 1024

// CommandExecutor 在目标节点执行一段固定脚本并返回 stdout。
type CommandExecutor interface {
	Run(ctx context.Context, node SSHNodeConfig, script string) ([]byte, error)
}

// SSHCommandExecutor 基于连接池执行远程脚本：
// 复用长连接，每轮只开一个 session，输出有上限，超时可中止。
type SSHCommandExecutor struct {
	pool           *ConnectionPool
	commandTimeout time.Duration
	maxOutputBytes int64
}

// NewSSHCommandExecutor 创建执行器；commandTimeout/maxOutputBytes
// 非正值时使用默认（5s / 2MB）。
func NewSSHCommandExecutor(pool *ConnectionPool, commandTimeout time.Duration, maxOutputBytes int64) *SSHCommandExecutor {
	if commandTimeout <= 0 {
		commandTimeout = 5 * time.Second
	}
	if maxOutputBytes <= 0 {
		maxOutputBytes = maxMetricsOutputBytes
	}
	return &SSHCommandExecutor{
		pool:           pool,
		commandTimeout: commandTimeout,
		maxOutputBytes: maxOutputBytes,
	}
}

// Run 在节点上执行脚本：
//   - 建立连接/session 失败 → ssh_connect_failed（并失效缓存连接）；
//   - 脚本执行超时 → ssh_command_timeout；
//   - 非零退出 → ssh_command_failed；
//   - 通道意外关闭（EOF/连接丢失）→ ssh_connect_failed（并失效缓存连接）。
func (e *SSHCommandExecutor) Run(ctx context.Context, node SSHNodeConfig, script string) ([]byte, error) {
	runCtx, cancel := context.WithTimeout(ctx, e.commandTimeout)
	defer cancel()

	client, err := e.pool.Get(runCtx, node)
	if err != nil {
		return nil, err
	}

	session, err := client.NewSession()
	if err != nil {
		// 新 session 失败通常意味着连接已失效：失效后下一轮重新拨号。
		e.pool.Invalidate(node)
		return nil, fmt.Errorf("%w: new session failed: %v", ErrSSHConnectFailed, err)
	}
	defer session.Close()

	stdin, err := session.StdinPipe()
	if err != nil {
		e.pool.Invalidate(node)
		return nil, fmt.Errorf("%w: stdin pipe failed: %v", ErrSSHConnectFailed, err)
	}
	stdout, err := session.StdoutPipe()
	if err != nil {
		e.pool.Invalidate(node)
		return nil, fmt.Errorf("%w: stdout pipe failed: %v", ErrSSHConnectFailed, err)
	}
	stderr, err := session.StderrPipe()
	if err != nil {
		e.pool.Invalidate(node)
		return nil, fmt.Errorf("%w: stderr pipe failed: %v", ErrSSHConnectFailed, err)
	}

	if err := session.Start(remoteCommand); err != nil {
		e.pool.Invalidate(node)
		return nil, fmt.Errorf("%w: start command failed: %v", ErrSSHConnectFailed, err)
	}

	// 与 Wait 并发读取 stdout，避免输出超过管道缓冲时死锁。
	type readResult struct {
		data []byte
		err  error
	}
	stdoutCh := make(chan readResult, 1)
	go func() {
		data, err := io.ReadAll(io.LimitReader(stdout, e.maxOutputBytes+1))
		stdoutCh <- readResult{data: data, err: err}
	}()

	stderrCh := make(chan []byte, 1)
	go func() {
		stderrCh <- captureStderr(stderr)
	}()

	if _, err := stdin.Write([]byte(script)); err != nil {
		e.pool.Invalidate(node)
		return nil, fmt.Errorf("%w: send script failed: %v", ErrSSHCommandFailed, err)
	}
	if err := stdin.Close(); err != nil {
		e.pool.Invalidate(node)
		return nil, fmt.Errorf("%w: close stdin failed: %v", ErrSSHCommandFailed, err)
	}

	waitCh := make(chan error, 1)
	go func() { waitCh <- session.Wait() }()

	var waitErr error
	select {
	case <-runCtx.Done():
		// 关闭 session 中止远程命令；连接本身保持可用，不做失效。
		_ = session.Close()
		return nil, fmt.Errorf("%w: metrics command exceeded %s", ErrSSHCommandTimeout, e.commandTimeout)
	case waitErr = <-waitCh:
	}

	if waitErr != nil {
		var exitErr *ssh.ExitError
		if errors.As(waitErr, &exitErr) {
			return nil, fmt.Errorf("%w: exit status %d", ErrSSHCommandFailed, exitErr.ExitStatus())
		}
		// ExitMissingError / EOF 等：连接层异常，失效后下一轮重新拨号。
		e.pool.Invalidate(node)
		return nil, fmt.Errorf("%w: command channel closed: %v", ErrSSHConnectFailed, waitErr)
	}

	result := <-stdoutCh
	if result.err != nil {
		return nil, fmt.Errorf("%w: read output failed: %v", ErrSSHCommandFailed, result.err)
	}
	if int64(len(result.data)) > e.maxOutputBytes {
		return nil, fmt.Errorf("%w: output exceeds %d bytes", ErrSSHOutputTooLarge, e.maxOutputBytes)
	}

	if stderrData := <-stderrCh; len(stderrData) > 0 {
		logStderr(node, stderrData)
	}
	return result.data, nil
}

// captureStderr 有限度地捕获 stderr 供内部日志使用。
func captureStderr(stderr io.Reader) []byte {
	buf := make([]byte, maxStderrCaptureBytes)
	n, _ := io.ReadFull(stderr, buf)
	if n <= 0 {
		return nil
	}
	return buf[:n]
}

// logStderr 只输出摘要日志，不把原始 stderr 返回给上层调用方。
func logStderr(node SSHNodeConfig, data []byte) {
	message := bytes.ToValidUTF8(data, []byte("?"))
	message = bytes.TrimSpace(message)
	if len(message) > 200 {
		message = message[:200]
	}
	// stderr 可能包含任意远端输出，单行化避免日志注入。
	flattened := bytes.ReplaceAll(message, []byte("\n"), []byte(" "))
	log.Printf("ssh metrics stderr host=%s port=%d user=%s: %s", node.Host, node.Port, node.User, flattened)
}
