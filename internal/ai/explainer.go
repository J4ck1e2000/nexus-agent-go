package ai

import (
	"fmt"
	"strings"
)

// AnomalyExplainer generates deterministic node anomaly explanations.
type AnomalyExplainer struct{}

// NewAnomalyExplainer creates a rule-based anomaly explainer.
func NewAnomalyExplainer() *AnomalyExplainer {
	return &AnomalyExplainer{}
}

// Explain returns structured findings/causes/suggestions for one node snapshot.
func (e *AnomalyExplainer) Explain(node NodeSnapshot) AnomalyExplanation {
	explanation := AnomalyExplanation{
		NodeName:       node.Name,
		Severity:       "medium",
		Findings:       []string{},
		PossibleCauses: []string{},
		Suggestions:    []string{},
		Confidence:     "medium",
	}

	if !strings.EqualFold(node.Status, "online") {
		explanation.Severity = "high"
		explanation.Confidence = "high"
		explanation.Findings = append(explanation.Findings, "Node is offline and cannot provide real-time metrics.")
		explanation.PossibleCauses = append(explanation.PossibleCauses,
			"Agent process is unreachable.",
			"Network connectivity is unstable.",
			"Node service may have restarted or crashed.",
		)
		explanation.Suggestions = append(explanation.Suggestions,
			"Check node agent process and service logs.",
			"Verify network connectivity and firewall policy.",
			"Temporarily avoid scheduling new workloads on this node.",
		)
		return explanation
	}

	age := ptrToFloat(node.DataAgeSec)
	if age > stalePenaltyThresholdSec {
		explanation.Findings = append(explanation.Findings, fmt.Sprintf("Metrics are stale (%.0fs old).", age))
		explanation.PossibleCauses = append(explanation.PossibleCauses,
			"Polling delay or temporary network latency.",
			"Node metrics endpoint responded slowly.",
		)
		explanation.Suggestions = append(explanation.Suggestions,
			"Wait for next refresh and verify trend persistence.",
			"Check gateway poll logs for this node.",
		)
		explanation.Confidence = "low"
	}

	gpuUtil := ptrToFloat(node.GPUSummary.AvgUtilization)
	gpuMem := ptrToFloat(node.GPUSummary.AvgMemoryPercent)
	cpu := ptrToFloat(node.CPUUsage)
	ram := ptrToFloat(node.RAMPercent)

	if gpuUtil >= 75 && gpuMem <= 45 {
		explanation.Findings = append(explanation.Findings, "GPU utilization is high while VRAM usage is relatively low.")
		explanation.PossibleCauses = append(explanation.PossibleCauses,
			"Compute-intensive kernels dominate runtime.",
			"Frequent short kernels or preprocessing reduce memory residency.",
			"Small-batch inference keeps VRAM moderate but keeps SM busy.",
			"Many GPU processes each hold small VRAM footprints.",
		)
		explanation.Suggestions = append(explanation.Suggestions,
			"Inspect kernel profile and batch size configuration.",
			"Check whether data preprocessing is fragmented.",
			"Review GPU process list for many short-lived tasks.",
		)
	}

	if gpuMem >= 80 && gpuUtil <= 35 {
		explanation.Findings = append(explanation.Findings, "VRAM usage is high but GPU utilization is low.")
		explanation.PossibleCauses = append(explanation.PossibleCauses,
			"Model is loaded but workload is blocked or waiting for input.",
			"Processes hold VRAM cache without sustained compute.",
			"Input pipeline starvation prevents continuous GPU execution.",
		)
		explanation.Suggestions = append(explanation.Suggestions,
			"Inspect dataloader/input queue throughput.",
			"Check blocked processes and synchronization points.",
			"Clear stale GPU processes if they are no longer needed.",
		)
	}

	if cpu >= 80 && gpuUtil <= 35 {
		explanation.Findings = append(explanation.Findings, "CPU usage is high while GPU usage stays low.")
		explanation.PossibleCauses = append(explanation.PossibleCauses,
			"Data loading or preprocessing is CPU bottlenecked.",
			"Workload is CPU-dominant and does not effectively use GPU.",
			"GPU binding/configuration may be incorrect for running processes.",
		)
		explanation.Suggestions = append(explanation.Suggestions,
			"Profile dataloader and preprocessing threads.",
			"Confirm processes are correctly bound to GPU devices.",
			"Increase input pipeline parallelism.",
		)
	}

	if ram >= 88 {
		explanation.Findings = append(explanation.Findings, "RAM usage is high.")
		explanation.PossibleCauses = append(explanation.PossibleCauses,
			"Multiple processes are accumulating memory usage.",
			"Large data cache or dataloader buffers are retained.",
			"Potential long-running process memory growth.",
		)
		explanation.Suggestions = append(explanation.Suggestions,
			"Inspect top memory-consuming processes.",
			"Reduce cache/buffer sizes for data pipeline.",
			"Restart suspicious long-running jobs if memory leak is suspected.",
		)
	}

	if node.AvailabilityScore <= 35 {
		explanation.Severity = "high"
		explanation.Findings = append(explanation.Findings,
			fmt.Sprintf("Availability score is low (%d), indicating high scheduling risk.", node.AvailabilityScore),
		)
		explanation.PossibleCauses = append(explanation.PossibleCauses,
			"High CPU/RAM/GPU pressure combined with active user contention.",
			"Busy GPU ratio and stale metrics reduce confidence.",
		)
		explanation.Suggestions = append(explanation.Suggestions,
			"Avoid placing new heavy jobs on this node for now.",
			"Prioritize workload cleanup or migration.",
		)
	}

	if len(explanation.Findings) == 0 {
		explanation.Findings = append(explanation.Findings, "No strong anomaly pattern detected from current snapshot.")
		explanation.PossibleCauses = append(explanation.PossibleCauses, "Node may be in a normal mixed-load state.")
		explanation.Suggestions = append(explanation.Suggestions,
			"Continue monitoring trend in the next few polling cycles.",
			"Use history analysis if issue is intermittent.",
		)
		explanation.Confidence = "low"
	}
	if explanation.Severity != "high" && containsRiskSignal(explanation.Findings) {
		explanation.Severity = "medium"
	}
	return explanation
}

func containsRiskSignal(findings []string) bool {
	for _, finding := range findings {
		if strings.Contains(strings.ToLower(finding), "high") ||
			strings.Contains(strings.ToLower(finding), "low") ||
			strings.Contains(strings.ToLower(finding), "offline") {
			return true
		}
	}
	return false
}
