package ai

import (
	"sync"
	"time"
)

// RetrievalStatsCollector collects thread-safe online retrieval metrics.
type RetrievalStatsCollector struct {
	mu             sync.RWMutex
	stats          RetrievalStats
	accTopScoreSum float64
}

// NewRetrievalStatsCollector creates a collector with initial knowledge state.
func NewRetrievalStatsCollector(knowledgeEnabled bool) *RetrievalStatsCollector {
	return &RetrievalStatsCollector{
		stats: RetrievalStats{
			KnowledgeEnabled: knowledgeEnabled,
		},
	}
}

// UpdateKnowledgeState updates loaded document/chunk state after reload.
func (c *RetrievalStatsCollector) UpdateKnowledgeState(enabled bool, loadedDocuments, loadedChunks int, at time.Time) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	c.stats.KnowledgeEnabled = enabled
	if loadedDocuments < 0 {
		loadedDocuments = 0
	}
	if loadedChunks < 0 {
		loadedChunks = 0
	}
	c.stats.LoadedDocuments = loadedDocuments
	c.stats.LoadedChunks = loadedChunks
	if !at.IsZero() {
		c.stats.LastReloadAtUnix = at.Unix()
	}
}

// RecordSearch records one retrieval request for online hit-rate stats.
func (c *RetrievalStatsCollector) RecordSearch(meta RetrievalMeta) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	c.stats.TotalSearches++
	c.accTopScoreSum += meta.TopScore
	if meta.Hit {
		c.stats.RetrievalHits++
	} else {
		c.stats.RetrievalMisses++
	}

	total := c.stats.TotalSearches
	if total > 0 {
		c.stats.OnlineHitRate = float64(c.stats.RetrievalHits) / float64(total)
		c.stats.AvgTopScore = c.accTopScoreSum / float64(total)
	}
}

// Snapshot returns a copy of current retrieval stats.
func (c *RetrievalStatsCollector) Snapshot() RetrievalStats {
	if c == nil {
		return RetrievalStats{}
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.stats
}
