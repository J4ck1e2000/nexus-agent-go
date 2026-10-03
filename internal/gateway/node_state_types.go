package gateway

import (
	"fmt"
	"time"

	"nexus-agent-go/internal/model"
)

const (
	// NodeStatusPending 表示节点已配置但暂未拿到状态。
	NodeStatusPending = "pending"
	// NodeStatusOnline 表示节点本轮轮询成功。
	NodeStatusOnline = "online"
	// NodeStatusOffline 表示节点本轮轮询失败。
	NodeStatusOffline = "offline"
)

const (
	// AvailabilityTierHighlyAvailable 可用性高。
	AvailabilityTierHighlyAvailable = "highlyAvailable"
	// AvailabilityTierAvailable 可用。
	AvailabilityTierAvailable = "available"
	// AvailabilityTierBusy 繁忙。
	AvailabilityTierBusy = "busy"
	// AvailabilityTierSaturated 饱和。
	AvailabilityTierSaturated = "saturated"
	// AvailabilityTierOffline 离线。
	AvailabilityTierOffline = "offline"
)

// NodeNetworkMetrics 为节点网络速度指标（MB/s）。
type NodeNetworkMetrics struct {
	UpSpeed   float64 `json:"upSpeed"`
	DownSpeed float64 `json:"downSpeed"`
}

// NodeGPUSummary 为节点 GPU 派生汇总。
type NodeGPUSummary struct {
	GPUCount         int      `json:"gpuCount"`
	BusyGpuCount     int      `json:"busyGpuCount"`
	IdleGpuCount     int      `json:"idleGpuCount"`
	AvgUtilization   *float64 `json:"avgUtilization"`
	AvgMemoryPercent *float64 `json:"avgMemoryPercent"`
	TotalMemoryUsed  float64  `json:"totalMemoryUsed"`
	TotalMemory      float64  `json:"totalMemory"`
	TotalPower       float64  `json:"totalPower"`
	GpuPressure      float64  `json:"gpuPressure"`
	BusyRatio        float64  `json:"busyRatio"`
}

// NodeState 为 Redis 热状态层的节点状态模型。
type NodeState struct {
	ID                int64                `json:"id"`
	Name              string               `json:"name"`
	URL               string               `json:"url"`
	Status            string               `json:"status"`
	LastPolledAtUnix  int64                `json:"lastPolledAtUnix"`
	LastSeenAtUnix    int64                `json:"lastSeenAtUnix"`
	CollectedAtUnix   int64                `json:"collectedAtUnix"`
	Data              *model.SystemMetrics `json:"data"`
	Metrics           *NodeNetworkMetrics  `json:"metrics"`
	ActiveUsers       []string             `json:"activeUsers"`
	ActiveUserCount   int                  `json:"activeUserCount"`
	GPUSummary        NodeGPUSummary       `json:"gpuSummary"`
	AvailabilityScore int                  `json:"availabilityScore"`
	AvailabilityTier  string               `json:"availabilityTier"`
	DataAgeSec        *float64             `json:"dataAgeSec"`
	LastError         string               `json:"lastError,omitempty"`
}

// NodeOverview 是 /api/nodes/overview 返回结构。
type NodeOverview struct {
	ID                int64                `json:"id"`
	Name              string               `json:"name"`
	URL               string               `json:"url"`
	Status            string               `json:"status"`
	Data              *model.SystemMetrics `json:"data"`
	Metrics           *NodeNetworkMetrics  `json:"metrics"`
	LastSeenAt        *int64               `json:"lastSeenAt"`
	LastPolledAtUnix  int64                `json:"lastPolledAtUnix"`
	CollectedAtUnix   int64                `json:"collectedAtUnix"`
	ActiveUsers       []string             `json:"activeUsers"`
	ActiveUserCount   int                  `json:"activeUserCount"`
	GPUSummary        NodeGPUSummary       `json:"gpuSummary"`
	AvailabilityScore int                  `json:"availabilityScore"`
	AvailabilityTier  string               `json:"availabilityTier"`
	DataAgeSec        *float64             `json:"dataAgeSec"`
	Error             string               `json:"error,omitempty"`
}

func emptyGPUSummary() NodeGPUSummary {
	return NodeGPUSummary{
		GPUCount:         0,
		BusyGpuCount:     0,
		IdleGpuCount:     0,
		AvgUtilization:   nil,
		AvgMemoryPercent: nil,
		TotalMemoryUsed:  0,
		TotalMemory:      0,
		TotalPower:       0,
		GpuPressure:      0,
		BusyRatio:        0,
	}
}

// nodeDisplayEndpoint 返回节点在概览中的展示端点：
// agent 模式为 Agent URL，SSH 模式为 ssh://user@host:port。
func nodeDisplayEndpoint(node model.AgentConfig) string {
	if node.CollectorType == model.CollectorTypeSSH {
		return fmt.Sprintf("ssh://%s@%s:%d", node.SSHUser, node.SSHHost, node.SSHPort)
	}
	return node.URL
}

func pendingNodeState(node model.AgentConfig) NodeState {
	return NodeState{
		ID:                node.ID,
		Name:              node.Name,
		URL:               nodeDisplayEndpoint(node),
		Status:            NodeStatusPending,
		ActiveUsers:       []string{},
		ActiveUserCount:   0,
		GPUSummary:        emptyGPUSummary(),
		AvailabilityScore: 0,
		AvailabilityTier:  AvailabilityTierOffline,
	}
}

func nodeStateToOverview(state NodeState) NodeOverview {
	var lastSeenAt *int64
	if state.LastSeenAtUnix > 0 {
		ms := state.LastSeenAtUnix * 1000
		lastSeenAt = &ms
	}

	return NodeOverview{
		ID:                state.ID,
		Name:              state.Name,
		URL:               state.URL,
		Status:            state.Status,
		Data:              cloneSystemMetrics(state.Data),
		Metrics:           cloneNetworkMetrics(state.Metrics),
		LastSeenAt:        lastSeenAt,
		LastPolledAtUnix:  state.LastPolledAtUnix,
		CollectedAtUnix:   state.CollectedAtUnix,
		ActiveUsers:       append([]string(nil), state.ActiveUsers...),
		ActiveUserCount:   state.ActiveUserCount,
		GPUSummary:        state.GPUSummary,
		AvailabilityScore: state.AvailabilityScore,
		AvailabilityTier:  state.AvailabilityTier,
		DataAgeSec:        cloneFloat64Ptr(state.DataAgeSec),
		Error:             state.LastError,
	}
}

func cloneSystemMetrics(data *model.SystemMetrics) *model.SystemMetrics {
	if data == nil {
		return nil
	}
	cloned := *data
	cloned.Gpus = append([]model.GpuInfo(nil), data.Gpus...)
	cloned.Processes = append([]model.ProcessInfo(nil), data.Processes...)
	return &cloned
}

func cloneNetworkMetrics(metrics *NodeNetworkMetrics) *NodeNetworkMetrics {
	if metrics == nil {
		return nil
	}
	cloned := *metrics
	return &cloned
}

func cloneFloat64Ptr(v *float64) *float64 {
	if v == nil {
		return nil
	}
	c := *v
	return &c
}

func normalizePollTime(now time.Time) time.Time {
	if now.IsZero() {
		return time.Now()
	}
	return now
}
