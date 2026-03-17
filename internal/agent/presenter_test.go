package agent

import (
	"testing"

	"nexus-agent-go/internal/model"
)

func TestBuildMetricsSummary(t *testing.T) {
	gpuIdx := 0
	metrics := model.SystemMetrics{
		Hostname:      "node-a",
		IPAddress:     "10.0.0.8",
		OS:            "linux 6.1",
		UptimeSeconds: 7200,
		UptimeHuman:   "2h 0m",
		CPUModel:      "amd64",
		CPUUsage:      63.2,
		CPUCores:      16,
		RAMTotal:      64,
		RAMUsed:       26.5,
		RAMPercent:    41.4,
		NetSentMB:     512.1,
		NetRecvMB:     1024.3,
		Gpus: []model.GpuInfo{
			{
				ID:                0,
				Name:              "GPU-0",
				Utilization:       40,
				MemoryTotal:       8.0,
				MemoryUsed:        2.5,
				MemoryUtilization: 31.2,
			},
			{
				ID:                1,
				Name:              "GPU-1",
				Utilization:       60,
				MemoryTotal:       16.0,
				MemoryUsed:        4.0,
				MemoryUtilization: 25.0,
			},
		},
		Processes: []model.ProcessInfo{
			{PID: 101, Command: "python train.py", CPUPercent: 13.5, GPUIndex: &gpuIdx},
			{PID: 102, Command: "go run ./cmd/agent", CPUPercent: 44.4},
			{PID: 103, Command: "nv_worker", CPUPercent: 22.2, GPUIndex: &gpuIdx},
		},
	}

	got := BuildMetricsSummary(metrics, 1700001234)

	if got.CollectedAtUnix != 1700001234 {
		t.Fatalf("collected_at_unix mismatch: got=%d", got.CollectedAtUnix)
	}
	if got.Hostname != metrics.Hostname || got.IPAddress != metrics.IPAddress || got.OS != metrics.OS {
		t.Fatalf("basic system fields mismatch: got=%+v", got)
	}
	if got.GPUCount != 2 {
		t.Fatalf("gpu_count mismatch: got=%d", got.GPUCount)
	}
	if got.ProcessCount != 3 {
		t.Fatalf("process_count mismatch: got=%d", got.ProcessCount)
	}
	if got.GPUProcessCount != 2 {
		t.Fatalf("gpu_process_count mismatch: got=%d", got.GPUProcessCount)
	}
	if got.TopCPUProcessCommand != "go run ./cmd/agent" {
		t.Fatalf("top_cpu_process_command mismatch: got=%q", got.TopCPUProcessCommand)
	}
	if got.TopCPUProcessPercent != 44.4 {
		t.Fatalf("top_cpu_process_percent mismatch: got=%v", got.TopCPUProcessPercent)
	}
	if got.GPUUtilizationAvg != 50.0 {
		t.Fatalf("gpu_utilization_avg mismatch: got=%v", got.GPUUtilizationAvg)
	}
	if got.GPUMemoryUsedMBTotal != 6656.0 {
		t.Fatalf("gpu_memory_used_mb_total mismatch: got=%v", got.GPUMemoryUsedMBTotal)
	}
	if got.GPUMemoryTotalMB != 24576.0 {
		t.Fatalf("gpu_memory_total_mb mismatch: got=%v", got.GPUMemoryTotalMB)
	}
	if got.GPUMemoryPercentAvg != 28.1 {
		t.Fatalf("gpu_memory_percent_avg mismatch: got=%v", got.GPUMemoryPercentAvg)
	}
}

func TestBuildProcessesResponse(t *testing.T) {
	gpuIdx := 2
	metrics := model.SystemMetrics{
		Processes: []model.ProcessInfo{
			{PID: 1, Command: "python app.py", CPUPercent: 21.1, GPUIndex: &gpuIdx},
			{PID: 2, Command: "nginx", CPUPercent: 4.2},
		},
	}

	got := BuildProcessesResponse(metrics, 1700005678)

	if got.CollectedAtUnix != 1700005678 {
		t.Fatalf("collected_at_unix mismatch: got=%d", got.CollectedAtUnix)
	}
	if got.ProcessCount != 2 {
		t.Fatalf("process_count mismatch: got=%d", got.ProcessCount)
	}
	if got.GPUProcessCount != 1 {
		t.Fatalf("gpu_process_count mismatch: got=%d", got.GPUProcessCount)
	}
	if len(got.Processes) != 2 || got.Processes[0].PID != 1 || got.Processes[1].PID != 2 {
		t.Fatalf("processes mismatch: got=%+v", got.Processes)
	}

	got.Processes[0].Command = "mutated"
	if metrics.Processes[0].Command == "mutated" {
		t.Fatalf("process slice should be cloned")
	}
}

func TestBuildGPUsResponse(t *testing.T) {
	metrics := model.SystemMetrics{
		Gpus: []model.GpuInfo{
			{ID: 0, Name: "GPU-A", Utilization: 80},
			{ID: 1, Name: "GPU-B", Utilization: 20},
		},
	}

	got := BuildGPUsResponse(metrics, 1700009999)

	if got.CollectedAtUnix != 1700009999 {
		t.Fatalf("collected_at_unix mismatch: got=%d", got.CollectedAtUnix)
	}
	if got.GPUCount != 2 {
		t.Fatalf("gpu_count mismatch: got=%d", got.GPUCount)
	}
	if len(got.GPUs) != 2 || got.GPUs[0].ID != 0 || got.GPUs[1].ID != 1 {
		t.Fatalf("gpus mismatch: got=%+v", got.GPUs)
	}

	got.GPUs[0].Name = "mutated"
	if metrics.Gpus[0].Name == "mutated" {
		t.Fatalf("gpu slice should be cloned")
	}
}

func TestBuildMetricsSummaryNoGPUs(t *testing.T) {
	metrics := model.SystemMetrics{
		Processes: []model.ProcessInfo{
			{PID: 1, Command: "busybox", CPUPercent: 10.0},
		},
	}

	got := BuildMetricsSummary(metrics, 1)

	if got.GPUCount != 0 {
		t.Fatalf("gpu_count mismatch: got=%d", got.GPUCount)
	}
	if got.GPUUtilizationAvg != 0 {
		t.Fatalf("gpu_utilization_avg should be 0, got=%v", got.GPUUtilizationAvg)
	}
	if got.GPUMemoryUsedMBTotal != 0 || got.GPUMemoryTotalMB != 0 || got.GPUMemoryPercentAvg != 0 {
		t.Fatalf("gpu aggregation should be 0, got=%+v", got)
	}
}

func TestBuildMetricsSummaryNoProcesses(t *testing.T) {
	metrics := model.SystemMetrics{
		Gpus: []model.GpuInfo{
			{ID: 0, Utilization: 77, MemoryTotal: 12.0, MemoryUsed: 6.0, MemoryUtilization: 50.0},
		},
	}

	got := BuildMetricsSummary(metrics, 2)

	if got.ProcessCount != 0 {
		t.Fatalf("process_count mismatch: got=%d", got.ProcessCount)
	}
	if got.GPUProcessCount != 0 {
		t.Fatalf("gpu_process_count mismatch: got=%d", got.GPUProcessCount)
	}
	if got.TopCPUProcessCommand != "" {
		t.Fatalf("top_cpu_process_command should be empty, got=%q", got.TopCPUProcessCommand)
	}
	if got.TopCPUProcessPercent != 0 {
		t.Fatalf("top_cpu_process_percent should be 0, got=%v", got.TopCPUProcessPercent)
	}
}
