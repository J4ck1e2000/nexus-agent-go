package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"nexus-agent-go/internal/runtime"
)

// AIChatStore 负责 AI 会话与消息的数据库读写，并为 ai 包提供
// ConversationStore 的持久化实现。
type AIChatStore struct {
	db *gorm.DB
}

var (
	// ErrConversationNotFound 表示会话不存在或不属于当前用户。
	ErrConversationNotFound = errors.New("ai conversation not found")
	// ErrInvalidAIMessageRole 表示消息角色不合法。
	ErrInvalidAIMessageRole = errors.New("invalid ai message role")
)

const (
	// aiConversationTitleMaxLen 限制自动会话标题长度（rune 数）。
	aiConversationTitleMaxLen = 48
	// aiConversationListDefaultLimit 会话列表默认页大小。
	aiConversationListDefaultLimit = 50
	// aiConversationListMaxLimit 会话列表单页上限。
	aiConversationListMaxLimit = 200
)

// NewAIChatStore 创建 AI 会话存储实例。
func NewAIChatStore(db *gorm.DB) *AIChatStore {
	return &AIChatStore{db: db}
}

// CreateConversation 为用户创建空会话。
func (s *AIChatStore) CreateConversation(userID uint) (*AIConversation, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("ai chat store db is nil")
	}
	conv := AIConversation{UserID: userID}
	if err := s.db.Create(&conv).Error; err != nil {
		return nil, fmt.Errorf("create ai conversation failed: %w", err)
	}
	return &conv, nil
}

// ListConversations 返回用户的会话列表（最近活跃在前）。
func (s *AIChatStore) ListConversations(userID uint, limit, offset int) ([]AIConversation, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("ai chat store db is nil")
	}
	if limit <= 0 {
		limit = aiConversationListDefaultLimit
	}
	if limit > aiConversationListMaxLimit {
		limit = aiConversationListMaxLimit
	}
	if offset < 0 {
		offset = 0
	}
	var rows []AIConversation
	if err := s.db.Where("user_id = ?", userID).
		Order("updated_at DESC").
		Limit(limit).Offset(offset).
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list ai conversations failed: %w", err)
	}
	return rows, nil
}

// ConversationMessages 返回会话内全部消息（按时间正序），meta 已解码。
// 会话不存在或不属于该用户时返回 ErrConversationNotFound。
func (s *AIChatStore) ConversationMessages(userID uint, conversationID uint) ([]AIMessageView, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("ai chat store db is nil")
	}
	var rows []AIMessage
	if err := s.scopedConversationQuery(userID, conversationID).
		Order("id ASC").
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list ai messages failed: %w", err)
	}
	messages := make([]AIMessageView, 0, len(rows))
	for _, row := range rows {
		messages = append(messages, AIMessageView{
			ID:        row.ID,
			Role:      row.Role,
			Content:   row.Content,
			Meta:      decodeAIMessageMeta(row.MetaJSON),
			CreatedAt: row.CreatedAt,
		})
	}
	return messages, nil
}

// DeleteConversation 删除会话及其全部消息；会话不存在或越权时返回
// ErrConversationNotFound。
func (s *AIChatStore) DeleteConversation(userID uint, conversationID uint) error {
	if s == nil || s.db == nil {
		return errors.New("ai chat store db is nil")
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		result := tx.Where("id = ? AND user_id = ?", conversationID, userID).Delete(&AIConversation{})
		if result.Error != nil {
			return fmt.Errorf("delete ai conversation failed: %w", result.Error)
		}
		if result.RowsAffected == 0 {
			return ErrConversationNotFound
		}
		if err := tx.Where("conversation_id = ?", conversationID).Delete(&AIMessage{}).Error; err != nil {
			return fmt.Errorf("delete ai messages failed: %w", err)
		}
		return nil
	})
}

// ConversationOwnedBy 报告会话是否存在且属于该用户（实现 ai.ConversationStore）。
func (s *AIChatStore) ConversationOwnedBy(_ context.Context, userID int64, conversationID uint) (bool, error) {
	if s == nil || s.db == nil {
		return false, errors.New("ai chat store db is nil")
	}
	var count int64
	if err := s.db.Model(&AIConversation{}).
		Where("id = ? AND user_id = ?", conversationID, userID).
		Count(&count).Error; err != nil {
		return false, fmt.Errorf("check ai conversation owner failed: %w", err)
	}
	return count > 0, nil
}

