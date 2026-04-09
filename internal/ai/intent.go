package ai

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

var (
	numberGBPattern          = regexp.MustCompile(`(?i)(\d+(?:\.\d+)?)\s*(?:gib|gb|g)`)
	gpuCountPattern          = regexp.MustCompile(`(?i)(\d+)\s*(?:gpu|gpus|cards?)`)
	chineseGPUCountPattern   = regexp.MustCompile(`(\d+)\s*(?:张卡|块卡|个gpu|卡)`)
	genericNodeNameRegex     = regexp.MustCompile(`(?i)\b(?:server|node|host|gpu)[-_]?[a-z0-9]+\b`)
	topKEnglishPattern       = regexp.MustCompile(`(?i)\b(\d+)\s*(?:nodes?|machines?|servers?)\b`)
	topKChineseNumberPattern = regexp.MustCompile(`(\d+)\s*(?:台|个|套)`)
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

	historyLike := containsAny(lower,
		"recent", "history", "trend", "volatility", "30m", "1h",
		"最近", "历史", "趋势", "波动", "30分钟", "1小时", "半小时",
	)
	whyLike := containsAny(lower,
		"why", "reason", "busy", "saturated", "anomaly", "explain",
		"为什么", "原因", "繁忙", "饱和", "异常", "解释",
	)
	scheduleLike := containsAny(lower,
		"recommend", "suitable", "schedule", "training task", "launch training",
		"推荐", "适合", "调度", "训练任务", "启动训练", "分配任务",
	)
	idleLike := containsAny(lower,
		"most idle", "idle", "free", "available gpu",
		"最空闲", "空闲", "空", "可用gpu", "可用 gpu",
	)
	summaryLike := containsAny(lower,
		"resource", "status", "how is", "summary", "overview",
		"资源", "状态", "怎么样", "情况", "概览", "摘要",
	)

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

	if intent.TopK <= 0 {
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
	case containsAny(lower, "1h", "1 hour", "60 min", "60m", "1小时"):
		return time.Hour
	case containsAny(lower, "30m", "30 min", "30分钟", "半小时"):
		return 30 * time.Minute
	default:
		return 30 * time.Minute
	}
}

func parseTopK(query string) int {
	lower := strings.ToLower(strings.TrimSpace(query))

	if match := topKEnglishPattern.FindStringSubmatch(lower); len(match) == 2 {
		if parsed, err := strconv.Atoi(match[1]); err == nil {
			return normalizeTopK(parsed)
		}
	}
	if match := topKChineseNumberPattern.FindStringSubmatch(lower); len(match) == 2 {
		if parsed, err := strconv.Atoi(match[1]); err == nil {
			return normalizeTopK(parsed)
		}
	}

	switch {
	case containsAny(lower, "两台", "俩台", "二台", "两个", "俩个", "pair", "two"):
		return 2
	case containsAny(lower, "三台", "三个", "three"):
		return 3
	case containsAny(lower, "一台", "一个", "single", "one"):
		return 1
	default:
		return 3
	}
}

func normalizeTopK(topK int) int {
	if topK <= 0 {
		return 3
	}
	if topK > 10 {
		return 10
	}
	return topK
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
	} else if match := chineseGPUCountPattern.FindStringSubmatch(lower); len(match) == 2 {
		if parsed, err := strconv.Atoi(match[1]); err == nil && parsed > 0 {
			req.GPUCount = parsed
		}
	}

	req.PreferLowCPU = containsAny(lower, "low cpu", "cpu low", "cpu idle", "低cpu", "cpu低", "cpu空闲")
	req.PreferLowRAM = containsAny(lower, "low ram", "ram low", "memory idle", "低内存", "ram低", "内存空闲")
	req.PreferFewUsers = containsAny(lower, "fewer users", "few users", "less users", "用户少", "人少", "少人")

	if req.GPUCount == 0 && containsAny(lower, "训练", "任务", "显存", "training", "workload") {
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
