package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"

	"nexus-agent-go/internal/ai"
)

func main() {
	knowledgeDir := flag.String("knowledge-dir", "knowledge/anomalies", "knowledge markdown directory")
	casesPath := flag.String("cases", "testdata/knowledge_eval_cases.json", "evaluation cases json path")
	backend := flag.String("backend", "local", "retrieval backend: local|qdrant|auto")
	topK := flag.Int("topk", 5, "retrieval top-k for evaluation")
	minScore := flag.Float64("min-score", 1.1, "local lexical retrieval min score")
	flag.Parse()

	cfg, err := ai.LoadKnowledgeRetrievalConfigFromEnv()
	if err != nil {
		log.Printf("load retrieval config warning: %v", err)
	}
	cfg.Backend = *backend
	cfg.TopK = *topK
	if cfg.TopK <= 0 {
		cfg.TopK = 5
	}
	if cfg.MinScore < 0 {
		cfg.MinScore = 0
	}
	if err := cfg.Validate(); err != nil {
		log.Fatalf("invalid retrieval config: %v", err)
	}

	buildResult, err := ai.BuildKnowledgeSearcher(
		context.Background(),
		*knowledgeDir,
		ai.KnowledgeRetrieverOptions{
			DefaultTopK:       cfg.TopK,
			MinScore:          *minScore,
			MaxSnippetChars:   180,
			RetrievalStrategy: "local-lexical",
		},
		cfg,
		log.Printf,
	)
	if err != nil {
		log.Fatalf("build knowledge searcher failed: %v", err)
	}
	if buildResult.Searcher == nil {
		log.Fatalf("knowledge searcher is unavailable for backend=%s", cfg.NormalizedBackend())
	}

	cases, err := ai.LoadRetrievalEvalCases(*casesPath)
	if err != nil {
		log.Fatalf("load eval cases failed: %v", err)
	}

	report, err := ai.EvaluateRetrievalHitRate(
		context.Background(),
		buildResult.Searcher,
		cases,
		cfg.TopK,
		cfg.NormalizedBackend(),
	)
	if err != nil {
		log.Fatalf("evaluate hit rate failed: %v", err)
	}

	fmt.Printf("Knowledge docs: %d, chunks: %d\n", buildResult.Documents, buildResult.Chunks)
	fmt.Printf("Backend: %s, Strategy: %s\n", report.Backend, report.Strategy)
	fmt.Printf("Total cases: %d\n", report.TotalCases)
	fmt.Printf("HitRate@1: %.3f (%d/%d)\n", report.HitRateAt1, report.HitsAt1, report.TotalCases)
	fmt.Printf("HitRate@3: %.3f (%d/%d)\n", report.HitRateAt3, report.HitsAt3, report.TotalCases)
	fmt.Printf("HitRate@5: %.3f (%d/%d)\n", report.HitRateAt5, report.HitsAt5, report.TotalCases)
	fmt.Printf("Avg duration(ms): %.2f\n", report.AvgDurationMs)

	raw, err := json.MarshalIndent(report, "", "  ")
	if err == nil {
		_, _ = fmt.Fprintln(os.Stdout, string(raw))
	}
}
