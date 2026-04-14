package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOpenAICompatibleEmbeddingProvider_EmbedQuery(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/embeddings" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		if got := r.Header.Get("Authorization"); got == "" {
			http.Error(w, "missing auth", http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"index": 0, "embedding": []float64{0.1, 0.2, 0.3}},
			},
		})
	}))
	defer server.Close()

	provider, err := NewOpenAICompatibleEmbeddingProvider(EmbeddingConfig{
		Enabled: true,
		Model:   "test-embed",
		BaseURL: server.URL,
		APIKey:  "sk-test",
		Dim:     3,
	})
	if err != nil {
		t.Fatalf("init provider failed: %v", err)
	}

	vector, err := provider.EmbedQuery(context.Background(), "gpu oom")
	if err != nil {
		t.Fatalf("EmbedQuery failed: %v", err)
	}
	if len(vector) != 3 {
		t.Fatalf("vector dim mismatch: got=%d", len(vector))
	}
}

func TestOpenAICompatibleEmbeddingProvider_EmbedDocumentsBatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"index": 0, "embedding": []float64{0.1, 0.2, 0.3}},
				{"index": 1, "embedding": []float64{0.4, 0.5, 0.6}},
			},
		})
	}))
	defer server.Close()

	provider, err := NewOpenAICompatibleEmbeddingProvider(EmbeddingConfig{
		Enabled: true,
		Model:   "test-embed",
		BaseURL: server.URL,
		APIKey:  "sk-test",
		Dim:     3,
	})
	if err != nil {
		t.Fatalf("init provider failed: %v", err)
	}

	vectors, err := provider.EmbedDocuments(context.Background(), []string{"doc-1", "doc-2"})
	if err != nil {
		t.Fatalf("EmbedDocuments failed: %v", err)
	}
	if len(vectors) != 2 {
		t.Fatalf("vector count mismatch: got=%d", len(vectors))
	}
}

func TestOpenAICompatibleEmbeddingProvider_EmptyInput(t *testing.T) {
	provider, err := NewOpenAICompatibleEmbeddingProvider(EmbeddingConfig{
		Enabled: true,
		Model:   "test-embed",
		BaseURL: "https://example.com/v1",
		APIKey:  "sk-test",
		Dim:     3,
	})
	if err != nil {
		t.Fatalf("init provider failed: %v", err)
	}

	if _, err := provider.EmbedQuery(context.Background(), "   "); err == nil {
		t.Fatalf("expected error for empty query")
	}
	if _, err := provider.EmbedDocuments(context.Background(), []string{"doc-1", "  "}); err == nil {
		t.Fatalf("expected error for empty document in batch")
	}
}

func TestOpenAICompatibleEmbeddingProvider_APIErrorWrapped(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{
				"message": "bad embedding request",
			},
		})
	}))
	defer server.Close()

	provider, err := NewOpenAICompatibleEmbeddingProvider(EmbeddingConfig{
		Enabled: true,
		Model:   "test-embed",
		BaseURL: server.URL,
		APIKey:  "sk-test",
		Dim:     3,
	})
	if err != nil {
		t.Fatalf("init provider failed: %v", err)
	}

	_, err = provider.EmbedQuery(context.Background(), "gpu oom")
	if err == nil {
		t.Fatalf("expected api error")
	}
	if !strings.Contains(err.Error(), "status=400") {
		t.Fatalf("error should include status code, got=%v", err)
	}
}

func TestOpenAICompatibleEmbeddingProvider_DimensionMismatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"index": 0, "embedding": []float64{0.1, 0.2}},
			},
		})
	}))
	defer server.Close()

	provider, err := NewOpenAICompatibleEmbeddingProvider(EmbeddingConfig{
		Enabled: true,
		Model:   "test-embed",
		BaseURL: server.URL,
		APIKey:  "sk-test",
		Dim:     3,
	})
	if err != nil {
		t.Fatalf("init provider failed: %v", err)
	}

	_, err = provider.EmbedQuery(context.Background(), "gpu oom")
	if err == nil {
		t.Fatalf("expected dimension mismatch error")
	}
	if !strings.Contains(err.Error(), "dim mismatch") {
		t.Fatalf("error should contain dim mismatch, got=%v", err)
	}
}
