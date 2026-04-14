package ai

import (
	"context"
	"testing"
)

func TestEvaluateRetrievalHitRate(t *testing.T) {
	retriever := buildTestRetriever(t)
	searcher := NewLocalKnowledgeSearcher(retriever)
	cases := []RetrievalEvalCase{
		{Query: "GPU 显存爆满怎么处理", ExpectedDocIDs: []string{"gpu_oom"}},
		{Query: "节点离线后先排查什么", ExpectedDocIDs: []string{"node_offline"}},
		{Query: "指标太旧了怎么办", ExpectedDocIDs: []string{"metrics_stale"}},
		{Query: "完全无关的问题", ExpectedDocIDs: []string{"gpu_oom"}},
	}

	report, err := EvaluateRetrievalHitRate(context.Background(), searcher, cases, 5, KnowledgeBackendLocal)
	if err != nil {
		t.Fatalf("EvaluateRetrievalHitRate failed: %v", err)
	}
	if report.TotalCases != len(cases) {
		t.Fatalf("total cases mismatch: got=%d want=%d", report.TotalCases, len(cases))
	}
	if report.HitsAt1 < 3 {
		t.Fatalf("expected at least 3 hits@1, got=%d", report.HitsAt1)
	}
	if report.HitRateAt3 < report.HitRateAt1 {
		t.Fatalf("HitRate@3 should be >= HitRate@1, got %.3f < %.3f", report.HitRateAt3, report.HitRateAt1)
	}
	if report.Backend != KnowledgeBackendLocal {
		t.Fatalf("backend mismatch: got=%s want=%s", report.Backend, KnowledgeBackendLocal)
	}
	if report.Strategy == "" {
		t.Fatalf("strategy should not be empty")
	}
}
