package ai

import "time"

const (
	// AIModeRule uses deterministic rules only.
	AIModeRule = "rule"
	// AIModeLLM is reserved for direct LLM response mode.
	AIModeLLM = "llm"
	// AIModeAgent uses tool-orchestrated agent flow.
	AIModeAgent = "agent"
)

const (
	// IntentUnknown means no reliable intent was detected.
	IntentUnknown IntentType = "unknown"
	// IntentNodeSummary asks for one node status summary.
	IntentNodeSummary IntentType = "node_summary"
	// IntentIdleNodeRanking asks for the most idle nodes.
	IntentIdleNodeRanking IntentType = "idle_node_ranking"
	// IntentScheduleSuggestion asks for scheduling recommendation.
	IntentScheduleSuggestion IntentType = "schedule_suggestion"
	// IntentAnomalyExplanation asks for explaining abnormal behavior.
	IntentAnomalyExplanation IntentType = "anomaly_explanation"
	// IntentAlertSummary asks for summarized alert history.
	IntentAlertSummary IntentType = "alert_summary"
	// IntentHistoryAnalysis asks for trend analysis in a time window.
	IntentHistoryAnalysis IntentType = "history_analysis"
)

// IntentType classifies what the user wants to ask.
type IntentType string

// AIQueryRequest is the request body of /api/ai/query.
type AIQueryRequest struct {
	Query  string `json:"query"`
	Stream bool   `json:"stream"`
}

// ToolCallRecord keeps simplified tool invocation metadata.
type ToolCallRecord struct {
	Name string         `json:"name"`
	Args map[string]any `json:"args"`
}

// AIQueryResponse is the stable response schema of AI query APIs.
type AIQueryResponse struct {
	Answer           string           `json:"answer"`
	ReasoningSummary string           `json:"reasoning_summary"`
	Mode             string           `json:"mode"`
	ToolCalls        []ToolCallRecord `json:"tool_calls,omitempty"`
	RelatedNodes     []string         `json:"related_nodes,omitempty"`
	Warnings         []string         `json:"warnings,omitempty"`
}

// QueryIntent is a structured intent recognized from user query.
type QueryIntent struct {
	Type        IntentType
	NodeName    string
	Window      time.Duration
	TopK        int
	Requirement JobRequirement
}

// JobRequirement represents scheduling constraints parsed from query.
type JobRequirement struct {
	MinFreeVRAMGB   float64 `json:"min_free_vram_gb"`
	GPUCount        int     `json:"gpu_count"`
	PreferLowCPU    bool    `json:"prefer_low_cpu"`
	PreferLowRAM    bool    `json:"prefer_low_ram"`
	PreferFewUsers  bool    `json:"prefer_fewer_users"`
	PreferFreshData bool    `json:"prefer_fresh_data"`
}

// NodeSnapshot is a normalized node state used by AI domain logic.
type NodeSnapshot struct {
	ID                int64          `json:"id"`
	Name              string         `json:"name"`
	URL               string         `json:"url"`
	Status            string         `json:"status"`
	LastPolledAtUnix  int64          `json:"last_polled_at_unix"`
	CollectedAtUnix   int64          `json:"collected_at_unix"`
	AvailabilityScore int            `json:"availability_score"`
	AvailabilityTier  string         `json:"availability_tier"`
	DataAgeSec        *float64       `json:"data_age_sec,omitempty"`
	CPUUsage          *float64       `json:"cpu_usage,omitempty"`
	RAMPercent        *float64       `json:"ram_percent,omitempty"`
	ActiveUsers       []string       `json:"active_users,omitempty"`
	ActiveUserCount   int            `json:"active_user_count"`
	GPUSummary        NodeGPUSummary `json:"gpu_summary"`
	GPUs              []GPUCard      `json:"gpus,omitempty"`
	Processes         []GPUProcess   `json:"processes,omitempty"`
	Error             string         `json:"error,omitempty"`
}

// NodeGPUSummary is a compact GPU summary for one node.
type NodeGPUSummary struct {
	GPUCount         int      `json:"gpu_count"`
	BusyGPUCount     int      `json:"busy_gpu_count"`
	IdleGPUCount     int      `json:"idle_gpu_count"`
	AvgUtilization   *float64 `json:"avg_utilization,omitempty"`
	AvgMemoryPercent *float64 `json:"avg_memory_percent,omitempty"`
	TotalMemoryGB    float64  `json:"total_memory_gb"`
	UsedMemoryGB     float64  `json:"used_memory_gb"`
	GPUPressure      float64  `json:"gpu_pressure"`
	BusyRatio        float64  `json:"busy_ratio"`
}

// GPUCard stores per-GPU stats used by scheduler and explanation logic.
type GPUCard struct {
	Index           int     `json:"index"`
	Name            string  `json:"name"`
	Utilization     float64 `json:"utilization"`
	MemoryTotalGB   float64 `json:"memory_total_gb"`
	MemoryUsedGB    float64 `json:"memory_used_gb"`
	MemoryPercent   float64 `json:"memory_percent"`
	EstimatedFreeGB float64 `json:"estimated_free_gb"`
}

