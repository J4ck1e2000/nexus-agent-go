package ai

import (
	"context"
	"errors"
	"strings"

	"nexus-agent-go/internal/runtime"
)

// ExecutorModePi marks health/capability payloads served through the Pi runtime.
const ExecutorModePi = "pi"

// PiQueryService implements the gateway AI contract on top of the external Pi
// runtime while delegating non-query concerns (capabilities, health, knowledge
// reload) and pre-start fallback to the legacy service.
type PiQueryService struct {
	legacy *Service
	pi     *PiRuntimeExecutor
}

// NewPiQueryService wires the Pi-backed AI service.
func NewPiQueryService(legacy *Service, pi *PiRuntimeExecutor) *PiQueryService {
	return &PiQueryService{legacy: legacy, pi: pi}
}

// Query executes one request on the Pi runtime, falling back to the legacy
// service only when the runtime was unreachable before the run started.
func (s *PiQueryService) Query(ctx context.Context, req AIQueryRequest) (AIQueryResponse, error) {
	if !s.legacy.Enabled() {
		return AIQueryResponse{}, ErrServiceDisabled
	}
	resp, err := s.pi.Execute(ctx, req)
	if errors.Is(err, runtime.ErrRuntimeUnavailable) {
		return s.legacy.Query(ctx, req)
	}
	return resp, err
}

// QueryStream executes one streaming request on the Pi runtime.
// Event contract for the frontend is identical to the legacy service:
// start -> status -> (status/delta)* -> meta -> done.
func (s *PiQueryService) QueryStream(ctx context.Context, req AIQueryRequest, emit func(AIStreamEvent) error) error {
	if emit == nil {
		return errors.New("stream emitter is nil")
	}
	if !s.legacy.Enabled() {
		return ErrServiceDisabled
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
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return err
		}
		if errors.Is(err, runtime.ErrRuntimeUnavailable) {
			// Pre-start failure only: legacy fallback is allowed (design §6.3).
			if emitErr := safeEmit(AIStreamEvent{
				Event:   StreamEventStatus,
				Phase:   "fallback",
				Message: localizedStreamPhaseMessage(query, "fallback"),
			}); emitErr != nil {
				return emitErr
			}
			return s.legacyFallbackStream(ctx, query, safeEmit)
		}
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
	finalResp := resp
	return safeEmit(AIStreamEvent{Event: StreamEventDone, Done: &finalResp})
}

// legacyFallbackStream reproduces the legacy stream tail (deltas, meta, done)
// on top of the legacy non-stream query, without re-emitting the start event.
func (s *PiQueryService) legacyFallbackStream(ctx context.Context, query string, emit func(AIStreamEvent) error) error {
	resp, err := s.legacy.Query(ctx, AIQueryRequest{Query: query, Stream: false})
	if err != nil {
		return err
	}
	if err := emit(AIStreamEvent{
		Event:   StreamEventStatus,
		Phase:   "generating",
		Message: localizedStreamPhaseMessage(query, "generating"),
	}); err != nil {
		return err
	}
	for _, chunk := range chunkAnswerForStream(resp.Answer, defaultStreamChunkRuneSize) {
		if err := emit(AIStreamEvent{Event: StreamEventDelta, Text: chunk}); err != nil {
			return err
		}
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
	if err := emit(AIStreamEvent{Event: StreamEventMeta, Meta: &meta}); err != nil {
		return err
	}
	return emit(AIStreamEvent{Event: StreamEventDone, Done: &resp})
}

// Capabilities delegates to the legacy service and marks the Pi executor.
func (s *PiQueryService) Capabilities(ctx context.Context) CapabilitiesResponse {
	capabilities := s.legacy.Capabilities(ctx)
	capabilities.DefaultMode = ExecutorModePi
	return capabilities
}

// Health delegates to the legacy service and marks the Pi executor.
func (s *PiQueryService) Health(ctx context.Context) HealthResponse {
	health := s.legacy.Health(ctx)
	health.Executor = ExecutorModePi
	return health
}

// RetrievalStats delegates to the legacy service.
func (s *PiQueryService) RetrievalStats(ctx context.Context) RetrievalStats {
	return s.legacy.RetrievalStats(ctx)
}

// ReloadKnowledge delegates to the legacy service.
func (s *PiQueryService) ReloadKnowledge(ctx context.Context) (KnowledgeReloadResult, error) {
	return s.legacy.ReloadKnowledge(ctx)
}
