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
	return e.execute(ctx, req, nil)
}

// ExecuteStream runs constrained tool-calling and emits safe status events.
func (e *EinoAgentExecutor) ExecuteStream(ctx context.Context, req AIQueryRequest, emit func(AIStreamEvent) error) (AIQueryResponse, error) {
	return e.execute(ctx, req, emit)
}

func (e *EinoAgentExecutor) execute(ctx context.Context, req AIQueryRequest, emit func(AIStreamEvent) error) (AIQueryResponse, error) {
	if e == nil || e.llmClient == nil || strings.TrimSpace(e.model) == "" {
		return AIQueryResponse{}, ErrAgentUnavailable
	}
	query := strings.TrimSpace(req.Query)
	if query == "" {
		return AIQueryResponse{}, ErrInvalidQuery
	}
	lang := detectResponseLanguage(query)

	knownNodes, err := e.toolbox.KnownNodeNames(ctx)
	if err != nil {
		return AIQueryResponse{}, err
	}
	intent := e.classifier.Classify(query, knownNodes)
	if err := emitAgentStatus(emit, "thinking", localizedText(
		lang,
		"濮濓絽婀崚鍡樼€介懞鍌滃仯閻樿埖鈧礁鑻熼柅澶嬪閸氬牓鈧倸浼愰崗?..",
		"Analyzing node status and selecting tools...",
	)); err != nil {
		return AIQueryResponse{}, err
	}

	messages := []ChatMessage{
		{Role: "system", Content: e.systemPrompt},
		{Role: "user", Content: buildAgentUserPrompt(query, intent, knownNodes, lang)},
	}

	toolCalls := make([]ToolCallRecord, 0, 8)
	relatedNodeSet := map[string]struct{}{}
	warnings := make([]string, 0, 4)
	knowledgeHits := make([]KnowledgeHitSummary, 0, 4)
	var retrievalMeta *RetrievalMeta
	finalContent := ""

	for round := 0; round < e.maxRounds; round++ {
		if err := emitAgentStatus(emit, "thinking", localizedText(
			lang,
			"濮濓絽婀Ч鍥ㄢ偓璇插讲閻劌浼愰崗鐤箲閸ョ偟娈戠拠浣瑰祦...",
			"Gathering evidence from available tools...",
		)); err != nil {
			return AIQueryResponse{}, err
		}
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
			if err := emitAgentStatus(emit, "tooling", toolStatusMessage(call.Function.Name, lang)); err != nil {
				return AIQueryResponse{}, err
			}
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

			if strings.EqualFold(strings.TrimSpace(call.Function.Name), "search_knowledge_base") {
				hits, retrieval := parseKnowledgeToolOutput(toolOutput)
				if len(hits) > 0 {
					knowledgeHits = dedupeKnowledgeHitSummaries(append(knowledgeHits, hits...))
				}
				if retrieval != nil {
					retrievalMeta = retrieval
				}
			}
		}
	}

	if strings.TrimSpace(finalContent) == "" {
		return AIQueryResponse{}, fmt.Errorf("agent did not produce final response in %d rounds", e.maxRounds)
	}

	modelFinal := parseAgentFinalContent(finalContent)
	if strings.TrimSpace(modelFinal.Answer) == "" {
		modelFinal.Answer = sanitizeFinalText(finalContent)
	}
	if strings.TrimSpace(modelFinal.ReasoningSummary) == "" {
		modelFinal.ReasoningSummary = summarizeToolCalls(toolCalls, lang)
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
	if len(modelFinal.KnowledgeHits) > 0 {
		knowledgeHits = dedupeKnowledgeHitSummaries(append(knowledgeHits, modelFinal.KnowledgeHits...))
	}
	if modelFinal.Retrieval != nil {
		retrievalMeta = modelFinal.Retrieval
	}

	relatedNodes := make([]string, 0, len(relatedNodeSet))
	for nodeName := range relatedNodeSet {
		relatedNodes = append(relatedNodes, nodeName)
	}
	sort.Strings(relatedNodes)

	answer := rewriteOperationalAnswer(intent, sanitizeFinalText(modelFinal.Answer), relatedNodes, lang)
	reasoningSummary := rewriteReasoningSummary(intent, sanitizeFinalText(modelFinal.ReasoningSummary), relatedNodes, lang)
	if strings.TrimSpace(reasoningSummary) == "" {
		reasoningSummary = summarizeToolCalls(toolCalls, lang)
	}

	return AIQueryResponse{
		Answer:           answer,
		ReasoningSummary: reasoningSummary,
		Mode:             AIModeAgent,
		ToolCalls:        toolCalls,
		RelatedNodes:     relatedNodes,
		Warnings:         warnings,
		KnowledgeHits:    knowledgeHits,
		Retrieval:        retrievalMeta,
	}, nil
}

