package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"nexus-agent-go/internal/ai"
	"nexus-agent-go/internal/runtime"
)

// TestPiRuntimeFullStack runs the real TypeScript Pi runtime as a subprocess
// against a mock OpenAI server and the real Go tool gateway:
//
//	Go executor → runtime (real Pi SDK) → mock model → tool callback →
//	real dispatcher → fake cluster data → NDJSON events → final response.
//
// Skipped with -short or when node is unavailable. No real model key needed.
func TestPiRuntimeFullStack(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping full-stack test in short mode")
	}
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not available in PATH")
	}

	// --- 1. Mock OpenAI Chat Completions server (scripted two-turn loop) ---
	chatRequests := 0
	finalJSON := `{"answer":"结论：node-01 当前空闲，可直接投放任务。","reasoning_summary":"基于 get_node_metrics 实时快照。","related_nodes":["node-01"],"warnings":[],"knowledge_hits":[]}`
	mockLLM := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/chat/completions") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		var body struct {
			Stream bool `json:"stream"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		chatRequests += 1

		if chatRequests == 1 {
			toolCall := map[string]any{
				"index": 0,
				"id":    "call_fullstack_1",
				"type":  "function",
				"function": map[string]any{
					"name":      "get_node_metrics",
					"arguments": `{"node_name":"node-01"}`,
				},
			}
			writeMockCompletion(w, body.Stream, map[string]any{
				"role": "assistant", "content": nil, "tool_calls": []any{toolCall},
			}, "tool_calls", "")
			return
		}
		writeMockCompletion(w, body.Stream, map[string]any{
			"role": "assistant", "content": finalJSON,
		}, "stop", finalJSON)
	}))
	defer mockLLM.Close()

	// --- 2. Real tool gateway behind gin + fake cluster data ---
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := NewHandler(nil, nil, VersionInfo{})
	dispatcher := ai.NewToolDispatcher(ai.NewToolbox(ai.ToolboxOptions{
		DataProvider: &fakeAIProvider{nodes: []ai.NodeSnapshot{fakeAINode("node-01")}},
	}))
	runManager := runtime.NewManager()
	defer runManager.Close()
	handler.SetToolGateway(ToolGatewayDeps{
		Dispatcher:     dispatcher,
		Manager:        runManager,
		InternalToken:  "test-token",
		RunTokenSecret: "test-secret",
	})
	handler.RegisterInternalToolRoutes(router)
	toolGateway := httptest.NewServer(router)
	defer toolGateway.Close()

	// --- 3. Launch the real runtime subprocess ---
	runtimePort, err := freePort()
	if err != nil {
		t.Fatalf("alloc port failed: %v", err)
	}
	runtimeURL := fmt.Sprintf("http://127.0.0.1:%d", runtimePort)
	serverPath, err := filepath.Abs(filepath.Join("..", "..", "pi-runtime", "src", "server.ts"))
	if err != nil {
		t.Fatalf("resolve server path failed: %v", err)
	}

	cmd := exec.Command("node", serverPath)
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("PI_RUNTIME_PORT=%d", runtimePort),
		"PI_RUNTIME_TOKEN=test-token",
		fmt.Sprintf("GATEWAY_TOOL_URL=%s", toolGateway.URL),
		"GATEWAY_INTERNAL_TOKEN=test-token",
		fmt.Sprintf("AI_BASE_URL=%s/v1", mockLLM.URL),
		"AI_API_KEY=dummy-key",
		"AI_MODEL=mock-model",
		"PI_RUN_TIMEOUT_SEC=30",
		"PI_TOOL_TIMEOUT_SEC=10",
	)
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("start runtime subprocess failed: %v", err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	}()

	waitForRuntimeHealthy(t, runtimeURL, 20*time.Second)

	// --- 4. Run the query through the real Go executor ---
	cfg := runtime.Config{
		Enabled:        true,
		RuntimeURL:     runtimeURL,
		RuntimeToken:   "test-token",
		RunTokenSecret: "test-secret",
		RunTimeout:     30 * time.Second,
		ToolTimeout:    10 * time.Second,
		MaxToolCalls:   12,
	}
	executor := ai.NewPiRuntimeExecutor(ai.PiRuntimeExecutorOptions{
		Client:       runtime.NewClient(cfg.RuntimeURL, cfg.RuntimeToken),
		Manager:      runManager,
		Config:       cfg,
		Toolbox:      ai.NewToolbox(ai.ToolboxOptions{DataProvider: &fakeAIProvider{nodes: []ai.NodeSnapshot{fakeAINode("node-01")}}}),
		AllowedTools: dispatcher.ToolNames(),
	})

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	ctx = runtime.WithPrincipal(ctx, runtime.RunPrincipal{UserID: 1, Username: "tester", Role: "user"})

	var streamTypes []string
	resp, err := executor.ExecuteStream(ctx, ai.AIQueryRequest{
		Query:  "node-01 现在空闲吗？",
		Stream: true,
	}, func(event ai.AIStreamEvent) error {
		streamTypes = append(streamTypes, event.Event)
		return nil
	})
	if err != nil {
		t.Fatalf("full-stack execute failed: %v", err)
	}

	// --- 5. Assertions ---
	if resp.Mode != ai.AIModeAgent {
		t.Fatalf("expected agent mode, got %s", resp.Mode)
	}
	if !strings.Contains(resp.Answer, "node-01") {
		t.Fatalf("answer must reference node-01, got %q", resp.Answer)
	}
	if len(resp.ToolCalls) == 0 || resp.ToolCalls[0].Name != "get_node_metrics" {
		t.Fatalf("expected get_node_metrics tool call, got %+v", resp.ToolCalls)
	}
	if len(resp.RelatedNodes) == 0 {
		t.Fatal("expected related nodes from tool evidence")
	}
	if chatRequests != 2 {
		t.Fatalf("expected 2 model turns, got %d", chatRequests)
	}
	t.Logf("full-stack stream events: %v", streamTypes)
	t.Logf("full-stack answer: %s | reasoning: %s | related: %v", resp.Answer, resp.ReasoningSummary, resp.RelatedNodes)
}

// writeMockCompletion answers both streaming (SSE) and plain JSON requests.
func writeMockCompletion(w http.ResponseWriter, stream bool, message map[string]any, finishReason, streamText string) {
	if !stream {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":     "chatcmpl-fullstack",
			"object": "chat.completion",
			"model":  "mock-model",
			"choices": []any{map[string]any{
				"index":         0,
				"message":       message,
				"finish_reason": finishReason,
			}},
			"usage": map[string]any{"prompt_tokens": 20, "completion_tokens": 20, "total_tokens": 40},
		})
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.WriteHeader(http.StatusOK)
	sendChunk := func(delta map[string]any, reason any) {
		payload, _ := json.Marshal(map[string]any{
			"id":     "chatcmpl-fullstack",
			"object": "chat.completion.chunk",
			"model":  "mock-model",
			"choices": []any{map[string]any{
				"index":         0,
				"delta":         delta,
				"finish_reason": reason,
			}},
		})
		_, _ = fmt.Fprintf(w, "data: %s\n\n", payload)
		flusher.Flush()
	}

	if toolCalls, hasTools := message["tool_calls"]; hasTools {
		sendChunk(map[string]any{"role": "assistant", "tool_calls": toolCalls}, nil)
	} else if text, isText := message["content"].(string); isText {
		sendChunk(map[string]any{"role": "assistant", "content": text}, nil)
	}
	sendChunk(map[string]any{}, finishReason)
	_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	flusher.Flush()
}

func freePort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port, nil
}

func waitForRuntimeHealthy(t *testing.T, runtimeURL string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 2 * time.Second}
	for time.Now().Before(deadline) {
		resp, err := client.Get(runtimeURL + "/healthz")
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("runtime did not become healthy within %s", timeout)
}

// fakeAIProvider feeds deterministic node snapshots to the toolbox.
type fakeAIProvider struct {
	nodes []ai.NodeSnapshot
}

func (f *fakeAIProvider) ListNodeSnapshots(ctx context.Context) ([]ai.NodeSnapshot, error) {
	return f.nodes, nil
}

func fakeAINode(name string) ai.NodeSnapshot {
	cpu := 12.5
	ram := 40.0
	return ai.NodeSnapshot{
		Name:              name,
		Status:            "online",
		AvailabilityScore: 90,
		AvailabilityTier:  "good",
		CPUUsage:          &cpu,
		RAMPercent:        &ram,
		ActiveUserCount:   1,
		GPUSummary: ai.NodeGPUSummary{
			GPUCount:     2,
			BusyGPUCount: 0,
			IdleGPUCount: 2,
		},
		Processes: []ai.GPUProcess{},
	}
}
