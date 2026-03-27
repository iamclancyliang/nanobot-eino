package app

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"reflect"
	"sync"
	"syscall"
	"time"
)

// isNilInterface returns true for nil or typed-nil interface values.
func isNilInterface(v any) bool {
	if v == nil {
		return true
	}
	rv := reflect.ValueOf(v)
	return rv.Kind() == reflect.Ptr && rv.IsNil()
}

type cancelAller interface {
	CancelAll() int
}

type feishuStopper interface {
	Stop(ctx context.Context) error
}

type heartbeatStopper interface {
	Stop()
}

type cronStopper interface {
	Stop()
}

type RuntimeComponents struct {
	Feishu               feishuStopper
	Heartbeat            heartbeatStopper
	Cron                 cronStopper
	CloseMCP             func() int
	ComponentStopTimeout time.Duration
}

func StopRuntimeComponents(components RuntimeComponents) {
	if !isNilInterface(components.Feishu) {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), components.ComponentStopTimeout)
		if err := components.Feishu.Stop(stopCtx); err != nil {
			logApp.Warn("Feishu channel stop warning", "error", err)
		}
		stopCancel()
		logApp.Info("Feishu channel stopped")
	}
	if !isNilInterface(components.Heartbeat) {
		components.Heartbeat.Stop()
		logApp.Info("Heartbeat service stopped")
	}
	if !isNilInterface(components.Cron) {
		components.Cron.Stop()
		logApp.Info("Cron service stopped")
	}
	if components.CloseMCP != nil {
		closed := components.CloseMCP()
		logApp.Info("MCP connections closed", "count", closed)
	}
}

type GracefulShutdownConfig struct {
	SigCh <-chan os.Signal

	CancelRoot         func()
	CancelAgentTasks   cancelAller
	CancelSubagentTask cancelAller
	CloseInbound       func()

	WaitGroup       *sync.WaitGroup
	ShutdownTimeout time.Duration
	Components      RuntimeComponents

	CompleteMessage string
}

func NewSignalChannel() chan os.Signal {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	return sigCh
}

func StartGracefulShutdown(cfg GracefulShutdownConfig) <-chan struct{} {
	shutdownDone := make(chan struct{})
	go func() {
		defer close(shutdownDone)
		sig := <-cfg.SigCh
		slog.Info("Received signal, initiating graceful shutdown",
			"module", "app", "signal", sig.String())

		if cfg.CancelRoot != nil {
			cfg.CancelRoot()
		}

		agentCancelled := 0
		if cfg.CancelAgentTasks != nil {
			agentCancelled = cfg.CancelAgentTasks.CancelAll()
		}
		subCancelled := 0
		if cfg.CancelSubagentTask != nil {
			subCancelled = cfg.CancelSubagentTask.CancelAll()
		}
		logApp.Info("Tasks cancelled",
			"agent_tasks", agentCancelled, "subagent_tasks", subCancelled)

		if cfg.CloseInbound != nil {
			cfg.CloseInbound()
		}

		done := make(chan struct{})
		go func() {
			if cfg.WaitGroup != nil {
				cfg.WaitGroup.Wait()
			}
			close(done)
		}()

		select {
		case <-done:
			logApp.Info("All in-flight requests completed")
		case <-time.After(cfg.ShutdownTimeout):
			logApp.Warn("Shutdown timed out, forcing exit",
				"timeout", cfg.ShutdownTimeout)
		}

		StopRuntimeComponents(cfg.Components)

		if cfg.CompleteMessage != "" {
			logApp.Info(cfg.CompleteMessage)
		}
	}()
	return shutdownDone
}
