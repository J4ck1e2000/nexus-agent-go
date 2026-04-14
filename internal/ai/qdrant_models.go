package ai

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"hash/fnv"
	"strings"
	"time"
)

type qdrantPoint struct {
	ID      any            `json:"id"`
	Vector  []float32      `json:"vector"`
	Payload map[string]any `json:"payload,omitempty"`
}

type qdrantSearchResult struct {
	ID      any            `json:"id"`
	Score   float64        `json:"score"`
	Payload map[string]any `json:"payload,omitempty"`
}

type qdrantSearchResponse struct {
	Status string               `json:"status"`
	Time   float64              `json:"time"`
	Result []qdrantSearchResult `json:"result"`
}

type qdrantOperationResponse struct {
	Status string  `json:"status"`
	Time   float64 `json:"time"`
}

type qdrantCollectionVectorsConfig struct {
	Size     int    `json:"size"`
	Distance string `json:"distance"`
}

type qdrantCreateCollectionRequest struct {
	Vectors qdrantCollectionVectorsConfig `json:"vectors"`
}

// BuildQdrantPoint converts one chunk + embedding vector into a qdrant point.
func BuildQdrantPoint(chunk KnowledgeChunk, vector []float32) (qdrantPoint, error) {
	if strings.TrimSpace(chunk.ID) == "" {
		return qdrantPoint{}, fmt.Errorf("chunk id is empty")
	}
	if len(vector) == 0 {
		return qdrantPoint{}, fmt.Errorf("vector is empty")
	}
	return qdrantPoint{
		ID:      BuildQdrantPointID(chunk),
		Vector:  append([]float32(nil), vector...),
		Payload: BuildPayload(chunk),
	}, nil
}

// BuildQdrantPointID creates deterministic point id from chunk id.
func BuildQdrantPointID(chunk KnowledgeChunk) uint64 {
	hasher := fnv.New64a()
	_, _ = hasher.Write([]byte(strings.TrimSpace(chunk.ID)))
	id := hasher.Sum64()
	if id == 0 {
		return 1
	}
	return id
}

// BuildPayload builds qdrant payload fields from one chunk.
func BuildPayload(chunk KnowledgeChunk) map[string]any {
	content := strings.TrimSpace(chunk.Content)
	payload := map[string]any{
		"chunk_id":        strings.TrimSpace(chunk.ID),
		"document_id":     strings.TrimSpace(chunk.DocumentID),
		"title":           strings.TrimSpace(chunk.Title),
		"category":        strings.TrimSpace(chunk.Category),
		"tags":            append([]string(nil), chunk.Tags...),
		"content":         content,
		"heading_path":    strings.TrimSpace(chunk.HeadingPath),
		"source_path":     strings.TrimSpace(chunk.SourcePath),
		"content_hash":    buildContentHash(content),
		"updated_at":      time.Now().UTC().Format(time.RFC3339),
		"content_preview": compactSnippet(content, 140),
	}
	return payload
}

// PayloadToKnowledgeHit converts qdrant payload back into stable KnowledgeHit.
func PayloadToKnowledgeHit(payload map[string]any, score float64, query string, maxSnippetChars int) KnowledgeHit {
	content := strings.TrimSpace(asPayloadString(payload, "content"))
	snippet := compactSnippet(content, maxSnippetChars)
	if snippet == "" {
		snippet = strings.TrimSpace(asPayloadString(payload, "content_preview"))
	}
	if snippet == "" {
		snippet = compactSnippet(content, defaultKnowledgeMaxSnippetLen)
	}
	return KnowledgeHit{
		ChunkID:    strings.TrimSpace(asPayloadString(payload, "chunk_id")),
		DocumentID: strings.TrimSpace(asPayloadString(payload, "document_id")),
		Title:      strings.TrimSpace(asPayloadString(payload, "title")),
		Category:   strings.TrimSpace(asPayloadString(payload, "category")),
		Score:      score,
		Snippet:    snippet,
		SourcePath: strings.TrimSpace(asPayloadString(payload, "source_path")),
	}
}

// BuildKnowledgeCollectionConfig builds qdrant collection vectors config.
func BuildKnowledgeCollectionConfig(dim int) qdrantCreateCollectionRequest {
	if dim <= 0 {
		dim = defaultEmbeddingDim
	}
	return qdrantCreateCollectionRequest{
		Vectors: qdrantCollectionVectorsConfig{
			Size:     dim,
			Distance: "Cosine",
		},
	}
}

func buildContentHash(content string) string {
	raw := strings.TrimSpace(content)
	if raw == "" {
		return ""
	}
	sum := sha1.Sum([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func asPayloadString(payload map[string]any, key string) string {
	if len(payload) == 0 {
		return ""
	}
	value, ok := payload[key]
	if !ok {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return typed
	case []byte:
		return string(typed)
	default:
		return fmt.Sprintf("%v", typed)
	}
}
