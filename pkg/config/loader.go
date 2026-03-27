package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/go-viper/mapstructure/v2"
	"github.com/spf13/viper"
)

// DefaultConfigDir returns ~/.nanobot-eino.
func DefaultConfigDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".nanobot-eino"
	}
	return filepath.Join(home, ".nanobot-eino")
}

// DefaultConfigPath returns ~/.nanobot-eino/config.yaml.
func DefaultConfigPath() string {
	return filepath.Join(DefaultConfigDir(), "config.yaml")
}

// DefaultConfig returns a Config where all paths default to ~/.nanobot-eino/.
func DefaultConfig() *Config {
	dataDir := DefaultConfigDir()
	return &Config{
		Agent: AgentConfig{
			PromptDir:           filepath.Join(dataDir, "prompts"),
			BuiltinSkillsDir:    filepath.Join(dataDir, "skills"),
			ContextWindowTokens: 65536,
			MaxStep:             50,
			MaxTokens:           8192,
			Temperature:         0.1,
			Provider:            "auto",
		},
		Gateway: GatewayConfig{
			Heartbeat: HeartbeatConfig{
				Path:     "HEARTBEAT.md",
				Interval: Duration{30 * time.Minute},
			},
		},
		Tools: ToolsConfig{
			Workspace: filepath.Join(dataDir, "workspace"),
			Web: WebConfig{
				Search: WebSearchConfig{
					Provider: "tavily",
				},
			},
		},
	}
}

// Load reads config from a file (YAML, JSON, or TOML) and applies environment variable overrides.
// If path is empty, it searches ~/.nanobot-eino/ and the current directory for config.{yaml,json,toml}.
// A missing file is not an error — defaults + env vars are used.
func Load(path string) (*Config, error) {
	v := viper.New()

	setDefaults(v)
	configureFile(v, path)
	bindEnvVars(v)

	if err := v.ReadInConfig(); err != nil {
		if path != "" {
			return nil, fmt.Errorf("failed to read config %s: %w", path, err)
		}
		if !isConfigNotFound(err) {
			return nil, fmt.Errorf("failed to read config: %w", err)
		}
	}

	if used := v.ConfigFileUsed(); used != "" {
		SetConfigPath(used)
	} else if path != "" {
		SetConfigPath(path)
	}

	cfg := &Config{}
	if err := v.Unmarshal(cfg, decoderOptions()...); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	migrateConfig(cfg)

	return cfg, nil
}

