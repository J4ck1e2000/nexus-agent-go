package ai

import (
	"context"
	"fmt"
)

// LocalKnowledgeSearcher adapts the in-memory lexical retriever to KnowledgeSearcher.
type LocalKnowledgeSearcher struct {
	retriever *KnowledgeRetriever
}

// NewLocalKnowledgeSearcher wraps local lexical retrieval.
func NewLocalKnowledgeSearcher(retriever *KnowledgeRetriever) *LocalKnowledgeSearcher {
	return &LocalKnowledgeSearcher{retriever: retriever}
}

// StrategyName returns the stable strategy label of local lexical retrieval.
func (s *LocalKnowledgeSearcher) StrategyName() string {
	return localKnowledgeStrategy
}

// Health validates that local retriever is ready.
func (s *LocalKnowledgeSearcher) Health(ctx context.Context) error {
	_ = ctx
	if s == nil || s.retriever == nil {
		return fmt.Errorf("local knowledge retriever is nil")
	}
	return nil
}

// Search delegates retrieval to the existing in-memory lexical implementation.
func (s *LocalKnowledgeSearcher) Search(ctx context.Context, query string, topK int) ([]KnowledgeHit, RetrievalMeta, error) {
	if s == nil || s.retriever == nil {
		return nil, newRetrievalMeta(query, topK, localKnowledgeStrategy), nil
	}
	hits, meta, err := s.retriever.Search(ctx, query, topK)
	meta.Strategy = localKnowledgeStrategy
	return hits, meta, err
}

// DocumentCount returns local loaded document count.
func (s *LocalKnowledgeSearcher) DocumentCount() int {
	if s == nil || s.retriever == nil {
		return 0
	}
	return s.retriever.DocumentCount()
}

// ChunkCount returns local loaded chunk count.
func (s *LocalKnowledgeSearcher) ChunkCount() int {
	if s == nil || s.retriever == nil {
		return 0
	}
	return s.retriever.ChunkCount()
}
