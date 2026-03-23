package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"nexus-agent-go/internal/model"
)

func TestNodePoller_PollOnceSuccessWritesOnlineState(t *testing.T) {
	now := time.Unix(1710000000, 0)
	currentNow := now

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/metrics/current" {
			http.NotFound(w, r)
			return
		}
		resp := struct {
			CollectedAtUnix int64 `json:"collected_at_unix"`
			model.SystemMetrics
		}{
			CollectedAtUnix: 1709999998,
			SystemMetrics: model.SystemMetrics{
				Hostname:   "node-01",
				CPUUsage:   42,
				RAMPercent: 48,
				NetSentMB:  100,
				NetRecvMB:  200,
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	service, stateStore, cleanup := newNodeStateServiceForTest(t, []model.AgentConfig{
		{ID: 1, Name: "server-01", URL: server.URL},
	}, func() time.Time { return currentNow })
	defer cleanup()

	if err := service.PollOnce(context.Background()); err != nil {
		t.Fatalf("PollOnce failed: %v", err)
	}

	state, ok, err := stateStore.LoadNodeState(context.Background(), 1)
	if err != nil {
		t.Fatalf("LoadNodeState failed: %v", err)
	}
	if !ok {
		t.Fatalf("node state not found")
	}
	if state.Status != NodeStatusOnline {
		t.Fatalf("status = %q, want %q", state.Status, NodeStatusOnline)
	}
	if state.Data == nil || state.Data.Hostname != "node-01" {
		t.Fatalf("data mismatch: %+v", state.Data)
	}
	if state.LastSeenAtUnix != now.Unix() {
		t.Fatalf("LastSeenAtUnix = %d, want %d", state.LastSeenAtUnix, now.Unix())
	}
	if state.CollectedAtUnix != 1709999998 {
		t.Fatalf("CollectedAtUnix = %d, want %d", state.CollectedAtUnix, 1709999998)
	}
}

func TestNodePoller_PollOnceFailureMarksOfflineButKeepsLastKnown(t *testing.T) {
	now := time.Unix(1710000100, 0)
	currentNow := now
	shouldFail := false

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if shouldFail {
			http.Error(w, "upstream down", http.StatusBadGateway)
			return
		}
		resp := struct {
			CollectedAtUnix int64 `json:"collected_at_unix"`
			model.SystemMetrics
		}{
			CollectedAtUnix: 1710000097,
			SystemMetrics: model.SystemMetrics{
				Hostname:  "node-keep",
				NetSentMB: 10,
				NetRecvMB: 20,
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	service, stateStore, cleanup := newNodeStateServiceForTest(t, []model.AgentConfig{
		{ID: 1, Name: "server-01", URL: server.URL},
	}, func() time.Time { return currentNow })
	defer cleanup()

	if err := service.PollOnce(context.Background()); err != nil {
		t.Fatalf("first PollOnce failed: %v", err)
	}

	currentNow = now.Add(2 * time.Second)
	shouldFail = true
	if err := service.PollOnce(context.Background()); err != nil {
		t.Fatalf("second PollOnce failed: %v", err)
	}

	state, ok, err := stateStore.LoadNodeState(context.Background(), 1)
	if err != nil {
		t.Fatalf("LoadNodeState failed: %v", err)
	}
	if !ok {
		t.Fatalf("node state not found")
	}
	if state.Status != NodeStatusOffline {
		t.Fatalf("status = %q, want %q", state.Status, NodeStatusOffline)
	}
	if state.Data == nil || state.Data.Hostname != "node-keep" {
		t.Fatalf("offline state should keep last known data: %+v", state.Data)
	}
	if state.LastSeenAtUnix != now.Unix() {
		t.Fatalf("LastSeenAtUnix should keep previous success time, got=%d want=%d", state.LastSeenAtUnix, now.Unix())
	}
	if state.LastError == "" {
		t.Fatalf("LastError should be recorded")
	}
}

func TestNodePoller_ComputesNetworkSpeed(t *testing.T) {
	now := time.Unix(1710000200, 0)
	currentNow := now

	var mu sync.Mutex
	netSent := 100.0
	netRecv := 200.0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		resp := struct {
			CollectedAtUnix int64 `json:"collected_at_unix"`
			model.SystemMetrics
		}{
			CollectedAtUnix: currentNow.Unix() - 1,
			SystemMetrics: model.SystemMetrics{
				Hostname:  "node-speed",
				NetSentMB: netSent,
				NetRecvMB: netRecv,
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	service, stateStore, cleanup := newNodeStateServiceForTest(t, []model.AgentConfig{
		{ID: 1, Name: "server-01", URL: server.URL},
	}, func() time.Time { return currentNow })
	defer cleanup()

	if err := service.PollOnce(context.Background()); err != nil {
		t.Fatalf("first PollOnce failed: %v", err)
	}

	mu.Lock()
	netSent = 160
	netRecv = 250
	mu.Unlock()
	currentNow = now.Add(2 * time.Second)
	if err := service.PollOnce(context.Background()); err != nil {
		t.Fatalf("second PollOnce failed: %v", err)
	}

	state, ok, err := stateStore.LoadNodeState(context.Background(), 1)
	if err != nil {
		t.Fatalf("LoadNodeState failed: %v", err)
	}
	if !ok || state.Metrics == nil {
		t.Fatalf("state metrics should exist, ok=%v state=%+v", ok, state)
	}

	// up=(160-100)/2=30, down=(250-200)/2=25
	if diff := mathAbs(state.Metrics.UpSpeed - 30); diff > 0.0001 {
		t.Fatalf("UpSpeed = %f, want 30", state.Metrics.UpSpeed)
	}
	if diff := mathAbs(state.Metrics.DownSpeed - 25); diff > 0.0001 {
		t.Fatalf("DownSpeed = %f, want 25", state.Metrics.DownSpeed)
	}
}

type staticConfigLoader struct {
	nodes []model.AgentConfig
}

func (s staticConfigLoader) Load() ([]model.AgentConfig, error) {
	return append([]model.AgentConfig(nil), s.nodes...), nil
}

func newNodeStateServiceForTest(t *testing.T, nodes []model.AgentConfig, nowFunc func() time.Time) (*NodeStateService, *NodeStateStore, func()) {
	t.Helper()

	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis failed: %v", err)
	}
	redisClient := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	store := NewNodeStateStore(redisClient, "nexus", 10*time.Minute)
	service := NewNodeStateService(staticConfigLoader{nodes: nodes}, store, NodeStateServiceOptions{
		PollTimeout: 3 * time.Second,
		NowFunc:     nowFunc,
	})

	cleanup := func() {
		_ = redisClient.Close()
		mr.Close()
	}
	return service, store, cleanup
}

func mathAbs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}
