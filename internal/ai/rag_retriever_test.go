package ai

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestKnowledgeRetriever_MatchesCoreQueries(t *testing.T) {
	retriever := buildTestRetriever(t)

	cases := []struct {
		name      string
		query     string
		wantDocID string
	}{
		{name: "gpu oom", query: "GPU 显存爆满怎么处理", wantDocID: "gpu_oom"},
		{name: "node offline", query: "节点离线后先排查什么", wantDocID: "node_offline"},
		{name: "metrics stale", query: "指标太旧了怎么办", wantDocID: "metrics_stale"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			hits, meta, err := retriever.Search(context.Background(), tc.query, 3)
			if err != nil {
				t.Fatalf("Search failed: %v", err)
			}
			if !meta.Hit {
				t.Fatalf("expected hit for query=%q", tc.query)
			}
			if len(hits) == 0 {
				t.Fatalf("expected hits for query=%q", tc.query)
			}
			if hits[0].DocumentID != tc.wantDocID {
				t.Fatalf("top hit mismatch: got=%q want=%q", hits[0].DocumentID, tc.wantDocID)
			}
		})
	}
}

func TestKnowledgeRetriever_StableTopKOrder(t *testing.T) {
	retriever := buildTestRetriever(t)
	query := "GPU 显存不足排障步骤"

	hitsA, _, err := retriever.Search(context.Background(), query, 3)
	if err != nil {
		t.Fatalf("Search A failed: %v", err)
	}
	hitsB, _, err := retriever.Search(context.Background(), query, 3)
	if err != nil {
		t.Fatalf("Search B failed: %v", err)
	}
	if len(hitsA) != len(hitsB) {
		t.Fatalf("hit length mismatch: %d vs %d", len(hitsA), len(hitsB))
	}
	for i := range hitsA {
		if hitsA[i].ChunkID != hitsB[i].ChunkID {
			t.Fatalf("order mismatch at %d: %q vs %q", i, hitsA[i].ChunkID, hitsB[i].ChunkID)
		}
	}
}

func TestKnowledgeRetriever_EmptyQuery(t *testing.T) {
	retriever := buildTestRetriever(t)
	hits, meta, err := retriever.Search(context.Background(), "   ", 3)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(hits) != 0 {
		t.Fatalf("expected no hits, got=%d", len(hits))
	}
	if meta.Hit {
		t.Fatalf("empty query should not be a hit")
	}
}

func buildTestRetriever(t *testing.T) *KnowledgeRetriever {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"GPU_OOM.md": `# GPU OOM / 显存不足
Category: gpu_memory
Tags: gpu, oom, cuda, 显存不足
## Symptoms
CUDA out of memory and VRAM near 100%.
## Recommended Actions
Reduce batch size and clean orphan processes.`,
		"NODE_OFFLINE.md": `# 节点离线
Category: node_connectivity
Tags: offline, unreachable, 节点离线
## Symptoms
Node is offline and unreachable.
## Recommended Actions
Check agent service and network connectivity.`,
		"METRICS_STALE.md": `# 指标数据陈旧 / Metrics Stale
Category: data_freshness
Tags: stale, metrics stale, 数据太旧
## Symptoms
Collected timestamp lags behind polling interval.
## Recommended Actions
Refresh telemetry and re-evaluate anomalies.`,
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatalf("write %s failed: %v", name, err)
		}
	}
	base, err := LoadKnowledgeBase(dir)
	if err != nil {
		t.Fatalf("LoadKnowledgeBase failed: %v", err)
	}
	return NewKnowledgeRetriever(base, KnowledgeRetrieverOptions{
		DefaultTopK:       3,
		MinScore:          0.3,
		MaxSnippetChars:   120,
		RetrievalStrategy: "hybrid-lite",
	})
}
