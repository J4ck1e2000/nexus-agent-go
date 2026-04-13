package ai

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	envAIEnabled         = "AI_ENABLED"
	envAIMode            = "AI_MODE"
	envAIProvider        = "AI_PROVIDER"
	envAIModel           = "AI_MODEL"
	envAIAPIKey          = "AI_API_KEY"
	envAIBaseURL         = "AI_BASE_URL"
	envAIRAGEnabled      = "AI_RAG_ENABLED"
	envAIRAGKnowledgeDir = "AI_RAG_KNOWLEDGE_DIR"
	envAIRAGTopK         = "AI_RAG_TOP_K"
	envAIRAGMinScore     = "AI_RAG_MIN_SCORE"
	envAIRAGMaxSnippet   = "AI_RAG_MAX_SNIPPET_CHARS"

	defaultOpenAIBaseURL = "https://api.openai.com/v1"
	defaultQwenBaseURL   = "https://dashscope.aliyuncs.com/compatible-mode/v1"
	defaultKnowledgeDir  = "knowledge/anomalies"

	defaultStreamChunkRuneSize = 64
)

// QueryExecutor executes one query request and returns normalized response.
type QueryExecutor interface {
	Execute(ctx context.Context, req AIQueryRequest) (AIQueryResponse, error)
}

// StreamQueryExecutor executes one query request with optional stream callbacks.
type StreamQueryExecutor interface {
	ExecuteStream(ctx context.Context, req AIQueryRequest, emit func(AIStreamEvent) error) (AIQueryResponse, error)
}

// Config controls runtime behavior of AI service.
type Config struct {
	Enabled         bool
	Mode            string
	Provider        string
	Model           string
	APIKey          string
	BaseURL         string
	RequestTimeout  time.Duration
	RAGEnabled      bool
	RAGKnowledgeDir string
	RAGTopK         int
	RAGMinScore     float64
	RAGMaxSnippet   int
}

// AgentReady indicates whether agent mode has enough model config.
func (c Config) AgentReady() bool {
	return strings.TrimSpace(c.Model) != "" &&
		strings.TrimSpace(c.APIKey) != "" &&
		strings.TrimSpace(c.BaseURL) != ""
}

func (c Config) normalizedMode() string {
	mode := strings.ToLower(strings.TrimSpace(c.Mode))
	switch mode {
	case "", AIModeRule:
		return AIModeRule
	case AIModeAgent, "eino":
		return AIModeAgent
	case AIModeLLM:
		return AIModeLLM
	default:
		return AIModeRule
	}
}

// LoadConfigFromEnv loads AI runtime config from environment variables.
func LoadConfigFromEnv() Config {
	cfg := Config{
		Enabled:         envBool(envAIEnabled, true),
		Mode:            strings.TrimSpace(os.Getenv(envAIMode)),
		Provider:        strings.TrimSpace(os.Getenv(envAIProvider)),
		Model:           strings.TrimSpace(os.Getenv(envAIModel)),
		APIKey:          strings.TrimSpace(os.Getenv(envAIAPIKey)),
		BaseURL:         strings.TrimSpace(os.Getenv(envAIBaseURL)),
		RequestTimeout:  defaultOpenAICompatibleTimeout,
		RAGEnabled:      envBool(envAIRAGEnabled, true),
		RAGKnowledgeDir: strings.TrimSpace(os.Getenv(envAIRAGKnowledgeDir)),
		RAGTopK:         envInt(envAIRAGTopK, defaultKnowledgeTopK),
		RAGMinScore:     envFloat(envAIRAGMinScore, defaultKnowledgeMinScore),
		RAGMaxSnippet:   envInt(envAIRAGMaxSnippet, defaultKnowledgeMaxSnippetLen),
	}
	if cfg.Mode == "" {
		cfg.Mode = AIModeRule
	}
	if cfg.BaseURL == "" {
		switch strings.ToLower(strings.TrimSpace(cfg.Provider)) {
		case "qwen", "dashscope", "aliyun":
			cfg.BaseURL = defaultQwenBaseURL
		default:
			cfg.BaseURL = defaultOpenAIBaseURL
		}
	}
	if cfg.RAGKnowledgeDir == "" {
		cfg.RAGKnowledgeDir = defaultKnowledgeDir
	}
	if cfg.RAGTopK <= 0 {
		cfg.RAGTopK = defaultKnowledgeTopK
	}
	if cfg.RAGMinScore <= 0 {
		cfg.RAGMinScore = defaultKnowledgeMinScore
	}
	if cfg.RAGMaxSnippet <= 0 {
		cfg.RAGMaxSnippet = defaultKnowledgeMaxSnippetLen
	}
	return cfg
}

