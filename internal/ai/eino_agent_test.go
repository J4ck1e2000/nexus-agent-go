package ai

import (
	"context"
	"strings"
	"testing"

	einomodel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

type scriptedToolCallingModel struct {
	responses []*schema.Message
	index     int
}

func (m *scriptedToolCallingModel) Generate(ctx context.Context, input []*schema.Message, opts ...einomodel.Option) (*schema.Message, error) {
	_ = ctx
	_ = input
	_ = opts
	if m.index >= len(m.responses) {
		return schema.AssistantMessage("", nil), nil
	}
	resp := m.responses[m.index]
	m.index++
	return resp, nil
}

func (m *scriptedToolCallingModel) Stream(ctx context.Context, input []*schema.Message, opts ...einomodel.Option) (*schema.StreamReader[*schema.Message], error) {
	resp, err := m.Generate(ctx, input, opts...)
	if err != nil {
		return nil, err
	}
	if resp == nil {
		return schema.StreamReaderFromArray([]*schema.Message{}), nil
	}
	return schema.StreamReaderFromArray([]*schema.Message{resp}), nil
}

func (m *scriptedToolCallingModel) WithTools(tools []*schema.ToolInfo) (einomodel.ToolCallingChatModel, error) {
	_ = tools
	return m, nil
}

func TestEinoAgentExecutor_ToolCallingLoop(t *testing.T) {
	provider := &staticProvider{
		nodes: []NodeSnapshot{
			buildNode("server-01", "online", 85, 20, 30, 4, 1, 0, 28, 24, []GPUCard{
				{Index: 0, MemoryTotalGB: 24, MemoryUsedGB: 6, EstimatedFreeGB: 18},
			}),
		},
	}
	toolbox := NewToolbox(ToolboxOptions{DataProvider: provider, HistoryProvider: provider})
	model := &scriptedToolCallingModel{
		responses: []*schema.Message{
			schema.AssistantMessage("", []schema.ToolCall{
				{
					ID:   "call_1",
					Type: "function",
					Function: schema.FunctionCall{
						Name:      "get_node_summary",
						Arguments: `{"node_name":"server-01"}`,
					},
				},
			}),
			schema.AssistantMessage(`{"answer":"推荐 server-01","reasoning_summary":"基于工具结果","related_nodes":["server-01"],"warnings":[]}`, nil),
		},
	}

	executor := NewEinoAgentExecutor(EinoAgentExecutorOptions{
		Classifier:   NewIntentClassifier(),
		Toolbox:      toolbox,
		SystemPrompt: DefaultSystemPrompt,
		ChatModel:    model,
		MaxRounds:    4,
	})

	resp, err := executor.Execute(context.Background(), AIQueryRequest{Query: "recommend one node"})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if resp.Mode != AIModeAgent {
		t.Fatalf("mode mismatch: got=%q want=%q", resp.Mode, AIModeAgent)
	}
	if len(resp.ToolCalls) != 1 || resp.ToolCalls[0].Name != "get_node_summary" {
		t.Fatalf("unexpected tool calls: %+v", resp.ToolCalls)
	}
	if resp.Answer == "" {
		t.Fatalf("answer should not be empty")
	}
	if len(resp.RelatedNodes) == 0 || resp.RelatedNodes[0] != "server-01" {
		t.Fatalf("related nodes mismatch: %+v", resp.RelatedNodes)
	}
}

func TestEinoAgentExecutor_UnsupportedToolBecomesWarning(t *testing.T) {
	provider := &staticProvider{
		nodes: []NodeSnapshot{
			buildNode("server-01", "online", 80, 25, 35, 4, 1, 0, 30, 28, []GPUCard{
				{Index: 0, MemoryTotalGB: 24, MemoryUsedGB: 7, EstimatedFreeGB: 17},
			}),
		},
	}
	toolbox := NewToolbox(ToolboxOptions{DataProvider: provider})
	model := &scriptedToolCallingModel{
		responses: []*schema.Message{
			schema.AssistantMessage("", []schema.ToolCall{
				{
					ID:   "call_x",
					Type: "function",
					Function: schema.FunctionCall{
						Name:      "unknown_tool",
						Arguments: `{}`,
					},
				},
			}),
			schema.AssistantMessage(`{"answer":"完成","reasoning_summary":"存在未知工具调用","related_nodes":[],"warnings":[]}`, nil),
		},
	}

	executor := NewEinoAgentExecutor(EinoAgentExecutorOptions{
		Classifier: NewIntentClassifier(),
		Toolbox:    toolbox,
		ChatModel:  model,
		MaxRounds:  4,
	})

	resp, err := executor.Execute(context.Background(), AIQueryRequest{Query: "test"})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if len(resp.Warnings) == 0 {
		t.Fatalf("expected warning for unsupported tool, got none")
	}
}

func TestParseAgentFinalContent_CleaningFallbacks(t *testing.T) {
	cases := []struct {
		name       string
		input      string
		wantAns    string
		wantReason string
	}{
		{
			name:       "plain json",
			input:      `{"answer":"node-a is currently the best fit.","reasoning_summary":"based on idle gpu.","related_nodes":["node-a"]}`,
			wantAns:    "node-a is currently the best fit.",
			wantReason: "based on idle gpu.",
		},
		{
			name:       "fenced json",
			input:      "```json\n{\"answer\":\"node-b is idle.\",\"reasoning_summary\":\"from tool data.\"}\n```",
			wantAns:    "node-b is idle.",
			wantReason: "from tool data.",
		},
		{
			name:       "nested answer json string",
			input:      `{"answer":"{\"answer\":\"node-c looks best\",\"reasoning_summary\":\"nested\"}","reasoning_summary":""}`,
			wantAns:    "node-c looks best",
			wantReason: "nested",
		},
		{
			name:       "plain text fallback",
			input:      "node-d appears healthy and has lower pressure.",
			wantAns:    "node-d appears healthy and has lower pressure.",
			wantReason: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			parsed := parseAgentFinalContent(tc.input)
			if strings.TrimSpace(parsed.Answer) != tc.wantAns {
				t.Fatalf("answer mismatch: got=%q want=%q", parsed.Answer, tc.wantAns)
			}
			if tc.wantReason != "" && strings.TrimSpace(parsed.ReasoningSummary) != tc.wantReason {
				t.Fatalf("reasoning mismatch: got=%q want=%q", parsed.ReasoningSummary, tc.wantReason)
			}
			if strings.Contains(parsed.Answer, "```") {
				t.Fatalf("answer should not contain code fences: %q", parsed.Answer)
			}
			if strings.HasPrefix(strings.TrimSpace(parsed.Answer), "{") && strings.HasSuffix(strings.TrimSpace(parsed.Answer), "}") {
				t.Fatalf("answer should not remain raw json object: %q", parsed.Answer)
			}
		})
	}
}