type parsedAgentFinal struct {
	Answer           string                `json:"answer"`
	ReasoningSummary string                `json:"reasoning_summary"`
	RelatedNodes     []string              `json:"related_nodes"`
	Warnings         []string              `json:"warnings"`
	KnowledgeHits    []KnowledgeHitSummary `json:"knowledge_hits"`
	Retrieval        *RetrievalMeta        `json:"retrieval,omitempty"`
}

func parseAgentFinalContent(content string) parsedAgentFinal {
	parsed, ok := parseAgentFinalJSON(content, 0)
	if ok {
		if strings.TrimSpace(parsed.Answer) == "" {
			parsed.Answer = sanitizeFinalText(content)
		}
		parsed.Answer = sanitizeFinalText(parsed.Answer)
		parsed.ReasoningSummary = sanitizeFinalText(parsed.ReasoningSummary)
		return parsed
	}

	fallback := sanitizeFinalText(content)
	if fallback == "" {
		return parsedAgentFinal{}
	}
	return parsedAgentFinal{Answer: fallback}
}

func parseAgentFinalJSON(raw string, depth int) (parsedAgentFinal, bool) {
	if depth > 4 {
		return parsedAgentFinal{}, false
	}

	trimmed := sanitizeFinalText(raw)
	if trimmed == "" {
		return parsedAgentFinal{}, false
	}

	var parsed parsedAgentFinal
	if err := json.Unmarshal([]byte(trimmed), &parsed); err == nil {
		return normalizeParsedAgentFinal(parsed, depth+1), true
	}

	var asQuotedString string
	if err := json.Unmarshal([]byte(trimmed), &asQuotedString); err == nil {
		return parseAgentFinalJSON(asQuotedString, depth+1)
	}

	var generic map[string]any
	if err := json.Unmarshal([]byte(trimmed), &generic); err == nil {
		parsed = parsedAgentFinal{
			Answer:           asString(generic["answer"]),
			ReasoningSummary: asString(generic["reasoning_summary"]),
			RelatedNodes:     asStringSlice(generic["related_nodes"]),
			Warnings:         asStringSlice(generic["warnings"]),
			KnowledgeHits:    asKnowledgeHitSummarySlice(generic["knowledge_hits"]),
			Retrieval:        asRetrievalMeta(generic["retrieval"]),
		}
		if parsed.Answer != "" || parsed.ReasoningSummary != "" || len(parsed.RelatedNodes) > 0 || len(parsed.Warnings) > 0 || len(parsed.KnowledgeHits) > 0 || parsed.Retrieval != nil {
			return normalizeParsedAgentFinal(parsed, depth+1), true
		}
	}

	start := strings.Index(trimmed, "{")
	end := strings.LastIndex(trimmed, "}")
	if start >= 0 && end > start {
		chunk := trimmed[start : end+1]
		if parsed, ok := parseAgentFinalJSON(chunk, depth+1); ok {
			return parsed, true
		}
	}
	return parsedAgentFinal{}, false
}

func normalizeParsedAgentFinal(parsed parsedAgentFinal, depth int) parsedAgentFinal {
	result := parsedAgentFinal{
		Answer:           sanitizeFinalText(parsed.Answer),
		ReasoningSummary: sanitizeFinalText(parsed.ReasoningSummary),
		RelatedNodes:     uniqueStrings(parsed.RelatedNodes),
		Warnings:         uniqueStrings(parsed.Warnings),
		KnowledgeHits:    dedupeKnowledgeHitSummaries(parsed.KnowledgeHits),
		Retrieval:        parsed.Retrieval,
	}

	if nested, ok := parseAgentFinalJSON(result.Answer, depth+1); ok && strings.TrimSpace(nested.Answer) != "" {
		result.Answer = sanitizeFinalText(nested.Answer)
		if strings.TrimSpace(result.ReasoningSummary) == "" {
			result.ReasoningSummary = sanitizeFinalText(nested.ReasoningSummary)
		}
		if len(result.RelatedNodes) == 0 && len(nested.RelatedNodes) > 0 {
			result.RelatedNodes = uniqueStrings(nested.RelatedNodes)
		}
		if len(nested.Warnings) > 0 {
			result.Warnings = uniqueStrings(append(result.Warnings, nested.Warnings...))
		}
		if len(result.KnowledgeHits) == 0 && len(nested.KnowledgeHits) > 0 {
			result.KnowledgeHits = dedupeKnowledgeHitSummaries(nested.KnowledgeHits)
		}
		if result.Retrieval == nil && nested.Retrieval != nil {
			result.Retrieval = nested.Retrieval
		}
	}
	return result
}

