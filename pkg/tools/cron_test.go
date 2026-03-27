package tools

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wall/nanobot-eino/pkg/cron"
)

func newTestCronService(t *testing.T) *cron.CronService {
	t.Helper()
	storePath := filepath.Join(t.TempDir(), "jobs.json")
	svc := cron.NewCronService(storePath, func(ctx context.Context, job *cron.CronJob) error {
		return nil
	})
	if err := svc.Start(context.Background()); err != nil {
		t.Fatalf("CronService.Start failed: %v", err)
	}
	t.Cleanup(svc.Stop)
	return svc
}

func TestCronTool_AddJobEvery(t *testing.T) {
	svc := newTestCronService(t)
	cronTool, err := NewCronTool(svc, "feishu", "chat1")
	if err != nil {
		t.Fatalf("NewCronTool error: %v", err)
	}

	args, _ := json.Marshal(CronArgs{
		Action:       "add",
		Message:      "check status",
		EverySeconds: 60,
	})
	result, err := cronTool.InvokableRun(context.Background(), string(args))
	if err != nil {
		t.Fatalf("InvokableRun error: %v", err)
	}

	if !strings.Contains(result, "Created job") {
		t.Errorf("expected success, got: %s", result)
	}
}

func TestCronTool_AddJobCron(t *testing.T) {
	svc := newTestCronService(t)
	cronTool, _ := NewCronTool(svc, "feishu", "chat1")

	args, _ := json.Marshal(CronArgs{
		Action:   "add",
		Message:  "daily check",
		CronExpr: "0 9 * * *",
		TZ:       "Asia/Shanghai",
	})
	result, err := cronTool.InvokableRun(context.Background(), string(args))
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	if !strings.Contains(result, "Created job") {
		t.Errorf("expected success, got: %s", result)
	}
}

func TestCronTool_AddJobAt(t *testing.T) {
	svc := newTestCronService(t)
	cronTool, _ := NewCronTool(svc, "feishu", "chat1")

	args, _ := json.Marshal(CronArgs{
		Action:  "add",
		Message: "one-time reminder",
		At:      "2099-12-31T23:59:59",
	})
	result, _ := cronTool.InvokableRun(context.Background(), string(args))

	if !strings.Contains(result, "Created job") {
		t.Errorf("expected success, got: %s", result)
	}
}

func TestCronTool_AddJobNoMessage(t *testing.T) {
	svc := newTestCronService(t)
	cronTool, _ := NewCronTool(svc, "feishu", "chat1")

	args, _ := json.Marshal(CronArgs{
		Action:       "add",
		EverySeconds: 60,
	})
	_, err := cronTool.InvokableRun(context.Background(), string(args))
	if err == nil {
		t.Error("should error when message is empty")
	}
}

func TestCronTool_AddJobNoSchedule(t *testing.T) {
	svc := newTestCronService(t)
	cronTool, _ := NewCronTool(svc, "feishu", "chat1")

	args, _ := json.Marshal(CronArgs{
		Action:  "add",
		Message: "no schedule",
	})
	_, err := cronTool.InvokableRun(context.Background(), string(args))
	if err == nil {
		t.Error("should error when no schedule provided")
	}
}

func TestCronTool_ListJobs(t *testing.T) {
	svc := newTestCronService(t)
	cronTool, _ := NewCronTool(svc, "feishu", "chat1")

	addArgs, _ := json.Marshal(CronArgs{
		Action:       "add",
		Message:      "test job",
		EverySeconds: 300,
	})
	cronTool.InvokableRun(context.Background(), string(addArgs))

	listArgs, _ := json.Marshal(CronArgs{Action: "list"})
	result, _ := cronTool.InvokableRun(context.Background(), string(listArgs))

	if !strings.Contains(result, "test job") {
		t.Errorf("list should contain job, got: %s", result)
	}
}

func TestCronTool_ListJobsEmpty(t *testing.T) {
	svc := newTestCronService(t)
	cronTool, _ := NewCronTool(svc, "feishu", "chat1")

	args, _ := json.Marshal(CronArgs{Action: "list"})
	result, _ := cronTool.InvokableRun(context.Background(), string(args))

	if !strings.Contains(result, "No scheduled jobs") {
		t.Errorf("expected 'No scheduled jobs', got: %s", result)
	}
}

func TestCronTool_RemoveJob(t *testing.T) {
	svc := newTestCronService(t)
	cronTool, _ := NewCronTool(svc, "feishu", "chat1")

	addArgs, _ := json.Marshal(CronArgs{
		Action:       "add",
		Message:      "removable",
		EverySeconds: 300,
	})
	addResult, _ := cronTool.InvokableRun(context.Background(), string(addArgs))

	// Extract job ID from "Created job 'removable' (id: xxx)"
	parts := strings.Split(addResult, "id: ")
	if len(parts) < 2 {
		t.Fatalf("cannot extract job ID from: %s", addResult)
	}
	jobID := strings.TrimSuffix(parts[1], ")")

	removeArgs, _ := json.Marshal(CronArgs{Action: "remove", JobID: jobID})
	result, _ := cronTool.InvokableRun(context.Background(), string(removeArgs))

	if !strings.Contains(result, "Removed job") {
		t.Errorf("expected removal success, got: %s", result)
	}
}

