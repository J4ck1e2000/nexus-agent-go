package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	defaultNodeHistoryRetention = 25 * time.Hour
	nodeHistoryKeyPattern       = "node:%d:history"
)

// NodeHistorySnapshot is a compact, query-friendly timeline record for one node.
type NodeHistorySnapshot struct {
	TimestampUnix     int64            `json:"timestampUnix"`
	NodeID            int64            `json:"nodeId"`
	NodeName          string           `json:"nodeName"`
	Status            string           `json:"status"`
	AvailabilityScore int              `json:"availabilityScore"`
	AvailabilityTier  string           `json:"availabilityTier"`
	CPUUsage          *float64         `json:"cpuUsage,omitempty"`
	RAMPercent        *float64         `json:"ramPercent,omitempty"`
	GPUSummary        NodeGPUSummary   `json:"gpuSummary"`
	ActiveUserCount   int              `json:"activeUserCount"`
	DataAgeSec        *float64         `json:"dataAgeSec,omitempty"`
	RiskFlags         []string         `json:"riskFlags,omitempty"`
	GPUs              []NodeGPUHistory `json:"gpus,omitempty"`
}

// NodeGPUHistory 保留单卡趋势所需的紧凑指标，不包含进程命令或用户信息。
type NodeGPUHistory struct {
	ID           int     `json:"id"`
	Name         string  `json:"name"`
	Utilization  int     `json:"utilization"`
	MemoryUsed   float64 `json:"memoryUsed"`
	MemoryTotal  float64 `json:"memoryTotal"`
	Temperature  int     `json:"temperature"`
	PowerDraw    int     `json:"powerDraw"`
	ProcessCount int     `json:"processCount"`
}

func marshalNodeHistorySnapshot(state NodeState) (string, int64, error) {
	snapshot := buildNodeHistorySnapshot(state)
	raw, err := json.Marshal(snapshot)
	if err != nil {
		return "", 0, fmt.Errorf("marshal node history snapshot failed: %w", err)
	}
	return string(raw), snapshot.TimestampUnix, nil
}

func buildNodeHistorySnapshot(state NodeState) NodeHistorySnapshot {
	timestamp := state.LastPolledAtUnix
	if timestamp <= 0 {
		timestamp = time.Now().Unix()
	}

	var cpuUsage *float64
	var ramPercent *float64
	if state.Data != nil {
		cpuUsage = cloneFloat64Ptr(&state.Data.CPUUsage)
		ramPercent = cloneFloat64Ptr(&state.Data.RAMPercent)
	}

	gpus := make([]NodeGPUHistory, 0)
	if state.Data != nil {
		gpus = make([]NodeGPUHistory, 0, len(state.Data.Gpus))
		for _, gpu := range state.Data.Gpus {
			processCount := 0
			for _, process := range state.Data.Processes {
				if process.GPUIndex != nil && *process.GPUIndex == gpu.ID && process.VRAMUsedMB != nil && *process.VRAMUsedMB >= 100 {
					processCount++
				}
			}
			gpus = append(gpus, NodeGPUHistory{
				ID:           gpu.ID,
				Name:         gpu.Name,
				Utilization:  gpu.Utilization,
				MemoryUsed:   gpu.MemoryUsed,
				MemoryTotal:  gpu.MemoryTotal,
				Temperature:  gpu.Temperature,
				PowerDraw:    gpu.PowerDraw,
				ProcessCount: processCount,
			})
		}
	}

	return NodeHistorySnapshot{
		TimestampUnix:     timestamp,
		NodeID:            state.ID,
		NodeName:          strings.TrimSpace(state.Name),
		Status:            state.Status,
		AvailabilityScore: state.AvailabilityScore,
		AvailabilityTier:  state.AvailabilityTier,
		CPUUsage:          cpuUsage,
		RAMPercent:        ramPercent,
		GPUSummary:        state.GPUSummary,
		ActiveUserCount:   state.ActiveUserCount,
		DataAgeSec:        cloneFloat64Ptr(state.DataAgeSec),
		RiskFlags:         computeHistoryRiskFlags(state),
		GPUs:              gpus,
	}
}

func computeHistoryRiskFlags(state NodeState) []string {
	flags := make([]string, 0, 8)
	seen := map[string]struct{}{}
	add := func(flag string) {
		flag = strings.TrimSpace(flag)
		if flag == "" {
			return
		}
		if _, ok := seen[flag]; ok {
			return
		}
		seen[flag] = struct{}{}
		flags = append(flags, flag)
	}

	if state.Status != NodeStatusOnline {
		add("offline")
	}
	if state.DataAgeSec == nil {
		add("missing_data_age")
	} else if *state.DataAgeSec > dataStaleCriticalSec {
		add("stale_data")
	}
	if state.AvailabilityScore <= 35 {
		add("low_availability_score")
	}
	if state.GPUSummary.GpuPressure >= 85 {
		add("high_gpu_pressure")
	}
	if state.GPUSummary.BusyRatio >= 0.9 {
		add("high_gpu_busy_ratio")
	}
	if state.ActiveUserCount >= 3 {
		add("many_active_users")
	}
	if state.Data != nil {
		if state.Data.CPUUsage >= 85 {
			add("high_cpu")
		}
		if state.Data.RAMPercent >= 90 {
			add("high_ram")
		}
	}

	sort.Strings(flags)
	return flags
}

// LoadNodeHistorySince loads one node's compact history snapshots since a unix timestamp.
func (s *NodeStateStore) LoadNodeHistorySince(ctx context.Context, nodeID int64, sinceUnix int64) ([]NodeHistorySnapshot, error) {
	if s == nil || s.client == nil {
		return nil, fmt.Errorf("node state store is nil")
	}

	minScore := strconv.FormatInt(sinceUnix, 10)
	values, err := s.client.ZRangeByScore(ctx, s.nodeHistoryKey(nodeID), &redis.ZRangeBy{
		Min: minScore,
		Max: "+inf",
	}).Result()
	if err != nil {
		return nil, fmt.Errorf("load node history failed: %w", err)
	}
	if len(values) == 0 {
		return []NodeHistorySnapshot{}, nil
	}

	snapshots := make([]NodeHistorySnapshot, 0, len(values))
	for _, item := range values {
		var snapshot NodeHistorySnapshot
		if err := json.Unmarshal([]byte(item), &snapshot); err != nil {
			continue
		}
		snapshots = append(snapshots, snapshot)
	}
	return snapshots, nil
}

// LoadAllNodeHistorySince loads snapshots of all tracked nodes since a unix timestamp.
func (s *NodeStateStore) LoadAllNodeHistorySince(ctx context.Context, sinceUnix int64) (map[int64][]NodeHistorySnapshot, error) {
	result := map[int64][]NodeHistorySnapshot{}
	if s == nil || s.client == nil {
		return nil, fmt.Errorf("node state store is nil")
	}

	trackedMembers, err := s.client.SMembers(ctx, s.nodesTrackedKey()).Result()
	if err != nil {
		return nil, fmt.Errorf("load tracked nodes failed: %w", err)
	}
	for _, member := range trackedMembers {
		nodeID, err := strconv.ParseInt(member, 10, 64)
		if err != nil {
			continue
		}
		snapshots, err := s.LoadNodeHistorySince(ctx, nodeID, sinceUnix)
		if err != nil {
			return nil, err
		}
		if len(snapshots) == 0 {
			continue
		}
		result[nodeID] = snapshots
	}
	return result, nil
}

func (s *NodeStateStore) nodeHistoryKey(nodeID int64) string {
	return s.key(fmt.Sprintf(nodeHistoryKeyPattern, nodeID))
}
