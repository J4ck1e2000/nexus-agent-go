package gateway

import (
	"sort"
)

const (
	HistoryAggregationAverage = "average"
	HistoryAggregationPeak    = "peak"
)

type historyBucketAccumulator struct {
	snapshot     NodeHistorySnapshot
	count        int
	cpuTotal     float64
	cpuCount     int
	cpuMax       float64
	ramTotal     float64
	ramCount     int
	ramMax       float64
	scoreTotal   int
	userTotal    int
	maxUsers     int
	dataAgeTotal float64
	dataAgeCount int
	maxDataAge   float64
	minScore     int
	gpuByID      map[int]*gpuHistoryAccumulator
	flags        map[string]struct{}
}

type gpuHistoryAccumulator struct {
	metric      NodeGPUHistory
	count       int
	utilTotal   float64
	memoryTotal float64
	tempTotal   float64
	powerTotal  float64
}

// DownsampleNodeHistory bounds history responses while preserving short-lived resource spikes.
// Peak mode is used for idle eligibility; average mode is used for charts and heatmaps.
func DownsampleNodeHistory(samples []NodeHistorySnapshot, stepSeconds int64, aggregation string) []NodeHistorySnapshot {
	if stepSeconds <= 0 || len(samples) < 2 || (aggregation != HistoryAggregationAverage && aggregation != HistoryAggregationPeak) {
		return samples
	}
	ordered := append([]NodeHistorySnapshot(nil), samples...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].TimestampUnix < ordered[j].TimestampUnix })
	buckets := make(map[int64]*historyBucketAccumulator)
	for _, sample := range ordered {
		bucketID := sample.TimestampUnix / stepSeconds
		accumulator := buckets[bucketID]
		if accumulator == nil {
			accumulator = &historyBucketAccumulator{
				snapshot: sample,
				count:    0,
				minScore: sample.AvailabilityScore,
				gpuByID:  make(map[int]*gpuHistoryAccumulator),
				flags:    make(map[string]struct{}),
			}
			accumulator.snapshot.GPUs = []NodeGPUHistory{}
			accumulator.snapshot.RiskFlags = []string{}
			buckets[bucketID] = accumulator
		}
		accumulator.count++
		if sample.TimestampUnix >= accumulator.snapshot.TimestampUnix {
			accumulator.snapshot.TimestampUnix = sample.TimestampUnix
			accumulator.snapshot.NodeName = sample.NodeName
		}
		if sample.Status != NodeStatusOnline {
			accumulator.snapshot.Status = sample.Status
		}
		accumulator.snapshot.NodeID = sample.NodeID
		if sample.CPUUsage != nil {
			accumulator.cpuTotal += *sample.CPUUsage
			accumulator.cpuCount++
			accumulator.cpuMax = max(accumulator.cpuMax, *sample.CPUUsage)
		}
		if sample.RAMPercent != nil {
			accumulator.ramTotal += *sample.RAMPercent
			accumulator.ramCount++
			accumulator.ramMax = max(accumulator.ramMax, *sample.RAMPercent)
		}
		accumulator.scoreTotal += sample.AvailabilityScore
		accumulator.userTotal += sample.ActiveUserCount
		accumulator.maxUsers = max(accumulator.maxUsers, sample.ActiveUserCount)
		if sample.AvailabilityScore < accumulator.minScore {
			accumulator.minScore = sample.AvailabilityScore
		}
		if sample.DataAgeSec != nil {
			accumulator.dataAgeTotal += *sample.DataAgeSec
			accumulator.dataAgeCount++
			if *sample.DataAgeSec > accumulator.maxDataAge {
				accumulator.maxDataAge = *sample.DataAgeSec
			}
		}
		for _, flag := range sample.RiskFlags {
			accumulator.flags[flag] = struct{}{}
		}
		for _, gpu := range sample.GPUs {
			gpuAccumulator := accumulator.gpuByID[gpu.ID]
			if gpuAccumulator == nil {
				gpuAccumulator = &gpuHistoryAccumulator{metric: gpu}
				accumulator.gpuByID[gpu.ID] = gpuAccumulator
			}
			gpuAccumulator.count++
			gpuAccumulator.metric.Name = gpu.Name
			if aggregation == HistoryAggregationPeak {
				gpuAccumulator.metric.Utilization = max(gpuAccumulator.metric.Utilization, gpu.Utilization)
				gpuAccumulator.metric.MemoryUsed = max(gpuAccumulator.metric.MemoryUsed, gpu.MemoryUsed)
				gpuAccumulator.metric.Temperature = max(gpuAccumulator.metric.Temperature, gpu.Temperature)
				gpuAccumulator.metric.PowerDraw = max(gpuAccumulator.metric.PowerDraw, gpu.PowerDraw)
			} else {
				gpuAccumulator.utilTotal += float64(gpu.Utilization)
				gpuAccumulator.memoryTotal += gpu.MemoryUsed
				gpuAccumulator.tempTotal += float64(gpu.Temperature)
				gpuAccumulator.powerTotal += float64(gpu.PowerDraw)
			}
			gpuAccumulator.metric.MemoryTotal = gpu.MemoryTotal
			gpuAccumulator.metric.ProcessCount = max(gpuAccumulator.metric.ProcessCount, gpu.ProcessCount)
		}
	}

	ids := make([]int64, 0, len(buckets))
	for id := range buckets {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	result := make([]NodeHistorySnapshot, 0, len(ids))
	for _, id := range ids {
		bucket := buckets[id]
		if bucket.count == 0 {
			continue
		}
		if aggregation == HistoryAggregationAverage {
			if bucket.cpuCount > 0 {
				bucket.snapshot.CPUUsage = floatPointer(bucket.cpuTotal / float64(bucket.cpuCount))
			}
			if bucket.ramCount > 0 {
				bucket.snapshot.RAMPercent = floatPointer(bucket.ramTotal / float64(bucket.ramCount))
			}
			bucket.snapshot.AvailabilityScore = bucket.scoreTotal / bucket.count
			bucket.snapshot.ActiveUserCount = bucket.userTotal / bucket.count
		} else {
			bucket.snapshot.CPUUsage = nil
			bucket.snapshot.RAMPercent = nil
			if bucket.cpuCount == bucket.count {
				bucket.snapshot.CPUUsage = floatPointer(bucket.cpuMax)
			}
			if bucket.ramCount == bucket.count {
				bucket.snapshot.RAMPercent = floatPointer(bucket.ramMax)
			}
			bucket.snapshot.AvailabilityScore = bucket.minScore
			bucket.snapshot.ActiveUserCount = bucket.maxUsers
		}
		if bucket.dataAgeCount > 0 {
			if aggregation == HistoryAggregationPeak {
				bucket.snapshot.DataAgeSec = floatPointer(bucket.maxDataAge)
			} else {
				bucket.snapshot.DataAgeSec = floatPointer(bucket.dataAgeTotal / float64(bucket.dataAgeCount))
			}
		}
		for flag := range bucket.flags {
			bucket.snapshot.RiskFlags = append(bucket.snapshot.RiskFlags, flag)
		}
		sort.Strings(bucket.snapshot.RiskFlags)
		bucket.snapshot.GPUs = make([]NodeGPUHistory, 0, len(bucket.gpuByID))
		for _, gpu := range bucket.gpuByID {
			if aggregation == HistoryAggregationPeak && gpu.count != bucket.count {
				continue
			}
			if aggregation == HistoryAggregationAverage && gpu.count > 0 {
				gpu.metric.Utilization = int(gpu.utilTotal / float64(gpu.count))
				gpu.metric.MemoryUsed = gpu.memoryTotal / float64(gpu.count)
				gpu.metric.Temperature = int(gpu.tempTotal / float64(gpu.count))
				gpu.metric.PowerDraw = int(gpu.powerTotal / float64(gpu.count))
			}
			bucket.snapshot.GPUs = append(bucket.snapshot.GPUs, gpu.metric)
		}
		sort.Slice(bucket.snapshot.GPUs, func(i, j int) bool { return bucket.snapshot.GPUs[i].ID < bucket.snapshot.GPUs[j].ID })
		bucket.snapshot.GPUSummary = summarizeHistoryGPUs(bucket.snapshot.GPUs)
		bucket.snapshot.AvailabilityTier = computeAvailabilityTier(bucket.snapshot.Status, bucket.snapshot.AvailabilityScore)
		result = append(result, bucket.snapshot)
	}
	return result
}

