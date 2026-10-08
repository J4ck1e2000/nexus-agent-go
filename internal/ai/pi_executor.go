package ai

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"nexus-agent-go/internal/runtime"
)

// PiRuntimeExecutor executes AI queries on the external TypeScript Pi Runtime
// and maps its NDJSON events onto the gateway AI stream contract.
type PiRuntimeExecutor struct {
	client       *runtime.Client
	manager      *runtime.Manager
	config       runtime.Config
	toolbox      *Toolbox
	allowedTools []string
	modelID      string
}

// PiRuntimeExecutorOptions wires the executor dependencies.
type PiRuntimeExecutorOptions struct {
	Client       *runtime.Client
	Manager      *runtime.Manager
	Config       runtime.Config
	Toolbox      *Toolbox
	AllowedTools []string
	// ModelID selects the runtime model profile; empty means the runtime default.
	ModelID string
}

// NewPiRuntimeExecutor builds the executor; nil toolbox yields an empty tool set.
func NewPiRuntimeExecutor(opts PiRuntimeExecutorOptions) *PiRuntimeExecutor {
	toolbox := opts.Toolbox
	if toolbox == nil {
		toolbox = NewToolbox(ToolboxOptions{})
	}
	return &PiRuntimeExecutor{
		client:       opts.Client,
		manager:      opts.Manager,
		config:       opts.Config,
		toolbox:      toolbox,
		allowedTools: append([]string(nil), opts.AllowedTools...),
		modelID:      strings.TrimSpace(opts.ModelID),
	}
}

// Execute runs one query and returns the buffered response.
func (e *PiRuntimeExecutor) Execute(ctx context.Context, req AIQueryRequest) (AIQueryResponse, error) {
	return e.ExecuteStream(ctx, req, nil)
}

// Health probes the configured Pi Runtime.
func (e *PiRuntimeExecutor) Health(ctx context.Context) error {
	if e == nil || e.client == nil {
		return fmt.Errorf("%w: runtime client not configured", runtime.ErrRuntimeUnavailable)
	}
	return e.client.Health(ctx)
}

