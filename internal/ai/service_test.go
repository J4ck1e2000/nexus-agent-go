package ai

import (
	"context"
	"errors"
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
