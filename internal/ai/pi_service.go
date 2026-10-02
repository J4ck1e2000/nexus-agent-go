package ai

import (
	"context"
	"errors"
	"strings"
	"time"

	"nexus-agent-go/internal/runtime"
)

// ExecutorModePi identifies the Pi Runtime executor in health responses.
const ExecutorModePi = "pi"

// PiQueryService implements the gateway AI contract using only the Pi Runtime.
type PiQueryService struct {
	support *Service
	pi      *PiRuntimeExecutor
}

// NewPiQueryService wires the Pi Runtime and shared AI support services.
func NewPiQueryService(support *Service, pi *PiRuntimeExecutor) *PiQueryService {
	return &PiQueryService{support: support, pi: pi}
}

// Query executes one request on the Pi Runtime. Runtime failures are returned to the caller.
func (s *PiQueryService) Query(ctx context.Context, req AIQueryRequest) (AIQueryResponse, error) {
	if s.support == nil || !s.support.Enabled() {
		return AIQueryResponse{}, ErrServiceDisabled
	}
	if s.pi == nil {
		return AIQueryResponse{}, runtime.ErrRuntimeUnavailable
	}
	return s.pi.Execute(ctx, req)
}

// QueryStream executes one streaming request on the Pi Runtime.
func (s *PiQueryService) QueryStream(ctx context.Context, req AIQueryRequest, emit func(AIStreamEvent) error) error {
	if emit == nil {
		return errors.New("stream emitter is nil")
	}
	if s.support == nil || !s.support.Enabled() {
		return ErrServiceDisabled
	}
	if s.pi == nil {
		return runtime.ErrRuntimeUnavailable
	}
	query := strings.TrimSpace(req.Query)
	if query == "" {
		return ErrInvalidQuery
	}

	safeEmit := func(event AIStreamEvent) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		return emit(event)
	}
	if err := safeEmit(AIStreamEvent{Event: StreamEventStart, Query: query}); err != nil {
		return err
	}
	if err := safeEmit(AIStreamEvent{
		Event:   StreamEventStatus,
		Phase:   "thinking",
		Message: localizedStreamPhaseMessage(query, "thinking"),
	}); err != nil {
		return err
	}

	resp, err := s.pi.ExecuteStream(ctx, AIQueryRequest{Query: query, Stream: true}, emit)
	if err != nil {
		return err
	}
	meta := AIStreamMeta{
		ReasoningSummary: strings.TrimSpace(resp.ReasoningSummary),
		Mode:             strings.TrimSpace(resp.Mode),
		ToolCalls:        resp.ToolCalls,
		RelatedNodes:     resp.RelatedNodes,
		Warnings:         resp.Warnings,
		KnowledgeHits:    resp.KnowledgeHits,
		Retrieval:        resp.Retrieval,
	}
	if err := safeEmit(AIStreamEvent{Event: StreamEventMeta, Meta: &meta}); err != nil {
		return err
	}
	return safeEmit(AIStreamEvent{Event: StreamEventDone, Done: &resp})
}

// Capabilities reports the single supported AI mode and the Pi tool set.
func (s *PiQueryService) Capabilities(ctx context.Context) CapabilitiesResponse {
	capabilities := s.support.Capabilities(ctx)
	capabilities.DefaultMode = AIModeAgent
	capabilities.SupportedModes = []string{AIModeAgent}
	return capabilities
}

// Health reports shared AI configuration and the Pi Runtime readiness.
func (s *PiQueryService) Health(ctx context.Context) HealthResponse {
	if s.support == nil {
		ready := false
		return HealthResponse{
			Status:       "degraded",
			Mode:         AIModeAgent,
			Executor:     ExecutorModePi,
			RuntimeReady: &ready,
		}
	}
	health := s.support.Health(ctx)
	health.Mode = AIModeAgent
	health.Executor = ExecutorModePi
	healthCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	ready := s.pi != nil && s.pi.Health(healthCtx) == nil
	health.RuntimeReady = &ready
	if !ready && health.Status != "disabled" {
		health.AgentReady = false
		health.Status = "degraded"
	}
	return health
}

// RetrievalStats delegates to the shared support service.
func (s *PiQueryService) RetrievalStats(ctx context.Context) RetrievalStats {
	if s.support == nil {
		return RetrievalStats{}
	}
	return s.support.RetrievalStats(ctx)
}

// ReloadKnowledge delegates to the shared support service.
func (s *PiQueryService) ReloadKnowledge(ctx context.Context) (KnowledgeReloadResult, error) {
	if s.support == nil {
		return KnowledgeReloadResult{}, ErrKnowledgeReloadUnavailable
	}
	return s.support.ReloadKnowledge(ctx)
}
