package ai

import (
	"context"
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
