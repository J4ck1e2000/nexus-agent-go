package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

const defaultAgentMaxRounds = 6

// EinoAgentExecutorOptions defines constructor options for the agent executor.
type EinoAgentExecutorOptions struct {
	Classifier   *IntentClassifier
	Toolbox      *Toolbox
	SystemPrompt string
	LLMClient    ChatCompletionClient
	Model        string
	MaxRounds    int
}

// EinoAgentExecutor uses OpenAI-compatible tool-calling flow.
type EinoAgentExecutor struct {
	classifier   *IntentClassifier
	toolbox      *Toolbox
	systemPrompt string
	llmClient    ChatCompletionClient
	model        string
	maxRounds    int
}

// NewEinoAgentExecutor creates an agent executor backed by an OpenAI-compatible client.
func NewEinoAgentExecutor(opts EinoAgentExecutorOptions) *EinoAgentExecutor {
	classifier := opts.Classifier
	if classifier == nil {
		classifier = NewIntentClassifier()
	}
	toolbox := opts.Toolbox
	if toolbox == nil {
		toolbox = NewToolbox(ToolboxOptions{})
	}
	systemPrompt := strings.TrimSpace(opts.SystemPrompt)
	if systemPrompt == "" {
		systemPrompt = DefaultSystemPrompt
	}
	maxRounds := opts.MaxRounds
	if maxRounds <= 0 {
		maxRounds = defaultAgentMaxRounds
	}

	return &EinoAgentExecutor{
		classifier:   classifier,
		toolbox:      toolbox,
		systemPrompt: systemPrompt,
		llmClient:    opts.LLMClient,
		model:        strings.TrimSpace(opts.Model),
		maxRounds:    maxRounds,
	}
}

// Execute runs constrained tool-calling and returns a unified response.
func (e *EinoAgentExecutor) Execute(ctx context.Context, req AIQueryRequest) (AIQueryResponse, error) {
	if e == nil || e.llmClient == nil || strings.TrimSpace(e.model) == "" {
		return AIQueryResponse{}, ErrAgentUnavailable
	}
	query := strings.TrimSpace(req.Query)
	if query == "" {
		return AIQueryResponse{}, ErrInvalidQuery
	}

	knownNodes, err := e.toolbox.KnownNodeNames(ctx)
	if err != nil {
		return AIQueryResponse{}, err
	}
	intent := e.classifier.Classify(query, knownNodes)

	messages := []ChatMessage{
		{Role: "system", Content: e.systemPrompt},
		{Role: "user", Content: buildAgentUserPrompt(query, intent, knownNodes)},
	}

	toolCalls := make([]ToolCallRecord, 0, 8)
	relatedNodeSet := map[string]struct{}{}
	warnings := make([]string, 0, 4)
	finalContent := ""

	for round := 0; round < e.maxRounds; round++ {
		resp, err := e.llmClient.CreateChatCompletion(ctx, ChatCompletionRequest{
			Model:       e.model,
			Messages:    messages,
			Tools:       buildAgentToolSchemas(),
			ToolChoice:  "auto",
			Temperature: 0.1,
		})
		if err != nil {
			return AIQueryResponse{}, err
		}
		if len(resp.Choices) == 0 {
			return AIQueryResponse{}, fmt.Errorf("agent returned no choices")
		}

		msg := resp.Choices[0].Message
		if len(msg.ToolCalls) == 0 {
			finalContent = strings.TrimSpace(msg.Content)
			break
		}

		messages = append(messages, ChatMessage{
			Role:      "assistant",
			Content:   msg.Content,
			ToolCalls: msg.ToolCalls,
		})

		for _, call := range msg.ToolCalls {
			toolOutput, record, relatedNodes, callWarnings := e.executeToolCall(ctx, call)
			if record.Name != "" {
				toolCalls = append(toolCalls, record)
			}
			for _, nodeName := range relatedNodes {
				normalized := strings.TrimSpace(nodeName)
				if normalized == "" {
					continue
				}
				relatedNodeSet[normalized] = struct{}{}
			}
			warnings = append(warnings, callWarnings...)

			messages = append(messages, ChatMessage{
				Role:       "tool",
				Name:       call.Function.Name,
				ToolCallID: call.ID,
				Content:    toolOutput,
			})
		}
	}

	if strings.TrimSpace(finalContent) == "" {
		return AIQueryResponse{}, fmt.Errorf("agent did not produce final response in %d rounds", e.maxRounds)
	}

	modelFinal := parseAgentFinalContent(finalContent)
	if strings.TrimSpace(modelFinal.Answer) == "" {
		modelFinal.Answer = finalContent
	}
	if strings.TrimSpace(modelFinal.ReasoningSummary) == "" {
		modelFinal.ReasoningSummary = summarizeToolCalls(toolCalls)
	}

	for _, nodeName := range modelFinal.RelatedNodes {
		normalized := strings.TrimSpace(nodeName)
		if normalized == "" {
			continue
		}
		relatedNodeSet[normalized] = struct{}{}
	}

	warnings = append(warnings, modelFinal.Warnings...)
	warnings = uniqueStrings(warnings)

	relatedNodes := make([]string, 0, len(relatedNodeSet))
	for nodeName := range relatedNodeSet {
		relatedNodes = append(relatedNodes, nodeName)
	}
	sort.Strings(relatedNodes)

	return AIQueryResponse{
		Answer:           strings.TrimSpace(modelFinal.Answer),
		ReasoningSummary: strings.TrimSpace(modelFinal.ReasoningSummary),
		Mode:             AIModeAgent,
		ToolCalls:        toolCalls,
		RelatedNodes:     relatedNodes,
		Warnings:         warnings,
	}, nil
}

