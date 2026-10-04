package gateway

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func aiConversationIDPath(suffix string, id uint) string {
	return "/api/ai/conversations/" + strconv.FormatUint(uint64(id), 10) + suffix
}

func setupAIConversationTestRouter(t *testing.T) *gin.Engine {
	t.Helper()
	r, _, _ := setupAIConversationTestRouterWithDB(t)
	return r
}

func setupAIConversationTestRouterWithDB(t *testing.T) (*gin.Engine, *AIChatStore, *gorm.DB) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	dsn := "file:" + strings.ReplaceAll(t.Name(), "/", "_") + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite failed: %v", err)
	}
	if err := AutoMigrate(db); err != nil {
		t.Fatalf("auto migrate failed: %v", err)
	}
	seedTestUser(t, db, "admin", "admin123", RoleAdmin)
	seedTestUser(t, db, "user", "user123", RoleUser)

	chatStore := NewAIChatStore(db)
	r := gin.New()
	h := NewHandler(NewConfigStore(db), NewAuthService(db, "test-secret"), VersionInfo{})
	h.SetAIChatStore(chatStore)
	h.RegisterAPIRoutes(r)
	return r, chatStore, db
}

func setupAIConversationTestRouterWithoutStore(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)

	dsn := "file:" + strings.ReplaceAll(t.Name(), "/", "_") + "no-store?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite failed: %v", err)
	}
	if err := AutoMigrate(db); err != nil {
		t.Fatalf("auto migrate failed: %v", err)
	}
	seedTestUser(t, db, "admin", "admin123", RoleAdmin)

	r := gin.New()
	h := NewHandler(NewConfigStore(db), NewAuthService(db, "test-secret"), VersionInfo{})
	h.RegisterAPIRoutes(r)
	return r
}

func TestAIConversationCRUDLifecycle(t *testing.T) {
	r, chatStore, _ := setupAIConversationTestRouterWithDB(t)
	adminToken := loginAndGetToken(t, r, "admin", "admin123")

	// 创建会话
	req := authorizedRequest("POST", "/api/ai/conversations", nil, adminToken)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create conversation status = %d body=%s", w.Code, w.Body.String())
	}
	var created AIConversation
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatalf("parse create response failed: %v", err)
	}
	if created.ID == 0 {
		t.Fatal("conversation id should not be empty")
	}

	// 首条提问落库后标题自动补全，列表可见
	if err := chatStore.AppendMessage(nil, int64(created.UserID), created.ID, AIMessageRoleUser, "帮我看看 GPU 状态", ""); err != nil {
		t.Fatalf("append message failed: %v", err)
	}
	req = authorizedRequest("GET", "/api/ai/conversations", nil, adminToken)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("list conversations status = %d", w.Code)
	}
	var listResp struct {
		Conversations []AIConversation `json:"conversations"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("parse list response failed: %v", err)
	}
	if len(listResp.Conversations) != 1 || listResp.Conversations[0].ID != created.ID {
		t.Fatalf("list should contain the created conversation, got %+v", listResp.Conversations)
	}
	if listResp.Conversations[0].Title == "" {
		t.Fatal("title should be derived from first user message")
	}

	// 消息接口返回已落库消息
	req = authorizedRequest("GET", aiConversationIDPath("/messages", created.ID), nil, adminToken)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("messages status = %d", w.Code)
	}
	var messagesResp struct {
		Messages []AIMessageView `json:"messages"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &messagesResp); err != nil {
		t.Fatalf("parse messages response failed: %v", err)
	}
	if len(messagesResp.Messages) != 1 || messagesResp.Messages[0].Content != "帮我看看 GPU 状态" {
		t.Fatalf("unexpected messages: %+v", messagesResp.Messages)
	}

	// 不存在的会话返回空数组（不泄露他人会话）
	req = authorizedRequest("GET", aiConversationIDPath("/messages", 999), nil, adminToken)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK || w.Body.String() != `{"messages":[]}` {
		t.Fatalf("missing conversation messages should be empty list, got %d %s", w.Code, w.Body.String())
	}

	// 删除会话
	req = authorizedRequest("DELETE", aiConversationIDPath("", created.ID), nil, adminToken)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("delete conversation status = %d body=%s", w.Code, w.Body.String())
	}

	// 重复删除应 404
	req = authorizedRequest("DELETE", aiConversationIDPath("", created.ID), nil, adminToken)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound || w.Body.String() != `{"error":"conversation_not_found"}` {
		t.Fatalf("second delete should 404, got %d %s", w.Code, w.Body.String())
	}
}

func TestAIConversationCRUD_OwnershipAndErrorCodes(t *testing.T) {
	r, chatStore, _ := setupAIConversationTestRouterWithDB(t)
	adminToken := loginAndGetToken(t, r, "admin", "admin123")
	userToken := loginAndGetToken(t, r, "user", "user123")

	req := authorizedRequest("POST", "/api/ai/conversations", nil, adminToken)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	var created AIConversation
	_ = json.Unmarshal(w.Body.Bytes(), &created)
	_ = chatStore.AppendMessage(nil, 1, created.ID, AIMessageRoleUser, "私有数据", "")

	tests := []struct {
		name     string
		method   string
		path     string
		token    string
		wantCode int
		wantBody string
	}{
		{
			name:     "unauthenticated list",
			method:   "GET",
			path:     "/api/ai/conversations",
			token:    "",
			wantCode: http.StatusUnauthorized,
			wantBody: `{"error":"unauthorized"}`,
		},
		{
			name:     "foreign messages hidden",
			method:   "GET",
			path:     aiConversationIDPath("/messages", created.ID),
			token:    userToken,
			wantCode: http.StatusOK,
			wantBody: `{"messages":[]}`,
		},
		{
			name:     "foreign delete hidden as 404",
			method:   "DELETE",
			path:     aiConversationIDPath("", created.ID),
			token:    userToken,
			wantCode: http.StatusNotFound,
			wantBody: `{"error":"conversation_not_found"}`,
		},
		{
			name:     "invalid id param",
			method:   "GET",
			path:     "/api/ai/conversations/not-a-number/messages",
			token:    adminToken,
			wantCode: http.StatusBadRequest,
			wantBody: `{"error":"invalid_payload"}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := authorizedRequest(tt.method, tt.path, nil, tt.token)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if w.Code != tt.wantCode {
				t.Fatalf("status = %d, want %d body=%s", w.Code, tt.wantCode, w.Body.String())
			}
			if tt.wantBody != "" && w.Body.String() != tt.wantBody {
				t.Fatalf("body = %s, want %s", w.Body.String(), tt.wantBody)
			}
		})
	}
}

func TestAIConversationRoutes_WithoutStore(t *testing.T) {
	r := setupAIConversationTestRouterWithoutStore(t)
	token := loginAndGetToken(t, r, "admin", "admin123")

	req := authorizedRequest("GET", "/api/ai/conversations", nil, token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable || w.Body.String() != `{"error":"ai_unavailable"}` {
		t.Fatalf("status = %d body=%s, want 503 ai_unavailable", w.Code, w.Body.String())
	}
}
