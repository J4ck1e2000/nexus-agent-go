package ai

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type stubExecutor struct {
	response AIQueryResponse
	err      error
}

func (s *stubExecutor) Execute(ctx context.Context, req AIQueryRequest) (AIQueryResponse, error) {
	_ = ctx
	_ = req
	if s.err != nil {
		return AIQueryResponse{}, s.err
	}
	return s.response, nil
}

type stubStreamExecutor struct {
	response AIQueryResponse
	err      error
}

func (s *stubStreamExecutor) Execute(ctx context.Context, req AIQueryRequest) (AIQueryResponse, error) {
	_ = ctx
	_ = req
	if s.err != nil {
		return AIQueryResponse{}, s.err
	}
	return s.response, nil
}

func (s *stubStreamExecutor) ExecuteStream(ctx context.Context, req AIQueryRequest, emit func(AIStreamEvent) error) (AIQueryResponse, error) {
	_ = ctx
	_ = req
	if emit != nil {
		_ = emit(AIStreamEvent{Event: StreamEventStatus, Phase: "thinking", Message: "test"})
	}
	if s.err != nil {
		return AIQueryResponse{}, s.err
	}
	return s.response, nil
}

func TestService_NoModelConfigUsesRuleMode(t *testing.T) {
	nodes := []NodeSnapshot{
		buildNode("server-01", "online", 80, 30, 40, 4, 1, 1, 35, 30, []GPUCard{
			{Index: 0, MemoryTotalGB: 24, MemoryUsedGB: 8, EstimatedFreeGB: 16},
		}),
	}
	provider := &staticProvider{nodes: nodes}
	toolbox := NewToolbox(ToolboxOptions{DataProvider: provider})
	service := NewService(ServiceOptions{
		Config: Config{
			Enabled: true,
			Mode:    AIModeAgent,
		},
		Toolbox: toolbox,
	})

	resp, err := service.Query(context.Background(), AIQueryRequest{Query: "哪台机器最空闲？"})
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if resp.Mode != AIModeRule {
		t.Fatalf("mode mismatch: got=%q want=%q", resp.Mode, AIModeRule)
	}
}

func TestService_AgentFailureFallsBackToRule(t *testing.T) {
	rule := &stubExecutor{
		response: AIQueryResponse{
			Answer:           "rule answer",
			ReasoningSummary: "rule reasoning",
			Mode:             AIModeRule,
			ToolCalls: []ToolCallRecord{
				{Name: "list_idle_nodes", Args: map[string]any{"limit": 1}},
			},
		},
	}
	agent := &stubExecutor{
		err: errors.New("agent failed"),
	}

	service := NewService(ServiceOptions{
		Config: Config{
			Enabled:  true,
			Mode:     AIModeAgent,
			Provider: "openai",
			Model:    "gpt-test",
			APIKey:   "test-key",
			BaseURL:  "http://127.0.0.1/mock",
		},
		RuleExecutor:  rule,
		AgentExecutor: agent,
	})

	resp, err := service.Query(context.Background(), AIQueryRequest{Query: "推荐一台机器"})
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if resp.Mode != AIModeRule {
		t.Fatalf("mode mismatch: got=%q want=%q", resp.Mode, AIModeRule)
	}
	if len(resp.ToolCalls) == 0 {
		t.Fatalf("tool calls should be kept in fallback response")
	}
	if len(resp.Warnings) == 0 {
		t.Fatalf("fallback warning should exist")
	}
}

func TestService_Disabled(t *testing.T) {
	service := NewService(ServiceOptions{
		Config: Config{
			Enabled: false,
			Mode:    AIModeRule,
		},
		RuleExecutor: &stubExecutor{},
	})

	_, err := service.Query(context.Background(), AIQueryRequest{Query: "test"})
	if !errors.Is(err, ErrServiceDisabled) {
		t.Fatalf("expected ErrServiceDisabled, got=%v", err)
	}
}

