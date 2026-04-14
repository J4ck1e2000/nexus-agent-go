package ai

import (
	"context"
	"fmt"
	"strings"
)

const (
	KnowledgeBackendLocal  = "local"
	KnowledgeBackendQdrant = "qdrant"
	KnowledgeBackendAuto   = "auto"
)

const (
	localKnowledgeStrategy    = "local-lexical"
	qdrantKnowledgeStrategy   = "qdrant-vector"
	autoFallbackLocalStrategy = "auto-fallback-local"
)

// KnowledgeSearcher defines a pluggable knowledge retrieval backend.
type KnowledgeSearcher interface {
	Search(ctx context.Context, query string, topK int) ([]KnowledgeHit, RetrievalMeta, error)
	Health(ctx context.Context) error
	StrategyName() string
}

// KnowledgeSearchRouter centralizes backend selection and fallback behavior.
type KnowledgeSearchRouter struct {
	backend string
	local   KnowledgeSearcher
	qdrant  KnowledgeSearcher
	logf    func(format string, args ...any)
}

// NewKnowledgeSearchRouter creates a router for local/qdrant/auto backends.
func NewKnowledgeSearchRouter(backend string, local, qdrant KnowledgeSearcher, logf func(format string, args ...any)) *KnowledgeSearchRouter {
	return &KnowledgeSearchRouter{
		backend: normalizeKnowledgeBackend(backend),
		local:   local,
		qdrant:  qdrant,
		logf:    logf,
	}
}

// StrategyName reports the configured routing strategy.
func (r *KnowledgeSearchRouter) StrategyName() string {
	if r == nil {
		return KnowledgeBackendAuto
	}
	return normalizeKnowledgeBackend(r.backend)
}

// Health checks selected backend health according to router mode.
func (r *KnowledgeSearchRouter) Health(ctx context.Context) error {
	if r == nil {
		return fmt.Errorf("knowledge search router is nil")
	}

	switch r.backend {
	case KnowledgeBackendLocal:
		if r.local == nil {
			return fmt.Errorf("local knowledge searcher is not configured")
		}
		return r.local.Health(ctx)
	case KnowledgeBackendQdrant:
		if r.qdrant == nil {
			return fmt.Errorf("qdrant knowledge searcher is not configured")
		}
		return r.qdrant.Health(ctx)
	default:
		if r.qdrant != nil {
			if err := r.qdrant.Health(ctx); err == nil {
				return nil
			}
		}
		if r.local != nil {
			return r.local.Health(ctx)
		}
		return fmt.Errorf("no knowledge search backend is configured")
	}
}

// Search routes retrieval to the configured backend with optional auto fallback.
func (r *KnowledgeSearchRouter) Search(ctx context.Context, query string, topK int) ([]KnowledgeHit, RetrievalMeta, error) {
	if r == nil {
		return nil, RetrievalMeta{
			Query:    strings.TrimSpace(query),
			TopK:     normalizeKnowledgeTopK(topK),
			Strategy: KnowledgeBackendAuto,
		}, fmt.Errorf("knowledge search router is nil")
	}

	switch r.backend {
	case KnowledgeBackendLocal:
		return r.searchWithLocal(ctx, query, topK)
	case KnowledgeBackendQdrant:
		return r.searchWithQdrant(ctx, query, topK)
	default:
		return r.searchWithAutoFallback(ctx, query, topK)
	}
}

func (r *KnowledgeSearchRouter) searchWithLocal(ctx context.Context, query string, topK int) ([]KnowledgeHit, RetrievalMeta, error) {
	if r.local == nil {
		return nil, newRetrievalMeta(query, topK, localKnowledgeStrategy), nil
	}
	hits, meta, err := r.local.Search(ctx, query, topK)
	if strings.TrimSpace(meta.Strategy) == "" {
		meta.Strategy = localKnowledgeStrategy
	}
	return hits, meta, err
}

func (r *KnowledgeSearchRouter) searchWithQdrant(ctx context.Context, query string, topK int) ([]KnowledgeHit, RetrievalMeta, error) {
	if r.qdrant == nil {
		return nil, newRetrievalMeta(query, topK, qdrantKnowledgeStrategy), fmt.Errorf("qdrant knowledge searcher is not configured")
	}
	if err := r.qdrant.Health(ctx); err != nil {
		return nil, newRetrievalMeta(query, topK, qdrantKnowledgeStrategy), err
	}
	hits, meta, err := r.qdrant.Search(ctx, query, topK)
	if strings.TrimSpace(meta.Strategy) == "" {
		meta.Strategy = qdrantKnowledgeStrategy
	}
	return hits, meta, err
}

func (r *KnowledgeSearchRouter) searchWithAutoFallback(ctx context.Context, query string, topK int) ([]KnowledgeHit, RetrievalMeta, error) {
	if r.qdrant != nil {
		if err := r.qdrant.Health(ctx); err == nil {
			hits, meta, searchErr := r.qdrant.Search(ctx, query, topK)
			if searchErr == nil {
				if strings.TrimSpace(meta.Strategy) == "" {
					meta.Strategy = qdrantKnowledgeStrategy
				}
				return hits, meta, nil
			}
			r.logfSafe("knowledge search auto fallback: qdrant search failed: %v", searchErr)
		} else {
			r.logfSafe("knowledge search auto fallback: qdrant health failed: %v", err)
		}
	} else {
		r.logfSafe("knowledge search auto fallback: qdrant backend is not configured")
	}

	hits, meta, err := r.searchWithLocal(ctx, query, topK)
	if err != nil {
		return nil, meta, err
	}
	meta.Strategy = autoFallbackLocalStrategy
	if strings.TrimSpace(meta.Query) == "" {
		meta.Query = strings.TrimSpace(query)
	}
	if meta.TopK <= 0 {
		meta.TopK = normalizeKnowledgeTopK(topK)
	}
	return hits, meta, nil
}

func (r *KnowledgeSearchRouter) logfSafe(format string, args ...any) {
	if r == nil || r.logf == nil {
		return
	}
	r.logf(format, args...)
}

func normalizeKnowledgeBackend(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case KnowledgeBackendLocal:
		return KnowledgeBackendLocal
	case KnowledgeBackendQdrant:
		return KnowledgeBackendQdrant
	default:
		return KnowledgeBackendAuto
	}
}

func normalizeKnowledgeTopK(topK int) int {
	if topK <= 0 {
		return defaultKnowledgeTopK
	}
	return topK
}

func newRetrievalMeta(query string, topK int, strategy string) RetrievalMeta {
	return RetrievalMeta{
		Query:    strings.TrimSpace(query),
		TopK:     normalizeKnowledgeTopK(topK),
		Strategy: strings.TrimSpace(strategy),
	}
}
