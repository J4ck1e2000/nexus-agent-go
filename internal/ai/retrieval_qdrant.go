package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var (
	// ErrQdrantCollectionNotFound indicates collection does not exist.
	ErrQdrantCollectionNotFound = errors.New("qdrant collection not found")
)

type qdrantHTTPClient struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

type qdrantCollectionListResponse struct {
	Result struct {
		Collections []struct {
			Name string `json:"name"`
		} `json:"collections"`
	} `json:"result"`
}

type qdrantCollectionInfoResponse struct {
	Result struct {
		Config struct {
			Params struct {
				Vectors any `json:"vectors"`
			} `json:"params"`
		} `json:"config"`
	} `json:"result"`
}

type qdrantUpsertRequest struct {
	Points []qdrantPoint `json:"points"`
}

type qdrantSearchRequest struct {
	Vector      []float32 `json:"vector"`
	Limit       int       `json:"limit"`
	WithPayload bool      `json:"with_payload"`
	WithVectors bool      `json:"with_vector"`
}

// NewQdrantHTTPClient creates a minimal qdrant HTTP client.
func NewQdrantHTTPClient(cfg QdrantConfig) *qdrantHTTPClient {
	return &qdrantHTTPClient{
		baseURL: strings.TrimRight(cfg.BaseURL(), "/"),
		apiKey:  strings.TrimSpace(cfg.APIKey),
		httpClient: &http.Client{
			Timeout: cfg.Timeout(),
		},
	}
}

// Health checks qdrant service reachability.
func (c *qdrantHTTPClient) Health(ctx context.Context) error {
	if c == nil {
		return fmt.Errorf("qdrant client is nil")
	}
	var resp qdrantCollectionListResponse
	if err := c.doJSON(ctx, http.MethodGet, "/collections", nil, &resp, http.StatusOK); err != nil {
		return fmt.Errorf("qdrant health check failed: %w", err)
	}
	return nil
}

// DeleteCollection deletes one collection if exists.
func (c *qdrantHTTPClient) DeleteCollection(ctx context.Context, collection string) error {
	name := strings.TrimSpace(collection)
	if name == "" {
		return fmt.Errorf("collection is empty")
	}
	err := c.doJSON(ctx, http.MethodDelete, "/collections/"+url.PathEscape(name), nil, nil, http.StatusOK)
	if err != nil && errors.Is(err, ErrQdrantCollectionNotFound) {
		return nil
	}
	return err
}

// CreateCollection creates collection with cosine vector configuration.
func (c *qdrantHTTPClient) CreateCollection(ctx context.Context, collection string, dim int) error {
	name := strings.TrimSpace(collection)
	if name == "" {
		return fmt.Errorf("collection is empty")
	}
	request := BuildKnowledgeCollectionConfig(dim)
	return c.doJSON(ctx, http.MethodPut, "/collections/"+url.PathEscape(name), request, nil, http.StatusOK)
}

// GetCollectionVectorConfig returns configured vector size and distance.
func (c *qdrantHTTPClient) GetCollectionVectorConfig(ctx context.Context, collection string) (int, string, error) {
	name := strings.TrimSpace(collection)
	if name == "" {
		return 0, "", fmt.Errorf("collection is empty")
	}
	var resp qdrantCollectionInfoResponse
	err := c.doJSON(ctx, http.MethodGet, "/collections/"+url.PathEscape(name), nil, &resp, http.StatusOK)
	if err != nil {
		return 0, "", err
	}
	return parseQdrantVectorsConfig(resp.Result.Config.Params.Vectors)
}

// UpsertPoints upserts points into collection.
func (c *qdrantHTTPClient) UpsertPoints(ctx context.Context, collection string, points []qdrantPoint) error {
	if len(points) == 0 {
		return nil
	}
	path := fmt.Sprintf("/collections/%s/points?wait=true", url.PathEscape(strings.TrimSpace(collection)))
	request := qdrantUpsertRequest{Points: points}
	return c.doJSON(ctx, http.MethodPut, path, request, nil, http.StatusOK)
}

// SearchPoints runs vector similarity search in one collection.
func (c *qdrantHTTPClient) SearchPoints(ctx context.Context, collection string, vector []float32, limit int) ([]qdrantSearchResult, error) {
	if len(vector) == 0 {
		return nil, fmt.Errorf("search vector is empty")
	}
	if limit <= 0 {
		limit = defaultKnowledgeTopK
	}
	path := fmt.Sprintf("/collections/%s/points/search", url.PathEscape(strings.TrimSpace(collection)))
	request := qdrantSearchRequest{
		Vector:      vector,
		Limit:       limit,
		WithPayload: true,
		WithVectors: false,
	}

	var resp qdrantSearchResponse
	if err := c.doJSON(ctx, http.MethodPost, path, request, &resp, http.StatusOK); err != nil {
		return nil, err
	}
	return resp.Result, nil
}

