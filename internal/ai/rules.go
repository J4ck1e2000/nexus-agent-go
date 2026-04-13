package ai

import (
	"context"
	"fmt"
	"strings"
)

// RuleExecutor answers AI queries with deterministic rule/tool logic.
type RuleExecutor struct {
	classifier *IntentClassifier
	toolbox    *Toolbox
}

// NewRuleExecutor creates the pure-rule query executor.
func NewRuleExecutor(classifier *IntentClassifier, toolbox *Toolbox) *RuleExecutor {
	if classifier == nil {
		classifier = NewIntentClassifier()
	}
	if toolbox == nil {
		toolbox = NewToolbox(ToolboxOptions{})
	}
	return &RuleExecutor{
		classifier: classifier,
		toolbox:    toolbox,
	}
}

// Execute runs one AI query with deterministic rules only.
func (e *RuleExecutor) Execute(ctx context.Context, req AIQueryRequest) (AIQueryResponse, error) {
	return e.execute(ctx, req, AIModeRule)
}

func (e *RuleExecutor) execute(ctx context.Context, req AIQueryRequest, mode string) (AIQueryResponse, error) {
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

	response := AIQueryResponse{
		Mode:      mode,
		ToolCalls: []ToolCallRecord{},
		Warnings:  []string{},
	}

	switch intent.Type {
	case IntentIdleNodeRanking:
		addToolCall(&response.ToolCalls, "list_idle_nodes", map[string]any{
			"min_free_vram_gb": nil,
			"limit":            intent.TopK,
		})
		candidates, err := e.toolbox.ListIdleNodes(ctx, nil, intent.TopK)
		if err != nil {
			return AIQueryResponse{}, err
		}
		if len(candidates) == 0 {
			response.Answer = localizedText(
				lang,
				"当前没有可用在线节点用于推荐。建议先确认节点是否在线并等待下一轮数据刷新。",
				"No online node is currently available for recommendation. Please verify node health and wait for the next refresh.",
			)
			response.ReasoningSummary = localizedText(
				lang,
				"判断依据：在线状态、GPU 可用量与数据新鲜度不足。",
				"Reasoning: online status, usable GPU capacity, and data freshness are currently insufficient.",
			)
			response.Warnings = append(response.Warnings, basedOnCurrentSampleWarning(lang))
			return response, nil
		}

		response.RelatedNodes = collectNodeNames(candidates)
		response.Answer = buildIdleAnswer(candidates, lang)
		response.ReasoningSummary = buildCandidateReasoning(candidates, lang)
		response.Warnings = append(response.Warnings, buildDataFreshnessWarning(candidates, lang)...)
		return response, nil

	case IntentScheduleSuggestion:
		reqArg := intent.Requirement
		if reqArg.MinFreeVRAMGB <= 0 && strings.Contains(strings.ToLower(query), "16") && strings.Contains(strings.ToLower(query), "gb") {
			reqArg.MinFreeVRAMGB = 16
			if reqArg.GPUCount == 0 {
				reqArg.GPUCount = 1
			}
		}
		addToolCall(&response.ToolCalls, "recommend_nodes_for_job", map[string]any{
			"requirements": reqArg,
			"limit":        intent.TopK,
		})
		candidates, err := e.toolbox.RecommendNodesForJob(ctx, reqArg, intent.TopK)
		if err != nil {
			return AIQueryResponse{}, err
		}
		if len(candidates) == 0 {
			response.Answer = localizedText(
				lang,
				"当前没有节点满足你的任务约束。建议先放宽显存或并发要求，再重试调度。",
				"No node currently matches your workload constraints. Please relax VRAM or concurrency requirements and retry.",
			)
			response.ReasoningSummary = localizedText(
				lang,
				"判断依据：候选节点中 GPU 空闲资源或系统负载条件未达标。",
				"Reasoning: candidate nodes do not meet required idle GPU resources or load limits.",
			)
			response.Warnings = append(response.Warnings, basedOnCurrentSampleWarning(lang))
			return response, nil
		}

		response.RelatedNodes = collectNodeNames(candidates)
		response.Answer = buildScheduleAnswer(candidates, reqArg, intent.TopK, lang)
		response.ReasoningSummary = buildCandidateReasoning(candidates, lang)
		response.Warnings = append(response.Warnings, buildDataFreshnessWarning(candidates, lang)...)
		response = e.enrichResponseWithKnowledge(ctx, query, intent, response, lang, "")
		return response, nil

	case IntentNodeSummary:
		if strings.TrimSpace(intent.NodeName) == "" {
			response.Answer = localizedText(
				lang,
				"请先提供节点名，例如：server-03。",
				"Please provide a node name first, for example: server-03.",
			)
			response.ReasoningSummary = localizedText(
				lang,
				"该问题需要指定目标节点后才能给出结论。",
				"This request needs a specific node target before a conclusion can be made.",
			)
			return response, nil
		}
		addToolCall(&response.ToolCalls, "get_node_summary", map[string]any{
			"node_name": intent.NodeName,
		})
		summary, err := e.toolbox.GetNodeSummary(ctx, intent.NodeName)
		if err != nil {
			return AIQueryResponse{}, err
		}

		response.RelatedNodes = []string{summary.NodeName}
		response.Answer = buildNodeSummaryAnswer(summary, lang)
		response.ReasoningSummary = buildNodeSummaryReasoning(summary, lang)
		response.Warnings = append(response.Warnings, buildStaleWarning(summary.DataAgeSec, lang)...)
		return response, nil

	case IntentAnomalyExplanation:
		if strings.TrimSpace(intent.NodeName) == "" {
			if resolved, ok := e.answerWithKnowledgeOnly(ctx, query, intent, response, lang); ok {
				return resolved, nil
			}
			intent.NodeName = fallbackNodeName(knownNodes)
		}
		if strings.TrimSpace(intent.NodeName) == "" {
			response.Answer = localizedText(
				lang,
				"请先告诉我要排查的节点名称。",
				"Please tell me which node you want to investigate.",
			)
			response.ReasoningSummary = localizedText(
				lang,
				"异常解释需要绑定具体节点。",
				"Anomaly explanation requires a specific node.",
			)
			return response, nil
		}
		addToolCall(&response.ToolCalls, "explain_node_anomaly", map[string]any{
			"node_name": intent.NodeName,
		})
		explanation, err := e.toolbox.ExplainNodeAnomaly(ctx, intent.NodeName)
		if err != nil {
			return AIQueryResponse{}, err
		}

		response.RelatedNodes = []string{explanation.NodeName}
		response.Answer = buildAnomalyAnswer(explanation, lang)
		response.ReasoningSummary = buildAnomalyReasoning(explanation, lang)
		response = e.enrichResponseWithKnowledge(ctx, query, intent, response, lang, strings.Join(append(explanation.Findings, explanation.PossibleCauses...), " "))
		return response, nil

	case IntentAlertSummary, IntentHistoryAnalysis:
		var nodeNamePtr *string
		if strings.TrimSpace(intent.NodeName) != "" {
			nodeName := intent.NodeName
			nodeNamePtr = &nodeName
		}
		windowArg := windowLabel(intent.Window)
		addToolCall(&response.ToolCalls, "get_alert_history", map[string]any{
			"node_name": nodeNamePtr,
			"window":    windowArg,
		})
		result, err := e.toolbox.GetAlertHistory(ctx, nodeNamePtr, windowArg)
		if err != nil {
			return AIQueryResponse{}, err
		}
		if len(result.Summaries) == 0 {
			response.Answer = localizedText(
				lang,
				"当前时间窗口内没有足够历史数据用于趋势分析。",
				"There is not enough history in the current time window for trend analysis.",
			)
			response.ReasoningSummary = localizedText(
				lang,
				"建议等待更多采样周期后再查看趋势。",
				"Please wait for more polling cycles and check again.",
			)
			response.Warnings = append(response.Warnings, basedOnCurrentSampleWarning(lang))
			return response, nil
		}

		response.RelatedNodes = collectAlertNodeNames(result.Summaries)
		top := result.Summaries[0]
		response.Answer = buildAlertAnswer(result.WindowLabel, top, lang)
		response.ReasoningSummary = buildAlertReasoning(result.Summaries, lang)
		return response, nil

	default:
		if e.shouldUseKnowledgeOnly(query, intent) {
			if resolved, ok := e.answerWithKnowledgeOnly(ctx, query, intent, response, lang); ok {
				return resolved, nil
			}
		}
		addToolCall(&response.ToolCalls, "list_idle_nodes", map[string]any{
			"min_free_vram_gb": nil,
			"limit":            3,
		})
		candidates, err := e.toolbox.ListIdleNodes(ctx, nil, 3)
		if err != nil {
			return AIQueryResponse{}, err
		}
		if len(candidates) == 0 {
			response.Answer = localizedText(
				lang,
				"我暂时无法从当前数据给出可靠建议。请先确认节点在线并稍后重试。",
				"I cannot provide a reliable recommendation from current data yet. Please verify node connectivity and retry.",
			)
			response.ReasoningSummary = localizedText(
				lang,
				"判断依据不足：在线节点或最新采样数据不完整。",
				"Insufficient evidence: online nodes or fresh samples are incomplete.",
			)
			response.Warnings = append(response.Warnings, basedOnCurrentSampleWarning(lang))
			return response, nil
		}

		response.RelatedNodes = collectNodeNames(candidates)
		response.Answer = localizedText(
			lang,
			"我先给出当前可优先使用的节点："+strings.Join(response.RelatedNodes, "、")+"。",
			"Here are the currently preferred nodes: "+strings.Join(response.RelatedNodes, ", ")+".",
		)
		response.ReasoningSummary = buildCandidateReasoning(candidates, lang)
		response.Warnings = append(response.Warnings, localizedText(
			lang,
			"本次请求采用了规则模式的意图兜底。",
			"This request used deterministic intent fallback.",
		))
		return response, nil
	}
}

