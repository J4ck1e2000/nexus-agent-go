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
	minScore := flag.Float64("min-score", 1.1, "retrieval min score")
	flag.Parse()

	base, err := ai.LoadKnowledgeBase(*knowledgeDir)
	if err != nil {
		log.Fatalf("load knowledge base failed: %v", err)
	}
	if len(base.Chunks) == 0 {
		log.Fatalf("knowledge base has no chunks: %s", *knowledgeDir)
	}

	retriever := ai.NewKnowledgeRetriever(base, ai.KnowledgeRetrieverOptions{
		DefaultTopK:       5,
		MinScore:          *minScore,
		MaxSnippetChars:   180,
		RetrievalStrategy: "hybrid-lite",
	})

	cases, err := ai.LoadRetrievalEvalCases(*casesPath)
	if err != nil {
		log.Fatalf("load eval cases failed: %v", err)
	}

	report, err := ai.EvaluateRetrievalHitRate(context.Background(), retriever, cases)
	if err != nil {
		log.Fatalf("evaluate hit rate failed: %v", err)
	}

	fmt.Printf("Knowledge docs: %d, chunks: %d\n", retriever.DocumentCount(), retriever.ChunkCount())
	fmt.Printf("Total cases: %d\n", report.TotalCases)
	fmt.Printf("HitRate@1: %.3f (%d/%d)\n", report.HitRateAt1, report.HitsAt1, report.TotalCases)
	fmt.Printf("HitRate@3: %.3f (%d/%d)\n", report.HitRateAt3, report.HitsAt3, report.TotalCases)
	fmt.Printf("HitRate@5: %.3f (%d/%d)\n", report.HitRateAt5, report.HitsAt5, report.TotalCases)

	raw, err := json.MarshalIndent(report, "", "  ")
	if err == nil {
		_, _ = fmt.Fprintln(os.Stdout, string(raw))
	}
}
