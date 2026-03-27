package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func clearProviderEnv(t *testing.T) {
	t.Helper()
	t.Setenv("NANOBOT_PROVIDER", "")
	t.Setenv("NANOBOT_AGENT_MODEL", "")
	t.Setenv("NANOBOT_MAX_TOKENS", "")
	t.Setenv("NANOBOT_TEMPERATURE", "")
	t.Setenv("NANOBOT_REASONING_EFFORT", "")
}

func isolateConfigSearchPath(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	wd := t.TempDir()
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error: %v", err)
	}
	if err := os.Chdir(wd); err != nil {
		t.Fatalf("Chdir(%q) error: %v", wd, err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(oldWD)
	})
}

func TestDefaultConfigPath(t *testing.T) {
	if filepath.Base(DefaultConfigPath()) != "config.yaml" {
		t.Fatalf("DefaultConfigPath() = %q, want filename config.yaml", DefaultConfigPath())
	}
	if filepath.Base(DefaultConfigDir()) != ".nanobot-eino" {
		t.Fatalf("DefaultConfigDir() = %q, want ending .nanobot-eino", DefaultConfigDir())
	}
}

func TestLoadDefaults(t *testing.T) {
	clearProviderEnv(t)
	isolateConfigSearchPath(t)

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.Agent.Provider != "auto" {
		t.Errorf("Agent.Provider = %q, want %q", cfg.Agent.Provider, "auto")
	}
	if cfg.EffectiveModel() != "gpt-4o" {
		t.Errorf("EffectiveModel() = %q, want %q", cfg.EffectiveModel(), "gpt-4o")
	}
	if cfg.Agent.ContextWindowTokens != 65536 {
		t.Errorf("Agent.ContextWindowTokens = %d, want %d", cfg.Agent.ContextWindowTokens, 65536)
	}
	if cfg.Agent.MaxStep != 20 {
		t.Errorf("Agent.MaxStep = %d, want %d", cfg.Agent.MaxStep, 20)
	}
	if cfg.Gateway.Heartbeat.Interval.Duration != 30*time.Minute {
		t.Errorf("Heartbeat.Interval = %v, want %v", cfg.Gateway.Heartbeat.Interval.Duration, 30*time.Minute)
	}
	if !cfg.Gateway.Heartbeat.IsEnabled() {
		t.Error("Heartbeat should be enabled by default")
	}
	home := os.Getenv("HOME")
	wantMemory := filepath.Join(home, ".nanobot-eino", "memory")
	if cfg.ResolveMemoryDir() != wantMemory {
		t.Errorf("ResolveMemoryDir() = %q, want %q", cfg.ResolveMemoryDir(), wantMemory)
	}
}

func TestLoadEnvOverrides(t *testing.T) {
	clearProviderEnv(t)
	isolateConfigSearchPath(t)

	t.Setenv("NANOBOT_PROVIDER", "qwen")
	t.Setenv("NANOBOT_AGENT_MODEL", "qwen-max")
	t.Setenv("NANOBOT_MAX_TOKENS", "4096")
	t.Setenv("NANOBOT_TEMPERATURE", "0.2")
	t.Setenv("NANOBOT_REASONING_EFFORT", "high")
	t.Setenv("FEISHU_APP_ID", "cli_xxx")

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.Agent.Provider != "qwen" {
		t.Errorf("Agent.Provider = %q, want %q", cfg.Agent.Provider, "qwen")
	}
	if cfg.Agent.Model != "qwen-max" {
		t.Errorf("Agent.Model = %q, want %q", cfg.Agent.Model, "qwen-max")
	}
	if cfg.Agent.MaxTokens != 4096 {
		t.Errorf("Agent.MaxTokens = %d, want %d", cfg.Agent.MaxTokens, 4096)
	}
	if cfg.Agent.Temperature != 0.2 {
		t.Errorf("Agent.Temperature = %v, want 0.2", cfg.Agent.Temperature)
	}
	if cfg.Agent.ReasoningEffort != "high" {
		t.Errorf("Agent.ReasoningEffort = %q, want %q", cfg.Agent.ReasoningEffort, "high")
	}
	if cfg.Channels.Feishu.AppID != "cli_xxx" {
		t.Errorf("Feishu.AppID = %q, want %q", cfg.Channels.Feishu.AppID, "cli_xxx")
	}
}

