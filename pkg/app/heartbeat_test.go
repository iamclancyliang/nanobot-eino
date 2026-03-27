package app

import (
	"context"
	"testing"
	"time"

	"github.com/wall/nanobot-eino/pkg/bus"
	"github.com/wall/nanobot-eino/pkg/config"
)

func TestStartHeartbeatService_DisabledReturnsNil(t *testing.T) {
	enabled := false
	cfg := config.HeartbeatConfig{
		Enabled: &enabled,
	}
	svc := StartHeartbeatService(context.Background(), cfg, nil, bus.NewMessageBus())
	if svc != nil {
		t.Fatalf("expected nil heartbeat service when disabled, got %+v", svc)
	}
}

func TestStartHeartbeatService_EnabledReturnsService(t *testing.T) {
	enabled := true
	cfg := config.HeartbeatConfig{
		Enabled:  &enabled,
		Path:     "HEARTBEAT.md",
		Interval: config.Duration{Duration: time.Hour},
	}
	svc := StartHeartbeatService(context.Background(), cfg, nil, bus.NewMessageBus())
	if svc == nil {
		t.Fatal("expected heartbeat service when enabled")
	}
	svc.Stop()
}
