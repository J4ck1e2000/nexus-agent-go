package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"time"

	"nexus-agent-go/internal/ai"
)

func main() {
	knowledgeDir := flag.String("knowledge-dir", "knowledge/anomalies", "knowledge markdown directory")
	recreate := flag.Bool("recreate", false, "recreate qdrant collection before sync")
	batchSize := flag.Int("batch-size", 0, "sync batch size, default from env KNOWLEDGE_SYNC_BATCH_SIZE")
	flag.Parse()

	cfg, err := ai.LoadKnowledgeRetrievalConfigFromEnv()
	if err != nil {
		log.Fatalf("load retrieval config failed: %v", err)
	}
	if !cfg.VectorRetrievalEnabled() {
		log.Fatalf("vector retrieval is not ready: %s", cfg.SafeSummary())
	}

	base, err := ai.LoadKnowledgeBase(*knowledgeDir)
	if err != nil {
		log.Fatalf("load knowledge base failed: %v", err)
	}
	if len(base.Chunks) == 0 {
		log.Fatalf("knowledge base has no chunks: %s", *knowledgeDir)
	}

	embedding, err := ai.NewEmbeddingProvider(cfg)
	if err != nil {
		log.Fatalf("init embedding provider failed: %v", err)
	}
	client := ai.NewQdrantHTTPClient(cfg.Qdrant)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	if err := client.Health(ctx); err != nil {
		log.Fatalf("qdrant health check failed: %v", err)
	}

	log.Printf("knowledge-sync started: docs=%d chunks=%d collection=%s model=%s batch=%d",
		len(base.Documents),
		len(base.Chunks),
		cfg.Qdrant.Collection,
		cfg.Embedding.Model,
		resolveBatchSize(*batchSize, cfg.SyncBatchSize),
	)

	result, err := ai.SyncKnowledgeChunks(ctx, base.Chunks, cfg, embedding, client, ai.KnowledgeSyncOptions{
		BatchSize: *batchSize,
		Recreate:  *recreate,
		Logf:      log.Printf,
	})
	if err != nil {
		log.Fatalf("knowledge-sync failed: %v", err)
	}
	result.Documents = len(base.Documents)

	fmt.Printf("Knowledge docs: %d\n", result.Documents)
	fmt.Printf("Knowledge chunks: %d\n", result.Chunks)
	fmt.Printf("Upserted points: %d\n", result.Upserted)
	fmt.Printf("Collection: %s\n", result.Collection)
	fmt.Printf("Embedding model: %s\n", result.EmbeddingModel)
	fmt.Printf("Duration(ms): %d\n", result.DurationMs)
}

func resolveBatchSize(flagValue int, cfgValue int) int {
	if flagValue > 0 {
		return flagValue
	}
	if cfgValue > 0 {
		return cfgValue
	}
	return 32
}
