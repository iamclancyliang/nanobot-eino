package app

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cloudwego/eino/schema"
	"github.com/wall/nanobot-eino/pkg/apperr"
	"github.com/wall/nanobot-eino/pkg/bus"
)

type fakeChatStreamer struct {
	resp string
	err  error
}

func (f *fakeChatStreamer) ChatStream(ctx context.Context, sessionID, input string) (*schema.StreamReader[*schema.Message], error) {
	if f.err != nil {
		return nil, f.err
	}
	sr, sw := schema.Pipe[*schema.Message](1)
	go func() {
		sw.Send(&schema.Message{Role: schema.Assistant, Content: f.resp}, nil)
		sw.Close()
	}()
	return sr, nil
}

// funcChatStreamer lets tests inject a custom ChatStream function.
type funcChatStreamer struct {
	fn func(ctx context.Context, sessionID, input string) (*schema.StreamReader[*schema.Message], error)
}

func (f *funcChatStreamer) ChatStream(ctx context.Context, sessionID, input string) (*schema.StreamReader[*schema.Message], error) {
	return f.fn(ctx, sessionID, input)
}

// TestRunInboundLoop_SameSessionIsSerial verifies that two messages for the same
// session are NOT processed concurrently — they must be serialized.
// RED: fails with the old goroutine-per-message approach (fake has no session lock).
// GREEN: passes after per-session worker is added to RunInboundLoop.
func TestRunInboundLoop_SameSessionIsSerial(t *testing.T) {
	ctx := context.Background()
	messageBus := bus.NewMessageBus()

	var mu sync.Mutex
	concurrentCalls := 0
	maxConcurrent := 0
	started := make(chan struct{}, 2)
	proceed := make(chan struct{})

	bot := &funcChatStreamer{fn: func(ctx context.Context, sessionID, input string) (*schema.StreamReader[*schema.Message], error) {
		mu.Lock()
		concurrentCalls++
		if concurrentCalls > maxConcurrent {
			maxConcurrent = concurrentCalls
		}
		mu.Unlock()

		started <- struct{}{}
		<-proceed

		mu.Lock()
		concurrentCalls--
		mu.Unlock()

		sr, sw := schema.Pipe[*schema.Message](1)
		go func() {
			sw.Send(&schema.Message{Role: schema.Assistant, Content: "ok"}, nil)
			sw.Close()
		}()
		return sr, nil
	}}

	var wg sync.WaitGroup
	done := make(chan struct{})
	go func() {
		RunInboundLoop(ctx, messageBus, bot, &wg)
		close(done)
	}()

	// Send 2 messages for the same session.
	messageBus.PublishInbound(ctx, &bus.InboundMessage{Channel: "feishu", ChatID: "oc_123", Content: "msg1"})
	messageBus.PublishInbound(ctx, &bus.InboundMessage{Channel: "feishu", ChatID: "oc_123", Content: "msg2"})

	// Wait for first to start.
	select {
	case <-started:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("first message never started")
	}

	// Second must NOT have started yet.
	select {
	case <-started:
		t.Fatal("second message started before first completed — same session must be serial")
	case <-time.After(50 * time.Millisecond):
		// Good: second is queued.
	}

	// Release first.
	proceed <- struct{}{}

	// Now second should start.
	select {
	case <-started:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("second message never started after first completed")
	}

	// Release second.
	proceed <- struct{}{}

	messageBus.Close()
	wg.Wait()
	<-done

	if maxConcurrent != 1 {
		t.Errorf("maxConcurrent = %d, want 1 (same session must be serial)", maxConcurrent)
	}
}

