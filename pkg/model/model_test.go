package model

import (
	"context"
	"strings"
	"testing"
)

func TestNewChatModel_UnsupportedType(t *testing.T) {
	t.Parallel()

	_, err := NewChatModel(context.Background(), Config{
		Type:  "unknown",
		Model: "x",
	})
	if err == nil {
		t.Fatal("expected error for unsupported model type")
	}
	if !strings.Contains(err.Error(), "unsupported model type") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNewChatModel_GoogleType(t *testing.T) {
	t.Parallel()

	_, err := NewChatModel(context.Background(), Config{
		Type:   "google",
		APIKey: "test-api-key",
		Model:  "gemini-2.5-flash",
	})
	if err != nil {
		t.Fatalf("google model config should be accepted, got error: %v", err)
	}
}

func TestNewChatModel_SiliconFlowType(t *testing.T) {
	t.Parallel()

	_, err := NewChatModel(context.Background(), Config{
		Type:    "siliconflow",
		BaseURL: "https://api.siliconflow.cn/v1",
		APIKey:  "test-api-key",
		Model:   "Qwen/Qwen3-8B",
	})
	if err != nil {
		t.Fatalf("siliconflow model config should be accepted, got error: %v", err)
	}
}

func TestNewChatModel_QianfanRequiresKeyPair(t *testing.T) {
	t.Parallel()

	_, err := NewChatModel(context.Background(), Config{
		Type:  "qianfan",
		Model: "ernie-4.0-8k",
	})
	if err == nil {
		t.Fatal("expected error when qianfan credentials are missing")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "qianfan") {
		t.Fatalf("unexpected error: %v", err)
	}
}