// Service is the facade for AI query APIs.
type Service struct {
	config        Config
	toolbox       *Toolbox
	ruleExecutor  QueryExecutor
	agentExecutor QueryExecutor
}

// ServiceOptions wires dependencies for AI service.
type ServiceOptions struct {
	Config        Config
	Toolbox       *Toolbox
	Classifier    *IntentClassifier
	RuleExecutor  QueryExecutor
	AgentExecutor QueryExecutor
	SystemPrompt  string
}

// NewService creates a fully wired AI service with rule + optional agent executors.
func NewService(opts ServiceOptions) *Service {
	cfg := opts.Config
	if cfg.Mode == "" {
		cfg.Mode = AIModeRule
	}

	classifier := opts.Classifier
	if classifier == nil {
		classifier = NewIntentClassifier()
	}
	toolbox := opts.Toolbox
	if toolbox == nil {
		toolbox = NewToolbox(ToolboxOptions{})
	}

	ruleExecutor := opts.RuleExecutor
	if ruleExecutor == nil {
		ruleExecutor = NewRuleExecutor(classifier, toolbox)
	}

	agentExecutor := opts.AgentExecutor
	if agentExecutor == nil {
		var llmClient ChatCompletionClient
		if cfg.AgentReady() {
			llmClient = NewOpenAICompatibleClient(cfg.BaseURL, cfg.APIKey, cfg.RequestTimeout)
		}
		agentExecutor = NewEinoAgentExecutor(
			EinoAgentExecutorOptions{
				Classifier:   classifier,
				Toolbox:      toolbox,
				SystemPrompt: opts.SystemPrompt,
				LLMClient:    llmClient,
				Model:        cfg.Model,
			},
		)
	}

	return &Service{
		config:        cfg,
		toolbox:       toolbox,
		ruleExecutor:  ruleExecutor,
		agentExecutor: agentExecutor,
	}
}

// Query executes one AI request with rule/agent mode and fallback behavior.
func (s *Service) Query(ctx context.Context, req AIQueryRequest) (AIQueryResponse, error) {
	if !s.config.Enabled {
		return AIQueryResponse{}, ErrServiceDisabled
	}
	if strings.TrimSpace(req.Query) == "" {
		return AIQueryResponse{}, ErrInvalidQuery
	}

	mode := s.config.normalizedMode()
	if mode == AIModeAgent {
		if s.agentExecutor != nil && s.config.AgentReady() {
			resp, err := s.agentExecutor.Execute(ctx, req)
			if err == nil {
				resp.Mode = AIModeAgent
				return resp, nil
			}
			fallback, fallbackErr := s.ruleExecutor.Execute(ctx, req)
			if fallbackErr != nil {
				return AIQueryResponse{}, fallbackErr
			}
			fallback.Mode = AIModeRule
			fallback.Warnings = appendIfMissing(fallback.Warnings, localizedAgentFallbackWarning(req.Query))
			return fallback, nil
		}
	}

	resp, err := s.ruleExecutor.Execute(ctx, req)
	if err != nil {
		return AIQueryResponse{}, err
	}
	resp.Mode = AIModeRule
	return resp, nil
}