func TestLoadYAMLFile(t *testing.T) {
	clearProviderEnv(t)

	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "config.yaml")

	yaml := `
agent:
  provider: ark
  model: doubao-pro-256k
  maxStep: 50
  contextWindowTokens: 131072
providers:
  ark:
    apiKey: test-ak
    apiBase: https://ark.cn-beijing.volces.com/api/v3
gateway:
  heartbeat:
    enabled: false
    interval: 1h
tools:
  mcp:
    - name: test-server
      command: npx
      args: ["-y", "test-mcp"]
`
	if err := os.WriteFile(cfgFile, []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(cfgFile)
	if err != nil {
		t.Fatalf("Load(%q) error: %v", cfgFile, err)
	}

	if cfg.Agent.Provider != "ark" {
		t.Errorf("Agent.Provider = %q, want %q", cfg.Agent.Provider, "ark")
	}
	if cfg.Agent.Model != "doubao-pro-256k" {
		t.Errorf("Agent.Model = %q, want %q", cfg.Agent.Model, "doubao-pro-256k")
	}
	if p, ok := cfg.Providers["ark"]; !ok || p.APIKey != "test-ak" {
		t.Errorf("Providers[ark] invalid: %+v (ok=%v)", p, ok)
	}
	if cfg.Agent.MaxStep != 50 {
		t.Errorf("Agent.MaxStep = %d, want %d", cfg.Agent.MaxStep, 50)
	}
	if cfg.Agent.ContextWindowTokens != 131072 {
		t.Errorf("ContextWindowTokens = %d, want %d", cfg.Agent.ContextWindowTokens, 131072)
	}
	if cfg.Gateway.Heartbeat.IsEnabled() {
		t.Error("Heartbeat should be disabled")
	}
	if cfg.Gateway.Heartbeat.Interval.Duration != 1*time.Hour {
		t.Errorf("Heartbeat.Interval = %v, want %v", cfg.Gateway.Heartbeat.Interval.Duration, 1*time.Hour)
	}
	wantPromptDir := filepath.Join(DefaultConfigDir(), "prompts")
	if cfg.Agent.PromptDir != wantPromptDir {
		t.Errorf("Agent.PromptDir = %q, want default %q", cfg.Agent.PromptDir, wantPromptDir)
	}
	if len(cfg.Tools.MCP) != 1 || cfg.Tools.MCP[0].Name != "test-server" {
		t.Errorf("Tools.MCP = %+v, want 1 server named 'test-server'", cfg.Tools.MCP)
	}
}

func TestLoadJSONFile(t *testing.T) {
	clearProviderEnv(t)

	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "config.json")

	content := `{
  "agent": {
    "provider": "ollama",
    "model": "llama3"
  },
  "providers": {
    "ollama": {
      "apiBase": "http://localhost:11434"
    }
  },
  "tools": {
    "exec": {
      "timeout": "120s",
      "maxOutput": 20000,
      "pathAppend": "/usr/local/bin"
    }
  }
}`
	if err := os.WriteFile(cfgFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(cfgFile)
	if err != nil {
		t.Fatalf("Load(%q) error: %v", cfgFile, err)
	}

	if cfg.Agent.Provider != "ollama" {
		t.Errorf("Agent.Provider = %q, want %q", cfg.Agent.Provider, "ollama")
	}
	if cfg.Tools.Exec.Timeout.Duration != 120*time.Second {
		t.Errorf("Exec.Timeout = %v, want %v", cfg.Tools.Exec.Timeout.Duration, 120*time.Second)
	}
	if cfg.Tools.Exec.MaxOutput != 20000 {
		t.Errorf("Exec.MaxOutput = %d, want %d", cfg.Tools.Exec.MaxOutput, 20000)
	}
	if cfg.Tools.Exec.PathAppend != "/usr/local/bin" {
		t.Errorf("Exec.PathAppend = %q, want %q", cfg.Tools.Exec.PathAppend, "/usr/local/bin")
	}
}

func TestLoadEnvOverridesFileValues(t *testing.T) {
	clearProviderEnv(t)

	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "config.yaml")

	yaml := `
agent:
  provider: ark
providers:
  ark:
    apiKey: file-key
`
	if err := os.WriteFile(cfgFile, []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("NANOBOT_PROVIDER", "openai")

	cfg, err := Load(cfgFile)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.Agent.Provider != "openai" {
		t.Errorf("Agent.Provider = %q, want env override %q", cfg.Agent.Provider, "openai")
	}
	if p, ok := cfg.Providers["ark"]; !ok || p.APIKey != "file-key" {
		t.Errorf("Providers[ark] invalid after env override: %+v (ok=%v)", p, ok)
	}
}

func TestLoadExplicitPathNotFound(t *testing.T) {
	_, err := Load("/nonexistent/path/config.yaml")
	if err == nil {
		t.Error("Load() should error for explicit nonexistent path")
	}
}

func TestSaveAndLoad(t *testing.T) {
	clearProviderEnv(t)

	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "saved.json")

	original := DefaultConfig()
	original.Agent.Provider = "qwen"
	original.Agent.Model = "qwen-max"
	original.Providers = map[string]ProviderConfig{
		"qwen": {APIKey: "my-key"},
	}
	original.Agent.MaxStep = 30

	if err := Save(cfgFile, original); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	loaded, err := Load(cfgFile)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if loaded.Agent.Provider != "qwen" {
		t.Errorf("Agent.Provider = %q, want %q", loaded.Agent.Provider, "qwen")
	}
	if p, ok := loaded.Providers["qwen"]; !ok || p.APIKey != "my-key" {
		t.Errorf("Providers[qwen] invalid: %+v (ok=%v)", p, ok)
	}
	if loaded.Agent.MaxStep != 30 {
		t.Errorf("Agent.MaxStep = %d, want %d", loaded.Agent.MaxStep, 30)
	}
}
