package ai

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	"nexus-agent-go/internal/runtime"
)

// stubConversationStore 是内存版 ConversationStore。
type stubConversationStore struct {
	mu         sync.Mutex
	owners     map[uint]int64 // conversationID -> userID
	messages   []stubMessage
	ownedByErr error
	recentErr  error
	appendErr  error
	nextID     uint
}

type stubMessage struct {
	ConversationID uint
	UserID         int64
	Role           string
	Content        string
	MetaJSON       string
}

func newStubConversationStore() *stubConversationStore {
	return &stubConversationStore{owners: map[uint]int64{}, nextID: 1}
}

func (s *stubConversationStore) create(userID int64) uint {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := s.nextID
	s.nextID++
	s.owners[id] = userID
	return id
}

func (s *stubConversationStore) ConversationOwnedBy(_ context.Context, userID int64, conversationID uint) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ownedByErr != nil {
		return false, s.ownedByErr
	}
	return s.owners[conversationID] == userID, nil
}

func (s *stubConversationStore) RecentMessages(_ context.Context, userID int64, conversationID uint, limit int) ([]runtime.RunHistoryEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.recentErr != nil {
		return nil, s.recentErr
	}
	var entries []runtime.RunHistoryEntry
	for _, msg := range s.messages {
		if msg.ConversationID != conversationID || msg.UserID != userID {
			continue
		}
		entries = append(entries, runtime.RunHistoryEntry{Role: msg.Role, Content: msg.Content})
	}
	if len(entries) > limit {
		entries = entries[len(entries)-limit:]
	}
	return entries, nil
}

func (s *stubConversationStore) AppendMessage(_ context.Context, userID int64, conversationID uint, role, content, metaJSON string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.appendErr != nil {
		return s.appendErr
	}
	s.messages = append(s.messages, stubMessage{
		ConversationID: conversationID,
		UserID:         userID,
		Role:           role,
		Content:        content,
		MetaJSON:       metaJSON,
	})
	return nil
}

func (s *stubConversationStore) userMessages(userID int64) []stubMessage {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []stubMessage
	for _, msg := range s.messages {
		if msg.UserID == userID {
			out = append(out, msg)
		}
	}
	return out
}

// recordingExecutor 记录调用并返回固定回答。
type recordingExecutor struct {
	mu          sync.Mutex
	executeCtx  context.Context
	executeReqs []AIQueryRequest
	executeErr  error
	resp        AIQueryResponse
}

func (e *recordingExecutor) Execute(ctx context.Context, req AIQueryRequest) (AIQueryResponse, error) {
	e.mu.Lock()
	e.executeCtx = ctx
	e.executeReqs = append(e.executeReqs, req)
	e.mu.Unlock()
	if e.executeErr != nil {
		return AIQueryResponse{}, e.executeErr
	}
	return e.resp, nil
}

func (e *recordingExecutor) ExecuteStream(ctx context.Context, req AIQueryRequest, emit func(AIStreamEvent) error) (AIQueryResponse, error) {
	if emit != nil {
		_ = emit(AIStreamEvent{Event: StreamEventDelta, Text: "partial "})
	}
	return e.Execute(ctx, req)
}

func (e *recordingExecutor) Health(_ context.Context) error { return nil }

func (e *recordingExecutor) lastRequest() AIQueryRequest {
	e.mu.Lock()
	defer e.mu.Unlock()
	if len(e.executeReqs) == 0 {
		return AIQueryRequest{}
	}
	return e.executeReqs[len(e.executeReqs)-1]
}

func (e *recordingExecutor) sawHistory() []runtime.RunHistoryEntry {
	e.mu.Lock()
	defer e.mu.Unlock()
	return runtime.RecentHistoryFromCtx(e.executeCtx)
}

func newTestPiQueryService(store ConversationStore, executor *recordingExecutor) *PiQueryService {
	support := NewService(ServiceOptions{Config: Config{Enabled: true, Mode: AIModeAgent}})
	return NewPiQueryService(support, executor, store)
}

func principalContext(userID int64) context.Context {
	return runtime.WithPrincipal(context.Background(), runtime.RunPrincipal{
		UserID: userID, Username: "alice", Role: "user",
	})
}

