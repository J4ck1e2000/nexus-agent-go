package ai

import (
	"context"
	"testing"
)

func TestLocalKnowledgeSearcher_Compatibility(t *testing.T) {
	retriever := buildTestRetriever(t)
	searcher := NewLocalKnowledgeSearcher(retriever)

	query := "GPU 显存爆满怎么处理"
	hitsA, metaA, err := retriever.Search(context.Background(), query, 3)
	if err != nil {
		t.Fatalf("retriever search failed: %v", err)
	}
	hitsB, metaB, err := searcher.Search(context.Background(), query, 3)
	if err != nil {
		t.Fatalf("searcher search failed: %v", err)
	}

	if len(hitsA) == 0 || len(hitsB) == 0 {
		t.Fatalf("expected non-empty hits from both retriever and searcher")
	}
	if hitsA[0].ChunkID != hitsB[0].ChunkID {
		t.Fatalf("top hit mismatch: retriever=%s searcher=%s", hitsA[0].ChunkID, hitsB[0].ChunkID)
	}
	if metaA.ReturnedCount != metaB.ReturnedCount {
		t.Fatalf("returned count mismatch: retriever=%d searcher=%d", metaA.ReturnedCount, metaB.ReturnedCount)
	}
	if searcher.StrategyName() != localKnowledgeStrategy {
		t.Fatalf("strategy mismatch: got=%s want=%s", searcher.StrategyName(), localKnowledgeStrategy)
	}
}
