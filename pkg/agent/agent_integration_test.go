//go:build integration

package agent

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wall/nanobot-eino/pkg/memory"
	"github.com/wall/nanobot-eino/pkg/model"
	"github.com/wall/nanobot-eino/pkg/session"
	"github.com/wall/nanobot-eino/pkg/tools"
	"github.com/wall/nanobot-eino/pkg/workspace"
)

func TestIntegration_AgentInitAndChat(t *testing.T) {
	apiKey := os.Getenv("NANOBOT_MODEL_API_KEY")
	if apiKey == "" {
		t.Skip("NANOBOT_MODEL_API_KEY not set")
	}

	ctx := context.Background()

	memStore, err := memory.NewMemoryStore(filepath.Join(t.TempDir(), "memory"))
	if err != nil {
		t.Fatalf("failed to init memory store: %v", err)
	}

	sessionMgr, err := session.NewSessionManager(filepath.Join(t.TempDir(), "sessions"))
	if err != nil {
		t.Fatalf("failed to init session manager: %v", err)
	}

	promptDir := t.TempDir()
	if err := workspace.SyncTemplates(promptDir); err != nil {
		t.Fatalf("failed to sync templates: %v", err)
	}

	modelCfg := model.GetDefaultConfig()
	toolCfg := tools.ToolConfig{}

	bot, err := NewAgent(ctx, modelCfg, toolCfg, memStore, promptDir,
		"", nil, sessionMgr, 65536, 5, nil)
	if err != nil {
		t.Fatalf("failed to create agent: %v", err)
	}

	reader, err := bot.ChatStream(ctx, "test-session", "Hello, who are you? Reply in one sentence.")
	if err != nil {
		t.Fatalf("ChatStream failed: %v", err)
	}
	defer reader.Close()

	var buf strings.Builder
	for {
		msg, recvErr := reader.Recv()
		if recvErr != nil {
			if recvErr != io.EOF {
				t.Fatalf("Recv error: %v", recvErr)
			}
			break
		}
		buf.WriteString(msg.Content)
	}

	if buf.Len() == 0 {
		t.Error("expected non-empty response")
	}
	t.Logf("Response: %s", buf.String())
}

func TestIntegration_AgentWithSkills(t *testing.T) {
	apiKey := os.Getenv("NANOBOT_MODEL_API_KEY")
	if apiKey == "" {
		t.Skip("NANOBOT_MODEL_API_KEY not set")
	}

	ctx := context.Background()

	memStore, err := memory.NewMemoryStore(filepath.Join(t.TempDir(), "memory"))
	if err != nil {
		t.Fatalf("failed to init memory store: %v", err)
	}

	sessionMgr, err := session.NewSessionManager(filepath.Join(t.TempDir(), "sessions"))
	if err != nil {
		t.Fatalf("failed to init session manager: %v", err)
	}

	modelCfg := model.GetDefaultConfig()
	// Fallback to ollama if no API key for openai
	if modelCfg.Type == "openai" && apiKey == "" {
		modelCfg.Type = "ollama"
		modelCfg.BaseURL = "http://localhost:11434"
		modelCfg.Model = "qwen3:8b"
	}

	promptDir := t.TempDir()
	if err := workspace.SyncTemplates(promptDir); err != nil {
		t.Fatalf("failed to sync templates: %v", err)
	}

	toolCfg := tools.ToolConfig{}
	bot, err := NewAgent(ctx, modelCfg, toolCfg, memStore, promptDir,
		"", nil, sessionMgr, 65536, 5, nil)
	if err != nil {
		t.Fatalf("failed to create agent: %v", err)
	}

	reader, err := bot.ChatStream(ctx, "test-skill-session", "你是谁？用一句话回答。")
	if err != nil {
		t.Fatalf("ChatStream failed: %v", err)
	}
	defer reader.Close()

	var buf strings.Builder
	for {
		msg, recvErr := reader.Recv()
		if recvErr != nil {
			if recvErr != io.EOF {
				t.Fatalf("Recv error: %v", recvErr)
			}
			break
		}
		buf.WriteString(msg.Content)
	}

	if buf.Len() == 0 {
		t.Error("expected non-empty response")
	}
	t.Logf("Response: %s", buf.String())
}
