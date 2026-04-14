package ai

import (
	"context"
	"fmt"
	"strings"
)

// EmbeddingProvider defines query/document embedding capability for vector retrieval.
type EmbeddingProvider interface {
	EmbedQuery(ctx context.Context, text string) ([]float32, error)
	EmbedDocuments(ctx context.Context, texts []string) ([][]float32, error)
	ProviderName() string
}

// NewEmbeddingProvider creates a provider instance from retrieval config.
func NewEmbeddingProvider(cfg KnowledgeRetrievalConfig) (EmbeddingProvider, error) {
	if !cfg.Embedding.Enabled {
		return nil, fmt.Errorf("embedding is disabled")
	}
	switch strings.ToLower(strings.TrimSpace(cfg.Embedding.Provider)) {
	case "", defaultEmbeddingProvider, "openai-compatible", "openai":
		return NewOpenAICompatibleEmbeddingProvider(cfg.Embedding)
	default:
		return nil, fmt.Errorf("unsupported embedding provider: %s", cfg.Embedding.Provider)
	}
}
