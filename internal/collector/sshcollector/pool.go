package sshcollector

import (
	"context"
	"log"
	"sync"
	"time"
)

// keepaliveRequestName 是 OpenSSH 风格的 keepalive 请求名。
const keepaliveRequestName = "keepalive@openssh.com"

// ManagedClient 缓存一条可复用的 SSH 连接。
type ManagedClient struct {
	key       string
	node      SSHNodeConfig
	client    SSHClient
	createdAt time.Time
	lastUsed  time.Time
	stop      chan struct{}
	stopOnce  sync.Once
}

func (mc *ManagedClient) stopKeepAlive() {
	mc.stopOnce.Do(func() { close(mc.stop) })
}

// ConnectionPool 按 host:port:user 复用长连接。
// 每轮采集只开新 session；连接失效由使用方调用 Invalidate，
// 下一轮轮询自动重新拨号（不做立即重试）。
type ConnectionPool struct {
	mu      sync.Mutex
	clients map[string]*ManagedClient

	dial      DialFunc
	keepAlive time.Duration
	nowFunc   func() time.Time
}

// NewConnectionPool 创建连接池；dial 必须提供（生产为 productionDial，
// 测试注入假实现），keepAlive <= 0 表示关闭 keepalive。
func NewConnectionPool(dial DialFunc, keepAlive time.Duration) *ConnectionPool {
	nowFunc := time.Now
	return &ConnectionPool{
		clients:   make(map[string]*ManagedClient),
		dial:      dial,
		keepAlive: keepAlive,
		nowFunc:   nowFunc,
	}
}

// Get 返回该节点的缓存连接；没有可用连接时拨号建立并缓存。
func (p *ConnectionPool) Get(ctx context.Context, node SSHNodeConfig) (SSHClient, error) {
	key := poolKey(node)

	p.mu.Lock()
	if mc, ok := p.clients[key]; ok {
		mc.lastUsed = p.nowFunc()
		client := mc.client
		p.mu.Unlock()
		return client, nil
	}
	p.mu.Unlock()

	// 拨号在锁外执行，避免握手期间阻塞其他节点的采集。
	client, err := p.dial(ctx, node)
	if err != nil {
		return nil, err
	}

	p.mu.Lock()
	if existing, ok := p.clients[key]; ok {
		// 并发拨号竞态：保留已有连接，关闭多余的一条。
		p.mu.Unlock()
		_ = client.Close()
		return existing.client, nil
	}
	mc := &ManagedClient{
		key:       key,
		node:      node,
		client:    client,
		createdAt: p.nowFunc(),
		lastUsed:  p.nowFunc(),
		stop:      make(chan struct{}),
	}
	p.clients[key] = mc
	p.mu.Unlock()

	log.Printf("ssh connected host=%s port=%d user=%s", node.Host, node.Port, node.User)
	if p.keepAlive > 0 {
		p.startKeepAlive(mc)
	}
	return client, nil
}

// Invalidate 关闭并移除该节点的缓存连接（连接失效/EOF/broken pipe 时调用）。
func (p *ConnectionPool) Invalidate(node SSHNodeConfig) {
	key := poolKey(node)

	p.mu.Lock()
	mc, ok := p.clients[key]
	if ok {
		delete(p.clients, key)
	}
	p.mu.Unlock()
	if !ok {
		return
	}

	mc.stopKeepAlive()
	_ = mc.client.Close()
	log.Printf("ssh invalidated host=%s port=%d user=%s", node.Host, node.Port, node.User)
}

// CloseAll 关闭全部缓存连接，Gateway 关停时调用，避免遗留 socket。
func (p *ConnectionPool) CloseAll() {
	p.mu.Lock()
	clients := make([]*ManagedClient, 0, len(p.clients))
	for key, mc := range p.clients {
		clients = append(clients, mc)
		delete(p.clients, key)
	}
	p.mu.Unlock()

	for _, mc := range clients {
		mc.stopKeepAlive()
		_ = mc.client.Close()
	}
}

// startKeepAlive 周期发送 keepalive 请求探测连接活性；
// 失败或超时则失效连接，等待下一轮轮询重新拨号。
func (p *ConnectionPool) startKeepAlive(mc *ManagedClient) {
	interval := p.keepAlive
	node := mc.node
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-mc.stop:
				return
			case <-ticker.C:
				done := make(chan error, 1)
				go func() {
					_, _, err := mc.client.SendRequest(keepaliveRequestName, true, nil)
					done <- err
				}()
				// 回复超时按连接失效处理；遗留的等待 goroutine 会在
				// 连接真正报错后自行退出，结果被丢弃。
				select {
				case <-mc.stop:
					return
				case err := <-done:
					if err != nil {
						p.Invalidate(node)
						return
					}
				case <-time.After(2 * interval):
					p.Invalidate(node)
					return
				}
			}
		}
	}()
}
