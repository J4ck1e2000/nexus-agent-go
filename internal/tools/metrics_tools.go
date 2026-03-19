package tools

import (
	"context"
	"math"

	"nexus-agent-go/internal/model"
)

// MetricsReader defines the read-only dependency for tool layer.
type MetricsReader interface {
	Snapshot() (model.SystemMetrics, error)
}

type metricsMetaReader interface {
	SnapshotWithMeta() (model.SystemMetrics, int64, error)
}

// MetricsTools provides read-only tools for current metrics data.
type MetricsTools struct {
	reader MetricsReader
}

// NewMetricsTools creates a metrics tool service.
func NewMetricsTools(reader MetricsReader) *MetricsTools {
	return &MetricsTools{reader: reader}
}

// GetCurrentMetrics returns the latest cached metrics snapshot.
func (t *MetricsTools) GetCurrentMetrics(ctx context.Context) (model.SystemMetrics, error) {
	_ = ctx
	return t.reader.Snapshot()
}

// GetCurrentMetricsWithMeta returns one consistent snapshot with collection timestamp.
func (t *MetricsTools) GetCurrentMetricsWithMeta(ctx context.Context) (model.SystemMetrics, int64, error) {
	return t.readSnapshot(ctx)
}

// GetMetricsSummary returns the summary view from current snapshot.
func (t *MetricsTools) GetMetricsSummary(ctx context.Context) (model.MetricsSummary, error) {
	metrics, collectedAtUnix, err := t.readSnapshot(ctx)
	if err != nil {
		return model.MetricsSummary{}, err
	}
	return buildSummary(metrics, collectedAtUnix), nil
}

// GetProcesses returns process-focused view from current snapshot.
func (t *MetricsTools) GetProcesses(ctx context.Context) (model.ProcessesResponse, error) {
	metrics, collectedAtUnix, err := t.readSnapshot(ctx)
	if err != nil {
		return model.ProcessesResponse{}, err
	}
	return buildProcessesResponse(metrics, collectedAtUnix), nil
}

// GetGPUs returns GPU-focused view from current snapshot.
func (t *MetricsTools) GetGPUs(ctx context.Context) (model.GPUsResponse, error) {
	metrics, collectedAtUnix, err := t.readSnapshot(ctx)
	if err != nil {
		return model.GPUsResponse{}, err
	}
	return buildGPUsResponse(metrics, collectedAtUnix), nil
}

func (t *MetricsTools) readSnapshot(ctx context.Context) (model.SystemMetrics, int64, error) {
	_ = ctx

	metrics, err := t.reader.Snapshot()
	if err != nil {
		return model.SystemMetrics{}, 0, err
	}

	collectedAtUnix := int64(0)
	if withMeta, ok := t.reader.(metricsMetaReader); ok {
		metricsWithMeta, ts, err := withMeta.SnapshotWithMeta()
		if err == nil {
			metrics = metricsWithMeta
			collectedAtUnix = ts
		}
	}

	return metrics, collectedAtUnix, nil
}

func buildSummary(metrics model.SystemMetrics, collectedAtUnix int64) model.MetricsSummary {
	gpuCount := len(metrics.Gpus)
	processCount := len(metrics.Processes)

	gpuProcessCount := 0
	topCPUProcessPercent := 0.0
	topCPUProcessCommand := ""
	for _, proc := range metrics.Processes {
		if proc.GPUIndex != nil {
			gpuProcessCount++
		}
		if proc.CPUPercent > topCPUProcessPercent {
			topCPUProcessPercent = proc.CPUPercent
			topCPUProcessCommand = proc.Command
		}
	}

	gpuUtilizationAvg, gpuMemoryUsedMBTotal, gpuMemoryTotalMB, gpuMemoryPercentAvg := buildGPUStats(metrics.Gpus)

	return model.MetricsSummary{
		CollectedAtUnix:      collectedAtUnix,
		Hostname:             metrics.Hostname,
		IPAddress:            metrics.IPAddress,
		OS:                   metrics.OS,
		UptimeSeconds:        metrics.UptimeSeconds,
		UptimeHuman:          metrics.UptimeHuman,
		CPUModel:             metrics.CPUModel,
		CPUUsage:             metrics.CPUUsage,
		CPUCores:             metrics.CPUCores,
		RAMTotal:             metrics.RAMTotal,
		RAMUsed:              metrics.RAMUsed,
		RAMPercent:           metrics.RAMPercent,
		NetSentMB:            metrics.NetSentMB,
		NetRecvMB:            metrics.NetRecvMB,
		GPUCount:             gpuCount,
		ProcessCount:         processCount,
		GPUProcessCount:      gpuProcessCount,
		TopCPUProcessCommand: topCPUProcessCommand,
		TopCPUProcessPercent: topCPUProcessPercent,
		GPUUtilizationAvg:    gpuUtilizationAvg,
		GPUMemoryUsedMBTotal: gpuMemoryUsedMBTotal,
		GPUMemoryTotalMB:     gpuMemoryTotalMB,
		GPUMemoryPercentAvg:  gpuMemoryPercentAvg,
	}
}

func buildProcessesResponse(metrics model.SystemMetrics, collectedAtUnix int64) model.ProcessesResponse {
	processes := append([]model.ProcessInfo(nil), metrics.Processes...)
	gpuProcessCount := 0
	for _, proc := range processes {
		if proc.GPUIndex != nil {
			gpuProcessCount++
		}
	}

	return model.ProcessesResponse{
		CollectedAtUnix: collectedAtUnix,
		ProcessCount:    len(processes),
		GPUProcessCount: gpuProcessCount,
		Processes:       processes,
	}
}

func buildGPUsResponse(metrics model.SystemMetrics, collectedAtUnix int64) model.GPUsResponse {
	gpus := append([]model.GpuInfo(nil), metrics.Gpus...)
	gpuUtilizationAvg, gpuMemoryUsedMBTotal, gpuMemoryTotalMB, gpuMemoryPercentAvg := buildGPUStats(gpus)

	return model.GPUsResponse{
		CollectedAtUnix:      collectedAtUnix,
		GPUCount:             len(gpus),
		GPUUtilizationAvg:    gpuUtilizationAvg,
		GPUMemoryUsedMBTotal: gpuMemoryUsedMBTotal,
		GPUMemoryTotalMB:     gpuMemoryTotalMB,
		GPUMemoryPercentAvg:  gpuMemoryPercentAvg,
		GPUs:                 gpus,
	}
}

func buildGPUStats(gpus []model.GpuInfo) (float64, float64, float64, float64) {
	if len(gpus) == 0 {
		return 0, 0, 0, 0
	}

	gpuUtilizationTotal := 0.0
	gpuMemoryUsedMBTotal := 0.0
	gpuMemoryTotalMB := 0.0
	gpuMemoryPercentTotal := 0.0
	for _, gpu := range gpus {
		gpuUtilizationTotal += float64(gpu.Utilization)
		gpuMemoryUsedMBTotal += gpu.MemoryUsed * 1024
		gpuMemoryTotalMB += gpu.MemoryTotal * 1024
		gpuMemoryPercentTotal += gpu.MemoryUtilization
	}

	gpuCount := float64(len(gpus))
	return round1(gpuUtilizationTotal / gpuCount),
		round1(gpuMemoryUsedMBTotal),
		round1(gpuMemoryTotalMB),
		round1(gpuMemoryPercentTotal / gpuCount)
}

func round1(v float64) float64 {
	return math.Round(v*10) / 10
}
