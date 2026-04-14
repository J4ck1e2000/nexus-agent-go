package ai

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// ClusterDataProvider exposes normalized node snapshots from gateway aggregate view.
type ClusterDataProvider interface {
	ListNodeSnapshots(ctx context.Context) ([]NodeSnapshot, error)
}

// HistoryProvider exposes compact node history snapshots.
type HistoryProvider interface {
	QueryNodeHistory(ctx context.Context, nodeName *string, window time.Duration) ([]NodeHistorySnapshot, error)
}

// Toolbox groups domain tools shared by rule and agent executors.
type Toolbox struct {
	mu              sync.RWMutex
	dataProvider    ClusterDataProvider
	historyProvider HistoryProvider
	scheduler       *Scheduler
	explainer       *AnomalyExplainer
	historyAnalyzer *HistoryAnalyzer
	knowledge       KnowledgeSearcher
	knowledgeReady  bool
	retrievalStats  *RetrievalStatsCollector
	knowledgeTopK   int
}

// ToolboxOptions defines dependencies of Toolbox.
type ToolboxOptions struct {
	DataProvider       ClusterDataProvider
	HistoryProvider    HistoryProvider
	Scheduler          *Scheduler
	Explainer          *AnomalyExplainer
	HistoryAnalyzer    *HistoryAnalyzer
	KnowledgeRetriever *KnowledgeRetriever
	KnowledgeSearcher  KnowledgeSearcher
	KnowledgeDocuments int
	KnowledgeChunks    int
	RetrievalStats     *RetrievalStatsCollector
	KnowledgeTopK      int
}

// NewToolbox creates a toolbox with defaults.
func NewToolbox(opts ToolboxOptions) *Toolbox {
	scheduler := opts.Scheduler
	if scheduler == nil {
		scheduler = NewScheduler()
	}
	explainer := opts.Explainer
	if explainer == nil {
		explainer = NewAnomalyExplainer()
	}
	historyAnalyzer := opts.HistoryAnalyzer
	if historyAnalyzer == nil {
		historyAnalyzer = NewHistoryAnalyzer()
	}
	knowledgeTopK := opts.KnowledgeTopK
	if knowledgeTopK <= 0 {
		knowledgeTopK = defaultKnowledgeTopK
	}
	retrievalStats := opts.RetrievalStats
	if retrievalStats == nil {
		retrievalStats = NewRetrievalStatsCollector(false)
	}

	searcher := opts.KnowledgeSearcher
	loadedDocs := opts.KnowledgeDocuments
	loadedChunks := opts.KnowledgeChunks
	if opts.KnowledgeRetriever != nil {
		if searcher == nil {
			searcher = NewLocalKnowledgeSearcher(opts.KnowledgeRetriever)
		}
		if loadedDocs <= 0 {
			loadedDocs = opts.KnowledgeRetriever.DocumentCount()
		}
		if loadedChunks <= 0 {
			loadedChunks = opts.KnowledgeRetriever.ChunkCount()
		}
	}
	knowledgeEnabled := searcher != nil
	retrievalStats.UpdateKnowledgeState(knowledgeEnabled, loadedDocs, loadedChunks, time.Now())

	return &Toolbox{
		dataProvider:    opts.DataProvider,
		historyProvider: opts.HistoryProvider,
		scheduler:       scheduler,
		explainer:       explainer,
		historyAnalyzer: historyAnalyzer,
		knowledge:       searcher,
		knowledgeReady:  knowledgeEnabled,
		retrievalStats:  retrievalStats,
		knowledgeTopK:   knowledgeTopK,
	}
}

// KnownNodeNames returns all known node names for intent recognition hints.
func (t *Toolbox) KnownNodeNames(ctx context.Context) ([]string, error) {
	nodes, err := t.listNodeSnapshots(ctx)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(nodes))
	for _, node := range nodes {
		if strings.TrimSpace(node.Name) == "" {
			continue
		}
		names = append(names, node.Name)
	}
	sort.Strings(names)
	return names, nil
}