func floatPointer(value float64) *float64 {
	result := value
	return &result
}

func summarizeHistoryGPUs(gpus []NodeGPUHistory) NodeGPUSummary {
	summary := emptyGPUSummary()
	summary.GPUCount = len(gpus)
	if len(gpus) == 0 {
		return summary
	}
	utilizationTotal := 0.0
	memoryPercentTotal := 0.0
	for _, gpu := range gpus {
		memoryPercent := 0.0
		if gpu.MemoryTotal > 0 {
			memoryPercent = gpu.MemoryUsed / gpu.MemoryTotal * 100
		}
		utilizationTotal += float64(gpu.Utilization)
		memoryPercentTotal += memoryPercent
		summary.TotalMemoryUsed += gpu.MemoryUsed
		summary.TotalMemory += gpu.MemoryTotal
		summary.TotalPower += float64(gpu.PowerDraw)
		if gpu.Utilization >= 55 || memoryPercent >= 60 || gpu.ProcessCount > 0 {
			summary.BusyGpuCount++
		}
	}
	summary.IdleGpuCount = summary.GPUCount - summary.BusyGpuCount
	avgUtilization := utilizationTotal / float64(summary.GPUCount)
	avgMemoryPercent := memoryPercentTotal / float64(summary.GPUCount)
	summary.AvgUtilization = &avgUtilization
	summary.AvgMemoryPercent = &avgMemoryPercent
	summary.GpuPressure = avgUtilization*0.6 + avgMemoryPercent*0.4
	summary.BusyRatio = float64(summary.BusyGpuCount) / float64(summary.GPUCount)
	return summary
}
