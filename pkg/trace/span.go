package trace

import (
	"context"
	"time"

	"github.com/cloudwego/eino/callbacks"
	"github.com/cloudwego/eino/components"
)

type spanKey struct{}

type spanData struct {
	name      string
	startTime time.Time
}

// StartSpan creates a manual span for non-Eino components (e.g., memory consolidation).
// The returned context carries span state for EndSpan.
func StartSpan(ctx context.Context, name string, input map[string]any) context.Context {
	info := &callbacks.RunInfo{
		Name:      name,
		Type:      "custom",
		Component: components.ComponentOfTool,
	}
	ctx = callbacks.InitCallbacks(ctx, info)
	ctx = callbacks.OnStart(ctx, input)
	return context.WithValue(ctx, spanKey{}, &spanData{
		name:      name,
		startTime: time.Now(),
	})
}

// EndSpan ends the current span. If err is non-nil, records the error.
func EndSpan(ctx context.Context, output map[string]any, err error) {
	if err != nil {
		callbacks.OnError(ctx, err)
		return
	}
	callbacks.OnEnd(ctx, output)
}
