package config

import (
	"os"
	"path/filepath"
	"sync"
)

var (
	mu                sync.RWMutex
	currentConfigPath string
)

// SetConfigPath stores the active config file path so that runtime directories
// can be derived from its parent (supports multi-instance deployments).
func SetConfigPath(path string) {
	mu.Lock()
	defer mu.Unlock()
	currentConfigPath = expandHome(path)
}

// GetConfigPath returns the active config file path, falling back to
// ~/.nanobot-eino/config.yaml when none was explicitly set.
func GetConfigPath() string {
	mu.RLock()
	defer mu.RUnlock()
	if currentConfigPath != "" {
		return currentConfigPath
	}
	return DefaultConfigPath()
}

// GetDataDir returns the instance-level runtime data directory, derived from
// the config file's parent directory. This mirrors nanobot's get_data_dir():
//
//	~/.nanobot-eino/config.yaml  →  ~/.nanobot-eino/
func GetDataDir() string {
	return ensureDir(filepath.Dir(GetConfigPath()))
}

// GetRuntimeSubdir returns a named subdirectory under the data dir,
// creating it if necessary.  e.g. GetRuntimeSubdir("cron") → ~/.nanobot-eino/cron/
func GetRuntimeSubdir(name string) string {
	return ensureDir(filepath.Join(GetDataDir(), name))
}

// GetWorkspacePath resolves the agent workspace directory.
// Default: ~/.nanobot-eino/workspace
func GetWorkspacePath(workspace string) string {
	if workspace != "" {
		return ensureDir(expandHome(workspace))
	}
	return ensureDir(filepath.Join(GetDataDir(), "workspace"))
}

// GetSessionsDir returns ~/.nanobot-eino/sessions/
func GetSessionsDir() string {
	return GetRuntimeSubdir("sessions")
}

// GetMemoryDir returns ~/.nanobot-eino/memory/
func GetMemoryDir() string {
	return GetRuntimeSubdir("memory")
}

// GetMediaDir returns ~/.nanobot-eino/media/ or ~/.nanobot-eino/media/{channel}
func GetMediaDir(channel string) string {
	base := GetRuntimeSubdir("media")
	if channel != "" {
		return ensureDir(filepath.Join(base, channel))
	}
	return base
}

// GetCronDir returns ~/.nanobot-eino/cron/
func GetCronDir() string {
	return GetRuntimeSubdir("cron")
}

// GetLogsDir returns ~/.nanobot-eino/logs/
func GetLogsDir() string {
	return GetRuntimeSubdir("logs")
}

// GetPromptsDir returns ~/.nanobot-eino/prompts/
func GetPromptsDir() string {
	return GetRuntimeSubdir("prompts")
}

// GetSkillsDir returns ~/.nanobot-eino/skills/
func GetSkillsDir() string {
	return GetRuntimeSubdir("skills")
}

// GetCLIHistoryPath returns ~/.nanobot-eino/history/cli_history
func GetCLIHistoryPath() string {
	dir := GetRuntimeSubdir("history")
	return filepath.Join(dir, "cli_history")
}

// GetCronStorePath returns ~/.nanobot-eino/cron/jobs.json
func GetCronStorePath() string {
	return filepath.Join(GetCronDir(), "jobs.json")
}

func ensureDir(path string) string {
	_ = os.MkdirAll(path, 0755)
	return path
}
