package gateway

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"nexus-agent-go/internal/collector"
	"nexus-agent-go/internal/model"
)

func newMiniredisForTest(t *testing.T) *miniredis.Miniredis {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis failed: %v", err)
	}
	return mr
}

func newRedisClientForTest(t *testing.T, mr *miniredis.Miniredis) *redis.Client {
	t.Helper()
	return redis.NewClient(&redis.Options{Addr: mr.Addr()})
}

// fakeCollector 按节点返回固定结果，用于在无真实 Agent/SSH 服务时测试状态层。
type fakeCollector struct {
	metrics     model.SystemMetrics
	collectedAt int64
	err         error
	lastNode    model.AgentConfig
}

func (f *fakeCollector) Collect(ctx context.Context, node model.AgentConfig) (model.SystemMetrics, int64, error) {
	f.lastNode = node
	if f.err != nil {
		return model.SystemMetrics{}, 0, f.err
	}
	return f.metrics, f.collectedAt, nil
}

func newServiceWithCollectors(t *testing.T, nodes []model.AgentConfig, agentCollector, sshCollector collector.NodeMetricsCollector, nowFunc func() time.Time) (*NodeStateService, *NodeStateStore, func()) {
	t.Helper()

	router := collector.NewCollectorRouter(agentCollector, sshCollector)
	mr := newMiniredisForTest(t)
	redisClient := newRedisClientForTest(t, mr)
	store := NewNodeStateStore(redisClient, "nexus", 10*time.Minute)
	service := NewNodeStateService(staticConfigLoader{nodes: nodes}, store, NodeStateServiceOptions{
		Collectors: router,
		NowFunc:    nowFunc,
	})

	cleanup := func() {
		_ = redisClient.Close()
		mr.Close()
	}
	return service, store, cleanup
}

func TestNodeStateService_SSHNodePollSuccess(t *testing.T) {
	now := time.Unix(1710000300, 0)

	sshCollector := &fakeCollector{
		metrics: model.SystemMetrics{
			Hostname:   "a6000-d-02",
			IPAddress:  "10.0.0.15",
			CPUUsage:   18.2,
			RAMPercent: 41.1,
		},
		collectedAt: 1710000299,
	}
	service, stateStore, cleanup := newServiceWithCollectors(t,
		[]model.AgentConfig{{
			ID:            7,
			Name:          "A6000-01",
			CollectorType: model.CollectorTypeSSH,
			SSHHost:       "10.0.0.15",
			SSHPort:       22,
			SSHUser:       "renhaokun",
		}},
		&fakeCollector{err: errors.New("agent collector should not be used")},
		sshCollector,
		func() time.Time { return now },
	)
	defer cleanup()

	if err := service.PollOnce(context.Background()); err != nil {
		t.Fatalf("PollOnce failed: %v", err)
	}

	if sshCollector.lastNode.SSHHost != "10.0.0.15" || sshCollector.lastNode.SSHPort != 22 {
		t.Fatalf("ssh collector received wrong node: %+v", sshCollector.lastNode)
	}

	state, ok, err := stateStore.LoadNodeState(context.Background(), 7)
	if err != nil || !ok {
		t.Fatalf("load node state failed: ok=%v err=%v", ok, err)
	}
	if state.Status != NodeStatusOnline {
		t.Fatalf("status = %q, want online", state.Status)
	}
	if state.CollectedAtUnix != 1710000299 {
		t.Fatalf("CollectedAtUnix = %d, want 1710000299", state.CollectedAtUnix)
	}
	if state.URL != "ssh://renhaokun@10.0.0.15:22" {
		t.Fatalf("display endpoint mismatch: %q", state.URL)
	}
	if state.ActiveUserCount != 0 || state.GPUSummary.GPUCount != 0 {
		t.Fatalf("derived fields mismatch: %+v", state)
	}
}

