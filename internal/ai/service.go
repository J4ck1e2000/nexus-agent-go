package ai

import (
	"context"
	"os"
	"strings"
	"time"
)

const (
	envAIEnabled  = "AI_ENABLED"
	envAIMode     = "AI_MODE"
	envAIProvider = "AI_PROVIDER"
	envAIModel    = "AI_MODEL"
	envAIAPIKey   = "AI_API_KEY"
	envAIBaseURL  = "AI_BASE_URL"

	defaultOpenAIBaseURL = "https://api.openai.com/v1"
	defaultQwenBaseURL   = "https://dashscope.aliyuncs.com/compatible-mode/v1"
)

// QueryExecutor executes one query request and returns normalized response.
type QueryExecutor interface {
	Execute(ctx context.Context, req AIQueryRequest) (AIQueryResponse, error)
}

// Config controls runtime behavior of AI service.
type Config struct {
	Enabled        bool
	Mode           string
	Provider       string
	Model          string
	APIKey         string
	BaseURL        string
	RequestTimeout time.Duration
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
		Enabled:        envBool(envAIEnabled, true),
		Mode:           strings.TrimSpace(os.Getenv(envAIMode)),
		Provider:       strings.TrimSpace(os.Getenv(envAIProvider)),
		Model:          strings.TrimSpace(os.Getenv(envAIModel)),
		APIKey:         strings.TrimSpace(os.Getenv(envAIAPIKey)),
		BaseURL:        strings.TrimSpace(os.Getenv(envAIBaseURL)),
		RequestTimeout: defaultOpenAICompatibleTimeout,
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
	return cfg
}

// Service is the facade for AI query APIs.
type Service struct {
	config        Config
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

	ruleExecutor := opts.RuleExecutor
	if ruleExecutor == nil {
		ruleExecutor = NewRuleExecutor(classifier, opts.Toolbox)
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
				Toolbox:      opts.Toolbox,
				SystemPrompt: opts.SystemPrompt,
				LLMClient:    llmClient,
				Model:        cfg.Model,
			},
		)
	}

	return &Service{
		config:        cfg,
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
			fallback.Warnings = appendIfMissing(fallback.Warnings, "Agent execution failed; fell back to deterministic rules.")
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

// Capabilities returns supported mode/intent/tool metadata.
func (s *Service) Capabilities(ctx context.Context) CapabilitiesResponse {
	_ = ctx
	return CapabilitiesResponse{
		Enabled:          s.config.Enabled,
		DefaultMode:      s.config.normalizedMode(),
		SupportedModes:   []string{AIModeRule, AIModeAgent},
		SupportedIntents: []string{string(IntentNodeSummary), string(IntentIdleNodeRanking), string(IntentScheduleSuggestion), string(IntentAnomalyExplanation), string(IntentAlertSummary), string(IntentHistoryAnalysis)},
		SupportedTools:   []string{"get_node_metrics", "list_idle_nodes", "get_gpu_processes", "get_node_summary", "get_alert_history", "recommend_nodes_for_job", "explain_node_anomaly"},
	}
}

// Health returns the current runtime health of AI service.
func (s *Service) Health(ctx context.Context) HealthResponse {
	_ = ctx
	status := "ok"
	if !s.config.Enabled {
		status = "disabled"
	}
	return HealthResponse{
		Status:     status,
		Mode:       s.config.normalizedMode(),
		AgentReady: s.config.AgentReady(),
	}
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
