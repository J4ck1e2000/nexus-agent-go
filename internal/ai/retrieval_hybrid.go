package ai

import "context"

// HybridKnowledgeSearcher is a reserved extension point for lexical+vector fusion.
//
// TODO(next phase):
// 1. lexical topK retrieval
// 2. vector topK retrieval
// 3. merge and deduplicate by chunk_id/document_id
// 4. rerank/fusion (for example reciprocal rank fusion)
type HybridKnowledgeSearcher struct {
	local  KnowledgeSearcher
	vector KnowledgeSearcher
}

func (s *HybridKnowledgeSearcher) StrategyName() string {
	return "hybrid-reserved"
}

func (s *HybridKnowledgeSearcher) Health(ctx context.Context) error {
	_ = ctx
	return nil
}

func (s *HybridKnowledgeSearcher) Search(ctx context.Context, query string, topK int) ([]KnowledgeHit, RetrievalMeta, error) {
	_ = ctx
	return nil, RetrievalMeta{
		Query:    query,
		TopK:     topK,
		Strategy: s.StrategyName(),
	}, nil
}