func (c *qdrantHTTPClient) doJSON(ctx context.Context, method, path string, requestBody any, responseBody any, expectedCodes ...int) error {
	if c == nil {
		return fmt.Errorf("qdrant client is nil")
	}
	endpoint := strings.TrimRight(c.baseURL, "/") + path

	var bodyReader io.Reader
	if requestBody != nil {
		raw, err := json.Marshal(requestBody)
		if err != nil {
			return fmt.Errorf("marshal qdrant request failed: %w", err)
		}
		bodyReader = bytes.NewReader(raw)
	}

	req, err := http.NewRequestWithContext(ctx, method, endpoint, bodyReader)
	if err != nil {
		return fmt.Errorf("create qdrant request failed: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("api-key", c.apiKey)
	}

	client := c.httpClient
	if client == nil {
		client = &http.Client{Timeout: defaultOpenAICompatibleTimeout}
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("request qdrant failed: %w", err)
	}
	defer resp.Body.Close()

	rawResp, err := io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024))
	if err != nil {
		return fmt.Errorf("read qdrant response failed: %w", err)
	}

	if !containsStatus(expectedCodes, resp.StatusCode) {
		if resp.StatusCode == http.StatusNotFound {
			return fmt.Errorf("%w: %s", ErrQdrantCollectionNotFound, compactErrorBody(rawResp))
		}
		return fmt.Errorf("qdrant status=%d body=%s", resp.StatusCode, compactErrorBody(rawResp))
	}

	if responseBody != nil && len(rawResp) > 0 {
		if err := json.Unmarshal(rawResp, responseBody); err != nil {
			return fmt.Errorf("decode qdrant response failed: %w", err)
		}
	}
	return nil
}

func containsStatus(expected []int, actual int) bool {
	if len(expected) == 0 {
		return actual >= http.StatusOK && actual < http.StatusMultipleChoices
	}
	for _, code := range expected {
		if actual == code {
			return true
		}
	}
	return false
}

func parseQdrantVectorsConfig(raw any) (int, string, error) {
	switch typed := raw.(type) {
	case map[string]any:
		// unnamed vector config: {"size": 1024, "distance":"Cosine"}
		if size, distance, ok := parseSingleVectorConfig(typed); ok {
			return size, distance, nil
		}
		// named vector config: {"default": {"size": 1024, "distance":"Cosine"}}
		for _, value := range typed {
			nested, ok := value.(map[string]any)
			if !ok {
				continue
			}
			if size, distance, ok := parseSingleVectorConfig(nested); ok {
				return size, distance, nil
			}
		}
	}
	return 0, "", fmt.Errorf("unable to parse qdrant vectors config")
}

func parseSingleVectorConfig(cfg map[string]any) (int, string, bool) {
	sizeRaw, sizeOK := cfg["size"]
	distanceRaw, distanceOK := cfg["distance"]
	if !sizeOK || !distanceOK {
		return 0, "", false
	}
	size := asInt(sizeRaw, 0)
	distance := strings.TrimSpace(asString(distanceRaw))
	if size <= 0 || distance == "" {
		return 0, "", false
	}
	return size, distance, true
}

// EnsureKnowledgeCollection checks and creates collection when needed.
func EnsureKnowledgeCollection(ctx context.Context, client *qdrantHTTPClient, cfg KnowledgeRetrievalConfig) error {
	if client == nil {
		return fmt.Errorf("qdrant client is nil")
	}
	collection := strings.TrimSpace(cfg.Qdrant.Collection)
	if collection == "" {
		collection = defaultQdrantCollection
	}
	dim := cfg.Embedding.Dim
	if dim <= 0 {
		dim = defaultEmbeddingDim
	}

	size, distance, err := client.GetCollectionVectorConfig(ctx, collection)
	if err != nil {
		if !errors.Is(err, ErrQdrantCollectionNotFound) {
			return err
		}
		return client.CreateCollection(ctx, collection, dim)
	}

	if size != dim {
		return fmt.Errorf("qdrant collection dim mismatch: collection=%s got=%d want=%d", collection, size, dim)
	}
	if !strings.EqualFold(strings.TrimSpace(distance), "cosine") {
		return fmt.Errorf("qdrant collection distance mismatch: collection=%s got=%s want=Cosine", collection, distance)
	}
	return nil
}

