package gateway

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"nexus-agent-go/internal/collector"
	"nexus-agent-go/internal/collector/agenthttp"
	"nexus-agent-go/internal/model"
)

const defaultPollMaxConcurrency = 16

type nodeConfigLoader interface {
	Load() ([]model.AgentConfig, error)
}

// NodeStateServiceOptions 为节点状态服务可选参数。
type NodeStateServiceOptions struct {
	PollTimeout time.Duration
	HTTPClient  *http.Client
	NowFunc     func() time.Time
	// Collectors 指定采集路由；为 nil 时按旧行为构建仅含 Agent HTTP 的默认路由。
	Collectors *collector.CollectorRouter
	// MaxConcurrency 限制单轮轮询的并发节点数；<=0 时使用默认值 16。
	MaxConcurrency int
	// MetricHistoryStore 保存按小时采样的长周期资源历史；可为 nil。
	MetricHistoryStore *NodeMetricHistoryStore
}

// NodeStateService 聚合节点轮询、状态缓存和聚合读取能力。
type NodeStateService struct {
	configLoader   nodeConfigLoader
	stateStore     *NodeStateStore
	collectors     *collector.CollectorRouter
	maxConcurrency int
	nowFunc        func() time.Time
	historyStore   *NodeMetricHistoryStore
}

// NewNodeStateService 创建节点状态服务。
func NewNodeStateService(loader nodeConfigLoader, stateStore *NodeStateStore, opts NodeStateServiceOptions) *NodeStateService {
	nowFunc := opts.NowFunc
	if nowFunc == nil {
		nowFunc = time.Now
	}
	router := opts.Collectors
	if router == nil {
		// 兼容旧行为：未显式配置采集路由时，仅启用 Agent HTTP 采集。
		router = collector.NewCollectorRouter(
			agenthttp.New(opts.HTTPClient, opts.PollTimeout),
			nil,
		)
	}
	maxConcurrency := opts.MaxConcurrency
	if maxConcurrency <= 0 {
		maxConcurrency = defaultPollMaxConcurrency
	}
	return &NodeStateService{
		configLoader:   loader,
		stateStore:     stateStore,
		collectors:     router,
		maxConcurrency: maxConcurrency,
		nowFunc:        nowFunc,
		historyStore:   opts.MetricHistoryStore,
	}
}

// PollOnce 执行一次节点轮询并更新 Redis 热状态层。
func (s *NodeStateService) PollOnce(ctx context.Context) error {
	if s == nil || s.configLoader == nil || s.stateStore == nil {
		return fmt.Errorf("node state service not initialized")
	}

	nodes, err := s.configLoader.Load()
	if err != nil {
		return fmt.Errorf("load nodes failed: %w", err)
	}

	nodeIDs := make([]int64, 0, len(nodes))
	for _, node := range nodes {
		nodeIDs = append(nodeIDs, node.ID)
	}
	if s.historyStore != nil {
		if err := s.historyStore.PruneIfDue(ctx, nodeIDs, s.nowFunc()); err != nil {
			log.Printf("node history retention cleanup failed: %v", err)
		}
	}

	if err := s.stateStore.CleanupRemovedNodes(ctx, nodeIDs); err != nil {
		return err
	}
	if len(nodes) == 0 {
		return nil
	}

	previousStates, err := s.stateStore.LoadNodeStates(ctx, nodeIDs)
	if err != nil {
		return err
	}

	results := make(chan NodeState, len(nodes))
	var wg sync.WaitGroup
	// 信号量限制单轮并发采集数，避免节点规模放大后同时发起大量连接。
	sem := make(chan struct{}, s.maxConcurrency)
	for _, node := range nodes {
		node := node
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			var previous *NodeState
			if prev, ok := previousStates[node.ID]; ok {
				prevCopy := prev
				previous = &prevCopy
			}
			results <- s.pollNode(ctx, node, previous)
		}()
	}

	wg.Wait()
	close(results)

	for state := range results {
		if err := s.stateStore.SaveNodeState(ctx, state); err != nil {
			return err
		}
		if s.historyStore != nil {
			if err := s.historyStore.RecordNodeState(ctx, state); err != nil {
				log.Printf("node history sample save failed: node=%d err=%v", state.ID, err)
			}
		}
	}
	return nil
}

