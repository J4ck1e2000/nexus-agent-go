package ai

import (
	"fmt"
	"os"
	"strings"
	"time"
)

const (
	envKnowledgeBackend = "KNOWLEDGE_RETRIEVAL_BACKEND"

	envKnowledgeQdrantEnabled    = "KNOWLEDGE_QDRANT_ENABLED"
	envKnowledgeQdrantHost       = "KNOWLEDGE_QDRANT_HOST"
	envKnowledgeQdrantPort       = "KNOWLEDGE_QDRANT_PORT"
	envKnowledgeQdrantAPIKey     = "KNOWLEDGE_QDRANT_API_KEY"
	envKnowledgeQdrantUseTLS     = "KNOWLEDGE_QDRANT_USE_TLS"
	envKnowledgeQdrantCollection = "KNOWLEDGE_QDRANT_COLLECTION"
	envKnowledgeQdrantTimeoutSec = "KNOWLEDGE_QDRANT_TIMEOUT_SEC"

	envKnowledgeEmbeddingEnabled  = "KNOWLEDGE_EMBEDDING_ENABLED"
	envKnowledgeEmbeddingProvider = "KNOWLEDGE_EMBEDDING_PROVIDER"
	envKnowledgeEmbeddingModel    = "KNOWLEDGE_EMBEDDING_MODEL"
	envKnowledgeEmbeddingBaseURL  = "KNOWLEDGE_EMBEDDING_BASE_URL"
	envKnowledgeEmbeddingAPIKey   = "KNOWLEDGE_EMBEDDING_API_KEY"
	envKnowledgeEmbeddingDim      = "KNOWLEDGE_EMBEDDING_DIM"

	envKnowledgeTopK          = "KNOWLEDGE_TOPK"
	envKnowledgeMinScore      = "KNOWLEDGE_MIN_SCORE"
	envKnowledgeSyncBatch     = "KNOWLEDGE_SYNC_BATCH_SIZE"
	defaultKnowledgeBackend   = KnowledgeBackendAuto
	defaultQdrantHost         = "127.0.0.1"
	defaultQdrantPort         = 6333
	defaultQdrantCollection   = "knowledge_chunks"
	defaultQdrantTimeoutSec   = 5
	defaultEmbeddingProvider  = "openai_compatible"
	defaultEmbeddingModel     = "text-embedding-v4"
	defaultQwenEmbeddingModel = "text-embedding-v3"
	defaultEmbeddingDim       = 1024
	defaultVectorMinScore     = 0.35
	defaultSyncBatchSize      = 32
	defaultAliSyncBatchSize   = 10
)

// QdrantConfig contains runtime configuration for the qdrant knowledge backend.
type QdrantConfig struct {
	Enabled    bool
	Host       string
	Port       int
	APIKey     string
	UseTLS     bool
	Collection string
	TimeoutSec int
}

