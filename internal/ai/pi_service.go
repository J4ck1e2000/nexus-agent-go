package ai

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"strings"
	"time"

	"nexus-agent-go/internal/runtime"
)

// ExecutorModePi identifies the Pi Runtime executor in health responses.
const ExecutorModePi = "pi"

// conversationContextEntries 多轮上下文携带的最大历史条数，与 pi-runtime
// runner.ts 的 maxEntries 保持一致。
const conversationContextEntries = 6

// piRuntimeExecutor 是 PiQueryService 依赖的执行器表面，便于测试替换。
type piRuntimeExecutor interface {
	Execute(ctx context.Context, req AIQueryRequest) (AIQueryResponse, error)
	ExecuteStream(ctx context.Context, req AIQueryRequest, emit func(AIStreamEvent) error) (AIQueryResponse, error)
	Health(ctx context.Context) error
}

// PiQueryService implements the gateway AI contract using only the Pi Runtime.
type PiQueryService struct {
	support *Service
	pi      piRuntimeExecutor
	// store 为nil时退化为无会话模式：不校验、不落库、不注入历史。
	store ConversationStore
}

// NewPiQueryService wires the Pi Runtime and shared AI support services.
func NewPiQueryService(support *Service, pi piRuntimeExecutor, store ConversationStore) *PiQueryService {
	return &PiQueryService{support: support, pi: pi, store: store}
}

// Query executes one request on the Pi Runtime. Runtime failures are returned to the caller.
func (s *PiQueryService) Query(ctx context.Context, req AIQueryRequest) (AIQueryResponse, error) {
	if s.support == nil || !s.support.Enabled() {
		return AIQueryResponse{}, ErrServiceDisabled
	}
	if s.pi == nil {
		return AIQueryResponse{}, runtime.ErrRuntimeUnavailable
	}

	conversationActive := s.store != nil && req.ConversationID != 0
	runCtx, err := s.prepareConversation(ctx, &req)
	if err != nil {
		return AIQueryResponse{}, err
	}
	s.persistUserMessage(ctx, req.ConversationID, strings.TrimSpace(req.Query))

	resp, err := s.pi.Execute(runCtx, req)
	if err != nil {
		return AIQueryResponse{}, err
	}
	if conversationActive {
		resp.ConversationID = req.ConversationID
	}
	s.persistAssistantResponse(ctx, req.ConversationID, resp)
	return resp, nil
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

	conversationActive := s.store != nil && req.ConversationID != 0
	runCtx, err := s.prepareConversation(ctx, &req)
	if err != nil {
		return err
	}
	s.persistUserMessage(ctx, req.ConversationID, query)

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

	resp, err := s.pi.ExecuteStream(runCtx, AIQueryRequest{Query: query, Stream: true, ConversationID: req.ConversationID}, emit)
	if err != nil {
		return err
	}
	if conversationActive {
		resp.ConversationID = req.ConversationID
	}
	s.persistAssistantResponse(ctx, req.ConversationID, resp)
	meta := AIStreamMeta{
		ReasoningSummary: strings.TrimSpace(resp.ReasoningSummary),
		Mode:             strings.TrimSpace(resp.Mode),
		ConversationID:   resp.ConversationID,
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

// prepareConversation 校验会话归属并把最近历史注入运行上下文。
// 请求未携带会话或未配置存储时原样返回（无会话模式）。
func (s *PiQueryService) prepareConversation(ctx context.Context, req *AIQueryRequest) (context.Context, error) {
	if s.store == nil || req.ConversationID == 0 {
		return ctx, nil
	}
	userID := conversationUserID(ctx)
	owned, err := s.store.ConversationOwnedBy(ctx, userID, req.ConversationID)
	if err != nil {
		return ctx, err
	}
	if !owned {
		return ctx, ErrInvalidConversation
	}
	entries, err := s.store.RecentMessages(ctx, userID, req.ConversationID, conversationContextEntries)
	if err != nil {
		// 历史只是增强：读取失败时降级为无上下文继续回答。
		log.Printf("load conversation %d recent messages failed: %v", req.ConversationID, err)
		return ctx, nil
	}
	return runtime.WithRecentHistory(ctx, entries), nil
}

// persistUserMessage 在查询开始时落库提问；失败仅记录，不影响回答。
func (s *PiQueryService) persistUserMessage(ctx context.Context, conversationID uint, query string) {
	if s.store == nil || conversationID == 0 || strings.TrimSpace(query) == "" {
		return
	}
	if err := s.store.AppendMessage(ctx, conversationUserID(ctx), conversationID, "user", query, ""); err != nil {
		log.Printf("persist user message to conversation %d failed: %v", conversationID, err)
	}
}

// persistAssistantResponse 在回答完成后落库助手消息（含 meta JSON）。
func (s *PiQueryService) persistAssistantResponse(ctx context.Context, conversationID uint, resp AIQueryResponse) {
	if s.store == nil || conversationID == 0 {
		return
	}
	if strings.TrimSpace(resp.Answer) == "" {
		return
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
	metaJSON, err := json.Marshal(&meta)
	if err != nil {
		log.Printf("marshal assistant meta for conversation %d failed: %v", conversationID, err)
		return
	}
	if err := s.store.AppendMessage(ctx, conversationUserID(ctx), conversationID, "assistant", resp.Answer, string(metaJSON)); err != nil {
		log.Printf("persist assistant message to conversation %d failed: %v", conversationID, err)
	}
}

// conversationUserID 提取发起查询的用户 ID；未认证时为 0（无法命中任何会话）。
func conversationUserID(ctx context.Context) int64 {
	if principal, ok := runtime.PrincipalFromCtx(ctx); ok {
		return principal.UserID
	}
	return 0
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