func TestRewriteOperationalAnswer_ScheduleSuggestion(t *testing.T) {
	intent := QueryIntent{
		Type: IntentScheduleSuggestion,
		TopK: 2,
	}
	answer := rewriteOperationalAnswer(intent, "raw technical answer", []string{"node-a", "node-b"}, responseLanguageZH)
	if !strings.Contains(answer, "node-a") || !strings.Contains(answer, "node-b") {
		t.Fatalf("rewritten answer should contain both nodes, got=%q", answer)
	}
	if !strings.Contains(answer, "建议优先使用") {
		t.Fatalf("rewritten answer should use concise operational style, got=%q", answer)
	}

	reason := rewriteReasoningSummary(intent, "raw reason", []string{"node-a", "node-b"}, responseLanguageZH)
	if !strings.Contains(reason, "判断依据") {
		t.Fatalf("rewritten reasoning mismatch: %q", reason)
	}
}

func TestRewriteOperationalAnswer_KeepLanguageMatchedOutput(t *testing.T) {
	intent := QueryIntent{
		Type: IntentIdleNodeRanking,
		TopK: 1,
	}
	current := "结论：node-a 当前最空闲，建议优先放置任务。"
	answer := rewriteOperationalAnswer(intent, current, []string{"node-a"}, responseLanguageZH)
	if answer != current {
		t.Fatalf("expected matched-language answer to be kept, got=%q", answer)
	}
}

func TestRewriteOperationalAnswer_LocalizeWhenLanguageMismatch(t *testing.T) {
	intent := QueryIntent{
		Type: IntentIdleNodeRanking,
		TopK: 1,
	}
	answer := rewriteOperationalAnswer(intent, "node-a is currently the most idle node.", []string{"node-a"}, responseLanguageZH)
	if !strings.Contains(answer, "结论") {
		t.Fatalf("expected chinese fallback answer, got=%q", answer)
	}
	reason := rewriteReasoningSummary(intent, "based on cpu/ram/gpu headroom", []string{"node-a"}, responseLanguageZH)
	if !strings.Contains(reason, "判断依据") {
		t.Fatalf("expected chinese fallback reasoning, got=%q", reason)
	}
}
