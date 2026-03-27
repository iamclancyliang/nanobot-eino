package main

import (
	"testing"

	"github.com/wall/nanobot-eino/pkg/bus"
)

func TestExtractReplyTo_UsesMessageID(t *testing.T) {
	got := bus.ExtractReplyTo(map[string]any{"message_id": "om_123"})
	if got != "om_123" {
		t.Fatalf("extractReplyTo = %q, want %q", got, "om_123")
	}
}

func TestExtractReplyTo_EmptyWhenMissing(t *testing.T) {
	got := bus.ExtractReplyTo(map[string]any{"foo": "bar"})
	if got != "" {
		t.Fatalf("extractReplyTo = %q, want empty", got)
	}
}
