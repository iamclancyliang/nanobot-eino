package config

import "testing"

func TestEffectiveModel_AgentModelPriority(t *testing.T) {
	cfg := &Config{
		Agent: AgentConfig{Model: "deepseek-r1"},
	}
	if got := cfg.EffectiveModel(); got != "deepseek-r1" {
		t.Errorf("EffectiveModel() = %q, want %q (Agent.Model must have priority)", got, "deepseek-r1")
	}
}

func TestEffectiveProviderName_AgentProviderPriority(t *testing.T) {
	cfg := &Config{
		Agent: AgentConfig{Provider: "deepseek"},
	}
	if got := cfg.EffectiveProviderName(); got != "deepseek" {
		t.Errorf("EffectiveProviderName() = %q, want %q", got, "deepseek")
	}
}

func TestEffectiveProviderName_DefaultsToOpenAI(t *testing.T) {
	cfg := &Config{}
	if got := cfg.EffectiveProviderName(); got != "openai" {
		t.Errorf("EffectiveProviderName() = %q, want default %q", got, "openai")
	}
}

func TestEffectiveModel_DefaultsToGPT4o(t *testing.T) {
	cfg := &Config{}
	if got := cfg.EffectiveModel(); got != "gpt-4o" {
		t.Errorf("EffectiveModel() = %q, want default %q", got, "gpt-4o")
	}
}
