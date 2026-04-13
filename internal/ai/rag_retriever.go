package ai

import (
	"context"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	defaultKnowledgeTopK          = 3
	defaultKnowledgeMinScore      = 1.1
	defaultKnowledgeMaxSnippetLen = 180
	defaultKnowledgeStrategy      = "hybrid-lite"
)

// KnowledgeRetrieverOptions controls lexical retriever behavior.
type KnowledgeRetrieverOptions struct {
	DefaultTopK       int
	MinScore          float64
	MaxSnippetChars   int
	RetrievalStrategy string
}

// KnowledgeRetriever performs in-memory lightweight lexical retrieval.
type KnowledgeRetriever struct {
	documents       []KnowledgeDocument
	chunks          []indexedKnowledgeChunk
	defaultTopK     int
	minScore        float64
	maxSnippetChars int
	strategy        string
}

type indexedKnowledgeChunk struct {
	chunk         KnowledgeChunk
	titleLower    string
	categoryLower string
	headingLower  string
	contentLower  string
	titleTokens   map[string]struct{}
	headingTokens map[string]struct{}
	contentTokens map[string]struct{}
	tagSet        map[string]struct{}
}

type scoredKnowledgeChunk struct {
	idx   int
	score float64
}

// NewKnowledgeRetriever builds an immutable in-memory retriever.
func NewKnowledgeRetriever(base KnowledgeBase, opts KnowledgeRetrieverOptions) *KnowledgeRetriever {
	defaultTopK := opts.DefaultTopK
	if defaultTopK <= 0 {
		defaultTopK = defaultKnowledgeTopK
	}
	minScore := opts.MinScore
	if minScore <= 0 {
		minScore = defaultKnowledgeMinScore
	}
	maxSnippet := opts.MaxSnippetChars
	if maxSnippet <= 0 {
		maxSnippet = defaultKnowledgeMaxSnippetLen
	}
	strategy := strings.TrimSpace(opts.RetrievalStrategy)
	if strategy == "" {
		strategy = defaultKnowledgeStrategy
	}

	indexedChunks := make([]indexedKnowledgeChunk, 0, len(base.Chunks))
	for _, chunk := range base.Chunks {
		titleLower := strings.ToLower(strings.TrimSpace(chunk.Title))
		categoryLower := strings.ToLower(strings.TrimSpace(chunk.Category))
		headingLower := strings.ToLower(strings.TrimSpace(chunk.HeadingPath))
		contentLower := strings.ToLower(strings.TrimSpace(chunk.Content))
		indexedChunks = append(indexedChunks, indexedKnowledgeChunk{
			chunk:         chunk,
			titleLower:    titleLower,
			categoryLower: categoryLower,
			headingLower:  headingLower,
			contentLower:  contentLower,
			titleTokens:   tokenSet(tokenizeForSearch(chunk.Title)),
			headingTokens: tokenSet(tokenizeForSearch(chunk.HeadingPath)),
			contentTokens: tokenSet(tokenizeForSearch(chunk.Content)),
			tagSet:        tokenSet(normalizeTags(chunk.Tags)),
		})
	}

	docs := make([]KnowledgeDocument, 0, len(base.Documents))
	for _, doc := range base.Documents {
		docs = append(docs, doc)
	}

	return &KnowledgeRetriever{
		documents:       docs,
		chunks:          indexedChunks,
		defaultTopK:     defaultTopK,
		minScore:        minScore,
		maxSnippetChars: maxSnippet,
		strategy:        strategy,
	}
}

// DocumentCount returns loaded knowledge document count.
func (r *KnowledgeRetriever) DocumentCount() int {
	if r == nil {
		return 0
	}
	return len(r.documents)
}

// ChunkCount returns loaded chunk count.
func (r *KnowledgeRetriever) ChunkCount() int {
	if r == nil {
		return 0
	}
	return len(r.chunks)
}