type parsedAgentFinal struct {
	Answer           string   `json:"answer"`
	ReasoningSummary string   `json:"reasoning_summary"`
	RelatedNodes     []string `json:"related_nodes"`
	Warnings         []string `json:"warnings"`
}

func parseAgentFinalContent(content string) parsedAgentFinal {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return parsedAgentFinal{}
	}

	var parsed parsedAgentFinal
	if err := json.Unmarshal([]byte(trimmed), &parsed); err == nil {
		return parsed
	}

	start := strings.Index(trimmed, "{")
	end := strings.LastIndex(trimmed, "}")
	if start >= 0 && end > start {
		chunk := trimmed[start : end+1]
		if err := json.Unmarshal([]byte(chunk), &parsed); err == nil {
			return parsed
		}
	}

	return parsedAgentFinal{Answer: trimmed}
}

func summarizeToolCalls(toolCalls []ToolCallRecord) string {
	if len(toolCalls) == 0 {
		return "Generated by agent mode without explicit tool calls."
	}
	names := make([]string, 0, len(toolCalls))
	for _, call := range toolCalls {
		names = append(names, call.Name)
	}
	return "Generated from tool evidence: " + strings.Join(names, ", ") + "."
}

func buildAgentUserPrompt(query string, intent QueryIntent, knownNodes []string) string {
	intentJSON, _ := json.Marshal(map[string]any{
		"type":         intent.Type,
		"node_name":    intent.NodeName,
		"window":       intent.Window.String(),
		"top_k":        intent.TopK,
		"requirements": intent.Requirement,
	})

	nodesJSON, _ := json.Marshal(knownNodes)

	return strings.TrimSpace(fmt.Sprintf(`
User query:
%s

Detected intent hint (from deterministic classifier):
%s

Known node names:
%s

Please decide whether to call tools. Every conclusion must be grounded in tool outputs.
Your final response MUST be strict JSON (no markdown):
{"answer":"...","reasoning_summary":"...","related_nodes":["..."],"warnings":["..."]}
`, query, string(intentJSON), string(nodesJSON)))
}

