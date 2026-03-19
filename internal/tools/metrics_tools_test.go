package tools

import (
	"context"
	"errors"
	"testing"

	"nexus-agent-go/internal/model"
)

type fakeReader struct {
	metrics model.SystemMetrics
	err     error
}

func (f *fakeReader) Snapshot() (model.SystemMetrics, error) {
	if f.err != nil {
		return model.SystemMetrics{}, f.err
	}
	return f.metrics, nil
}

type fakeReaderWithMeta struct {
	metrics         model.SystemMetrics
	err             error
	collectedAtUnix int64
}

func (f *fakeReaderWithMeta) Snapshot() (model.SystemMetrics, error) {
	if f.err != nil {
		return model.SystemMetrics{}, f.err
	}
	return f.metrics, nil
}

func (f *fakeReaderWithMeta) SnapshotWithMeta() (model.SystemMetrics, int64, error) {
	if f.err != nil {
		return model.SystemMetrics{}, 0, f.err
	}
	return f.metrics, f.collectedAtUnix, nil
}

func TestMetricsTools_GetCurrentMetrics(t *testing.T) {
	gpuIdx := 0
	want := model.SystemMetrics{
		Hostname: "node-a",
		Gpus: []model.GpuInfo{
			{ID: 0, Name: "GPU-0", Utilization: 70},
		},
		Processes: []model.ProcessInfo{
			{PID: 1001, Command: "python train.py", CPUPercent: 32.2, GPUIndex: &gpuIdx},
		},
	}

	tools := NewMetricsTools(&fakeReader{metrics: want})
	got, err := tools.GetCurrentMetrics(context.Background())
	if err != nil {
		t.Fatalf("GetCurrentMetrics returned error: %v", err)
	}
	if got.Hostname != want.Hostname {
		t.Fatalf("hostname mismatch: got=%q want=%q", got.Hostname, want.Hostname)
	}
	if len(got.Gpus) != 1 || got.Gpus[0].ID != 0 {
		t.Fatalf("gpus mismatch: got=%+v", got.Gpus)
	}
	if len(got.Processes) != 1 || got.Processes[0].PID != 1001 {
		t.Fatalf("processes mismatch: got=%+v", got.Processes)
	}

	readerErr := errors.New("metrics_unavailable")
	tools = NewMetricsTools(&fakeReader{err: readerErr})
	_, err = tools.GetCurrentMetrics(context.Background())
	if !errors.Is(err, readerErr) {
		t.Fatalf("expected error passthrough, got=%v", err)
	}
}

func TestMetricsTools_GetCurrentMetricsWithMeta(t *testing.T) {
	want := model.SystemMetrics{
		Hostname: "node-meta",
		Gpus: []model.GpuInfo{
			{ID: 1, Name: "GPU-1", Utilization: 55},
		},
	}

	tools := NewMetricsTools(&fakeReaderWithMeta{
		metrics:         want,
		collectedAtUnix: 1700010001,
	})
	gotMetrics, gotTS, err := tools.GetCurrentMetricsWithMeta(context.Background())
	if err != nil {
		t.Fatalf("GetCurrentMetricsWithMeta returned error: %v", err)
	}
	if gotTS != 1700010001 {
		t.Fatalf("collected_at_unix mismatch: got=%d", gotTS)
	}
	if gotMetrics.Hostname != want.Hostname {
		t.Fatalf("hostname mismatch: got=%q want=%q", gotMetrics.Hostname, want.Hostname)
	}
	if len(gotMetrics.Gpus) != 1 || gotMetrics.Gpus[0].ID != 1 {
		t.Fatalf("gpus mismatch: got=%+v", gotMetrics.Gpus)
	}
}

