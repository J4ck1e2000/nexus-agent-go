package gateway

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"nexus-agent-go/internal/model"
)

func TestNodeStateStore_SaveAndLoad(t *testing.T) {
	store, redisClient, cleanup := newNodeStateStoreForTest(t)
	defer cleanup()

	state := NodeState{
		ID:                1,
		Name:              "server-01",
		URL:               "http://127.0.0.1:8005",
		Status:            NodeStatusOnline,
		LastPolledAtUnix:  1710000100,
		LastSeenAtUnix:    1710000100,
		CollectedAtUnix:   1710000098,
		Data:              &model.SystemMetrics{Hostname: "node-01", NetSentMB: 100, NetRecvMB: 200},
		Metrics:           &NodeNetworkMetrics{UpSpeed: 1.2, DownSpeed: 3.4},
		ActiveUsers:       []string{"alice"},
		ActiveUserCount:   1,
		GPUSummary:        NodeGPUSummary{GPUCount: 2, IdleGpuCount: 1, BusyGpuCount: 1, TotalMemory: 48, TotalMemoryUsed: 24},
		AvailabilityScore: 72,
		AvailabilityTier:  AvailabilityTierAvailable,
	}
	applyDerivedFields(&state, time.Unix(1710000100, 0))

	if err := store.SaveNodeState(context.Background(), state); err != nil {
		t.Fatalf("SaveNodeState failed: %v", err)
	}

	got, ok, err := store.LoadNodeState(context.Background(), state.ID)
	if err != nil {
		t.Fatalf("LoadNodeState failed: %v", err)
	}
	if !ok {
		t.Fatalf("state not found")
	}
	if got.Status != NodeStatusOnline {
		t.Fatalf("status = %q, want %q", got.Status, NodeStatusOnline)
	}
	if got.Data == nil || got.Data.Hostname != "node-01" {
		t.Fatalf("data mismatch: %+v", got.Data)
	}
	if got.Metrics == nil || math.Abs(got.Metrics.DownSpeed-3.4) > 0.0001 {
		t.Fatalf("metrics mismatch: %+v", got.Metrics)
	}

	member := "1"
	if _, err := redisClient.ZScore(context.Background(), store.rankAvailabilityKey(), member).Result(); err != nil {
		t.Fatalf("availability zset should contain node: %v", err)
	}
	if _, err := redisClient.ZScore(context.Background(), store.rankIdleGPUKey(), member).Result(); err != nil {
		t.Fatalf("idle gpu zset should contain node: %v", err)
	}
	if _, err := redisClient.ZScore(context.Background(), store.rankFreeVRAMKey(), member).Result(); err != nil {
		t.Fatalf("free vram zset should contain node: %v", err)
	}
}

func TestNodeStateStore_OfflineRemovesRanks(t *testing.T) {
	store, redisClient, cleanup := newNodeStateStoreForTest(t)
	defer cleanup()

	online := NodeState{
		ID:                1,
		Name:              "server-01",
		URL:               "http://127.0.0.1:8005",
		Status:            NodeStatusOnline,
		AvailabilityScore: 88,
		GPUSummary:        NodeGPUSummary{GPUCount: 4, IdleGpuCount: 3, TotalMemory: 96, TotalMemoryUsed: 10},
	}
	if err := store.SaveNodeState(context.Background(), online); err != nil {
		t.Fatalf("save online state failed: %v", err)
	}

	offline := online
	offline.Status = NodeStatusOffline
	if err := store.SaveNodeState(context.Background(), offline); err != nil {
		t.Fatalf("save offline state failed: %v", err)
	}

	member := "1"
	if _, err := redisClient.ZScore(context.Background(), store.rankAvailabilityKey(), member).Result(); err == nil {
		t.Fatalf("availability zset should remove offline node")
	}
	if _, err := redisClient.ZScore(context.Background(), store.rankIdleGPUKey(), member).Result(); err == nil {
		t.Fatalf("idle gpu zset should remove offline node")
	}
	if _, err := redisClient.ZScore(context.Background(), store.rankFreeVRAMKey(), member).Result(); err == nil {
		t.Fatalf("free vram zset should remove offline node")
	}
}