func sanitizeFinalText(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}

	if strings.HasPrefix(trimmed, "```") {
		lines := strings.Split(trimmed, "\n")
		if len(lines) >= 2 {
			lines = lines[1:]
			if len(lines) > 0 {
				last := strings.TrimSpace(lines[len(lines)-1])
				if strings.HasPrefix(last, "```") {
					lines = lines[:len(lines)-1]
				}
			}
			trimmed = strings.TrimSpace(strings.Join(lines, "\n"))
		}
	}

	trimmed = strings.ReplaceAll(trimmed, "```json", "")
	trimmed = strings.ReplaceAll(trimmed, "```", "")
	trimmed = strings.TrimSpace(trimmed)

	var asQuotedString string
	if err := json.Unmarshal([]byte(trimmed), &asQuotedString); err == nil {
		return sanitizeFinalText(asQuotedString)
	}
	return trimmed
}

func summarizeToolCalls(toolCalls []ToolCallRecord, lang responseLanguage) string {
	if len(toolCalls) == 0 {
		return localizedText(
			lang,
			"本次回答由 agent 直接生成，未触发显式工具调用。",
			"Generated by agent mode without explicit tool calls.",
		)
	}
	names := make([]string, 0, len(toolCalls))
	for _, call := range toolCalls {
		names = append(names, call.Name)
	}
	return localizedText(
		lang,
		"回答依据的工具证据包括："+strings.Join(names, "、")+"。",
		"Generated from tool evidence: "+strings.Join(names, ", ")+".",
	)
}
func rewriteOperationalAnswer(intent QueryIntent, current string, relatedNodes []string, lang responseLanguage) string {
	text := sanitizeFinalText(current)
	if responseLanguageMatches(lang, text) {
		return text
	}

	switch intent.Type {
	case IntentScheduleSuggestion:
		nodes := uniqueStrings(relatedNodes)
		if len(nodes) == 0 {
			return text
		}
		requested := intent.TopK
		if requested <= 0 {
			requested = 1
		}
		if requested > len(nodes) {
			requested = len(nodes)
		}

		primary := nodes[0]
		if requested == 1 {
			answer := localizedText(
				lang,
				fmt.Sprintf("结论：%s 当前相对空闲，可以先投放 1 个中等负载任务；如果是大任务，建议先观测 5 分钟再扩容。", primary),
				fmt.Sprintf("Conclusion: %s is relatively idle now. Start with one medium-load job, then observe for 5 minutes before scaling.", primary),
			)
			if intent.Requirement.MinFreeVRAMGB > 0 {
				answer += localizedText(
					lang,
					fmt.Sprintf(" 当前需求约 %.0fGB 显存，请先确认该节点剩余显存可持续满足。", intent.Requirement.MinFreeVRAMGB),
					fmt.Sprintf(" Required VRAM is about %.0fGB, so verify sustained free VRAM before placing more jobs.", intent.Requirement.MinFreeVRAMGB),
				)
			}
			return answer
		}

		secondary := nodes[1]
		answer := localizedText(
			lang,
			fmt.Sprintf("结论：建议优先使用 %s 和 %s。第一批任务先放在 %s，%s 作为备选；若是大任务，先观察 5 分钟再扩容。", primary, secondary, primary, secondary),
			fmt.Sprintf("Prefer %s and %s first. Start the first batch on %s, keep %s as backup; for larger jobs, observe for 5 minutes before scaling.", primary, secondary, primary, secondary),
		)
		if intent.Requirement.MinFreeVRAMGB > 0 {
			answer += localizedText(
				lang,
				fmt.Sprintf(" 你当前任务约需要 %.0fGB 显存，建议优先检查这两台机器的可用显存是否稳定。", intent.Requirement.MinFreeVRAMGB),
				fmt.Sprintf(" Your workload needs about %.0fGB VRAM, so confirm stable free VRAM on both nodes.", intent.Requirement.MinFreeVRAMGB),
			)
		}
		return answer

	case IntentIdleNodeRanking:
		nodes := uniqueStrings(relatedNodes)
		if len(nodes) == 0 {
			return text
		}
		if len(nodes) == 1 {
			return localizedText(
				lang,
				fmt.Sprintf("结论：当前最空闲的节点是 %s，建议优先把新任务放到这台机器。", nodes[0]),
				fmt.Sprintf("%s is currently the most idle node. Start new workloads there first.", nodes[0]),
			)
		}
		return localizedText(
			lang,
			"结论：当前空闲优先顺序为 "+strings.Join(nodes[:minInt(len(nodes), 3)], "、")+"，建议按这个顺序尝试调度。",
			"Current preferred nodes: "+strings.Join(nodes[:minInt(len(nodes), 3)], ", ")+".",
		)
	default:
		return text
	}
}
func rewriteReasoningSummary(intent QueryIntent, current string, relatedNodes []string, lang responseLanguage) string {
	summary := sanitizeFinalText(current)
	if responseLanguageMatches(lang, summary) {
		return summary
	}

	switch intent.Type {
	case IntentScheduleSuggestion, IntentIdleNodeRanking:
		nodes := uniqueStrings(relatedNodes)
		if len(nodes) == 0 {
			if summary != "" {
				return summary
			}
			return localizedText(
				lang,
				"判断依据：资源余量与数据新鲜度。",
				"Based on resource headroom and data freshness.",
			)
		}
		return localizedText(
			lang,
			"判断依据：综合了 CPU、内存、GPU 余量以及数据新鲜度；当前优先顺序为 "+strings.Join(nodes[:minInt(len(nodes), 3)], "、")+"。",
			"Based on CPU/RAM/GPU headroom and data freshness; current priority order: "+strings.Join(nodes[:minInt(len(nodes), 3)], ", ")+".",
		)
	default:
		return summary
	}
}

