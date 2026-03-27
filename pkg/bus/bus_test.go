package bus

import (
	"context"
	"testing"
	"time"
)

func TestInboundMessage_SessionKey_Default(t *testing.T) {
	msg := &InboundMessage{Channel: "feishu", ChatID: "group123"}
	if got := msg.SessionKey(); got != "feishu:group123" {
		t.Errorf("SessionKey() = %q, want %q", got, "feishu:group123")
	}
}

func TestInboundMessage_SessionKey_Override(t *testing.T) {
	msg := &InboundMessage{
		Channel:            "feishu",
		ChatID:             "group123",
		SessionKeyOverride: "custom-key",
	}
	if got := msg.SessionKey(); got != "custom-key" {
		t.Errorf("SessionKey() = %q, want %q", got, "custom-key")
	}
}

func TestExtractReplyTo_UsesMessageID(t *testing.T) {
	got := ExtractReplyTo(map[string]any{"message_id": "om_123"})
	if got != "om_123" {
		t.Fatalf("ExtractReplyTo() = %q, want %q", got, "om_123")
	}
}

func TestExtractReplyTo_EmptyWhenMissing(t *testing.T) {
	if got := ExtractReplyTo(map[string]any{"foo": "bar"}); got != "" {
		t.Fatalf("ExtractReplyTo() = %q, want empty", got)
	}
}

func TestMessageBus_InboundPubSub(t *testing.T) {
	bus := NewMessageBus()
	ctx := context.Background()

	msg := &InboundMessage{Channel: "test", Content: "hello"}
	bus.PublishInbound(ctx, msg)

	select {
	case got := <-bus.ConsumeInbound(ctx):
		if got.Content != "hello" {
			t.Errorf("Content = %q, want %q", got.Content, "hello")
		}
		if got.Channel != "test" {
			t.Errorf("Channel = %q, want %q", got.Channel, "test")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for inbound message")
	}
}

func TestMessageBus_OutboundPubSub(t *testing.T) {
	bus := NewMessageBus()
	ctx := context.Background()

	msg := &OutboundMessage{Channel: "feishu", ChatID: "abc", Content: "reply"}
	bus.PublishOutbound(ctx, msg)

	select {
	case got := <-bus.ConsumeOutbound(ctx):
		if got.Content != "reply" {
			t.Errorf("Content = %q, want %q", got.Content, "reply")
		}
		if got.ChatID != "abc" {
			t.Errorf("ChatID = %q, want %q", got.ChatID, "abc")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for outbound message")
	}
}

func TestMessageBus_MultipleMessages(t *testing.T) {
	bus := NewMessageBus()
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		bus.PublishInbound(ctx, &InboundMessage{Content: "msg"})
	}

	count := 0
	for range 5 {
		select {
		case <-bus.ConsumeInbound(ctx):
			count++
		case <-time.After(time.Second):
			t.Fatal("timed out")
		}
	}
	if count != 5 {
		t.Errorf("received %d messages, want 5", count)
	}
}

func TestMessageBus_CloseOutboundClosesChannel(t *testing.T) {
	bus := NewMessageBus()
	ctx := context.Background()

	outbound := bus.ConsumeOutbound(ctx)
	bus.CloseOutbound()

	select {
	case _, ok := <-outbound:
		if ok {
			t.Error("expected outbound channel to be closed (ok=false), got a message")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out — outbound channel was not closed by CloseOutbound()")
	}
}

func TestMessageBus_CloseOutboundSafeToCallMultipleTimes(t *testing.T) {
	bus := NewMessageBus()
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("CloseOutbound panicked on second call: %v", r)
		}
	}()
	bus.CloseOutbound()
	bus.CloseOutbound() // must not panic
}

func TestMessageBus_PublishOutboundAfterCloseOutbound_DoesNotPanic(t *testing.T) {
	bus := NewMessageBus()
	bus.CloseOutbound()

	defer func() {
		if r := recover(); r != nil {
			t.Errorf("PublishOutbound panicked after CloseOutbound(): %v", r)
		}
	}()
	bus.PublishOutbound(context.Background(), &OutboundMessage{Content: "late"})
}

func TestMessageBus_BufferedChannels(t *testing.T) {
	bus := NewMessageBus()
	ctx := context.Background()

	for i := 0; i < 100; i++ {
		bus.PublishInbound(ctx, &InboundMessage{Content: "buffered"})
	}

	for range 100 {
		select {
		case <-bus.ConsumeInbound(ctx):
		case <-time.After(time.Second):
			t.Fatal("timed out reading buffered messages")
		}
	}
}