func TestNodeStateStore_CleanupRemovedNodes(t *testing.T) {
	store, redisClient, cleanup := newNodeStateStoreForTest(t)
	defer cleanup()

	state1 := NodeState{ID: 1, Name: "n1", URL: "http://n1", Status: NodeStatusOnline, AvailabilityScore: 80, GPUSummary: NodeGPUSummary{GPUCount: 1, IdleGpuCount: 1, TotalMemory: 24, TotalMemoryUsed: 8}}
	state2 := NodeState{ID: 2, Name: "n2", URL: "http://n2", Status: NodeStatusOnline, AvailabilityScore: 60, GPUSummary: NodeGPUSummary{GPUCount: 1, IdleGpuCount: 0, TotalMemory: 24, TotalMemoryUsed: 20}}
	if err := store.SaveNodeState(context.Background(), state1); err != nil {
		t.Fatalf("save state1 failed: %v", err)
	}
	if err := store.SaveNodeState(context.Background(), state2); err != nil {
		t.Fatalf("save state2 failed: %v", err)
	}

	if err := store.CleanupRemovedNodes(context.Background(), []int64{1}); err != nil {
		t.Fatalf("cleanup failed: %v", err)
	}

	if _, ok, err := store.LoadNodeState(context.Background(), 1); err != nil || !ok {
		t.Fatalf("node 1 should remain, ok=%v err=%v", ok, err)
	}
	if _, ok, err := store.LoadNodeState(context.Background(), 2); err != nil {
		t.Fatalf("load node 2 failed: %v", err)
	} else if ok {
		t.Fatalf("node 2 should be removed")
	}

	tracked, err := redisClient.SMembers(context.Background(), store.nodesTrackedKey()).Result()
	if err != nil {
		t.Fatalf("smembers failed: %v", err)
	}
	if len(tracked) != 1 || tracked[0] != "1" {
		t.Fatalf("tracked nodes mismatch: %+v", tracked)
	}

	if entries, err := redisClient.ZCard(context.Background(), store.nodeHistoryKey(2)).Result(); err != nil {
		t.Fatalf("query node history failed: %v", err)
	} else if entries != 0 {
		t.Fatalf("node 2 history should be removed, got=%d", entries)
	}
}

func TestNodeStateStore_LoadNodeHistorySince(t *testing.T) {
	store, _, cleanup := newNodeStateStoreForTest(t)
	defer cleanup()

	state := NodeState{
		ID:                7,
		Name:              "server-07",
		URL:               "http://127.0.0.1:8017",
		Status:            NodeStatusOnline,
		LastPolledAtUnix:  1710000200,
		LastSeenAtUnix:    1710000200,
		AvailabilityScore: 76,
		AvailabilityTier:  AvailabilityTierAvailable,
		Data: &model.SystemMetrics{
			CPUUsage:   44,
			RAMPercent: 55,
		},
		GPUSummary:      NodeGPUSummary{GPUCount: 2, IdleGpuCount: 1, BusyGpuCount: 1, GpuPressure: 42, BusyRatio: 0.5},
		ActiveUserCount: 1,
	}
	if err := store.SaveNodeState(context.Background(), state); err != nil {
		t.Fatalf("save node state failed: %v", err)
	}

	state2 := state
	state2.LastPolledAtUnix = 1710000260
	state2.AvailabilityScore = 60
	state2.AvailabilityTier = AvailabilityTierBusy
	state2.GPUSummary.GpuPressure = 70
	state2.GPUSummary.BusyRatio = 1
	if err := store.SaveNodeState(context.Background(), state2); err != nil {
		t.Fatalf("save node state2 failed: %v", err)
	}

	history, err := store.LoadNodeHistorySince(context.Background(), state.ID, 1710000000)
	if err != nil {
		t.Fatalf("LoadNodeHistorySince failed: %v", err)
	}
	if len(history) != 2 {
		t.Fatalf("history length mismatch: got=%d want=2", len(history))
	}
	if history[0].NodeName != "server-07" {
		t.Fatalf("node name mismatch: got=%q", history[0].NodeName)
	}
	if history[1].AvailabilityTier != AvailabilityTierBusy {
		t.Fatalf("availability tier mismatch: got=%q want=%q", history[1].AvailabilityTier, AvailabilityTierBusy)
	}
	if history[1].TimestampUnix != 1710000260 {
		t.Fatalf("timestamp mismatch: got=%d want=%d", history[1].TimestampUnix, 1710000260)
	}
}

func newNodeStateStoreForTest(t *testing.T) (*NodeStateStore, *redis.Client, func()) {
	t.Helper()

	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis failed: %v", err)
	}
	redisClient := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	store := NewNodeStateStore(redisClient, "nexus", 10*time.Minute)

	cleanup := func() {
		_ = redisClient.Close()
		mr.Close()
	}
	return store, redisClient, cleanup
}
