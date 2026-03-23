package gateway

import (
	"math"
	"sort"
	"strings"
	"time"

	"nexus-agent-go/internal/model"
)

const (
	activeUserVRAMThresholdMB = 0.1 * 1024
	gpuBusyUtilThreshold      = 55
	gpuBusyVRAMThreshold      = 60
	dataStaleWarningSec       = 8
	dataStaleCriticalSec      = 18
)

var systemUserBlacklist = map[string]struct{}{
	"root": {},
	"gdm":  {},
}

func applyDerivedFields(state *NodeState, now time.Time) {
	if state == nil {
		return
	}
	now = normalizePollTime(now)

	activeUsers := computeActiveUsers(state.Data)
	gpuSummary := computeGPUSummary(state.Data)
	ageSec := computeDataAgeSec(state.LastSeenAtUnix, now)

	state.ActiveUsers = activeUsers
	state.ActiveUserCount = len(activeUsers)
	state.GPUSummary = gpuSummary
	state.DataAgeSec = ageSec
	state.AvailabilityScore = computeAvailabilityScore(state.Status, state.Data, len(activeUsers), gpuSummary, ageSec)
	state.AvailabilityTier = computeAvailabilityTier(state.Status, state.AvailabilityScore)
}

func computeActiveUsers(metrics *model.SystemMetrics) []string {
	if metrics == nil || len(metrics.Processes) == 0 {
		return []string{}
	}

	users := make(map[string]struct{})
	for _, proc := range metrics.Processes {
		if !isActiveGPUProcess(proc) {
			continue
		}
		user := resolveProcessUser(proc)
		if user == "" {
			continue
		}
		users[user] = struct{}{}
	}

	result := make([]string, 0, len(users))
	for name := range users {
		result = append(result, name)
	}
	sort.Strings(result)
	return result
}

func computeGPUSummary(metrics *model.SystemMetrics) NodeGPUSummary {
	if metrics == nil || len(metrics.Gpus) == 0 {
		return emptyGPUSummary()
	}

	activeIndexes := activeGPUIndexes(metrics.Processes)
	utilTotal := 0.0
	utilCount := 0
	memoryPercentTotal := 0.0
	memoryPercentCount := 0
	totalMemoryUsed := 0.0
	totalMemory := 0.0
	totalPower := 0.0
	busyGpuCount := 0

	for idx, gpu := range metrics.Gpus {
		utilization := float64(gpu.Utilization)
		memoryUsed := gpu.MemoryUsed
		memoryTotal := gpu.MemoryTotal
		memoryPercent := 0.0
		if memoryTotal > 0 {
			memoryPercent = clampPercent((memoryUsed / memoryTotal) * 100)
		} else {
			memoryPercent = clampPercent(gpu.MemoryUtilization)
		}

		totalMemoryUsed += memoryUsed
		totalMemory += memoryTotal
		totalPower += float64(gpu.PowerDraw)
		utilTotal += utilization
		utilCount++
		memoryPercentTotal += memoryPercent
		memoryPercentCount++

		_, hasActiveProc := activeIndexes[idx]
		isBusy := utilization >= gpuBusyUtilThreshold ||
			memoryPercent >= gpuBusyVRAMThreshold ||
			hasActiveProc
		if isBusy {
			busyGpuCount++
		}
	}

	idleGPUCount := len(metrics.Gpus) - busyGpuCount
	if idleGPUCount < 0 {
		idleGPUCount = 0
	}

	var avgUtil *float64
	var avgMemoryPercent *float64
	if utilCount > 0 {
		v := utilTotal / float64(utilCount)
		avgUtil = &v
	}
	if memoryPercentCount > 0 {
		v := memoryPercentTotal / float64(memoryPercentCount)
		avgMemoryPercent = &v
	}

	avgUtilValue := 0.0
	if avgUtil != nil {
		avgUtilValue = *avgUtil
	}
	avgMemoryValue := 0.0
	if avgMemoryPercent != nil {
		avgMemoryValue = *avgMemoryPercent
	}
	gpuPressure := clampPercent((avgUtilValue * 0.6) + (avgMemoryValue * 0.4))
	busyRatio := 0.0
	if len(metrics.Gpus) > 0 {
		busyRatio = float64(busyGpuCount) / float64(len(metrics.Gpus))
	}

	return NodeGPUSummary{
		GPUCount:         len(metrics.Gpus),
		BusyGpuCount:     busyGpuCount,
		IdleGpuCount:     idleGPUCount,
		AvgUtilization:   avgUtil,
		AvgMemoryPercent: avgMemoryPercent,
		TotalMemoryUsed:  totalMemoryUsed,
		TotalMemory:      totalMemory,
		TotalPower:       totalPower,
		GpuPressure:      gpuPressure,
		BusyRatio:        busyRatio,
	}
}

