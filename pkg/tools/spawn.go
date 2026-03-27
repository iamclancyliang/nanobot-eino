package tools

import (
	"context"
	"fmt"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
)

// SubagentSpawner launches background subagent tasks.
// Implemented by subagent.SubagentManager; defined here to avoid circular imports.
type SubagentSpawner interface {
	Spawn(ctx context.Context, task, label, channel, chatID, sessionKey string) (taskID string, err error)
}

type SpawnArgs struct {
	Task  string `json:"task" jsonschema:"description=The task for the subagent to execute in the background"`
	Label string `json:"label,omitempty" jsonschema:"description=Short human-readable label for the task"`
}

func NewSpawnTool(spawner SubagentSpawner) tool.InvokableTool {
	t, _ := utils.InferTool(
		"spawn",
		"Spawn a background subagent to handle a self-contained subtask. "+
			"The subagent has filesystem, shell, and web tools. "+
			"Results are delivered asynchronously via a system message when done.",
		func(ctx context.Context, args *SpawnArgs) (string, error) {
			channel := ""
			chatID := ""
			if pi := GetProgressInfo(ctx); pi != nil {
				channel = pi.Channel
				chatID = pi.ChatID
			}
			sessionKey := SessionIDFromContext(ctx)

			if args.Task == "" {
				return "Error: task is required", nil
			}

			label := args.Label
			if label == "" {
				label = args.Task
				if len(label) > 60 {
					label = label[:60] + "..."
				}
			}

			taskID, err := spawner.Spawn(ctx, args.Task, label, channel, chatID, sessionKey)
			if err != nil {
				return "", fmt.Errorf("failed to spawn subagent: %w", err)
			}

			return fmt.Sprintf("Subagent spawned successfully.\nLabel: %s\nTask ID: %s\nThe result will be delivered as a system message when the task completes.", label, taskID), nil
		},
	)
	return t
}
