package heartbeat

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

var logHeartbeat = slog.With("module", "heartbeat")

type HeartbeatAction string

const (
	ActionSkip HeartbeatAction = "skip"
	ActionRun  HeartbeatAction = "run"
)

type HeartbeatResult struct {
	Action HeartbeatAction `json:"action"`
	Tasks  string          `json:"tasks"`
}

type HeartbeatService struct {
	heartbeatPath string
	model         model.ChatModel
	onExecute     func(ctx context.Context, tasks string) error
	interval      time.Duration
	enabled       bool
	stopChan      chan struct{}
}

func NewHeartbeatService(path string, chatModel model.ChatModel, onExecute func(ctx context.Context, tasks string) error, interval time.Duration) *HeartbeatService {
	return &HeartbeatService{
		heartbeatPath: path,
		model:         chatModel,
		onExecute:     onExecute,
		interval:      interval,
		enabled:       true,
		stopChan:      make(chan struct{}),
	}
}

func (s *HeartbeatService) Start(ctx context.Context) {
	if !s.enabled {
		logHeartbeat.Info("Service disabled")
		return
	}

	logHeartbeat.Info("Service started", "interval", s.interval)
	ticker := time.NewTicker(s.interval)
	go func() {
		for {
			select {
			case <-ticker.C:
				s.Tick(context.Background())
			case <-s.stopChan:
				ticker.Stop()
				return
			}
		}
	}()
}

func (s *HeartbeatService) Stop() {
	close(s.stopChan)
}

func (s *HeartbeatService) Tick(ctx context.Context) {
	content, err := os.ReadFile(s.heartbeatPath)
	if err != nil {
		if !os.IsNotExist(err) {
			logHeartbeat.Warn("Failed to read file", "path", s.heartbeatPath, "error", err)
		}
		return
	}

	if len(content) == 0 {
		return
	}

	logHeartbeat.Debug("Checking for tasks...")

	action, tasks, err := s.decide(ctx, string(content))
	if err != nil {
		logHeartbeat.Warn("Decision failed", "error", err)
		return
	}

	if action != ActionRun {
		logHeartbeat.Debug("OK, nothing to report")
		return
	}

	logHeartbeat.Info("Tasks found, executing", "tasks", tasks)
	if s.onExecute != nil {
		if err := s.onExecute(ctx, tasks); err != nil {
			logHeartbeat.Warn("Execution failed", "error", err)
		}
	}
}

func (s *HeartbeatService) decide(ctx context.Context, content string) (HeartbeatAction, string, error) {
	heartbeatTool := &schema.ToolInfo{
		Name: "heartbeat",
		Desc: "Report heartbeat decision after reviewing tasks.",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"action": {
				Type: "string",
				Desc: "skip = nothing to do, run = has active tasks",
				Enum: []string{"skip", "run"},
			},
			"tasks": {
				Type: "string",
				Desc: "Natural-language summary of active tasks (required for run)",
			},
		}),
	}

	systemMsg := &schema.Message{
		Role:    schema.System,
		Content: "You are a heartbeat agent. Call the heartbeat tool to report your decision.",
	}

	userMsg := &schema.Message{
		Role: schema.User,
		Content: fmt.Sprintf("Current Time: %s\n\nReview the following HEARTBEAT.md and decide whether there are active tasks.\n\n%s",
			time.Now().Format(time.RFC3339), content),
	}

	// Invoke the model with the heartbeat tool. Eino normally binds tools via
	// model.BindTools; we pass schema.ToolInfo directly at call time and assume
	// the model supports tool calling.
	resp, err := s.model.Generate(ctx, []*schema.Message{systemMsg, userMsg}, model.WithTools([]*schema.ToolInfo{heartbeatTool}))
	if err != nil {
		return ActionSkip, "", err
	}

	if len(resp.ToolCalls) == 0 {
		return ActionSkip, "", nil
	}

	tc := resp.ToolCalls[0]
	if tc.Function.Name != "heartbeat" {
		return ActionSkip, "", nil
	}

	var result struct {
		Action string `json:"action"`
		Tasks  string `json:"tasks"`
	}
	if err := json.Unmarshal([]byte(tc.Function.Arguments), &result); err != nil {
		return ActionSkip, "", err
	}

	return HeartbeatAction(result.Action), result.Tasks, nil
}