func TestMetricsTools_GetMetricsSummary(t *testing.T) {
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
			{ID: 0, Name: "GPU-0", Utilization: 40, MemoryTotal: 8.0, MemoryUsed: 2.5, MemoryUtilization: 31.2},
			{ID: 1, Name: "GPU-1", Utilization: 60, MemoryTotal: 16.0, MemoryUsed: 4.0, MemoryUtilization: 25.0},
		},
		Processes: []model.ProcessInfo{
			{PID: 101, Command: "python train.py", CPUPercent: 13.5, GPUIndex: &gpuIdx},
			{PID: 102, Command: "go run ./cmd/agent", CPUPercent: 44.4},
			{PID: 103, Command: "nv_worker", CPUPercent: 22.2, GPUIndex: &gpuIdx},
		},
	}

	tools := NewMetricsTools(&fakeReaderWithMeta{
		metrics:         metrics,
		collectedAtUnix: 1700001234,
	})
	got, err := tools.GetMetricsSummary(context.Background())
	if err != nil {
		t.Fatalf("GetMetricsSummary returned error: %v", err)
	}

	if got.CollectedAtUnix != 1700001234 {
		t.Fatalf("collected_at_unix mismatch: got=%d", got.CollectedAtUnix)
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

func TestMetricsTools_GetProcesses(t *testing.T) {
	gpuIdx := 2
	metrics := model.SystemMetrics{
		Processes: []model.ProcessInfo{
			{PID: 1, Command: "python app.py", CPUPercent: 21.1, GPUIndex: &gpuIdx},
			{PID: 2, Command: "nginx", CPUPercent: 4.2},
		},
	}

	tools := NewMetricsTools(&fakeReaderWithMeta{
		metrics:         metrics,
		collectedAtUnix: 1700005678,
	})
	got, err := tools.GetProcesses(context.Background())
	if err != nil {
		t.Fatalf("GetProcesses returned error: %v", err)
	}

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

func TestMetricsTools_GetGPUs(t *testing.T) {
	metrics := model.SystemMetrics{
		Gpus: []model.GpuInfo{
			{ID: 0, Name: "GPU-A", Utilization: 40, MemoryTotal: 8.0, MemoryUsed: 2.5, MemoryUtilization: 31.2},
			{ID: 1, Name: "GPU-B", Utilization: 60, MemoryTotal: 16.0, MemoryUsed: 4.0, MemoryUtilization: 25.0},
		},
	}

	tools := NewMetricsTools(&fakeReaderWithMeta{
		metrics:         metrics,
		collectedAtUnix: 1700009999,
	})
	got, err := tools.GetGPUs(context.Background())
	if err != nil {
		t.Fatalf("GetGPUs returned error: %v", err)
	}

	if got.CollectedAtUnix != 1700009999 {
		t.Fatalf("collected_at_unix mismatch: got=%d", got.CollectedAtUnix)
	}
	if got.GPUCount != 2 {
		t.Fatalf("gpu_count mismatch: got=%d", got.GPUCount)
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
	if len(got.GPUs) != 2 || got.GPUs[0].ID != 0 || got.GPUs[1].ID != 1 {
		t.Fatalf("gpus mismatch: got=%+v", got.GPUs)
	}

	got.GPUs[0].Name = "mutated"
	if metrics.Gpus[0].Name == "mutated" {
		t.Fatalf("gpu slice should be cloned")
	}
}

func TestMetricsTools_GetGPUsNoGPUs(t *testing.T) {
	tools := NewMetricsTools(&fakeReader{
		metrics: model.SystemMetrics{},
	})

	got, err := tools.GetGPUs(context.Background())
	if err != nil {
		t.Fatalf("GetGPUs returned error: %v", err)
	}
	if got.GPUCount != 0 {
		t.Fatalf("gpu_count mismatch: got=%d", got.GPUCount)
	}
	if got.GPUUtilizationAvg != 0 || got.GPUMemoryUsedMBTotal != 0 || got.GPUMemoryTotalMB != 0 || got.GPUMemoryPercentAvg != 0 {
		t.Fatalf("gpu aggregation should be 0, got=%+v", got)
	}
}

func TestMetricsTools_GetMetricsSummaryNoProcesses(t *testing.T) {
	metrics := model.SystemMetrics{
		Gpus: []model.GpuInfo{
			{ID: 0, Utilization: 77, MemoryTotal: 12.0, MemoryUsed: 6.0, MemoryUtilization: 50.0},
		},
	}

	tools := NewMetricsTools(&fakeReader{
		metrics: metrics,
	})
	got, err := tools.GetMetricsSummary(context.Background())
	if err != nil {
		t.Fatalf("GetMetricsSummary returned error: %v", err)
	}

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