func TestNodeStateService_SSHNodeFailureKeepsPreviousState(t *testing.T) {
	now := time.Unix(1710000400, 0)
	sshCollector := &fakeCollector{
		metrics: model.SystemMetrics{
			Hostname: "a6000-d-02",
			Gpus: []model.GpuInfo{{
				ID:          0,
				Name:        "NVIDIA RTX A6000",
				Utilization: 90,
				MemoryUsed:  40,
				MemoryTotal: 48,
			}},
		},
		collectedAt: 1710000398,
	}

	service, stateStore, cleanup := newServiceWithCollectors(t,
		[]model.AgentConfig{{
			ID:            7,
			Name:          "A6000-01",
			CollectorType: model.CollectorTypeSSH,
			SSHHost:       "10.0.0.15",
			SSHPort:       22,
			SSHUser:       "renhaokun",
		}},
		&fakeCollector{},
		sshCollector,
		func() time.Time { return now },
	)
	defer cleanup()

	if err := service.PollOnce(context.Background()); err != nil {
		t.Fatalf("first PollOnce failed: %v", err)
	}

	// 第二轮采集失败：状态转 offline，但上一轮指标/GPU 汇总/可用性保留。
	sshCollector.err = errors.New("ssh_connect_failed: dial tcp failed")
	offlineNow := now.Add(2 * time.Second)
	service.nowFunc = func() time.Time { return offlineNow }
	if err := service.PollOnce(context.Background()); err != nil {
		t.Fatalf("second PollOnce failed: %v", err)
	}

	state, ok, err := stateStore.LoadNodeState(context.Background(), 7)
	if err != nil || !ok {
		t.Fatalf("load node state failed: ok=%v err=%v", ok, err)
	}
	if state.Status != NodeStatusOffline {
		t.Fatalf("status = %q, want offline", state.Status)
	}
	if state.Data == nil || state.Data.Hostname != "a6000-d-02" || len(state.Data.Gpus) != 1 {
		t.Fatalf("offline state should retain previous data: %+v", state.Data)
	}
	if state.GPUSummary.GPUCount != 1 {
		t.Fatalf("offline state should retain gpu summary: %+v", state.GPUSummary)
	}
	if state.LastSeenAtUnix != now.Unix() {
		t.Fatalf("LastSeenAtUnix should keep previous success time: got=%d want=%d", state.LastSeenAtUnix, now.Unix())
	}
	if state.LastError == "" {
		t.Fatal("LastError should be recorded")
	}
	if state.AvailabilityScore != 0 || state.AvailabilityTier != AvailabilityTierOffline {
		t.Fatalf("offline availability mismatch: score=%d tier=%q", state.AvailabilityScore, state.AvailabilityTier)
	}

	// 第三轮恢复：offline → online，无需重启 Gateway。
	sshCollector.err = nil
	recoveredNow := offlineNow.Add(2 * time.Second)
	service.nowFunc = func() time.Time { return recoveredNow }
	if err := service.PollOnce(context.Background()); err != nil {
		t.Fatalf("third PollOnce failed: %v", err)
	}

	state, ok, err = stateStore.LoadNodeState(context.Background(), 7)
	if err != nil || !ok {
		t.Fatalf("load node state failed: ok=%v err=%v", ok, err)
	}
	if state.Status != NodeStatusOnline {
		t.Fatalf("status = %q, want online after recovery", state.Status)
	}
	if state.LastError != "" {
		t.Fatalf("LastError should be cleared after recovery, got %q", state.LastError)
	}
	if state.CollectedAtUnix != 1710000398 {
		t.Fatalf("CollectedAtUnix should refresh from collector, got %d", state.CollectedAtUnix)
	}
}

func TestNodeStateService_UnsupportedCollectorTypeGoesOffline(t *testing.T) {
	now := time.Unix(1710000500, 0)
	service, stateStore, cleanup := newServiceWithCollectors(t,
		[]model.AgentConfig{{ID: 3, Name: "weird", CollectorType: "prometheus", URL: "http://127.0.0.1:9090"}},
		&fakeCollector{},
		&fakeCollector{},
		func() time.Time { return now },
	)
	defer cleanup()

	if err := service.PollOnce(context.Background()); err != nil {
		t.Fatalf("PollOnce failed: %v", err)
	}

	state, ok, err := stateStore.LoadNodeState(context.Background(), 3)
	if err != nil || !ok {
		t.Fatalf("load node state failed: ok=%v err=%v", ok, err)
	}
	if state.Status != NodeStatusOffline {
		t.Fatalf("status = %q, want offline", state.Status)
	}
	if state.LastError == "" {
		t.Fatal("LastError should describe the unsupported collector type")
	}
}
