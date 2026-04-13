package ai

import (
	"sync"
	"testing"
	"time"
)

func TestRetrievalStatsCollector_RecordSearch(t *testing.T) {
	collector := NewRetrievalStatsCollector(true)
	collector.UpdateKnowledgeState(true, 12, 48, time.Unix(1710000000, 0))
	collector.RecordSearch(RetrievalMeta{Hit: true, TopScore: 2.4})
	collector.RecordSearch(RetrievalMeta{Hit: false, TopScore: 0.0})

	stats := collector.Snapshot()
	if !stats.KnowledgeEnabled {
		t.Fatalf("knowledge should be enabled")
	}
	if stats.LoadedDocuments != 12 || stats.LoadedChunks != 48 {
		t.Fatalf("loaded counters mismatch: %+v", stats)
	}
	if stats.TotalSearches != 2 {
		t.Fatalf("total searches mismatch: got=%d", stats.TotalSearches)
	}
	if stats.RetrievalHits != 1 || stats.RetrievalMisses != 1 {
		t.Fatalf("hit/miss mismatch: hits=%d misses=%d", stats.RetrievalHits, stats.RetrievalMisses)
	}
	if stats.OnlineHitRate != 0.5 {
		t.Fatalf("online hit rate mismatch: got=%f", stats.OnlineHitRate)
	}
	if stats.AvgTopScore != 1.2 {
		t.Fatalf("avg top score mismatch: got=%f", stats.AvgTopScore)
	}
}

func TestRetrievalStatsCollector_ConcurrentRecord(t *testing.T) {
	collector := NewRetrievalStatsCollector(true)
	collector.UpdateKnowledgeState(true, 3, 9, time.Now())

	const workers = 16
	const perWorker = 100
	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func(idx int) {
			defer wg.Done()
			for j := 0; j < perWorker; j++ {
				collector.RecordSearch(RetrievalMeta{
					Hit:      (idx+j)%2 == 0,
					TopScore: float64((idx+j)%7) / 10,
				})
			}
		}(i)
	}
	wg.Wait()

	stats := collector.Snapshot()
	expected := int64(workers * perWorker)
	if stats.TotalSearches != expected {
		t.Fatalf("total searches mismatch: got=%d want=%d", stats.TotalSearches, expected)
	}
	if stats.RetrievalHits+stats.RetrievalMisses != expected {
		t.Fatalf("hit+miss mismatch: hits=%d misses=%d total=%d", stats.RetrievalHits, stats.RetrievalMisses, stats.TotalSearches)
	}
	if stats.OnlineHitRate < 0 || stats.OnlineHitRate > 1 {
		t.Fatalf("online hit rate out of range: %f", stats.OnlineHitRate)
	}
}