func buildIdleAnswer(candidates []NodeCandidate, lang responseLanguage) string {
	if len(candidates) == 1 {
		return localizedText(
			lang,
			fmt.Sprintf("结论：当前最空闲的节点是 %s，建议优先把新任务放到该节点。", candidates[0].NodeName),
			fmt.Sprintf("Conclusion: %s is currently the most idle node. Prefer scheduling new jobs there first.", candidates[0].NodeName),
		)
	}
	nodes := collectNodeNames(candidates)
	return localizedText(
		lang,
		fmt.Sprintf("结论：当前可优先考虑的节点依次为 %s，建议按这个顺序尝试调度。", strings.Join(nodes, "、")),
		fmt.Sprintf("Conclusion: preferred nodes are %s in order. Try scheduling in this order.", strings.Join(nodes, ", ")),
	)
}
func buildScheduleAnswer(candidates []NodeCandidate, requirement JobRequirement, requestedCount int, lang responseLanguage) string {
	if len(candidates) == 0 {
		return ""
	}

	requested := requestedCount
	if requested <= 0 {
		requested = 1
	}
	if requested > len(candidates) {
		requested = len(candidates)
	}

	primary := candidates[0].NodeName
	headline := localizedText(
		lang,
		fmt.Sprintf("%s 当前在线且相对空闲，可先投放 1 个中等负载任务；如果是大任务，建议先观测 5 分钟再扩容。", primary),
		fmt.Sprintf("%s is online and relatively idle. You can assign one medium-load job now; for larger jobs, observe for 5 minutes before scaling.", primary),
	)

	if requested == 1 {
		if requestedCount > 1 && len(candidates) == 1 {
			return localizedText(
				lang,
				fmt.Sprintf("当前仅筛出 1 台满足条件：%s。%s", primary, headline),
				fmt.Sprintf("Only one qualified node is available right now: %s. %s", primary, headline),
			)
		}
		if requirement.MinFreeVRAMGB > 0 {
			return localizedText(
				lang,
				fmt.Sprintf("%s 该节点可覆盖约 %.0fGB 显存需求。", headline, requirement.MinFreeVRAMGB),
				fmt.Sprintf("%s It can cover roughly %.0fGB VRAM requirement.", headline, requirement.MinFreeVRAMGB),
			)
		}
		return headline
	}

	secondary := candidates[1].NodeName
	body := localizedText(
		lang,
		fmt.Sprintf("建议优先使用 %s 和 %s。%s 先承载第一批任务，%s 作为备选；若是大任务，建议先观测 5 分钟再扩容。", primary, secondary, primary, secondary),
		fmt.Sprintf("Prefer %s and %s first. Start the first batch on %s, keep %s as backup; for larger jobs, observe for 5 minutes before scaling.", primary, secondary, primary, secondary),
	)
	if requirement.MinFreeVRAMGB > 0 {
		return localizedText(
			lang,
			fmt.Sprintf("%s 两台节点都可覆盖约 %.0fGB 显存需求。", body, requirement.MinFreeVRAMGB),
			fmt.Sprintf("%s Both can cover roughly %.0fGB VRAM requirement.", body, requirement.MinFreeVRAMGB),
		)
	}
	return body
}
func buildCandidateReasoning(candidates []NodeCandidate, lang responseLanguage) string {
	if len(candidates) == 0 {
		return ""
	}
	ordered := make([]string, 0, len(candidates))
	for idx, candidate := range candidates {
		if idx >= 3 {
			break
		}
		ordered = append(ordered, candidate.NodeName)
	}
	return localizedText(
		lang,
		"依据：综合了 CPU、内存和 GPU 余量，以及数据新鲜度；当前优先顺序："+strings.Join(ordered, "、")+"。",
		"Based on CPU/RAM headroom, GPU availability, and data freshness; current priority order: "+strings.Join(ordered, ", ")+".",
	)
}
func buildNodeSummaryAnswer(summary NodeSummary, lang responseLanguage) string {
	status := localizedStatusText(lang, summary.Status)
	cpu := ptrToFloat(summary.CPUUsage)
	ram := ptrToFloat(summary.RAMPercent)

	gpuPart := localizedText(
		lang,
		fmt.Sprintf("GPU 空闲 %d/%d", summary.IdleGPUCount, summary.GPUCount),
		fmt.Sprintf("idle GPU %d/%d", summary.IdleGPUCount, summary.GPUCount),
	)
	if summary.GPUCount == 0 {
		gpuPart = localizedText(lang, "未检测到可用 GPU", "no available GPU detected")
	}

	recommendation := localizedText(
		lang,
		"建议继续小批量调度并观察下一轮指标。",
		"Recommendation: continue with a small rollout and watch the next polling cycle.",
	)
	if strings.EqualFold(summary.Status, "offline") {
		recommendation = localizedText(
			lang,
			"建议先恢复节点连通性，再安排新任务。",
			"Recommendation: restore node connectivity before scheduling new jobs.",
		)
	} else if summary.AvailabilityScore < 50 || summary.IdleGPUCount == 0 || cpu >= 80 || ram >= 85 {
		recommendation = localizedText(
			lang,
			"建议暂缓新任务，先降低当前负载后再调度。",
			"Recommendation: hold new workloads for now and reduce current pressure first.",
		)
	}

	if lang == responseLanguageZH {
		return fmt.Sprintf(
			"结论：节点 %s 当前%s，可用性评分 %d。%s，CPU %.1f%%，内存 %.1f%%。%s",
			summary.NodeName,
			status,
			summary.AvailabilityScore,
			gpuPart,
			cpu,
			ram,
			recommendation,
		)
	}
	return fmt.Sprintf(
		"Conclusion: node %s is %s with availability score %d. %s, CPU %.1f%%, RAM %.1f%%. %s",
		summary.NodeName,
		status,
		summary.AvailabilityScore,
		gpuPart,
		cpu,
		ram,
		recommendation,
	)
}

