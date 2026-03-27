package prompt

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoader_BuildSystemMessages(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "SOUL.md"), []byte("I am a helpful cat"), 0644)
	os.WriteFile(filepath.Join(dir, "USER.md"), []byte("User is a developer"), 0644)
	os.WriteFile(filepath.Join(dir, "TOOLS.md"), []byte("Use tools wisely"), 0644)
	os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("Follow instructions"), 0644)

	loader := NewLoader(dir)
	msgs, err := loader.BuildSystemMessages(context.Background())
	if err != nil {
		t.Fatalf("BuildSystemMessages error: %v", err)
	}

	if len(msgs) != 1 {
		t.Fatalf("expected 1 system message, got %d", len(msgs))
	}

	content := msgs[0].Content
	for _, expected := range []string{"SOUL", "helpful cat", "USER PROFILE", "developer", "TOOL USAGE", "wisely", "AGENT INSTRUCTIONS", "Follow instructions"} {
		if !strings.Contains(content, expected) {
			t.Errorf("system message missing %q", expected)
		}
	}
}

func TestLoader_BuildSystemMessages_WithHeartbeat(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "SOUL.md"), []byte("soul"), 0644)
	os.WriteFile(filepath.Join(dir, "USER.md"), []byte("user"), 0644)
	os.WriteFile(filepath.Join(dir, "TOOLS.md"), []byte("tools"), 0644)
	os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("agents"), 0644)
	os.WriteFile(filepath.Join(dir, "HEARTBEAT.md"), []byte("check weather daily"), 0644)

	loader := NewLoader(dir)
	msgs, err := loader.BuildSystemMessages(context.Background())
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	if !strings.Contains(msgs[0].Content, "HEARTBEAT TASKS") {
		t.Error("should include HEARTBEAT section")
	}
	if !strings.Contains(msgs[0].Content, "check weather daily") {
		t.Error("should include heartbeat content")
	}
}

func TestLoader_BuildSystemMessages_NoHeartbeat(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "SOUL.md"), []byte("soul"), 0644)
	os.WriteFile(filepath.Join(dir, "USER.md"), []byte("user"), 0644)
	os.WriteFile(filepath.Join(dir, "TOOLS.md"), []byte("tools"), 0644)
	os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("agents"), 0644)

	loader := NewLoader(dir)
	msgs, err := loader.BuildSystemMessages(context.Background())
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	if strings.Contains(msgs[0].Content, "HEARTBEAT") {
		t.Error("should not include HEARTBEAT section when file missing")
	}
}

func TestLoader_BuildSystemMessages_MissingRequiredFile(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "SOUL.md"), []byte("soul"), 0644)

	loader := NewLoader(dir)
	_, err := loader.BuildSystemMessages(context.Background())
	if err == nil {
		t.Error("should error when required file is missing")
	}
}

func TestLoader_ReadFile(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "test.md"), []byte("content here"), 0644)

	loader := NewLoader(dir)
	content, err := loader.readFile("test.md")
	if err != nil {
		t.Fatalf("readFile error: %v", err)
	}
	if content != "content here" {
		t.Errorf("readFile = %q, want %q", content, "content here")
	}
}

func TestLoader_ReadFile_NotFound(t *testing.T) {
	loader := NewLoader(t.TempDir())
	_, err := loader.readFile("nonexistent.md")
	if err == nil {
		t.Error("should error for nonexistent file")
	}
}