// ExecuteStream runs one query on the Pi runtime, emitting gateway events.
// A runtime-unreachable failure before the first event maps to
// runtime.ErrRuntimeUnavailable so callers can fall back to the legacy path.
func (e *PiRuntimeExecutor) ExecuteStream(ctx context.Context, req AIQueryRequest, emit func(AIStreamEvent) error) (AIQueryResponse, error) {
	query := strings.TrimSpace(req.Query)
	if query == "" {
		return AIQueryResponse{}, ErrInvalidQuery
	}
	if e.client == nil || e.config.RuntimeURL == "" {
		return AIQueryResponse{}, fmt.Errorf("%w: runtime not configured", runtime.ErrRuntimeUnavailable)
	}

	lang := detectResponseLanguage(query)
	runID := newRunID()
	deadline := time.Now().Add(e.config.RunTimeout)

	knownNodes := e.knownNodes(ctx)
	principal, ok := runtime.PrincipalFromCtx(ctx)
	if !ok {
		principal = runtime.AnonymousPrincipal()
	}

	allowedTools := e.allowedTools
	if len(allowedTools) == 0 {
		allowedTools = []string{}
	}
	credential, err := runtime.SignRunCredential(runtime.RunCredential{
		RunID:    runID,
		UserID:   principal.UserID,
		Username: principal.Username,
		Role:     principal.Role,
	}, e.config.RunTokenSecret)
	if err != nil {
		return AIQueryResponse{}, fmt.Errorf("sign run credential failed: %w", err)
	}

	// Budget accounting on the gateway side mirrors the runtime policy.
	if _, err := e.manager.StartRun(runID, runID, principal.UserID, principal.Username, principal.Role, allowedTools, e.config.MaxToolCalls); err != nil {
		return AIQueryResponse{}, fmt.Errorf("register run failed: %w", err)
	}
	// Best-effort cleanup: any early return that has not reached a terminal
	// state (stream closed, emit error, cancellation) must not leave the run
	// registered as active. Terminal transitions are idempotent no-ops.
	defer func() { _ = e.manager.EnsureTerminal(runID, runtime.RunStateFailed) }()

	startRequest := runtime.StartRunRequest{
		ProtocolVersion: runtime.ProtocolVersion(),
		RunID:           runID,
		SessionID:       runID,
		Input:           runtime.StartRunInput{Message: query},
		Context: runtime.RunContext{
			KnownNodes: knownNodes,
			LocaleHint: string(lang),
			// 多轮上下文：服务层经 ctx 注入的最近对话历史（可为 nil）。
			RecentEntries: runtime.RecentHistoryFromCtx(ctx),
		},
		Policy: runtime.RunPolicy{
			EnabledToolNames: allowedTools,
			DeadlineAt:       deadline.UTC().Format(time.RFC3339),
		},
		ModelProfile: runtime.ModelProfile{
			Provider: "nexus-llm",
			ModelID:  e.modelID,
		},
	}

	events, closeStream, err := e.client.StartRun(ctx, startRequest, credential)
	if err != nil {
		// Pre-start failure: no events emitted yet, fallback is still allowed.
		return AIQueryResponse{}, err
	}
	defer closeStream()

	// Client disconnect / deadline cancellation must reach the runtime.
	cancelCtx, cancelRun := context.WithCancel(context.WithoutCancel(ctx))
	defer cancelRun()
	go func() {
		select {
		case <-ctx.Done():
			if beginErr := e.manager.BeginCancel(runID); beginErr == nil {
				_ = e.client.CancelRun(cancelCtx, runID, "client_disconnected")
			}
		case <-cancelCtx.Done():
		}
	}()
	if err := e.manager.MarkRunning(runID); err != nil && !errors.Is(err, runtime.ErrRunAlreadyFinished) {
		return AIQueryResponse{}, fmt.Errorf("mark run running failed: %w", err)
	}

	if err := emitPiStatus(emit, lang, "running",
		localizedText(lang, "Pi 运行时已接管本次问答...", "Pi runtime took over this query...")); err != nil {
		return AIQueryResponse{}, err
	}

	collector := newPiRunCollector(runID)
	for {
		select {
		case event, ok := <-events:
			if !ok {
				// Stream closed without a terminal event: fail, never succeed.
				_ = e.manager.EnsureTerminal(runID, runtime.RunStateFailed)
				return AIQueryResponse{}, fmt.Errorf("%w: no terminal event received", ErrAgentUnavailable)
			}
			if err := ctx.Err(); err != nil {
				return AIQueryResponse{}, err
			}
			if done, resp, runErr := e.handleEvent(ctx, event, collector, emit, lang); done {
				return resp, runErr
			}
		case <-ctx.Done():
			return AIQueryResponse{}, ctx.Err()
		}
	}
}