// Search performs lexical retrieval and returns top hits and retrieval metadata.
func (r *KnowledgeRetriever) Search(ctx context.Context, query string, limit int) ([]KnowledgeHit, RetrievalMeta, error) {
	start := time.Now()
	topK := limit
	if topK <= 0 {
		topK = defaultKnowledgeTopK
	}
	if r != nil && r.defaultTopK > 0 && limit <= 0 {
		topK = r.defaultTopK
	}

	meta := RetrievalMeta{
		Query:    strings.TrimSpace(query),
		TopK:     topK,
		Strategy: defaultKnowledgeStrategy,
	}
	if r != nil && strings.TrimSpace(r.strategy) != "" {
		meta.Strategy = r.strategy
	}

	if r == nil || len(r.chunks) == 0 {
		meta.DurationMs = time.Since(start).Milliseconds()
		return nil, meta, nil
	}

	normalizedQuery := normalizeSearchText(query)
	queryTokens := tokenizeForSearch(query)
	queryTokenSet := tokenSet(queryTokens)
	if normalizedQuery == "" || len(queryTokenSet) == 0 {
		meta.DurationMs = time.Since(start).Milliseconds()
		return nil, meta, nil
	}

	candidates := make([]scoredKnowledgeChunk, 0, len(r.chunks)/3)
	for idx, chunk := range r.chunks {
		if err := ctx.Err(); err != nil {
			return nil, meta, err
		}
		score := calculateKnowledgeScore(normalizedQuery, queryTokenSet, chunk)
		if score <= 0 {
			continue
		}
		candidates = append(candidates, scoredKnowledgeChunk{
			idx:   idx,
			score: score,
		})
	}
	meta.CandidateCount = len(candidates)

	sort.Slice(candidates, func(i, j int) bool {
		left := candidates[i]
		right := candidates[j]
		if left.score != right.score {
			return left.score > right.score
		}
		leftChunk := r.chunks[left.idx].chunk
		rightChunk := r.chunks[right.idx].chunk
		if leftChunk.Title != rightChunk.Title {
			return leftChunk.Title < rightChunk.Title
		}
		if leftChunk.SourcePath != rightChunk.SourcePath {
			return leftChunk.SourcePath < rightChunk.SourcePath
		}
		return leftChunk.ID < rightChunk.ID
	})

	minScore := r.minScore
	if minScore <= 0 {
		minScore = defaultKnowledgeMinScore
	}

	hits := make([]KnowledgeHit, 0, topK)
	for _, candidate := range candidates {
		if len(hits) >= topK {
			break
		}
		if candidate.score < minScore {
			continue
		}

		chunk := r.chunks[candidate.idx].chunk
		hits = append(hits, KnowledgeHit{
			ChunkID:    chunk.ID,
			DocumentID: chunk.DocumentID,
			Title:      chunk.Title,
			Category:   chunk.Category,
			Score:      candidate.score,
			Snippet:    buildKnowledgeSnippet(chunk.Content, normalizedQuery, queryTokens, r.maxSnippetChars),
			SourcePath: chunk.SourcePath,
		})
	}

	meta.ReturnedCount = len(hits)
	meta.Hit = len(hits) > 0
	if len(hits) > 0 {
		meta.TopScore = hits[0].Score
	}
	meta.DurationMs = time.Since(start).Milliseconds()
	return hits, meta, nil
}

func calculateKnowledgeScore(query string, queryTokens map[string]struct{}, chunk indexedKnowledgeChunk) float64 {
	if len(queryTokens) == 0 {
		return 0
	}
	queryTokenCount := float64(len(queryTokens))

	titleOverlap := overlapRatio(queryTokens, chunk.titleTokens)
	headingOverlap := overlapRatio(queryTokens, chunk.headingTokens)
	contentOverlap := overlapRatio(queryTokens, chunk.contentTokens)

	score := 0.0
	score += titleOverlap * 4.2
	score += headingOverlap * 2.8
	score += contentOverlap * 3.8

	if chunk.categoryLower != "" && (strings.Contains(query, chunk.categoryLower) || setContains(queryTokens, chunk.categoryLower)) {
		score += 1.5
	}

	for tag := range chunk.tagSet {
		if strings.Contains(query, tag) || setContains(queryTokens, tag) {
			score += 1.1
		}
	}

	if query != "" {
		if strings.Contains(chunk.titleLower, query) {
			score += 3.4
		}
		if strings.Contains(chunk.headingLower, query) {
			score += 2.2
		}
		if strings.Contains(chunk.contentLower, query) {
			score += 2.0
		}
	}

	if queryTokenCount > 0 {
		score += float64(intersectionCount(queryTokens, chunk.titleTokens)) / queryTokenCount * 1.2
		score += float64(intersectionCount(queryTokens, chunk.headingTokens)) / queryTokenCount * 0.8
	}
	return score
}

func overlapRatio(left map[string]struct{}, right map[string]struct{}) float64 {
	if len(left) == 0 || len(right) == 0 {
		return 0
	}
	matched := intersectionCount(left, right)
	if matched == 0 {
		return 0
	}
	return float64(matched) / float64(len(left))
}

