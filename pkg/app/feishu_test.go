package app

import (
	"context"
	"testing"

	"github.com/wall/nanobot-eino/pkg/bus"
	"github.com/wall/nanobot-eino/pkg/channels"
	"github.com/wall/nanobot-eino/pkg/config"
)

func TestBuildFeishuConfig_MapsFields(t *testing.T) {
	cfg := &config.Config{}
	cfg.Channels.Feishu.AppID = "cli_a"
	cfg.Channels.Feishu.AppSecret = "sec"
	cfg.Channels.Feishu.VerificationToken = "vt"
	cfg.Channels.Feishu.EncryptKey = "ek"
	cfg.Channels.Feishu.AllowFrom = []string{"u1", "u2"}
	cfg.Channels.Feishu.GroupPolicy = "open"

	got := BuildFeishuConfig(cfg)
	if got.AppID != "cli_a" ||
		got.AppSecret != "sec" ||
		got.VerificationToken != "vt" ||
		got.EncryptKey != "ek" ||
		got.GroupPolicy != "open" {
		t.Fatalf("feishu config mapping mismatch: %+v", got)
	}
	if len(got.AllowFrom) != 2 || got.AllowFrom[0] != "u1" || got.AllowFrom[1] != "u2" {
		t.Fatalf("allowFrom mapping mismatch: %+v", got.AllowFrom)
	}
}

func TestStartFeishuChannel_NotConfiguredCallsCallback(t *testing.T) {
	messageBus := bus.NewMessageBus()
	called := false

	ch := StartFeishuChannel(
		context.Background(),
		channels.FeishuConfig{},
		messageBus,
		func() { called = true },
	)

	if ch != nil {
		t.Fatalf("expected nil channel when app id is empty, got %+v", ch)
	}
	if !called {
		t.Fatal("expected onNotConfigured callback to be called")
	}
}

func TestStartFeishuChannel_NotConfiguredNoCallback(t *testing.T) {
	messageBus := bus.NewMessageBus()
	ch := StartFeishuChannel(
		context.Background(),
		channels.FeishuConfig{},
		messageBus,
		nil,
	)
	if ch != nil {
		t.Fatalf("expected nil channel when app id is empty, got %+v", ch)
	}
}

func TestStartFeishuChannel_EmptyAllowFromReturnsNil(t *testing.T) {
	messageBus := bus.NewMessageBus()
	ch := StartFeishuChannel(
		context.Background(),
		channels.FeishuConfig{AppID: "cli_test", AppSecret: "sec"},
		messageBus,
		nil,
	)
	if ch != nil {
		t.Fatal("expected nil channel when allowFrom is empty")
	}
}
