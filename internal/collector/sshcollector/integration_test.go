package sshcollector

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"

	"nexus-agent-go/internal/model"
)

// ---------------------------------------------------------------------------
// 进程内真实 SSH 服务器：完整走 TCP + SSH 握手 + 公钥认证 + known_hosts 校验
// + session/exec 通道，但以 fixture 作为脚本输出（macOS 等环境没有 /proc）。
// newIntegrationServer 会把能通过认证的客户端私钥与匹配的 known_hosts
// 写入临时文件并设置 SSH_PRIVATE_KEY_PATH / SSH_KNOWN_HOSTS_PATH，
// 因此默认构造出的 Collector 与 Gateway 生产路径完全一致。
// ---------------------------------------------------------------------------

type integrationServer struct {
	listener   net.Listener
	hostSigner ssh.Signer

	mu           sync.Mutex
	connCount    int
	sessionCount int
	liveConns    []*ssh.ServerConn
	clientPub    ssh.PublicKey
	output       []byte
}

func newIntegrationServer(t *testing.T, output []byte) *integrationServer {
	t.Helper()

	clientSigner, clientKey := generateEd25519Signer(t)
	hostSigner, _ := generateEd25519Signer(t)

	server := &integrationServer{
		hostSigner: hostSigner,
		clientPub:  clientSigner.PublicKey(),
		output:     output,
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen failed: %v", err)
	}
	server.listener = listener
	go server.serve()
	t.Cleanup(func() { _ = listener.Close() })

	// 客户端私钥：生产加载路径会读取该文件。
	writeKeyFile(t, "id_ed25519", marshalPrivateKey(t, clientKey), "SSH_PRIVATE_KEY_PATH")
	// known_hosts：记录真实主机公钥，验证强制开启的指纹校验。
	writeKnownHosts(t, listener.Addr(), hostSigner.PublicKey())
	return server
}

func (s *integrationServer) addr() string {
	return s.listener.Addr().String()
}

func (s *integrationServer) counts() (conn int, session int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.connCount, s.sessionCount
}

// killConnections 模拟服务器侧断开（网线拔掉/sshd 重启）。
func (s *integrationServer) killConnections() {
	s.mu.Lock()
	conns := s.liveConns
	s.liveConns = nil
	s.mu.Unlock()
	for _, conn := range conns {
		_ = conn.Close()
	}
}

func (s *integrationServer) serve() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return
		}
		go s.handleConn(conn)
	}
}

func (s *integrationServer) handleConn(raw net.Conn) {
	serverConfig := &ssh.ServerConfig{
		PublicKeyCallback: func(meta ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
			if meta.User() != "renhaokun" {
				return nil, fmt.Errorf("unexpected user %q", meta.User())
			}
			if string(key.Marshal()) != string(s.clientPub.Marshal()) {
				return nil, errors.New("unknown public key")
			}
			return &ssh.Permissions{}, nil
		},
	}
	serverConfig.AddHostKey(s.hostSigner)
	serverConn, chans, reqs, err := ssh.NewServerConn(raw, serverConfig)
	if err != nil {
		return
	}

	s.mu.Lock()
	s.connCount++
	s.liveConns = append(s.liveConns, serverConn)
	s.mu.Unlock()
	defer func() {
		_ = serverConn.Close()
	}()

	go func() {
		for req := range reqs {
			if req.WantReply {
				_ = req.Reply(true, nil)
			}
		}
	}()

	for newChannel := range chans {
		if newChannel.ChannelType() != "session" {
			_ = newChannel.Reject(ssh.UnknownChannelType, "unsupported")
			continue
		}
		s.mu.Lock()
		s.sessionCount++
		s.mu.Unlock()
		go s.handleSession(newChannel)
	}
}

