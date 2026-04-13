package ai

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	einoopenai "github.com/cloudwego/eino-ext/components/model/openai"
	einomodel "github.com/cloudwego/eino/components/model"
)

const (
	defaultOpenAICompatibleTimeout = 20 * time.Second
)

// NewEinoChatModelFromConfig creates an Eino ChatModel using the existing AI config/env semantics.
func NewEinoChatModelFromConfig(ctx context.Context, cfg Config) (einomodel.ToolCallingChatModel, error) {
	apiKey := strings.TrimSpace(cfg.APIKey)
	if apiKey == "" {
		return nil, fmt.Errorf("ai api key is empty")
	}
	modelName := strings.TrimSpace(cfg.Model)
	if modelName == "" {
		return nil, fmt.Errorf("ai model is empty")
	}

	timeout := cfg.RequestTimeout
	if timeout <= 0 {
		timeout = defaultOpenAICompatibleTimeout
	}

	chatModel, err := einoopenai.NewChatModel(ctx, &einoopenai.ChatModelConfig{
		APIKey:  apiKey,
		BaseURL: strings.TrimSpace(cfg.BaseURL),
		Model:   modelName,
		Timeout: timeout,
		HTTPClient: &http.Client{
			Timeout: timeout,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("init eino chat model failed: %w", err)
	}
	return chatModel, nil
}