// GetNodeMetrics returns full normalized snapshot for one node.
func (t *Toolbox) GetNodeMetrics(ctx context.Context, nodeName string) (NodeSnapshot, error) {
	node, err := t.findNodeByName(ctx, nodeName)
	if err != nil {
		return NodeSnapshot{}, err
	}
	return node, nil
}

// ListIdleNodes returns ranked idle node candidates.
func (t *Toolbox) ListIdleNodes(ctx context.Context, minFreeVRAMGB *float64, limit int) ([]NodeCandidate, error) {
	nodes, err := t.listNodeSnapshots(ctx)
	if err != nil {
		return nil, err
	}
	return t.scheduler.RankIdleNodes(nodes, minFreeVRAMGB, limit), nil
}

// GetGPUProcesses returns GPU-related processes on one node.
func (t *Toolbox) GetGPUProcesses(ctx context.Context, nodeName string) ([]GPUProcess, error) {
	node, err := t.findNodeByName(ctx, nodeName)
	if err != nil {
		return nil, err
	}

	processes := make([]GPUProcess, 0, len(node.Processes))
	for _, process := range node.Processes {
		if process.GPUIndex == nil {
			continue
		}
		processes = append(processes, process)
	}
	sort.SliceStable(processes, func(i, j int) bool {
		left := 0
		if processes[i].VRAMUsedMB != nil {
			left = *processes[i].VRAMUsedMB
		}
		right := 0
		if processes[j].VRAMUsedMB != nil {
			right = *processes[j].VRAMUsedMB
		}
		if left == right {
			return processes[i].CPUPercent > processes[j].CPUPercent
		}
		return left > right
	})
	return processes, nil
}

// GetNodeSummary returns a readable summary structure for one node.
func (t *Toolbox) GetNodeSummary(ctx context.Context, nodeName string) (NodeSummary, error) {
	node, err := t.findNodeByName(ctx, nodeName)
	if err != nil {
		return NodeSummary{}, err
	}

	topProcess := ""
	topScore := -1.0
	for _, process := range node.Processes {
		vramWeight := 0.0
		if process.VRAMUsedMB != nil {
			vramWeight = float64(*process.VRAMUsedMB) / 256
		}
		score := process.CPUPercent + process.MemoryPercent + vramWeight
		if score > topScore {
			topScore = score
			topProcess = strings.TrimSpace(process.Command)
		}
	}

	riskFlags := buildNodeRiskFlags(node)
	healthText := fmt.Sprintf(
		"%s is %s with availability score %d (%s). CPU %.1f%%, RAM %.1f%%, active users %d.",
		node.Name,
		node.Status,
		node.AvailabilityScore,
		node.AvailabilityTier,
		ptrToFloat(node.CPUUsage),
		ptrToFloat(node.RAMPercent),
		node.ActiveUserCount,
	)
	if node.DataAgeSec != nil {
		healthText += fmt.Sprintf(" Data age %.0fs.", *node.DataAgeSec)
	} else {
		healthText += " Data age unavailable."
	}

	gpuText := fmt.Sprintf(
		"GPU %d total, busy %d, idle %d, avg utilization %.1f%%, avg memory %.1f%%, used %.1f/%.1fGB.",
		node.GPUSummary.GPUCount,
		node.GPUSummary.BusyGPUCount,
		node.GPUSummary.IdleGPUCount,
		ptrToFloat(node.GPUSummary.AvgUtilization),
		ptrToFloat(node.GPUSummary.AvgMemoryPercent),
		node.GPUSummary.UsedMemoryGB,
		node.GPUSummary.TotalMemoryGB,
	)

	processText := "No obvious GPU process."
	if topProcess != "" {
		processText = "Top process: " + topProcess
	}

	return NodeSummary{
		NodeName:          node.Name,
		Status:            node.Status,
		DataAgeSec:        node.DataAgeSec,
		AvailabilityScore: node.AvailabilityScore,
		AvailabilityTier:  node.AvailabilityTier,
		CPUUsage:          node.CPUUsage,
		RAMPercent:        node.RAMPercent,
		GPUCount:          node.GPUSummary.GPUCount,
		BusyGPUCount:      node.GPUSummary.BusyGPUCount,
		IdleGPUCount:      node.GPUSummary.IdleGPUCount,
		AvgGPUUtilization: node.GPUSummary.AvgUtilization,
		AvgGPUMemoryPct:   node.GPUSummary.AvgMemoryPercent,
		TotalGPUMemoryGB:  node.GPUSummary.TotalMemoryGB,
		UsedGPUMemoryGB:   node.GPUSummary.UsedMemoryGB,
		ActiveUserCount:   node.ActiveUserCount,
		TopProcess:        topProcess,
		HealthText:        healthText,
		GPUText:           gpuText,
		ProcessText:       processText,
		RiskFlags:         riskFlags,
	}, nil
}

