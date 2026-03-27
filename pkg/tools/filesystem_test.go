package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cloudwego/eino/components/tool"
)

// --- helpers ---

func invokeReadFile(t *testing.T, it tool.InvokableTool, path string) string {
	t.Helper()
	args, _ := json.Marshal(ReadFileArgs{Path: path})
	result, err := it.InvokableRun(context.Background(), string(args))
	if err != nil {
		t.Fatalf("InvokableRun error: %v", err)
	}
	return result
}

func invokeEditFile(t *testing.T, it tool.InvokableTool, path, oldText, newText string) string {
	t.Helper()
	args, _ := json.Marshal(EditFileArgs{Path: path, OldText: oldText, NewText: newText})
	result, err := it.InvokableRun(context.Background(), string(args))
	if err != nil {
		t.Fatalf("InvokableRun error: %v", err)
	}
	return result
}

func invokeWriteFile(t *testing.T, it tool.InvokableTool, path, content string) string {
	t.Helper()
	args, _ := json.Marshal(WriteFileArgs{Path: path, Content: content})
	result, err := it.InvokableRun(context.Background(), string(args))
	if err != nil {
		t.Fatalf("InvokableRun error: %v", err)
	}
	return result
}

// --- Test: read_file can read from extra_allowed_dirs (skills dir) ---
// Corresponds to nanobot's test_read_allowed_with_extra_dir

func TestReadAllowedWithExtraDir(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), "ws")
	os.MkdirAll(workspace, 0755)
	skillsDir := filepath.Join(t.TempDir(), "skills")
	skillDir := filepath.Join(skillsDir, "test_skill")
	os.MkdirAll(skillDir, 0755)
	skillFile := filepath.Join(skillDir, "SKILL.md")
	os.WriteFile(skillFile, []byte("# Test Skill\nDo something."), 0644)

	tool := NewReadFileTool(workspace, workspace, skillsDir)
	result := invokeReadFile(t, tool, skillFile)

	if !strings.Contains(result, "Test Skill") {
		t.Errorf("expected result to contain 'Test Skill', got: %s", result)
	}
	if strings.Contains(result, "Error") {
		t.Errorf("unexpected error in result: %s", result)
	}
}

// --- Test: read_file blocked for unrelated dir with extra_allowed_dirs ---
// Corresponds to nanobot's test_read_still_blocked_for_unrelated_dir

func TestReadBlockedForUnrelatedDir(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), "ws")
	os.MkdirAll(workspace, 0755)
	skillsDir := filepath.Join(t.TempDir(), "skills")
	os.MkdirAll(skillsDir, 0755)
	unrelated := filepath.Join(t.TempDir(), "other")
	os.MkdirAll(unrelated, 0755)
	secret := filepath.Join(unrelated, "secret.txt")
	os.WriteFile(secret, []byte("nope"), 0644)

	tool := NewReadFileTool(workspace, workspace, skillsDir)
	result := invokeReadFile(t, tool, secret)

	if !strings.Contains(result, "Error") {
		t.Errorf("expected Error for unrelated dir, got: %s", result)
	}
	if !strings.Contains(strings.ToLower(result), "outside") {
		t.Errorf("expected 'outside' in error, got: %s", result)
	}
}

// --- Test: workspace file still readable with extra_allowed_dirs ---
// Corresponds to nanobot's test_workspace_file_still_readable_with_extra_dirs

func TestWorkspaceFileStillReadableWithExtraDirs(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), "ws")
	os.MkdirAll(workspace, 0755)
	wsFile := filepath.Join(workspace, "README.md")
	os.WriteFile(wsFile, []byte("hello from workspace"), 0644)
	skillsDir := filepath.Join(t.TempDir(), "skills")
	os.MkdirAll(skillsDir, 0755)

	tool := NewReadFileTool(workspace, workspace, skillsDir)
	result := invokeReadFile(t, tool, wsFile)

	if !strings.Contains(result, "hello from workspace") {
		t.Errorf("expected workspace content, got: %s", result)
	}
	if strings.Contains(result, "Error") {
		t.Errorf("unexpected error: %s", result)
	}
}

// --- Test: extra_allowed_dirs does not widen write ---
// Corresponds to nanobot's test_extra_dirs_does_not_widen_write

func TestExtraDirsDoesNotWidenWrite(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), "ws")
	os.MkdirAll(workspace, 0755)
	outside := filepath.Join(t.TempDir(), "outside")
	os.MkdirAll(outside, 0755)

	tool := NewWriteFileTool(workspace, workspace)
	hackPath := filepath.Join(outside, "hack.txt")
	result := invokeWriteFile(t, tool, hackPath, "pwned")

	if !strings.Contains(result, "Error") {
		t.Errorf("expected Error for write outside workspace, got: %s", result)
	}
	if !strings.Contains(strings.ToLower(result), "outside") {
		t.Errorf("expected 'outside' in error, got: %s", result)
	}
	if _, err := os.Stat(hackPath); !os.IsNotExist(err) {
		t.Error("hack.txt should NOT have been created")
	}
}

