package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/wall/nanobot-eino/pkg/bus"
	"github.com/wall/nanobot-eino/pkg/cron"
)

type CronDispatchOptions struct {
	RequireChannel         bool
	RequireNonEmptyMessage bool
	EnableDeliver          bool
}

func BuildCronJobHandler(
	messageBus *bus.MessageBus,
	opts CronDispatchOptions,
) func(ctx context.Context, job *cron.CronJob) error {
	return func(ctx context.Context, job *cron.CronJob) error {
		logApp.Info("Cron job triggered",
			"job_id", job.ID, "message", job.Payload.Message)
		if opts.RequireChannel && job.Payload.Channel == "" {
			return fmt.Errorf("cron job %s has empty target channel", job.ID)
		}
		if job.Payload.Channel != "" && job.Payload.To == "" {
			return fmt.Errorf("cron job %s has empty target chat id for channel %s", job.ID, job.Payload.Channel)
		}
		if opts.RequireNonEmptyMessage && strings.TrimSpace(job.Payload.Message) == "" {
			return fmt.Errorf("cron job %s has empty message payload", job.ID)
		}
		if opts.EnableDeliver && job.Payload.Deliver {
			messageBus.PublishOutbound(ctx, &bus.OutboundMessage{
				Channel: job.Payload.Channel,
				ChatID:  job.Payload.To,
				Content: job.Payload.Message,
				Metadata: map[string]any{
					"cron_job_id": job.ID,
				},
			})
			return nil
		}
		messageBus.PublishInbound(ctx, &bus.InboundMessage{
			Channel:  job.Payload.Channel,
			ChatID:   job.Payload.To,
			Content:  job.Payload.Message,
			Metadata: map[string]any{"cron_job_id": job.ID},
		})
		return nil
	}
}