// GetAlertHistory summarizes history snapshots in the requested window.
func (t *Toolbox) GetAlertHistory(ctx context.Context, nodeName *string, window string) (AlertHistoryResult, error) {
	duration := parseWindow(window)
	if duration <= 0 {
		duration = 30 * time.Minute
	}
	if t.historyProvider == nil {
		return AlertHistoryResult{
			WindowLabel: windowLabel(duration),
			Summaries:   []AlertSummary{},
		}, nil
	}

	snapshots, err := t.historyProvider.QueryNodeHistory(ctx, nodeName, duration)
	if err != nil {
		return AlertHistoryResult{}, err
	}
	return t.historyAnalyzer.Summarize(snapshots, duration), nil
}

// RecommendNodesForJob ranks nodes based on scheduling requirements.
func (t *Toolbox) RecommendNodesForJob(ctx context.Context, requirement JobRequirement, limit int) ([]NodeCandidate, error) {
	nodes, err := t.listNodeSnapshots(ctx)
	if err != nil {
		return nil, err
	}
	return t.scheduler.RecommendNodesForJob(nodes, requirement, limit), nil
}

// ExplainNodeAnomaly runs deterministic anomaly explanation for one node.
func (t *Toolbox) ExplainNodeAnomaly(ctx context.Context, nodeName string) (AnomalyExplanation, error) {
	node, err := t.findNodeByName(ctx, nodeName)
	if err != nil {
		return AnomalyExplanation{}, err
	}
	return t.explainer.Explain(node), nil
}

// SearchKnowledge runs pluggable knowledge retrieval (local/qdrant/auto).
func (t *Toolbox) SearchKnowledge(ctx context.Context, query string, limit int) ([]KnowledgeHit, RetrievalMeta, error) {
	if t == nil {
		meta := RetrievalMeta{
			Query:    strings.TrimSpace(query),
			TopK:     normalizeKnowledgeTopK(limit),
			Strategy: localKnowledgeStrategy,
		}
		return nil, meta, nil
	}

	t.mu.RLock()
	searcher := t.knowledge
	topKDefault := t.knowledgeTopK
	stats := t.retrievalStats
	t.mu.RUnlock()

	topK := limit
	if topK <= 0 {
		topK = defaultKnowledgeTopK
	}
	if topKDefault > 0 && limit <= 0 {
		topK = topKDefault
	}
	meta := RetrievalMeta{
		Query:    strings.TrimSpace(query),
		TopK:     topK,
		Strategy: localKnowledgeStrategy,
	}
	if searcher != nil && strings.TrimSpace(searcher.StrategyName()) != "" {
		meta.Strategy = searcher.StrategyName()
	}

	if searcher == nil {
		if stats != nil {
			stats.RecordSearch(meta)
		}
		return nil, meta, nil
	}

	hits, searchMeta, err := searcher.Search(ctx, query, topK)
	if err != nil {
		return nil, searchMeta, err
	}
	if stats != nil {
		stats.RecordSearch(searchMeta)
	}
	return hits, searchMeta, nil
}

