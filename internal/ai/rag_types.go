package ai

// KnowledgeDocument is one markdown document loaded from local knowledge files.
type KnowledgeDocument struct {
	ID         string
	Title      string
	Category   string
	Tags       []string
	SourcePath string
	RawContent string
}

// KnowledgeChunk is one chunk split from a knowledge document.
type KnowledgeChunk struct {
	ID          string
	DocumentID  string
	Title       string
	Category    string
	Tags        []string
	Content     string
	HeadingPath string
	SourcePath  string
}

// KnowledgeHit is one retrieval hit returned by knowledge search.
type KnowledgeHit struct {
	ChunkID    string  `json:"chunk_id"`
	DocumentID string  `json:"document_id"`
	Title      string  `json:"title"`
	Category   string  `json:"category"`
	Score      float64 `json:"score"`
	Snippet    string  `json:"snippet"`
	SourcePath string  `json:"source_path"`
}

// RetrievalMeta contains retrieval execution metadata.
type RetrievalMeta struct {
	Query          string  `json:"query"`
	TopK           int     `json:"top_k"`
	CandidateCount int     `json:"candidate_count"`
	ReturnedCount  int     `json:"returned_count"`
	Hit            bool    `json:"hit"`
	TopScore       float64 `json:"top_score"`
	DurationMs     int64   `json:"duration_ms"`
	Strategy       string  `json:"strategy"`
}

// RetrievalStats tracks online retrieval metrics.
type RetrievalStats struct {
	KnowledgeEnabled bool    `json:"knowledge_enabled"`
	LoadedDocuments  int     `json:"loaded_documents"`
	LoadedChunks     int     `json:"loaded_chunks"`
	TotalSearches    int64   `json:"total_searches"`
	RetrievalHits    int64   `json:"retrieval_hits"`
	RetrievalMisses  int64   `json:"retrieval_misses"`
	OnlineHitRate    float64 `json:"online_hit_rate"`
	AvgTopScore      float64 `json:"avg_top_score"`
	LastReloadAtUnix int64   `json:"last_reload_at_unix"`
}
