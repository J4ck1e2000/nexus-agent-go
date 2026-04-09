package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	rankAvailabilitySuffix = "rank:availability"
	rankIdleGPUSuffix      = "rank:idle_gpu"
	rankFreeVRAMSuffix     = "rank:free_vram_mb"
	nodesTrackedSuffix     = "nodes:tracked"
	nodeStateKeyPattern    = "node:%d:state"
)

// NodeStateStore 负责 Redis 热状态层读写。
type NodeStateStore struct {
	client           redis.Cmdable
	keyPrefix        string
	ttl              time.Duration
	historyRetention time.Duration
}

// NewNodeStateStore 创建 Redis 热状态存储。
func NewNodeStateStore(client redis.Cmdable, keyPrefix string, ttl time.Duration) *NodeStateStore {
	normalizedPrefix := strings.TrimSpace(keyPrefix)
	if normalizedPrefix == "" {
		normalizedPrefix = defaultRedisKeyPrefix
	}
	if ttl <= 0 {
		ttl = defaultNodeStateTTL
	}
	return &NodeStateStore{
		client:           client,
		keyPrefix:        normalizedPrefix,
		ttl:              ttl,
		historyRetention: defaultNodeHistoryRetention,
	}
}

// SaveNodeState 写入节点最新热状态，并维护在线排名索引。
func (s *NodeStateStore) SaveNodeState(ctx context.Context, state NodeState) error {
	if s == nil || s.client == nil {
		return fmt.Errorf("node state store is nil")
	}

	member := strconv.FormatInt(state.ID, 10)
	payload, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("marshal node state failed: %w", err)
	}
	historyPayload, historyTimestamp, err := marshalNodeHistorySnapshot(state)
	if err != nil {
		return err
	}

	pipe := s.client.TxPipeline()
	pipe.Set(ctx, s.nodeStateKey(state.ID), payload, s.ttl)
	pipe.SAdd(ctx, s.nodesTrackedKey(), member)
	pipe.ZAdd(ctx, s.nodeHistoryKey(state.ID), redis.Z{
		Score:  float64(historyTimestamp),
		Member: historyPayload,
	})
	cutoffUnix := historyTimestamp - int64(s.historyRetention.Seconds())
	if cutoffUnix > 0 {
		pipe.ZRemRangeByScore(ctx, s.nodeHistoryKey(state.ID), "-inf", strconv.FormatInt(cutoffUnix, 10))
	}
	pipe.Expire(ctx, s.nodeHistoryKey(state.ID), s.historyRetention)

	if state.Status == NodeStatusOnline {
		freeVRAM := state.GPUSummary.TotalMemory - state.GPUSummary.TotalMemoryUsed
		if freeVRAM < 0 {
			freeVRAM = 0
		}

		pipe.ZAdd(ctx, s.rankAvailabilityKey(), redis.Z{Score: float64(state.AvailabilityScore), Member: member})
		pipe.ZAdd(ctx, s.rankIdleGPUKey(), redis.Z{Score: float64(state.GPUSummary.IdleGpuCount), Member: member})
		pipe.ZAdd(ctx, s.rankFreeVRAMKey(), redis.Z{Score: freeVRAM, Member: member})
	} else {
		pipe.ZRem(ctx, s.rankAvailabilityKey(), member)
		pipe.ZRem(ctx, s.rankIdleGPUKey(), member)
		pipe.ZRem(ctx, s.rankFreeVRAMKey(), member)
	}

	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("save node state failed: %w", err)
	}
	return nil
}

// LoadNodeState 读取单节点热状态。
func (s *NodeStateStore) LoadNodeState(ctx context.Context, nodeID int64) (NodeState, bool, error) {
	states, err := s.LoadNodeStates(ctx, []int64{nodeID})
	if err != nil {
		return NodeState{}, false, err
	}
	state, ok := states[nodeID]
	return state, ok, nil
}

// LoadNodeStates 按节点 ID 批量读取热状态。
func (s *NodeStateStore) LoadNodeStates(ctx context.Context, nodeIDs []int64) (map[int64]NodeState, error) {
	result := make(map[int64]NodeState, len(nodeIDs))
	if s == nil || s.client == nil || len(nodeIDs) == 0 {
		return result, nil
	}

	keys := make([]string, 0, len(nodeIDs))
	for _, nodeID := range nodeIDs {
		keys = append(keys, s.nodeStateKey(nodeID))
	}

	values, err := s.client.MGet(ctx, keys...).Result()
	if err != nil {
		return nil, fmt.Errorf("load node states failed: %w", err)
	}
	if len(values) != len(nodeIDs) {
		return nil, fmt.Errorf("mget values length mismatch")
	}

	for idx, value := range values {
		if value == nil {
			continue
		}
		raw, ok := redisValueToString(value)
		if !ok {
			return nil, fmt.Errorf("invalid redis value for node %d", nodeIDs[idx])
		}
		var state NodeState
		if err := json.Unmarshal([]byte(raw), &state); err != nil {
			return nil, fmt.Errorf("unmarshal node %d state failed: %w", nodeIDs[idx], err)
		}
		result[nodeIDs[idx]] = state
	}

	return result, nil
}

// CleanupRemovedNodes 清理配置已删除节点的热状态与排名索引。
func (s *NodeStateStore) CleanupRemovedNodes(ctx context.Context, activeNodeIDs []int64) error {
	if s == nil || s.client == nil {
		return fmt.Errorf("node state store is nil")
	}

	active := make(map[string]struct{}, len(activeNodeIDs))
	for _, nodeID := range activeNodeIDs {
		active[strconv.FormatInt(nodeID, 10)] = struct{}{}
	}

	trackedMembers, err := s.client.SMembers(ctx, s.nodesTrackedKey()).Result()
	if err != nil {
		return fmt.Errorf("load tracked nodes failed: %w", err)
	}
	if len(trackedMembers) == 0 {
		return nil
	}

	pipe := s.client.TxPipeline()
	changed := false
	for _, member := range trackedMembers {
		if _, exists := active[member]; exists {
			continue
		}
		changed = true
		pipe.SRem(ctx, s.nodesTrackedKey(), member)
		pipe.ZRem(ctx, s.rankAvailabilityKey(), member)
		pipe.ZRem(ctx, s.rankIdleGPUKey(), member)
		pipe.ZRem(ctx, s.rankFreeVRAMKey(), member)

		if nodeID, err := strconv.ParseInt(member, 10, 64); err == nil {
			pipe.Del(ctx, s.nodeStateKey(nodeID))
			pipe.Del(ctx, s.nodeHistoryKey(nodeID))
		}
	}

	if !changed {
		return nil
	}
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("cleanup removed nodes failed: %w", err)
	}
	return nil
}

func redisValueToString(value interface{}) (string, bool) {
	switch v := value.(type) {
	case string:
		return v, true
	case []byte:
		return string(v), true
	default:
		return "", false
	}
}

func (s *NodeStateStore) key(suffix string) string {
	return s.keyPrefix + ":" + suffix
}

func (s *NodeStateStore) nodeStateKey(nodeID int64) string {
	return s.key(fmt.Sprintf(nodeStateKeyPattern, nodeID))
}

func (s *NodeStateStore) rankAvailabilityKey() string {
	return s.key(rankAvailabilitySuffix)
}

func (s *NodeStateStore) rankIdleGPUKey() string {
	return s.key(rankIdleGPUSuffix)
}

func (s *NodeStateStore) rankFreeVRAMKey() string {
	return s.key(rankFreeVRAMSuffix)
}

func (s *NodeStateStore) nodesTrackedKey() string {
	return s.key(nodesTrackedSuffix)
}