func buildNodeSummaryReasoning(summary NodeSummary, lang responseLanguage) string {
	risk := strings.Join(summary.RiskFlags, ", ")
	if risk == "" {
		return localizedText(
			lang,
			"依据：在线状态、可用性评分、GPU 空闲量与系统负载的综合判断。",
			"Reasoning is based on node status, availability score, idle GPU, and system load.",
		)
	}
	return localizedText(
		lang,
		fmt.Sprintf("关键风险标记：%s。", risk),
		fmt.Sprintf("Key risk flags: %s.", risk),
	)
}

func buildAnomalyAnswer(explanation AnomalyExplanation, lang responseLanguage) string {
	severity := strings.ToLower(strings.TrimSpace(explanation.Severity))
	switch severity {
	case "high":
		return localizedText(
			lang,
			fmt.Sprintf("结论：%s 风险较高，不建议继续投放新任务。请优先检查节点在线状态、网络连通性和关键进程。", explanation.NodeName),
			fmt.Sprintf("Conclusion: %s is high risk. Avoid assigning new workloads now and check node reachability, networking, and key processes first.", explanation.NodeName),
		)
	case "medium":
		return localizedText(
			lang,
			fmt.Sprintf("结论：%s 存在中等风险，建议先做负载整理，再继续调度。", explanation.NodeName),
			fmt.Sprintf("Conclusion: %s has medium risk. Stabilize current load before adding new workloads.", explanation.NodeName),
		)
	default:
		return localizedText(
			lang,
			fmt.Sprintf("结论：%s 当前未发现明显高风险，可继续观察后按需调度。", explanation.NodeName),
			fmt.Sprintf("Conclusion: no strong high-risk signal on %s right now; continue monitoring and schedule as needed.", explanation.NodeName),
		)
	}
}

