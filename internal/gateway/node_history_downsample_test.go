package gateway

import (
	"testing"
)

func TestDownsampleNodeHistoryPeakKeepsShortBusySpikes(t *testing.T) {
	samples := []NodeHistorySnapshot{
		{TimestampUnix: 101, NodeID: 1, Status: NodeStatusOnline, GPUs: []NodeGPUHistory{{ID: 0, Name: "gpu", Utilization: 2, MemoryUsed: 1, MemoryTotal: 80}}},
		{TimestampUnix: 104, NodeID: 1, Status: NodeStatusOnline, GPUs: []NodeGPUHistory{{ID: 0, Name: "gpu", Utilization: 96, MemoryUsed: 30, MemoryTotal: 80, ProcessCount: 1}}},
	}
	result := DownsampleNodeHistory(samples, 15, HistoryAggregationPeak)
	if len(result) != 1 || result[0].GPUs[0].Utilization != 96 || result[0].GPUs[0].ProcessCount != 1 {
		t.Fatalf("peak downsample hid a short busy period: %+v", result)
	}
}

func TestDownsampleNodeHistoryAverageComputesChartBuckets(t *testing.T) {
	samples := []NodeHistorySnapshot{
		{TimestampUnix: 301, NodeID: 1, Status: NodeStatusOnline, CPUUsage: floatValuePtr(20), RAMPercent: floatValuePtr(40), GPUs: []NodeGPUHistory{{ID: 0, Name: "gpu", Utilization: 20, MemoryUsed: 10, MemoryTotal: 80}}},
		{TimestampUnix: 309, NodeID: 1, Status: NodeStatusOnline, CPUUsage: floatValuePtr(60), RAMPercent: floatValuePtr(80), GPUs: []NodeGPUHistory{{ID: 0, Name: "gpu", Utilization: 60, MemoryUsed: 30, MemoryTotal: 80}}},
	}
	result := DownsampleNodeHistory(samples, 15, HistoryAggregationAverage)
	if len(result) != 1 || result[0].CPUUsage == nil || *result[0].CPUUsage != 40 || result[0].GPUs[0].Utilization != 40 || result[0].GPUs[0].MemoryUsed != 20 {
		t.Fatalf("average history bucket mismatch: %+v", result)
	}
}

func TestDownsampleNodeHistoryPeakDoesNotInventMissingGPUCoverage(t *testing.T) {
	samples := []NodeHistorySnapshot{
		{TimestampUnix: 301, NodeID: 1, Status: NodeStatusOnline, GPUs: []NodeGPUHistory{{ID: 0, Utilization: 1, MemoryTotal: 80}}},
		{TimestampUnix: 309, NodeID: 1, Status: NodeStatusOnline, GPUs: nil},
	}
	result := DownsampleNodeHistory(samples, 15, HistoryAggregationPeak)
	if len(result) != 1 || len(result[0].GPUs) != 0 {
		t.Fatalf("missing GPU observations must fail sustained-idle eligibility: %+v", result)
	}
}

func floatValuePtr(value float64) *float64 { return &value }
