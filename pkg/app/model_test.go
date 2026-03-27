package app

import (
	"testing"

	"github.com/wall/nanobot-eino/pkg/config"
)

func TestBuildModelConfig_MapsFields(t *testing.T) {
	cfg := &config.Config{
		Agent: config.AgentConfig{
			Provider: "openai",
			Model:    "gpt-4o",
		},
		Providers: map[string]config.ProviderConfig{
			"openai": {
				APIKey:  "k",
				APIBase: "https://api.openai.com/v1",
			},
		},
	}

	got := BuildModelConfig(cfg)
	if got.Type != "openai" ||
		got.BaseURL != "https://api.openai.com/v1" ||
		got.APIKey != "k" ||
		got.Model != "gpt-4o" {
		t.Fatalf("model config mapping mismatch: %+v", got)
	}
}

// TestBuildModelConfig_UsesProviders verifies that BuildModelConfig resolves
// type and credentials from the Providers map, not just legacy Model fields.
// RED: current code ignores Providers and returns empty APIKey.
func TestBuildModelConfig_UsesProviders(t *testing.T) {
	cfg := &config.Config{
		Agent: config.AgentConfig{Provider: "deepseek", Model: "deepseek-chat"},
		Providers: map[string]config.ProviderConfig{
			"deepseek": {APIKey: "sk-ds", APIBase: "https://api.deepseek.com/v1"},
		},
	}
	// credentials live only in Providers.

	got := BuildModelConfig(cfg)
	if got.APIKey != "sk-ds" {
		t.Errorf("APIKey = %q, want %q (should come from Providers)", got.APIKey, "sk-ds")
	}
	if got.Type != "deepseek" {
		t.Errorf("Type = %q, want %q (deepseek native EinoType)", got.Type, "deepseek")
	}
	if got.BaseURL != "https://api.deepseek.com/v1" {
		t.Errorf("BaseURL = %q, want %q", got.BaseURL, "https://api.deepseek.com/v1")
	}
	if got.Model != "deepseek-chat" {
		t.Errorf("Model = %q, want %q", got.Model, "deepseek-chat")
	}
}

func TestBuildModelConfig_PassesAgentAndProviderExtras(t *testing.T) {
	cfg := &config.Config{
		Agent: config.AgentConfig{
			Provider:        "qianfan",
			Model:           "ernie-4.0-8k",
			MaxTokens:       4096,
			Temperature:     0.2,
			ReasoningEffort: "high",
		},
		Providers: map[string]config.ProviderConfig{
			"qianfan": {
				APIKey:    "qf-ak",
				APISecret: "qf-sk",
			},
		},
	}

	got := BuildModelConfig(cfg)
	if got.Type != "qianfan" {
		t.Fatalf("Type = %q, want %q", got.Type, "qianfan")
	}
	if got.APISecret != "qf-sk" {
		t.Errorf("APISecret = %q, want %q", got.APISecret, "qf-sk")
	}
	if got.MaxTokens != 4096 {
		t.Errorf("MaxTokens = %d, want 4096", got.MaxTokens)
	}
	if got.Temperature != 0.2 {
		t.Errorf("Temperature = %v, want 0.2", got.Temperature)
	}
	if got.ReasoningEffort != "high" {
		t.Errorf("ReasoningEffort = %q, want %q", got.ReasoningEffort, "high")
	}
}

func TestBuildModelConfig_AgentModelApplied(t *testing.T) {
	cfg := &config.Config{
		Agent: config.AgentConfig{
			Provider: "deepseek",
			Model:    "deepseek-r1",
		},
		Providers: map[string]config.ProviderConfig{
			"deepseek": {APIKey: "sk-ds"},
		},
	}
	got := BuildModelConfig(cfg)
	if got.Model != "deepseek-r1" {
		t.Errorf("Model = %q, want %q", got.Model, "deepseek-r1")
	}
}

func TestBuildModelConfig_ForcedProviderWithoutCredentials(t *testing.T) {
	cfg := &config.Config{
		Agent: config.AgentConfig{
			Provider: "ark",
			Model:    "doubao-pro-256k",
		},
	}

	got := BuildModelConfig(cfg)
	if got.Type != "ark" {
		t.Errorf("Type = %q, want %q", got.Type, "ark")
	}
	if got.BaseURL != "https://ark.cn-beijing.volces.com/api/v3" {
		t.Errorf("BaseURL = %q, want default ark base URL", got.BaseURL)
	}
	if got.APIKey != "" {
		t.Errorf("APIKey = %q, want empty", got.APIKey)
	}
}
