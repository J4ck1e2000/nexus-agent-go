package gateway

import (
	"context"
	"log"
	"time"
)

// NodePoller 周期性触发节点状态轮询。
type NodePoller struct {
	service      *NodeStateService
	pollInterval time.Duration
}

// NewNodePoller 创建后台轮询器。
func NewNodePoller(service *NodeStateService, pollInterval time.Duration) *NodePoller {
	if pollInterval <= 0 {
		pollInterval = defaultGatewayPollIntvl
	}
	return &NodePoller{
		service:      service,
		pollInterval: pollInterval,
	}
}

// Start 启动轮询循环，直到 ctx 取消。
func (p *NodePoller) Start(ctx context.Context) {
	if p == nil || p.service == nil {
		return
	}

	if err := p.service.PollOnce(ctx); err != nil {
		log.Printf("node poller initial run failed: %v", err)
	}

	ticker := time.NewTicker(p.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := p.service.PollOnce(ctx); err != nil {
				log.Printf("node poller tick failed: %v", err)
			}
		}
	}
}
