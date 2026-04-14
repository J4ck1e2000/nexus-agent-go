package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// RetrievalEvalCase defines one offline retrieval evaluation case.
type RetrievalEvalCase struct {
	Query          string   `json:"query"`
	ExpectedDocIDs []string `json:"expected_doc_ids"`
	ExpectedTitles []string `json:"expected_titles"`
}

// RetrievalEvalReport stores offline HitRate@K metrics.
type RetrievalEvalReport struct {
	TotalCases      int     `json:"total_cases"`
	HitsAt1         int     `json:"hits_at_1"`
	HitsAt3         int     `json:"hits_at_3"`
	HitsAt5         int     `json:"hits_at_5"`
	HitRateAt1      float64 `json:"hit_rate_at_1"`
	HitRateAt3      float64 `json:"hit_rate_at_3"`
	HitRateAt5      float64 `json:"hit_rate_at_5"`
	Backend         string  `json:"backend,omitempty"`
	Strategy        string  `json:"strategy,omitempty"`
	AvgDurationMs   float64 `json:"avg_duration_ms,omitempty"`
	TotalQueries    int     `json:"total_queries"`
	AccumDurationMs int64   `json:"-"`
}

// LoadRetrievalEvalCases reads evaluation cases from local JSON file.
func LoadRetrievalEvalCases(path string) ([]RetrievalEvalCase, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cases []RetrievalEvalCase
	if err := json.Unmarshal(raw, &cases); err != nil {
		return nil, err
	}
	return cases, nil
}

// EvaluateRetrievalHitRate computes offline HitRate@1/3/5 for any search backend.
func EvaluateRetrievalHitRate(ctx context.Context, searcher KnowledgeSearcher, cases []RetrievalEvalCase, topK int, backend string) (RetrievalEvalReport, error) {
	report := RetrievalEvalReport{
		TotalCases:   len(cases),
		Backend:      strings.TrimSpace(backend),
		TotalQueries: len(cases),
	}
	if searcher == nil {
		return report, fmt.Errorf("searcher is nil")
	}
	if len(cases) == 0 {
		return report, nil
	}
	if topK <= 0 {
		topK = 5
	}

	for _, testCase := range cases {
		if err := ctx.Err(); err != nil {
			return RetrievalEvalReport{}, err
		}
		hits, meta, err := searcher.Search(ctx, testCase.Query, topK)
		if err != nil {
			return RetrievalEvalReport{}, err
		}
		if strings.TrimSpace(meta.Strategy) != "" {
			report.Strategy = meta.Strategy
		}
		report.AccumDurationMs += meta.DurationMs

		if hitWithinTopK(hits, testCase, 1) {
			report.HitsAt1++
		}
		if hitWithinTopK(hits, testCase, 3) {
			report.HitsAt3++
		}
		if hitWithinTopK(hits, testCase, 5) {
			report.HitsAt5++
		}
	}

	total := float64(report.TotalCases)
	if total > 0 {
		report.HitRateAt1 = float64(report.HitsAt1) / total
		report.HitRateAt3 = float64(report.HitsAt3) / total
		report.HitRateAt5 = float64(report.HitsAt5) / total
		report.AvgDurationMs = float64(report.AccumDurationMs) / total
	}
	return report, nil
}

func hitWithinTopK(hits []KnowledgeHit, testCase RetrievalEvalCase, k int) bool {
	if k <= 0 {
		return false
	}
	docSet := make(map[string]struct{}, len(testCase.ExpectedDocIDs))
	for _, id := range testCase.ExpectedDocIDs {
		key := strings.ToLower(strings.TrimSpace(id))
		if key == "" {
			continue
		}
		docSet[key] = struct{}{}
	}

	titleSet := make(map[string]struct{}, len(testCase.ExpectedTitles))
	for _, title := range testCase.ExpectedTitles {
		key := strings.ToLower(strings.TrimSpace(title))
		if key == "" {
			continue
		}
		titleSet[key] = struct{}{}
	}

	limit := k
	if limit > len(hits) {
		limit = len(hits)
	}
	for i := 0; i < limit; i++ {
		hit := hits[i]
		if len(docSet) > 0 {
			if _, ok := docSet[strings.ToLower(strings.TrimSpace(hit.DocumentID))]; ok {
				return true
			}
		}
		if len(titleSet) > 0 {
			if _, ok := titleSet[strings.ToLower(strings.TrimSpace(hit.Title))]; ok {
				return true
			}
		}
	}
	return false
}
