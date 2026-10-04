package gateway

import (
	"context"
	"errors"
	"strings"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupAIChatStore(t *testing.T) *AIChatStore {
	t.Helper()
	dsn := "file:" + strings.ReplaceAll(t.Name(), "/", "_") + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite failed: %v", err)
	}
	if err := AutoMigrate(db); err != nil {
		t.Fatalf("auto migrate failed: %v", err)
	}
	return NewAIChatStore(db)
}

func TestAIChatStore_CreateAndListConversations(t *testing.T) {
	store := setupAIChatStore(t)

	first, err := store.CreateConversation(1)
	if err != nil {
		t.Fatalf("create conversation failed: %v", err)
	}
	second, err := store.CreateConversation(1)
	if err != nil {
		t.Fatalf("create conversation failed: %v", err)
	}
	if _, err := store.CreateConversation(2); err != nil {
		t.Fatalf("create conversation failed: %v", err)
	}

	conversations, err := store.ListConversations(1, 0, 0)
	if err != nil {
		t.Fatalf("list conversations failed: %v", err)
	}
	if len(conversations) != 2 {
		t.Fatalf("user1 conversations = %d, want 2", len(conversations))
	}
	if conversations[1].ID != first.ID || conversations[0].ID != second.ID {
		t.Fatalf("list should put most recent conversation first, got %+v", conversations)
	}
	for _, conv := range conversations {
		if conv.UserID != 1 {
			t.Fatalf("list leaked conversation of user %d", conv.UserID)
		}
	}
}

func TestAIChatStore_AppendMessageDerivesTitleAndTouchesConversation(t *testing.T) {
	store := setupAIChatStore(t)
	conv, err := store.CreateConversation(1)
	if err != nil {
		t.Fatalf("create conversation failed: %v", err)
	}

	longQuery := "这是一条用来生成标题的超长提问" + strings.Repeat("长", 60)
	if err := store.AppendMessage(context.Background(), 1, conv.ID, AIMessageRoleUser, longQuery, ""); err != nil {
		t.Fatalf("append user message failed: %v", err)
	}
	if err := store.AppendMessage(context.Background(), 1, conv.ID, AIMessageRoleAssistant, "回答", `{"reasoning_summary":"推理"}`); err != nil {
		t.Fatalf("append assistant message failed: %v", err)
	}

	conversations, err := store.ListConversations(1, 0, 0)
	if err != nil {
		t.Fatalf("list conversations failed: %v", err)
	}
	if conversations[0].ID != conv.ID {
		t.Fatalf("conversation should move to front after activity")
	}
	title := conversations[0].Title
	if runeCount := len([]rune(title)); runeCount != 48 {
		t.Fatalf("title rune count = %d, want 48", runeCount)
	}
	if !strings.HasSuffix(title, "…") {
		t.Fatalf("title should end with ellipsis, got %q", title)
	}

	messages, err := store.ConversationMessages(1, conv.ID)
	if err != nil {
		t.Fatalf("conversation messages failed: %v", err)
	}
	if len(messages) != 2 {
		t.Fatalf("messages = %d, want 2", len(messages))
	}
	if messages[0].Role != AIMessageRoleUser || messages[1].Role != AIMessageRoleAssistant {
		t.Fatalf("messages should keep chronological order: %+v", messages)
	}
	if messages[1].Meta == nil || messages[1].Meta["reasoning_summary"] != "推理" {
		t.Fatalf("assistant meta should be decoded, got %+v", messages[1].Meta)
	}
	if messages[0].Meta != nil {
		t.Fatalf("user message meta should be nil, got %+v", messages[0].Meta)
	}
}

func TestAIChatStore_AppendMessageKeepsExistingTitle(t *testing.T) {
	store := setupAIChatStore(t)
	conv, _ := store.CreateConversation(1)
	if err := store.AppendMessage(context.Background(), 1, conv.ID, AIMessageRoleUser, "第一问", ""); err != nil {
		t.Fatalf("append message failed: %v", err)
	}
	if err := store.AppendMessage(context.Background(), 1, conv.ID, AIMessageRoleUser, "第二问标题不应覆盖", ""); err != nil {
		t.Fatalf("append message failed: %v", err)
	}

	conversations, err := store.ListConversations(1, 0, 0)
	if err != nil {
		t.Fatalf("list conversations failed: %v", err)
	}
	if !strings.Contains(conversations[0].Title, "第一问") {
		t.Fatalf("title should stay from first question, got %q", conversations[0].Title)
	}
}

