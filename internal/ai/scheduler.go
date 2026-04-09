package ai

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

const stalePenaltyThresholdSec = 20.0

// Scheduler scores and ranks nodes for idle/scheduling recommendations.
type Scheduler struct{}

// NewScheduler creates a scheduler scoring helper.
func NewScheduler() *Scheduler {
	return &Scheduler{}
}

// RankIdleNodes returns online candidates sorted by deterministic idle score.
func (s *Scheduler) RankIdleNodes(nodes []NodeSnapshot, minFreeVRAMGB *float64, limit int) []NodeCandidate {
	req := JobRequirement{
		PreferFreshData: true,
	}
	if minFreeVRAMGB != nil {
		req.MinFreeVRAMGB = *minFreeVRAMGB
		req.GPUCount = 1
	}
	return s.RecommendNodesForJob(nodes, req, limit)
}

// RecommendNodesForJob returns ranked candidates that satisfy the job requirement.
func (s *Scheduler) RecommendNodesForJob(nodes []NodeSnapshot, req JobRequirement, limit int) []NodeCandidate {
	if limit <= 0 {
		limit = 3
	}

	candidates := make([]NodeCandidate, 0, len(nodes))
	for _, node := range nodes {
		candidate, ok := s.scoreNode(node, req)
		if !ok {
			continue
		}
		candidates = append(candidates, candidate)
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].Score == candidates[j].Score {
			if candidates[i].MaxFreeVRAMGB == candidates[j].MaxFreeVRAMGB {
				if candidates[i].IdleGPUCount == candidates[j].IdleGPUCount {
					return candidates[i].NodeName < candidates[j].NodeName
				}
				return candidates[i].IdleGPUCount > candidates[j].IdleGPUCount
			}
			return candidates[i].MaxFreeVRAMGB > candidates[j].MaxFreeVRAMGB
		}
		return candidates[i].Score > candidates[j].Score
	})

	if len(candidates) > limit {
		return candidates[:limit]
	}
	return candidates
}

func (s *Scheduler) scoreNode(node NodeSnapshot, req JobRequirement) (NodeCandidate, bool) {
	if !strings.EqualFold(node.Status, "online") {
		return NodeCandidate{}, false
	}
	if node.GPUSummary.GPUCount <= 0 {
		return NodeCandidate{}, false
	}

	cpu := ptrToFloat(node.CPUUsage)
	ram := ptrToFloat(node.RAMPercent)
	age := ptrToFloat(node.DataAgeSec)
	maxFree := maxFreeVRAMGB(node.GPUs, node.GPUSummary)
	qualified := countQualifiedGPU(node.GPUs, node.GPUSummary, req.MinFreeVRAMGB)

	if req.MinFreeVRAMGB > 0 && qualified == 0 {
		return NodeCandidate{}, false
	}
	requiredGPUCount := req.GPUCount
	if requiredGPUCount <= 0 && req.MinFreeVRAMGB > 0 {
		requiredGPUCount = 1
	}
	if requiredGPUCount > 0 && qualified < requiredGPUCount {
		return NodeCandidate{}, false
	}

	score := float64(node.AvailabilityScore)
	score += float64(node.GPUSummary.IdleGPUCount) * 4
	score += maxFree * 1.8
	score -= cpu * 0.25
	score -= ram * 0.20
	score -= node.GPUSummary.BusyRatio * 14
	score -= float64(node.ActiveUserCount) * 4
	score -= node.GPUSummary.GPUPressure * 0.12

	if req.PreferLowCPU {
		score -= cpu * 0.12
	}
	if req.PreferLowRAM {
		score -= ram * 0.12
	}
	if req.PreferFewUsers {
		score -= float64(node.ActiveUserCount) * 3
	}
	if req.PreferFreshData && age > stalePenaltyThresholdSec {
		score -= 20
	}
	if age == 0 {
		score -= 8
	}

	reasons := []string{
		fmt.Sprintf("availability score %d", node.AvailabilityScore),
		fmt.Sprintf("idle GPUs %d", node.GPUSummary.IdleGPUCount),
		fmt.Sprintf("max free VRAM %.1fGB", maxFree),
	}
	if age > 0 {
		reasons = append(reasons, fmt.Sprintf("data age %.0fs", age))
	}
	if req.MinFreeVRAMGB > 0 {
		reasons = append(reasons, fmt.Sprintf("qualified GPUs %d (>= %.1fGB)", qualified, req.MinFreeVRAMGB))
	}

	return NodeCandidate{
		NodeName:          node.Name,
		Score:             round2(score),
		AvailabilityScore: node.AvailabilityScore,
		IdleGPUCount:      node.GPUSummary.IdleGPUCount,
		MaxFreeVRAMGB:     round2(maxFree),
		QualifiedGPUCount: qualified,
		ActiveUserCount:   node.ActiveUserCount,
		CPUUsage:          round2(cpu),
		RAMPercent:        round2(ram),
		DataAgeSec:        round2(age),
		Reasons:           reasons,
	}, true
}

func maxFreeVRAMGB(gpus []GPUCard, summary NodeGPUSummary) float64 {
	if len(gpus) == 0 {
		totalFree := summary.TotalMemoryGB - summary.UsedMemoryGB
		if totalFree < 0 {
			return 0
		}
		if summary.GPUCount <= 1 {
			return totalFree
		}
		return totalFree / float64(summary.GPUCount)
	}

	maxFree := 0.0
	for _, gpu := range gpus {
		free := gpu.EstimatedFreeGB
		if free <= 0 && gpu.MemoryTotalGB > 0 {
			free = gpu.MemoryTotalGB - gpu.MemoryUsedGB
		}
		if free > maxFree {
			maxFree = free
		}
	}
	return maxFree
}

func countQualifiedGPU(gpus []GPUCard, summary NodeGPUSummary, minFreeVRAMGB float64) int {
	if minFreeVRAMGB <= 0 {
		if len(gpus) > 0 {
			return len(gpus)
		}
		return summary.GPUCount
	}
	if len(gpus) == 0 {
		maxFree := maxFreeVRAMGB(gpus, summary)
		if maxFree >= minFreeVRAMGB {
			if summary.GPUCount <= 0 {
				return 1
			}
			return summary.GPUCount
		}
		return 0
	}

	count := 0
	for _, gpu := range gpus {
		free := gpu.EstimatedFreeGB
		if free <= 0 {
			free = gpu.MemoryTotalGB - gpu.MemoryUsedGB
		}
		if free >= minFreeVRAMGB {
			count++
		}
	}
	return count
}

func ptrToFloat(v *float64) float64 {
	if v == nil {
		return 0
	}
	return *v
}

func round2(v float64) float64 {
	return math.Round(v*100) / 100
}
