package ai

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

type scriptedChatClient struct {
	responses []ChatCompletionResponse
	index     int
}

func (c *scriptedChatClient) CreateChatCompletion(ctx context.Context, req ChatCompletionRequest) (ChatCompletionResponse, error) {
	_ = ctx
	_ = req
	if c.index >= len(c.responses) {
		return ChatCompletionResponse{}, nil
	}
	resp := c.responses[c.index]
	c.index++
	return resp, nil
}

func (c *scriptedChatClient) CreateChatCompletionStream(ctx context.Context, req ChatCompletionRequest, onChunk func(ChatCompletionStreamChunk) error) error {
	resp, err := c.CreateChatCompletion(ctx, req)
	if err != nil {
		return err
	}
	if onChunk == nil {
		return fmt.Errorf("stream callback is nil")
	}

	chunk := ChatCompletionStreamChunk{
		ID:    resp.ID,
		Model: resp.Model,
	}
	for _, choice := range resp.Choices {
		chunk.Choices = append(chunk.Choices, ChatCompletionStreamChoice{
			Index: choice.Index,
			Delta: ChatMessageDelta{
				Role:      choice.Message.Role,
				Content:   choice.Message.Content,
				ToolCalls: completeToolCallsToDelta(choice.Message.ToolCalls),
			},
			FinishReason: choice.FinishReason,
		})
	}
	return onChunk(chunk)
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
	client := &scriptedChatClient{
		responses: []ChatCompletionResponse{
			{
				Choices: []ChatCompletionChoice{
					{
						Message: ChatMessage{
							Role: "assistant",
							ToolCalls: []ChatToolCall{
								{
									ID:   "call_1",
									Type: "function",
									Function: ChatFunctionCall{
										Name:      "get_node_summary",
										Arguments: `{"node_name":"server-01"}`,
									},
								},
							},
						},
					},
				},
			},
			{
				Choices: []ChatCompletionChoice{
					{
						Message: ChatMessage{
							Role:    "assistant",
							Content: `{"answer":"推荐 server-01","reasoning_summary":"基于工具结果","related_nodes":["server-01"],"warnings":[]}`,
						},
					},
				},
			},
		},
	}

	executor := NewEinoAgentExecutor(EinoAgentExecutorOptions{
		Classifier:   NewIntentClassifier(),
		Toolbox:      toolbox,
		SystemPrompt: DefaultSystemPrompt,
		LLMClient:    client,
		Model:        "qwen-plus",
		MaxRounds:    4,
	})

	resp, err := executor.Execute(context.Background(), AIQueryRequest{Query: "推荐一台机器"})
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
	client := &scriptedChatClient{
		responses: []ChatCompletionResponse{
			{
				Choices: []ChatCompletionChoice{
					{
						Message: ChatMessage{
							Role: "assistant",
							ToolCalls: []ChatToolCall{
								{
									ID:   "call_x",
									Type: "function",
									Function: ChatFunctionCall{
										Name:      "unknown_tool",
										Arguments: `{}`,
									},
								},
							},
						},
					},
				},
			},
			{
				Choices: []ChatCompletionChoice{
					{
						Message: ChatMessage{
							Role:    "assistant",
							Content: `{"answer":"完成","reasoning_summary":"有工具失败","related_nodes":[],"warnings":[]}`,
						},
					},
				},
			},
		},
	}

	executor := NewEinoAgentExecutor(EinoAgentExecutorOptions{
		Classifier: NewIntentClassifier(),
		Toolbox:    toolbox,
		LLMClient:  client,
		Model:      "qwen-plus",
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
	if !strings.Contains(reason, "依据是 CPU、内存和 GPU 余量") {
		t.Fatalf("rewritten reasoning mismatch: %q", reason)
	}
}
