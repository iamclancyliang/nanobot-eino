package tools

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/wall/nanobot-eino/pkg/cron"
)

type cronContextKey struct{}

// WithCronContext marks ctx as running inside a cron job execution. The
// cron tool refuses to schedule new jobs in this state to prevent recursion.
func WithCronContext(ctx context.Context) context.Context {
	return context.WithValue(ctx, cronContextKey{}, true)
}

func isInCronContext(ctx context.Context) bool {
	v, _ := ctx.Value(cronContextKey{}).(bool)
	return v
}

// CronArgs are the arguments accepted by the cron tool.
type CronArgs struct {
	Action       string `json:"action" jsonschema:"enum=add,list,remove,description=Action to perform. For remove, always call list first to decide target job IDs."`
	Message      string `json:"message,omitempty" jsonschema:"description=Reminder message used when action=add"`
	EverySeconds int    `json:"every_seconds,omitempty" jsonschema:"description=Recurring interval in seconds when action=add"`
	CronExpr     string `json:"cron_expr,omitempty" jsonschema:"description=Cron expression when action=add, e.g. '0 9 * * *'"`
	TZ           string `json:"tz,omitempty" jsonschema:"description=IANA timezone for cron_expr, e.g. 'America/Vancouver'"`
	At           string `json:"at,omitempty" jsonschema:"description=One-time ISO datetime when action=add, e.g. '2026-03-16T10:30:00'"`
	JobID        string `json:"job_id,omitempty" jsonschema:"description=Exact job ID to remove. Must come from the latest list output; free-text intent is not accepted."`
}

// NewCronTool returns the "cron" tool bound to cronService. channel/chatID
// are the default delivery target when the agent does not provide one.
func NewCronTool(cronService *cron.CronService, channel, chatID string) (tool.InvokableTool, error) {
	return utils.InferTool("cron",
		"Schedule reminders and recurring tasks. For removals, you must call list first, then remove by exact job_id from list output.",
		func(ctx context.Context, args *CronArgs) (string, error) {
			switch args.Action {
			case "add":
				if isInCronContext(ctx) {
					return "Error: cannot schedule new jobs from within a cron job execution", nil
				}
				resolvedChannel, resolvedChatID := resolveCronTarget(ctx, channel, chatID)
				return cronAddJob(cronService, resolvedChannel, resolvedChatID, args)
			case "list":
				return cronListJobs(cronService), nil
			case "remove":
				return cronRemoveJob(cronService, args.JobID), nil
			default:
				return "", fmt.Errorf("unknown action: %s", args.Action)
			}
		})
}

func cronAddJob(svc *cron.CronService, channel, chatID string, args *CronArgs) (string, error) {
	if args.Message == "" {
		return "", fmt.Errorf("message is required for add")
	}
	if channel == "" || chatID == "" {
		return "Error: no session context (channel/chat_id)", nil
	}

	if args.TZ != "" && args.CronExpr == "" {
		return "Error: tz can only be used with cron_expr", nil
	}

	if args.TZ != "" {
		if _, err := time.LoadLocation(args.TZ); err != nil {
			return fmt.Sprintf("Error: unknown timezone '%s'", args.TZ), nil
		}
	}

	var schedule cron.CronSchedule
	deleteAfterRun := false

	switch {
	case args.EverySeconds > 0:
		schedule = cron.CronSchedule{
			Kind:    cron.JobKindEvery,
			EveryMs: int64(args.EverySeconds * 1000),
		}
	case args.CronExpr != "":
		schedule = cron.CronSchedule{
			Kind: cron.JobKindCron,
			Expr: args.CronExpr,
			TZ:   args.TZ,
		}
	case args.At != "":
		t, err := time.Parse(time.RFC3339, args.At)
		if err != nil {
			t, err = time.Parse("2006-01-02T15:04:05", args.At)
			if err != nil {
				return fmt.Sprintf(
					"Error: invalid ISO datetime format '%s'. Expected format: YYYY-MM-DDTHH:MM:SS",
					args.At,
				), nil
			}
		}
		schedule = cron.CronSchedule{
			Kind: cron.JobKindAt,
			AtMs: t.UnixMilli(),
		}
		deleteAfterRun = true
	default:
		return "", fmt.Errorf("either every_seconds, cron_expr or at is required")
	}

	payload := cron.CronPayload{
		Kind:    "agent_turn",
		Message: args.Message,
		Deliver: true,
		Channel: channel,
		To:      chatID,
	}

	name := args.Message
	if len(name) > 30 {
		name = name[:30]
	}

	job, err := svc.AddJob(name, schedule, payload, deleteAfterRun)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("Created job '%s' (id: %s)", job.Name, job.ID), nil
}

func resolveCronTarget(ctx context.Context, defaultChannel, defaultChatID string) (string, string) {
	channel := defaultChannel
	chatID := defaultChatID
	if pi := GetProgressInfo(ctx); pi != nil {
		if channel == "" {
			channel = pi.Channel
		}
		if chatID == "" {
			chatID = pi.ChatID
		}
	}
	return channel, chatID
}

func cronListJobs(svc *cron.CronService) string {
	jobs := svc.ListJobs()
	if len(jobs) == 0 {
		return "No scheduled jobs."
	}
	var sb strings.Builder
	sb.WriteString("Scheduled jobs:\n")
	for _, j := range jobs {
		sb.WriteString(fmt.Sprintf(
			"- id=%s kind=%s name=%q channel=%q to=%q message=%q\n",
			j.ID,
			j.Schedule.Kind,
			j.Name,
			j.Payload.Channel,
			j.Payload.To,
			j.Payload.Message,
		))
	}
	return sb.String()
}

func cronRemoveJob(svc *cron.CronService, jobID string) string {
	if jobID == "" {
		return "Error: job_id is required for remove"
	}
	removed := svc.RemoveJob(jobID)
	if removed {
		return fmt.Sprintf("Removed job %s", jobID)
	}
	return fmt.Sprintf("Job %s not found", jobID)
}
