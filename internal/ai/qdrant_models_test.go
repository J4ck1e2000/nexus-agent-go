package ai

import "testing"

func TestBuildQdrantPoint_StableID(t *testing.T) {
	chunk := KnowledgeChunk{
		ID:         "gpu_oom:001",
		DocumentID: "gpu_oom",
		Title:      "GPU OOM",
		Category:   "gpu_memory",
		Content:    "CUDA out of memory. Reduce batch size.",
	}
	vector := []float32{0.1, 0.2, 0.3}

	pointA, err := BuildQdrantPoint(chunk, vector)
	if err != nil {
		t.Fatalf("BuildQdrantPoint A failed: %v", err)
	}
	pointB, err := BuildQdrantPoint(chunk, vector)
	if err != nil {
		t.Fatalf("BuildQdrantPoint B failed: %v", err)
	}
	if pointA.ID != pointB.ID {
		t.Fatalf("point id should be stable: A=%v B=%v", pointA.ID, pointB.ID)
	}
	if pointA.Payload["chunk_id"] != chunk.ID {
		t.Fatalf("payload chunk_id mismatch: got=%v want=%s", pointA.Payload["chunk_id"], chunk.ID)
	}
}

func TestPayloadToKnowledgeHit(t *testing.T) {
	payload := map[string]any{
		"chunk_id":     "metrics_stale:002",
		"document_id":  "metrics_stale",
		"title":        "Metrics Stale",
		"category":     "data_freshness",
		"content":      "Collected timestamp lags polling interval and causes stale metrics interpretation.",
		"source_path":  "knowledge/anomalies/METRICS_STALE.md",
		"heading_path": "Metrics Stale / Symptoms",
	}
	hit := PayloadToKnowledgeHit(payload, 0.87, "stale", 60)
	if hit.ChunkID != "metrics_stale:002" {
		t.Fatalf("chunk id mismatch: got=%s", hit.ChunkID)
	}
	if hit.DocumentID != "metrics_stale" {
		t.Fatalf("document id mismatch: got=%s", hit.DocumentID)
	}
	if hit.Score != 0.87 {
		t.Fatalf("score mismatch: got=%.2f", hit.Score)
	}
	if hit.Snippet == "" {
		t.Fatalf("snippet should not be empty")
	}
}

func TestBuildKnowledgeCollectionConfig(t *testing.T) {
	cfg := BuildKnowledgeCollectionConfig(1024)
	if cfg.Vectors.Size != 1024 {
		t.Fatalf("vector size mismatch: got=%d", cfg.Vectors.Size)
	}
	if cfg.Vectors.Distance != "Cosine" {
		t.Fatalf("distance mismatch: got=%s", cfg.Vectors.Distance)
	}
}
