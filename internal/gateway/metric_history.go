package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	defaultMetricHistoryRetentionDays = 90
	metricHistoryBucketSeconds        = int64(time.Hour / time.Second)
	metricHistoryPruneInterval        = 24 * time.Hour
)

// NodeMetricHistoryStore 保存低频、紧凑的长期节点样本；高频近期数据仍由 Redis 管理。
type NodeMetricHistoryStore struct {
	db        *gorm.DB
	retention time.Duration
	mu        sync.Mutex
	written   map[int64]historyWriteMarker
	prunedAt  time.Time
}

type historyWriteMarker struct {
	bucket int64
	status string
}

func NewNodeMetricHistoryStore(db *gorm.DB) *NodeMetricHistoryStore {
	return &NodeMetricHistoryStore{
		db:        db,
		retention: defaultMetricHistoryRetentionDays * 24 * time.Hour,
		written:   make(map[int64]historyWriteMarker),
	}
}

func (s *NodeMetricHistoryStore) RecordNodeState(ctx context.Context, state NodeState) error {
	if s == nil || s.db == nil {
		return errors.New("node metric history store is nil")
	}
	if state.ID <= 0 || state.LastPolledAtUnix <= 0 {
		return nil
	}
	bucket := state.LastPolledAtUnix / metricHistoryBucketSeconds * metricHistoryBucketSeconds
	marker := historyWriteMarker{bucket: bucket, status: state.Status}
	s.mu.Lock()
	previous, exists := s.written[state.ID]
	if exists && previous == marker {
		s.mu.Unlock()
		return nil
	}
	s.mu.Unlock()

	snapshot := buildNodeHistorySnapshot(state)
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		return fmt.Errorf("marshal long-term node history failed: %w", err)
	}
	row := NodeMetricHistory{
		NodeID:       state.ID,
		BucketUnix:   bucket,
		SnapshotJSON: string(encoded),
	}
	if err := s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "node_id"}, {Name: "bucket_unix"}},
		DoUpdates: clause.AssignmentColumns([]string{"snapshot_json", "updated_at"}),
	}).Create(&row).Error; err != nil {
		return fmt.Errorf("save long-term node history failed: %w", err)
	}
	s.mu.Lock()
	s.written[state.ID] = marker
	s.mu.Unlock()
	return nil
}

func (s *NodeMetricHistoryStore) LoadNodeHistorySince(ctx context.Context, nodeID int64, fromUnix, untilUnix int64) ([]NodeHistorySnapshot, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("node metric history store is nil")
	}
	if nodeID <= 0 || untilUnix <= fromUnix {
		return []NodeHistorySnapshot{}, nil
	}
	startBucket := fromUnix / metricHistoryBucketSeconds * metricHistoryBucketSeconds
	endBucket := untilUnix/metricHistoryBucketSeconds*metricHistoryBucketSeconds + metricHistoryBucketSeconds
	var rows []NodeMetricHistory
	if err := s.db.WithContext(ctx).
		Where("node_id = ? AND bucket_unix >= ? AND bucket_unix < ?", nodeID, startBucket, endBucket).
		Order("bucket_unix ASC").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("load long-term node history failed: %w", err)
	}
	snapshots := make([]NodeHistorySnapshot, 0, len(rows))
	for _, row := range rows {
		var snapshot NodeHistorySnapshot
		if err := json.Unmarshal([]byte(row.SnapshotJSON), &snapshot); err != nil {
			continue
		}
		if snapshot.TimestampUnix >= fromUnix && snapshot.TimestampUnix < untilUnix {
			snapshots = append(snapshots, snapshot)
		}
	}
	return snapshots, nil
}

// PruneIfDue expires old samples and histories for removed nodes once per day.
func (s *NodeMetricHistoryStore) PruneIfDue(ctx context.Context, activeNodeIDs []int64, now time.Time) error {
	if s == nil || s.db == nil {
		return nil
	}
	s.mu.Lock()
	if !s.prunedAt.IsZero() && now.Sub(s.prunedAt) < metricHistoryPruneInterval {
		s.mu.Unlock()
		return nil
	}
	s.mu.Unlock()

	cutoff := now.Add(-s.retention).Unix()
	if err := s.db.WithContext(ctx).Where("bucket_unix < ?", cutoff).Delete(&NodeMetricHistory{}).Error; err != nil {
		return fmt.Errorf("delete expired node history failed: %w", err)
	}
	if len(activeNodeIDs) == 0 {
		if err := s.db.WithContext(ctx).Where("node_id > 0").Delete(&NodeMetricHistory{}).Error; err != nil {
			return fmt.Errorf("delete removed-node history failed: %w", err)
		}
	} else {
		if err := s.db.WithContext(ctx).Where("node_id NOT IN ?", activeNodeIDs).Delete(&NodeMetricHistory{}).Error; err != nil {
			return fmt.Errorf("delete removed-node history failed: %w", err)
		}
	}
	activeSet := make(map[int64]struct{}, len(activeNodeIDs))
	for _, nodeID := range activeNodeIDs {
		activeSet[nodeID] = struct{}{}
	}
	s.mu.Lock()
	s.prunedAt = now
	for nodeID := range s.written {
		if _, ok := activeSet[nodeID]; !ok {
			delete(s.written, nodeID)
		}
	}
	s.mu.Unlock()
	return nil
}