// QueryStream executes one AI request and emits stream events.
func (s *Service) QueryStream(ctx context.Context, req AIQueryRequest, emit func(AIStreamEvent) error) error {
	if emit == nil {
		return fmt.Errorf("stream emitter is nil")
	}
	if !s.config.Enabled {
		return ErrServiceDisabled
	}
	query := strings.TrimSpace(req.Query)
	if query == "" {
		return ErrInvalidQuery
	}

	safeEmit := func(event AIStreamEvent) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		return emit(event)
	}

	if err := safeEmit(AIStreamEvent{
		Event: StreamEventStart,
		Query: query,
	}); err != nil {
		return err
	}
	if err := safeEmit(AIStreamEvent{
		Event:   StreamEventStatus,
		Phase:   "thinking",
		Message: localizedStreamPhaseMessage(query, "thinking"),
	}); err != nil {
		return err
	}

	resp, err := s.queryWithOptionalStream(ctx, AIQueryRequest{
		Query:  query,
		Stream: true,
	}, safeEmit)
	if err != nil {
		return err
	}

	if err := safeEmit(AIStreamEvent{
		Event:   StreamEventStatus,
		Phase:   "generating",
		Message: localizedStreamPhaseMessage(query, "generating"),
	}); err != nil {
		return err
	}

	for _, chunk := range chunkAnswerForStream(resp.Answer, defaultStreamChunkRuneSize) {
		if err := safeEmit(AIStreamEvent{
			Event: StreamEventDelta,
			Text:  chunk,
		}); err != nil {
			return err
		}
	}

	meta := AIStreamMeta{
		ReasoningSummary: strings.TrimSpace(resp.ReasoningSummary),
		Mode:             strings.TrimSpace(resp.Mode),
		ToolCalls:        resp.ToolCalls,
		RelatedNodes:     resp.RelatedNodes,
		Warnings:         resp.Warnings,
		KnowledgeHits:    resp.KnowledgeHits,
		Retrieval:        resp.Retrieval,
	}
	if err := safeEmit(AIStreamEvent{
		Event: StreamEventMeta,
		Meta:  &meta,
	}); err != nil {
		return err
	}

	finalResp := resp
	if err := safeEmit(AIStreamEvent{
		Event: StreamEventDone,
		Done:  &finalResp,
	}); err != nil {
		return err
	}
	return nil
}

// Capabilities returns supported mode/intent/tool metadata.
func (s *Service) Capabilities(ctx context.Context) CapabilitiesResponse {
	_ = ctx
	tools := []string{"get_node_metrics", "list_idle_nodes", "get_gpu_processes", "get_node_summary", "get_alert_history", "recommend_nodes_for_job", "explain_node_anomaly"}
	if s.toolbox != nil && s.toolbox.HasKnowledge() {
		tools = append(tools, "search_knowledge_base")
	}
	return CapabilitiesResponse{
		Enabled:          s.config.Enabled,
		DefaultMode:      s.config.normalizedMode(),
		SupportedModes:   []string{AIModeRule, AIModeAgent},
		SupportedIntents: []string{string(IntentNodeSummary), string(IntentIdleNodeRanking), string(IntentScheduleSuggestion), string(IntentAnomalyExplanation), string(IntentAlertSummary), string(IntentHistoryAnalysis)},
		SupportedTools:   tools,
	}
}

// Health returns the current runtime health of AI service.
func (s *Service) Health(ctx context.Context) HealthResponse {
	status := "ok"
	if !s.config.Enabled {
		status = "disabled"
	}
	stats := s.RetrievalStats(ctx)
	return HealthResponse{
		Status:                 status,
		Mode:                   s.config.normalizedMode(),
		AgentReady:             s.config.AgentReady(),
		KnowledgeEnabled:       stats.KnowledgeEnabled,
		KnowledgeDocuments:     stats.LoadedDocuments,
		KnowledgeChunks:        stats.LoadedChunks,
		RetrievalOnlineHitRate: stats.OnlineHitRate,
	}
}

// RetrievalStats returns online retrieval metrics snapshot.
func (s *Service) RetrievalStats(ctx context.Context) RetrievalStats {
	if s == nil || s.toolbox == nil {
		return RetrievalStats{}
	}
	return s.toolbox.KnowledgeStats(ctx)
}

func envBool(key string, fallback bool) bool {
	value := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	if value == "" {
		return fallback
	}
	switch value {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}

func envInt(key string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return parsed
}

func envFloat(key string, fallback float64) float64 {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	parsed, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return fallback
	}
	return parsed
}

func appendIfMissing(values []string, value string) []string {
	target := strings.TrimSpace(value)
	if target == "" {
		return values
	}
	for _, item := range values {
		if strings.EqualFold(strings.TrimSpace(item), target) {
			return values
		}
	}
	return append(values, target)
}

