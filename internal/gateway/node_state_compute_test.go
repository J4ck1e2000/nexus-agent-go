package gateway

import (
	"math"
	"testing"
	"time"

	"nexus-agent-go/internal/model"
)

func TestIsActiveGPUProcess(t *testing.T) {
	gpuIndex0 := 0
	vramHigh := 256
	vramLow := 64

	tests := []struct {
		name string
		proc model.ProcessInfo
		want bool
	}{
		{
			name: "active gpu user",
			proc: model.ProcessInfo{User: "alice", GPUIndex: &gpuIndex0, VRAMUsedMB: &vramHigh},
			want: true,
		},
		{
			name: "system user filtered",
			proc: model.ProcessInfo{User: "root", GPUIndex: &gpuIndex0, VRAMUsedMB: &vramHigh},
			want: false,
		},
		{
			name: "below threshold",
			proc: model.ProcessInfo{User: "alice", GPUIndex: &gpuIndex0, VRAMUsedMB: &vramLow},
			want: false,
		},
		{
			name: "missing gpu index",
			proc: model.ProcessInfo{User: "alice", VRAMUsedMB: &vramHigh},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isActiveGPUProcess(tt.proc)
			if got != tt.want {
				t.Fatalf("isActiveGPUProcess() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestComputeGPUSummary(t *testing.T) {
	gpuIndex0 := 0
	vramHigh := 200
	metrics := &model.SystemMetrics{
		Gpus: []model.GpuInfo{
			{Utilization: 70, MemoryUsed: 8, MemoryTotal: 16, PowerDraw: 200},
			{Utilization: 10, MemoryUsed: 1, MemoryTotal: 16, PowerDraw: 100},
		},
		Processes: []model.ProcessInfo{
			{User: "alice", GPUIndex: &gpuIndex0, VRAMUsedMB: &vramHigh},
		},
	}

	got := computeGPUSummary(metrics)
	if got.GPUCount != 2 {
		t.Fatalf("GPUCount = %d, want 2", got.GPUCount)
	}
	if got.BusyGpuCount != 1 {
		t.Fatalf("BusyGpuCount = %d, want 1", got.BusyGpuCount)
	}
	if got.IdleGpuCount != 1 {
		t.Fatalf("IdleGpuCount = %d, want 1", got.IdleGpuCount)
	}
	if got.AvgUtilization == nil || math.Abs(*got.AvgUtilization-40) > 0.001 {
		t.Fatalf("AvgUtilization = %v, want 40", got.AvgUtilization)
	}
	if got.AvgMemoryPercent == nil || math.Abs(*got.AvgMemoryPercent-28.125) > 0.001 {
		t.Fatalf("AvgMemoryPercent = %v, want 28.125", got.AvgMemoryPercent)
	}
	if math.Abs(got.TotalMemoryUsed-9) > 0.001 {
		t.Fatalf("TotalMemoryUsed = %f, want 9", got.TotalMemoryUsed)
	}
	if math.Abs(got.TotalMemory-32) > 0.001 {
		t.Fatalf("TotalMemory = %f, want 32", got.TotalMemory)
	}
	if math.Abs(got.TotalPower-300) > 0.001 {
		t.Fatalf("TotalPower = %f, want 300", got.TotalPower)
	}
	if math.Abs(got.GpuPressure-35.25) > 0.001 {
		t.Fatalf("GpuPressure = %f, want 35.25", got.GpuPressure)
	}
	if math.Abs(got.BusyRatio-0.5) > 0.0001 {
		t.Fatalf("BusyRatio = %f, want 0.5", got.BusyRatio)
	}
}

func TestComputeAvailabilityScore(t *testing.T) {
	cpu := 50.0
	ram := 40.0
	age := 10.0
	gpuSummary := NodeGPUSummary{
		GPUCount:        2,
		IdleGpuCount:    1,
		BusyRatio:       0.5,
		GpuPressure:     50,
		TotalPower:      200,
		BusyGpuCount:    1,
		TotalMemory:     24,
		TotalMemoryUsed: 10,
	}

	got := computeAvailabilityScore(
		NodeStatusOnline,
		&model.SystemMetrics{CPUUsage: cpu, RAMPercent: ram},
		2,
		gpuSummary,
		&age,
	)
	if got != 32 {
		t.Fatalf("computeAvailabilityScore() = %d, want 32", got)
	}
}

func TestComputeAvailabilityTier(t *testing.T) {
	tests := []struct {
		name   string
		status string
		score  int
		want   string
	}{
		{name: "offline", status: NodeStatusOffline, score: 90, want: AvailabilityTierOffline},
		{name: "highlyAvailable", status: NodeStatusOnline, score: 80, want: AvailabilityTierHighlyAvailable},
		{name: "available", status: NodeStatusOnline, score: 60, want: AvailabilityTierAvailable},
		{name: "busy", status: NodeStatusOnline, score: 35, want: AvailabilityTierBusy},
		{name: "saturated", status: NodeStatusOnline, score: 10, want: AvailabilityTierSaturated},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := computeAvailabilityTier(tt.status, tt.score)
			if got != tt.want {
				t.Fatalf("computeAvailabilityTier() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestApplyDerivedFields_DataAge(t *testing.T) {
	state := pendingNodeState(model.AgentConfig{ID: 1, Name: "n1", URL: "http://127.0.0.1:8005"})
	state.Status = NodeStatusOffline
	state.LastSeenAtUnix = 100

	applyDerivedFields(&state, time.Unix(105, 0))
	if state.DataAgeSec == nil {
		t.Fatalf("DataAgeSec should not be nil")
	}
	if math.Abs(*state.DataAgeSec-5) > 0.0001 {
		t.Fatalf("DataAgeSec = %f, want 5", *state.DataAgeSec)
	}
}
