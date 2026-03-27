package app

import (
	"context"
	"testing"
	"time"

	"github.com/wall/nanobot-eino/pkg/bus"
	"github.com/wall/nanobot-eino/pkg/config"
	"github.com/wall/nanobot-eino/pkg/tools"
)

func TestBuildToolConfig_MapsExecAndWebAndMCP(t *testing.T) {
	cfg := &config.Config{}
	cfg.Tools.Workspace = "data"
	cfg.Tools.RestrictToWorkspace = true
	cfg.Tools.ExtraReadDirs = []string{"configs/skills"}
	cfg.Tools.Web.Proxy = "http://127.0.0.1:7890"
	cfg.Tools.Web.Search.Provider = "brave"
	cfg.Tools.Web.Search.APIKey = "k"
	cfg.Tools.Web.Search.BaseURL = "https://searx.example"
	cfg.Tools.Web.Search.MaxResults = 7
	cfg.Tools.Exec.Timeout.Duration = 30 * time.Second
	cfg.Tools.Exec.MaxOutput = 2048
	cfg.Tools.Exec.DenyPatterns = []string{"rm -rf"}
	cfg.Tools.Exec.AllowPatterns = []string{"ls"}
	cfg.Tools.Exec.PathAppend = "/usr/local/bin"
	cfg.Tools.MCP = []config.MCPConfig{
		{
			Name:         "m1",
			Type:         "stdio",
			Command:      "npx",
			Args:         []string{"-y", "abc"},
			Env:          map[string]string{"A": "1"},
			URL:          "https://example.com/mcp",
			Headers:      map[string]string{"Authorization": "Bearer x"},
			EnabledTools: []string{"*"},
		},
	}

	messageBus := bus.NewMessageBus()
	got := BuildToolConfig(cfg, messageBus, "feishu")

	if got.Workspace != "data" || !got.RestrictToWorkspace {
		t.Fatalf("workspace mapping mismatch: %+v", got)
	}
	if got.Web.Proxy != "http://127.0.0.1:7890" || got.Web.Search.Provider != "brave" || got.Web.Search.MaxResults != 7 {
		t.Fatalf("web mapping mismatch: %+v", got.Web)
	}
	if got.Exec.Timeout != 30*time.Second || got.Exec.MaxOutput != 2048 || got.Exec.PathAppend != "/usr/local/bin" {
		t.Fatalf("exec mapping mismatch: %+v", got.Exec)
	}
	if len(got.MCP) != 1 || got.MCP[0].Name != "m1" || got.MCP[0].Command != "npx" {
		t.Fatalf("mcp mapping mismatch: %+v", got.MCP)
	}
	if got.DefaultChannel != "feishu" || got.OnMessage == nil {
		t.Fatalf("default channel or OnMessage callback mismatch: %+v", got)
	}
}

func TestNewProgressHandler_PublishesWhenProgressInfoExists(t *testing.T) {
	messageBus := bus.NewMessageBus()
	handler := NewProgressHandler(messageBus)

	ctx := tools.ContextWithProgressInfo(context.Background(), "feishu", "oc_123")
	handler(ctx, "web_fetch", "running")

	select {
	case msg := <-messageBus.ConsumeOutbound(context.Background()):
		if msg.Channel != "feishu" || msg.ChatID != "oc_123" {
			t.Fatalf("unexpected route: %+v", msg)
		}
		if msg.Content != "🔧 web_fetch: running" {
			t.Fatalf("unexpected content: %q", msg.Content)
		}
		if v, ok := msg.Metadata["_progress"].(bool); !ok || !v {
			t.Fatalf("missing _progress metadata: %+v", msg.Metadata)
		}
	case <-time.After(300 * time.Millisecond):
		t.Fatal("expected outbound progress message")
	}
}

func TestNewProgressHandler_SkipsWithoutProgressInfo(t *testing.T) {
	messageBus := bus.NewMessageBus()
	handler := NewProgressHandler(messageBus)
	handler(context.Background(), "shell", "completed")

	select {
	case msg := <-messageBus.ConsumeOutbound(context.Background()):
		t.Fatalf("unexpected outbound message: %+v", msg)
	default:
	}
}
