package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSetGetConfigPath(t *testing.T) {
	mu.Lock()
	old := currentConfigPath
	mu.Unlock()
	defer func() {
		mu.Lock()
		currentConfigPath = old
		mu.Unlock()
	}()

	custom := "/tmp/test-nanobot/config.yaml"
	SetConfigPath(custom)
	got := GetConfigPath()
	if got != custom {
		t.Errorf("GetConfigPath() = %q, want %q", got, custom)
	}
}

func TestGetConfigPathDefault(t *testing.T) {
	mu.Lock()
	old := currentConfigPath
	currentConfigPath = ""
	mu.Unlock()
	defer func() {
		mu.Lock()
		currentConfigPath = old
		mu.Unlock()
	}()

	got := GetConfigPath()
	if filepath.Base(got) != "config.yaml" {
		t.Errorf("GetConfigPath() default = %q, want ending config.yaml", got)
	}
	if filepath.Base(filepath.Dir(got)) != ".nanobot-eino" {
		t.Errorf("GetConfigPath() dir = %q, want parent .nanobot-eino", filepath.Dir(got))
	}
}

func TestGetDataDir(t *testing.T) {
	mu.Lock()
	old := currentConfigPath
	mu.Unlock()
	defer func() {
		mu.Lock()
		currentConfigPath = old
		mu.Unlock()
	}()

	SetConfigPath("/tmp/test-nanobot-eino/config.yaml")
	got := GetDataDir()
	if got != "/tmp/test-nanobot-eino" {
		t.Errorf("GetDataDir() = %q, want %q", got, "/tmp/test-nanobot-eino")
	}
}

func TestGetRuntimeSubdir(t *testing.T) {
	dir := t.TempDir()
	mu.Lock()
	old := currentConfigPath
	mu.Unlock()
	defer func() {
		mu.Lock()
		currentConfigPath = old
		mu.Unlock()
	}()

	SetConfigPath(filepath.Join(dir, "config.yaml"))
	sub := GetRuntimeSubdir("logs")

	want := filepath.Join(dir, "logs")
	if sub != want {
		t.Errorf("GetRuntimeSubdir(\"logs\") = %q, want %q", sub, want)
	}
	if _, err := os.Stat(sub); os.IsNotExist(err) {
		t.Errorf("directory %q should have been created", sub)
	}
}

func TestGetMediaDir(t *testing.T) {
	dir := t.TempDir()
	mu.Lock()
	old := currentConfigPath
	mu.Unlock()
	defer func() {
		mu.Lock()
		currentConfigPath = old
		mu.Unlock()
	}()

	SetConfigPath(filepath.Join(dir, "config.yaml"))

	base := GetMediaDir("")
	if filepath.Base(base) != "media" {
		t.Errorf("GetMediaDir(\"\") = %q, want ending media", base)
	}

	channeled := GetMediaDir("feishu")
	if filepath.Base(channeled) != "feishu" {
		t.Errorf("GetMediaDir(\"feishu\") = %q, want ending feishu", channeled)
	}
}

func TestGetWorkspacePath(t *testing.T) {
	dir := t.TempDir()
	mu.Lock()
	old := currentConfigPath
	mu.Unlock()
	defer func() {
		mu.Lock()
		currentConfigPath = old
		mu.Unlock()
	}()

	SetConfigPath(filepath.Join(dir, "config.yaml"))

	explicit := GetWorkspacePath("/tmp/my-workspace")
	if explicit != "/tmp/my-workspace" {
		t.Errorf("GetWorkspacePath with explicit = %q", explicit)
	}

	defaultWs := GetWorkspacePath("")
	want := filepath.Join(dir, "workspace")
	if defaultWs != want {
		t.Errorf("GetWorkspacePath(\"\") = %q, want %q", defaultWs, want)
	}
}

func TestGetCLIHistoryPath(t *testing.T) {
	dir := t.TempDir()
	mu.Lock()
	old := currentConfigPath
	mu.Unlock()
	defer func() {
		mu.Lock()
		currentConfigPath = old
		mu.Unlock()
	}()

	SetConfigPath(filepath.Join(dir, "config.yaml"))
	got := GetCLIHistoryPath()
	want := filepath.Join(dir, "history", "cli_history")
	if got != want {
		t.Errorf("GetCLIHistoryPath() = %q, want %q", got, want)
	}
}

func TestGetCronStorePath(t *testing.T) {
	dir := t.TempDir()
	mu.Lock()
	old := currentConfigPath
	mu.Unlock()
	defer func() {
		mu.Lock()
		currentConfigPath = old
		mu.Unlock()
	}()

	SetConfigPath(filepath.Join(dir, "config.yaml"))
	got := GetCronStorePath()
	want := filepath.Join(dir, "cron", "jobs.json")
	if got != want {
		t.Errorf("GetCronStorePath() = %q, want %q", got, want)
	}
}

func TestAllPathsUnderDataDir(t *testing.T) {
	dir := t.TempDir()
	mu.Lock()
	old := currentConfigPath
	mu.Unlock()
	defer func() {
		mu.Lock()
		currentConfigPath = old
		mu.Unlock()
	}()

	SetConfigPath(filepath.Join(dir, "config.yaml"))

	paths := map[string]string{
		"sessions": GetSessionsDir(),
		"memory":   GetMemoryDir(),
		"media":    GetMediaDir(""),
		"cron":     GetCronDir(),
		"logs":     GetLogsDir(),
		"prompts":  GetPromptsDir(),
		"skills":   GetSkillsDir(),
	}
	for name, p := range paths {
		rel, err := filepath.Rel(dir, p)
		if err != nil || rel[:2] == ".." {
			t.Errorf("%s path %q is not under data dir %q", name, p, dir)
		}
	}
}
