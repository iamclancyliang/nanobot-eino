package main

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/spf13/cobra"
	"github.com/wall/nanobot-eino/pkg/agent"
	"github.com/wall/nanobot-eino/pkg/app"
	"github.com/wall/nanobot-eino/pkg/bus"
	"github.com/wall/nanobot-eino/pkg/cron"
	"github.com/wall/nanobot-eino/pkg/memory"
	"github.com/wall/nanobot-eino/pkg/model"
	"github.com/wall/nanobot-eino/pkg/session"
	"github.com/wall/nanobot-eino/pkg/subagent"
	"github.com/wall/nanobot-eino/pkg/tools"
	"github.com/wall/nanobot-eino/pkg/trace"
	"github.com/wall/nanobot-eino/pkg/workspace"
)

const gatewayShutdownTimeout = 15 * time.Second
const gatewayComponentStopTimeout = 5 * time.Second

func newGatewayCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "gateway",
		Short: "Start the full gateway server (channels + agent + heartbeat + cron)",
		RunE:  runGateway,
	}
}

func runGateway(cmd *cobra.Command, args []string) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := app.NewSignalChannel()

	cfg := mustLoadConfig()

	traceShutdown, err := trace.Init(cfg.Trace)
	if err != nil {
		return fmt.Errorf("init tracing: %w", err)
	}
	defer traceShutdown()

	promptDir := cfg.ResolvePromptDir()
	skillsDir := cfg.ResolveSkillsDir()

	if err := workspace.SyncTemplates(promptDir); err != nil {
		slog.Warn("Template sync failed", "error", err)
	}

	memStore, err := memory.NewMemoryStore(cfg.ResolveMemoryDir())
	if err != nil {
		return fmt.Errorf("init memory store: %w", err)
	}

	sessionMgr, err := session.NewSessionManager(cfg.ResolveSessionsDir())
	if err != nil {
		return fmt.Errorf("init session manager: %w", err)
	}

	messageBus := bus.NewMessageBus()

	modelCfg := app.BuildModelConfig(cfg)

	cronService := cron.NewCronService(
		cfg.ResolveCronStorePath(),
		app.BuildCronJobHandler(messageBus, app.CronDispatchOptions{
			RequireChannel:         false,
			RequireNonEmptyMessage: false,
			EnableDeliver:          false,
		}),
	)
	cronService.Start(ctx)

	chatModel, err := model.NewChatModel(ctx, modelCfg)
	if err != nil {
		return fmt.Errorf("create chat model: %w", err)
	}

	heartbeatService := app.StartHeartbeatService(ctx, cfg.Gateway.Heartbeat, chatModel, messageBus)

	toolCfg := app.BuildToolConfig(cfg, messageBus, "feishu")
	subagentMgr := subagent.NewSubagentManager(chatModel, toolCfg, messageBus, cfg.Agent.MaxStep)
	bot, err := agent.NewAgent(ctx, modelCfg, toolCfg, memStore, promptDir,
		skillsDir, cronService, sessionMgr, cfg.Agent.ContextWindowTokens, cfg.Agent.MaxStep, subagentMgr)
	if err != nil {
		return fmt.Errorf("create agent: %w", err)
	}

	bot.OnProgress = app.NewProgressHandler(messageBus)

	feishuCfg := app.BuildFeishuConfig(cfg)
	feishuChannel := app.StartFeishuChannel(
		ctx,
		feishuCfg,
		messageBus,
		func() { slog.Info("Feishu channel not configured, skipping") },
	)

	slog.Info("Gateway started, waiting for messages...")

	var wg sync.WaitGroup
	var shutdownDone <-chan struct{}

	shutdownDone = app.StartGracefulShutdown(app.GracefulShutdownConfig{
		SigCh:              sigCh,
		CancelRoot:         cancel,
		CancelAgentTasks:   bot,
		CancelSubagentTask: subagentMgr,
		CloseInbound:       messageBus.Close,
		WaitGroup:          &wg,
		ShutdownTimeout:    gatewayShutdownTimeout,
		Components: app.RuntimeComponents{
			Feishu:               feishuChannel,
			Heartbeat:            heartbeatService,
			Cron:                 cronService,
			CloseMCP:             tools.CloseMCPConnections,
			ComponentStopTimeout: gatewayComponentStopTimeout,
		},
		CompleteMessage: "Gateway shutdown complete",
	})

	app.RunInboundLoop(ctx, messageBus, bot, &wg)

	wg.Wait()
	// Close outbound AFTER all workers finish so in-flight publishes succeed.
	messageBus.CloseOutbound()
	<-shutdownDone
	return nil
}
