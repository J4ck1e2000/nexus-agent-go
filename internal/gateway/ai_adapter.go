package gateway

import (
	"context"
	"fmt"
	"strings"
	"time"

	"nexus-agent-go/internal/ai"
	"nexus-agent-go/internal/model"
)

// AIDataAdapter adapts gateway aggregate node views for AI toolbox interfaces.
type AIDataAdapter struct {
	nodeStateService *NodeStateService
	nodeStateStore   *NodeStateStore
}

// NewAIDataAdapter creates an adapter from gateway services to AI providers.
func NewAIDataAdapter(nodeStateService *NodeStateService, nodeStateStore *NodeStateStore) *AIDataAdapter {
	return &AIDataAdapter{
		nodeStateService: nodeStateService,
		nodeStateStore:   nodeStateStore,
	}
}

// ListNodeSnapshots provides current node aggregate snapshots for AI tools.
func (a *AIDataAdapter) ListNodeSnapshots(ctx context.Context) ([]ai.NodeSnapshot, error) {
	if a == nil || a.nodeStateService == nil {
		return nil, fmt.Errorf("node state service is not initialized")
	}
	overview, err := a.nodeStateService.GetNodesOverview(ctx)
	if err != nil {
		return nil, err
	}

	result := make([]ai.NodeSnapshot, 0, len(overview))
	for _, node := range overview {
		result = append(result, convertOverviewToAISnapshot(node))
	}
	return result, nil
}

// QueryNodeHistory returns compact snapshots in the selected time window.
func (a *AIDataAdapter) QueryNodeHistory(ctx context.Context, nodeName *string, window time.Duration) ([]ai.NodeHistorySnapshot, error) {
	if a == nil || a.nodeStateStore == nil {
		return []ai.NodeHistorySnapshot{}, nil
	}
	if window <= 0 {
		window = 30 * time.Minute
	}
	sinceUnix := time.Now().Add(-window).Unix()

	if nodeName != nil && strings.TrimSpace(*nodeName) != "" {
		nodeID, resolvedName, err := a.resolveNodeIDByName(ctx, strings.TrimSpace(*nodeName))
		if err != nil {
			return nil, err
		}
		series, err := a.nodeStateStore.LoadNodeHistorySince(ctx, nodeID, sinceUnix)
		if err != nil {
			return nil, err
		}
		result := make([]ai.NodeHistorySnapshot, 0, len(series))
		for _, item := range series {
			result = append(result, convertHistorySnapshot(item, resolvedName))
		}
		return result, nil
	}

	allSeries, err := a.nodeStateStore.LoadAllNodeHistorySince(ctx, sinceUnix)
	if err != nil {
		return nil, err
	}
	result := make([]ai.NodeHistorySnapshot, 0)
	for _, nodeSeries := range allSeries {
		for _, item := range nodeSeries {
			result = append(result, convertHistorySnapshot(item, item.NodeName))
		}
	}
	return result, nil
}

func (a *AIDataAdapter) resolveNodeIDByName(ctx context.Context, nodeName string) (int64, string, error) {
	if a == nil || a.nodeStateService == nil {
		return 0, "", fmt.Errorf("node state service is not initialized")
	}
	overview, err := a.nodeStateService.GetNodesOverview(ctx)
	if err != nil {
		return 0, "", err
	}
	for _, node := range overview {
		if strings.EqualFold(node.Name, nodeName) {
			return node.ID, node.Name, nil
		}
	}
	for _, node := range overview {
		if strings.Contains(strings.ToLower(node.Name), strings.ToLower(nodeName)) {
			return node.ID, node.Name, nil
		}
	}
	return 0, "", fmt.Errorf("%w: %s", ai.ErrNodeNotFound, nodeName)
}