func TestCronTool_RemoveNonexistent(t *testing.T) {
	svc := newTestCronService(t)
	cronTool, _ := NewCronTool(svc, "feishu", "chat1")

	args, _ := json.Marshal(CronArgs{Action: "remove", JobID: "nonexistent-id"})
	result, _ := cronTool.InvokableRun(context.Background(), string(args))

	if !strings.Contains(result, "not found") {
		t.Errorf("expected 'not found', got: %s", result)
	}
}

func TestCronTool_RemoveNoJobID(t *testing.T) {
	svc := newTestCronService(t)
	cronTool, _ := NewCronTool(svc, "feishu", "chat1")

	args, _ := json.Marshal(CronArgs{Action: "remove"})
	result, _ := cronTool.InvokableRun(context.Background(), string(args))

	if !strings.Contains(result, "job_id is required") {
		t.Errorf("expected error about missing job_id, got: %s", result)
	}
}

func TestCronTool_BlocksAddFromCronContext(t *testing.T) {
	svc := newTestCronService(t)
	cronTool, _ := NewCronTool(svc, "feishu", "chat1")

	ctx := WithCronContext(context.Background())
	args, _ := json.Marshal(CronArgs{
		Action:       "add",
		Message:      "nested job",
		EverySeconds: 60,
	})
	result, _ := cronTool.InvokableRun(ctx, string(args))

	if !strings.Contains(result, "cannot schedule") {
		t.Errorf("should block add from cron context, got: %s", result)
	}
}

func TestCronTool_InvalidTimezone(t *testing.T) {
	svc := newTestCronService(t)
	cronTool, _ := NewCronTool(svc, "feishu", "chat1")

	args, _ := json.Marshal(CronArgs{
		Action:   "add",
		Message:  "tz test",
		CronExpr: "0 9 * * *",
		TZ:       "Invalid/Timezone",
	})
	result, _ := cronTool.InvokableRun(context.Background(), string(args))

	if !strings.Contains(result, "unknown timezone") {
		t.Errorf("should report unknown timezone, got: %s", result)
	}
}

func TestCronTool_TZWithoutCronExpr(t *testing.T) {
	svc := newTestCronService(t)
	cronTool, _ := NewCronTool(svc, "feishu", "chat1")

	args, _ := json.Marshal(CronArgs{
		Action:       "add",
		Message:      "tz without cron",
		EverySeconds: 60,
		TZ:           "Asia/Shanghai",
	})
	result, _ := cronTool.InvokableRun(context.Background(), string(args))

	if !strings.Contains(result, "tz can only be used with cron_expr") {
		t.Errorf("should error when tz used without cron_expr, got: %s", result)
	}
}

func TestCronTool_NoChannel(t *testing.T) {
	svc := newTestCronService(t)
	cronTool, _ := NewCronTool(svc, "", "")

	args, _ := json.Marshal(CronArgs{
		Action:       "add",
		Message:      "test",
		EverySeconds: 60,
	})
	result, _ := cronTool.InvokableRun(context.Background(), string(args))

	if !strings.Contains(result, "no session context") {
		t.Errorf("should error with no channel, got: %s", result)
	}
}

func TestCronTool_UsesProgressContextWhenDefaultChatMissing(t *testing.T) {
	svc := newTestCronService(t)
	cronTool, _ := NewCronTool(svc, "feishu", "")

	args, _ := json.Marshal(CronArgs{
		Action:       "add",
		Message:      "progress context job",
		EverySeconds: 60,
	})
	ctx := ContextWithProgressInfo(context.Background(), "feishu", "oc_progress_chat")
	result, _ := cronTool.InvokableRun(ctx, string(args))

	if !strings.Contains(result, "Created job") {
		t.Fatalf("expected job creation, got: %s", result)
	}

	jobs := svc.ListJobs()
	if len(jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(jobs))
	}
	if jobs[0].Payload.Channel != "feishu" {
		t.Fatalf("payload channel = %q, want feishu", jobs[0].Payload.Channel)
	}
	if jobs[0].Payload.To != "oc_progress_chat" {
		t.Fatalf("payload to = %q, want oc_progress_chat", jobs[0].Payload.To)
	}
}

func TestCronTool_ToolInfo(t *testing.T) {
	svc := newTestCronService(t)
	cronTool, _ := NewCronTool(svc, "feishu", "chat1")

	info, err := cronTool.Info(context.Background())
	if err != nil {
		t.Fatalf("Info error: %v", err)
	}
	if info.Name != "cron" {
		t.Errorf("tool name = %q, want %q", info.Name, "cron")
	}
}

func TestWithCronContext(t *testing.T) {
	ctx := context.Background()
	if isInCronContext(ctx) {
		t.Error("should not be in cron context initially")
	}

	cronCtx := WithCronContext(ctx)
	if !isInCronContext(cronCtx) {
		t.Error("should be in cron context")
	}
}