func (s *integrationServer) handleSession(newChannel ssh.NewChannel) {
	channel, requests, err := newChannel.Accept()
	if err != nil {
		return
	}
	defer channel.Close()

	for req := range requests {
		fmt.Printf("SERVER-DEBUG: request type=%q wantReply=%v\n", req.Type, req.WantReply)
		switch req.Type {
		case "exec":
			if len(req.Payload) < 4 {
				_ = req.Reply(false, nil)
				continue
			}
			if command := string(req.Payload[4:]); command != remoteCommand {
				_ = req.Reply(false, nil)
				continue
			}
			_ = req.Reply(true, nil)

			// 读入 stdin 上的脚本直到 EOF，再回放 fixture 输出。
			// 与真实 sshd 一致：命令结束后发送 exit-status 并关闭整个
			// channel（客户端 Session.Wait 需要它结束）。
			go func(ch ssh.Channel) {
				_, _ = io.ReadAll(ch)
				_, _ = ch.Write(s.output)
				_, _ = ch.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{Status: 0}))
				_ = ch.Close()
			}(channel)
		default:
			if req.WantReply {
				_ = req.Reply(true, nil)
			}
		}
	}
}

func generateEd25519Signer(t *testing.T) (ssh.Signer, ed25519.PrivateKey) {
	t.Helper()
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate ed25519 key failed: %v", err)
	}
	signer, err := ssh.NewSignerFromKey(privateKey)
	if err != nil {
		t.Fatalf("build signer failed: %v", err)
	}
	return signer, privateKey
}

// marshalPrivateKey 把私钥编码为 OpenSSH PEM。
func marshalPrivateKey(t *testing.T, privateKey ed25519.PrivateKey) []byte {
	t.Helper()
	block, err := ssh.MarshalPrivateKey(privateKey, "nexus-integration-test")
	if err != nil {
		t.Fatalf("marshal private key failed: %v", err)
	}
	return pem.EncodeToMemory(block)
}

func writeKeyFile(t *testing.T, name string, content []byte, envKey string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("write %s failed: %v", name, err)
	}
	t.Setenv(envKey, path)
}

// writeKnownHosts 写出明文格式的 known_hosts（与 knownhosts.New 兼容）。
func writeKnownHosts(t *testing.T, addr net.Addr, hostKey ssh.PublicKey) {
	t.Helper()
	host, port, _ := net.SplitHostPort(addr.String())
	line := fmt.Sprintf("[%s]:%s %s\n", host, port, ssh.MarshalAuthorizedKey(hostKey))
	path := filepath.Join(t.TempDir(), "known_hosts")
	if err := os.WriteFile(path, []byte(line), 0o600); err != nil {
		t.Fatalf("write known_hosts failed: %v", err)
	}
	t.Setenv("SSH_KNOWN_HOSTS_PATH", path)
}

func newEnvCollector(t *testing.T) *Collector {
	t.Helper()
	collector, err := NewWithOptionsFromEnv(LoadOptionsFromEnv())
	if err != nil {
		t.Fatalf("build collector failed: %v", err)
	}
	t.Cleanup(collector.Close)
	return collector
}

func portFromText(t *testing.T, portText string) int {
	t.Helper()
	var port int
	if _, err := fmt.Sscanf(portText, "%d", &port); err != nil {
		t.Fatalf("parse port failed: %v", err)
	}
	return port
}

func integrationNode(t *testing.T, server *integrationServer) model.AgentConfig {
	t.Helper()
	_, portText, err := net.SplitHostPort(server.addr())
	if err != nil {
		t.Fatalf("split addr failed: %v", err)
	}
	port := portFromText(t, portText)
	return model.AgentConfig{
		CollectorType: model.CollectorTypeSSH,
		Name:          "integration-node",
		SSHHost:       "127.0.0.1",
		SSHPort:       port,
		SSHUser:       "renhaokun",
	}
}

func TestSSHCollectorIntegration_RealHandshakeCollectsMetrics(t *testing.T) {
	server := newIntegrationServer(t, loadFixture(t, "snapshot_a6000.txt"))
	collector := newEnvCollector(t)
	node := integrationNode(t, server)
	ctx := context.Background()

	metrics, collectedAt, err := collector.Collect(ctx, node)
	if err != nil {
		t.Fatalf("Collect failed: %v", err)
	}
	if metrics.Hostname != "a6000-d-02" || len(metrics.Gpus) != 2 {
		t.Fatalf("metrics mismatch: hostname=%q gpus=%d", metrics.Hostname, len(metrics.Gpus))
	}
	if metrics.IPAddress != "127.0.0.1" || metrics.CPUCores != 32 {
		t.Fatalf("metrics fields mismatch: ip=%q cores=%d", metrics.IPAddress, metrics.CPUCores)
	}
	if collectedAt <= 0 {
		t.Fatalf("collectedAt should be set, got %d", collectedAt)
	}

	// 第二轮复用同一条 TCP 连接（连接数仍为 1，session 数为 2）。
	if _, _, err := collector.Collect(ctx, node); err != nil {
		t.Fatalf("second Collect failed: %v", err)
	}
	connCount, sessionCount := server.counts()
	if connCount != 1 {
		t.Fatalf("connection count = %d, want 1 (connection must be reused)", connCount)
	}
	if sessionCount != 2 {
		t.Fatalf("session count = %d, want 2", sessionCount)
	}
}