func TestPiQueryService_QueryWithoutConversationSkipsPersistence(t *testing.T) {
	store := newStubConversationStore()
	executor := &recordingExecutor{resp: AIQueryResponse{Answer: "ok", Mode: AIModeAgent}}
	service := newTestPiQueryService(store, executor)

	resp, err := service.Query(principalContext(1), AIQueryRequest{Query: "hi"})
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if resp.ConversationID != 0 {
		t.Fatalf("conversation id should stay 0, got %d", resp.ConversationID)
	}
	if len(store.userMessages(1)) != 0 {
		t.Fatalf("nothing should be persisted without conversation id")
	}
	if entries := executor.sawHistory(); entries != nil {
		t.Fatalf("no history should be injected, got %v", entries)
	}
}

func TestPiQueryService_QueryPersistsExchange(t *testing.T) {
	store := newStubConversationStore()
	conversationID := store.create(1)
	executor := &recordingExecutor{resp: AIQueryResponse{
		Answer:           "答案",
		ReasoningSummary: "推理",
		Mode:             AIModeAgent,
		RelatedNodes:     []string{"node-1"},
	}}
	service := newTestPiQueryService(store, executor)

	resp, err := service.Query(principalContext(1), AIQueryRequest{Query: "第一问", ConversationID: conversationID})
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if resp.ConversationID != conversationID {
		t.Fatalf("response conversation id = %d, want %d", resp.ConversationID, conversationID)
	}

	messages := store.userMessages(1)
	if len(messages) != 2 {
		t.Fatalf("persisted %d messages, want 2 (user+assistant)", len(messages))
	}
	if messages[0].Role != "user" || messages[0].Content != "第一问" {
		t.Fatalf("unexpected user message: %+v", messages[0])
	}
	if messages[1].Role != "assistant" || messages[1].Content != "答案" {
		t.Fatalf("unexpected assistant message: %+v", messages[1])
	}
	var meta map[string]any
	if err := json.Unmarshal([]byte(messages[1].MetaJSON), &meta); err != nil {
		t.Fatalf("assistant meta should be valid json: %v", err)
	}
	if meta["related_nodes"] == nil {
		t.Fatalf("assistant meta should keep related_nodes, got %v", meta)
	}
}

func TestPiQueryService_QueryInjectsRecentHistory(t *testing.T) {
	store := newStubConversationStore()
	conversationID := store.create(1)
	// 预置三条历史，验证按时间正序注入。
	store.AppendMessage(principalContext(1), 1, conversationID, "user", "q1", "")
	store.AppendMessage(principalContext(1), 1, conversationID, "assistant", "a1", "")
	store.AppendMessage(principalContext(1), 1, conversationID, "user", "q2", "")

	executor := &recordingExecutor{resp: AIQueryResponse{Answer: "ok", Mode: AIModeAgent}}
	service := newTestPiQueryService(store, executor)

	if _, err := service.Query(principalContext(1), AIQueryRequest{Query: "q3", ConversationID: conversationID}); err != nil {
		t.Fatalf("query failed: %v", err)
	}
	entries := executor.sawHistory()
	if len(entries) != 3 {
		t.Fatalf("history entries = %d, want 3", len(entries))
	}
	if entries[0].Content != "q1" || entries[2].Content != "q2" {
		t.Fatalf("history should be chronological, got %+v", entries)
	}
	if req := executor.lastRequest(); req.ConversationID != conversationID {
		t.Fatalf("executor should receive conversation id, got %d", req.ConversationID)
	}
}

func TestPiQueryService_QueryForeignConversationRejected(t *testing.T) {
	store := newStubConversationStore()
	conversationID := store.create(2) // 属于用户 2
	executor := &recordingExecutor{resp: AIQueryResponse{Answer: "ok", Mode: AIModeAgent}}
	service := newTestPiQueryService(store, executor)

	_, err := service.Query(principalContext(1), AIQueryRequest{Query: "hi", ConversationID: conversationID})
	if !errors.Is(err, ErrInvalidConversation) {
		t.Fatalf("err = %v, want ErrInvalidConversation", err)
	}
	if len(store.userMessages(1)) != 0 {
		t.Fatalf("nothing should be persisted for foreign conversation")
	}
}

