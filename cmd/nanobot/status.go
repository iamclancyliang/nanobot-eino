package main

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/wall/nanobot-eino/pkg/config"
)

func newStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show current configuration and service status",
		RunE:  runStatus,
	}
}

func runStatus(cmd *cobra.Command, args []string) error {
	cfg := mustLoadConfig()

	fmt.Println("=== Nanobot Eino Status ===")

	fmt.Println("Model:")
	fmt.Printf("  Provider:    %s\n", cfg.EffectiveProviderName())
	fmt.Printf("  Model:       %s\n", cfg.EffectiveModel())
	spec, prov := cfg.MatchProvider("")
	if spec != nil && prov != nil {
		base := prov.APIBase
		if base == "" {
			base = spec.DefaultAPIBase
		}
		if base != "" {
			fmt.Printf("  Base URL:    %s\n", base)
		}
	}
	fmt.Printf("  API Key:     %s\n", maskSecret(cfg.GetAPIKey(cfg.EffectiveProviderName())))

	if len(cfg.Providers) > 0 {
		fmt.Println("Providers:")
		for name, p := range cfg.Providers {
			spec := config.FindProviderByName(name)
			display := name
			if spec != nil && spec.DisplayName != "" {
				display = spec.DisplayName
			}
			status := "configured"
			if p.APIKey == "" {
				status = "no api key"
			}
			base := ""
			if p.APIBase != "" {
				base = " (base: " + p.APIBase + ")"
			}
			fmt.Printf("  %s: %s%s\n", display, status, base)
		}
	}

	if spec != nil {
		fmt.Printf("Auto-matched Provider: %s (eino type: %s)\n", spec.Name, spec.EinoType)
	}

	fmt.Println("Agent:")
	fmt.Printf("  Prompt Dir:      %s\n", cfg.ResolvePromptDir())
	fmt.Printf("  Skills Dir:      %s\n", cfg.ResolveSkillsDir())
	fmt.Printf("  Context Window:  %d tokens\n", cfg.Agent.ContextWindowTokens)
	fmt.Printf("  Max Steps:       %d\n", cfg.Agent.MaxStep)
	fmt.Printf("  Max Tokens:      %d\n", cfg.Agent.MaxTokens)
	fmt.Printf("  Temperature:     %.1f\n", cfg.Agent.Temperature)
	if cfg.Agent.ReasoningEffort != "" {
		fmt.Printf("  Reasoning:       %s\n", cfg.Agent.ReasoningEffort)
	}

	fmt.Println("Tools:")
	fmt.Printf("  Workspace:  %s\n", cfg.WorkspacePath())
	if cfg.Tools.Web.Search.Provider != "" {
		fmt.Printf("  Search:     %s\n", cfg.Tools.Web.Search.Provider)
	}
	if len(cfg.Tools.MCP) > 0 {
		fmt.Printf("  MCP (%d):\n", len(cfg.Tools.MCP))
		for _, m := range cfg.Tools.MCP {
			t := m.Type
			if t == "" {
				t = "stdio"
			}
			fmt.Printf("    - %s (%s)\n", m.Name, t)
		}
	}

	fmt.Println("Channels:")
	if cfg.Channels.Feishu.AppID != "" {
		fmt.Printf("  Feishu:  configured (app=%s)\n", maskSecret(cfg.Channels.Feishu.AppID))
	} else {
		fmt.Println("  Feishu:  not configured")
	}

	fmt.Println("Gateway:")
	fmt.Printf("  Heartbeat:  %v (interval %s)\n",
		cfg.Gateway.Heartbeat.IsEnabled(), cfg.Gateway.Heartbeat.Interval.Duration)
	fmt.Printf("  Cron Store: %s\n", cfg.ResolveCronStorePath())

	fmt.Println("Paths:")
	fmt.Printf("  Config:    %s\n", config.GetConfigPath())
	fmt.Printf("  Data Dir:  %s\n", config.GetDataDir())
	fmt.Printf("  Sessions:  %s\n", cfg.ResolveSessionsDir())
	fmt.Printf("  Memory:    %s\n", cfg.ResolveMemoryDir())
	fmt.Printf("  Logs:      %s\n", config.GetLogsDir())
	fmt.Printf("  History:   %s\n", config.GetCLIHistoryPath())

	return nil
}

func maskSecret(s string) string {
	if s == "" {
		return "(not set)"
	}
	if len(s) <= 8 {
		return "****"
	}
	return s[:4] + "****" + s[len(s)-4:]
}
