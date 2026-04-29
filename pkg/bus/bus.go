package bus

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"
)

var logBus = slog.With("module", "bus")

// InboundMessage is a message received from a channel and dispatched into
// the agent for processing.
type InboundMessage struct {
	Channel            string
	SenderID           string
	ChatID             string
	Content            string
	Timestamp          time.Time
	Media              []string
	Metadata           map[string]any
	SessionKeyOverride string
}

// SessionKey returns the session identifier for this message. It uses
// SessionKeyOverride when set, otherwise "<channel>:<chat_id>".
func (m *InboundMessage) SessionKey() string {
	if m.SessionKeyOverride != "" {
		return m.SessionKeyOverride
	}
	return m.Channel + ":" + m.ChatID
}

// ExtractReplyTo returns message_id from metadata when available.
func ExtractReplyTo(metadata map[string]any) string {
	if metadata == nil {
		return ""
	}
	if id, ok := metadata["message_id"].(string); ok {
		return strings.TrimSpace(id)
	}
	return ""
}

// OutboundMessage is a message produced by the agent to be delivered through
// a channel.
type OutboundMessage struct {
	Channel  string
	ChatID   string
	Content  string
	ReplyTo  string
	Media    []string
	Metadata map[string]any
}

// MessageBus is a buffered in-memory bus that decouples channel adapters from
// the agent. Channels publish inbound messages and consume outbound messages.
type MessageBus struct {
	inbound       chan *InboundMessage
	outbound      chan *OutboundMessage
	inboundOnce   sync.Once
	outboundOnce  sync.Once
}

// NewMessageBus creates a MessageBus with bounded inbound and outbound
// buffers.
func NewMessageBus() *MessageBus {
	return &MessageBus{
		inbound:  make(chan *InboundMessage, 100),
		outbound: make(chan *OutboundMessage, 100),
	}
}

// Close closes the inbound channel, causing ConsumeInbound range loops to exit.
// Safe to call multiple times. In-flight workers may still publish to outbound
// after this call; call CloseOutbound() only after all workers have finished.
func (b *MessageBus) Close() {
	b.inboundOnce.Do(func() {
		close(b.inbound)
	})
}

// CloseOutbound closes the outbound channel, causing ConsumeOutbound range
// loops to exit cleanly. Call this AFTER all workers that may publish to
// outbound have finished (e.g. after wg.Wait()). Safe to call multiple times.
func (b *MessageBus) CloseOutbound() {
	b.outboundOnce.Do(func() {
		close(b.outbound)
	})
}

// PublishInbound enqueues msg on the inbound channel. It returns when the
// message is accepted or ctx is cancelled, and never blocks the caller after
// the bus has been closed.
func (b *MessageBus) PublishInbound(ctx context.Context, msg *InboundMessage) {
	defer func() {
		if r := recover(); r != nil {
			logBus.Error("MessageBus publish dropped", "direction", "inbound", "panic", r)
		}
	}()
	select {
	case b.inbound <- msg:
	case <-ctx.Done():
	}
}

// ConsumeInbound returns the receive end of the inbound channel for the
// agent loop.
func (b *MessageBus) ConsumeInbound(ctx context.Context) <-chan *InboundMessage {
	return b.inbound
}

// PublishOutbound enqueues msg on the outbound channel. It returns when the
// message is accepted or ctx is cancelled.
func (b *MessageBus) PublishOutbound(ctx context.Context, msg *OutboundMessage) {
	defer func() {
		if r := recover(); r != nil {
			logBus.Error("MessageBus publish dropped", "direction", "outbound", "panic", r)
		}
	}()
	select {
	case b.outbound <- msg:
	case <-ctx.Done():
	}
}

// ConsumeOutbound returns the receive end of the outbound channel for
// channel adapters.
func (b *MessageBus) ConsumeOutbound(ctx context.Context) <-chan *OutboundMessage {
	return b.outbound
}
