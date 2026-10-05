package ai

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"nexus-agent-go/internal/runtime"
)

// stubRuntimeServer emits a canned NDJSON event sequence for one run.
type stubRuntimeServer struct {
	server  *httptest.Server
	events  []runtime.Event
	gotRuns chan runtime.StartRunRequest
}

func newStubRuntime(t *testing.T, events []runtime.Event) *stubRuntimeServer {
	t.Helper()
	stub := &stubRuntimeServer{events: events, gotRuns: make(chan runtime.StartRunRequest, 4)}
	stub.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/healthz":
			w.WriteHeader(http.StatusOK)
			return
		case r.URL.Path == "/v1/runs" && r.Method == http.MethodPost:
			var request runtime.StartRunRequest
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			stub.gotRuns <- request
			w.Header().Set("Content-Type", "application/x-ndjson")
			w.WriteHeader(http.StatusOK)
			for _, event := range stub.events {
				if event.RunID == "" {
					event.RunID = request.RunID
				}
				raw, err := json.Marshal(event)
				if err != nil {
					continue
				}
				_, _ = w.Write(raw)
				_, _ = w.Write([]byte("\n"))
			}
			return
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(stub.server.Close)
	return stub
}

func mustEvent(t *testing.T, runID, eventType string, seq int64, payload any) runtime.Event {
	t.Helper()
	event, err := runtime.NewEvent(runID, eventType, seq, payload)
	if err != nil {
		t.Fatalf("build event failed: %v", err)
	}
	return event
}

func newTestExecutor(t *testing.T, stub *stubRuntimeServer, manager *runtime.Manager) *PiRuntimeExecutor {
	t.Helper()
	cfg := runtime.Config{
		Enabled:        true,
		RuntimeURL:     stub.server.URL,
		RuntimeToken:   "token",
		RunTokenSecret: "secret",
		RunTimeout:     defaultPiTestTimeout,
		ToolTimeout:    defaultPiTestTimeout,
		MaxToolCalls:   4,
	}
	return NewPiRuntimeExecutor(PiRuntimeExecutorOptions{
		Client:  runtime.NewClient(cfg.RuntimeURL, cfg.RuntimeToken),
		Manager: manager,
		Config:  cfg,
		Toolbox: NewToolbox(ToolboxOptions{DataProvider: &fakeClusterData{nodes: []NodeSnapshot{testSnapshot("node-01")}}}),
		AllowedTools: []string{
			"get_node_metrics", "list_idle_nodes", "get_gpu_processes", "get_node_summary",
			"get_alert_history", "recommend_nodes_for_job", "explain_node_anomaly", "search_knowledge_base",
		},
	})
}

const defaultPiTestTimeout = 5_000_000_000 // 5s in nanoseconds