// GPUProcess stores process-level resource usage.
type GPUProcess struct {
	PID           int     `json:"pid"`
	User          string  `json:"user"`
	Command       string  `json:"command"`
	GPUIndex      *int    `json:"gpu_index,omitempty"`
	VRAMUsedMB    *int    `json:"vram_used_mb,omitempty"`
	CPUPercent    float64 `json:"cpu_percent"`
	MemoryPercent float64 `json:"memory_percent"`
}

// NodeSummary is a readable per-node summary for final response generation.
type NodeSummary struct {
	NodeName          string   `json:"node_name"`
	Status            string   `json:"status"`
	DataAgeSec        *float64 `json:"data_age_sec,omitempty"`
	AvailabilityScore int      `json:"availability_score"`
	AvailabilityTier  string   `json:"availability_tier"`
	CPUUsage          *float64 `json:"cpu_usage,omitempty"`
	RAMPercent        *float64 `json:"ram_percent,omitempty"`
	GPUCount          int      `json:"gpu_count"`
	BusyGPUCount      int      `json:"busy_gpu_count"`
	IdleGPUCount      int      `json:"idle_gpu_count"`
	AvgGPUUtilization *float64 `json:"avg_gpu_utilization,omitempty"`
	AvgGPUMemoryPct   *float64 `json:"avg_gpu_memory_percent,omitempty"`
	TotalGPUMemoryGB  float64  `json:"total_gpu_memory_gb"`
	UsedGPUMemoryGB   float64  `json:"used_gpu_memory_gb"`
	ActiveUserCount   int      `json:"active_user_count"`
	TopProcess        string   `json:"top_process,omitempty"`
	HealthText        string   `json:"health_text"`
	GPUText           string   `json:"gpu_text"`
	ProcessText       string   `json:"process_text"`
	RiskFlags         []string `json:"risk_flags,omitempty"`
}

// NodeCandidate is one scheduling recommendation candidate.
type NodeCandidate struct {
	NodeName          string   `json:"node_name"`
	Score             float64  `json:"score"`
	AvailabilityScore int      `json:"availability_score"`
	IdleGPUCount      int      `json:"idle_gpu_count"`
	MaxFreeVRAMGB     float64  `json:"max_free_vram_gb"`
	QualifiedGPUCount int      `json:"qualified_gpu_count"`
	ActiveUserCount   int      `json:"active_user_count"`
	CPUUsage          float64  `json:"cpu_usage"`
	RAMPercent        float64  `json:"ram_percent"`
	DataAgeSec        float64  `json:"data_age_sec"`
	Reasons           []string `json:"reasons,omitempty"`
}

// AnomalyExplanation is deterministic explanation output for one node.
type AnomalyExplanation struct {
	NodeName       string   `json:"node_name"`
	Severity       string   `json:"severity"`
	Findings       []string `json:"findings"`
	PossibleCauses []string `json:"possible_causes"`
	Suggestions    []string `json:"suggestions"`
	Confidence     string   `json:"confidence"`
}

// NodeHistorySnapshot is the compact timeline record consumed by AI analysis.
type NodeHistorySnapshot struct {
	TimestampUnix     int64    `json:"timestamp_unix"`
	NodeName          string   `json:"node_name"`
	Status            string   `json:"status"`
	AvailabilityScore int      `json:"availability_score"`
	AvailabilityTier  string   `json:"availability_tier"`
	CPUUsage          *float64 `json:"cpu_usage,omitempty"`
	RAMPercent        *float64 `json:"ram_percent,omitempty"`
	GPUPressure       float64  `json:"gpu_pressure"`
	BusyRatio         float64  `json:"busy_ratio"`
	ActiveUserCount   int      `json:"active_user_count"`
	DataAgeSec        *float64 `json:"data_age_sec,omitempty"`
	RiskFlags         []string `json:"risk_flags,omitempty"`
}

// AlertSummary is one node's history/alert summary in a time window.
type AlertSummary struct {
	NodeName             string   `json:"node_name"`
	Severity             string   `json:"severity"`
	Summary              string   `json:"summary"`
	KeyFindings          []string `json:"key_findings,omitempty"`
	OfflineTransitions   int      `json:"offline_transitions"`
	AvgAvailabilityScore float64  `json:"avg_availability_score"`
	AvgGPUPressure       float64  `json:"avg_gpu_pressure"`
	AvgActiveUsers       float64  `json:"avg_active_users"`
	VolatilityScore      float64  `json:"volatility_score"`
}

// AlertHistoryResult is the tool output of alert/history analysis.
type AlertHistoryResult struct {
	WindowLabel string         `json:"window_label"`
	Summaries   []AlertSummary `json:"summaries"`
}

// CapabilitiesResponse exposes currently supported AI capabilities.
type CapabilitiesResponse struct {
	Enabled          bool     `json:"enabled"`
	DefaultMode      string   `json:"default_mode"`
	SupportedModes   []string `json:"supported_modes"`
	SupportedIntents []string `json:"supported_intents"`
	SupportedTools   []string `json:"supported_tools"`
}

// HealthResponse provides AI feature health information.
type HealthResponse struct {
	Status     string `json:"status"`
	Mode       string `json:"mode"`
	AgentReady bool   `json:"agent_ready"`
}