// Save writes the config as JSON, creating parent directories as needed.
func Save(path string, cfg *Config) error {
	if path == "" {
		path = DefaultConfigPath()
	} else {
		path = expandHome(path)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	return os.WriteFile(path, data, 0644)
}

func setDefaults(v *viper.Viper) {
	dataDir := DefaultConfigDir()

	v.SetDefault("agent.promptDir", filepath.Join(dataDir, "prompts"))
	v.SetDefault("agent.builtinSkillsDir", filepath.Join(dataDir, "skills"))
	v.SetDefault("agent.contextWindowTokens", 65536)
	v.SetDefault("agent.maxStep", 20)
	v.SetDefault("agent.maxTokens", 8192)
	v.SetDefault("agent.temperature", 0.1)
	v.SetDefault("agent.provider", "auto")

	v.SetDefault("channels.feishu.groupPolicy", "mention")
	v.SetDefault("channels.sendProgress", true)

	v.SetDefault("gateway.heartbeat.path", "HEARTBEAT.md")
	v.SetDefault("gateway.heartbeat.interval", "30m")

	v.SetDefault("tools.workspace", filepath.Join(dataDir, "workspace"))
	v.SetDefault("tools.web.search.provider", "tavily")
}

func configureFile(v *viper.Viper, path string) {
	if path != "" {
		v.SetConfigFile(expandHome(path))
		return
	}
	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath(DefaultConfigDir())
	v.AddConfigPath(".")
}

// bindEnvVars maps environment variables to config keys.
// NANOBOT_* for new-style, plus legacy FEISHU_* vars.
func bindEnvVars(v *viper.Viper) {
	_ = v.BindEnv("channels.feishu.appId", "FEISHU_APP_ID")
	_ = v.BindEnv("channels.feishu.appSecret", "FEISHU_APP_SECRET")
	_ = v.BindEnv("channels.feishu.verificationToken", "FEISHU_VERIFICATION_TOKEN")
	_ = v.BindEnv("channels.feishu.encryptKey", "FEISHU_ENCRYPT_KEY")

	_ = v.BindEnv("agent.provider", "NANOBOT_PROVIDER")
	_ = v.BindEnv("agent.model", "NANOBOT_AGENT_MODEL")
	_ = v.BindEnv("agent.maxTokens", "NANOBOT_MAX_TOKENS")
	_ = v.BindEnv("agent.temperature", "NANOBOT_TEMPERATURE")
	_ = v.BindEnv("agent.reasoningEffort", "NANOBOT_REASONING_EFFORT")

	_ = v.BindEnv("tools.workspace", "NANOBOT_WORKSPACE")
}

func decoderOptions() []viper.DecoderConfigOption {
	return []viper.DecoderConfigOption{
		func(dc *mapstructure.DecoderConfig) {
			dc.TagName = "json"
		},
		viper.DecodeHook(mapstructure.ComposeDecodeHookFunc(
			durationDecodeHook(),
			mapstructure.StringToTimeDurationHookFunc(),
			mapstructure.StringToSliceHookFunc(","),
		)),
	}
}

func durationDecodeHook() mapstructure.DecodeHookFuncType {
	return func(from reflect.Type, to reflect.Type, data any) (any, error) {
		if to != reflect.TypeOf(Duration{}) {
			return data, nil
		}
		switch v := data.(type) {
		case string:
			if v == "" {
				return Duration{}, nil
			}
			d, err := time.ParseDuration(v)
			if err != nil {
				return nil, fmt.Errorf("invalid duration %q: %w", v, err)
			}
			return Duration{d}, nil
		case int:
			return Duration{time.Duration(v) * time.Second}, nil
		case int64:
			return Duration{time.Duration(v) * time.Second}, nil
		case float64:
			return Duration{time.Duration(v * float64(time.Second))}, nil
		default:
			return data, nil
		}
	}
}

func isConfigNotFound(err error) bool {
	_, ok := err.(viper.ConfigFileNotFoundError)
	return ok || os.IsNotExist(err)
}

func expandHome(path string) string {
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}

// migrateConfig applies backward-compatible transformations to a loaded config.
func migrateConfig(cfg *Config) {
	dataDir := DefaultConfigDir()

	// Migrate legacy relative data paths → absolute under ~/.nanobot-eino/.
	if cfg.Data.Dir == "data" || cfg.Data.Dir == "" {
		cfg.Data.Dir = filepath.Join(dataDir, "sessions")
	}
	if cfg.Data.MemoryDir == "data/memory" || cfg.Data.MemoryDir == "" {
		cfg.Data.MemoryDir = filepath.Join(dataDir, "memory")
	}
	if cfg.Gateway.Cron.StorePath == "data/jobs.json" || cfg.Gateway.Cron.StorePath == "" {
		cfg.Gateway.Cron.StorePath = filepath.Join(dataDir, "cron", "jobs.json")
	}
	if cfg.Tools.Workspace == "data" {
		cfg.Tools.Workspace = filepath.Join(dataDir, "workspace")
	}
	if cfg.Agent.PromptDir == "configs/prompts" {
		cfg.Agent.PromptDir = filepath.Join(dataDir, "prompts")
	}
	if cfg.Agent.BuiltinSkillsDir == "configs/skills" {
		cfg.Agent.BuiltinSkillsDir = filepath.Join(dataDir, "skills")
	}
}
