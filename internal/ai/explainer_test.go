package ai

import (
	"strings"
	"testing"
)

func TestAnomalyExplainer_HighGPUUtilLowMemory(t *testing.T) {
	node := buildNode("server-03", "online", 55, 42, 48, 4, 0, 2, 88, 30, []GPUCard{
		{Index: 0, MemoryTotalGB: 24, MemoryUsedGB: 6, EstimatedFreeGB: 18},
		{Index: 1, MemoryTotalGB: 24, MemoryUsedGB: 7, EstimatedFreeGB: 17},
	})
	explainer := NewAnomalyExplainer()
	result := explainer.Explain(node)

	if !containsJoined(result.Findings, "high while VRAM") {
		t.Fatalf("findings should mention high util low memory: %+v", result.Findings)
	}
}

func TestAnomalyExplainer_HighMemoryLowGPU(t *testing.T) {
	node := buildNode("server-04", "online", 48, 35, 55, 4, 0, 2, 22, 86, []GPUCard{
		{Index: 0, MemoryTotalGB: 24, MemoryUsedGB: 22, EstimatedFreeGB: 2},
		{Index: 1, MemoryTotalGB: 24, MemoryUsedGB: 21, EstimatedFreeGB: 3},
	})
	explainer := NewAnomalyExplainer()
	result := explainer.Explain(node)

	if !containsJoined(result.Findings, "VRAM usage is high") {
		t.Fatalf("findings should mention high memory low gpu: %+v", result.Findings)
	}
}

func TestAnomalyExplainer_HighCPULowGPU(t *testing.T) {
	node := buildNode("node-a", "online", 50, 91, 58, 4, 1, 1, 22, 28, []GPUCard{
		{Index: 0, MemoryTotalGB: 24, MemoryUsedGB: 8, EstimatedFreeGB: 16},
	})
	explainer := NewAnomalyExplainer()
	result := explainer.Explain(node)

	if !containsJoined(result.Findings, "CPU usage is high") {
		t.Fatalf("findings should mention high cpu low gpu: %+v", result.Findings)
	}
}

func TestAnomalyExplainer_OfflineNode(t *testing.T) {
	node := buildNode("node-offline", "offline", 0, 0, 0, 5, 0, 0, 0, 0, nil)
	explainer := NewAnomalyExplainer()
	result := explainer.Explain(node)

	if result.Severity != "high" {
		t.Fatalf("severity mismatch: got=%q want=%q", result.Severity, "high")
	}
	if !containsJoined(result.Findings, "offline") {
		t.Fatalf("findings should mention offline: %+v", result.Findings)
	}
}

func TestAnomalyExplainer_StaleData(t *testing.T) {
	node := buildNode("node-stale", "online", 72, 30, 40, 45, 1, 0, 25, 20, []GPUCard{
		{Index: 0, MemoryTotalGB: 24, MemoryUsedGB: 4, EstimatedFreeGB: 20},
	})
	explainer := NewAnomalyExplainer()
	result := explainer.Explain(node)

	if !containsJoined(result.Findings, "stale") {
		t.Fatalf("findings should mention stale data: %+v", result.Findings)
	}
}

func containsJoined(items []string, keyword string) bool {
	return strings.Contains(strings.ToLower(strings.Join(items, " ")), strings.ToLower(keyword))
}
