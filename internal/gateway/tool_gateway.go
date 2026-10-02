package gateway

import (
	"crypto/subtle"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"nexus-agent-go/internal/ai"
	"nexus-agent-go/internal/runtime"
)

const (
	runCredentialContextKey = "run_credential"
	toolLogResultPreviewLen = 120
)

// ToolGatewayDeps wires the internal tool execution surface.
type ToolGatewayDeps struct {
	Dispatcher     *ai.ToolDispatcher
	Manager        *runtime.Manager
	InternalToken  string
	RunTokenSecret string
}

// SetToolGateway injects tool gateway dependencies; disabled when Manager is nil.
func (h *Handler) SetToolGateway(deps ToolGatewayDeps) {
	if h == nil {
		return
	}
	h.toolDispatcher = deps.Dispatcher
	h.runManager = deps.Manager
	h.toolGatewayToken = deps.InternalToken
	h.runTokenSecret = deps.RunTokenSecret
}

// RegisterInternalToolRoutes exposes POST /internal/api/tools/:name.
// The surface only exists when a run manager was injected.
func (h *Handler) RegisterInternalToolRoutes(r *gin.Engine) {
	if h == nil || h.runManager == nil {
		return
	}
	group := r.Group("/internal/api/tools")
	group.Use(h.internalToolAuth())
	group.POST("/:name", h.postInternalTool)
}

func (h *Handler) internalToolAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		token := strings.TrimSpace(c.GetHeader("X-Nexus-Internal-Token"))
		expected := strings.TrimSpace(h.toolGatewayToken)
		if expected == "" || token == "" ||
			subtle.ConstantTimeCompare([]byte(token), []byte(expected)) != 1 {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}

		authHeader := strings.TrimSpace(c.GetHeader("Authorization"))
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || strings.TrimSpace(parts[1]) == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		credential, err := runtime.VerifyRunCredential(strings.TrimSpace(parts[1]), h.runTokenSecret)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		c.Set(runCredentialContextKey, credential)
		c.Next()
	}
}

func (h *Handler) postInternalTool(c *gin.Context) {
	toolName := strings.TrimSpace(c.Param("name"))
	if h.toolDispatcher == nil || !h.toolDispatcher.HasTool(toolName) {
		c.JSON(http.StatusNotFound, gin.H{"error": "unknown_tool"})
		return
	}

	credential, ok := currentRunCredential(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var req ai.ToolCallRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_payload"})
		return
	}
	req.RunID = strings.TrimSpace(req.RunID)
	if req.RunID == "" || req.RunID != credential.RunID {
		c.JSON(http.StatusForbidden, gin.H{"error": "run_mismatch"})
		return
	}

	// The run must still be live, the tool allow-listed, and budget available.
	if err := h.runManager.AuthorizeTool(req.RunID, toolName); err != nil {
		switch {
		case errors.Is(err, runtime.ErrRunNotFound):
			c.JSON(http.StatusConflict, gin.H{"error": "run_not_found"})
		case errors.Is(err, runtime.ErrToolNotAllowed):
			c.JSON(http.StatusForbidden, gin.H{"error": "tool_not_allowed"})
		case errors.Is(err, runtime.ErrRunAlreadyFinished):
			c.JSON(http.StatusConflict, gin.H{"error": "run_finished"})
		default:
			c.JSON(http.StatusConflict, gin.H{"error": "run_not_active"})
		}
		return
	}

	started := time.Now()
	// Business failures return 200 with ok:false so the runtime can feed a
	// structured Observation back to the model (design §8.3).
	response := h.toolDispatcher.Dispatch(c.Request.Context(), toolName, req.Arguments)
	logInternalToolCall(toolName, credential.Username, started, response)

	c.JSON(http.StatusOK, response)
}

func currentRunCredential(c *gin.Context) (runtime.RunCredential, bool) {
	value, ok := c.Get(runCredentialContextKey)
	if !ok {
		return runtime.RunCredential{}, false
	}
	credential, ok := value.(runtime.RunCredential)
	return credential, ok && credential.RunID != ""
}

// logInternalToolCall audits one tool invocation without dumping raw arguments.
func logInternalToolCall(toolName, username string, started time.Time, response ai.ToolCallResponse) {
	preview := ""
	switch {
	case response.Error != nil:
		preview = response.Error.Code + ": " + response.Error.Message
	case response.OK:
		preview = "ok"
	}
	if len(preview) > toolLogResultPreviewLen {
		preview = preview[:toolLogResultPreviewLen]
	}
	log.Printf("[tool-gateway] user=%s tool=%s duration=%s ok=%t %s",
		username, toolName, time.Since(started).Round(time.Millisecond), response.OK, preview)
}
