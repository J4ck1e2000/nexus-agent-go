package gateway

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

// SetAIChatStore 注入 AI 会话存储依赖；未注入时会话历史 API 返回 503。
func (h *Handler) SetAIChatStore(store *AIChatStore) {
	if h == nil {
		return
	}
	h.aiChatStore = store
}

// registerConversationRoutes 注册 AI 会话历史 API（登录用户操作自己的会话）。
func (h *Handler) registerConversationRoutes(authorized *gin.RouterGroup) {
	if authorized == nil {
		return
	}
	authorized.GET("/ai/conversations", h.listAIConversations)
	authorized.POST("/ai/conversations", h.postAIConversation)
	authorized.GET("/ai/conversations/:id/messages", h.getAIConversationMessages)
	authorized.DELETE("/ai/conversations/:id", h.deleteAIConversation)
}

// listAIConversations 返回当前用户的会话列表（最近活跃在前）。
func (h *Handler) listAIConversations(c *gin.Context) {
	store, user, ok := h.aiConversationRequest(c)
	if !ok {
		return
	}

	limit := parsePositiveIntQuery(c, "limit", aiConversationListDefaultLimit)
	offset, _ := strconv.Atoi(c.Query("offset"))
	conversations, err := store.ListConversations(user.ID, limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"conversations": conversations})
}

// postAIConversation 创建一个空会话，返回会话概要。
func (h *Handler) postAIConversation(c *gin.Context) {
	store, user, ok := h.aiConversationRequest(c)
	if !ok {
		return
	}

	conversation, err := store.CreateConversation(user.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
		return
	}
	c.JSON(http.StatusCreated, conversation)
}

// getAIConversationMessages 返回会话内全部消息（时间正序）。
func (h *Handler) getAIConversationMessages(c *gin.Context) {
	store, user, ok := h.aiConversationRequest(c)
	if !ok {
		return
	}

	conversationID, err := parseUintIDParam(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_payload"})
		return
	}
	messages, err := store.ConversationMessages(user.ID, conversationID)
	if err != nil {
		writeAIConversationError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"messages": messages})
}

// deleteAIConversation 删除会话及其全部消息。
func (h *Handler) deleteAIConversation(c *gin.Context) {
	store, user, ok := h.aiConversationRequest(c)
	if !ok {
		return
	}

	conversationID, err := parseUintIDParam(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_payload"})
		return
	}
	if err := store.DeleteConversation(user.ID, conversationID); err != nil {
		writeAIConversationError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success"})
}

// aiConversationRequest 校验会话 API 的公共前置：存储可用且用户已认证。
func (h *Handler) aiConversationRequest(c *gin.Context) (*AIChatStore, *AuthUser, bool) {
	if h == nil || h.aiChatStore == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "ai_unavailable"})
		return nil, nil, false
	}
	user, ok := currentAuthUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return nil, nil, false
	}
	return h.aiChatStore, user, true
}

// writeAIConversationError 把存储层错误映射为稳定的错误码；
// 越权访问统一返回 404，不泄露会话存在性。
func writeAIConversationError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrConversationNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "conversation_not_found"})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
	}
}

// parsePositiveIntQuery 解析非负整型查询参数，非法或缺失时用默认值。
func parsePositiveIntQuery(c *gin.Context, key string, fallback int) int {
	raw := strings.TrimSpace(c.Query(key))
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 0 {
		return fallback
	}
	return value
}
