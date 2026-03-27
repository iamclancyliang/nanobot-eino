package main

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/wall/nanobot-eino/pkg/agent"
	"github.com/wall/nanobot-eino/pkg/app"
	"github.com/wall/nanobot-eino/pkg/config"
	"github.com/wall/nanobot-eino/pkg/cron"
	"github.com/wall/nanobot-eino/pkg/memory"
	"github.com/wall/nanobot-eino/pkg/session"
	"github.com/wall/nanobot-eino/pkg/tools"
	"github.com/wall/nanobot-eino/pkg/workspace"
)

func main() {
	app.InitLogger()

	ctx := context.Background()

	cfg, err := config.Load(os.Getenv("NANOBOT_CONFIG"))
	if err != nil {
		slog.Error("Failed to load config", "error", err)
		os.Exit(1)
	}

	promptDir := cfg.ResolvePromptDir()
	skillsDir := cfg.ResolveSkillsDir()

	if err := workspace.SyncTemplates(promptDir); err != nil {
		slog.Warn("Prompt template sync failed", "error", err)
	}

	memStore, err := memory.NewMemoryStore(cfg.ResolveMemoryDir())
	if err != nil {
		slog.Error("Failed to init memory", "error", err)
		os.Exit(1)
	}

	sessionMgr, err := session.NewSessionManager(cfg.ResolveSessionsDir())
	if err != nil {
		slog.Error("Failed to init session manager", "error", err)
		os.Exit(1)
	}

	modelCfg := app.BuildModelConfig(cfg)
	if modelCfg.Type != "ollama" && modelCfg.APIKey == "" {
		slog.Warn("no API key configured for selected provider, model calls may fail", "provider", cfg.EffectiveProviderName())
	}

	toolCfg := tools.ToolConfig{
		Workspace: cfg.WorkspacePath(),
	}
	if cfg.Tools.Web.Search.Provider != "" {
		toolCfg.Web.Search = tools.WebSearchConfig{
			Provider:   cfg.Tools.Web.Search.Provider,
			APIKey:     cfg.Tools.Web.Search.APIKey,
			BaseURL:    cfg.Tools.Web.Search.BaseURL,
			MaxResults: cfg.Tools.Web.Search.MaxResults,
		}
	} else {
		toolCfg.Web.Search = tools.WebSearchConfig{Provider: "duckduckgo"}
	}

	cronSvc := cron.NewCronService(cfg.ResolveCronStorePath(), func(ctx context.Context, job *cron.CronJob) error {
		slog.Info("Cron job executing", "module", "cli", "job", job.Name)
		return nil
	})
	if err := cronSvc.Start(ctx); err != nil {
		slog.Warn("Failed to start cron service", "error", err)
	}
	defer cronSvc.Stop()

	bot, err := agent.NewAgent(ctx, modelCfg, toolCfg, memStore, promptDir,
		skillsDir, cronSvc, sessionMgr, cfg.Agent.ContextWindowTokens, cfg.Agent.MaxStep, nil)
	if err != nil {
		slog.Error("Failed to create agent", "error", err)
		os.Exit(1)
	}

	fmt.Println("Nanobot Eino CLI (type 'exit' to quit)")
	scanner := bufio.NewScanner(os.Stdin)
	sessionID := "cli:user-1"

	for {
		fmt.Print("> ")
		if !scanner.Scan() {
			break
		}
		input := scanner.Text()
		if strings.ToLower(input) == "exit" {
			break
		}

		reader, err := bot.ChatStream(ctx, sessionID, input)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			continue
		}

		fmt.Print("Bot: ")
		for {
			msg, err := reader.Recv()
			if err != nil {
				break
			}
			fmt.Print(msg.Content)
		}
		fmt.Println()
		reader.Close()
	}
}
