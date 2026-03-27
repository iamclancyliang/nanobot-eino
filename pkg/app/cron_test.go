package app

import (
	"context"
	"testing"
	"time"

	"github.com/wall/nanobot-eino/pkg/bus"
	"github.com/wall/nanobot-eino/pkg/cron"
)

func TestBuildCronJobHandler_ServerValidationRejectsEmptyChannel(t *testing.T) {
	handler := BuildCronJobHandler(bus.NewMessageBus(), CronDispatchOptions{
		RequireChannel:        true,
		RequireNonEmptyMessage: true,
		EnableDeliver:         true,
	})

	err := handler(context.Background(), &cron.CronJob{
		ID: "j1",
		Payload: cron.CronPayload{
			Message: "hello",
			Channel: "",
			To:      "chat",
		},
	})
	if err == nil {
		t.Fatal("expected error for empty channel")
	}
}

func TestBuildCronJobHandler_RejectsEmptyTargetWhenChannelSet(t *testing.T) {
	handler := BuildCronJobHandler(bus.NewMessageBus(), CronDispatchOptions{})

	err := handler(context.Background(), &cron.CronJob{
		ID: "j2",
		Payload: cron.CronPayload{
			Message: "hello",
			Channel: "feishu",
			To:      "",
		},
	})
	if err == nil {
		t.Fatal("expected error for empty target chat id")
	}
}

func TestBuildCronJobHandler_ServerDeliverPublishesOutbound(t *testing.T) {
	messageBus := bus.NewMessageBus()
	handler := BuildCronJobHandler(messageBus, CronDispatchOptions{
		RequireChannel:        true,
		RequireNonEmptyMessage: true,
		EnableDeliver:         true,
	})

	err := handler(context.Background(), &cron.CronJob{
		ID: "j3",
		Payload: cron.CronPayload{
			Message: "deliver now",
			Deliver: true,
			Channel: "feishu",
			To:      "oc_123",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	select {
	case msg := <-messageBus.ConsumeOutbound(context.Background()):
		if msg.Channel != "feishu" || msg.ChatID != "oc_123" || msg.Content != "deliver now" {
			t.Fatalf("unexpected outbound message: %+v", msg)
		}
	case <-time.After(300 * time.Millisecond):
		t.Fatal("expected outbound message")
	}
}

func TestBuildCronJobHandler_GatewayAlwaysPublishesInbound(t *testing.T) {
	messageBus := bus.NewMessageBus()
	handler := BuildCronJobHandler(messageBus, CronDispatchOptions{
		RequireChannel:        false,
		RequireNonEmptyMessage: false,
		EnableDeliver:         false,
	})

	err := handler(context.Background(), &cron.CronJob{
		ID: "j4",
		Payload: cron.CronPayload{
			Message: "agent turn",
			Deliver: true,
			Channel: "",
			To:      "",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	select {
	case msg := <-messageBus.ConsumeInbound(context.Background()):
		if msg.Content != "agent turn" {
			t.Fatalf("unexpected inbound message: %+v", msg)
		}
	case <-time.After(300 * time.Millisecond):
		t.Fatal("expected inbound message")
	}
}