func (e *EinoAgentExecutor) executeToolCall(ctx context.Context, call ChatToolCall) (string, ToolCallRecord, []string, []string) {
	args := parseToolArgs(call.Function.Arguments)
	record := ToolCallRecord{Name: call.Function.Name, Args: args}
	warnings := make([]string, 0, 1)

	fail := func(err error) (string, ToolCallRecord, []string, []string) {
		warning := fmt.Sprintf("tool %s failed: %v", call.Function.Name, err)
		warnings = append(warnings, warning)
		result := map[string]any{"error": err.Error()}
		return marshalToolOutput(result), record, nil, warnings
	}

	switch call.Function.Name {
	case "get_node_metrics":
		nodeName := asString(args["node_name"])
		result, err := e.toolbox.GetNodeMetrics(ctx, nodeName)
		if err != nil {
			return fail(err)
		}
		return marshalToolOutput(result), record, []string{nodeName}, warnings

	case "list_idle_nodes":
		limit := asInt(args["limit"], 3)
		minFree, hasMin := asFloatPtr(args["min_free_vram_gb"])
		var ptr *float64
		if hasMin {
			ptr = &minFree
		}
		result, err := e.toolbox.ListIdleNodes(ctx, ptr, limit)
		if err != nil {
			return fail(err)
		}
		return marshalToolOutput(result), record, collectNodeNames(result), warnings

	case "get_gpu_processes":
		nodeName := asString(args["node_name"])
		result, err := e.toolbox.GetGPUProcesses(ctx, nodeName)
		if err != nil {
			return fail(err)
		}
		return marshalToolOutput(result), record, []string{nodeName}, warnings

	case "get_node_summary":
		nodeName := asString(args["node_name"])
		result, err := e.toolbox.GetNodeSummary(ctx, nodeName)
		if err != nil {
			return fail(err)
		}
		return marshalToolOutput(result), record, []string{nodeName}, warnings

	case "get_alert_history":
		window := asStringWithDefault(args["window"], "30m")
		nodeNameStr := asString(args["node_name"])
		var nodeNamePtr *string
		if nodeNameStr != "" {
			nodeNamePtr = &nodeNameStr
		}
		result, err := e.toolbox.GetAlertHistory(ctx, nodeNamePtr, window)
		if err != nil {
			return fail(err)
		}
		return marshalToolOutput(result), record, collectAlertNodeNames(result.Summaries), warnings

	case "recommend_nodes_for_job":
		requirements := parseJobRequirementArgs(args["requirements"])
		limit := asInt(args["limit"], 3)
		result, err := e.toolbox.RecommendNodesForJob(ctx, requirements, limit)
		if err != nil {
			return fail(err)
		}
		return marshalToolOutput(result), record, collectNodeNames(result), warnings

	case "explain_node_anomaly":
		nodeName := asString(args["node_name"])
		result, err := e.toolbox.ExplainNodeAnomaly(ctx, nodeName)
		if err != nil {
			return fail(err)
		}
		return marshalToolOutput(result), record, []string{nodeName}, warnings
	default:
		return fail(fmt.Errorf("unsupported tool: %s", call.Function.Name))
	}
}

func parseToolArgs(raw string) map[string]any {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return map[string]any{}
	}
	var args map[string]any
	if err := json.Unmarshal([]byte(trimmed), &args); err != nil {
		return map[string]any{}
	}
	if args == nil {
		return map[string]any{}
	}
	return args
}

func parseJobRequirementArgs(value any) JobRequirement {
	req := JobRequirement{PreferFreshData: true}
	obj, ok := value.(map[string]any)
	if !ok {
		return req
	}

	if v, ok := asFloatPtr(obj["min_free_vram_gb"]); ok {
		req.MinFreeVRAMGB = v
	}
	req.GPUCount = asInt(obj["gpu_count"], 0)
	req.PreferLowCPU = asBool(obj["prefer_low_cpu"])
	req.PreferLowRAM = asBool(obj["prefer_low_ram"])
	req.PreferFewUsers = asBool(obj["prefer_fewer_users"])
	if _, exists := obj["prefer_fresh_data"]; exists {
		req.PreferFreshData = asBool(obj["prefer_fresh_data"])
	}
	return req
}

func asString(value any) string {
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v)
	case json.Number:
		return strings.TrimSpace(v.String())
	case float64:
		return strings.TrimSpace(strconv.FormatFloat(v, 'f', -1, 64))
	default:
		return ""
	}
}

func asStringWithDefault(value any, fallback string) string {
	v := asString(value)
	if v == "" {
		return fallback
	}
	return v
}

