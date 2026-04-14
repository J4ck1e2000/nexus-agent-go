package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

type openAIEmbeddingRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

type openAIEmbeddingError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    any    `json:"code"`
}

type openAIEmbeddingItem struct {
	Object    string    `json:"object"`
	Embedding []float64 `json:"embedding"`
	Index     int       `json:"index"`
}

type openAIEmbeddingResponse struct {
	Object string                `json:"object"`
	Model  string                `json:"model"`
	Data   []openAIEmbeddingItem `json:"data"`
	Error  *openAIEmbeddingError `json:"error,omitempty"`
}

// OpenAICompatibleEmbeddingProvider is an OpenAI-compatible embedding client.
type OpenAICompatibleEmbeddingProvider struct {
	name       string
	model      string
	dim        int
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// NewOpenAICompatibleEmbeddingProvider creates an embedding provider from config.
func NewOpenAICompatibleEmbeddingProvider(cfg EmbeddingConfig) (*OpenAICompatibleEmbeddingProvider, error) {
	model := strings.TrimSpace(cfg.Model)
	if model == "" {
		return nil, fmt.Errorf("embedding model is empty")
	}
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if baseURL == "" {
		return nil, fmt.Errorf("embedding base url is empty")
	}
	apiKey := strings.TrimSpace(cfg.APIKey)
	if apiKey == "" {
		return nil, fmt.Errorf("embedding api key is empty")
	}
	if cfg.Dim <= 0 {
		return nil, fmt.Errorf("embedding dim must be positive")
	}

	return &OpenAICompatibleEmbeddingProvider{
		name:    defaultEmbeddingProvider,
		model:   model,
		dim:     cfg.Dim,
		baseURL: baseURL,
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: defaultOpenAICompatibleTimeout,
		},
	}, nil
}

// ProviderName returns provider name.
func (p *OpenAICompatibleEmbeddingProvider) ProviderName() string {
	if p == nil {
		return defaultEmbeddingProvider
	}
	return p.name
}

// EmbedQuery embeds one query text.
func (p *OpenAICompatibleEmbeddingProvider) EmbedQuery(ctx context.Context, text string) ([]float32, error) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return nil, fmt.Errorf("embed query: input text is empty")
	}
	vectors, err := p.embed(ctx, []string{trimmed})
	if err != nil {
		return nil, fmt.Errorf("embed query failed: %w", err)
	}
	if len(vectors) != 1 {
		return nil, fmt.Errorf("embed query failed: expected 1 vector, got %d", len(vectors))
	}
	return vectors[0], nil
}

// EmbedDocuments embeds a batch of documents.
func (p *OpenAICompatibleEmbeddingProvider) EmbedDocuments(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, fmt.Errorf("embed documents: empty input")
	}
	inputs := make([]string, 0, len(texts))
	for idx, text := range texts {
		trimmed := strings.TrimSpace(text)
		if trimmed == "" {
			return nil, fmt.Errorf("embed documents: empty text at index %d", idx)
		}
		inputs = append(inputs, trimmed)
	}
	vectors, err := p.embed(ctx, inputs)
	if err != nil {
		return nil, fmt.Errorf("embed documents failed: %w", err)
	}
	if len(vectors) != len(inputs) {
		return nil, fmt.Errorf("embed documents failed: vector count mismatch got=%d want=%d", len(vectors), len(inputs))
	}
	return vectors, nil
}

func (p *OpenAICompatibleEmbeddingProvider) embed(ctx context.Context, texts []string) ([][]float32, error) {
	if p == nil {
		return nil, fmt.Errorf("embedding provider is nil")
	}
	if len(texts) == 0 {
		return nil, fmt.Errorf("embedding input is empty")
	}

	payload := openAIEmbeddingRequest{
		Model: p.model,
		Input: texts,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal embedding request failed: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/embeddings", bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("create embedding request failed: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.apiKey)

	client := p.httpClient
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request embedding api failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024))
	if err != nil {
		return nil, fmt.Errorf("read embedding response failed: %w", err)
	}

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("embedding api status=%d body=%s", resp.StatusCode, compactErrorBody(body))
	}

	var decoded openAIEmbeddingResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		return nil, fmt.Errorf("decode embedding response failed: %w", err)
	}
	if decoded.Error != nil {
		return nil, fmt.Errorf("embedding api error: %s", strings.TrimSpace(decoded.Error.Message))
	}
	if len(decoded.Data) == 0 {
		return nil, fmt.Errorf("embedding response has no data")
	}

	sort.SliceStable(decoded.Data, func(i, j int) bool {
		return decoded.Data[i].Index < decoded.Data[j].Index
	})

	vectors := make([][]float32, 0, len(decoded.Data))
	for idx, item := range decoded.Data {
		vector := make([]float32, 0, len(item.Embedding))
		for _, value := range item.Embedding {
			vector = append(vector, float32(value))
		}
		if p.dim > 0 && len(vector) != p.dim {
			return nil, fmt.Errorf("embedding dim mismatch at index=%d got=%d want=%d", idx, len(vector), p.dim)
		}
		vectors = append(vectors, vector)
	}
	return vectors, nil
}

func compactErrorBody(body []byte) string {
	text := strings.TrimSpace(string(body))
	if text == "" {
		return ""
	}
	if len(text) > 320 {
		return text[:320] + "..."
	}
	return text
}
