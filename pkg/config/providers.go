package config

import "strings"

// ProviderSpec describes an LLM provider's metadata used for auto-detection
// and configuration matching. Modeled after nanobot's ProviderSpec.
type ProviderSpec struct {
	Name           string   // config key, e.g. "deepseek"
	DisplayName    string   // shown in status output
	Keywords       []string // model-name keywords for matching (lowercase)
	EinoType       string   // maps to model.Config.Type for eino SDK
	DefaultAPIBase string   // fallback base URL for this provider
	IsGateway      bool     // routes any model (e.g. OpenRouter, AiHubMix)
	IsLocal        bool     // local deployment (Ollama, vLLM)
	DetectByBase   string   // match substring in api_base URL for auto-detection
}

// Registry is the ordered list of known providers.
// Order matters: it controls match priority. Gateways and aggregators first.
var Registry = []ProviderSpec{
	{
		Name:           "openrouter",
		DisplayName:    "OpenRouter",
		Keywords:       []string{"openrouter"},
		EinoType:       "openrouter",
		DefaultAPIBase: "https://openrouter.ai/api/v1",
		IsGateway:      true,
	},
	{
		Name:           "aihubmix",
		DisplayName:    "AiHubMix",
		Keywords:       []string{"aihubmix"},
		EinoType:       "openai",
		DefaultAPIBase: "https://aihubmix.com/v1",
		IsGateway:      true,
		DetectByBase:   "aihubmix",
	},
	{
		Name:           "siliconflow",
		DisplayName:    "SiliconFlow",
		Keywords:       []string{"siliconflow", "silicon-flow"},
		EinoType:       "siliconflow",
		DefaultAPIBase: "https://api.siliconflow.cn/v1",
	},
	{
		Name:        "anthropic",
		DisplayName: "Anthropic",
		Keywords:    []string{"anthropic", "claude"},
		EinoType:    "claude",
	},
	{
		Name:         "azure_openai",
		DisplayName:  "Azure OpenAI",
		Keywords:     []string{"azure"},
		EinoType:     "openai",
		DetectByBase: ".openai.azure.com",
	},
	{
		Name:        "openai",
		DisplayName: "OpenAI",
		Keywords:    []string{"openai", "gpt", "o1", "o3", "o4"},
		EinoType:    "openai",
	},
	{
		Name:           "deepseek",
		DisplayName:    "DeepSeek",
		Keywords:       []string{"deepseek"},
		EinoType:       "deepseek",
		DefaultAPIBase: "https://api.deepseek.com/v1",
	},
	{
		Name:        "qianfan",
		DisplayName: "Baidu Qianfan (ERNIE)",
		Keywords:    []string{"qianfan", "ernie", "wenxin"},
		EinoType:    "qianfan",
	},
	{
		Name:        "dashscope",
		DisplayName: "DashScope (Qwen)",
		Keywords:    []string{"dashscope", "qwen"},
		EinoType:    "qwen",
	},
	{
		Name:           "ark",
		DisplayName:    "Volcengine Ark (Doubao)",
		Keywords:       []string{"ark", "volcengine", "doubao"},
		EinoType:       "ark",
		DefaultAPIBase: "https://ark.cn-beijing.volces.com/api/v3",
	},
	{
		Name:        "gemini",
		DisplayName: "Google Gemini",
		Keywords:    []string{"gemini", "google"},
		EinoType:    "gemini",
	},
	{
		Name:        "moonshot",
		DisplayName: "Moonshot (Kimi)",
		Keywords:    []string{"moonshot", "kimi"},
		EinoType:    "openai",
	},
	{
		Name:        "minimax",
		DisplayName: "MiniMax",
		Keywords:    []string{"minimax"},
		EinoType:    "openai",
	},
	{
		Name:        "zhipu",
		DisplayName: "Zhipu (GLM)",
		Keywords:    []string{"zhipu", "glm", "chatglm"},
		EinoType:    "openai",
	},
	{
		Name:        "groq",
		DisplayName: "Groq",
		Keywords:    []string{"groq"},
		EinoType:    "openai",
	},
	{
		Name:           "ollama",
		DisplayName:    "Ollama",
		Keywords:       []string{"ollama"},
		EinoType:       "ollama",
		DefaultAPIBase: "http://localhost:11434",
		IsLocal:        true,
		DetectByBase:   "11434",
	},
	{
		Name:        "vllm",
		DisplayName: "vLLM",
		Keywords:    []string{"vllm"},
		EinoType:    "openai",
		IsLocal:     true,
	},
}