func convertOverviewToAISnapshot(node NodeOverview) ai.NodeSnapshot {
	snapshot := ai.NodeSnapshot{
		ID:                node.ID,
		Name:              node.Name,
		URL:               node.URL,
		Status:            node.Status,
		LastPolledAtUnix:  node.LastPolledAtUnix,
		CollectedAtUnix:   node.CollectedAtUnix,
		AvailabilityScore: node.AvailabilityScore,
		AvailabilityTier:  node.AvailabilityTier,
		DataAgeSec:        cloneFloat64Ptr(node.DataAgeSec),
		ActiveUsers:       append([]string(nil), node.ActiveUsers...),
		ActiveUserCount:   node.ActiveUserCount,
		GPUSummary: ai.NodeGPUSummary{
			GPUCount:         node.GPUSummary.GPUCount,
			BusyGPUCount:     node.GPUSummary.BusyGpuCount,
			IdleGPUCount:     node.GPUSummary.IdleGpuCount,
			AvgUtilization:   cloneFloat64Ptr(node.GPUSummary.AvgUtilization),
			AvgMemoryPercent: cloneFloat64Ptr(node.GPUSummary.AvgMemoryPercent),
			TotalMemoryGB:    node.GPUSummary.TotalMemory,
			UsedMemoryGB:     node.GPUSummary.TotalMemoryUsed,
			GPUPressure:      node.GPUSummary.GpuPressure,
			BusyRatio:        node.GPUSummary.BusyRatio,
		},
		Error: node.Error,
	}

	if node.Data != nil {
		snapshot.CPUUsage = cloneFloat64Ptr(&node.Data.CPUUsage)
		snapshot.RAMPercent = cloneFloat64Ptr(&node.Data.RAMPercent)
		snapshot.GPUs = convertGPUCards(node.Data.Gpus)
		snapshot.Processes = convertProcesses(node.Data.Processes)
	}
	return snapshot
}

func convertGPUCards(gpus []model.GpuInfo) []ai.GPUCard {
	result := make([]ai.GPUCard, 0, len(gpus))
	for _, gpu := range gpus {
		estimatedFree := gpu.MemoryTotal - gpu.MemoryUsed
		if estimatedFree < 0 {
			estimatedFree = 0
		}
		result = append(result, ai.GPUCard{
			Index:           gpu.ID,
			Name:            gpu.Name,
			Utilization:     float64(gpu.Utilization),
			MemoryTotalGB:   gpu.MemoryTotal,
			MemoryUsedGB:    gpu.MemoryUsed,
			MemoryPercent:   gpu.MemoryUtilization,
			EstimatedFreeGB: estimatedFree,
		})
	}
	return result
}

func convertProcesses(processes []model.ProcessInfo) []ai.GPUProcess {
	result := make([]ai.GPUProcess, 0, len(processes))
	for _, process := range processes {
		var gpuIndex *int
		if process.GPUIndex != nil {
			idx := *process.GPUIndex
			gpuIndex = &idx
		}
		var vram *int
		if process.VRAMUsedMB != nil {
			value := *process.VRAMUsedMB
			vram = &value
		}
		result = append(result, ai.GPUProcess{
			PID:           process.PID,
			User:          process.User,
			Command:       process.Command,
			GPUIndex:      gpuIndex,
			VRAMUsedMB:    vram,
			CPUPercent:    process.CPUPercent,
			MemoryPercent: process.MemoryPercent,
		})
	}
	return result
}

func convertHistorySnapshot(snapshot NodeHistorySnapshot, fallbackNodeName string) ai.NodeHistorySnapshot {
	nodeName := strings.TrimSpace(snapshot.NodeName)
	if nodeName == "" {
		nodeName = strings.TrimSpace(fallbackNodeName)
	}
	return ai.NodeHistorySnapshot{
		TimestampUnix:     snapshot.TimestampUnix,
		NodeName:          nodeName,
		Status:            snapshot.Status,
		AvailabilityScore: snapshot.AvailabilityScore,
		AvailabilityTier:  snapshot.AvailabilityTier,
		CPUUsage:          cloneFloat64Ptr(snapshot.CPUUsage),
		RAMPercent:        cloneFloat64Ptr(snapshot.RAMPercent),
		GPUPressure:       snapshot.GPUSummary.GpuPressure,
		BusyRatio:         snapshot.GPUSummary.BusyRatio,
		ActiveUserCount:   snapshot.ActiveUserCount,
		DataAgeSec:        cloneFloat64Ptr(snapshot.DataAgeSec),
		RiskFlags:         append([]string(nil), snapshot.RiskFlags...),
	}
}
