package ai

import (
	"context"
	"strings"
	"time"
)

type staticProvider struct {
	nodes   []NodeSnapshot
	history []NodeHistorySnapshot
	err     error
}

func (p *staticProvider) ListNodeSnapshots(ctx context.Context) ([]NodeSnapshot, error) {
	_ = ctx
	if p.err != nil {
		return nil, p.err
	}
	out := make([]NodeSnapshot, 0, len(p.nodes))
	for _, node := range p.nodes {
		out = append(out, node)
	}
	return out, nil
}

func (p *staticProvider) QueryNodeHistory(ctx context.Context, nodeName *string, window time.Duration) ([]NodeHistorySnapshot, error) {
	_ = ctx
	_ = window
	if p.err != nil {
		return nil, p.err
	}
	if nodeName == nil || strings.TrimSpace(*nodeName) == "" {
		return append([]NodeHistorySnapshot(nil), p.history...), nil
	}

	target := strings.ToLower(strings.TrimSpace(*nodeName))
	result := make([]NodeHistorySnapshot, 0, len(p.history))
	for _, item := range p.history {
		if strings.EqualFold(item.NodeName, target) || strings.EqualFold(strings.ToLower(item.NodeName), target) {
			result = append(result, item)
		}
	}
	return result, nil
}

func floatPtr(v float64) *float64 {
	return &v
}

func intPtr(v int) *int {
	return &v
}

func buildNode(name string, status string, score int, cpu, ram float64, age float64, idleGPU, busyGPU int, gpuUtil, gpuMem float64, gpus []GPUCard) NodeSnapshot {
	totalMemory := 0.0
	usedMemory := 0.0
	for _, gpu := range gpus {
		totalMemory += gpu.MemoryTotalGB
		usedMemory += gpu.MemoryUsedGB
	}
	return NodeSnapshot{
		ID:                int64(len(name)),
		Name:              name,
		Status:            status,
		AvailabilityScore: score,
		AvailabilityTier:  "available",
		DataAgeSec:        floatPtr(age),
		CPUUsage:          floatPtr(cpu),
		RAMPercent:        floatPtr(ram),
		ActiveUserCount:   1,
		GPUSummary: NodeGPUSummary{
			GPUCount:         len(gpus),
			BusyGPUCount:     busyGPU,
			IdleGPUCount:     idleGPU,
			AvgUtilization:   floatPtr(gpuUtil),
			AvgMemoryPercent: floatPtr(gpuMem),
			TotalMemoryGB:    totalMemory,
			UsedMemoryGB:     usedMemory,
			GPUPressure:      (gpuUtil*0.6 + gpuMem*0.4),
			BusyRatio:        0.0,
		},
		GPUs: gpus,
		Processes: []GPUProcess{
			{
				PID:           1,
				User:          "alice",
				Command:       "python train.py",
				GPUIndex:      intPtr(0),
				VRAMUsedMB:    intPtr(1024),
				CPUPercent:    20,
				MemoryPercent: 5,
			},
		},
	}
}
