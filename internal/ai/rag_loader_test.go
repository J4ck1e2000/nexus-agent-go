package ai

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadKnowledgeBase_MissingDirectory(t *testing.T) {
	_, err := LoadKnowledgeBase(filepath.Join(t.TempDir(), "missing"))
	if err == nil {
		t.Fatalf("expected error for missing directory")
	}
	if !os.IsNotExist(err) {
		t.Fatalf("expected not-exist error, got=%v", err)
	}
}

func TestLoadKnowledgeBase_ParsesMetadataAndChunks(t *testing.T) {
	dir := t.TempDir()
	content := `# GPU OOM / 显存不足

Category: gpu_memory
Tags: gpu, oom, cuda, 显存不足

## Symptoms
- CUDA out of memory
- VRAM near 100%

## Recommended Actions
- Reduce batch size
- Check orphan processes
`
	if err := os.WriteFile(filepath.Join(dir, "GPU_OOM.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("write file failed: %v", err)
	}

	base, err := LoadKnowledgeBase(dir)
	if err != nil {
		t.Fatalf("LoadKnowledgeBase failed: %v", err)
	}
	if len(base.Documents) != 1 {
		t.Fatalf("documents mismatch: got=%d want=1", len(base.Documents))
	}
	doc := base.Documents[0]
	if doc.ID != "gpu_oom" {
		t.Fatalf("doc id mismatch: got=%q", doc.ID)
	}
	if doc.Title != "GPU OOM / 显存不足" {
		t.Fatalf("title mismatch: got=%q", doc.Title)
	}
	if doc.Category != "gpu_memory" {
		t.Fatalf("category mismatch: got=%q", doc.Category)
	}
	if len(doc.Tags) == 0 || !containsString(doc.Tags, "oom") {
		t.Fatalf("tags mismatch: %+v", doc.Tags)
	}
	if len(base.Chunks) == 0 {
		t.Fatalf("expected chunks to be built")
	}
	first := base.Chunks[0]
	if strings.TrimSpace(first.HeadingPath) == "" {
		t.Fatalf("heading path should not be empty")
	}
	if strings.TrimSpace(first.Content) == "" {
		t.Fatalf("chunk content should not be empty")
	}
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), strings.TrimSpace(target)) {
			return true
		}
	}
	return false
}