func TestPiExecutorMapsEventStream(t *testing.T) {
	events := []runtime.Event{
		mustEvent(t, "", runtime.EventRunStarted, 1, runtime.RunStartedPayload{}),
		mustEvent(t, "", runtime.EventToolStarted, 2, runtime.ToolStartedPayload{
			ToolCallID: "call-1", ToolName: "get_node_metrics",
			Arguments: map[string]any{"node_name": "node-01"},
		}),
		mustEvent(t, "", runtime.EventToolCompleted, 3, runtime.ToolCompletedPayload{
			ToolCallID: "call-1", ToolName: "get_node_metrics", OK: true,
			Result: map[string]any{"node": map[string]any{"name": "node-01"}},
		}),
		mustEvent(t, "", runtime.EventToolStarted, 4, runtime.ToolStartedPayload{
			ToolCallID: "call-2", ToolName: "search_knowledge_base",
			Arguments: map[string]any{"query": "gpu"},
		}),
		mustEvent(t, "", runtime.EventToolCompleted, 5, runtime.ToolCompletedPayload{
			ToolCallID: "call-2", ToolName: "search_knowledge_base", OK: true,
			Result: map[string]any{
				"hits":      []any{map[string]any{"title": "GPU OOM", "snippet": "check vram"}},
				"retrieval": map[string]any{"query": "gpu", "top_k": 3, "hit": true},
			},
		}),
		mustEvent(t, "", runtime.EventToolStarted, 6, runtime.ToolStartedPayload{
			ToolCallID: "call-3", ToolName: "get_alert_history",
			Arguments: map[string]any{"node_name": "node-01"},
		}),
		mustEvent(t, "", runtime.EventToolCompleted, 7, runtime.ToolCompletedPayload{
			ToolCallID: "call-3", ToolName: "get_alert_history", OK: false,
			Error: "history unavailable",
		}),
		mustEvent(t, "", runtime.EventMessageDelta, 8, runtime.MessageDeltaPayload{Text: "结论：node-01 空闲。"}),
		mustEvent(t, "", runtime.EventRunCompleted, 9, runtime.RunCompletedPayload{
			Text: `{"answer":"结论：node-01 空闲。","reasoning_summary":"基于 get_node_metrics 证据","related_nodes":["node-01"]}`,
		}),
	}
	stub := newStubRuntime(t, events)
	manager := runtime.NewManager()
	defer manager.Close()
	executor := newTestExecutor(t, stub, manager)

	var mu sync.Mutex
	var streamEvents []AIStreamEvent
	resp, err := executor.ExecuteStream(context.Background(),
		AIQueryRequest{Query: "node-01 现在空闲吗", Stream: true},
		func(event AIStreamEvent) error {
			mu.Lock()
			defer mu.Unlock()
			streamEvents = append(streamEvents, event)
			return nil
		})
	if err != nil {
		t.Fatalf("execute stream failed: %v", err)
	}

	// Final response assembly.
	if resp.Mode != AIModeAgent {
		t.Fatalf("expected agent mode, got %s", resp.Mode)
	}
	if resp.Answer != "结论：node-01 空闲。" {
		t.Fatalf("unexpected answer: %q", resp.Answer)
	}
	if len(resp.RelatedNodes) != 1 || resp.RelatedNodes[0] != "node-01" {
		t.Fatalf("expected related node node-01, got %v", resp.RelatedNodes)
	}
	if len(resp.ToolCalls) != 3 {
		t.Fatalf("expected 3 tool calls, got %d", len(resp.ToolCalls))
	}
	if len(resp.KnowledgeHits) != 1 || resp.KnowledgeHits[0].Title != "GPU OOM" {
		t.Fatalf("expected knowledge hit, got %+v", resp.KnowledgeHits)
	}
	if resp.Retrieval == nil || !resp.Retrieval.Hit {
		t.Fatalf("expected retrieval meta, got %+v", resp.Retrieval)
	}
	found := false
	for _, warning := range resp.Warnings {
		if strings.Contains(warning, "get_alert_history failed") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected tool failure warning, got %v", resp.Warnings)
	}

	// Stream contract mapping.
	if streamEvents[0].Event != StreamEventStatus {
		t.Fatalf("first event should be a status, got %s", streamEvents[0].Event)
	}
	var sawDelta bool
	for _, event := range streamEvents {
		if event.Event == StreamEventDelta && event.Text == "结论：node-01 空闲。" {
			sawDelta = true
		}
	}
	if !sawDelta {
		t.Fatalf("delta event missing from stream: %+v", streamEvents)
	}

	// Run lifecycle cleanup.
	select {
	case request := <-stub.gotRuns:
		if len(request.Policy.EnabledToolNames) == 0 {
			t.Fatal("policy must carry enabled tools")
		}
		if request.Context.LocaleHint != "zh" {
			t.Fatalf("expected zh locale hint, got %s", request.Context.LocaleHint)
		}
	default:
		t.Fatal("runtime did not receive start request")
	}
}

func TestPiExecutorFailsWithoutTerminalEvent(t *testing.T) {
	events := []runtime.Event{
		mustEvent(t, "", runtime.EventRunStarted, 1, runtime.RunStartedPayload{}),
		mustEvent(t, "", runtime.EventMessageDelta, 2, runtime.MessageDeltaPayload{Text: "partial"}),
	}
	stub := newStubRuntime(t, events)
	manager := runtime.NewManager()
	defer manager.Close()
	executor := newTestExecutor(t, stub, manager)

	_, err := executor.ExecuteStream(context.Background(), AIQueryRequest{Query: "hi"}, nil)
	if err == nil {
		t.Fatal("stream without terminal event must fail")
	}
}

