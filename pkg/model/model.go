package model

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/cloudwego/eino-ext/components/model/ark"
	"github.com/cloudwego/eino-ext/components/model/claude"
	"github.com/cloudwego/eino-ext/components/model/deepseek"
	"github.com/cloudwego/eino-ext/components/model/gemini"
	"github.com/cloudwego/eino-ext/components/model/ollama"
	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino-ext/components/model/openrouter"
	"github.com/cloudwego/eino-ext/components/model/qianfan"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"google.golang.org/genai"
)

type Config struct {
	Type            string // openai, qwen, siliconflow, ollama, ark, google, gemini, claude, deepseek, openrouter, qianfan
	BaseURL         string
	APIKey          string
	APISecret       string
	Model           string
	MaxTokens       int
	Temperature     float64
	ReasoningEffort string
}

func NewChatModel(ctx context.Context, cfg Config) (model.ChatModel, error) {
	switch cfg.Type {
	case "openai", "qwen", "siliconflow", "silicon-flow":
		// Qwen and SiliconFlow are compatible with OpenAI API.
		oaiCfg := &openai.ChatModelConfig{
			BaseURL: cfg.BaseURL,
			APIKey:  cfg.APIKey,
			Model:   cfg.Model,
		}
		base := strings.ToLower(strings.TrimSpace(cfg.BaseURL))
		if strings.Contains(base, ".openai.azure.com") {
			oaiCfg.ByAzure = true
			oaiCfg.APIVersion = "2024-08-01-preview"
		}
		return openai.NewChatModel(ctx, oaiCfg)
	case "ollama":
		return ollama.NewChatModel(ctx, &ollama.ChatModelConfig{
			BaseURL: cfg.BaseURL,
			Model:   cfg.Model,
		})
	case "ark":
		return ark.NewChatModel(ctx, &ark.ChatModelConfig{
			BaseURL: cfg.BaseURL,
			APIKey:  cfg.APIKey,
			Model:   cfg.Model,
		})
	case "google", "gemini":
		client, err := genai.NewClient(ctx, &genai.ClientConfig{
			APIKey: cfg.APIKey,
		})
		if err != nil {
			return nil, fmt.Errorf("create google genai client: %w", err)
		}
		return gemini.NewChatModel(ctx, &gemini.Config{
			Client: client,
			Model:  cfg.Model,
		})
	case "claude":
		maxTokens := cfg.MaxTokens
		if maxTokens <= 0 {
			maxTokens = 8192
		}
		return claude.NewChatModel(ctx, &claude.Config{
			APIKey:    cfg.APIKey,
			Model:     cfg.Model,
			MaxTokens: maxTokens,
			BaseURL:   toStringPtr(cfg.BaseURL),
		})
	case "deepseek":
		dsCfg := &deepseek.ChatModelConfig{
			APIKey: cfg.APIKey,
			Model:  cfg.Model,
		}
		if cfg.BaseURL != "" {
			dsCfg.BaseURL = cfg.BaseURL
		}
		return deepseek.NewChatModel(ctx, dsCfg)
	case "openrouter":
		orCfg := &openrouter.Config{
			APIKey: cfg.APIKey,
			Model:  cfg.Model,
		}
		if cfg.BaseURL != "" {
			orCfg.BaseURL = cfg.BaseURL
		}
		orModel, err := openrouter.NewChatModel(ctx, orCfg)
		if err != nil {
			return nil, err
		}
		return &openRouterCompat{inner: orModel}, nil
	case "qianfan":
		if strings.TrimSpace(cfg.APIKey) == "" || strings.TrimSpace(cfg.APISecret) == "" {
			return nil, fmt.Errorf("qianfan requires both apiKey and apiSecret")
		}
		qfCfg := qianfan.GetQianfanSingletonConfig()
		qfCfg.AccessKey = cfg.APIKey
		qfCfg.SecretKey = cfg.APISecret
		return qianfan.NewChatModel(ctx, &qianfan.ChatModelConfig{
			Model: cfg.Model,
		})
	default:
		return nil, fmt.Errorf("unsupported model type: %s", cfg.Type)
	}
}

// GetDefaultConfig returns config from environment variables.
// Prefer config.Load() for production use, which supports JSON config files with env var overrides.
func GetDefaultConfig() Config {
	return Config{
		Type:    getEnv("NANOBOT_MODEL_TYPE", "openai"),
		BaseURL: getEnv("NANOBOT_MODEL_BASE_URL", ""),
		APIKey:  getEnv("NANOBOT_MODEL_API_KEY", ""),
		Model:   getEnv("NANOBOT_MODEL_NAME", "gpt-4o"),
	}
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func toStringPtr(s string) *string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return &s
}

// openrouter.ChatModel currently does not implement the deprecated BindTools method
// required by eino's ChatModel interface; this adapter bridges that gap.
type openRouterCompat struct {
	inner *openrouter.ChatModel
}

func (o *openRouterCompat) Generate(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	return o.inner.Generate(ctx, input, opts...)
}

func (o *openRouterCompat) Stream(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	return o.inner.Stream(ctx, input, opts...)
}

func (o *openRouterCompat) BindTools(tools []*schema.ToolInfo) error {
	next, err := o.inner.WithTools(tools)
	if err != nil {
		return err
	}
	typed, ok := next.(*openrouter.ChatModel)
	if !ok {
		return fmt.Errorf("openrouter WithTools returned unexpected type %T", next)
	}
	o.inner = typed
	return nil
}
