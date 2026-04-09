package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOpenAICompatibleClient_CreateChatCompletion(t *testing.T) {
	var capturedAuth string
	var capturedPath string
	var capturedModel string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		capturedPath = r.URL.Path

		var req ChatCompletionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err == nil {
			capturedModel = req.Model
		}

		_ = json.NewEncoder(w).Encode(ChatCompletionResponse{
			Choices: []ChatCompletionChoice{
				{
					Index: 0,
					Message: ChatMessage{
						Role:    "assistant",
						Content: "ok",
					},
				},
			},
		})
	}))
	defer server.Close()

	client := NewOpenAICompatibleClient(server.URL, "test-key", 0)
	resp, err := client.CreateChatCompletion(context.Background(), ChatCompletionRequest{
		Model: "qwen-plus",
		Messages: []ChatMessage{
			{Role: "user", Content: "hello"},
		},
	})
	if err != nil {
		t.Fatalf("CreateChatCompletion failed: %v", err)
	}
	if capturedAuth != "Bearer test-key" {
		t.Fatalf("auth header mismatch: %q", capturedAuth)
	}
	if capturedPath != "/chat/completions" {
		t.Fatalf("path mismatch: %q", capturedPath)
	}
	if capturedModel != "qwen-plus" {
		t.Fatalf("model mismatch: %q", capturedModel)
	}
	if len(resp.Choices) != 1 || resp.Choices[0].Message.Content != "ok" {
		t.Fatalf("response mismatch: %+v", resp)
	}
}

func TestOpenAICompatibleClient_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(ChatCompletionResponse{
			Error: &ChatCompletionError{
				Message: "invalid api key",
			},
		})
	}))
	defer server.Close()

	client := NewOpenAICompatibleClient(server.URL, "bad-key", 0)
	_, err := client.CreateChatCompletion(context.Background(), ChatCompletionRequest{
		Model:    "qwen-plus",
		Messages: []ChatMessage{{Role: "user", Content: "hello"}},
	})
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
}