// handleEvent maps one runtime event; done=true means the run reached a terminal state.
func (e *PiRuntimeExecutor) handleEvent(
	ctx context.Context,
	event runtime.Event,
	collector *piRunCollector,
	emit func(AIStreamEvent) error,
	lang responseLanguage,
) (bool, AIQueryResponse, error) {
	switch event.Type {
	case runtime.EventRunStarted:
		return false, AIQueryResponse{}, nil

	case runtime.EventMessageDelta:
		var payload runtime.MessageDeltaPayload
		if err := decodePayload(event.Payload, &payload); err != nil {
			return false, AIQueryResponse{}, nil
		}
		collector.appendText(payload.Text)
		if err := emitPiDelta(emit, payload.Text); err != nil {
			return true, AIQueryResponse{}, err
		}
		return false, AIQueryResponse{}, nil

	case runtime.EventMessageCompleted:
		var payload runtime.MessageCompletedPayload
		if err := decodePayload(event.Payload, &payload); err == nil {
			collector.recordCompletedMessage(payload.Text)
		}
		return false, AIQueryResponse{}, nil

	case runtime.EventToolStarted:
		var payload runtime.ToolStartedPayload
		if err := decodePayload(event.Payload, &payload); err != nil {
			return false, AIQueryResponse{}, nil
		}
		collector.recordToolStarted(payload)
		if err := emitPiStatus(emit, lang, "tooling", toolStatusMessage(payload.ToolName, lang)); err != nil {
			return true, AIQueryResponse{}, err
		}
		return false, AIQueryResponse{}, nil

	case runtime.EventToolCompleted:
		var payload runtime.ToolCompletedPayload
		if err := decodePayload(event.Payload, &payload); err != nil {
			return false, AIQueryResponse{}, nil
		}
		collector.recordToolCompleted(payload)
		return false, AIQueryResponse{}, nil

	case runtime.EventRunCompleted:
		var payload runtime.RunCompletedPayload
		if err := decodePayload(event.Payload, &payload); err != nil {
			return true, AIQueryResponse{}, fmt.Errorf("decode run.completed payload failed: %w", err)
		}
		resp := collector.buildResponse(payload.Text)
		if strings.TrimSpace(resp.Answer) == "" {
			_ = e.manager.MarkTerminal(event.RunID, runtime.RunStateFailed)
			return true, AIQueryResponse{}, fmt.Errorf("%w: runtime completed without a final answer", ErrAgentUnavailable)
		}
		_ = e.manager.MarkTerminal(event.RunID, runtime.RunStateCompleted)
		return true, resp, nil

	case runtime.EventRunFailed:
		var payload runtime.RunFailedPayload
		if err := decodePayload(event.Payload, &payload); err != nil {
			payload.Error = "unknown runtime failure"
		}
		_ = e.manager.MarkTerminal(event.RunID, runtime.RunStateFailed)
		return true, AIQueryResponse{}, fmt.Errorf("pi run failed: %s", payload.Error)

	case runtime.EventRunCancelled:
		_ = e.manager.MarkTerminal(event.RunID, runtime.RunStateCancelled)
		return true, AIQueryResponse{}, context.Canceled

	default:
		// Unknown event types are tolerated for forward compatibility.
		return false, AIQueryResponse{}, nil
	}
}

func (e *PiRuntimeExecutor) knownNodes(ctx context.Context) []string {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	names, err := e.toolbox.KnownNodeNames(ctx)
	if err != nil {
		return nil
	}
	return names
}

func emitPiStatus(emit func(AIStreamEvent) error, lang responseLanguage, phase, message string) error {
	if emit == nil {
		return nil
	}
	return emit(AIStreamEvent{Event: StreamEventStatus, Phase: phase, Message: message})
}

func emitPiDelta(emit func(AIStreamEvent) error, text string) error {
	if emit == nil || text == "" {
		return nil
	}
	return emit(AIStreamEvent{Event: StreamEventDelta, Text: text})
}

func decodePayload(raw json.RawMessage, target any) error {
	if len(raw) == 0 {
		return errors.New("empty payload")
	}
	return json.Unmarshal(raw, target)
}

func newRunID() string {
	buffer := make([]byte, 16)
	if _, err := rand.Read(buffer); err != nil {
		return fmt.Sprintf("run-%d", time.Now().UnixNano())
	}
	return "run-" + hex.EncodeToString(buffer)
}

// piRunCollector accumulates evidence across one run's events.
type piRunCollector struct {
	runID         string
	text          strings.Builder
	completedText string
	toolCalls     []ToolCallRecord
	relatedNodes  map[string]struct{}
	warnings      []string
	knowledgeHits []KnowledgeHitSummary
	retrievalMeta *RetrievalMeta
}

func newPiRunCollector(runID string) *piRunCollector {
	return &piRunCollector{
		runID:        runID,
		relatedNodes: map[string]struct{}{},
	}
}

func (c *piRunCollector) appendText(text string) {
	c.text.WriteString(text)
}

func (c *piRunCollector) recordCompletedMessage(text string) {
	if strings.TrimSpace(text) != "" {
		c.completedText = text
	}
}

func (c *piRunCollector) recordToolStarted(payload runtime.ToolStartedPayload) {
	name := strings.TrimSpace(payload.ToolName)
	if name == "" {
		return
	}
	record := ToolCallRecord{Name: name, Args: payload.Arguments}
	c.toolCalls = append(c.toolCalls, record)
	trackRelatedNodesFromArgs(record, c.relatedNodes)
}