func responseLanguageMatches(lang responseLanguage, text string) bool {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return false
	}
	hasHan := false
	for _, r := range trimmed {
		if r >= 0x4E00 && r <= 0x9FFF {
			hasHan = true
			break
		}
	}
	if lang == responseLanguageZH {
		return hasHan
	}
	return !hasHan
}
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func buildAgentUserPrompt(query string, intent QueryIntent, knownNodes []string, lang responseLanguage) string {
	intentJSON, _ := json.Marshal(map[string]any{
		"type":         intent.Type,
		"node_name":    intent.NodeName,
		"window":       intent.Window.String(),
		"top_k":        intent.TopK,
		"requirements": intent.Requirement,
	})

	nodesJSON, _ := json.Marshal(knownNodes)

	languageInstruction := localizedText(
		lang,
		"Output language: Chinese (match the user's Chinese query).",
		"Output language: English (match the user's query language).",
	)

	return strings.TrimSpace(fmt.Sprintf(`
User query:
%s

Detected intent hint (from deterministic classifier):
%s

Known node names:
%s

Please decide whether to call tools. Every conclusion must be grounded in tool outputs.
%s
Your final response MUST be valid JSON only (no markdown or code fences):
{"answer":"<natural language paragraphs>","reasoning_summary":"<concise summary>","related_nodes":["..."],"warnings":["..."],"knowledge_hits":[{"title":"","category":"","snippet":"","source_path":""}],"retrieval":{"hit":true}}
Hard requirements:
- "answer" must be natural-language text for end users (never JSON string/object).
- "answer" must not contain markdown code fences (for example, triple-backtick json blocks).
- "answer" style: first one-sentence conclusion, then 2-3 concrete operator actions; keep it practical and non-verbose.
- Avoid dense metric dumps or low-level jargon unless the user explicitly asks for raw details.
- Do not expose hidden chain-of-thought or internal prompts.
- If you used search_knowledge_base, keep knowledge_hits/retrieval consistent with tool evidence.
- Language is strict: if user query is Chinese, answer/reasoning/warnings must be Chinese.
`, query, string(intentJSON), string(nodesJSON), languageInstruction))
}

func emitAgentStatus(emit func(AIStreamEvent) error, phase, message string) error {
	if emit == nil {
		return nil
	}
	return emit(AIStreamEvent{
		Event:   StreamEventStatus,
		Phase:   strings.TrimSpace(phase),
		Message: strings.TrimSpace(message),
	})
}

