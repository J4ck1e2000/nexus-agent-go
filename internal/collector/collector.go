// Package collector 抽象节点指标采集入口：
// 上层 NodeState 只依赖 NodeMetricsCollector 接口与 CollectorRouter，
// 具体传输（旧版 Agent HTTP / Agentless SSH）由各自子包实现。
package collector

import (
	"context"
	"errors"
	"fmt"

	"nexus-agent-go/internal/model"
)

// NodeMetricsCollector 从单个节点采集一次 SystemMetrics。
// 返回指标、本次采集完成时间（Unix 秒）与错误；错误时上层保留既有状态。
type NodeMetricsCollector interface {
	Collect(ctx context.Context, node model.AgentConfig) (model.SystemMetrics, int64, error)
}

// CollectorRouter 按 collector_type 将节点分发到具体采集器。
// collector_type 为空时按旧版 agent 处理，保证存量数据库节点继续可用。
type CollectorRouter struct {
	agent NodeMetricsCollector
	ssh   NodeMetricsCollector
}

// NewCollectorRouter 创建采集路由；未配置的采集器保持 nil。
func NewCollectorRouter(agentCollector, sshCollector NodeMetricsCollector) *CollectorRouter {
	return &CollectorRouter{
		agent: agentCollector,
		ssh:   sshCollector,
	}
}

// CollectorFor 返回该节点应使用的采集器。
func (r *CollectorRouter) CollectorFor(node model.AgentConfig) (NodeMetricsCollector, error) {
	if r == nil {
		return nil, errors.New("collector router not initialized")
	}

	switch node.CollectorType {
	case "", model.CollectorTypeAgent:
		if r.agent == nil {
			return nil, errors.New("agent http collector not configured")
		}
		return r.agent, nil
	case model.CollectorTypeSSH:
		if r.ssh == nil {
			return nil, errors.New("ssh collector not configured")
		}
		return r.ssh, nil
	default:
		return nil, fmt.Errorf("unsupported collector type %q", node.CollectorType)
	}
}
