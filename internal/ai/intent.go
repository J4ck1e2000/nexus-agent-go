package ai

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

var (
	numberGBPattern      = regexp.MustCompile(`(?i)(\d+(?:\.\d+)?)\s*gb`)
	gpuCountPattern      = regexp.MustCompile(`(?i)(\d+)\s*(?:gpu|gpus|卡)`)
	genericNodeNameRegex = regexp.MustCompile(`(?i)\b(?:server|node)[-_]?[a-z0-9]+\b`)
)

// IntentClassifier converts free-form user text into deterministic intent hints.
type IntentClassifier struct{}

// NewIntentClassifier creates a lightweight query classifier.
func NewIntentClassifier() *IntentClassifier {
	return &IntentClassifier{}
}

// Classify detects intent, node target, job constraints, and history window.
func (c *IntentClassifier) Classify(query string, knownNodes []string) QueryIntent {
	lower := strings.ToLower(strings.TrimSpace(query))
	intent := QueryIntent{
		Type:   IntentUnknown,
		Window: 30 * time.Minute,
		TopK:   3,
		Requirement: JobRequirement{
			PreferFreshData: true,
		},
	}
	if lower == "" {
		return intent
	}

	intent.NodeName = extractNodeName(query, knownNodes)
	intent.Window = parseWindow(query)
	intent.TopK = parseTopK(query)
	intent.Requirement = parseJobRequirement(query)

	historyLike := containsAny(lower, "最近", "历史", "趋势", "波动", "30m", "1h", "30分钟", "1小时")
	whyLike := containsAny(lower, "为什么", "原因", "忙", "繁忙", "饱和", "异常", "解释")
	scheduleLike := containsAny(lower, "推荐", "适合", "训练任务", "调度", "显存任务", "启动训练", "哪台机器适合")
	idleLike := containsAny(lower, "最空闲", "最闲", "空闲", "idle", "闲")
	summaryLike := containsAny(lower, "资源情况", "状态", "怎么样", "概览", "summary")

	switch {
	case historyLike && containsAny(lower, "异常", "告警", "offline", "离线", "最明显"):
		intent.Type = IntentAlertSummary
	case historyLike:
		intent.Type = IntentHistoryAnalysis
	case whyLike:
		intent.Type = IntentAnomalyExplanation
	case scheduleLike || intent.Requirement.MinFreeVRAMGB > 0 || intent.Requirement.GPUCount > 0:
		intent.Type = IntentScheduleSuggestion
	case summaryLike && intent.NodeName != "":
		intent.Type = IntentNodeSummary
	case idleLike:
		intent.Type = IntentIdleNodeRanking
	case intent.NodeName != "":
		intent.Type = IntentNodeSummary
	default:
		intent.Type = IntentIdleNodeRanking
	}

	if intent.Type == IntentIdleNodeRanking && intent.TopK <= 0 {
		intent.TopK = 3
	}
	return intent
}

func extractNodeName(query string, knownNodes []string) string {
	if len(knownNodes) > 0 {
		sorted := append([]string(nil), knownNodes...)
		sort.Slice(sorted, func(i, j int) bool {
			return len(sorted[i]) > len(sorted[j])
		})
		lowerQuery := strings.ToLower(query)
		for _, node := range sorted {
			node = strings.TrimSpace(node)
			if node == "" {
				continue
			}
			if strings.Contains(lowerQuery, strings.ToLower(node)) {
				return node
			}
		}
	}

	match := genericNodeNameRegex.FindString(query)
	return strings.TrimSpace(match)
}

func parseWindow(query string) time.Duration {
	lower := strings.ToLower(query)
	switch {
	case containsAny(lower, "1h", "1小时", "60分钟"):
		return time.Hour
	case containsAny(lower, "30m", "30分钟", "半小时"):
		return 30 * time.Minute
	default:
		return 30 * time.Minute
	}
}

func parseTopK(query string) int {
	lower := strings.ToLower(query)
	switch {
	case containsAny(lower, "两台", "2台", "两个"):
		return 2
	case containsAny(lower, "三台", "3台", "三个"):
		return 3
	case containsAny(lower, "一台", "1台", "一个"):
		return 1
	default:
		return 3
	}
}

func parseJobRequirement(query string) JobRequirement {
	lower := strings.ToLower(query)
	req := JobRequirement{
		PreferFreshData: true,
	}

	if match := numberGBPattern.FindStringSubmatch(lower); len(match) == 2 {
		if parsed, err := strconv.ParseFloat(match[1], 64); err == nil {
			req.MinFreeVRAMGB = parsed
		}
	}
	if match := gpuCountPattern.FindStringSubmatch(lower); len(match) == 2 {
		if parsed, err := strconv.Atoi(match[1]); err == nil && parsed > 0 {
			req.GPUCount = parsed
		}
	}

	req.PreferLowCPU = containsAny(lower, "低cpu", "cpu低", "cpu空闲")
	req.PreferLowRAM = containsAny(lower, "低内存", "ram低", "内存空闲")
	req.PreferFewUsers = containsAny(lower, "用户少", "人少", "更空", "少人")
	if req.GPUCount == 0 && containsAny(lower, "训练", "任务", "显存") {
		req.GPUCount = 1
	}
	return req
}

func containsAny(content string, keywords ...string) bool {
	for _, keyword := range keywords {
		if strings.Contains(content, strings.ToLower(keyword)) {
			return true
		}
	}
	return false
}
