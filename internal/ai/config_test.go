package ai

import "testing"

func TestLoadConfigFromEnv_QwenDefaultBaseURL(t *testing.T) {
	t.Setenv(envAIEnabled, "true")
	t.Setenv(envAIMode, "agent")
	t.Setenv(envAIProvider, "qwen")
	t.Setenv(envAIModel, "qwen-plus")
	t.Setenv(envAIAPIKey, "test-key")
	t.Setenv(envAIBaseURL, "")

	cfg := LoadConfigFromEnv()
	if cfg.BaseURL != defaultQwenBaseURL {
		t.Fatalf("base url mismatch: got=%q want=%q", cfg.BaseURL, defaultQwenBaseURL)
	}
	if !cfg.AgentReady() {
		t.Fatalf("config should be agent ready")
	}
}

func TestLoadConfigFromEnv_RAGDefaults(t *testing.T) {
	t.Setenv(envAIRAGEnabled, "")
	t.Setenv(envAIRAGKnowledgeDir, "")
	t.Setenv(envAIRAGTopK, "")
	t.Setenv(envAIRAGMinScore, "")
	t.Setenv(envAIRAGMaxSnippet, "")

	cfg := LoadConfigFromEnv()
	if !cfg.RAGEnabled {
		t.Fatalf("rag should be enabled by default")
	}
	if cfg.RAGKnowledgeDir != defaultKnowledgeDir {
		t.Fatalf("rag knowledge dir mismatch: got=%q want=%q", cfg.RAGKnowledgeDir, defaultKnowledgeDir)
	}
	if cfg.RAGTopK != defaultKnowledgeTopK {
		t.Fatalf("rag top k mismatch: got=%d want=%d", cfg.RAGTopK, defaultKnowledgeTopK)
	}
	if cfg.RAGMinScore != defaultKnowledgeMinScore {
		t.Fatalf("rag min score mismatch: got=%f want=%f", cfg.RAGMinScore, defaultKnowledgeMinScore)
	}
	if cfg.RAGMaxSnippet != defaultKnowledgeMaxSnippetLen {
		t.Fatalf("rag max snippet mismatch: got=%d want=%d", cfg.RAGMaxSnippet, defaultKnowledgeMaxSnippetLen)
	}
}