func buildAnomalyReasoning(explanation AnomalyExplanation, lang responseLanguage) string {
	severity := localizedSeverityText(lang, explanation.Severity)
	confidence := strings.TrimSpace(explanation.Confidence)
	if confidence == "" {
		confidence = localizedText(lang, "中", "medium")
	}
	return localizedText(
		lang,
		fmt.Sprintf("判断依据：节点状态、资源负载与数据新鲜度；风险等级 %s，置信度 %s。", severity, confidence),
		fmt.Sprintf("Reasoning: node status, resource pressure, and data freshness; severity %s, confidence %s.", severity, confidence),
	)
}

func buildAlertAnswer(windowLabel string, top AlertSummary, lang responseLanguage) string {
	return localizedText(
		lang,
		fmt.Sprintf("结论：近 %s 需要优先关注 %s（风险 %s）。建议先核查该节点的离线切换与负载波动。", windowLabel, top.NodeName, localizedSeverityText(lang, top.Severity)),
		fmt.Sprintf("Conclusion: in the last %s, %s needs the highest attention (severity %s). Review offline transitions and load volatility first.", windowLabel, top.NodeName, localizedSeverityText(lang, top.Severity)),
	)
}

func buildAlertReasoning(summaries []AlertSummary, lang responseLanguage) string {
	if len(summaries) == 0 {
		return ""
	}
	highlights := make([]string, 0, len(summaries))
	for i, summary := range summaries {
		if i >= 3 {
			break
		}
		highlights = append(highlights, fmt.Sprintf(
			"%s(%s, score %.1f, offline %d)",
			summary.NodeName,
			localizedSeverityText(lang, summary.Severity),
			summary.AvgAvailabilityScore,
			summary.OfflineTransitions,
		))
	}
	return localizedText(
		lang,
		"主要依据为可用性均值、离线切换次数与波动度；重点节点："+strings.Join(highlights, "；")+"。",
		"Based on average availability, offline transitions, and volatility; key nodes: "+strings.Join(highlights, "; ")+".",
	)
}
func addToolCall(records *[]ToolCallRecord, name string, args map[string]any) {
	if records == nil {
		return
	}
	*records = append(*records, ToolCallRecord{Name: name, Args: args})
}

