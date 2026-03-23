package gateway

import (
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

	"nexus-agent-go/internal/model"
)

func TestNodesOverview_RequiresLogin(t *testing.T) {
	r, _, _, cleanup := setupNodesOverviewRouter(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/nodes/overview", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status mismatch: got=%d want=%d body=%s", w.Code, http.StatusUnauthorized, w.Body.String())
	}
}

func TestNodesOverview_ReturnsAllConfiguredNodes(t *testing.T) {
	r, store, stateStore, cleanup := setupNodesOverviewRouter(t)
	defer cleanup()

	node1, err := store.Add(model.AgentConfig{Name: "n1", URL: "http://127.0.0.1:8005"}, 1)
	if err != nil {
		t.Fatalf("add node1 failed: %v", err)
	}
	node2, err := store.Add(model.AgentConfig{Name: "n2", URL: "http://127.0.0.1:8006"}, 1)
	if err != nil {
		t.Fatalf("add node2 failed: %v", err)
	}

	state := pendingNodeState(node1)
	state.Status = NodeStatusOnline
	state.LastSeenAtUnix = 1710000000
	state.LastPolledAtUnix = 1710000000
	state.CollectedAtUnix = 1709999999
	state.Data = &model.SystemMetrics{Hostname: "node-1"}
	state.Metrics = &NodeNetworkMetrics{UpSpeed: 1.1, DownSpeed: 2.2}
	applyDerivedFields(&state, time.Unix(1710000000, 0))
	if err := stateStore.SaveNodeState(context.Background(), state); err != nil {
		t.Fatalf("save state failed: %v", err)
	}

	token := loginAndGetToken(t, r, "user", "user123")
	req := authorizedRequest(http.MethodGet, "/api/nodes/overview", nil, token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status mismatch: got=%d want=%d body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	var overview []NodeOverview
	if err := json.Unmarshal(w.Body.Bytes(), &overview); err != nil {
		t.Fatalf("unmarshal response failed: %v", err)
	}
	if len(overview) != 2 {
		t.Fatalf("overview length mismatch: got=%d want=2", len(overview))
	}

	found := map[int64]NodeOverview{}
	for _, item := range overview {
		found[item.ID] = item
	}

	if found[node1.ID].Status != NodeStatusOnline {
		t.Fatalf("node1 status mismatch: %+v", found[node1.ID])
	}
	if found[node2.ID].Status != NodeStatusPending {
		t.Fatalf("node2 should be pending: %+v", found[node2.ID])
	}
	if found[node2.ID].Data != nil {
		t.Fatalf("pending node should not include data: %+v", found[node2.ID])
	}
}

func TestNodesOverview_UsesRedisStateWhenExists(t *testing.T) {
	r, store, stateStore, cleanup := setupNodesOverviewRouter(t)
	defer cleanup()

	node, err := store.Add(model.AgentConfig{Name: "node-online", URL: "http://127.0.0.1:8005"}, 1)
	if err != nil {
		t.Fatalf("add node failed: %v", err)
	}

	state := pendingNodeState(node)
	state.Status = NodeStatusOnline
	state.LastSeenAtUnix = 1710001000
	state.LastPolledAtUnix = 1710001001
	state.CollectedAtUnix = 1710000999
	state.Data = &model.SystemMetrics{Hostname: "node-online", NetSentMB: 12, NetRecvMB: 20}
	state.Metrics = &NodeNetworkMetrics{UpSpeed: 2.5, DownSpeed: 7.2}
	applyDerivedFields(&state, time.Unix(1710001001, 0))
	if err := stateStore.SaveNodeState(context.Background(), state); err != nil {
		t.Fatalf("save node state failed: %v", err)
	}

	token := loginAndGetToken(t, r, "user", "user123")
	req := authorizedRequest(http.MethodGet, "/api/nodes/overview", nil, token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status mismatch: got=%d want=%d body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	var overview []NodeOverview
	if err := json.Unmarshal(w.Body.Bytes(), &overview); err != nil {
		t.Fatalf("unmarshal response failed: %v", err)
	}
	if len(overview) != 1 {
		t.Fatalf("overview length mismatch: got=%d want=1", len(overview))
	}

	got := overview[0]
	if got.Status != NodeStatusOnline {
		t.Fatalf("status mismatch: got=%q want=%q", got.Status, NodeStatusOnline)
	}
	if got.Metrics == nil || got.Metrics.DownSpeed != 7.2 {
		t.Fatalf("metrics mismatch: %+v", got.Metrics)
	}
	if got.Data == nil || got.Data.Hostname != "node-online" {
		t.Fatalf("data mismatch: %+v", got.Data)
	}
	if got.LastSeenAt == nil || *got.LastSeenAt != 1710001000*1000 {
		t.Fatalf("lastSeenAt mismatch: %+v", got.LastSeenAt)
	}
}

func TestNodesOverview_ReturnsPendingWhenRedisEmpty(t *testing.T) {
	r, store, _, cleanup := setupNodesOverviewRouter(t)
	defer cleanup()

	node, err := store.Add(model.AgentConfig{Name: "node-pending", URL: "http://127.0.0.1:8010"}, 1)
	if err != nil {
		t.Fatalf("add node failed: %v", err)
	}

	token := loginAndGetToken(t, r, "user", "user123")
	req := authorizedRequest(http.MethodGet, "/api/nodes/overview", nil, token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status mismatch: got=%d want=%d body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	var overview []NodeOverview
	if err := json.Unmarshal(w.Body.Bytes(), &overview); err != nil {
		t.Fatalf("unmarshal response failed: %v", err)
	}
	if len(overview) != 1 {
		t.Fatalf("overview length mismatch: got=%d want=1", len(overview))
	}

	got := overview[0]
	if got.ID != node.ID {
		t.Fatalf("id mismatch: got=%d want=%d", got.ID, node.ID)
	}
	if got.Status != NodeStatusPending {
		t.Fatalf("status mismatch: got=%q want=%q", got.Status, NodeStatusPending)
	}
	if got.Data != nil || got.Metrics != nil {
		t.Fatalf("pending node data/metrics should be nil: %+v", got)
	}
}

func setupNodesOverviewRouter(t *testing.T) (*gin.Engine, *ConfigStore, *NodeStateStore, func()) {
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

	r := gin.New()
	h := NewHandler(configStore, NewAuthService(db, "test-secret"), VersionInfo{}, nodeService)
	h.RegisterAPIRoutes(r)

	cleanup := func() {
		_ = redisClient.Close()
		mr.Close()
	}
	return r, configStore, stateStore, cleanup
}
