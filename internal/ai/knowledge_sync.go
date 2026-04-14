package ai

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// KnowledgeSyncOptions controls sync behavior.
type KnowledgeSyncOptions struct {
	BatchSize int
	Recreate  bool
	Logf      func(format string, args ...any)
}

// KnowledgeSyncResult summarizes sync execution outcome.
type KnowledgeSyncResult struct {
	Documents      int
	Chunks         int
	Upserted       int
	Collection     string
	EmbeddingModel string
	DurationMs     int64
}

// SyncKnowledgeChunks syncs local chunks into qdrant collection using embeddings.
func SyncKnowledgeChunks(
	ctx context.Context,
	chunks []KnowledgeChunk,
	cfg KnowledgeRetrievalConfig,
	embedding EmbeddingProvider,
	client *qdrantHTTPClient,
	opts KnowledgeSyncOptions,
) (KnowledgeSyncResult, error) {
	start := time.Now()
	if len(chunks) == 0 {
		return KnowledgeSyncResult{
			Collection:     cfg.Qdrant.Collection,
			EmbeddingModel: cfg.Embedding.Model,
		}, nil
	}
	if embedding == nil {
		return KnowledgeSyncResult{}, fmt.Errorf("embedding provider is nil")
	}
	if client == nil {
		return KnowledgeSyncResult{}, fmt.Errorf("qdrant client is nil")
	}

	batchSize := opts.BatchSize
	if batchSize <= 0 {
		batchSize = cfg.SyncBatchSize
	}
	if batchSize <= 0 {
		batchSize = defaultSyncBatchSize
	}

	collection := strings.TrimSpace(cfg.Qdrant.Collection)
	if collection == "" {
		collection = defaultQdrantCollection
	}

	if opts.Recreate {
		logfSync(opts.Logf, "knowledge-sync: recreating qdrant collection=%s", collection)
		if err := client.DeleteCollection(ctx, collection); err != nil {
			return KnowledgeSyncResult{}, err
		}
	}
	if err := EnsureKnowledgeCollection(ctx, client, cfg); err != nil {
		return KnowledgeSyncResult{}, err
	}

	totalUpserted := 0
	for _, batch := range BatchSlice(chunks, batchSize) {
		if len(batch) == 0 {
			continue
		}
		texts := make([]string, 0, len(batch))
		for _, chunk := range batch {
			texts = append(texts, embeddingTextForChunk(chunk))
		}

		vectors, err := embedding.EmbedDocuments(ctx, texts)
		if err != nil {
			return KnowledgeSyncResult{}, fmt.Errorf("embed batch failed size=%d: %w", len(batch), err)
		}
		if len(vectors) != len(batch) {
			return KnowledgeSyncResult{}, fmt.Errorf("embedding result length mismatch got=%d want=%d", len(vectors), len(batch))
		}

		points := make([]qdrantPoint, 0, len(batch))
		for idx, chunk := range batch {
			point, err := BuildQdrantPoint(chunk, vectors[idx])
			if err != nil {
				return KnowledgeSyncResult{}, err
			}
			points = append(points, point)
		}

		if err := client.UpsertPoints(ctx, collection, points); err != nil {
			return KnowledgeSyncResult{}, err
		}
		totalUpserted += len(points)
		logfSync(opts.Logf, "knowledge-sync: upserted batch size=%d collection=%s", len(points), collection)
	}

	return KnowledgeSyncResult{
		Chunks:         len(chunks),
		Upserted:       totalUpserted,
		Collection:     collection,
		EmbeddingModel: cfg.Embedding.Model,
		DurationMs:     time.Since(start).Milliseconds(),
	}, nil
}

// BatchSlice splits input into batches of fixed size.
func BatchSlice[T any](items []T, batchSize int) [][]T {
	if len(items) == 0 {
		return nil
	}
	if batchSize <= 0 {
		batchSize = defaultSyncBatchSize
	}
	result := make([][]T, 0, (len(items)+batchSize-1)/batchSize)
	for start := 0; start < len(items); start += batchSize {
		end := start + batchSize
		if end > len(items) {
			end = len(items)
		}
		result = append(result, items[start:end])
	}
	return result
}

func embeddingTextForChunk(chunk KnowledgeChunk) string {
	return strings.TrimSpace(strings.Join([]string{
		strings.TrimSpace(chunk.Title),
		strings.TrimSpace(chunk.HeadingPath),
		strings.TrimSpace(chunk.Content),
	}, "\n"))
}

func logfSync(logf func(format string, args ...any), format string, args ...any) {
	if logf == nil {
		return
	}
	logf(format, args...)
}
