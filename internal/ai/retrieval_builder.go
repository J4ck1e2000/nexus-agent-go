package ai

import (
	"context"
	"fmt"
	"strings"
)

// KnowledgeSearchBuildResult contains initialized knowledge search backends.
type KnowledgeSearchBuildResult struct {
	Searcher  KnowledgeSearcher
	Local     KnowledgeSearcher
	Qdrant    KnowledgeSearcher
	Documents int
	Chunks    int
}

// BuildKnowledgeSearcher wires local/qdrant backends and returns one unified searcher.
func BuildKnowledgeSearcher(
	ctx context.Context,
	knowledgeDir string,
	localOpts KnowledgeRetrieverOptions,
	cfg KnowledgeRetrievalConfig,
	logf func(format string, args ...any),
) (KnowledgeSearchBuildResult, error) {
	result := KnowledgeSearchBuildResult{}
	backend := cfg.NormalizedBackend()

	localSearcher, docs, chunks, localErr := buildLocalSearcher(knowledgeDir, localOpts)
	if localErr != nil {
		if backend == KnowledgeBackendLocal {
			return result, localErr
		}
		logfBuild(logf, "knowledge local retriever disabled: %v", localErr)
	}
	result.Local = localSearcher
	result.Documents = docs
	result.Chunks = chunks

	var qdrantSearcher KnowledgeSearcher
	if backend == KnowledgeBackendQdrant || backend == KnowledgeBackendAuto {
		if cfg.VectorRetrievalEnabled() {
			embedding, embErr := NewEmbeddingProvider(cfg)
			if embErr != nil {
				if backend == KnowledgeBackendQdrant {
					return result, embErr
				}
				logfBuild(logf, "knowledge qdrant retriever disabled: embedding init failed: %v", embErr)
			} else {
				searcher, qErr := NewQdrantKnowledgeSearcher(ctx, cfg, embedding)
				if qErr != nil {
					if backend == KnowledgeBackendQdrant {
						return result, qErr
					}
					logfBuild(logf, "knowledge qdrant retriever disabled: %v", qErr)
				} else {
					qdrantSearcher = searcher
				}
			}
		} else if backend == KnowledgeBackendQdrant {
			return result, fmt.Errorf("backend=qdrant but vector retrieval is not enabled")
		} else {
			logfBuild(logf, "knowledge qdrant retriever disabled: vector retrieval config incomplete")
		}
	}
	result.Qdrant = qdrantSearcher

	switch backend {
	case KnowledgeBackendLocal:
		result.Searcher = result.Local
	case KnowledgeBackendQdrant:
		result.Searcher = result.Qdrant
	default:
		if result.Local == nil && result.Qdrant == nil {
			result.Searcher = nil
		} else {
			result.Searcher = NewKnowledgeSearchRouter(backend, result.Local, result.Qdrant, logf)
		}
	}
	logfBuild(
		logf,
		"knowledge backend initialized: backend=%s local_ready=%t qdrant_ready=%t docs=%d chunks=%d",
		backend,
		result.Local != nil,
		result.Qdrant != nil,
		result.Documents,
		result.Chunks,
	)
	return result, nil
}

func buildLocalSearcher(knowledgeDir string, opts KnowledgeRetrieverOptions) (KnowledgeSearcher, int, int, error) {
	dir := strings.TrimSpace(knowledgeDir)
	if dir == "" {
		return nil, 0, 0, fmt.Errorf("knowledge dir is empty")
	}
	base, err := LoadKnowledgeBase(dir)
	if err != nil {
		return nil, 0, 0, err
	}
	if len(base.Chunks) == 0 {
		return nil, len(base.Documents), 0, fmt.Errorf("knowledge base has no chunks: %s", dir)
	}
	retriever := NewKnowledgeRetriever(base, opts)
	return NewLocalKnowledgeSearcher(retriever), len(base.Documents), len(base.Chunks), nil
}

func logfBuild(logf func(format string, args ...any), format string, args ...any) {
	if logf == nil {
		return
	}
	logf(format, args...)
}
