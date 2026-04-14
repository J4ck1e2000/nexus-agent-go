package ai

import "testing"

func TestBatchSlice(t *testing.T) {
	input := []int{1, 2, 3, 4, 5}
	batches := BatchSlice(input, 2)
	if len(batches) != 3 {
		t.Fatalf("batch count mismatch: got=%d want=%d", len(batches), 3)
	}
	if len(batches[0]) != 2 || len(batches[1]) != 2 || len(batches[2]) != 1 {
		t.Fatalf("unexpected batch sizes: %+v", batches)
	}
	if batches[2][0] != 5 {
		t.Fatalf("last batch value mismatch: got=%d want=%d", batches[2][0], 5)
	}
}

func TestEmbeddingTextForChunk(t *testing.T) {
	text := embeddingTextForChunk(KnowledgeChunk{
		Title:       "GPU OOM",
		HeadingPath: "GPU OOM / Symptoms",
		Content:     "CUDA out of memory",
	})
	if text == "" {
		t.Fatalf("embedding text should not be empty")
	}
	if text != "GPU OOM\nGPU OOM / Symptoms\nCUDA out of memory" {
		t.Fatalf("unexpected embedding text: %q", text)
	}
}
