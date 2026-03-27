package main

import (
	"context"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/wall/nanobot-eino/pkg/agent"
	"github.com/wall/nanobot-eino/pkg/app"
	"github.com/wall/nanobot-eino/pkg/bus"
	"github.com/wall/nanobot-eino/pkg/config"
	"github.com/wall/nanobot-eino/pkg/cron"
	"github.com/wall/nanobot-eino/pkg/memory"
	"github.com/wall/nanobot-eino/pkg/model"
	"github.com/wall/nanobot-eino/pkg/session"
	"github.com/wall/nanobot-eino/pkg/subagent"
	"github.com/wall/nanobot-eino/pkg/tools"
	"github.com/wall/nanobot-eino/pkg/workspace"
)

const shutdownTimeout = 15 * time.Second
const componentStopTimeout = 5 * time.Second

func main() {
	app.InitLogger()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := app.NewSignalChannel()

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
		slog.Error("Failed to init memory store", "error", err)
		os.Exit(1)
	}

	sessionMgr, err := session.NewSessionManager(cfg.ResolveSessionsDir())
	if err != nil {
		slog.Error("Failed to init session manager", "error", err)
		os.Exit(1)
	}

	messageBus := bus.NewMessageBus()

	modelCfg := app.BuildModelConfig(cfg)
	chatModel, err := model.NewChatModel(ctx, modelCfg)
	if err != nil {
		slog.Error("Failed to create model", "error", err)
		os.Exit(1)
	}

	cronService := cron.NewCronService(
		cfg.ResolveCronStorePath(),
		app.BuildCronJobHandler(messageBus, app.CronDispatchOptions{
			RequireChannel:         true,
			RequireNonEmptyMessage: true,
			EnableDeliver:          true,
		}),
	)
	cronService.Start(ctx)

	heartbeatService := app.StartHeartbeatService(ctx, cfg.Gateway.Heartbeat, chatModel, messageBus)

	toolCfg := app.BuildToolConfig(cfg, messageBus, "feishu")
	subagentMgr := subagent.NewSubagentManager(chatModel, toolCfg, messageBus, cfg.Agent.MaxStep)
	bot, err := agent.NewAgent(ctx, modelCfg, toolCfg, memStore, promptDir,
		skillsDir, cronService, sessionMgr, cfg.Agent.ContextWindowTokens, cfg.Agent.MaxStep, subagentMgr)
	if err != nil {
		slog.Error("Failed to create agent", "error", err)
		os.Exit(1)
	}

	bot.OnProgress = app.NewProgressHandler(messageBus)

	feishuCfg := app.BuildFeishuConfig(cfg)

	feishuChannel := app.StartFeishuChannel(
		ctx,
		feishuCfg,
		messageBus,
		func() { slog.Info("FEISHU_APP_ID not set, skipping Feishu channel") },
	)

	slog.Info("Nanobot Server started")

	// Track in-flight requests so shutdown can wait for them.
	var wg sync.WaitGroup
	var shutdownDone <-chan struct{}

	shutdownDone = app.StartGracefulShutdown(app.GracefulShutdownConfig{
		SigCh:              sigCh,
		CancelRoot:         cancel,
		CancelAgentTasks:   bot,
		CancelSubagentTask: subagentMgr,
		CloseInbound:       messageBus.Close,
		WaitGroup:          &wg,
		ShutdownTimeout:    shutdownTimeout,
		Components: app.RuntimeComponents{
			Feishu:               feishuChannel,
			Heartbeat:            heartbeatService,
			Cron:                 cronService,
			CloseMCP:             tools.CloseMCPConnections,
			ComponentStopTimeout: componentStopTimeout,
		},
		CompleteMessage: "Shutdown complete",
	})

	app.RunInboundLoop(ctx, messageBus, bot, &wg)

	// If the for-range exits naturally (bus closed), wait for stragglers.
	wg.Wait()
	// Close outbound AFTER all workers finish so in-flight publishes succeed.
	messageBus.CloseOutbound()
	<-shutdownDone
	_ = chatModel
}
