package main

import (
	"testing"

	"github.com/wall/nanobot-eino/pkg/bus"
)

func TestGatewayExtractReplyTo_UsesMessageID(t *testing.T) {
	got := bus.ExtractReplyTo(map[string]any{"message_id": "om_abc"})
	if got != "om_abc" {
		t.Fatalf("extractReplyTo = %q, want %q", got, "om_abc")
	}
}

func TestGatewayExtractReplyTo_EmptyWhenMissing(t *testing.T) {
	got := bus.ExtractReplyTo(nil)
	if got != "" {
		t.Fatalf("extractReplyTo = %q, want empty", got)
	}
}
