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
