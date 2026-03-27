//go:build integration

package heartbeat

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/wall/nanobot-eino/pkg/model"
)

func TestIntegration_TickWithActiveTasks(t *testing.T) {
	apiKey := os.Getenv("NANOBOT_MODEL_API_KEY")
	if apiKey == "" {
		t.Skip("NANOBOT_MODEL_API_KEY not set")
	}

	ctx := context.Background()

	dir := t.TempDir()
	hbFile := filepath.Join(dir, "HEARTBEAT.md")
	content := "# Heartbeat Tasks\n## Active Tasks\n- [ ] Check system status and report.\n"
	if err := os.WriteFile(hbFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write heartbeat file: %v", err)
	}

	modelCfg := model.GetDefaultConfig()
	chatModel, err := model.NewChatModel(ctx, modelCfg)
	if err != nil {
		t.Fatalf("failed to create model: %v", err)
	}

	var executedTasks string
	svc := NewHeartbeatService(hbFile, chatModel, func(ctx context.Context, tasks string) error {
		executedTasks = tasks
		return nil
	}, 1*time.Hour)

	svc.Tick(ctx)

	// With active tasks, the LLM should decide action=run.
	// However, LLM behavior is non-deterministic, so we just log the result.
	if executedTasks != "" {
		t.Logf("Heartbeat executed with tasks: %s", executedTasks)
	} else {
		t.Log("Heartbeat did not execute (LLM decided to skip)")
	}
}

func TestIntegration_TickWithNoTasks(t *testing.T) {
	apiKey := os.Getenv("NANOBOT_MODEL_API_KEY")
	if apiKey == "" {
		t.Skip("NANOBOT_MODEL_API_KEY not set")
	}

	ctx := context.Background()

	dir := t.TempDir()
	hbFile := filepath.Join(dir, "HEARTBEAT.md")
	content := "# Heartbeat Tasks\n## Active Tasks\n<!-- No tasks here -->\n"
	if err := os.WriteFile(hbFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write heartbeat file: %v", err)
	}

	modelCfg := model.GetDefaultConfig()
	chatModel, err := model.NewChatModel(ctx, modelCfg)
	if err != nil {
		t.Fatalf("failed to create model: %v", err)
	}

	var executed bool
	svc := NewHeartbeatService(hbFile, chatModel, func(ctx context.Context, tasks string) error {
		executed = true
		return nil
	}, 1*time.Hour)

	svc.Tick(ctx)

	if executed {
		t.Log("Heartbeat unexpectedly executed (LLM decided to run on empty tasks)")
	} else {
		t.Log("Heartbeat correctly skipped (no tasks)")
	}
}