// QdrantKnowledgeSearcher performs embedding + qdrant vector retrieval.
type QdrantKnowledgeSearcher struct {
	cfg       KnowledgeRetrievalConfig
	client    *qdrantHTTPClient
	embedding EmbeddingProvider
}

// NewQdrantKnowledgeSearcher initializes qdrant knowledge searcher.
func NewQdrantKnowledgeSearcher(ctx context.Context, cfg KnowledgeRetrievalConfig, embedding EmbeddingProvider) (*QdrantKnowledgeSearcher, error) {
	if !cfg.Qdrant.Enabled {
		return nil, fmt.Errorf("qdrant is disabled")
	}
	if embedding == nil {
		return nil, fmt.Errorf("embedding provider is nil")
	}
	client := NewQdrantHTTPClient(cfg.Qdrant)
	if err := client.Health(ctx); err != nil {
		return nil, err
	}
	if err := EnsureKnowledgeCollection(ctx, client, cfg); err != nil {
		return nil, err
	}
	return &QdrantKnowledgeSearcher{
		cfg:       cfg,
		client:    client,
		embedding: embedding,
	}, nil
}

// StrategyName returns strategy label.
func (s *QdrantKnowledgeSearcher) StrategyName() string {
	return qdrantKnowledgeStrategy
}

// Health checks qdrant backend health.
func (s *QdrantKnowledgeSearcher) Health(ctx context.Context) error {
	if s == nil || s.client == nil {
		return fmt.Errorf("qdrant searcher is nil")
	}
	if err := s.client.Health(ctx); err != nil {
		return err
	}
	return EnsureKnowledgeCollection(ctx, s.client, s.cfg)
}

// Search performs query embedding and qdrant similarity retrieval.
func (s *QdrantKnowledgeSearcher) Search(ctx context.Context, query string, topK int) ([]KnowledgeHit, RetrievalMeta, error) {
	start := time.Now()
	meta := newRetrievalMeta(query, topK, qdrantKnowledgeStrategy)
	meta.TopK = normalizeKnowledgeTopK(topK)
	query = strings.TrimSpace(query)
	meta.Query = query
	if query == "" {
		meta.DurationMs = time.Since(start).Milliseconds()
		return nil, meta, nil
	}
	if s == nil || s.client == nil || s.embedding == nil {
		meta.DurationMs = time.Since(start).Milliseconds()
		return nil, meta, fmt.Errorf("qdrant searcher is not ready")
	}

	vector, err := s.embedding.EmbedQuery(ctx, query)
	if err != nil {
		meta.DurationMs = time.Since(start).Milliseconds()
		return nil, meta, fmt.Errorf("embed query failed: %w", err)
	}
	candidates, err := s.client.SearchPoints(ctx, s.cfg.Qdrant.Collection, vector, meta.TopK)
	if err != nil {
		meta.DurationMs = time.Since(start).Milliseconds()
		return nil, meta, err
	}
	meta.CandidateCount = len(candidates)

	minScore := s.cfg.MinScore
	if minScore < 0 {
		minScore = 0
	}

	hits := make([]KnowledgeHit, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.Score < minScore {
			continue
		}
		hit := PayloadToKnowledgeHit(candidate.Payload, candidate.Score, query, defaultKnowledgeMaxSnippetLen)
		if hit.ChunkID == "" {
			hit.ChunkID = strings.TrimSpace(asString(candidate.ID))
		}
		if hit.DocumentID == "" {
			hit.DocumentID = hit.ChunkID
		}
		if hit.Title == "" {
			hit.Title = hit.DocumentID
		}
		hits = append(hits, hit)
		if len(hits) >= meta.TopK {
			break
		}
	}

	// Avoid an overly strict min-score filter that causes full empty results.
	if len(hits) == 0 && len(candidates) > 0 {
		fallback := PayloadToKnowledgeHit(candidates[0].Payload, candidates[0].Score, query, defaultKnowledgeMaxSnippetLen)
		if fallback.ChunkID == "" {
			fallback.ChunkID = strings.TrimSpace(asString(candidates[0].ID))
		}
		if fallback.DocumentID == "" {
			fallback.DocumentID = fallback.ChunkID
		}
		if fallback.Title == "" {
			fallback.Title = fallback.DocumentID
		}
		hits = append(hits, fallback)
	}

	meta.ReturnedCount = len(hits)
	meta.Hit = len(hits) > 0
	if len(hits) > 0 {
		meta.TopScore = hits[0].Score
	}
	meta.DurationMs = time.Since(start).Milliseconds()
	return hits, meta, nil
}