// TestRunInboundLoop_DifferentSessionsAreParallel verifies that messages for
// different sessions CAN be processed concurrently.
func TestRunInboundLoop_DifferentSessionsAreParallel(t *testing.T) {
	ctx := context.Background()
	messageBus := bus.NewMessageBus()

	var mu sync.Mutex
	maxConcurrent := 0
	concurrentCalls := 0
	started := make(chan struct{}, 2)
	proceed := make(chan struct{})

	bot := &funcChatStreamer{fn: func(ctx context.Context, sessionID, input string) (*schema.StreamReader[*schema.Message], error) {
		mu.Lock()
		concurrentCalls++
		if concurrentCalls > maxConcurrent {
			maxConcurrent = concurrentCalls
		}
		mu.Unlock()

		started <- struct{}{}
		<-proceed

		mu.Lock()
		concurrentCalls--
		mu.Unlock()

		sr, sw := schema.Pipe[*schema.Message](1)
		go func() {
			sw.Send(&schema.Message{Role: schema.Assistant, Content: "ok"}, nil)
			sw.Close()
		}()
		return sr, nil
	}}

	var wg sync.WaitGroup
	done := make(chan struct{})
	go func() {
		RunInboundLoop(ctx, messageBus, bot, &wg)
		close(done)
	}()

	// Send messages for TWO DIFFERENT sessions.
	messageBus.PublishInbound(ctx, &bus.InboundMessage{Channel: "feishu", ChatID: "oc_aaa", Content: "hello"})
	messageBus.PublishInbound(ctx, &bus.InboundMessage{Channel: "feishu", ChatID: "oc_bbb", Content: "world"})

	// Both should start concurrently.
	for i := 0; i < 2; i++ {
		select {
		case <-started:
		case <-time.After(500 * time.Millisecond):
			t.Fatalf("message %d never started", i+1)
		}
	}

	// Release both.
	proceed <- struct{}{}
	proceed <- struct{}{}

	messageBus.Close()
	wg.Wait()
	<-done

	if maxConcurrent < 2 {
		t.Errorf("maxConcurrent = %d, want >= 2 (different sessions should run in parallel)", maxConcurrent)
	}
}

type fakeBrokenStreamChatStreamer struct{}

func (f *fakeBrokenStreamChatStreamer) ChatStream(ctx context.Context, sessionID, input string) (*schema.StreamReader[*schema.Message], error) {
	sr, sw := schema.Pipe[*schema.Message](1)
	go func() {
		sw.Send(nil, errors.New("stream boom"))
		sw.Close()
	}()
	return sr, nil
}

func TestRunInboundLoop_PublishesOutboundWithReplyAndRoute(t *testing.T) {
	ctx := context.Background()
	messageBus := bus.NewMessageBus()
	bot := &fakeChatStreamer{resp: "ok"}
	var wg sync.WaitGroup

	done := make(chan struct{})
	go func() {
		RunInboundLoop(ctx, messageBus, bot, &wg)
		close(done)
	}()

	metadata := map[string]any{"message_id": "msg_1"}
	messageBus.PublishInbound(ctx, &bus.InboundMessage{
		Channel:  "system",
		ChatID:   "feishu:oc_123",
		Content:  "hello",
		Metadata: metadata,
	})
	messageBus.Close()

	wg.Wait()
	<-done

	select {
	case out := <-messageBus.ConsumeOutbound(ctx):
		if out.Channel != "feishu" || out.ChatID != "oc_123" {
			t.Fatalf("unexpected route: %+v", out)
		}
		if out.Content != "ok" || out.ReplyTo != "msg_1" {
			t.Fatalf("unexpected content/reply: %+v", out)
		}
	case <-time.After(300 * time.Millisecond):
		t.Fatal("expected outbound message")
	}
}

func TestRunInboundLoop_SkipsWhenChatStreamError(t *testing.T) {
	ctx := context.Background()
	messageBus := bus.NewMessageBus()
	bot := &fakeChatStreamer{err: errors.New("boom")}
	var wg sync.WaitGroup

	done := make(chan struct{})
	go func() {
		RunInboundLoop(ctx, messageBus, bot, &wg)
		close(done)
	}()

	messageBus.PublishInbound(ctx, &bus.InboundMessage{
		Channel: "feishu",
		ChatID:  "oc_123",
		Content: "hello",
	})
	messageBus.Close()

	wg.Wait()
	<-done

	select {
	case out := <-messageBus.ConsumeOutbound(ctx):
		if out.Channel != "feishu" || out.ChatID != "oc_123" {
			t.Fatalf("unexpected route: %+v", out)
		}
		if out.Content != defaultAgentErrorReply {
			t.Fatalf("unexpected fallback error content: %+v", out)
		}
	case <-time.After(300 * time.Millisecond):
		t.Fatal("expected fallback error outbound message")
	}
}

