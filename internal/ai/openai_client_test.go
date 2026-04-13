package ai

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestNewEinoChatModelFromConfig_Success(t *testing.T) {
	cfg := Config{
		Provider:       "openai",
		Model:          "gpt-4o-mini",
		APIKey:         "test-key",
		BaseURL:        "https://api.openai.com/v1",
		RequestTimeout: 5 * time.Second,
	}

	chatModel, err := NewEinoChatModelFromConfig(context.Background(), cfg)
	if err != nil {
		t.Fatalf("NewEinoChatModelFromConfig failed: %v", err)
	}
	if chatModel == nil {
		t.Fatalf("chat model should not be nil")
	}
}

func TestNewEinoChatModelFromConfig_MissingAPIKey(t *testing.T) {
	cfg := Config{
		Model:   "gpt-4o-mini",
		BaseURL: "https://api.openai.com/v1",
	}
	_, err := NewEinoChatModelFromConfig(context.Background(), cfg)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "api key") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNewEinoChatModelFromConfig_MissingModel(t *testing.T) {
	cfg := Config{
		APIKey:  "test-key",
		BaseURL: "https://api.openai.com/v1",
	}
	_, err := NewEinoChatModelFromConfig(context.Background(), cfg)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "model") {
		t.Fatalf("unexpected error: %v", err)
	}
}
