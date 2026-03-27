package config

import "testing"

func TestFindProviderByName(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{"openai", "openai"},
		{"deepseek", "deepseek"},
		{"ollama", "ollama"},
		{"Ark", "ark"},
		{"gemini", "gemini"},
		{"nonexistent", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := FindProviderByName(tt.name)
			if tt.want == "" {
				if spec != nil {
					t.Errorf("FindProviderByName(%q) = %v, want nil", tt.name, spec)
				}
				return
			}
			if spec == nil || spec.Name != tt.want {
				t.Errorf("FindProviderByName(%q) = %v, want name=%q", tt.name, spec, tt.want)
			}
		})
	}
}

func TestMatchProvider_ExplicitProvider(t *testing.T) {
	cfg := &Config{
		Agent: AgentConfig{Provider: "deepseek"},
		Providers: map[string]ProviderConfig{
			"deepseek": {APIKey: "sk-deepseek"},
		},
	}
	spec, p := cfg.MatchProvider("gpt-4o")
	if spec == nil || spec.Name != "deepseek" {
		t.Fatalf("expected deepseek provider, got %v", spec)
	}
	if spec.EinoType != "deepseek" {
		t.Fatalf("deepseek EinoType = %q, want %q", spec.EinoType, "deepseek")
	}
	if p == nil || p.APIKey != "sk-deepseek" {
		t.Errorf("expected deepseek config, got %v", p)
	}
}

func TestMatchProvider_ModelPrefix(t *testing.T) {
	cfg := &Config{
		Agent: AgentConfig{Provider: "auto", Model: "deepseek/deepseek-chat"},
		Providers: map[string]ProviderConfig{
			"deepseek": {APIKey: "sk-ds"},
		},
	}
	spec, _ := cfg.MatchProvider("")
	if spec == nil || spec.Name != "deepseek" {
		t.Errorf("expected deepseek from prefix, got %v", spec)
	}
}

func TestMatchProvider_KeywordMatch(t *testing.T) {
	cfg := &Config{
		Agent: AgentConfig{Provider: "auto", Model: "claude-3-opus"},
		Providers: map[string]ProviderConfig{
			"anthropic": {APIKey: "sk-ant"},
		},
	}
	spec, _ := cfg.MatchProvider("")
	if spec == nil || spec.Name != "anthropic" {
		t.Errorf("expected anthropic from keyword, got %v", spec)
	}
}

func TestMatchProvider_LocalFallback(t *testing.T) {
	cfg := &Config{
		Agent: AgentConfig{Provider: "auto", Model: "llama3.2"},
		Providers: map[string]ProviderConfig{
			"ollama": {APIBase: "http://localhost:11434"},
		},
	}
	spec, _ := cfg.MatchProvider("")
	if spec == nil || spec.Name != "ollama" {
		t.Errorf("expected ollama local fallback, got %v", spec)
	}
}

func TestMatchProvider_FirstAPIKey(t *testing.T) {
	cfg := &Config{
		Agent: AgentConfig{Provider: "auto", Model: "some-unknown-model"},
		Providers: map[string]ProviderConfig{
			"openai": {APIKey: "sk-openai"},
		},
	}
	spec, p := cfg.MatchProvider("")
	if spec == nil || p == nil || p.APIKey != "sk-openai" {
		t.Errorf("expected openai fallback, got spec=%v p=%v", spec, p)
	}
}

func TestMatchProvider_NoProviderConfigured(t *testing.T) {
	cfg := &Config{
		Agent: AgentConfig{Provider: "auto"},
	}
	spec, p := cfg.MatchProvider("")
	if spec != nil || p != nil {
		t.Fatalf("expected nil match without configured providers, got spec=%v p=%v", spec, p)
	}
}

func TestGetProvider(t *testing.T) {
	cfg := &Config{
		Providers: map[string]ProviderConfig{
			"openai": {APIKey: "sk-test", APIBase: "https://api.openai.com/v1"},
		},
	}
	p, ok := cfg.GetProvider("openai")
	if !ok {
		t.Fatal("GetProvider should find provider config")
	}
	if p.APIKey != "sk-test" {
		t.Errorf("APIKey = %q, want %q", p.APIKey, "sk-test")
	}
}

func TestEffectiveProviderName(t *testing.T) {
	tests := []struct {
		agent AgentConfig
		want  string
	}{
		{AgentConfig{Provider: "deepseek"}, "deepseek"},
		{AgentConfig{Provider: "auto"}, "openai"},
		{AgentConfig{}, "openai"},
	}
	for _, tt := range tests {
		cfg := &Config{Agent: tt.agent}
		got := cfg.EffectiveProviderName()
		if got != tt.want {
			t.Errorf("EffectiveProviderName() = %q, want %q (agent=%+v)", got, tt.want, tt.agent)
		}
	}
}

func TestFindGateway(t *testing.T) {
	spec := FindGateway("openrouter", "", "")
	if spec == nil || spec.Name != "openrouter" {
		t.Errorf("FindGateway by name failed, got %v", spec)
	}

	spec = FindGateway("", "", "http://localhost:11434/v1")
	if spec == nil || spec.Name != "ollama" {
		t.Errorf("FindGateway by base URL failed, got %v", spec)
	}

	spec = FindGateway("", "", "https://api.openai.com/v1")
	if spec != nil {
		t.Errorf("FindGateway should return nil for non-gateway, got %v", spec)
	}
}

func TestProviderRegistry_NewParityEntries(t *testing.T) {
	tests := []struct {
		name     string
		einoType string
	}{
		{name: "anthropic", einoType: "claude"},
		{name: "deepseek", einoType: "deepseek"},
		{name: "openrouter", einoType: "openrouter"},
		{name: "qianfan", einoType: "qianfan"},
		{name: "azure_openai", einoType: "openai"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := FindProviderByName(tt.name)
			if spec == nil {
				t.Fatalf("provider %q not found", tt.name)
			}
			if spec.EinoType != tt.einoType {
				t.Fatalf("provider %q EinoType = %q, want %q", tt.name, spec.EinoType, tt.einoType)
			}
		})
	}
}
