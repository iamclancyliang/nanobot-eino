//go:build integration

package model

import (
	"context"
	"os"
	"testing"

	"github.com/cloudwego/eino/schema"
)

func TestIntegration_NewChatModel_Generate(t *testing.T) {
	apiKey := os.Getenv("NANOBOT_MODEL_API_KEY")
	if apiKey == "" {
		t.Skip("NANOBOT_MODEL_API_KEY not set")
	}

	ctx := context.Background()

	cfg := GetDefaultConfig()
	chatModel, err := NewChatModel(ctx, cfg)
	if err != nil {
		t.Fatalf("failed to create chat model: %v", err)
	}

	messages := []*schema.Message{
		schema.UserMessage("Hello, who are you? Reply in one sentence."),
	}

	resp, err := chatModel.Generate(ctx, messages)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	if resp.Content == "" {
		t.Error("expected non-empty response content")
	}
	t.Logf("Response: %s", resp.Content)
}