func collectNodeNames(candidates []NodeCandidate) []string {
	result := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		result = append(result, candidate.NodeName)
	}
	return result
}

func collectAlertNodeNames(summaries []AlertSummary) []string {
	result := make([]string, 0, len(summaries))
	for _, summary := range summaries {
		result = append(result, summary.NodeName)
	}
	return result
}

func fallbackNodeName(knownNodes []string) string {
	if len(knownNodes) == 0 {
		return ""
	}
	return knownNodes[0]
}

func buildStaleWarning(age *float64, lang responseLanguage) []string {
	if age == nil {
		return []string{
			localizedText(
				lang,
				"数据时间戳缺失，结论仅供参考。",
				"Data age is missing; conclusions are for reference only.",
			),
		}
	}
	if *age > stalePenaltyThresholdSec {
		return []string{
			localizedText(
				lang,
				fmt.Sprintf("数据已过旧（%.0fs），建议结合下一轮采样复核。", *age),
				fmt.Sprintf("Data is stale (%.0fs); re-check with the next polling cycle.", *age),
			),
		}
	}
	return nil
}

func buildDataFreshnessWarning(candidates []NodeCandidate, lang responseLanguage) []string {
	warnings := make([]string, 0, 1)
	for _, candidate := range candidates {
		if candidate.DataAgeSec > stalePenaltyThresholdSec {
			warnings = append(warnings, localizedText(
				lang,
				fmt.Sprintf("%s 的数据约 %.0fs 前采集，建议结合最新快照复核。", candidate.NodeName, candidate.DataAgeSec),
				fmt.Sprintf("%s data is about %.0fs old; validate with the latest snapshot.", candidate.NodeName, candidate.DataAgeSec),
			))
		}
	}
	return warnings
}

