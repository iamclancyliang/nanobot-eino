package workspace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSyncTemplates_CreatesAllFromDefaults(t *testing.T) {
	dstDir := filepath.Join(t.TempDir(), "prompts")

	if err := SyncTemplates(dstDir); err != nil {
		t.Fatalf("SyncTemplates failed: %v", err)
	}

	for _, name := range TemplateFiles {
		got, err := os.ReadFile(filepath.Join(dstDir, name))
		if err != nil {
			t.Fatalf("%s not created: %v", name, err)
		}
		if len(got) == 0 {
			t.Errorf("%s is empty", name)
		}
	}
}

func TestSyncTemplates_DoesNotOverwrite(t *testing.T) {
	dstDir := t.TempDir()
	os.WriteFile(filepath.Join(dstDir, "SOUL.md"), []byte("existing soul"), 0644)

	if err := SyncTemplates(dstDir); err != nil {
		t.Fatalf("SyncTemplates failed: %v", err)
	}

	got, _ := os.ReadFile(filepath.Join(dstDir, "SOUL.md"))
	if string(got) != "existing soul" {
		t.Errorf("SOUL.md was overwritten: got %q, want %q", string(got), "existing soul")
	}
}

func TestSyncTemplates_CreatesDstDir(t *testing.T) {
	dstDir := filepath.Join(t.TempDir(), "nested", "prompts")

	if err := SyncTemplates(dstDir); err != nil {
		t.Fatalf("SyncTemplates failed: %v", err)
	}

	info, err := os.Stat(dstDir)
	if err != nil {
		t.Fatalf("dstDir not created: %v", err)
	}
	if !info.IsDir() {
		t.Error("dstDir is not a directory")
	}

	for _, name := range TemplateFiles {
		if _, err := os.Stat(filepath.Join(dstDir, name)); os.IsNotExist(err) {
			t.Errorf("%s was not created", name)
		}
	}
}

func TestSyncTemplates_ExistingFilesPreserved(t *testing.T) {
	dir := t.TempDir()
	for _, name := range TemplateFiles {
		os.WriteFile(filepath.Join(dir, name), []byte("custom-"+name), 0644)
	}

	if err := SyncTemplates(dir); err != nil {
		t.Fatalf("SyncTemplates failed: %v", err)
	}

	for _, name := range TemplateFiles {
		got, _ := os.ReadFile(filepath.Join(dir, name))
		if string(got) != "custom-"+name {
			t.Errorf("%s changed: got %q", name, string(got))
		}
	}
}

func TestSyncTemplates_EmbeddedContent(t *testing.T) {
	dstDir := t.TempDir()

	if err := SyncTemplates(dstDir); err != nil {
		t.Fatalf("SyncTemplates failed: %v", err)
	}

	soul, _ := os.ReadFile(filepath.Join(dstDir, "SOUL.md"))
	if !strings.Contains(string(soul), "nanobot") {
		t.Error("SOUL.md missing nanobot identity")
	}

	user, _ := os.ReadFile(filepath.Join(dstDir, "USER.md"))
	if !strings.Contains(string(user), "User Profile") {
		t.Error("USER.md missing profile structure")
	}
	if !strings.Contains(string(user), "Primary Role") {
		t.Error("USER.md missing Work Context section")
	}

	agents, _ := os.ReadFile(filepath.Join(dstDir, "AGENTS.md"))
	if !strings.Contains(string(agents), "User Profile Management") {
		t.Error("AGENTS.md missing User Profile Management section")
	}
	if !strings.Contains(string(agents), "Heartbeat Tasks") {
		t.Error("AGENTS.md missing Heartbeat Tasks section")
	}
}

func TestEmbeddedTemplates_AllPresent(t *testing.T) {
	for _, name := range TemplateFiles {
		content, err := embeddedTemplates.ReadFile("templates/" + name)
		if err != nil {
			t.Errorf("embedded template %s missing: %v", name, err)
			continue
		}
		if len(content) == 0 {
			t.Errorf("embedded template %s is empty", name)
		}
	}
}
