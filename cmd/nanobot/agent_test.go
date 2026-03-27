package main

import (
	"testing"
	"time"

	"github.com/wall/nanobot-eino/pkg/config"
)

func TestParseSessionTarget(t *testing.T) {
	channel, chatID := parseSessionTarget("cli:user-1")
	if channel != "cli" || chatID != "user-1" {
		t.Fatalf("unexpected parsed target: channel=%q chat_id=%q", channel, chatID)
	}

	channel, chatID = parseSessionTarget("standalone-session")
	if channel != "cli" || chatID != "standalone-session" {
		t.Fatalf("fallback parsed target mismatch: channel=%q chat_id=%q", channel, chatID)
	}
}

func TestBuildCLIToolConfig_UsesRuntimeConfig(t *testing.T) {
	cfg := &config.Config{
		Tools: config.ToolsConfig{
			Workspace:           "/tmp/workspace",
			RestrictToWorkspace: true,
			ExtraReadDirs:       []string{"/tmp/extra"},
			Web: config.WebConfig{
				Search: config.WebSearchConfig{
					Provider:   "",
					APIKey:     "search-key",
					BaseURL:    "https://example-search.local",
					MaxResults: 7,
				},
			},
			Exec: config.ExecConfig{
				Timeout:       config.Duration{Duration: 10 * time.Second},
				MaxOutput:     2048,
				DenyPatterns:  []string{"rm -rf"},
				AllowPatterns: []string{"echo"},
				PathAppend:    "/opt/tools",
			},
		},
	}

	toolCfg := buildCLIToolConfig(cfg, "cli:user-1")

	if !toolCfg.RestrictToWorkspace {
		t.Fatal("RestrictToWorkspace should follow config")
	}
	if toolCfg.DefaultChannel != "cli" || toolCfg.DefaultChatID != "user-1" {
		t.Fatalf("default target mismatch: channel=%q chat_id=%q", toolCfg.DefaultChannel, toolCfg.DefaultChatID)
	}
	if toolCfg.Web.Search.Provider != "tavily" {
		t.Fatalf("search provider should default to tavily, got %q", toolCfg.Web.Search.Provider)
	}
	if toolCfg.Exec.PathAppend != "/opt/tools" {
		t.Fatalf("exec path append mismatch: %q", toolCfg.Exec.PathAppend)
	}
	if toolCfg.Exec.Timeout != 10*time.Second {
		t.Fatalf("exec timeout mismatch: %s", toolCfg.Exec.Timeout)
	}
}