func TestPiExecutorPropagatesRuntimeUnavailable(t *testing.T) {
	manager := runtime.NewManager()
	defer manager.Close()
	cfg := runtime.Config{
		Enabled:        true,
		RuntimeURL:     "http://127.0.0.1:1",
		RuntimeToken:   "token",
		RunTokenSecret: "secret",
		RunTimeout:     defaultPiTestTimeout,
		ToolTimeout:    defaultPiTestTimeout,
		MaxToolCalls:   4,
	}
	executor := NewPiRuntimeExecutor(PiRuntimeExecutorOptions{
		Client:  runtime.NewClient(cfg.RuntimeURL, cfg.RuntimeToken),
		Manager: manager,
		Config:  cfg,
		Toolbox: NewToolbox(ToolboxOptions{}),
	})

	_, err := executor.ExecuteStream(context.Background(), AIQueryRequest{Query: "hi"}, nil)
	if !errors.Is(err, runtime.ErrRuntimeUnavailable) {
		t.Fatalf("expected runtime unavailable, got %v", err)
	}
}

func TestPiExecutorRejectsEmptyCompletedRun(t *testing.T) {
	events := []runtime.Event{
		mustEvent(t, "", runtime.EventRunStarted, 1, runtime.RunStartedPayload{}),
		mustEvent(t, "", runtime.EventRunCompleted, 2, runtime.RunCompletedPayload{}),
	}
	stub := newStubRuntime(t, events)
	manager := runtime.NewManager()
	defer manager.Close()
	executor := newTestExecutor(t, stub, manager)
	_, err := executor.ExecuteStream(context.Background(), AIQueryRequest{Query: "hi"}, nil)
	if !errors.Is(err, ErrAgentUnavailable) {
		t.Fatalf("empty completed run should fail, got %v", err)
	}
}

func TestPiExecutorPreservesFailedAndCancelledEvents(t *testing.T) {
	for _, eventType := range []string{runtime.EventRunFailed, runtime.EventRunCancelled} {
		t.Run(eventType, func(t *testing.T) {
			events := []runtime.Event{
				mustEvent(t, "", runtime.EventRunStarted, 1, runtime.RunStartedPayload{}),
				mustEvent(t, "", eventType, 2, runtime.RunFailedPayload{Error: "model quota exceeded"}),
			}
			stub := newStubRuntime(t, events)
			manager := runtime.NewManager()
			defer manager.Close()
			executor := newTestExecutor(t, stub, manager)
			_, err := executor.ExecuteStream(context.Background(), AIQueryRequest{Query: "hi"}, nil)
			if eventType == runtime.EventRunCancelled {
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("cancelled event should return context.Canceled, got %v", err)
				}
			} else if err == nil || !strings.Contains(err.Error(), "model quota exceeded") {
				t.Fatalf("failed event should preserve runtime diagnostics, got %v", err)
			}
		})
	}
}

func TestPiExecutorRunCanceledOnContextCancel(t *testing.T) {
	// Stream that opens, emits run.started, then stays open until the client
	// disconnects — like a real in-flight run.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request runtime.StartRunRequest
		_ = json.NewDecoder(r.Body).Decode(&request)
		w.Header().Set("Content-Type", "application/x-ndjson")
		w.WriteHeader(http.StatusOK)
		started := mustEvent(t, request.RunID, runtime.EventRunStarted, 1, runtime.RunStartedPayload{})
		raw, _ := json.Marshal(started)
		_, _ = w.Write(raw)
		_, _ = w.Write([]byte("\n"))
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer server.Close()

	manager := runtime.NewManager()
	defer manager.Close()
	cfg := runtime.Config{
		Enabled:        true,
		RuntimeURL:     server.URL,
		RuntimeToken:   "token",
		RunTokenSecret: "secret",
		RunTimeout:     defaultPiTestTimeout,
		ToolTimeout:    defaultPiTestTimeout,
		MaxToolCalls:   4,
	}
	executor := NewPiRuntimeExecutor(PiRuntimeExecutorOptions{
		Client:       runtime.NewClient(cfg.RuntimeURL, cfg.RuntimeToken),
		Manager:      manager,
		Config:       cfg,
		Toolbox:      NewToolbox(ToolboxOptions{DataProvider: &fakeClusterData{nodes: []NodeSnapshot{testSnapshot("node-01")}}}),
		AllowedTools: []string{"get_node_metrics"},
	})

	ctx, cancel := context.WithCancel(context.Background())
	type result struct {
		err error
	}
	results := make(chan result, 1)
	go func() {
		_, err := executor.ExecuteStream(ctx, AIQueryRequest{Query: "hi"}, nil)
		results <- result{err: err}
	}()

	time.Sleep(150 * time.Millisecond)
	cancel()
	select {
	case got := <-results:
		if !errors.Is(got.err, context.Canceled) {
			t.Fatalf("expected context canceled, got %v", got.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("execute stream did not return after cancellation")
	}
}