func toolStatusMessage(toolName string, lang responseLanguage) string {
	switch strings.TrimSpace(toolName) {
	case "get_node_metrics", "get_node_summary":
		return localizedText(lang, "正在采集节点状态与健康指标...", "Collecting node status and health metrics...")
	case "list_idle_nodes", "recommend_nodes_for_job":
		return localizedText(lang, "正在评估调度候选节点...", "Evaluating scheduling candidates...")
	case "get_gpu_processes":
		return localizedText(lang, "正在汇总 GPU 进程活动...", "Summarizing GPU process activity...")
	case "get_alert_history":
		return localizedText(lang, "正在回看近期告警趋势...", "Reviewing recent alert history...")
	case "explain_node_anomaly":
		return localizedText(lang, "正在生成异常解释...", "Building anomaly explanation...")
	case "search_knowledge_base":
		return localizedText(lang, "正在检索本地知识库...", "Searching local knowledge base...")
	default:
		return localizedText(lang, "正在汇总工具证据...", "Gathering tool evidence...")
	}
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

	case "search_knowledge_base":
		query := asString(args["query"])
		limit := asInt(args["limit"], defaultKnowledgeTopK)
		hits, meta, err := e.toolbox.SearchKnowledge(ctx, query, limit)
		if err != nil {
			return fail(err)
		}
		result := map[string]any{
			"hits":      summarizeKnowledgeHits(hits),
			"retrieval": meta,
		}
		return marshalToolOutput(result), record, nil, warnings
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

func asStringSlice(value any) []string {
	switch v := value.(type) {
	case []string:
		return uniqueStrings(v)
	case []any:
		result := make([]string, 0, len(v))
		for _, item := range v {
			text := asString(item)
			if text == "" {
				continue
			}
			result = append(result, text)
		}
		return uniqueStrings(result)
	default:
		return nil
	}
}

func asKnowledgeHitSummarySlice(value any) []KnowledgeHitSummary {
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	result := make([]KnowledgeHitSummary, 0, len(items))
	for _, item := range items {
		obj, ok := item.(map[string]any)
		if !ok {
			continue
		}
		hit := KnowledgeHitSummary{
			Title:      asString(obj["title"]),
			Category:   asString(obj["category"]),
			Snippet:    asString(obj["snippet"]),
			SourcePath: asString(obj["source_path"]),
		}
		if strings.TrimSpace(hit.Title) == "" {
			continue
		}
		result = append(result, hit)
	}
	return dedupeKnowledgeHitSummaries(result)
}

func asRetrievalMeta(value any) *RetrievalMeta {
	obj, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	meta := RetrievalMeta{
		Query:          asString(obj["query"]),
		TopK:           asInt(obj["top_k"], 0),
		CandidateCount: asInt(obj["candidate_count"], 0),
		ReturnedCount:  asInt(obj["returned_count"], 0),
		Hit:            asBool(obj["hit"]),
		DurationMs:     int64(asInt(obj["duration_ms"], 0)),
		Strategy:       asString(obj["strategy"]),
	}
	if score, ok := asFloatPtr(obj["top_score"]); ok {
		meta.TopScore = score
	}
	return &meta
}

func parseKnowledgeToolOutput(raw string) ([]KnowledgeHitSummary, *RetrievalMeta) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, nil
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(trimmed), &payload); err != nil {
		return nil, nil
	}
	return asKnowledgeHitSummarySlice(payload["hits"]), asRetrievalMeta(payload["retrieval"])
}

func dedupeKnowledgeHitSummaries(input []KnowledgeHitSummary) []KnowledgeHitSummary {
	if len(input) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	result := make([]KnowledgeHitSummary, 0, len(input))
	for _, item := range input {
		title := strings.TrimSpace(item.Title)
		source := strings.TrimSpace(item.SourcePath)
		if title == "" {
			continue
		}
		key := strings.ToLower(title + "|" + source)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, item)
	}
	return result
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
		{
			Type: "function",
			Function: ChatToolFunctionSchema{
				Name:        "search_knowledge_base",
				Description: "Search local troubleshooting knowledge for common causes and remediation steps.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"query": map[string]any{"type": "string"},
						"limit": map[string]any{"type": "integer", "minimum": 1, "maximum": 10},
					},
					"required": []string{"query"},
				},
			},
		},
	}
}