// RecentMessages 返回会话最近 limit 条消息（按时间正序），已按用户过滤
// （实现 ai.ConversationStore）。
func (s *AIChatStore) RecentMessages(_ context.Context, userID int64, conversationID uint, limit int) ([]runtime.RunHistoryEntry, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("ai chat store db is nil")
	}
	if limit <= 0 {
		return nil, nil
	}
	// 先取最近 limit 条（id 倒序），再反转为正序。
	var rows []AIMessage
	if err := s.scopedConversationQuery(uint(userID), conversationID).
		Order("id DESC").
		Limit(limit).
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("load recent ai messages failed: %w", err)
	}
	entries := make([]runtime.RunHistoryEntry, 0, len(rows))
	for i := len(rows) - 1; i >= 0; i-- {
		entries = append(entries, runtime.RunHistoryEntry{
			Role:    rows[i].Role,
			Content: rows[i].Content,
		})
	}
	return entries, nil
}

// AppendMessage 落库一条消息并刷新会话活跃时间；首条提问同时补全空标题
// （实现 ai.ConversationStore）。
func (s *AIChatStore) AppendMessage(_ context.Context, userID int64, conversationID uint, role, content, metaJSON string) error {
	if s == nil || s.db == nil {
		return errors.New("ai chat store db is nil")
	}
	role = strings.TrimSpace(role)
	if role != AIMessageRoleUser && role != AIMessageRoleAssistant {
		return fmt.Errorf("%w: %s", ErrInvalidAIMessageRole, role)
	}
	message := AIMessage{
		ConversationID: conversationID,
		Role:           role,
		Content:        content,
	}
	if meta := strings.TrimSpace(metaJSON); meta != "" {
		message.MetaJSON = &meta
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		// 事务内再次校验归属，避免越权写入。
		var count int64
		if err := tx.Model(&AIConversation{}).
			Where("id = ? AND user_id = ?", conversationID, userID).
			Count(&count).Error; err != nil {
			return fmt.Errorf("check ai conversation owner failed: %w", err)
		}
		if count == 0 {
			return ErrConversationNotFound
		}
		if err := tx.Create(&message).Error; err != nil {
			return fmt.Errorf("create ai message failed: %w", err)
		}
		updates := map[string]any{"updated_at": time.Now()}
		if role == AIMessageRoleUser {
			if title := deriveConversationTitle(content); title != "" {
				updates["title"] = gorm.Expr("CASE WHEN title = '' THEN ? ELSE title END", title)
			}
		}
		if err := tx.Model(&AIConversation{}).
			Where("id = ?", conversationID).
			Updates(updates).Error; err != nil {
			return fmt.Errorf("touch ai conversation failed: %w", err)
		}
		return nil
	})
}

// scopedConversationQuery 返回带用户归属过滤的消息查询，保证越权读返回空集。
func (s *AIChatStore) scopedConversationQuery(userID uint, conversationID uint) *gorm.DB {
	return s.db.Where(
		"conversation_id IN (?)",
		s.db.Model(&AIConversation{}).
			Select("id").
			Where("id = ? AND user_id = ?", conversationID, userID),
	)
}

// deriveConversationTitle 从首条提问派生单行标题。
func deriveConversationTitle(content string) string {
	normalized := strings.Join(strings.Fields(content), " ")
	runes := []rune(normalized)
	if len(runes) == 0 {
		return ""
	}
	if len(runes) > aiConversationTitleMaxLen {
		runes = append(runes[:aiConversationTitleMaxLen-1], '…')
	}
	return string(runes)
}

// AIMessageView 是 AI 消息的 API 视图，Meta 为解码后的 meta 载荷。
type AIMessageView struct {
	ID        uint           `json:"id"`
	Role      string         `json:"role"`
	Content   string         `json:"content"`
	Meta      map[string]any `json:"meta"`
	CreatedAt time.Time      `json:"created_at"`
}

// decodeAIMessageMeta 把存储的 meta JSON 解码为对象；空值或非法 JSON 返回 nil。
func decodeAIMessageMeta(raw *string) map[string]any {
	if raw == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*raw)
	if trimmed == "" {
		return nil
	}
	var meta map[string]any
	if err := json.Unmarshal([]byte(trimmed), &meta); err != nil {
		return nil
	}
	return meta
}