// KnowledgeStats returns current online retrieval stats snapshot.
func (t *Toolbox) KnowledgeStats(ctx context.Context) RetrievalStats {
	_ = ctx
	if t == nil || t.retrievalStats == nil {
		return RetrievalStats{}
	}
	return t.retrievalStats.Snapshot()
}

// HasKnowledge indicates whether the toolbox has enabled knowledge retrieval.
func (t *Toolbox) HasKnowledge() bool {
	if t == nil {
		return false
	}
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.knowledgeReady
}

// ReplaceKnowledgeSearcher hot-swaps runtime knowledge backend and updates online stats state.
func (t *Toolbox) ReplaceKnowledgeSearcher(searcher KnowledgeSearcher, loadedDocuments, loadedChunks, topK int) {
	if t == nil {
		return
	}
	t.mu.Lock()
	t.knowledge = searcher
	t.knowledgeReady = searcher != nil
	if topK > 0 {
		t.knowledgeTopK = topK
	}
	stats := t.retrievalStats
	t.mu.Unlock()

	if stats != nil {
		stats.UpdateKnowledgeState(searcher != nil, loadedDocuments, loadedChunks, time.Now())
	}
}

func summarizeKnowledgeHits(hits []KnowledgeHit) []KnowledgeHitSummary {
	if len(hits) == 0 {
		return nil
	}
	result := make([]KnowledgeHitSummary, 0, len(hits))
	for _, hit := range hits {
		result = append(result, KnowledgeHitSummary{
			Title:      hit.Title,
			Category:   hit.Category,
			Snippet:    hit.Snippet,
			SourcePath: hit.SourcePath,
		})
	}
	return result
}

func (t *Toolbox) findNodeByName(ctx context.Context, nodeName string) (NodeSnapshot, error) {
	name := strings.TrimSpace(nodeName)
	if name == "" {
		return NodeSnapshot{}, ErrNodeNameRequired
	}

	nodes, err := t.listNodeSnapshots(ctx)
	if err != nil {
		return NodeSnapshot{}, err
	}
	if len(nodes) == 0 {
		return NodeSnapshot{}, ErrNoNodesAvailable
	}

	for _, node := range nodes {
		if strings.EqualFold(node.Name, name) {
			return node, nil
		}
	}
	for _, node := range nodes {
		if strings.Contains(strings.ToLower(node.Name), strings.ToLower(name)) {
			return node, nil
		}
	}
	return NodeSnapshot{}, fmt.Errorf("%w: %s", ErrNodeNotFound, name)
}

func (t *Toolbox) listNodeSnapshots(ctx context.Context) ([]NodeSnapshot, error) {
	if t == nil || t.dataProvider == nil {
		return nil, ErrClusterDataProviderNil
	}
	nodes, err := t.dataProvider.ListNodeSnapshots(ctx)
	if err != nil {
		return nil, err
	}
	return nodes, nil
}

func buildNodeRiskFlags(node NodeSnapshot) []string {
	flags := make([]string, 0, 6)
	if !strings.EqualFold(node.Status, "online") {
		flags = append(flags, "offline")
	}
	if node.DataAgeSec == nil || ptrToFloat(node.DataAgeSec) > stalePenaltyThresholdSec {
		flags = append(flags, "stale_data")
	}
	if ptrToFloat(node.CPUUsage) >= 85 {
		flags = append(flags, "high_cpu")
	}
	if ptrToFloat(node.RAMPercent) >= 90 {
		flags = append(flags, "high_ram")
	}
	if node.GPUSummary.GPUPressure >= 85 || node.GPUSummary.BusyRatio >= 0.9 {
		flags = append(flags, "high_gpu_pressure")
	}
	if node.ActiveUserCount >= 3 {
		flags = append(flags, "many_active_users")
	}
	sort.Strings(flags)
	return flags
}