// BaseURL returns qdrant endpoint URL based on host/port/tls config.
func (c QdrantConfig) BaseURL() string {
	scheme := "http"
	if c.UseTLS {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s:%d", scheme, strings.TrimSpace(c.Host), c.Port)
}

// Timeout returns configured request timeout.
func (c QdrantConfig) Timeout() time.Duration {
	sec := c.TimeoutSec
	if sec <= 0 {
		sec = defaultQdrantTimeoutSec
	}
	return time.Duration(sec) * time.Second
}

// EmbeddingConfig contains settings for vector embedding provider.
type EmbeddingConfig struct {
	Enabled  bool
	Provider string
	Model    string
	BaseURL  string
	APIKey   string
	Dim      int
}

// IsComplete indicates embedding config is sufficiently configured.
func (c EmbeddingConfig) IsComplete() bool {
	if !c.Enabled {
		return false
	}
	if strings.TrimSpace(c.Provider) == "" {
		return false
	}
	if strings.TrimSpace(c.Model) == "" {
		return false
	}
	if strings.TrimSpace(c.BaseURL) == "" {
		return false
	}
	if strings.TrimSpace(c.APIKey) == "" {
		return false
	}
	return c.Dim > 0
}

// KnowledgeRetrievalConfig centralizes retrieval backend and qdrant/embedding settings.
type KnowledgeRetrievalConfig struct {
	Backend       string
	TopK          int
	MinScore      float64
	SyncBatchSize int
	Qdrant        QdrantConfig
	Embedding     EmbeddingConfig
}

// VectorRetrievalEnabled indicates qdrant vector retrieval can be used.
func (c KnowledgeRetrievalConfig) VectorRetrievalEnabled() bool {
	return c.Qdrant.Enabled && c.Embedding.IsComplete()
}

// NormalizedBackend returns normalized backend name.
func (c KnowledgeRetrievalConfig) NormalizedBackend() string {
	return normalizeKnowledgeBackend(c.Backend)
}

// Validate checks retrieval config consistency and required fields.
func (c KnowledgeRetrievalConfig) Validate() error {
	rawBackend := strings.ToLower(strings.TrimSpace(c.Backend))
	if rawBackend == "" {
		rawBackend = KnowledgeBackendAuto
	}
	switch rawBackend {
	case KnowledgeBackendAuto, KnowledgeBackendLocal, KnowledgeBackendQdrant:
	default:
		return fmt.Errorf("invalid knowledge backend: %q", c.Backend)
	}
	backend := rawBackend

	if c.TopK <= 0 {
		return fmt.Errorf("knowledge topk must be positive")
	}
	if c.MinScore < 0 {
		return fmt.Errorf("knowledge min score cannot be negative")
	}
	if c.SyncBatchSize <= 0 {
		return fmt.Errorf("knowledge sync batch size must be positive")
	}
	if backend != KnowledgeBackendLocal {
		if strings.TrimSpace(c.Qdrant.Host) == "" {
			return fmt.Errorf("qdrant host is empty")
		}
		if c.Qdrant.Port <= 0 || c.Qdrant.Port > 65535 {
			return fmt.Errorf("qdrant port is invalid: %d", c.Qdrant.Port)
		}
		if strings.TrimSpace(c.Qdrant.Collection) == "" {
			return fmt.Errorf("qdrant collection is empty")
		}
		if c.Qdrant.TimeoutSec <= 0 {
			return fmt.Errorf("qdrant timeout sec must be positive")
		}
	}
	if c.Embedding.Enabled {
		if c.Embedding.Dim <= 0 {
			return fmt.Errorf("embedding dim must be positive")
		}
	}
	if backend == KnowledgeBackendQdrant && !c.VectorRetrievalEnabled() {
		return fmt.Errorf("backend=qdrant requires complete embedding config and enabled qdrant")
	}
	return nil
}

// SafeSummary returns log-friendly retrieval config summary with key masking.
func (c KnowledgeRetrievalConfig) SafeSummary() string {
	return fmt.Sprintf(
		"backend=%s topk=%d min_score=%.3f sync_batch=%d qdrant_enabled=%t qdrant_endpoint=%s collection=%s embedding_enabled=%t embedding_provider=%s embedding_model=%s embedding_dim=%d embedding_base_url=%s embedding_api_key=%s qdrant_api_key=%s vector_ready=%t",
		c.NormalizedBackend(),
		c.TopK,
		c.MinScore,
		c.SyncBatchSize,
		c.Qdrant.Enabled,
		c.Qdrant.BaseURL(),
		c.Qdrant.Collection,
		c.Embedding.Enabled,
		c.Embedding.Provider,
		c.Embedding.Model,
		c.Embedding.Dim,
		c.Embedding.BaseURL,
		maskSecret(c.Embedding.APIKey),
		maskSecret(c.Qdrant.APIKey),
		c.VectorRetrievalEnabled(),
	)
}

// LoadKnowledgeRetrievalConfigFromEnv parses retrieval backend config from environment.
func LoadKnowledgeRetrievalConfigFromEnv() (KnowledgeRetrievalConfig, error) {
	aiProvider := strings.ToLower(strings.TrimSpace(os.Getenv(envAIProvider)))
	aiBaseURL := strings.TrimSpace(os.Getenv(envAIBaseURL))
	aiAPIKey := strings.TrimSpace(os.Getenv(envAIAPIKey))
	isAliProvider := aiProvider == "qwen" || aiProvider == "dashscope" || aiProvider == "aliyun"

	defaultEmbeddingBaseURL := defaultOpenAIBaseURL
	defaultEmbeddingModelByProvider := defaultEmbeddingModel
	if isAliProvider {
		if aiBaseURL != "" {
			defaultEmbeddingBaseURL = aiBaseURL
		} else {
			defaultEmbeddingBaseURL = defaultQwenBaseURL
		}
		defaultEmbeddingModelByProvider = defaultQwenEmbeddingModel
	}

	cfg := KnowledgeRetrievalConfig{
		Backend: strings.TrimSpace(os.Getenv(envKnowledgeBackend)),
		TopK:    envInt(envKnowledgeTopK, defaultKnowledgeTopK),
		MinScore: envFloat(
			envKnowledgeMinScore,
			defaultVectorMinScore,
		),
		SyncBatchSize: envInt(envKnowledgeSyncBatch, syncBatchDefaultByProvider(isAliProvider)),
		Qdrant: QdrantConfig{
			Enabled:    envBool(envKnowledgeQdrantEnabled, true),
			Host:       strings.TrimSpace(os.Getenv(envKnowledgeQdrantHost)),
			Port:       envInt(envKnowledgeQdrantPort, defaultQdrantPort),
			APIKey:     strings.TrimSpace(os.Getenv(envKnowledgeQdrantAPIKey)),
			UseTLS:     envBool(envKnowledgeQdrantUseTLS, false),
			Collection: strings.TrimSpace(os.Getenv(envKnowledgeQdrantCollection)),
			TimeoutSec: envInt(envKnowledgeQdrantTimeoutSec, defaultQdrantTimeoutSec),
		},
		Embedding: EmbeddingConfig{
			Enabled:  envBool(envKnowledgeEmbeddingEnabled, true),
			Provider: strings.TrimSpace(os.Getenv(envKnowledgeEmbeddingProvider)),
			Model:    strings.TrimSpace(os.Getenv(envKnowledgeEmbeddingModel)),
			BaseURL:  strings.TrimSpace(os.Getenv(envKnowledgeEmbeddingBaseURL)),
			APIKey:   strings.TrimSpace(os.Getenv(envKnowledgeEmbeddingAPIKey)),
			Dim:      envInt(envKnowledgeEmbeddingDim, defaultEmbeddingDim),
		},
	}

	if strings.TrimSpace(cfg.Backend) == "" {
		cfg.Backend = defaultKnowledgeBackend
	}
	if cfg.TopK <= 0 {
		cfg.TopK = defaultKnowledgeTopK
	}
	if cfg.MinScore < 0 {
		cfg.MinScore = defaultVectorMinScore
	}
	if cfg.SyncBatchSize <= 0 {
		cfg.SyncBatchSize = syncBatchDefaultByProvider(isAliProvider)
	}

	if strings.TrimSpace(cfg.Qdrant.Host) == "" {
		cfg.Qdrant.Host = defaultQdrantHost
	}
	if cfg.Qdrant.Port <= 0 {
		cfg.Qdrant.Port = defaultQdrantPort
	}
	if strings.TrimSpace(cfg.Qdrant.Collection) == "" {
		cfg.Qdrant.Collection = defaultQdrantCollection
	}
	if cfg.Qdrant.TimeoutSec <= 0 {
		cfg.Qdrant.TimeoutSec = defaultQdrantTimeoutSec
	}

	if strings.TrimSpace(cfg.Embedding.Provider) == "" {
		cfg.Embedding.Provider = defaultEmbeddingProvider
	}
	if strings.TrimSpace(cfg.Embedding.Model) == "" {
		cfg.Embedding.Model = defaultEmbeddingModelByProvider
	}
	if strings.TrimSpace(cfg.Embedding.BaseURL) == "" {
		cfg.Embedding.BaseURL = defaultEmbeddingBaseURL
	}
	if strings.TrimSpace(cfg.Embedding.APIKey) == "" && aiAPIKey != "" {
		cfg.Embedding.APIKey = aiAPIKey
	}
	if cfg.Embedding.Dim <= 0 {
		cfg.Embedding.Dim = defaultEmbeddingDim
	}

	// Incomplete embedding config cannot enable vector retrieval. In auto mode this will fallback to local.
	if cfg.Embedding.Enabled && (!cfg.Qdrant.Enabled || !cfg.Embedding.IsComplete()) {
		cfg.Embedding.Enabled = false
	}

	return cfg, cfg.Validate()
}

func maskSecret(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	if len(trimmed) <= 6 {
		return "***"
	}
	return trimmed[:3] + "***" + trimmed[len(trimmed)-2:]
}

func syncBatchDefaultByProvider(isAliProvider bool) int {
	if isAliProvider {
		return defaultAliSyncBatchSize
	}
	return defaultSyncBatchSize
}