func basedOnCurrentSampleWarning(lang responseLanguage) string {
	return localizedText(
		lang,
		"以上结论基于当前采样快照。",
		"The conclusion is based on the current sampled snapshot.",
	)
}

func (e *RuleExecutor) enrichResponseWithKnowledge(ctx context.Context, query string, intent QueryIntent, response AIQueryResponse, lang responseLanguage, extraContext string) AIQueryResponse {
	if e == nil || e.toolbox == nil || !e.shouldSearchKnowledge(query, intent) {
		return response
	}

	searchQuery := strings.TrimSpace(strings.TrimSpace(query) + " " + strings.TrimSpace(extraContext))
	hits, meta, err := e.toolbox.SearchKnowledge(ctx, searchQuery, intent.TopK)
	if err != nil {
		return response
	}
	response.Retrieval = &meta
	if len(hits) == 0 {
		return response
	}

	response.KnowledgeHits = summarizeKnowledgeHits(hits)
	supplement := renderKnowledgeSupplement(hits, lang)
	if supplement != "" {
		if strings.TrimSpace(response.Answer) == "" {
			response.Answer = supplement
		} else {
			response.Answer = strings.TrimSpace(response.Answer + "\n\n" + supplement)
		}
	}

	response.ReasoningSummary = strings.TrimSpace(strings.TrimSpace(response.ReasoningSummary) + " " + localizedText(
		lang,
		"同时补充了知识库中的通用原因与修复建议。",
		"Also supplemented with common-cause and remediation knowledge.",
	))
	return response
}

func (e *RuleExecutor) answerWithKnowledgeOnly(ctx context.Context, query string, intent QueryIntent, response AIQueryResponse, lang responseLanguage) (AIQueryResponse, bool) {
	hits, meta, err := e.toolbox.SearchKnowledge(ctx, query, intent.TopK)
	if err != nil {
		return response, false
	}
	response.Retrieval = &meta
	if len(hits) == 0 {
		return response, false
	}

	response.KnowledgeHits = summarizeKnowledgeHits(hits)
	response.Answer = localizedText(
		lang,
		"当前缺少可绑定节点的实时上下文。以下是来自本地知识库的通用排障建议：\n"+renderKnowledgeSupplement(hits, lang),
		"No node-specific realtime context is available. The following are general troubleshooting suggestions from the local knowledge base:\n"+renderKnowledgeSupplement(hits, lang),
	)
	response.ReasoningSummary = localizedText(
		lang,
		"暂无节点级实时证据，本次先返回通用排障建议。",
		"No node-specific realtime evidence was available, so generic troubleshooting guidance was returned first.",
	)
	response.Warnings = append(response.Warnings, localizedText(
		lang,
		"以上为通用建议，请结合实时监控结果执行。",
		"These are general recommendations; validate against realtime metrics before execution.",
	))
	return response, true
}

