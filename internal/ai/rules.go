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
			response.Answer = "当前没有可用的在线节点可以用于推荐。"
			response.ReasoningSummary = "节点可能离线、没有 GPU，或数据尚未刷新。"
			response.Warnings = append(response.Warnings, "Based on current sampled data.")
			return response, nil
		}
		response.RelatedNodes = collectNodeNames(candidates)
		response.Answer = buildIdleAnswer(candidates)
		response.ReasoningSummary = buildCandidateReasoning(candidates)
		response.Warnings = append(response.Warnings, buildDataFreshnessWarning(candidates)...)
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
			response.Answer = "当前没有节点满足任务约束。"
			response.ReasoningSummary = "在线节点中没有足够空闲显存/GPU，或节点负载过高。"
			response.Warnings = append(response.Warnings, "Based on current sampled data.")
			return response, nil
		}
		response.RelatedNodes = collectNodeNames(candidates)
		response.Answer = buildScheduleAnswer(candidates, reqArg)
		response.ReasoningSummary = buildCandidateReasoning(candidates)
		response.Warnings = append(response.Warnings, buildDataFreshnessWarning(candidates)...)
		return response, nil

	case IntentNodeSummary:
		if strings.TrimSpace(intent.NodeName) == "" {
			response.Answer = "请指定节点名称，例如 server-03。"
			response.ReasoningSummary = "该问题属于节点摘要查询，但缺少明确节点。"
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
		response.Answer = fmt.Sprintf("%s %s %s", summary.HealthText, summary.GPUText, summary.ProcessText)
		response.ReasoningSummary = fmt.Sprintf("风险标记：%s。", strings.Join(summary.RiskFlags, ", "))
		response.Warnings = append(response.Warnings, buildStaleWarning(summary.DataAgeSec)...)
		return response, nil

	case IntentAnomalyExplanation:
		if strings.TrimSpace(intent.NodeName) == "" {
			intent.NodeName = fallbackNodeName(knownNodes)
		}
		if strings.TrimSpace(intent.NodeName) == "" {
			response.Answer = "请提供要解释的节点名称。"
			response.ReasoningSummary = "异常解释需要绑定具体节点。"
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
		response.Answer = fmt.Sprintf("%s 的主要判断：%s", explanation.NodeName, strings.Join(explanation.Findings, " "))
		response.ReasoningSummary = fmt.Sprintf("可能原因：%s 建议：%s",
			strings.Join(explanation.PossibleCauses, "; "),
			strings.Join(explanation.Suggestions, "; "),
		)
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
			response.Answer = "当前窗口内没有足够历史样本用于分析。"
			response.ReasoningSummary = "请等待更多轮询快照后重试。"
			response.Warnings = append(response.Warnings, "Based on current sampled data.")
			return response, nil
		}
		response.RelatedNodes = collectAlertNodeNames(result.Summaries)
		top := result.Summaries[0]
		response.Answer = fmt.Sprintf("最近 %s 异常最明显的是 %s（%s）。", result.WindowLabel, top.NodeName, top.Severity)
		response.ReasoningSummary = buildAlertReasoning(result.Summaries)
		return response, nil

	default:
		addToolCall(&response.ToolCalls, "list_idle_nodes", map[string]any{
			"min_free_vram_gb": nil,
			"limit":            3,
		})
		candidates, err := e.toolbox.ListIdleNodes(ctx, nil, 3)
		if err != nil {
			return AIQueryResponse{}, err
		}
		if len(candidates) == 0 {
			response.Answer = "我暂时无法从当前数据中给出有效建议。"
			response.ReasoningSummary = "请确认节点在线并已有最新采样。"
			response.Warnings = append(response.Warnings, "Based on current sampled data.")
			return response, nil
		}
		response.RelatedNodes = collectNodeNames(candidates)
		response.Answer = "我先给出当前相对空闲节点：" + strings.Join(response.RelatedNodes, ", ") + "。"
		response.ReasoningSummary = buildCandidateReasoning(candidates)
		response.Warnings = append(response.Warnings, "Intent fallback used deterministic idle ranking.")
		return response, nil
	}
}

func buildIdleAnswer(candidates []NodeCandidate) string {
	if len(candidates) == 1 {
		return fmt.Sprintf("当前最空闲节点是 %s。", candidates[0].NodeName)
	}
	return fmt.Sprintf("当前更空闲的节点依次是 %s。", strings.Join(collectNodeNames(candidates), ", "))
}

func buildScheduleAnswer(candidates []NodeCandidate, requirement JobRequirement) string {
	if len(candidates) == 1 {
		return fmt.Sprintf("当前更适合的是 %s。", candidates[0].NodeName)
	}
	if requirement.MinFreeVRAMGB > 0 {
		return fmt.Sprintf("当前更适合 %.0fGB 显存任务的节点是 %s。",
			requirement.MinFreeVRAMGB, strings.Join(collectNodeNames(candidates), ", "))
	}
	return fmt.Sprintf("当前推荐节点是 %s。", strings.Join(collectNodeNames(candidates), ", "))
}

func buildCandidateReasoning(candidates []NodeCandidate) string {
	reasons := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		reasons = append(reasons, fmt.Sprintf("%s(score %.1f, idle GPU %d, max free VRAM %.1fGB)",
			candidate.NodeName,
			candidate.Score,
			candidate.IdleGPUCount,
			candidate.MaxFreeVRAMGB,
		))
	}
	return "排序依据：在线状态、数据新鲜度、availability score、idle GPU、可用显存、CPU/RAM 和活跃用户。结果为 " + strings.Join(reasons, "; ")
}

func buildAlertReasoning(summaries []AlertSummary) string {
	parts := make([]string, 0, len(summaries))
	for _, summary := range summaries {
		parts = append(parts, fmt.Sprintf("%s(%s, avg score %.1f, offline transitions %d)",
			summary.NodeName,
			summary.Severity,
			summary.AvgAvailabilityScore,
			summary.OfflineTransitions,
		))
	}
	return strings.Join(parts, "; ")
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

func buildStaleWarning(age *float64) []string {
	if age == nil {
		return []string{"Data age is missing; reasoning is based on limited samples."}
	}
	if *age > stalePenaltyThresholdSec {
		return []string{fmt.Sprintf("Data is stale (%.0fs); conclusions are based on current sampled data.", *age)}
	}
	return nil
}

func buildDataFreshnessWarning(candidates []NodeCandidate) []string {
	warnings := make([]string, 0, 1)
	for _, candidate := range candidates {
		if candidate.DataAgeSec > stalePenaltyThresholdSec {
			warnings = append(warnings,
				fmt.Sprintf("%s data is %.0fs old; recommendation is based on current sampled data.", candidate.NodeName, candidate.DataAgeSec),
			)
		}
	}
	return warnings
}