// --- Test: edit_file blocked in extra_allowed_dirs ---
// Corresponds to nanobot's test_edit_blocked_in_extra_dir

func TestEditBlockedInExtraDir(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), "ws")
	os.MkdirAll(workspace, 0755)
	skillsDir := filepath.Join(t.TempDir(), "skills")
	skillDir := filepath.Join(skillsDir, "weather")
	os.MkdirAll(skillDir, 0755)
	skillFile := filepath.Join(skillDir, "SKILL.md")
	os.WriteFile(skillFile, []byte("# Weather\nOriginal content."), 0644)

	tool := NewEditFileTool(workspace, workspace)
	result := invokeEditFile(t, tool, skillFile, "Original content.", "Hacked content.")

	if !strings.Contains(result, "Error") {
		t.Errorf("expected Error for edit outside workspace, got: %s", result)
	}
	if !strings.Contains(strings.ToLower(result), "outside") {
		t.Errorf("expected 'outside' in error, got: %s", result)
	}

	content, _ := os.ReadFile(skillFile)
	if string(content) != "# Weather\nOriginal content." {
		t.Errorf("skill file should not have been modified, got: %s", string(content))
	}
}

// --- Test: read_file basic functionality ---

func TestReadFileBasic(t *testing.T) {
	workspace := t.TempDir()
	testFile := filepath.Join(workspace, "test.txt")
	os.WriteFile(testFile, []byte("line1\nline2\nline3"), 0644)

	tool := NewReadFileTool(workspace, "", nil...)
	result := invokeReadFile(t, tool, testFile)

	if !strings.Contains(result, "line1") {
		t.Errorf("expected line1 in result: %s", result)
	}
	if !strings.Contains(result, "line2") {
		t.Errorf("expected line2 in result: %s", result)
	}
}

// --- Test: read_file with offset and limit ---

func TestReadFileOffsetLimit(t *testing.T) {
	workspace := t.TempDir()
	var lines []string
	for i := 1; i <= 10; i++ {
		lines = append(lines, "line"+string(rune('0'+i)))
	}
	testFile := filepath.Join(workspace, "big.txt")
	os.WriteFile(testFile, []byte(strings.Join(lines, "\n")), 0644)

	tool := NewReadFileTool(workspace, "")
	args, _ := json.Marshal(ReadFileArgs{Path: testFile, Offset: 3, Limit: 2})
	result, _ := tool.InvokableRun(context.Background(), string(args))

	if !strings.Contains(result, "3|") {
		t.Errorf("expected line 3 in offset result: %s", result)
	}
}

// --- Test: read_file nonexistent file ---

func TestReadFileNotFound(t *testing.T) {
	workspace := t.TempDir()
	tool := NewReadFileTool(workspace, "")
	result := invokeReadFile(t, tool, filepath.Join(workspace, "nope.txt"))

	if !strings.Contains(result, "Error") {
		t.Errorf("expected Error for nonexistent file: %s", result)
	}
}

// --- Test: write_file creates parent directories ---

func TestWriteFileCreatesParentDirs(t *testing.T) {
	workspace := t.TempDir()
	nested := filepath.Join(workspace, "a", "b", "c", "file.txt")

	tool := NewWriteFileTool(workspace, "")
	result := invokeWriteFile(t, tool, nested, "hello nested")

	if strings.Contains(result, "Error") {
		t.Errorf("unexpected error: %s", result)
	}

	content, err := os.ReadFile(nested)
	if err != nil {
		t.Fatalf("file should exist: %v", err)
	}
	if string(content) != "hello nested" {
		t.Errorf("unexpected content: %s", string(content))
	}
}

// --- Test: edit_file basic replacement ---

func TestEditFileBasic(t *testing.T) {
	workspace := t.TempDir()
	testFile := filepath.Join(workspace, "edit.txt")
	os.WriteFile(testFile, []byte("hello world"), 0644)

	tool := NewEditFileTool(workspace, "")
	result := invokeEditFile(t, tool, testFile, "hello", "goodbye")

	if strings.Contains(result, "Error") {
		t.Errorf("unexpected error: %s", result)
	}

	content, _ := os.ReadFile(testFile)
	if string(content) != "goodbye world" {
		t.Errorf("expected 'goodbye world', got: %s", string(content))
	}
}

// --- Test: edit_file old_text not found ---

func TestEditFileNotFound(t *testing.T) {
	workspace := t.TempDir()
	testFile := filepath.Join(workspace, "edit.txt")
	os.WriteFile(testFile, []byte("hello world"), 0644)

	tool := NewEditFileTool(workspace, "")
	result := invokeEditFile(t, tool, testFile, "nonexistent text", "replacement")

	if !strings.Contains(result, "Error") {
		t.Errorf("expected error for missing old_text: %s", result)
	}
}
