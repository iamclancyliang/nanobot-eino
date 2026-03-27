//go:build integration

package memory

import (
	"context"
	"os"
	"testing"

	"github.com/cloudwego/eino/schema"

	"github.com/wall/nanobot-eino/pkg/model"
	"github.com/wall/nanobot-eino/pkg/session"
)

func TestIntegration_Consolidate(t *testing.T) {
	apiKey := os.Getenv("NANOBOT_MODEL_API_KEY")
	if apiKey == "" {
		t.Skip("NANOBOT_MODEL_API_KEY not set")
	}

	ctx := context.Background()

	memDir := t.TempDir()
	memStore, err := NewMemoryStore(memDir)
	if err != nil {
		t.Fatalf("failed to init memory store: %v", err)
	}

	sessDir := t.TempDir()
	sessionMgr, err := session.NewSessionManager(sessDir)
	if err != nil {
		t.Fatalf("failed to init session manager: %v", err)
	}

	cfg := model.GetDefaultConfig()
	chatModel, err := model.NewChatModel(ctx, cfg)
	if err != nil {
		t.Fatalf("failed to create model: %v", err)
	}

	consolidator := NewMemoryConsolidator(memStore, chatModel, sessionMgr, 65536, 2000)

	sessionID := "test-session"
	sess := sessionMgr.GetOrCreate(sessionID)

	messages := []string{
		"My name is Alice and I love Go programming.",
		"I am working on a project called Nanobot-Eino.",
		"I prefer using file-based storage for simplicity.",
	}

	for _, msg := range messages {
		sess.AddMessage(schema.UserMessage(msg))
		sess.AddMessage(&schema.Message{Role: schema.Assistant, Content: "Understood."})
	}
	if err := sessionMgr.Save(sess); err != nil {
		t.Fatalf("failed to save session: %v", err)
	}

	chunk := sess.Messages[sess.LastConsolidated:]
	if !consolidator.ConsolidateMessages(ctx, chunk) {
		t.Fatal("consolidation failed")
	}

	fact := memStore.ReadLongTerm()
	if fact == "" {
		t.Error("expected non-empty consolidated facts")
	}
	t.Logf("Consolidated Facts:\n%s", fact)
}
