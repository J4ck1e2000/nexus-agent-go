package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"nexus-agent-go/internal/ai"
	"nexus-agent-go/internal/model"
)

func TestAIQuery_RequiresLogin(t *testing.T) {
	r, _, _, cleanup := setupAIRouter(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodPost, "/api/ai/query", bytes.NewBufferString(`{"query":"哪台机器最空闲？"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status mismatch: got=%d want=%d body=%s", w.Code, http.StatusUnauthorized, w.Body.String())
	}
}

func TestAIQuery_Returns200(t *testing.T) {
	r, configStore, stateStore, cleanup := setupAIRouter(t)
	defer cleanup()

	node, err := configStore.Add(model.AgentConfig{Name: "server-02", URL: "http://127.0.0.1:8002"}, 1)
	if err != nil {
		t.Fatalf("add node failed: %v", err)
	}
	state := pendingNodeState(node)
	state.Status = NodeStatusOnline
	state.LastPolledAtUnix = 1710000001
	state.LastSeenAtUnix = 1710000001
	state.CollectedAtUnix = 1710000001
	state.Data = &model.SystemMetrics{
		CPUUsage:   25,
		RAMPercent: 40,
		Gpus: []model.GpuInfo{
			{ID: 0, Name: "A10", Utilization: 20, MemoryTotal: 24, MemoryUsed: 8, MemoryUtilization: 33},
		},
		Processes: []model.ProcessInfo{},
	}
	applyDerivedFields(&state, time.Unix(1710000002, 0))
	if err := stateStore.SaveNodeState(context.Background(), state); err != nil {
		t.Fatalf("save node state failed: %v", err)
	}

	token := loginAndGetToken(t, r, "user", "user123")
	req := authorizedRequest(http.MethodPost, "/api/ai/query", bytes.NewBufferString(`{"query":"哪台机器最空闲？","stream":false}`), token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status mismatch: got=%d want=%d body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp ai.AIQueryResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response failed: %v", err)
	}
	if strings.TrimSpace(resp.Answer) == "" {
		t.Fatalf("answer should not be empty")
	}
	if resp.Mode != ai.AIModeRule {
		t.Fatalf("mode mismatch: got=%q want=%q", resp.Mode, ai.AIModeRule)
	}
}

func TestAIQuery_InvalidPayload(t *testing.T) {
	r, _, _, cleanup := setupAIRouter(t)
	defer cleanup()

	token := loginAndGetToken(t, r, "user", "user123")
	req := authorizedRequest(http.MethodPost, "/api/ai/query", bytes.NewBufferString(`{"query":"   "}`), token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status mismatch: got=%d want=%d body=%s", w.Code, http.StatusBadRequest, w.Body.String())
	}
}

func TestAIQuery_StreamSSEHeadersAndEvents(t *testing.T) {
	gin.SetMode(gin.TestMode)

	h := &Handler{}
	h.SetAIQueryService(&stubAIService{
		streamEvents: []ai.AIStreamEvent{
			{Event: ai.StreamEventStart, Query: "哪台机器 GPU 最空闲？"},
			{Event: ai.StreamEventStatus, Phase: "thinking", Message: "正在分析节点状态..."},
			{Event: ai.StreamEventDelta, Text: "当前最空闲的节点是 server-01。"},
			{
				Event: ai.StreamEventMeta,
				Meta: &ai.AIStreamMeta{
					Mode:             ai.AIModeRule,
					ReasoningSummary: "基于节点可用性和 GPU 空闲度综合判断。",
					RelatedNodes:     []string{"server-01"},
				},
			},
			{
				Event: ai.StreamEventDone,
				Done: &ai.AIQueryResponse{
					Answer:           "当前最空闲的节点是 server-01。",
					ReasoningSummary: "基于节点可用性和 GPU 空闲度综合判断。",
					Mode:             ai.AIModeRule,
					RelatedNodes:     []string{"server-01"},
				},
			},
		},
	})

	r := gin.New()
	authorized := r.Group("/api")
	h.registerAIRoutes(authorized)

	req := httptest.NewRequest(http.MethodPost, "/api/ai/query", bytes.NewBufferString(`{"query":"哪台机器 GPU 最空闲？","stream":true}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status mismatch: got=%d want=%d body=%s", w.Code, http.StatusOK, w.Body.String())
	}
	if contentType := w.Header().Get("Content-Type"); !strings.Contains(contentType, "text/event-stream") {
		t.Fatalf("content-type mismatch: got=%q", contentType)
	}

	body := w.Body.String()
	for _, expected := range []string{
		"event: start",
		"event: status",
		"event: delta",
		"event: meta",
		"event: done",
	} {
		if !strings.Contains(body, expected) {
			t.Fatalf("missing stream event %q in body: %s", expected, body)
		}
	}
}

func TestAIQuery_StreamErrorEvent(t *testing.T) {
	gin.SetMode(gin.TestMode)

	h := &Handler{}
	h.SetAIQueryService(&stubAIService{
		streamErr: ai.ErrServiceDisabled,
	})

	r := gin.New()
	authorized := r.Group("/api")
	h.registerAIRoutes(authorized)

	req := httptest.NewRequest(http.MethodPost, "/api/ai/query", bytes.NewBufferString(`{"query":"test stream error","stream":true}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status mismatch: got=%d want=%d body=%s", w.Code, http.StatusOK, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, "event: error") {
		t.Fatalf("expected event:error, got body=%s", body)
	}
	if !strings.Contains(body, `"error":"ai_disabled"`) {
		t.Fatalf("expected ai_disabled error payload, got body=%s", body)
	}
}

func TestAIRetrievalStats_Returns200(t *testing.T) {
	r, _, _, cleanup := setupAIRouter(t)
	defer cleanup()

	token := loginAndGetToken(t, r, "user", "user123")
	req := authorizedRequest(http.MethodGet, "/api/ai/retrieval/stats", nil, token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status mismatch: got=%d want=%d body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	var stats ai.RetrievalStats
	if err := json.Unmarshal(w.Body.Bytes(), &stats); err != nil {
		t.Fatalf("unmarshal stats failed: %v", err)
	}
	if stats.TotalSearches != 0 {
		t.Fatalf("expected initial total searches to be zero, got=%d", stats.TotalSearches)
	}
}

func TestAIKnowledgeReload_RequiresAdmin(t *testing.T) {
	r, _, _, cleanup := setupAIRouter(t)
	defer cleanup()

	token := loginAndGetToken(t, r, "user", "user123")
	req := authorizedRequest(http.MethodPost, "/api/ai/knowledge/reload", bytes.NewBufferString(`{}`), token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status mismatch: got=%d want=%d body=%s", w.Code, http.StatusForbidden, w.Body.String())
	}
}

func TestAIKnowledgeReload_AdminSuccess(t *testing.T) {
	r, _, _, cleanup := setupAIRouter(t)
	defer cleanup()

	token := loginAndGetToken(t, r, "admin", "admin123")
	req := authorizedRequest(http.MethodPost, "/api/ai/knowledge/reload", bytes.NewBufferString(`{}`), token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status mismatch: got=%d want=%d body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	var result ai.KnowledgeReloadResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal response failed: %v", err)
	}
	if !result.KnowledgeEnabled {
		t.Fatalf("expected knowledge enabled after reload")
	}
	if result.LoadedDocuments <= 0 || result.LoadedChunks <= 0 {
		t.Fatalf("invalid reload counts: %+v", result)
	}
}

func setupAIRouter(t *testing.T) (*gin.Engine, *ConfigStore, *NodeStateStore, func()) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	dsn := "file:" + strings.ReplaceAll(t.Name(), "/", "_") + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite failed: %v", err)
	}
	if err := AutoMigrate(db); err != nil {
		t.Fatalf("auto migrate failed: %v", err)
	}
	seedTestUser(t, db, "admin", "admin123", RoleAdmin)
	seedTestUser(t, db, "user", "user123", RoleUser)

	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis failed: %v", err)
	}
	redisClient := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	stateStore := NewNodeStateStore(redisClient, "nexus", 10*time.Minute)
	configStore := NewConfigStore(db)
	nodeService := NewNodeStateService(configStore, stateStore, NodeStateServiceOptions{
		PollTimeout: 3 * time.Second,
		NowFunc: func() time.Time {
			return time.Unix(1710000000, 0)
		},
	})
	knowledgeDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(knowledgeDir, "GPU_OOM.md"), []byte(`# GPU OOM
Category: gpu_memory
Tags: gpu, oom, cuda
## Symptoms
CUDA out of memory.
## Actions
Reduce batch size and release orphan processes.`), 0o644); err != nil {
		t.Fatalf("write test knowledge failed: %v", err)
	}

	adapter := NewAIDataAdapter(nodeService, stateStore)
	toolbox := ai.NewToolbox(ai.ToolboxOptions{
		DataProvider:    adapter,
		HistoryProvider: adapter,
	})
	aiService := ai.NewService(ai.ServiceOptions{
		Config: ai.Config{
			Enabled:         true,
			Mode:            ai.AIModeRule,
			RAGEnabled:      true,
			RAGKnowledgeDir: knowledgeDir,
			RAGTopK:         3,
			RAGMinScore:     0.3,
			RAGMaxSnippet:   180,
			KnowledgeRetrieval: ai.KnowledgeRetrievalConfig{
				Backend:       ai.KnowledgeBackendLocal,
				TopK:          3,
				MinScore:      0.3,
				SyncBatchSize: 32,
			},
		},
		Toolbox: toolbox,
	})

	r := gin.New()
	h := NewHandler(configStore, NewAuthService(db, "test-secret"), VersionInfo{}, nodeService)
	h.SetAIQueryService(aiService)
	h.RegisterAPIRoutes(r)

	cleanup := func() {
		_ = redisClient.Close()
		mr.Close()
	}
	return r, configStore, stateStore, cleanup
}

type stubAIService struct {
	queryResp      ai.AIQueryResponse
	queryErr       error
	streamEvents   []ai.AIStreamEvent
	streamErr      error
	retrievalStats ai.RetrievalStats
	reloadResult   ai.KnowledgeReloadResult
	reloadErr      error
}

func (s *stubAIService) Query(ctx context.Context, req ai.AIQueryRequest) (ai.AIQueryResponse, error) {
	_ = ctx
	_ = req
	if s.queryErr != nil {
		return ai.AIQueryResponse{}, s.queryErr
	}
	return s.queryResp, nil
}

func (s *stubAIService) QueryStream(ctx context.Context, req ai.AIQueryRequest, emit func(ai.AIStreamEvent) error) error {
	_ = req
	for _, event := range s.streamEvents {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if emit != nil {
			if err := emit(event); err != nil {
				return err
			}
		}
	}
	return s.streamErr
}

func (s *stubAIService) Capabilities(ctx context.Context) ai.CapabilitiesResponse {
	_ = ctx
	return ai.CapabilitiesResponse{Enabled: true, DefaultMode: ai.AIModeRule}
}

func (s *stubAIService) Health(ctx context.Context) ai.HealthResponse {
	_ = ctx
	return ai.HealthResponse{Status: "ok", Mode: ai.AIModeRule, AgentReady: true}
}

func (s *stubAIService) RetrievalStats(ctx context.Context) ai.RetrievalStats {
	_ = ctx
	return s.retrievalStats
}

func (s *stubAIService) ReloadKnowledge(ctx context.Context) (ai.KnowledgeReloadResult, error) {
	_ = ctx
	if s.reloadErr != nil {
		return ai.KnowledgeReloadResult{}, s.reloadErr
	}
	return s.reloadResult, nil
}
