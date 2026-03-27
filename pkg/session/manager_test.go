package session

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cloudwego/eino/schema"
)

func TestSessionManager_NewAndGetOrCreate(t *testing.T) {
	mgr, err := NewSessionManager(t.TempDir())
	if err != nil {
		t.Fatalf("NewSessionManager error: %v", err)
	}

	s := mgr.GetOrCreate("test-session")
	if s.Key != "test-session" {
		t.Errorf("Key = %q, want %q", s.Key, "test-session")
	}
	if s.CreatedAt.IsZero() {
		t.Error("CreatedAt should not be zero")
	}
	if len(s.Messages) != 0 {
		t.Errorf("new session should have 0 messages, got %d", len(s.Messages))
	}
}

func TestSessionManager_GetOrCreate_ReturnsCached(t *testing.T) {
	mgr, _ := NewSessionManager(t.TempDir())

	s1 := mgr.GetOrCreate("key1")
	s1.AddMessage(schema.UserMessage("hello"))

	s2 := mgr.GetOrCreate("key1")
	if len(s2.Messages) != 1 {
		t.Errorf("cached session should have 1 message, got %d", len(s2.Messages))
	}
}

func TestSession_AddMessage(t *testing.T) {
	s := &Session{Key: "test", Metadata: make(map[string]any)}

	s.AddMessage(schema.UserMessage("first"))
	s.AddMessage(&schema.Message{Role: schema.Assistant, Content: "second"})

	if len(s.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(s.Messages))
	}
	if s.Messages[0].Content != "first" {
		t.Errorf("Messages[0].Content = %q, want %q", s.Messages[0].Content, "first")
	}
	if s.UpdatedAt.IsZero() {
		t.Error("UpdatedAt should be set after AddMessage")
	}
}

func TestSession_GetHistory_All(t *testing.T) {
	s := &Session{Key: "test"}
	s.Messages = []*schema.Message{
		schema.UserMessage("a"),
		{Role: schema.Assistant, Content: "b"},
		schema.UserMessage("c"),
	}

	h := s.GetHistory(0)
	if len(h) != 3 {
		t.Errorf("GetHistory(0) returned %d messages, want 3", len(h))
	}
}

func TestSession_GetHistory_WithMaxMessages(t *testing.T) {
	s := &Session{Key: "test"}
	s.Messages = []*schema.Message{
		schema.UserMessage("a"),
		{Role: schema.Assistant, Content: "b"},
		schema.UserMessage("c"),
		{Role: schema.Assistant, Content: "d"},
	}

	h := s.GetHistory(2)
	if len(h) != 2 {
		t.Errorf("GetHistory(2) returned %d messages, want 2", len(h))
	}
	if h[0].Content != "c" {
		t.Errorf("first message should be 'c', got %q", h[0].Content)
	}
}

func TestSession_GetHistory_SkipsLeadingNonUser(t *testing.T) {
	s := &Session{Key: "test"}
	s.Messages = []*schema.Message{
		{Role: schema.Assistant, Content: "orphan"},
		schema.UserMessage("real"),
		{Role: schema.Assistant, Content: "reply"},
	}

	h := s.GetHistory(0)
	if len(h) != 2 {
		t.Errorf("GetHistory should skip leading assistant, got %d messages", len(h))
	}
	if h[0].Role != schema.User {
		t.Errorf("first message role = %s, want user", h[0].Role)
	}
}

func TestSession_GetHistory_WithConsolidation(t *testing.T) {
	s := &Session{Key: "test", LastConsolidated: 2}
	s.Messages = []*schema.Message{
		schema.UserMessage("old1"),
		{Role: schema.Assistant, Content: "old2"},
		schema.UserMessage("new1"),
		{Role: schema.Assistant, Content: "new2"},
	}

	h := s.GetHistory(0)
	if len(h) != 2 {
		t.Errorf("GetHistory should return 2 unconsolidated, got %d", len(h))
	}
	if h[0].Content != "new1" {
		t.Errorf("first unconsolidated = %q, want %q", h[0].Content, "new1")
	}
}

func TestSession_GetHistory_AllConsolidated(t *testing.T) {
	s := &Session{Key: "test", LastConsolidated: 3}
	s.Messages = []*schema.Message{
		schema.UserMessage("a"),
		{Role: schema.Assistant, Content: "b"},
		schema.UserMessage("c"),
	}

	h := s.GetHistory(0)
	if h != nil {
		t.Errorf("expected nil when all consolidated, got %d messages", len(h))
	}
}

func TestSession_Clear(t *testing.T) {
	s := &Session{Key: "test", LastConsolidated: 5}
	s.Messages = []*schema.Message{schema.UserMessage("x")}

	s.Clear()
	if s.Messages != nil {
		t.Error("Messages should be nil after Clear")
	}
	if s.LastConsolidated != 0 {
		t.Errorf("LastConsolidated = %d, want 0", s.LastConsolidated)
	}
}

func TestSessionManager_SaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	mgr, _ := NewSessionManager(dir)

	s := mgr.GetOrCreate("feishu:group1")
	s.AddMessage(schema.UserMessage("hello"))
	s.AddMessage(&schema.Message{Role: schema.Assistant, Content: "hi there"})
	s.LastConsolidated = 1
	s.Metadata["foo"] = "bar"

	if err := mgr.Save(s); err != nil {
		t.Fatalf("Save error: %v", err)
	}

	mgr2, _ := NewSessionManager(dir)
	s2 := mgr2.GetOrCreate("feishu:group1")

	if len(s2.Messages) != 2 {
		t.Fatalf("loaded session has %d messages, want 2", len(s2.Messages))
	}
	if s2.Messages[0].Content != "hello" {
		t.Errorf("Messages[0] = %q, want %q", s2.Messages[0].Content, "hello")
	}
	if s2.LastConsolidated != 1 {
		t.Errorf("LastConsolidated = %d, want 1", s2.LastConsolidated)
	}
}

func TestSessionManager_Invalidate(t *testing.T) {
	mgr, _ := NewSessionManager(t.TempDir())

	s := mgr.GetOrCreate("k1")
	s.AddMessage(schema.UserMessage("cached"))

	mgr.Invalidate("k1")

	s2 := mgr.GetOrCreate("k1")
	if len(s2.Messages) != 0 {
		t.Errorf("after Invalidate, new session should be empty, got %d messages", len(s2.Messages))
	}
}

func TestSessionManager_ListSessions(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.jsonl"), []byte("{}"), 0644)
	os.WriteFile(filepath.Join(dir, "b.jsonl"), []byte("{}"), 0644)

	mgr, _ := NewSessionManager(dir)
	list := mgr.ListSessions()

	if len(list) != 2 {
		t.Errorf("ListSessions returned %d, want 2", len(list))
	}
}

func TestSessionManager_SessionPathSanitization(t *testing.T) {
	dir := t.TempDir()
	mgr, _ := NewSessionManager(dir)

	s := mgr.GetOrCreate("feishu:group:123")
	s.AddMessage(schema.UserMessage("test"))
	if err := mgr.Save(s); err != nil {
		t.Fatalf("Save error: %v", err)
	}

	expected := filepath.Join(dir, "feishu_group_123.jsonl")
	if _, err := os.Stat(expected); os.IsNotExist(err) {
		t.Errorf("expected file %s to exist (colons replaced with underscores)", expected)
	}
}
