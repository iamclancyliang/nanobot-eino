package app

import (
	"github.com/wall/nanobot-eino/pkg/config"
	"github.com/wall/nanobot-eino/pkg/model"
)

// BuildModelConfig resolves the effective model configuration by consulting
// MatchProvider, then applying EffectiveModel for the model name.
func BuildModelConfig(cfg *config.Config) model.Config {
	spec, provCfg := cfg.MatchProvider("")
	if spec == nil {
		effectiveName := cfg.EffectiveProviderName()
		if fallback := config.FindProviderByName(effectiveName); fallback != nil {
			spec = fallback
		} else {
			spec = &config.ProviderSpec{
				Name:     effectiveName,
				EinoType: effectiveName,
			}
		}
	}
	if provCfg == nil {
		empty := config.ProviderConfig{}
		provCfg = &empty
	}

	apiBase := provCfg.APIBase
	if apiBase == "" {
		apiBase = spec.DefaultAPIBase
	}
	return model.Config{
		Type:            spec.EinoType,
		BaseURL:         apiBase,
		APIKey:          provCfg.APIKey,
		APISecret:       provCfg.APISecret,
		Model:           cfg.EffectiveModel(),
		MaxTokens:       cfg.Agent.MaxTokens,
		Temperature:     cfg.Agent.Temperature,
		ReasoningEffort: cfg.Agent.ReasoningEffort,
	}
}
