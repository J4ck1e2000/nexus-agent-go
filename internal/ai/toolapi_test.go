package ai

import (
	"context"
	"testing"
)

type fakeClusterData struct {
	nodes []NodeSnapshot
	err   error
}

func (f *fakeClusterData) ListNodeSnapshots(ctx context.Context) ([]NodeSnapshot, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.nodes, nil
}

func testSnapshot(name string) NodeSnapshot {
	return NodeSnapshot{
		Name:              name,
		Status:            "online",
		AvailabilityScore: 90,
		AvailabilityTier:  "good",
		CPUUsage:          floatPtr(12.5),
		RAMPercent:        floatPtr(40.0),
		ActiveUserCount:   1,
		GPUSummary: NodeGPUSummary{
			GPUCount:     2,
			BusyGPUCount: 0,
			IdleGPUCount: 2,
		},
		Processes: []GPUProcess{},
	}
}

func TestDispatcherToolList(t *testing.T) {
	dispatcher := NewToolDispatcher(NewToolbox(ToolboxOptions{}))
	expected := []string{
		"get_node_metrics", "list_idle_nodes", "get_gpu_processes", "get_node_summary",
		"get_alert_history", "recommend_nodes_for_job", "explain_node_anomaly", "search_knowledge_base",
	}
	for _, name := range expected {
		if !dispatcher.HasTool(name) {
			t.Fatalf("expected tool %s to be dispatchable", name)
		}
	}
	if dispatcher.HasTool("run_shell") {
		t.Fatal("unknown tool must not be dispatchable")
	}
}

func TestDispatcherValidatesArguments(t *testing.T) {
	dispatcher := NewToolDispatcher(NewToolbox(ToolboxOptions{
		DataProvider: &fakeClusterData{nodes: []NodeSnapshot{testSnapshot("node-01")}},
	}))

	response := dispatcher.Dispatch(context.Background(), "get_node_metrics", map[string]any{})
	if response.OK || response.Error == nil || response.Error.Code != ToolErrInvalidArguments {
		t.Fatalf("expected invalid arguments error, got %+v", response)
	}

	response = dispatcher.Dispatch(context.Background(), "get_alert_history", map[string]any{"window": "7d"})
	if response.OK || response.Error.Code != ToolErrInvalidArguments {
		t.Fatalf("expected window validation error, got %+v", response)
	}

	response = dispatcher.Dispatch(context.Background(), "search_knowledge_base", map[string]any{"query": "gpu"})
	if response.OK || response.Error == nil || response.Error.Code != ToolErrKnowledgeDisabled {
		t.Fatalf("expected knowledge disabled error, got %+v", response)
	}
}

func TestDispatcherNodeLookupErrors(t *testing.T) {
	data := &fakeClusterData{}
	emptyData := &fakeClusterData{nodes: []NodeSnapshot{}}
	populated := &fakeClusterData{nodes: []NodeSnapshot{testSnapshot("node-01")}}

	response := NewToolDispatcher(NewToolbox(ToolboxOptions{DataProvider: data})).
		Dispatch(context.Background(), "get_node_metrics", map[string]any{"node_name": "node-01"})
	if response.Error == nil || response.Error.Code != ToolErrNoNodesAvailable {
		t.Fatalf("expected no nodes available, got %+v", response)
	}

	response = NewToolDispatcher(NewToolbox(ToolboxOptions{DataProvider: emptyData})).
		Dispatch(context.Background(), "get_node_summary", map[string]any{"node_name": "node-01"})
	if response.Error == nil || response.Error.Code != ToolErrNoNodesAvailable {
		t.Fatalf("expected no nodes available, got %+v", response)
	}

	response = NewToolDispatcher(NewToolbox(ToolboxOptions{DataProvider: populated})).
		Dispatch(context.Background(), "get_gpu_processes", map[string]any{"node_name": "node-missing"})
	if response.Error == nil || response.Error.Code != ToolErrNodeNotFound {
		t.Fatalf("expected node not found, got %+v", response)
	}
}

func TestDispatcherSuccessCarriesMeta(t *testing.T) {
	snapshot := testSnapshot("node-01")
	snapshot.CollectedAtUnix = 1700000000
	dispatcher := NewToolDispatcher(NewToolbox(ToolboxOptions{
		DataProvider: &fakeClusterData{nodes: []NodeSnapshot{snapshot}},
	}))

	response := dispatcher.Dispatch(context.Background(), "get_node_summary", map[string]any{"node_name": "node-01"})
	if !response.OK || response.Meta == nil {
		t.Fatalf("expected success with meta, got %+v", response)
	}
	if response.Meta.RetrievedAtUnix == 0 {
		t.Fatal("retrieved_at must be set")
	}

	response = dispatcher.Dispatch(context.Background(), "get_node_metrics", map[string]any{"node_name": "node-01"})
	if !response.OK || response.Meta.ObservedAtUnix != 1700000000 {
		t.Fatalf("expected observed_at from collection, got %+v", response.Meta)
	}

	response = dispatcher.Dispatch(context.Background(), "list_idle_nodes", map[string]any{"limit": 5})
	if !response.OK {
		t.Fatalf("list_idle_nodes failed: %+v", response.Error)
	}
	candidates, ok := response.Data["candidates"].([]NodeCandidate)
	if !ok || len(candidates) != 1 {
		t.Fatalf("expected one candidate, got %+v", response.Data)
	}
}

func TestDispatcherUnknownTool(t *testing.T) {
	dispatcher := NewToolDispatcher(NewToolbox(ToolboxOptions{}))
	response := dispatcher.Dispatch(context.Background(), "totally_bogus", map[string]any{})
	if response.OK || response.Error == nil {
		t.Fatal("unknown tool must fail")
	}
}