func (e *RuleExecutor) shouldSearchKnowledge(query string, intent QueryIntent) bool {
	if e == nil || e.toolbox == nil || !e.toolbox.HasKnowledge() {
		return false
	}
	if strings.TrimSpace(query) == "" {
		return false
	}
	if intent.Type == IntentAnomalyExplanation {
		return true
	}
	return isKnowledgeDiagnosticQuery(query)
}

func (e *RuleExecutor) shouldUseKnowledgeOnly(query string, intent QueryIntent) bool {
	if !e.shouldSearchKnowledge(query, intent) {
		return false
	}
	if strings.TrimSpace(intent.NodeName) != "" {
		return false
	}
	switch intent.Type {
	case IntentNodeSummary, IntentAlertSummary, IntentHistoryAnalysis:
		return false
	}
	lower := strings.ToLower(strings.TrimSpace(query))
	if containsAny(lower, "current", "now", "how much", "\u591a\u5c11", "\u5f53\u524d", "\u5b9e\u65f6", "cpu", "memory", "gpu") &&
		!containsAny(lower, "why", "reason", "cause", "how to", "what should", "\u4e3a\u4ec0\u4e48", "\u539f\u56e0", "\u6392\u67e5", "\u5efa\u8bae", "\u5904\u7406") {
		return false
	}
	return true
}

func isKnowledgeDiagnosticQuery(query string) bool {
	lower := strings.ToLower(strings.TrimSpace(query))
	if lower == "" {
		return false
	}
	return containsAny(lower,
		"oom", "cuda", "out of memory", "fragmentation", "low utilization", "offline", "unreachable", "stale",
		"resource insufficient", "resource contention", "too many users", "active users", "troubleshoot", "diagnostic",
		"\u663e\u5b58", "\u7206\u6ee1", "\u788e\u7247", "\u5229\u7528\u7387\u4f4e", "\u8282\u70b9\u79bb\u7ebf", "\u4e0d\u53ef\u8fbe", "\u6570\u636e\u592a\u65e7", "\u6307\u6807\u592a\u65e7", "\u9648\u65e7", "\u8fdb\u7a0b\u5f02\u5e38",
		"\u8d44\u6e90\u4e0d\u8db3", "\u8c03\u5ea6\u4e0d\u4e0a", "\u6d3b\u8dc3\u7528\u6237", "\u7ade\u4e89", "\u6392\u969c", "\u6392\u67e5", "\u600e\u4e48\u5904\u7406", "\u4e00\u822c\u662f\u4ec0\u4e48\u539f\u56e0",
	)
}

func renderKnowledgeSupplement(hits []KnowledgeHit, lang responseLanguage) string {
	if len(hits) == 0 {
		return ""
	}
	limit := 2
	if len(hits) < limit {
		limit = len(hits)
	}
	lines := make([]string, 0, limit)
	for i := 0; i < limit; i++ {
		hit := hits[i]
		snippet := compactSnippet(hit.Snippet, 96)
		if snippet == "" {
			snippet = localizedText(lang, "可参考对应文档中的检查清单继续排查。", "Refer to the associated checklist for next diagnostics.")
		}
		lines = append(lines, fmt.Sprintf("- %s: %s", hit.Title, snippet))
	}
	return localizedText(
		lang,
		"结合知识库中的通用经验，优先建议检查：\n"+strings.Join(lines, "\n"),
		"Based on knowledge-base common patterns, prioritize checking:\n"+strings.Join(lines, "\n"),
	)
}

func compactSnippet(raw string, maxRunes int) string {
	if maxRunes <= 0 {
		maxRunes = 96
	}
	normalized := strings.Join(strings.Fields(strings.TrimSpace(raw)), " ")
	if normalized == "" {
		return ""
	}
	runes := []rune(normalized)
	if len(runes) <= maxRunes {
		return normalized
	}
	return strings.TrimSpace(string(runes[:maxRunes])) + "..."
}