func TestSSHCollectorIntegration_ReconnectsAfterServerKillsConnection(t *testing.T) {
	server := newIntegrationServer(t, loadFixture(t, "snapshot_a6000.txt"))
	collector := newEnvCollector(t)
	node := integrationNode(t, server)
	ctx := context.Background()

	if _, _, err := collector.Collect(ctx, node); err != nil {
		t.Fatalf("first Collect failed: %v", err)
	}

	// 服务器侧掐断连接：下一轮 session 建立失败 → 本轮报错并失效连接。
	server.killConnections()
	time.Sleep(50 * time.Millisecond)
	if _, _, err := collector.Collect(ctx, node); err == nil {
		t.Fatal("collect after kill should fail")
	}

	// 再下一轮自动重新握手恢复（无需重启 Gateway/Collector）。
	if _, _, err := collector.Collect(ctx, node); err != nil {
		t.Fatalf("collect after reconnect failed: %v", err)
	}
	connCount, _ := server.counts()
	if connCount != 2 {
		t.Fatalf("connection count = %d, want 2 (auto reconnect)", connCount)
	}
}

func TestSSHCollectorIntegration_AuthFailureIsClassified(t *testing.T) {
	server := newIntegrationServer(t, loadFixture(t, "snapshot_a6000.txt"))
	// 用一把服务器不认识的私钥覆盖默认客户端私钥 → 认证失败。
	_, rogueKey := generateEd25519Signer(t)
	writeKeyFile(t, "rogue_id_ed25519", marshalPrivateKey(t, rogueKey), "SSH_PRIVATE_KEY_PATH")
	collector := newEnvCollector(t)

	node := integrationNode(t, server)
	_, _, collectErr := collector.Collect(context.Background(), node)
	if collectErr == nil {
		t.Fatal("collect should fail with an untrusted client key")
	}
	if got := ErrorCode(collectErr); got != "ssh_auth_failed" {
		t.Fatalf("error code = %q, want ssh_auth_failed (err=%v)", got, collectErr)
	}
}

func TestSSHCollectorIntegration_HostKeyMismatchIsClassified(t *testing.T) {
	server := newIntegrationServer(t, loadFixture(t, "snapshot_a6000.txt"))
	collector := newEnvCollector(t)

	// known_hosts 记录一把“错误”的主机指纹，模拟服务器被替换/中间人。
	rogueSigner, _ := generateEd25519Signer(t)
	host, portText, err := net.SplitHostPort(server.addr())
	if err != nil {
		t.Fatalf("split addr failed: %v", err)
	}
	line := fmt.Sprintf("[%s]:%s %s", host, portText, ssh.MarshalAuthorizedKey(rogueSigner.PublicKey()))
	writeKeyFile(t, "known_hosts_rogue", []byte(line), "SSH_KNOWN_HOSTS_PATH")

	// 覆盖 SSH_KNOWN_HOSTS_PATH 后必须重新构造 Collector（knownhosts 回调
	// 在构造时读取文件）。
	collector = newEnvCollector(t)

	node := integrationNode(t, server)
	node.SSHHost = host
	node.SSHPort = portFromText(t, portText)
	_, _, collectErr := collector.Collect(context.Background(), node)
	if collectErr == nil {
		t.Fatal("collect should fail on host key mismatch")
	}
	if got := ErrorCode(collectErr); got != "ssh_host_key_failed" {
		t.Fatalf("error code = %q, want ssh_host_key_failed (err=%v)", got, collectErr)
	}
}
