package ai

import "testing"

func TestLoadKnowledgeRetrievalConfigFromEnv_Defaults(t *testing.T) {
	t.Setenv(envAIProvider, "")
	t.Setenv(envAIBaseURL, "")
	t.Setenv(envAIAPIKey, "")
	t.Setenv(envKnowledgeBackend, "")
	t.Setenv(envKnowledgeQdrantEnabled, "")
	t.Setenv(envKnowledgeQdrantHost, "")
	t.Setenv(envKnowledgeQdrantPort, "")
	t.Setenv(envKnowledgeQdrantCollection, "")
	t.Setenv(envKnowledgeEmbeddingEnabled, "")
	t.Setenv(envKnowledgeEmbeddingProvider, "")
	t.Setenv(envKnowledgeEmbeddingModel, "")
	t.Setenv(envKnowledgeEmbeddingBaseURL, "")
	t.Setenv(envKnowledgeEmbeddingAPIKey, "")
	t.Setenv(envKnowledgeEmbeddingDim, "")
	t.Setenv(envKnowledgeTopK, "")
	t.Setenv(envKnowledgeMinScore, "")
	t.Setenv(envKnowledgeSyncBatch, "")

	cfg, err := LoadKnowledgeRetrievalConfigFromEnv()
	if err != nil {
		t.Fatalf("load config failed: %v", err)
	}
	if cfg.NormalizedBackend() != KnowledgeBackendAuto {
		t.Fatalf("backend mismatch: got=%s", cfg.NormalizedBackend())
	}
	if cfg.Qdrant.Host != defaultQdrantHost {
		t.Fatalf("qdrant host mismatch: got=%s", cfg.Qdrant.Host)
	}
	if cfg.Qdrant.Port != defaultQdrantPort {
		t.Fatalf("qdrant port mismatch: got=%d", cfg.Qdrant.Port)
	}
	if cfg.Qdrant.Collection != defaultQdrantCollection {
		t.Fatalf("qdrant collection mismatch: got=%s", cfg.Qdrant.Collection)
	}
	if cfg.TopK != defaultKnowledgeTopK {
		t.Fatalf("topk mismatch: got=%d", cfg.TopK)
	}
	if cfg.SyncBatchSize != defaultSyncBatchSize {
		t.Fatalf("sync batch mismatch: got=%d", cfg.SyncBatchSize)
	}
	if cfg.VectorRetrievalEnabled() {
		t.Fatalf("vector retrieval should be disabled by default without embedding api key")
	}
}

func TestLoadKnowledgeRetrievalConfigFromEnv_InvalidBackend(t *testing.T) {
	t.Setenv(envKnowledgeBackend, "not-supported")
	_, err := LoadKnowledgeRetrievalConfigFromEnv()
	if err == nil {
		t.Fatalf("expected invalid backend error")
	}
}

func TestLoadKnowledgeRetrievalConfigFromEnv_QdrantRequiresEmbedding(t *testing.T) {
	t.Setenv(envAIProvider, "")
	t.Setenv(envAIBaseURL, "")
	t.Setenv(envAIAPIKey, "")
	t.Setenv(envKnowledgeBackend, KnowledgeBackendQdrant)
	t.Setenv(envKnowledgeQdrantEnabled, "true")
	t.Setenv(envKnowledgeEmbeddingEnabled, "true")
	t.Setenv(envKnowledgeEmbeddingProvider, "openai_compatible")
	t.Setenv(envKnowledgeEmbeddingModel, "text-embedding-v4")
	t.Setenv(envKnowledgeEmbeddingBaseURL, "https://example.com/v1")
	t.Setenv(envKnowledgeEmbeddingAPIKey, "")
	t.Setenv(envKnowledgeEmbeddingDim, "1024")

	_, err := LoadKnowledgeRetrievalConfigFromEnv()
	if err == nil {
		t.Fatalf("expected qdrant backend validation error when embedding is incomplete")
	}
}

func TestLoadKnowledgeRetrievalConfigFromEnv_CompleteVectorConfig(t *testing.T) {
	t.Setenv(envAIProvider, "")
	t.Setenv(envAIBaseURL, "")
	t.Setenv(envAIAPIKey, "")
	t.Setenv(envKnowledgeBackend, KnowledgeBackendAuto)
	t.Setenv(envKnowledgeQdrantEnabled, "true")
	t.Setenv(envKnowledgeQdrantHost, "127.0.0.1")
	t.Setenv(envKnowledgeQdrantPort, "6334")
	t.Setenv(envKnowledgeQdrantCollection, "knowledge_chunks")
	t.Setenv(envKnowledgeEmbeddingEnabled, "true")
	t.Setenv(envKnowledgeEmbeddingProvider, "openai_compatible")
	t.Setenv(envKnowledgeEmbeddingModel, "text-embedding-v4")
	t.Setenv(envKnowledgeEmbeddingBaseURL, "https://example.com/v1")
	t.Setenv(envKnowledgeEmbeddingAPIKey, "sk-test-key")
	t.Setenv(envKnowledgeEmbeddingDim, "1024")

	cfg, err := LoadKnowledgeRetrievalConfigFromEnv()
	if err != nil {
		t.Fatalf("load config failed: %v", err)
	}
	if !cfg.VectorRetrievalEnabled() {
		t.Fatalf("vector retrieval should be enabled")
	}
}

func TestLoadKnowledgeRetrievalConfigFromEnv_InheritAliDefaults(t *testing.T) {
	t.Setenv(envAIProvider, "qwen")
	t.Setenv(envAIBaseURL, "https://dashscope.aliyuncs.com/compatible-mode/v1")
	t.Setenv(envAIAPIKey, "ali-key-123")

	t.Setenv(envKnowledgeEmbeddingModel, "")
	t.Setenv(envKnowledgeEmbeddingBaseURL, "")
	t.Setenv(envKnowledgeEmbeddingAPIKey, "")
	t.Setenv(envKnowledgeEmbeddingEnabled, "true")
	t.Setenv(envKnowledgeEmbeddingDim, "1024")

	cfg, err := LoadKnowledgeRetrievalConfigFromEnv()
	if err != nil {
		t.Fatalf("load config failed: %v", err)
	}
	if cfg.Embedding.Model != defaultQwenEmbeddingModel {
		t.Fatalf("embedding model mismatch: got=%s want=%s", cfg.Embedding.Model, defaultQwenEmbeddingModel)
	}
	if cfg.Embedding.BaseURL != "https://dashscope.aliyuncs.com/compatible-mode/v1" {
		t.Fatalf("embedding base url mismatch: got=%s", cfg.Embedding.BaseURL)
	}
	if cfg.Embedding.APIKey != "ali-key-123" {
		t.Fatalf("embedding key should inherit AI_API_KEY")
	}
	if cfg.SyncBatchSize != defaultAliSyncBatchSize {
		t.Fatalf("aliyun sync batch should default to %d, got=%d", defaultAliSyncBatchSize, cfg.SyncBatchSize)
	}
}