func (c *piRunCollector) recordToolCompleted(payload runtime.ToolCompletedPayload) {
	name := strings.TrimSpace(payload.ToolName)
	if payload.Meta != nil && payload.Meta.Stale {
		c.warnings = append(c.warnings, fmt.Sprintf("tool %s returned stale data", name))
	}
	if payload.Meta != nil && payload.Meta.Truncated {
		c.warnings = append(c.warnings, fmt.Sprintf("tool %s returned truncated data", name))
	}
	if !payload.OK {
		message := strings.TrimSpace(payload.Error)
		if message == "" {
			message = "tool failed"
		}
		c.warnings = append(c.warnings, fmt.Sprintf("tool %s failed: %s", name, message))
		return
	}
	c.collectToolEvidence(name, payload.Result)
}

func (c *piRunCollector) collectToolEvidence(toolName string, result map[string]any) {
	if result == nil {
		return
	}
	switch toolName {
	case "search_knowledge_base":
		if hits := asKnowledgeHitSummarySlice(result["hits"]); len(hits) > 0 {
			c.knowledgeHits = dedupeKnowledgeHitSummaries(append(c.knowledgeHits, hits...))
		}
		if retrieval := asRetrievalMeta(result["retrieval"]); retrieval != nil {
			c.retrievalMeta = retrieval
		}
	case "get_node_metrics":
		if node, ok := result["node"].(map[string]any); ok {
			c.trackNode(asString(node["name"]))
		}
	case "get_node_summary":
		if summary, ok := result["summary"].(map[string]any); ok {
			c.trackNode(asString(summary["node_name"]))
		}
	case "explain_node_anomaly":
		if explanation, ok := result["explanation"].(map[string]any); ok {
			c.trackNode(asString(explanation["node_name"]))
		}
	case "list_idle_nodes", "recommend_nodes_for_job":
		c.trackCandidates(result["candidates"])
	case "get_alert_history":
		if history, ok := result["history"].(map[string]any); ok {
			c.trackCandidates(history["summaries"])
		}
	}
}

func (c *piRunCollector) trackCandidates(value any) {
	items, ok := value.([]any)
	if !ok {
		return
	}
	for _, item := range items {
		if obj, ok := item.(map[string]any); ok {
			c.trackNode(asString(obj["node_name"]))
		}
	}
}

func (c *piRunCollector) trackNode(name string) {
	if normalized := strings.TrimSpace(name); normalized != "" {
		c.relatedNodes[normalized] = struct{}{}
	}
}

func (c *piRunCollector) buildResponse(finalText string) AIQueryResponse {
	text := strings.TrimSpace(finalText)
	if text == "" {
		text = c.completedText
	}
	if text == "" {
		text = c.text.String()
	}

	modelFinal := parseAgentFinalContent(text)
	if strings.TrimSpace(modelFinal.Answer) == "" {
		modelFinal.Answer = sanitizeFinalText(text)
	}
	if strings.TrimSpace(modelFinal.ReasoningSummary) == "" {
		modelFinal.ReasoningSummary = summarizeToolCalls(c.toolCalls, responseLanguageZH)
	}

	for _, nodeName := range modelFinal.RelatedNodes {
		c.trackNode(nodeName)
	}
	c.warnings = append(c.warnings, modelFinal.Warnings...)
	if len(modelFinal.KnowledgeHits) > 0 {
		c.knowledgeHits = dedupeKnowledgeHitSummaries(append(c.knowledgeHits, modelFinal.KnowledgeHits...))
	}
	if c.retrievalMeta == nil {
		c.retrievalMeta = modelFinal.Retrieval
	}

	relatedNodes := make([]string, 0, len(c.relatedNodes))
	for nodeName := range c.relatedNodes {
		relatedNodes = append(relatedNodes, nodeName)
	}
	sort.Strings(relatedNodes)

	return AIQueryResponse{
		Answer:           sanitizeFinalText(modelFinal.Answer),
		ReasoningSummary: sanitizeFinalText(modelFinal.ReasoningSummary),
		Mode:             AIModeAgent,
		ToolCalls:        c.toolCalls,
		RelatedNodes:     relatedNodes,
		Warnings:         uniqueStrings(c.warnings),
		KnowledgeHits:    c.knowledgeHits,
		Retrieval:        c.retrievalMeta,
	}
}
