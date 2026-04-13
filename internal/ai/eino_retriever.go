package ai

import (
	"context"
	"strings"

	einoretriever "github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/schema"
)

// RetrieverWithMeta is an Eino retriever component with retrieval metadata support.
type RetrieverWithMeta interface {
	einoretriever.Retriever
	RetrieveWithMeta(ctx context.Context, query string, opts ...einoretriever.Option) ([]*schema.Document, RetrievalMeta, error)
}

type toolboxKnowledgeRetriever struct {
	toolbox *Toolbox
}

// NewToolboxKnowledgeRetriever wraps existing lexical retrieval into an Eino Retriever component.
func NewToolboxKnowledgeRetriever(toolbox *Toolbox) RetrieverWithMeta {
	return &toolboxKnowledgeRetriever{toolbox: toolbox}
}

func (r *toolboxKnowledgeRetriever) Retrieve(ctx context.Context, query string, opts ...einoretriever.Option) ([]*schema.Document, error) {
	docs, _, err := r.RetrieveWithMeta(ctx, query, opts...)
	if err != nil {
		return nil, err
	}
	return docs, nil
}

func (r *toolboxKnowledgeRetriever) RetrieveWithMeta(ctx context.Context, query string, opts ...einoretriever.Option) ([]*schema.Document, RetrievalMeta, error) {
	common := einoretriever.GetCommonOptions(nil, opts...)
	topK := defaultKnowledgeTopK
	if common != nil && common.TopK != nil && *common.TopK > 0 {
		topK = *common.TopK
	}

	if r == nil || r.toolbox == nil {
		meta := RetrievalMeta{
			Query:    strings.TrimSpace(query),
			TopK:     topK,
			Strategy: defaultKnowledgeStrategy,
		}
		return nil, meta, nil
	}

	hits, meta, err := r.toolbox.SearchKnowledge(ctx, query, topK)
	if err != nil {
		return nil, meta, err
	}

	docs := make([]*schema.Document, 0, len(hits))
	for _, hit := range hits {
		doc := (&schema.Document{
			ID:      hit.ChunkID,
			Content: hit.Snippet,
			MetaData: map[string]any{
				"chunk_id":    hit.ChunkID,
				"document_id": hit.DocumentID,
				"title":       hit.Title,
				"category":    hit.Category,
				"source_path": hit.SourcePath,
				"snippet":     hit.Snippet,
			},
		}).WithScore(hit.Score)
		docs = append(docs, doc)
	}

	return docs, meta, nil
}