func computeAvailabilityScore(status string, metrics *model.SystemMetrics, activeUsers int, gpuSummary NodeGPUSummary, ageSec *float64) int {
	if status != NodeStatusOnline {
		return 0
	}

	var cpu *float64
	var ram *float64
	if metrics != nil {
		cpu = &metrics.CPUUsage
		ram = &metrics.RAMPercent
	}

	avgPowerPerGPU := 0.0
	if gpuSummary.GPUCount > 0 {
		avgPowerPerGPU = gpuSummary.TotalPower / float64(gpuSummary.GPUCount)
	}

	score := 100.0
	score -= math.Min(30, float64(activeUsers)*8)
	if cpu != nil {
		score -= math.Min(24, *cpu*0.24)
	} else {
		score -= 6
	}
	if ram != nil {
		score -= math.Min(20, *ram*0.2)
	} else {
		score -= 6
	}
	score -= math.Min(22, gpuSummary.GpuPressure*0.22)
	score -= math.Min(16, gpuSummary.BusyRatio*16)
	score -= math.Min(10, avgPowerPerGPU*0.08)
	score += math.Min(12, float64(gpuSummary.IdleGpuCount)*3)
	if gpuSummary.GPUCount == 0 {
		score -= 8
	}

	if ageSec == nil {
		score -= 18
	} else if *ageSec > dataStaleCriticalSec {
		score -= 18
	} else if *ageSec > dataStaleWarningSec {
		score -= 8
	}

	score = math.Max(0, math.Min(100, score))
	return int(math.Round(score))
}

func computeAvailabilityTier(status string, score int) string {
	if status != NodeStatusOnline {
		return AvailabilityTierOffline
	}
	if score >= 80 {
		return AvailabilityTierHighlyAvailable
	}
	if score >= 60 {
		return AvailabilityTierAvailable
	}
	if score >= 35 {
		return AvailabilityTierBusy
	}
	return AvailabilityTierSaturated
}

func computeDataAgeSec(lastSeenAtUnix int64, now time.Time) *float64 {
	if lastSeenAtUnix <= 0 {
		return nil
	}
	seconds := now.Sub(time.Unix(lastSeenAtUnix, 0)).Seconds()
	if seconds < 0 {
		seconds = 0
	}
	return &seconds
}

func computeNetworkSpeed(current model.SystemMetrics, previous *NodeState, now time.Time) *NodeNetworkMetrics {
	now = normalizePollTime(now)
	result := &NodeNetworkMetrics{UpSpeed: 0, DownSpeed: 0}
	if previous == nil || previous.Data == nil || previous.LastSeenAtUnix <= 0 {
		return result
	}

	deltaSec := now.Sub(time.Unix(previous.LastSeenAtUnix, 0)).Seconds()
	if deltaSec <= 0 {
		return result
	}

	upDiff := current.NetSentMB - previous.Data.NetSentMB
	downDiff := current.NetRecvMB - previous.Data.NetRecvMB
	if upDiff < 0 {
		upDiff = 0
	}
	if downDiff < 0 {
		downDiff = 0
	}

	result.UpSpeed = upDiff / deltaSec
	result.DownSpeed = downDiff / deltaSec
	return result
}

func isActiveGPUProcess(proc model.ProcessInfo) bool {
	user := strings.ToLower(resolveProcessUser(proc))
	if user == "" {
		return false
	}
	if _, blocked := systemUserBlacklist[user]; blocked {
		return false
	}
	if proc.GPUIndex == nil {
		return false
	}

	vramUsed := 0.0
	if proc.VRAMUsedMB != nil {
		vramUsed = float64(*proc.VRAMUsedMB)
	}
	return vramUsed >= activeUserVRAMThresholdMB
}

func activeGPUIndexes(processes []model.ProcessInfo) map[int]struct{} {
	indexes := make(map[int]struct{})
	for _, proc := range processes {
		if !isActiveGPUProcess(proc) || proc.GPUIndex == nil {
			continue
		}
		indexes[*proc.GPUIndex] = struct{}{}
	}
	return indexes
}

func resolveProcessUser(proc model.ProcessInfo) string {
	return strings.TrimSpace(proc.User)
}

func clampPercent(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 100 {
		return 100
	}
	return value
}
