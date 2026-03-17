package agent

import "nexus-agent-go/internal/model"

// BuildMetricsSummary converts a snapshot into /metrics/summary payload.
func BuildMetricsSummary(metrics model.SystemMetrics, collectedAtUnix int64) model.MetricsSummary {
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

	gpuUtilizationTotal := 0.0
	gpuMemoryUsedMBTotal := 0.0
	gpuMemoryTotalMB := 0.0
	gpuMemoryPercentTotal := 0.0
	for _, gpu := range metrics.Gpus {
		gpuUtilizationTotal += float64(gpu.Utilization)
		gpuMemoryUsedMBTotal += gpu.MemoryUsed * 1024
		gpuMemoryTotalMB += gpu.MemoryTotal * 1024
		gpuMemoryPercentTotal += gpu.MemoryUtilization
	}

	gpuUtilizationAvg := 0.0
	gpuMemoryPercentAvg := 0.0
	if gpuCount > 0 {
		gpuUtilizationAvg = round1(gpuUtilizationTotal / float64(gpuCount))
		gpuMemoryPercentAvg = round1(gpuMemoryPercentTotal / float64(gpuCount))
	}

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
		GPUMemoryUsedMBTotal: round1(gpuMemoryUsedMBTotal),
		GPUMemoryTotalMB:     round1(gpuMemoryTotalMB),
		GPUMemoryPercentAvg:  gpuMemoryPercentAvg,
	}
}

// BuildProcessesResponse converts a snapshot into /metrics/processes payload.
func BuildProcessesResponse(metrics model.SystemMetrics, collectedAtUnix int64) model.ProcessesResponse {
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

// BuildGPUsResponse converts a snapshot into /metrics/gpus payload.
func BuildGPUsResponse(metrics model.SystemMetrics, collectedAtUnix int64) model.GPUsResponse {
	gpus := append([]model.GpuInfo(nil), metrics.Gpus...)
	return model.GPUsResponse{
		CollectedAtUnix: collectedAtUnix,
		GPUCount:        len(gpus),
		GPUs:            gpus,
	}
}