func TestPiQueryService_RecentFailureDegradesGracefully(t *testing.T) {
	store := newStubConversationStore()
	conversationID := store.create(1)
	store.recentErr = errors.New("db down")
	executor := &recordingExecutor{resp: AIQueryResponse{Answer: "ok", Mode: AIModeAgent}}
	service := newTestPiQueryService(store, executor)

	resp, err := service.Query(principalContext(1), AIQueryRequest{Query: "hi", ConversationID: conversationID})
	if err != nil {
		t.Fatalf("query should degrade when history read fails: %v", err)
	}
	if resp.ConversationID != conversationID {
		t.Fatalf("conversation id = %d, want %d", resp.ConversationID, conversationID)
	}
	if entries := executor.sawHistory(); entries != nil {
		t.Fatalf("history should be empty after read failure, got %v", entries)
	}
	if len(store.userMessages(1)) != 2 {
		t.Fatalf("exchange should still be persisted, got %d", len(store.userMessages(1)))
	}
}

func TestPiQueryService_EmptyAnswerNotPersisted(t *testing.T) {
	store := newStubConversationStore()
	conversationID := store.create(1)
	executor := &recordingExecutor{resp: AIQueryResponse{Answer: "", Mode: AIModeAgent}}
	service := newTestPiQueryService(store, executor)

	if _, err := service.Query(principalContext(1), AIQueryRequest{Query: "hi", ConversationID: conversationID}); err != nil {
		t.Fatalf("query failed: %v", err)
	}
	messages := store.userMessages(1)
	if len(messages) != 1 || messages[0].Role != "user" {
		t.Fatalf("only the user message should be persisted, got %+v", messages)
	}
}

func TestPiQueryService_QueryStreamCarriesConversation(t *testing.T) {
	store := newStubConversationStore()
	conversationID := store.create(1)
	executor := &recordingExecutor{resp: AIQueryResponse{Answer: "流式答案", Mode: AIModeAgent}}
	service := newTestPiQueryService(store, executor)

	var events []AIStreamEvent
	err := service.QueryStream(principalContext(1), AIQueryRequest{Query: "问", ConversationID: conversationID}, func(event AIStreamEvent) error {
		events = append(events, event)
		return nil
	})
	if err != nil {
		t.Fatalf("stream failed: %v", err)
	}

	var sawMetaID, sawDoneID uint
	for _, event := range events {
		switch event.Event {
		case StreamEventMeta:
			sawMetaID = event.Meta.ConversationID
		case StreamEventDone:
			sawDoneID = event.Done.ConversationID
		}
	}
	if sawMetaID != conversationID || sawDoneID != conversationID {
		t.Fatalf("meta/done conversation id = %d/%d, want %d", sawMetaID, sawDoneID, conversationID)
	}
	messages := store.userMessages(1)
	if len(messages) != 2 {
		t.Fatalf("persisted %d messages, want 2", len(messages))
	}
	if messages[0].Content != "问" || messages[1].Content != "流式答案" {
		t.Fatalf("unexpected persisted messages: %+v", messages)
	}
}

func TestPiQueryService_NilStoreKeepsLegacyBehavior(t *testing.T) {
	executor := &recordingExecutor{resp: AIQueryResponse{Answer: "ok", Mode: AIModeAgent}}
	service := NewPiQueryService(NewService(ServiceOptions{Config: Config{Enabled: true, Mode: AIModeAgent}}), executor, nil)

	resp, err := service.Query(principalContext(1), AIQueryRequest{Query: "hi", ConversationID: 7})
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if resp.ConversationID != 0 {
		t.Fatalf("nil store should keep conversation id 0, got %d", resp.ConversationID)
	}
	if entries := executor.sawHistory(); entries != nil {
		t.Fatalf("nil store should not inject history")
	}
}

func TestPiQueryService_DisabledServiceStillDisabled(t *testing.T) {
	executor := &recordingExecutor{resp: AIQueryResponse{Answer: "ok", Mode: AIModeAgent}}
	service := NewPiQueryService(NewService(ServiceOptions{Config: Config{Enabled: false}}), executor, nil)
	if _, err := service.Query(context.Background(), AIQueryRequest{Query: "hi"}); !errors.Is(err, ErrServiceDisabled) {
		t.Fatalf("err = %v, want ErrServiceDisabled", err)
	}
}
