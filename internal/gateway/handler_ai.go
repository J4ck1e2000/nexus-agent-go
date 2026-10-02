package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"nexus-agent-go/internal/ai"
	"nexus-agent-go/internal/runtime"
)

// AIQueryService defines the gateway-facing AI query service contract.
type AIQueryService interface {
	Query(ctx context.Context, req ai.AIQueryRequest) (ai.AIQueryResponse, error)
	QueryStream(ctx context.Context, req ai.AIQueryRequest, emit func(ai.AIStreamEvent) error) error
	Capabilities(ctx context.Context) ai.CapabilitiesResponse
	Health(ctx context.Context) ai.HealthResponse
	RetrievalStats(ctx context.Context) ai.RetrievalStats
	ReloadKnowledge(ctx context.Context) (ai.KnowledgeReloadResult, error)
}

// SetAIQueryService injects AI service dependency into gateway handler.
func (h *Handler) SetAIQueryService(service AIQueryService) {
	if h == nil {
		return
	}
	h.aiQueryService = service
}

func (h *Handler) registerAIRoutes(authorized *gin.RouterGroup) {
	if authorized == nil {
		return
	}
	authorized.POST("/ai/query", h.postAIQuery)
	authorized.POST("/ai/knowledge/reload", h.postAIKnowledgeReload)
	authorized.GET("/ai/capabilities", h.getAICapabilities)
	authorized.GET("/ai/health", h.getAIHealth)
	authorized.GET("/ai/retrieval/stats", h.getAIRetrievalStats)
}

func (h *Handler) postAIQuery(c *gin.Context) {
	if h.aiQueryService == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "ai_unavailable"})
		return
	}

	var req ai.AIQueryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_payload"})
		return
	}
	if strings.TrimSpace(req.Query) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_payload"})
		return
	}
	if req.Stream {
		h.postAIQueryStream(c, req)
		return
	}

	resp, err := h.aiQueryService.Query(aiRequestContext(c), req)
	if err != nil {
		status, code := mapAIServiceError(err)
		c.JSON(status, gin.H{"error": code})
		return
	}

	c.JSON(http.StatusOK, resp)
}

func (h *Handler) postAIQueryStream(c *gin.Context, req ai.AIQueryRequest) {
	c.Header("Content-Type", "text/event-stream; charset=utf-8")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)

	writeEvent := func(event ai.AIStreamEvent) error {
		payload, err := marshalAIStreamPayload(event)
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintf(c.Writer, "event: %s\ndata: %s\n\n", event.Event, payload); err != nil {
			return err
		}
		c.Writer.Flush()
		return nil
	}

	if err := h.aiQueryService.QueryStream(aiRequestContext(c), req, writeEvent); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return
		}

		_, code := mapAIServiceError(err)
		_ = writeEvent(ai.AIStreamEvent{
			Event: ai.StreamEventError,
			Error: code,
		})
	}
}

// aiRequestContext attaches the authenticated principal to the request
// context so executors can sign run credentials without gin dependencies.
func aiRequestContext(c *gin.Context) context.Context {
	ctx := c.Request.Context()
	if user, ok := currentAuthUser(c); ok {
		ctx = runtime.WithPrincipal(ctx, runtime.RunPrincipal{
			UserID:   int64(user.ID),
			Username: user.Username,
			Role:     string(user.Role),
		})
	}
	return ctx
}

func marshalAIStreamPayload(event ai.AIStreamEvent) (string, error) {
	var payload any
	switch strings.TrimSpace(event.Event) {
	case ai.StreamEventStart:
		payload = map[string]any{
			"query": strings.TrimSpace(event.Query),
		}
	case ai.StreamEventStatus:
		payload = map[string]any{
			"phase":   strings.TrimSpace(event.Phase),
			"message": strings.TrimSpace(event.Message),
		}
	case ai.StreamEventDelta:
		payload = map[string]any{
			"text": event.Text,
		}
	case ai.StreamEventMeta:
		if event.Meta != nil {
			payload = event.Meta
		} else {
			payload = map[string]any{}
		}
	case ai.StreamEventDone:
		if event.Done != nil {
			payload = event.Done
		} else {
			payload = ai.AIQueryResponse{}
		}
	case ai.StreamEventError:
		payload = map[string]any{
			"error": strings.TrimSpace(event.Error),
		}
	default:
		payload = map[string]any{
			"message": strings.TrimSpace(event.Message),
		}
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func mapAIServiceError(err error) (status int, code string) {
	switch {
	case errors.Is(err, ai.ErrServiceDisabled):
		return http.StatusServiceUnavailable, "ai_disabled"
	case errors.Is(err, ai.ErrInvalidQuery):
		return http.StatusBadRequest, "invalid_payload"
	case errors.Is(err, ai.ErrNodeNotFound), errors.Is(err, ai.ErrNodeNameRequired):
		return http.StatusBadRequest, "invalid_query"
	default:
		return http.StatusInternalServerError, "server_error"
	}
}

func (h *Handler) getAICapabilities(c *gin.Context) {
	if h.aiQueryService == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "ai_unavailable"})
		return
	}
	c.JSON(http.StatusOK, h.aiQueryService.Capabilities(c.Request.Context()))
}

func (h *Handler) getAIHealth(c *gin.Context) {
	if h.aiQueryService == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "ai_unavailable"})
		return
	}
	c.JSON(http.StatusOK, h.aiQueryService.Health(c.Request.Context()))
}

func (h *Handler) getAIRetrievalStats(c *gin.Context) {
	if h.aiQueryService == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "ai_unavailable"})
		return
	}
	c.JSON(http.StatusOK, h.aiQueryService.RetrievalStats(c.Request.Context()))
}

func (h *Handler) postAIKnowledgeReload(c *gin.Context) {
	if h.aiQueryService == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "ai_unavailable"})
		return
	}

	user, ok := currentAuthUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	if user.Role != RoleAdmin {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}

	result, err := h.aiQueryService.ReloadKnowledge(c.Request.Context())
	if err != nil {
		switch {
		case errors.Is(err, ai.ErrKnowledgeReloadDisabled):
			c.JSON(http.StatusBadRequest, gin.H{"error": "knowledge_reload_disabled"})
		case errors.Is(err, ai.ErrKnowledgeReloadUnavailable):
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "knowledge_reload_unavailable"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "knowledge_reload_failed"})
		}
		return
	}
	c.JSON(http.StatusOK, result)
}
