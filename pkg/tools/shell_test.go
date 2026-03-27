package tools

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"
	"testing"
)

func compilePatterns(patterns []string) []*regexp.Regexp {
	var result []*regexp.Regexp
	for _, p := range patterns {
		result = append(result, regexp.MustCompile(p))
	}
	return result
}

func TestGuardCommand_DangerousPatterns(t *testing.T) {
	deny := compilePatterns(defaultDenyPatterns)

	dangerous := []string{
		"rm -rf /",
		"rm -f important.txt",
		"shutdown now",
		"reboot",
		"dd if=/dev/zero of=/dev/sda",
		"mkfs.ext4 /dev/sda1",
	}

	for _, cmd := range dangerous {
		if msg := guardCommand(cmd, "/tmp", deny, nil, false); msg == "" {
			t.Errorf("expected %q to be blocked", cmd)
		}
	}
}

func TestGuardCommand_SafeCommands(t *testing.T) {
	deny := compilePatterns(defaultDenyPatterns)

	safe := []string{
		"ls -la",
		"cat file.txt",
		"echo hello",
		"go build ./...",
		"git status",
		"python3 script.py",
	}

	for _, cmd := range safe {
		if msg := guardCommand(cmd, "/tmp", deny, nil, false); msg != "" {
			t.Errorf("expected %q to be allowed, got: %s", cmd, msg)
		}
	}
}

func TestGuardCommand_AllowList(t *testing.T) {
	allow := []*regexp.Regexp{regexp.MustCompile(`^ls\b`)}

	if msg := guardCommand("ls -la", "/tmp", nil, allow, false); msg != "" {
		t.Errorf("ls should be allowed: %s", msg)
	}
	if msg := guardCommand("cat file.txt", "/tmp", nil, allow, false); msg == "" {
		t.Error("cat should be blocked when not in allowlist")
	}
}

func TestGuardCommand_RestrictToWorkspace_PathTraversal(t *testing.T) {
	if msg := guardCommand("cat ../../../etc/passwd", "/workspace", nil, nil, true); msg == "" {
		t.Error("path traversal should be blocked")
	}
}

func TestGuardCommand_RestrictToWorkspace_AbsolutePath(t *testing.T) {
	if msg := guardCommand("cat /etc/passwd", "/workspace", nil, nil, true); msg == "" {
		t.Error("absolute path outside workspace should be blocked")
	}
}

func TestGuardCommand_RestrictToWorkspace_AbsolutePathWithDoubleSpace(t *testing.T) {
	if msg := guardCommand("cat  /etc/passwd", "/workspace", nil, nil, true); msg == "" {
		t.Error("absolute path outside workspace should be blocked")
	}
}

func TestGuardCommand_RestrictToWorkspace_WorkspacePathAllowed(t *testing.T) {
	if msg := guardCommand("cat file.txt", "/workspace", nil, nil, true); msg != "" {
		t.Errorf("relative path should be allowed: %s", msg)
	}
}

func TestExtractAbsolutePaths(t *testing.T) {
	tests := []struct {
		command string
		wantLen int
	}{
		{"cat /etc/passwd", 1},
		{"cat  /etc/passwd", 1},
		{"echo hello", 0},
		{"ls  ~/Documents", 1},
		{"cat  /tmp/a  /tmp/b", 2},
		{"echo |  /dev/null", 1},
	}

	for _, tc := range tests {
		paths := extractAbsolutePaths(tc.command)
		if len(paths) != tc.wantLen {
			t.Errorf("extractAbsolutePaths(%q) = %d paths, want %d: %v",
				tc.command, len(paths), tc.wantLen, paths)
		}
	}
}

func TestGuardCommand_BlocksInternalURLInCommand(t *testing.T) {
	deny := compilePatterns(defaultDenyPatterns)
	msg := guardCommand(`curl "http://127.0.0.1:8080/health"`, "/workspace", deny, nil, false)
	if msg == "" {
		t.Fatal("expected command containing internal URL to be blocked")
	}
	if !strings.Contains(strings.ToLower(msg), "internal") {
		t.Fatalf("expected internal URL block message, got: %s", msg)
	}
}

func TestGuardCommand_AllowsPublicURLInCommand(t *testing.T) {
	deny := compilePatterns(defaultDenyPatterns)
	msg := guardCommand(`curl "https://example.com/health"`, "/workspace", deny, nil, false)
	if msg != "" {
		t.Fatalf("expected public URL command to be allowed, got: %s", msg)
	}
}

func TestShellTool_BasicExecution(t *testing.T) {
	shellTool, err := NewShellTool(ShellConfig{})
	if err != nil {
		t.Fatalf("NewShellTool error: %v", err)
	}

	args, _ := json.Marshal(ShellExecArgs{Command: "echo hello"})
	result, err := shellTool.InvokableRun(context.Background(), string(args))
	if err != nil {
		t.Fatalf("InvokableRun error: %v", err)
	}

	if !strings.Contains(result, "hello") {
		t.Errorf("expected 'hello' in output, got: %s", result)
	}
	if !strings.Contains(result, "Exit code: 0") {
		t.Errorf("expected exit code 0, got: %s", result)
	}
}

func TestShellTool_BlocksDangerousCommand(t *testing.T) {
	shellTool, _ := NewShellTool(ShellConfig{})

	args, _ := json.Marshal(ShellExecArgs{Command: "rm -rf /"})
	result, err := shellTool.InvokableRun(context.Background(), string(args))
	if err != nil {
		t.Fatalf("InvokableRun error: %v", err)
	}

	if !strings.Contains(result, "blocked") {
		t.Errorf("dangerous command should be blocked, got: %s", result)
	}
}

func TestShellTool_OutputTruncation(t *testing.T) {
	shellTool, _ := NewShellTool(ShellConfig{MaxOutput: 100})

	args, _ := json.Marshal(ShellExecArgs{Command: "seq 1 1000"})
	result, _ := shellTool.InvokableRun(context.Background(), string(args))

	if !strings.Contains(result, "truncated") {
		t.Errorf("long output should be truncated, got len=%d", len(result))
	}
}

func TestShellTool_NonZeroExitCode(t *testing.T) {
	shellTool, _ := NewShellTool(ShellConfig{})

	args, _ := json.Marshal(ShellExecArgs{Command: "exit 42"})
	result, _ := shellTool.InvokableRun(context.Background(), string(args))

	if !strings.Contains(result, "Exit code: 42") {
		t.Errorf("should report exit code 42, got: %s", result)
	}
}

func TestShellTool_ToolInfo(t *testing.T) {
	shellTool, _ := NewShellTool(ShellConfig{})

	info, err := shellTool.Info(context.Background())
	if err != nil {
		t.Fatalf("Info error: %v", err)
	}
	if info.Name != "exec" {
		t.Errorf("tool name = %q, want %q", info.Name, "exec")
	}
}