func (s *Service) queryWithOptionalStream(ctx context.Context, req AIQueryRequest, emit func(AIStreamEvent) error) (AIQueryResponse, error) {
	mode := s.config.normalizedMode()
	if mode == AIModeAgent {
		if s.agentExecutor != nil && s.config.AgentReady() {
			if streamExecutor, ok := s.agentExecutor.(StreamQueryExecutor); ok {
				resp, err := streamExecutor.ExecuteStream(ctx, req, emit)
				if err == nil {
					resp.Mode = AIModeAgent
					return resp, nil
				}
				if emit != nil {
					_ = emit(AIStreamEvent{
						Event:   StreamEventStatus,
						Phase:   "fallback",
						Message: localizedStreamPhaseMessage(req.Query, "fallback_stream"),
					})
				}

				fallback, fallbackErr := s.ruleExecutor.Execute(ctx, req)
				if fallbackErr != nil {
					return AIQueryResponse{}, fallbackErr
				}
				fallback.Mode = AIModeRule
				fallback.Warnings = appendIfMissing(fallback.Warnings, localizedAgentFallbackWarning(req.Query))
				return fallback, nil
			}

			resp, err := s.agentExecutor.Execute(ctx, req)
			if err == nil {
				resp.Mode = AIModeAgent
				return resp, nil
			}
			if emit != nil {
				_ = emit(AIStreamEvent{
					Event:   StreamEventStatus,
					Phase:   "fallback",
					Message: localizedStreamPhaseMessage(req.Query, "fallback"),
				})
			}
			fallback, fallbackErr := s.ruleExecutor.Execute(ctx, req)
			if fallbackErr != nil {
				return AIQueryResponse{}, fallbackErr
			}
			fallback.Mode = AIModeRule
			fallback.Warnings = appendIfMissing(fallback.Warnings, localizedAgentFallbackWarning(req.Query))
			return fallback, nil
		}
	}

	resp, err := s.ruleExecutor.Execute(ctx, req)
	if err != nil {
		return AIQueryResponse{}, err
	}
	resp.Mode = AIModeRule
	return resp, nil
}

func chunkAnswerForStream(answer string, chunkSize int) []string {
	trimmed := strings.TrimSpace(answer)
	if trimmed == "" {
		return nil
	}
	if chunkSize <= 0 {
		chunkSize = defaultStreamChunkRuneSize
	}

	runes := []rune(trimmed)
	if len(runes) <= chunkSize {
		return []string{trimmed}
	}

	chunks := make([]string, 0, len(runes)/chunkSize+1)
	for start := 0; start < len(runes); {
		end := start + chunkSize
		if end > len(runes) {
			end = len(runes)
		}
		if end < len(runes) {
			best := end
			for cursor := end; cursor > start+chunkSize/2; cursor-- {
				r := runes[cursor-1]
				if r == ' ' || r == '\n' || r == '\t' ||
					r == ',' || r == '.' || r == ';' || r == '!' || r == '?' ||
					r == '，' || r == '。' || r == '；' || r == '！' || r == '？' {
					best = cursor
					break
				}
			}
			end = best
		}
		chunk := strings.TrimLeft(string(runes[start:end]), " ")
		if chunk != "" {
			chunks = append(chunks, chunk)
		}
		start = end
	}
	if len(chunks) == 0 {
		return []string{trimmed}
	}
	return chunks
}

func localizedAgentFallbackWarning(query string) string {
	lang := detectResponseLanguage(query)
	return localizedText(
		lang,
		"AI 代理模式执行失败，已自动切换为规则模式。",
		"Agent execution failed, automatically switched to rule mode.",
	)
}

func localizedStreamPhaseMessage(query, phase string) string {
	lang := detectResponseLanguage(query)
	switch strings.ToLower(strings.TrimSpace(phase)) {
	case "thinking":
		return localizedText(
			lang,
			"正在分析节点状态与问题意图...",
			"Analyzing node status and query intent...",
		)
	case "generating":
		return localizedText(
			lang,
			"正在生成回答...",
			"Generating answer...",
		)
	case "fallback":
		return localizedText(
			lang,
			"AI 代理暂不可用，正在切换到规则模式...",
			"Agent unavailable, switching to rule mode...",
		)
	case "fallback_stream":
		return localizedText(
			lang,
			"AI 流式代理暂不可用，正在切换到规则模式...",
			"Agent stream unavailable, switching to rule mode...",
		)
	default:
		return localizedText(
			lang,
			"正在处理请求...",
			"Processing request...",
		)
	}
}
