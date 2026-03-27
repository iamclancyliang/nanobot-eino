package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// captured records one call to the SendMessageFunc callback.
type captured struct {
	channel string
	chatID  string
	content string
	media   []string
}

// makeSink returns a SendMessageFunc and a pointer to the last captured call.
func makeSink() (SendMessageFunc, *[]*captured) {
	var calls []*captured
	fn := func(_ context.Context, p SendMessagePayload) {
		calls = append(calls, &captured{
			channel: p.Channel,
			chatID:  p.ChatID,
			content: p.Content,
			media:   p.Media,
		})
	}
	return fn, &calls
}

func TestMessageTool_SendsMessage(t *testing.T) {
	fn, calls := makeSink()
	msgTool := NewMessageTool(fn, "feishu", "group1")

	args, _ := json.Marshal(MessageArgs{Content: "hello user"})
	result, err := msgTool.InvokableRun(context.Background(), string(args))
	if err != nil {
		t.Fatalf("InvokableRun error: %v", err)
	}
	if !strings.Contains(result, "Message sent") {
		t.Errorf("expected success message, got: %s", result)
	}
	if len(*calls) != 1 {
		t.Fatalf("expected 1 call to sink, got %d", len(*calls))
	}
	c := (*calls)[0]
	if c.content != "hello user" {
		t.Errorf("Content = %q, want %q", c.content, "hello user")
	}
	if c.channel != "feishu" {
		t.Errorf("Channel = %q, want %q", c.channel, "feishu")
	}
	if c.chatID != "group1" {
		t.Errorf("ChatID = %q, want %q", c.chatID, "group1")
	}
}

func TestMessageTool_CustomChannelAndChatID(t *testing.T) {
	fn, calls := makeSink()
	msgTool := NewMessageTool(fn, "feishu", "default-chat")

	args, _ := json.Marshal(MessageArgs{
		Content: "custom target",
		Channel: "telegram",
		ChatID:  "custom-chat",
	})
	result, _ := msgTool.InvokableRun(context.Background(), string(args))

	if !strings.Contains(result, "telegram:custom-chat") {
		t.Errorf("expected custom target in result, got: %s", result)
	}
	if len(*calls) != 1 || (*calls)[0].channel != "telegram" {
		t.Errorf("expected telegram channel, got calls: %+v", *calls)
	}
}

func TestMessageTool_NoChannel(t *testing.T) {
	fn, calls := makeSink()
	msgTool := NewMessageTool(fn, "", "")

	args, _ := json.Marshal(MessageArgs{Content: "hello"})
	result, _ := msgTool.InvokableRun(context.Background(), string(args))

	if !strings.Contains(result, "Error") {
		t.Errorf("expected error when no channel, got: %s", result)
	}
	if len(*calls) != 0 {
		t.Errorf("expected no call to sink when no channel, got %d", len(*calls))
	}
}

func TestMessageTool_WithMedia(t *testing.T) {
	fn, calls := makeSink()
	msgTool := NewMessageTool(fn, "feishu", "chat1")

	args, _ := json.Marshal(MessageArgs{
		Content: "here's a photo",
		Media:   []string{"/path/to/image.jpg", "/path/to/file.pdf"},
	})
	result, _ := msgTool.InvokableRun(context.Background(), string(args))

	if !strings.Contains(result, "2 attachments") {
		t.Errorf("expected attachment count in result, got: %s", result)
	}
	if len(*calls) != 1 || len((*calls)[0].media) != 2 {
		t.Errorf("expected 2 media items, got calls: %+v", *calls)
	}
}

func TestMessageTool_SetsTurnContextFlag(t *testing.T) {
	fn, _ := makeSink()
	msgTool := NewMessageTool(fn, "feishu", "chat1")

	ctx, tc := NewTurnContext(context.Background())

	if tc.WasMessageSent() {
		t.Error("should be false before message")
	}

	args, _ := json.Marshal(MessageArgs{Content: "hello"})
	msgTool.InvokableRun(ctx, string(args))

	if !tc.WasMessageSent() {
		t.Error("should be true after message tool runs")
	}
}

func TestMessageTool_Info(t *testing.T) {
	fn, _ := makeSink()
	msgTool := NewMessageTool(fn, "feishu", "chat1")

	info, err := msgTool.Info(context.Background())
	if err != nil {
		t.Fatalf("Info error: %v", err)
	}
	if info.Name != "message" {
		t.Errorf("tool name = %q, want %q", info.Name, "message")
	}
}