// LoadNodeHistory 返回 Redis 的高频近期样本与 MySQL 的小时样本，并按时间去重。
func (s *NodeStateService) LoadNodeHistory(ctx context.Context, nodeID int64, fromUnix int64) ([]NodeHistorySnapshot, error) {
	if s == nil || s.stateStore == nil {
		return nil, fmt.Errorf("node state service not initialized")
	}
	now := s.nowFunc()
	retentionCutoff := now.Add(-defaultMetricHistoryRetentionDays * 24 * time.Hour).Unix()
	if fromUnix < retentionCutoff {
		fromUnix = retentionCutoff
	}
	if fromUnix <= 0 {
		fromUnix = retentionCutoff
	}
	byTimestamp := make(map[int64]NodeHistorySnapshot)

	recentCutoff := now.Add(-defaultNodeHistoryRetention).Unix()
	databaseEnd := recentCutoff
	if s.historyStore != nil && fromUnix < databaseEnd {
		older, err := s.historyStore.LoadNodeHistorySince(ctx, nodeID, fromUnix, databaseEnd)
		if err != nil {
			return nil, err
		}
		for _, snapshot := range older {
			byTimestamp[snapshot.TimestampUnix] = snapshot
		}
	}

	recentFrom := fromUnix
	if recentFrom < recentCutoff {
		recentFrom = recentCutoff
	}
	recent, err := s.stateStore.LoadNodeHistorySince(ctx, nodeID, recentFrom)
	if err != nil {
		return nil, err
	}
	for _, snapshot := range recent {
		byTimestamp[snapshot.TimestampUnix] = snapshot
	}

	result := make([]NodeHistorySnapshot, 0, len(byTimestamp))
	for _, snapshot := range byTimestamp {
		result = append(result, snapshot)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].TimestampUnix < result[j].TimestampUnix })
	return result, nil
}

// GetNodesOverview 读取聚合后的节点概览（优先 Redis 热状态）。
func (s *NodeStateService) GetNodesOverview(ctx context.Context) ([]NodeOverview, error) {
	if s == nil || s.configLoader == nil || s.stateStore == nil {
		return nil, fmt.Errorf("node state service not initialized")
	}

	nodes, err := s.configLoader.Load()
	if err != nil {
		return nil, fmt.Errorf("load nodes failed: %w", err)
	}

	nodeIDs := make([]int64, 0, len(nodes))
	for _, node := range nodes {
		nodeIDs = append(nodeIDs, node.ID)
	}

	stateMap, err := s.stateStore.LoadNodeStates(ctx, nodeIDs)
	if err != nil {
		return nil, err
	}

	now := s.nowFunc()
	overview := make([]NodeOverview, 0, len(nodes))
	for _, node := range nodes {
		state, ok := stateMap[node.ID]
		if !ok {
			overview = append(overview, nodeStateToOverview(pendingNodeState(node)))
			continue
		}

		state.ID = node.ID
		state.Name = node.Name
		state.URL = nodeDisplayEndpoint(node)
		state.SSHHost = node.SSHHost
		state.SSHPort = node.SSHPort
		if strings.TrimSpace(state.Status) == "" {
			state.Status = NodeStatusOffline
		}
		applyDerivedFields(&state, now)
		overview = append(overview, nodeStateToOverview(state))
	}
	return overview, nil
}

func (s *NodeStateService) pollNode(ctx context.Context, node model.AgentConfig, previous *NodeState) NodeState {
	now := s.nowFunc()
	now = normalizePollTime(now)

	state := pendingNodeState(node)
	state.Status = NodeStatusOffline
	state.LastPolledAtUnix = now.Unix()
	if previous != nil {
		state.LastSeenAtUnix = previous.LastSeenAtUnix
		state.CollectedAtUnix = previous.CollectedAtUnix
		state.Data = cloneSystemMetrics(previous.Data)
		state.Metrics = cloneNetworkMetrics(previous.Metrics)
		state.ActiveUsers = append([]string(nil), previous.ActiveUsers...)
		state.ActiveUserCount = previous.ActiveUserCount
		state.GPUSummary = previous.GPUSummary
		state.AvailabilityScore = previous.AvailabilityScore
		state.AvailabilityTier = previous.AvailabilityTier
		state.DataAgeSec = cloneFloat64Ptr(previous.DataAgeSec)
	}

	collectorImpl, err := s.collectors.CollectorFor(node)
	if err != nil {
		state.LastError = err.Error()
		log.Printf("node poller collector select failed: node=%s err=%v", node.Name, err)
		applyDerivedFields(&state, now)
		return state
	}

	metrics, collectedAtUnix, err := collectorImpl.Collect(ctx, node)
	if err != nil {
		state.LastError = err.Error()
		applyDerivedFields(&state, now)
		return state
	}

	state.Status = NodeStatusOnline
	state.LastError = ""
	state.LastSeenAtUnix = now.Unix()
	state.CollectedAtUnix = collectedAtUnix
	state.Data = cloneSystemMetrics(&metrics)
	state.Metrics = computeNetworkSpeed(metrics, previous, now)
	applyDerivedFields(&state, now)
	return state
}
