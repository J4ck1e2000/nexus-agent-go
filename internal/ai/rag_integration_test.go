package ai

import (
	"context"
	"strings"
	"testing"
)

func TestRuleExecutor_AnomalyResponseIncludesKnowledgeSupplement(t *testing.T) {
	retriever := buildTestRetriever(t)
	provider := &staticProvider{
		nodes: []NodeSnapshot{
			buildNode("server-01", "online", 62, 35, 58, 5, 0, 1, 18, 97, []GPUCard{
				{Index: 0, MemoryTotalGB: 24, MemoryUsedGB: 23, EstimatedFreeGB: 1},
			}),
		},
	}
	toolbox := NewToolbox(ToolboxOptions{
		DataProvider:       provider,
		KnowledgeRetriever: retriever,
		KnowledgeTopK:      3,
	})
	executor := NewRuleExecutor(NewIntentClassifier(), toolbox)

	resp, err := executor.Execute(context.Background(), AIQueryRequest{
		Query: "why is server-01 GPU memory so high",
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if len(resp.KnowledgeHits) == 0 {
		t.Fatalf("expected knowledge hits in anomaly response")
	}
	if resp.Retrieval == nil || !resp.Retrieval.Hit {
		t.Fatalf("expected retrieval meta hit, got=%+v", resp.Retrieval)
	}
	if !strings.Contains(strings.ToLower(resp.Answer), "knowledge-base common patterns") {
		t.Fatalf("answer should include knowledge supplement, got=%q", resp.Answer)
	}
}

func TestRuleExecutor_WithoutKnowledgeDegradesGracefully(t *testing.T) {
	provider := &staticProvider{
		nodes: []NodeSnapshot{
			buildNode("server-01", "online", 62, 35, 58, 5, 0, 1, 18, 97, []GPUCard{
				{Index: 0, MemoryTotalGB: 24, MemoryUsedGB: 23, EstimatedFreeGB: 1},
			}),
		},
	}
	toolbox := NewToolbox(ToolboxOptions{
		DataProvider: provider,
	})
	executor := NewRuleExecutor(NewIntentClassifier(), toolbox)

	resp, err := executor.Execute(context.Background(), AIQueryRequest{
		Query: "why is server-01 GPU memory so high",
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if strings.TrimSpace(resp.Answer) == "" {
		t.Fatalf("answer should not be empty")
	}
	if len(resp.KnowledgeHits) != 0 {
		t.Fatalf("knowledge hits should be empty when knowledge is disabled")
	}
	if resp.Retrieval != nil {
		t.Fatalf("retrieval meta should be nil when no retrieval attempted")
	}
}

func TestService_QueryStream_WithKnowledgeDoesNotCrash(t *testing.T) {
	retriever := buildTestRetriever(t)
	toolbox := NewToolbox(ToolboxOptions{
		DataProvider: &staticProvider{
			nodes: []NodeSnapshot{
				buildNode("server-01", "online", 80, 30, 40, 4, 1, 1, 30, 42, []GPUCard{
					{Index: 0, MemoryTotalGB: 24, MemoryUsedGB: 10, EstimatedFreeGB: 14},
				}),
			},
		},
		KnowledgeRetriever: retriever,
	})
	service := NewService(ServiceOptions{
		Config: Config{
			Enabled: true,
			Mode:    AIModeRule,
		},
		Toolbox: toolbox,
	})

	var done *AIQueryResponse
	err := service.QueryStream(context.Background(), AIQueryRequest{
		Query:  "CUDA OOM how to fix",
		Stream: true,
	}, func(event AIStreamEvent) error {
		if event.Event == StreamEventDone && event.Done != nil {
			done = event.Done
		}
		return nil
	})
	if err != nil {
		t.Fatalf("QueryStream failed: %v", err)
	}
	if done == nil {
		t.Fatalf("expected done response")
	}
	if strings.TrimSpace(done.Answer) == "" {
		t.Fatalf("done answer should not be empty")
	}
}
