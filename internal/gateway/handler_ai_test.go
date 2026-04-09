package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
	adapter := NewAIDataAdapter(nodeService, stateStore)
	toolbox := ai.NewToolbox(ai.ToolboxOptions{
		DataProvider:    adapter,
		HistoryProvider: adapter,
	})
	aiService := ai.NewService(ai.ServiceOptions{
		Config: ai.Config{
			Enabled: true,
			Mode:    ai.AIModeRule,
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