func TestService_QueryStream_EmitsEventSequence(t *testing.T) {
	rule := &stubExecutor{
		response: AIQueryResponse{
			Answer:           "node-01 is currently the best option because it has idle GPU and lower CPU pressure.",
			ReasoningSummary: "Sorted by availability score and idle GPU.",
			Mode:             AIModeRule,
			RelatedNodes:     []string{"node-01"},
		},
	}
	service := NewService(ServiceOptions{
		Config: Config{
			Enabled: true,
			Mode:    AIModeRule,
		},
		RuleExecutor: rule,
	})

	events := make([]AIStreamEvent, 0, 8)
	err := service.QueryStream(context.Background(), AIQueryRequest{Query: "recommend one node", Stream: true}, func(event AIStreamEvent) error {
		events = append(events, event)
		return nil
	})
	if err != nil {
		t.Fatalf("QueryStream failed: %v", err)
	}
	if len(events) < 5 {
		t.Fatalf("expected at least 5 events, got=%d", len(events))
	}
	if events[0].Event != StreamEventStart {
		t.Fatalf("first event should be start, got=%q", events[0].Event)
	}
	if events[len(events)-1].Event != StreamEventDone {
		t.Fatalf("last event should be done, got=%q", events[len(events)-1].Event)
	}

	hasDelta := false
	hasMeta := false
	for _, event := range events {
		if event.Event == StreamEventDelta && strings.TrimSpace(event.Text) != "" {
			hasDelta = true
		}
		if event.Event == StreamEventMeta && event.Meta != nil {
			hasMeta = true
		}
	}
	if !hasDelta {
		t.Fatalf("expected at least one delta event")
	}
	if !hasMeta {
		t.Fatalf("expected meta event")
	}
}

func TestService_QueryStream_AgentFailureFallsBackToRule(t *testing.T) {
	rule := &stubExecutor{
		response: AIQueryResponse{
			Answer:           "rule fallback answer",
			ReasoningSummary: "rule summary",
			Mode:             AIModeRule,
		},
	}
	agent := &stubStreamExecutor{
		err: errors.New("agent stream failed"),
	}

	service := NewService(ServiceOptions{
		Config: Config{
			Enabled:  true,
			Mode:     AIModeAgent,
			Provider: "openai",
			Model:    "gpt-test",
			APIKey:   "test-key",
			BaseURL:  "http://127.0.0.1/mock",
		},
		RuleExecutor:  rule,
		AgentExecutor: agent,
	})

	var doneEvent *AIStreamEvent
	err := service.QueryStream(context.Background(), AIQueryRequest{Query: "fallback test", Stream: true}, func(event AIStreamEvent) error {
		if event.Event == StreamEventDone {
			copyEvent := event
			doneEvent = &copyEvent
		}
		return nil
	})
	if err != nil {
		t.Fatalf("QueryStream failed: %v", err)
	}
	if doneEvent == nil || doneEvent.Done == nil {
		t.Fatalf("expected done event with response")
	}
	if doneEvent.Done.Mode != AIModeRule {
		t.Fatalf("expected fallback mode rule, got=%q", doneEvent.Done.Mode)
	}
	if len(doneEvent.Done.Warnings) == 0 {
		t.Fatalf("expected fallback warning in done payload")
	}
}

func TestService_AgentFallbackWarningLocalizedByQueryLanguage(t *testing.T) {
	rule := &stubExecutor{
		response: AIQueryResponse{
			Answer:           "rule answer",
			ReasoningSummary: "rule reasoning",
			Mode:             AIModeRule,
		},
	}
	agent := &stubExecutor{err: errors.New("agent failed")}

	service := NewService(ServiceOptions{
		Config: Config{
			Enabled:  true,
			Mode:     AIModeAgent,
			Provider: "openai",
			Model:    "gpt-test",
			APIKey:   "test-key",
			BaseURL:  "http://127.0.0.1/mock",
		},
		RuleExecutor:  rule,
		AgentExecutor: agent,
	})

	zhResp, err := service.Query(context.Background(), AIQueryRequest{Query: "推荐一台机器"})
	if err != nil {
		t.Fatalf("zh Query failed: %v", err)
	}
	if len(zhResp.Warnings) == 0 || !strings.Contains(strings.Join(zhResp.Warnings, " "), "切换为规则模式") {
		t.Fatalf("zh fallback warning mismatch: %+v", zhResp.Warnings)
	}

	enResp, err := service.Query(context.Background(), AIQueryRequest{Query: "recommend one node"})
	if err != nil {
		t.Fatalf("en Query failed: %v", err)
	}
	if len(enResp.Warnings) == 0 || !strings.Contains(strings.Join(enResp.Warnings, " "), "switched to rule mode") {
		t.Fatalf("en fallback warning mismatch: %+v", enResp.Warnings)
	}
}