func intersectionCount(left map[string]struct{}, right map[string]struct{}) int {
	if len(left) == 0 || len(right) == 0 {
		return 0
	}
	count := 0
	for token := range left {
		if _, ok := right[token]; ok {
			count++
		}
	}
	return count
}

func setContains(set map[string]struct{}, value string) bool {
	_, ok := set[value]
	return ok
}

func tokenSet(tokens []string) map[string]struct{} {
	if len(tokens) == 0 {
		return map[string]struct{}{}
	}
	set := make(map[string]struct{}, len(tokens))
	for _, token := range tokens {
		normalized := strings.ToLower(strings.TrimSpace(token))
		if normalized == "" {
			continue
		}
		set[normalized] = struct{}{}
	}
	return set
}

func normalizeSearchText(raw string) string {
	text := strings.ToLower(strings.TrimSpace(raw))
	if text == "" {
		return ""
	}
	return strings.Join(strings.Fields(text), " ")
}

func tokenizeForSearch(raw string) []string {
	input := normalizeSearchText(raw)
	if input == "" {
		return nil
	}

	tokens := make([]string, 0, len(input)/2)
	ascii := strings.Builder{}
	chinese := make([]rune, 0, 8)

	flushASCII := func() {
		if ascii.Len() == 0 {
			return
		}
		token := strings.TrimSpace(ascii.String())
		ascii.Reset()
		if token == "" {
			return
		}
		tokens = append(tokens, token)
	}
	flushChinese := func() {
		if len(chinese) == 0 {
			return
		}
		for i := 0; i < len(chinese); i++ {
			tokens = append(tokens, string(chinese[i]))
		}
		for i := 0; i < len(chinese)-1; i++ {
			tokens = append(tokens, string(chinese[i:i+2]))
		}
		if len(chinese) >= 3 {
			for i := 0; i < len(chinese)-2; i++ {
				tokens = append(tokens, string(chinese[i:i+3]))
			}
		}
		chinese = chinese[:0]
	}

	for _, r := range input {
		switch {
		case isASCIIWordRune(r):
			flushChinese()
			ascii.WriteRune(r)
		case isCJKRune(r):
			flushASCII()
			chinese = append(chinese, r)
		default:
			flushASCII()
			flushChinese()
		}
	}
	flushASCII()
	flushChinese()

	return dedupeTokens(tokens)
}

func isASCIIWordRune(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
}

func isCJKRune(r rune) bool {
	return unicode.Is(unicode.Han, r)
}

func dedupeTokens(tokens []string) []string {
	if len(tokens) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(tokens))
	out := make([]string, 0, len(tokens))
	for _, token := range tokens {
		normalized := strings.TrimSpace(strings.ToLower(token))
		if normalized == "" {
			continue
		}
		if _, exists := seen[normalized]; exists {
			continue
		}
		seen[normalized] = struct{}{}
		out = append(out, normalized)
	}
	return out
}

func buildKnowledgeSnippet(content, query string, queryTokens []string, maxChars int) string {
	normalized := strings.Join(strings.Fields(strings.TrimSpace(content)), " ")
	if normalized == "" {
		return ""
	}

	if maxChars <= 0 {
		maxChars = defaultKnowledgeMaxSnippetLen
	}
	runes := []rune(normalized)
	if len(runes) <= maxChars {
		return normalized
	}

	lower := strings.ToLower(normalized)
	position := -1
	if query != "" {
		position = strings.Index(lower, query)
	}
	if position < 0 {
		longTokens := make([]string, 0, len(queryTokens))
		for _, token := range queryTokens {
			if utf8.RuneCountInString(token) < 2 {
				continue
			}
			longTokens = append(longTokens, token)
		}
		sort.Slice(longTokens, func(i, j int) bool {
			return utf8.RuneCountInString(longTokens[i]) > utf8.RuneCountInString(longTokens[j])
		})
		for _, token := range longTokens {
			position = strings.Index(lower, token)
			if position >= 0 {
				break
			}
		}
	}

	center := len(runes) / 2
	if position >= 0 {
		center = utf8.RuneCountInString(lower[:position])
	}
	half := maxChars / 2
	start := center - half
	if start < 0 {
		start = 0
	}
	end := start + maxChars
	if end > len(runes) {
		end = len(runes)
		start = end - maxChars
		if start < 0 {
			start = 0
		}
	}

	snippet := strings.TrimSpace(string(runes[start:end]))
	if start > 0 {
		snippet = "... " + snippet
	}
	if end < len(runes) {
		snippet += " ..."
	}
	return snippet
}
