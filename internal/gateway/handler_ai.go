package gateway

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"nexus-agent-go/internal/ai"
)

// AIQueryService defines the gateway-facing AI query service contract.
type AIQueryService interface {
	Query(ctx context.Context, req ai.AIQueryRequest) (ai.AIQueryResponse, error)
	Capabilities(ctx context.Context) ai.CapabilitiesResponse
	Health(ctx context.Context) ai.HealthResponse
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
	authorized.GET("/ai/capabilities", h.getAICapabilities)
	authorized.GET("/ai/health", h.getAIHealth)
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

	resp, err := h.aiQueryService.Query(c.Request.Context(), req)
	if err != nil {
		switch {
		case errors.Is(err, ai.ErrServiceDisabled):
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "ai_disabled"})
		case errors.Is(err, ai.ErrInvalidQuery):
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_payload"})
		case errors.Is(err, ai.ErrNodeNotFound), errors.Is(err, ai.ErrNodeNameRequired):
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_query"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
		}
		return
	}

	c.JSON(http.StatusOK, resp)
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