func TestRunInboundLoop_FallbackWhenStreamRecvErrorWithoutContent(t *testing.T) {
	ctx := context.Background()
	messageBus := bus.NewMessageBus()
	bot := &fakeBrokenStreamChatStreamer{}
	var wg sync.WaitGroup

	done := make(chan struct{})
	go func() {
		RunInboundLoop(ctx, messageBus, bot, &wg)
		close(done)
	}()

	messageBus.PublishInbound(ctx, &bus.InboundMessage{
		Channel: "feishu",
		ChatID:  "oc_123",
		Content: "hello",
	})
	messageBus.Close()

	wg.Wait()
	<-done

	select {
	case out := <-messageBus.ConsumeOutbound(ctx):
		if out.Channel != "feishu" || out.ChatID != "oc_123" {
			t.Fatalf("unexpected route: %+v", out)
		}
		if out.Content != defaultAgentErrorReply {
			t.Fatalf("unexpected fallback error content: %+v", out)
		}
	case <-time.After(300 * time.Millisecond):
		t.Fatal("expected fallback error outbound message")
	}
}

func TestDecodeSystemRoute_WithPrefixChannel(t *testing.T) {
	channel, chatID := DecodeSystemRoute("feishu:oc_123")
	if channel != "feishu" || chatID != "oc_123" {
		t.Fatalf("unexpected decode result: channel=%q chatID=%q", channel, chatID)
	}
}

func TestDecodeSystemRoute_DefaultCLI(t *testing.T) {
	channel, chatID := DecodeSystemRoute("terminal-1")
	if channel != "cli" || chatID != "terminal-1" {
		t.Fatalf("unexpected decode result: channel=%q chatID=%q", channel, chatID)
	}
}

func TestRunloopPublicErrorMessage(t *testing.T) {
	cases := []struct {
		name    string
		err     error
		want    string
		contain string
	}{
		{
			name: "nil falls back to default",
			err:  nil,
			want: defaultAgentErrorReply,
		},
		{
			name:    "503 service busy",
			err:     errors.New(`HTTP 503: {"error":{"message":"Service is too busy."}}`),
			contain: "繁忙",
		},
		{
			name:    "429 rate limit",
			err:     errors.New(`HTTP 429: rate limit exceeded`),
			contain: "频率受限",
		},
		{
			name:    "401 auth",
			err:     errors.New(`HTTP 401 Unauthorized: invalid api key`),
			contain: "鉴权失败",
		},
		{
			name:    "404 model not found",
			err:     errors.New(`HTTP 404: model_not_found`),
			contain: "模型不存在",
		},
		{
			name:    "400 surfaces detail",
			err:     errors.New(`HTTP 400: {"error":{"message":"Tool names must be unique."}}`),
			contain: "Tool names must be unique",
		},
		{
			name:    "context canceled",
			err:     errors.New("context canceled"),
			contain: "已取消",
		},
		{
			name:    "i/o timeout",
			err:     errors.New("dial tcp: i/o timeout"),
			contain: "超时",
		},
		{
			name:    "unknown error with embedded api message",
			err:     errors.New(`weird error: {"error":{"message":"something specific"}}`),
			contain: "something specific",
		},
		{
			name: "fully unknown falls back",
			err:  errors.New("boom"),
			want: defaultAgentErrorReply,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := apperr.PublicMessage(tc.err)
			if tc.want != "" && got != tc.want {
				t.Fatalf("want %q, got %q", tc.want, got)
			}
			if tc.contain != "" && !strings.Contains(got, tc.contain) {
				t.Fatalf("expected reply to contain %q, got %q", tc.contain, got)
			}
		})
	}
}
