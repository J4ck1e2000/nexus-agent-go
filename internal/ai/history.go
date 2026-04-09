package ai

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

// HistoryAnalyzer builds alert/trend summaries from compact history snapshots.
type HistoryAnalyzer struct{}

// NewHistoryAnalyzer creates a history analyzer.
func NewHistoryAnalyzer() *HistoryAnalyzer {
	return &HistoryAnalyzer{}
}

// Summarize returns node-level summaries sorted by severity and volatility.
func (a *HistoryAnalyzer) Summarize(snapshots []NodeHistorySnapshot, window time.Duration) AlertHistoryResult {
	grouped := map[string][]NodeHistorySnapshot{}
	for _, snapshot := range snapshots {
		name := strings.TrimSpace(snapshot.NodeName)
		if name == "" {
			continue
		}
		grouped[name] = append(grouped[name], snapshot)
	}

	summaries := make([]AlertSummary, 0, len(grouped))
	for nodeName, series := range grouped {
		if len(series) == 0 {
			continue
		}
		sort.Slice(series, func(i, j int) bool {
			return series[i].TimestampUnix < series[j].TimestampUnix
		})
		summaries = append(summaries, summarizeNodeHistory(nodeName, series))
	}

	sort.SliceStable(summaries, func(i, j int) bool {
		severityWeight := func(s string) int {
			switch strings.ToLower(s) {
			case "high":
				return 3
			case "medium":
				return 2
			default:
				return 1
			}
		}
		if severityWeight(summaries[i].Severity) == severityWeight(summaries[j].Severity) {
			return summaries[i].VolatilityScore > summaries[j].VolatilityScore
		}
		return severityWeight(summaries[i].Severity) > severityWeight(summaries[j].Severity)
	})

	return AlertHistoryResult{
		WindowLabel: windowLabel(window),
		Summaries:   summaries,
	}
}

func summarizeNodeHistory(nodeName string, series []NodeHistorySnapshot) AlertSummary {
	if len(series) == 0 {
		return AlertSummary{
			NodeName: nodeName,
			Severity: "low",
			Summary:  "No history records in selected time window.",
		}
	}

	offlineTransitions := 0
	offlineSamples := 0
	busySamples := 0
	staleSamples := 0
	availabilityTotal := 0.0
	gpuPressureTotal := 0.0
	activeUsersTotal := 0.0
	volatility := 0.0

	previousScore := float64(series[0].AvailabilityScore)
	previousStatusOffline := !strings.EqualFold(series[0].Status, "online")

	for idx, snapshot := range series {
		availabilityTotal += float64(snapshot.AvailabilityScore)
		gpuPressureTotal += snapshot.GPUPressure
		activeUsersTotal += float64(snapshot.ActiveUserCount)

		isOffline := !strings.EqualFold(snapshot.Status, "online")
		if isOffline {
			offlineSamples++
		}
		if strings.EqualFold(snapshot.AvailabilityTier, "busy") || strings.EqualFold(snapshot.AvailabilityTier, "saturated") {
			busySamples++
		}
		if snapshot.DataAgeSec != nil && *snapshot.DataAgeSec > stalePenaltyThresholdSec {
			staleSamples++
		}
		if idx > 0 {
			volatility += math.Abs(float64(snapshot.AvailabilityScore) - previousScore)
			if isOffline && !previousStatusOffline {
				offlineTransitions++
			}
			previousScore = float64(snapshot.AvailabilityScore)
			previousStatusOffline = isOffline
		}
	}

	count := float64(len(series))
	avgScore := availabilityTotal / count
	avgGPUPressure := gpuPressureTotal / count
	avgUsers := activeUsersTotal / count
	busyRatio := float64(busySamples) / count
	offlineRatio := float64(offlineSamples) / count

	severity := "low"
	if offlineTransitions >= 2 || offlineRatio >= 0.3 || avgScore < 35 || busyRatio > 0.7 {
		severity = "high"
	} else if offlineTransitions >= 1 || avgScore < 55 || busyRatio > 0.4 || avgGPUPressure > 65 {
		severity = "medium"
	}

	findings := []string{
		fmt.Sprintf("Avg availability score %.1f", avgScore),
		fmt.Sprintf("Avg GPU pressure %.1f", avgGPUPressure),
		fmt.Sprintf("Avg active users %.1f", avgUsers),
	}
	if offlineTransitions > 0 {
		findings = append(findings, fmt.Sprintf("Offline transitions %d", offlineTransitions))
	}
	if staleSamples > 0 {
		findings = append(findings, fmt.Sprintf("Stale samples %d", staleSamples))
	}

	summary := fmt.Sprintf(
		"Within the selected window %s stayed %s, avg score %.1f, avg GPU pressure %.1f, avg active users %.1f.",
		nodeName,
		tierSummary(offlineRatio, busyRatio),
		avgScore,
		avgGPUPressure,
		avgUsers,
	)
	if offlineTransitions > 0 {
		summary += fmt.Sprintf(" It switched to offline %d time(s).", offlineTransitions)
	}

	return AlertSummary{
		NodeName:             nodeName,
		Severity:             severity,
		Summary:              summary,
		KeyFindings:          findings,
		OfflineTransitions:   offlineTransitions,
		AvgAvailabilityScore: round2(avgScore),
		AvgGPUPressure:       round2(avgGPUPressure),
		AvgActiveUsers:       round2(avgUsers),
		VolatilityScore:      round2(volatility),
	}
}

func tierSummary(offlineRatio, busyRatio float64) string {
	if offlineRatio > 0.2 {
		return "mostly unstable/offline"
	}
	if busyRatio > 0.6 {
		return "consistently busy"
	}
	return "relatively stable"
}

func windowLabel(window time.Duration) string {
	switch {
	case window >= time.Hour:
		return "1h"
	case window >= 30*time.Minute:
		return "30m"
	default:
		return window.String()
	}
}