// MatchProvider resolves the best ProviderSpec for the given model name.
// Priority:
//  1. Explicit provider name (non-"auto")
//  2. Model name prefix match (e.g. "deepseek/xxx")
//  3. Keyword match in model name
//  4. Local provider fallback (api_base substring)
//  5. First provider with an api_key in the config
func (c *Config) MatchProvider(model string) (*ProviderSpec, *ProviderConfig) {
	if model == "" {
		model = c.EffectiveModel()
	}

	forced := c.Agent.Provider
	if forced != "" && forced != "auto" {
		if spec := FindProviderByName(forced); spec != nil {
			if p, ok := c.Providers[spec.Name]; ok {
				return spec, &p
			}
			return spec, nil
		}
	}

	modelLower := strings.ToLower(model)
	modelNorm := strings.ReplaceAll(modelLower, "-", "_")

	var modelPrefix string
	if idx := strings.Index(modelLower, "/"); idx > 0 {
		modelPrefix = strings.ReplaceAll(modelLower[:idx], "-", "_")
	}

	kwMatch := func(kw string) bool {
		kw = strings.ToLower(kw)
		return strings.Contains(modelLower, kw) || strings.Contains(modelNorm, strings.ReplaceAll(kw, "-", "_"))
	}

	// 2. Prefix match
	if modelPrefix != "" {
		for i := range Registry {
			spec := &Registry[i]
			if spec.Name == modelPrefix {
				p, _ := c.Providers[spec.Name]
				if spec.IsLocal || p.APIKey != "" {
					return spec, &p
				}
			}
		}
	}

	// 3. Keyword match
	for i := range Registry {
		spec := &Registry[i]
		for _, kw := range spec.Keywords {
			if kwMatch(kw) {
				p, ok := c.Providers[spec.Name]
				if spec.IsLocal || (ok && p.APIKey != "") {
					return spec, &p
				}
			}
		}
	}

	// 4. Provider fallback via api_base detect substring
	for i := range Registry {
		spec := &Registry[i]
		if spec.DetectByBase == "" {
			continue
		}
		p, ok := c.Providers[spec.Name]
		if ok && p.APIBase != "" && strings.Contains(strings.ToLower(p.APIBase), strings.ToLower(spec.DetectByBase)) {
			return spec, &p
		}
	}

	// 5. First configured provider with an API key
	for i := range Registry {
		spec := &Registry[i]
		if p, ok := c.Providers[spec.Name]; ok && p.APIKey != "" {
			return spec, &p
		}
	}

	return nil, nil
}

// FindProviderByName looks up a ProviderSpec by its config key.
func FindProviderByName(name string) *ProviderSpec {
	name = strings.ToLower(strings.ReplaceAll(name, "-", "_"))
	for i := range Registry {
		if Registry[i].Name == name {
			return &Registry[i]
		}
	}
	return nil
}

// FindGateway detects a gateway/local provider from hints.
func FindGateway(providerName, apiKey, apiBase string) *ProviderSpec {
	if providerName != "" {
		spec := FindProviderByName(providerName)
		if spec != nil && (spec.IsGateway || spec.IsLocal) {
			return spec
		}
	}
	for i := range Registry {
		spec := &Registry[i]
		if spec.DetectByBase != "" && apiBase != "" && strings.Contains(apiBase, spec.DetectByBase) {
			return spec
		}
	}
	return nil
}
