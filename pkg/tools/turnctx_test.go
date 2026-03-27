package tools

import (
	"context"
	"testing"
)

func TestTurnContext_MessageSent(t *testing.T) {
	ctx, tc := NewTurnContext(context.Background())
	_ = ctx

	if tc.WasMessageSent() {
		t.Error("should be false initially")
	}

	tc.SetMessageSent()
	if !tc.WasMessageSent() {
		t.Error("should be true after SetMessageSent")
	}
}

func TestTurnContext_FromContext(t *testing.T) {
	ctx, original := NewTurnContext(context.Background())

	got := GetTurnContext(ctx)
	if got != original {
		t.Error("GetTurnContext should return the same TurnContext")
	}
}

func TestTurnContext_NilFromPlainContext(t *testing.T) {
	got := GetTurnContext(context.Background())
	if got != nil {
		t.Error("GetTurnContext should return nil for plain context")
	}
}

func TestContextWithSessionID(t *testing.T) {
	ctx := ContextWithSessionID(context.Background(), "session-123")

	got := SessionIDFromContext(ctx)
	if got != "session-123" {
		t.Errorf("SessionIDFromContext = %q, want %q", got, "session-123")
	}
}

func TestSessionIDFromContext_Empty(t *testing.T) {
	got := SessionIDFromContext(context.Background())
	if got != "" {
		t.Errorf("should return empty for plain context, got %q", got)
	}
}

func TestContextWithProgressInfo(t *testing.T) {
	ctx := ContextWithProgressInfo(context.Background(), "feishu", "group1")

	pi := GetProgressInfo(ctx)
	if pi == nil {
		t.Fatal("ProgressInfo should not be nil")
	}
	if pi.Channel != "feishu" {
		t.Errorf("Channel = %q, want %q", pi.Channel, "feishu")
	}
	if pi.ChatID != "group1" {
		t.Errorf("ChatID = %q, want %q", pi.ChatID, "group1")
	}
}

func TestGetProgressInfo_Nil(t *testing.T) {
	pi := GetProgressInfo(context.Background())
	if pi != nil {
		t.Error("should return nil for plain context")
	}
}