func asInt(value any, fallback int) int {
	switch v := value.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	case json.Number:
		if parsed, err := v.Int64(); err == nil {
			return int(parsed)
		}
	case string:
		if parsed, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			return parsed
		}
	}
	return fallback
}

func asFloatPtr(value any) (float64, bool) {
	switch v := value.(type) {
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case json.Number:
		if parsed, err := v.Float64(); err == nil {
			return parsed, true
		}
	case string:
		if parsed, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil {
			return parsed, true
		}
	}
	return 0, false
}

func asBool(value any) bool {
	switch v := value.(type) {
	case bool:
		return v
	case string:
		lower := strings.ToLower(strings.TrimSpace(v))
		return lower == "1" || lower == "true" || lower == "yes" || lower == "on"
	case float64:
		return v != 0
	case int:
		return v != 0
	default:
		return false
	}
}

func marshalToolOutput(value any) string {
	raw, err := json.Marshal(value)
	if err != nil {
		fallback, _ := json.Marshal(map[string]any{"error": err.Error()})
		return string(fallback)
	}
	return string(raw)
}

func uniqueStrings(input []string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(input))
	for _, item := range input {
		normalized := strings.TrimSpace(item)
		if normalized == "" {
			continue
		}
		if _, exists := seen[normalized]; exists {
			continue
		}
		seen[normalized] = struct{}{}
		result = append(result, normalized)
	}
	return result
}

func buildAgentToolSchemas() []ChatTool {
	return []ChatTool{
		{
			Type: "function",
			Function: ChatToolFunctionSchema{
				Name:        "get_node_metrics",
				Description: "Get full current metrics snapshot of one node from gateway aggregate view.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"node_name": map[string]any{"type": "string"},
					},
					"required": []string{"node_name"},
				},
			},
		},
		{
			Type: "function",
			Function: ChatToolFunctionSchema{
				Name:        "list_idle_nodes",
				Description: "List currently available nodes ranked by idle/scheduling score.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"min_free_vram_gb": map[string]any{"type": "number"},
						"limit":            map[string]any{"type": "integer", "minimum": 1, "maximum": 10},
					},
				},
			},
		},
		{
			Type: "function",
			Function: ChatToolFunctionSchema{
				Name:        "get_gpu_processes",
				Description: "Get GPU process list of a node.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"node_name": map[string]any{"type": "string"},
					},
					"required": []string{"node_name"},
				},
			},
		},
		{
			Type: "function",
			Function: ChatToolFunctionSchema{
				Name:        "get_node_summary",
				Description: "Get readable node summary including health/gpu/process/risk flags.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"node_name": map[string]any{"type": "string"},
					},
					"required": []string{"node_name"},
				},
			},
		},
		{
			Type: "function",
			Function: ChatToolFunctionSchema{
				Name:        "get_alert_history",
				Description: "Get alert/trend summary for a node or all nodes in a window like 30m/1h.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"node_name": map[string]any{"type": "string"},
						"window":    map[string]any{"type": "string", "enum": []string{"30m", "1h"}},
					},
				},
			},
		},
		{
			Type: "function",
			Function: ChatToolFunctionSchema{
				Name:        "recommend_nodes_for_job",
				Description: "Recommend nodes for a job requirement.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"requirements": map[string]any{
							"type": "object",
							"properties": map[string]any{
								"min_free_vram_gb":   map[string]any{"type": "number"},
								"gpu_count":          map[string]any{"type": "integer"},
								"prefer_low_cpu":     map[string]any{"type": "boolean"},
								"prefer_low_ram":     map[string]any{"type": "boolean"},
								"prefer_fewer_users": map[string]any{"type": "boolean"},
							},
						},
						"limit": map[string]any{"type": "integer", "minimum": 1, "maximum": 10},
					},
				},
			},
		},
		{
			Type: "function",
			Function: ChatToolFunctionSchema{
				Name:        "explain_node_anomaly",
				Description: "Generate deterministic anomaly explanation for one node.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"node_name": map[string]any{"type": "string"},
					},
					"required": []string{"node_name"},
				},
			},
		},
	}
}
