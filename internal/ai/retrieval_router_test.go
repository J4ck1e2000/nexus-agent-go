package ai

import (
	"context"
	"errors"
	"testing"
)

type stubKnowledgeSearcher struct {
	strategy  string
	healthErr error
	searchErr error
	hits      []KnowledgeHit
	meta      RetrievalMeta
}

func (s *stubKnowledgeSearcher) Search(ctx context.Context, query string, topK int) ([]KnowledgeHit, RetrievalMeta, error) {
	_ = ctx
	if s.searchErr != nil {
		return nil, RetrievalMeta{Query: query, TopK: topK, Strategy: s.strategy}, s.searchErr
	}
	meta := s.meta
	if meta.TopK <= 0 {
		meta.TopK = topK
	}
	if meta.Query == "" {
		meta.Query = query
	}
	if meta.Strategy == "" {
		meta.Strategy = s.strategy
	}
	return s.hits, meta, nil
}

func (s *stubKnowledgeSearcher) Health(ctx context.Context) error {
	_ = ctx
	return s.healthErr
}

func (s *stubKnowledgeSearcher) StrategyName() string {
	return s.strategy
}

func TestKnowledgeSearchRouter_Local(t *testing.T) {
	local := &stubKnowledgeSearcher{
		strategy: localKnowledgeStrategy,
		hits: []KnowledgeHit{
			{ChunkID: "local-1", DocumentID: "doc-local", Score: 2.1},
		},
		meta: RetrievalMeta{ReturnedCount: 1, Hit: true, TopScore: 2.1},
	}

	router := NewKnowledgeSearchRouter(KnowledgeBackendLocal, local, nil, nil)
	hits, meta, err := router.Search(context.Background(), "test", 3)
	if err != nil {
		t.Fatalf("router search failed: %v", err)
	}
	if len(hits) != 1 || hits[0].ChunkID != "local-1" {
		t.Fatalf("unexpected local hits: %+v", hits)
	}
	if meta.Strategy != localKnowledgeStrategy {
		t.Fatalf("strategy mismatch: got=%s want=%s", meta.Strategy, localKnowledgeStrategy)
	}
}

func TestKnowledgeSearchRouter_Qdrant(t *testing.T) {
	qdrant := &stubKnowledgeSearcher{
		strategy: qdrantKnowledgeStrategy,
		hits: []KnowledgeHit{
			{ChunkID: "qdrant-1", DocumentID: "doc-qdrant", Score: 0.9},
		},
		meta: RetrievalMeta{ReturnedCount: 1, Hit: true, TopScore: 0.9},
	}
	router := NewKnowledgeSearchRouter(KnowledgeBackendQdrant, nil, qdrant, nil)
	hits, meta, err := router.Search(context.Background(), "oom", 3)
	if err != nil {
		t.Fatalf("router search failed: %v", err)
	}
	if len(hits) != 1 || hits[0].ChunkID != "qdrant-1" {
		t.Fatalf("unexpected qdrant hits: %+v", hits)
	}
	if meta.Strategy != qdrantKnowledgeStrategy {
		t.Fatalf("strategy mismatch: got=%s want=%s", meta.Strategy, qdrantKnowledgeStrategy)
	}
}

func TestKnowledgeSearchRouter_QdrantMissing(t *testing.T) {
	router := NewKnowledgeSearchRouter(KnowledgeBackendQdrant, nil, nil, nil)
	_, _, err := router.Search(context.Background(), "oom", 3)
	if err == nil {
		t.Fatalf("expected error for missing qdrant backend")
	}
}

func TestKnowledgeSearchRouter_AutoFallback(t *testing.T) {
	local := &stubKnowledgeSearcher{
		strategy: localKnowledgeStrategy,
		hits: []KnowledgeHit{
			{ChunkID: "local-1", DocumentID: "doc-local", Score: 2.0},
		},
		meta: RetrievalMeta{ReturnedCount: 1, Hit: true, TopScore: 2.0},
	}
	qdrant := &stubKnowledgeSearcher{
		strategy:  qdrantKnowledgeStrategy,
		searchErr: errors.New("qdrant search failed"),
	}
	router := NewKnowledgeSearchRouter(KnowledgeBackendAuto, local, qdrant, nil)
	hits, meta, err := router.Search(context.Background(), "oom", 3)
	if err != nil {
		t.Fatalf("auto fallback search failed: %v", err)
	}
	if len(hits) != 1 || hits[0].ChunkID != "local-1" {
		t.Fatalf("unexpected fallback hits: %+v", hits)
	}
	if meta.Strategy != autoFallbackLocalStrategy {
		t.Fatalf("fallback strategy mismatch: got=%s want=%s", meta.Strategy, autoFallbackLocalStrategy)
	}
}

func TestKnowledgeSearchRouter_AutoQdrantSuccess(t *testing.T) {
	local := &stubKnowledgeSearcher{
		strategy: localKnowledgeStrategy,
		hits: []KnowledgeHit{
			{ChunkID: "local-1", DocumentID: "doc-local", Score: 2.0},
		},
	}
	qdrant := &stubKnowledgeSearcher{
		strategy: qdrantKnowledgeStrategy,
		hits: []KnowledgeHit{
			{ChunkID: "qdrant-1", DocumentID: "doc-qdrant", Score: 0.91},
		},
		meta: RetrievalMeta{ReturnedCount: 1, Hit: true, TopScore: 0.91},
	}
	router := NewKnowledgeSearchRouter(KnowledgeBackendAuto, local, qdrant, nil)
	hits, meta, err := router.Search(context.Background(), "gpu oom", 3)
	if err != nil {
		t.Fatalf("auto search failed: %v", err)
	}
	if len(hits) != 1 || hits[0].ChunkID != "qdrant-1" {
		t.Fatalf("unexpected auto hits: %+v", hits)
	}
	if meta.Strategy != qdrantKnowledgeStrategy {
		t.Fatalf("strategy mismatch: got=%s want=%s", meta.Strategy, qdrantKnowledgeStrategy)
	}
}
