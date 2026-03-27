package heartbeat

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

type mockChatModel struct {
	generateFn func(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.Message, error)
}

func (m *mockChatModel) Generate(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	if m.generateFn != nil {
		return m.generateFn(ctx, input, opts...)
	}
	return &schema.Message{Role: schema.Assistant, Content: "ok"}, nil
}

func (m *mockChatModel) Stream(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	return nil, nil
}

func (m *mockChatModel) BindTools(tools []*schema.ToolInfo) error {
	return nil
}

func TestHeartbeatService_TickSkipsWhenFileNotExists(t *testing.T) {
	var executed atomic.Bool
	svc := NewHeartbeatService(
		filepath.Join(t.TempDir(), "nonexistent.md"),
		&mockChatModel{},
		func(ctx context.Context, tasks string) error {
			executed.Store(true)
			return nil
		},
		time.Minute,
	)

	svc.Tick(context.Background())

	if executed.Load() {
		t.Error("should not execute when file doesn't exist")
	}
}

func TestHeartbeatService_TickSkipsWhenFileEmpty(t *testing.T) {
	dir := t.TempDir()
	hbFile := filepath.Join(dir, "HEARTBEAT.md")
	os.WriteFile(hbFile, []byte(""), 0644)

	var executed atomic.Bool
	svc := NewHeartbeatService(
		hbFile,
		&mockChatModel{},
		func(ctx context.Context, tasks string) error {
			executed.Store(true)
			return nil
		},
		time.Minute,
	)

	svc.Tick(context.Background())

	if executed.Load() {
		t.Error("should not execute when file is empty")
	}
}

func TestHeartbeatService_TickExecutesOnActionRun(t *testing.T) {
	dir := t.TempDir()
	hbFile := filepath.Join(dir, "HEARTBEAT.md")
	os.WriteFile(hbFile, []byte("Check weather at 9am"), 0644)

	var executedTasks string
	svc := NewHeartbeatService(
		hbFile,
		&mockChatModel{
			generateFn: func(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.Message, error) {
				return &schema.Message{
					Role: schema.Assistant,
					ToolCalls: []schema.ToolCall{
						{
							Function: schema.FunctionCall{
								Name:      "heartbeat",
								Arguments: `{"action":"run","tasks":"Check the weather"}`,
							},
						},
					},
				}, nil
			},
		},
		func(ctx context.Context, tasks string) error {
			executedTasks = tasks
			return nil
		},
		time.Minute,
	)

	svc.Tick(context.Background())

	if executedTasks != "Check the weather" {
		t.Errorf("executed tasks = %q, want %q", executedTasks, "Check the weather")
	}
}

func TestHeartbeatService_TickSkipsOnActionSkip(t *testing.T) {
	dir := t.TempDir()
	hbFile := filepath.Join(dir, "HEARTBEAT.md")
	os.WriteFile(hbFile, []byte("Nothing right now"), 0644)

	var executed atomic.Bool
	svc := NewHeartbeatService(
		hbFile,
		&mockChatModel{
			generateFn: func(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.Message, error) {
				return &schema.Message{
					Role: schema.Assistant,
					ToolCalls: []schema.ToolCall{
						{
							Function: schema.FunctionCall{
								Name:      "heartbeat",
								Arguments: `{"action":"skip","tasks":""}`,
							},
						},
					},
				}, nil
			},
		},
		func(ctx context.Context, tasks string) error {
			executed.Store(true)
			return nil
		},
		time.Minute,
	)

	svc.Tick(context.Background())

	if executed.Load() {
		t.Error("should not execute on action=skip")
	}
}

func TestHeartbeatService_StartStop(t *testing.T) {
	svc := NewHeartbeatService(
		filepath.Join(t.TempDir(), "HEARTBEAT.md"),
		&mockChatModel{},
		nil,
		time.Hour,
	)

	svc.Start(context.Background())
	svc.Stop()
}

func TestHeartbeatService_DisabledDoesNotStart(t *testing.T) {
	svc := NewHeartbeatService(
		filepath.Join(t.TempDir(), "HEARTBEAT.md"),
		&mockChatModel{},
		nil,
		time.Hour,
	)
	svc.enabled = false

	svc.Start(context.Background())
}

func TestHeartbeatResult_Actions(t *testing.T) {
	if ActionSkip != "skip" {
		t.Errorf("ActionSkip = %q, want %q", ActionSkip, "skip")
	}
	if ActionRun != "run" {
		t.Errorf("ActionRun = %q, want %q", ActionRun, "run")
	}
}
