package ai

import (
	"context"

	"nexus-agent-go/internal/runtime"
)

// ConversationStore persists AI chat conversations per user. The gateway
// package provides the GORM-backed implementation; a nil store keeps the
// legacy stateless behavior (no conversation logic, nothing persisted).
type ConversationStore interface {
	// ConversationOwnedBy reports whether the conversation exists and belongs
	// to the user; callers must check ownership before any read or write.
	ConversationOwnedBy(ctx context.Context, userID int64, conversationID uint) (bool, error)
	// RecentMessages returns the last `limit` messages of the conversation in
	// chronological order, already scoped to conversations owned by the user.
	RecentMessages(ctx context.Context, userID int64, conversationID uint, limit int) ([]runtime.RunHistoryEntry, error)
	// AppendMessage persists one message of the conversation.
	AppendMessage(ctx context.Context, userID int64, conversationID uint, role, content, metaJSON string) error
}
