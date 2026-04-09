package ai

import (
	"context"
	"strings"
	"testing"
)

func TestRuleExecutor_EmptyNodes(t *testing.T) {
	toolbox := NewToolbox(ToolboxOptions{
		DataProvider: &staticProvider{nodes: []NodeSnapshot{}},
	})
	executor := NewRuleExecutor(NewIntentClassifier(), toolbox)

	resp, err := executor.Execute(context.Background(), AIQueryRequest{Query: "哪台机器 GPU 最空闲？"})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if !strings.Contains(resp.Answer, "没有可用") {
		t.Fatalf("answer mismatch: %q", resp.Answer)
	}
	if resp.Mode != AIModeRule {
		t.Fatalf("mode mismatch: got=%q want=%q", resp.Mode, AIModeRule)
	}
}

func TestRuleExecutor_AllOffline(t *testing.T) {
	nodes := []NodeSnapshot{
		buildNode("server-01", "offline", 0, 0, 0, 5, 0, 0, 0, 0, []GPUCard{{Index: 0, MemoryTotalGB: 24, MemoryUsedGB: 2, EstimatedFreeGB: 22}}),
		buildNode("server-02", "offline", 0, 0, 0, 5, 0, 0, 0, 0, []GPUCard{{Index: 0, MemoryTotalGB: 24, MemoryUsedGB: 10, EstimatedFreeGB: 14}}),
	}
	toolbox := NewToolbox(ToolboxOptions{DataProvider: &staticProvider{nodes: nodes}})
	executor := NewRuleExecutor(NewIntentClassifier(), toolbox)

	resp, err := executor.Execute(context.Background(), AIQueryRequest{Query: "推荐两台适合训练任务的机器"})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if !strings.Contains(resp.Answer, "没有节点满足") && !strings.Contains(resp.Answer, "没有可用") {
		t.Fatalf("unexpected answer: %q", resp.Answer)
	}
}

func TestRuleExecutor_RecommendFor16GB(t *testing.T) {
	nodes := []NodeSnapshot{
		buildNode("server-01", "online", 80, 40, 55, 5, 1, 1, 45, 50, []GPUCard{
			{Index: 0, MemoryTotalGB: 24, MemoryUsedGB: 16, EstimatedFreeGB: 8},
		}),
		buildNode("server-02", "online", 90, 20, 30, 4, 2, 0, 22, 20, []GPUCard{
			{Index: 0, MemoryTotalGB: 48, MemoryUsedGB: 18, EstimatedFreeGB: 30},
			{Index: 1, MemoryTotalGB: 24, MemoryUsedGB: 2, EstimatedFreeGB: 22},
		}),
		buildNode("server-05", "online", 84, 35, 40, 6, 1, 1, 35, 35, []GPUCard{
			{Index: 0, MemoryTotalGB: 40, MemoryUsedGB: 14, EstimatedFreeGB: 26},
		}),
	}
	toolbox := NewToolbox(ToolboxOptions{DataProvider: &staticProvider{nodes: nodes}})
	executor := NewRuleExecutor(NewIntentClassifier(), toolbox)

	resp, err := executor.Execute(context.Background(), AIQueryRequest{Query: "哪台服务器最适合跑一个16GB显存的训练任务？"})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if len(resp.RelatedNodes) == 0 {
		t.Fatalf("related nodes should not be empty")
	}
	if resp.RelatedNodes[0] != "server-02" {
		t.Fatalf("top recommendation mismatch: got=%q want=%q", resp.RelatedNodes[0], "server-02")
	}
	if len(resp.ToolCalls) == 0 {
		t.Fatalf("tool calls should be recorded")
	}
}

func TestScheduler_StaleNodePenalty(t *testing.T) {
	fresh := buildNode("fresh-node", "online", 82, 30, 35, 4, 1, 0, 20, 20, []GPUCard{
		{Index: 0, MemoryTotalGB: 24, MemoryUsedGB: 4, EstimatedFreeGB: 20},
	})
	stale := buildNode("stale-node", "online", 90, 30, 35, 45, 1, 0, 20, 20, []GPUCard{
		{Index: 0, MemoryTotalGB: 24, MemoryUsedGB: 1, EstimatedFreeGB: 23},
	})

	scheduler := NewScheduler()
	candidates := scheduler.RecommendNodesForJob([]NodeSnapshot{fresh, stale}, JobRequirement{
		MinFreeVRAMGB:   16,
		GPUCount:        1,
		PreferFreshData: true,
	}, 2)

	if len(candidates) != 2 {
		t.Fatalf("candidate length mismatch: got=%d want=2", len(candidates))
	}
	if candidates[0].NodeName != "fresh-node" {
		t.Fatalf("fresh node should rank first, got=%q", candidates[0].NodeName)
	}
}

func TestScheduler_BusyVsAvailable(t *testing.T) {
	available := buildNode("available-node", "online", 85, 25, 40, 5, 2, 0, 25, 25, []GPUCard{
		{Index: 0, MemoryTotalGB: 24, MemoryUsedGB: 6, EstimatedFreeGB: 18},
		{Index: 1, MemoryTotalGB: 24, MemoryUsedGB: 8, EstimatedFreeGB: 16},
	})
	busy := buildNode("busy-node", "online", 32, 92, 94, 4, 0, 2, 95, 92, []GPUCard{
		{Index: 0, MemoryTotalGB: 24, MemoryUsedGB: 22, EstimatedFreeGB: 2},
		{Index: 1, MemoryTotalGB: 24, MemoryUsedGB: 21, EstimatedFreeGB: 3},
	})
	busy.ActiveUserCount = 4
	busy.GPUSummary.BusyRatio = 1
	available.GPUSummary.BusyRatio = 0

	scheduler := NewScheduler()
	candidates := scheduler.RecommendNodesForJob([]NodeSnapshot{busy, available}, JobRequirement{}, 2)

	if len(candidates) != 2 {
		t.Fatalf("candidate length mismatch: got=%d want=2", len(candidates))
	}
	if candidates[0].NodeName != "available-node" {
		t.Fatalf("available node should rank first, got=%q", candidates[0].NodeName)
	}
}
