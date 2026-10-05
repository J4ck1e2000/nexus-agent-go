package gateway

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"nexus-agent-go/internal/model"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestNodeMetricHistoryStoreKeepsOneCompactSamplePerHour(t *testing.T) {
	db := newMetricHistoryTestDB(t)
	store := NewNodeMetricHistoryStore(db)
	firstTime := time.Date(2026, 10, 5, 10, 5, 0, 0, time.UTC).Unix()
	state := historyTestState(4, firstTime, NodeStatusOnline)
	if err := store.RecordNodeState(context.Background(), state); err != nil {
		t.Fatalf("record first sample: %v", err)
	}
	second := historyTestState(4, time.Date(2026, 10, 5, 10, 40, 0, 0, time.UTC).Unix(), NodeStatusOnline)
	second.Data.CPUUsage = 73
	if err := store.RecordNodeState(context.Background(), second); err != nil {
		t.Fatalf("record second sample: %v", err)
	}
	items, err := store.LoadNodeHistorySince(context.Background(), 4, firstTime-1, firstTime+3600)
	if err != nil {
		t.Fatalf("load samples: %v", err)
	}
	if len(items) != 1 || items[0].TimestampUnix != firstTime {
		t.Fatalf("expected one hourly sample at first poll, got %+v", items)
	}
	if len(items[0].GPUs) != 1 || items[0].GPUs[0].ID != 0 || items[0].GPUs[0].ProcessCount != 1 {
		t.Fatalf("compact GPU metrics missing: %+v", items[0].GPUs)
	}
}

func TestNodeMetricHistoryStorePrunesExpiredAndRemovedNodes(t *testing.T) {
	db := newMetricHistoryTestDB(t)
	store := NewNodeMetricHistoryStore(db)
	store.retention = 24 * time.Hour
	now := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	for _, sample := range []struct {
		node int64
		at   int64
	}{{1, now.Add(-48 * time.Hour).Unix()}, {1, now.Add(-time.Hour).Unix()}, {2, now.Add(-time.Hour).Unix()}} {
		if err := store.RecordNodeState(context.Background(), historyTestState(sample.node, sample.at, NodeStatusOnline)); err != nil {
			t.Fatalf("record history: %v", err)
		}
	}
	if err := store.PruneIfDue(context.Background(), []int64{1}, now); err != nil {
		t.Fatalf("prune history: %v", err)
	}
	var count int64
	if err := db.Model(&NodeMetricHistory{}).Count(&count).Error; err != nil {
		t.Fatalf("count history: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected only recent history for active node, got %d rows", count)
	}
}

func TestNodeStateServiceMergesRecentRedisAndLongTermMySQLHistory(t *testing.T) {
	db := newMetricHistoryTestDB(t)
	metricHistory := NewNodeMetricHistoryStore(db)
	redisServer, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}
	t.Cleanup(redisServer.Close)
	redisClient := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	t.Cleanup(func() { _ = redisClient.Close() })
	stateStore := NewNodeStateStore(redisClient, "test", 10*time.Minute)
	now := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	nodeID := int64(9)
	old := historyTestState(nodeID, now.Add(-48*time.Hour).Unix(), NodeStatusOnline)
	if err := metricHistory.RecordNodeState(context.Background(), old); err != nil {
		t.Fatalf("record long-term history: %v", err)
	}
	recent := historyTestState(nodeID, now.Add(-10*time.Minute).Unix(), NodeStatusOnline)
	if err := stateStore.SaveNodeState(context.Background(), recent); err != nil {
		t.Fatalf("record recent Redis history: %v", err)
	}
	service := NewNodeStateService(nil, stateStore, NodeStateServiceOptions{
		NowFunc:            func() time.Time { return now },
		MetricHistoryStore: metricHistory,
	})
	items, err := service.LoadNodeHistory(context.Background(), nodeID, now.Add(-72*time.Hour).Unix())
	if err != nil {
		t.Fatalf("load merged history: %v", err)
	}
	if len(items) != 2 || items[0].TimestampUnix != old.LastPolledAtUnix || items[1].TimestampUnix != recent.LastPolledAtUnix {
		t.Fatalf("expected old MySQL and recent Redis samples in time order, got %+v", items)
	}
	if len(items[1].GPUs) != 1 || items[1].GPUs[0].Name != "Test GPU" {
		t.Fatalf("recent GPU sample missing: %+v", items[1].GPUs)
	}
}

func historyTestState(nodeID int64, timestamp int64, status string) NodeState {
	gpuIndex := 0
	vram := 4096
	return NodeState{
		ID: nodeID, Name: "test-node", Status: status, LastPolledAtUnix: timestamp,
		AvailabilityScore: 80, AvailabilityTier: AvailabilityTierAvailable,
		Data: &model.SystemMetrics{
			CPUUsage: 35, RAMPercent: 48,
			Gpus:      []model.GpuInfo{{ID: 0, Name: "Test GPU", Utilization: 22, MemoryUsed: 8, MemoryTotal: 80, Temperature: 62, PowerDraw: 180}},
			Processes: []model.ProcessInfo{{PID: 42, User: "worker", Command: "train", GPUIndex: &gpuIndex, VRAMUsedMB: &vram}},
		},
	}
}

func newMetricHistoryTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&NodeMetricHistory{}); err != nil {
		t.Fatalf("migrate history: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}