func TestAIChatStore_ConversationIsolationAndErrors(t *testing.T) {
	store := setupAIChatStore(t)
	conv, _ := store.CreateConversation(1)
	if err := store.AppendMessage(context.Background(), 1, conv.ID, AIMessageRoleUser, "私有提问", ""); err != nil {
		t.Fatalf("append message failed: %v", err)
	}

	// 用户 2 读不到用户 1 的消息。
	messages, err := store.ConversationMessages(2, conv.ID)
	if err != nil {
		t.Fatalf("conversation messages failed: %v", err)
	}
	if len(messages) != 0 {
		t.Fatalf("foreign user should see no messages, got %d", len(messages))
	}

	// 用户 2 不能写入用户 1 的会话。
	if err := store.AppendMessage(context.Background(), 2, conv.ID, AIMessageRoleUser, "越权", ""); !errors.Is(err, ErrConversationNotFound) {
		t.Fatalf("err = %v, want ErrConversationNotFound", err)
	}

	// 用户 2 不能删除用户 1 的会话，且原会话完好。
	if err := store.DeleteConversation(2, conv.ID); !errors.Is(err, ErrConversationNotFound) {
		t.Fatalf("err = %v, want ErrConversationNotFound", err)
	}
	// 用户 1 不能删除不存在的会话。
	if err := store.DeleteConversation(1, 999); !errors.Is(err, ErrConversationNotFound) {
		t.Fatalf("err = %v, want ErrConversationNotFound", err)
	}
	// 非法角色被拒绝。
	if err := store.AppendMessage(context.Background(), 1, conv.ID, "system", "x", ""); !errors.Is(err, ErrInvalidAIMessageRole) {
		t.Fatalf("err = %v, want ErrInvalidAIMessageRole", err)
	}

	messages, err = store.ConversationMessages(1, conv.ID)
	if err != nil || len(messages) != 1 {
		t.Fatalf("original conversation should stay intact: err=%v messages=%d", err, len(messages))
	}
}

func TestAIChatStore_DeleteConversationRemovesMessages(t *testing.T) {
	store := setupAIChatStore(t)
	conv, _ := store.CreateConversation(1)
	_ = store.AppendMessage(context.Background(), 1, conv.ID, AIMessageRoleUser, "问", "")
	_ = store.AppendMessage(context.Background(), 1, conv.ID, AIMessageRoleAssistant, "答", "")

	if err := store.DeleteConversation(1, conv.ID); err != nil {
		t.Fatalf("delete conversation failed: %v", err)
	}
	conversations, err := store.ListConversations(1, 0, 0)
	if err != nil {
		t.Fatalf("list conversations failed: %v", err)
	}
	if len(conversations) != 0 {
		t.Fatalf("conversation should be gone, got %d", len(conversations))
	}
	messages, err := store.ConversationMessages(1, conv.ID)
	if err != nil {
		t.Fatalf("conversation messages failed: %v", err)
	}
	if len(messages) != 0 {
		t.Fatalf("messages should be cascade deleted, got %d", len(messages))
	}
}

func TestAIChatStore_RecentMessagesLimitAndOrder(t *testing.T) {
	store := setupAIChatStore(t)
	conv, _ := store.CreateConversation(1)
	for _, text := range []string{"q1", "a1", "q2", "a2", "q3"} {
		role := AIMessageRoleUser
		if strings.HasPrefix(text, "a") {
			role = AIMessageRoleAssistant
		}
		if err := store.AppendMessage(context.Background(), 1, conv.ID, role, text, ""); err != nil {
			t.Fatalf("append message failed: %v", err)
		}
	}

	entries, err := store.RecentMessages(context.Background(), 1, conv.ID, 2)
	if err != nil {
		t.Fatalf("recent messages failed: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("entries = %d, want 2", len(entries))
	}
	if entries[0].Content != "a2" || entries[1].Content != "q3" {
		t.Fatalf("recent entries should be the last two in order, got %+v", entries)
	}

	// 越权读取返回空。
	foreign, err := store.RecentMessages(context.Background(), 2, conv.ID, 2)
	if err != nil || len(foreign) != 0 {
		t.Fatalf("foreign user should see nothing: err=%v entries=%v", err, foreign)
	}
}
