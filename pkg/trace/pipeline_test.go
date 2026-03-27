package trace

import (
	"context"
	"sync"
	"testing"

	"github.com/cloudwego/eino/callbacks"
	"github.com/cloudwego/eino/components"
)

// TestGlobalHandlerRegistration verifies that AppendGlobalHandlers actually
// populates the global handlers list and that they fire during OnStart.
func TestGlobalHandlerRegistration(t *testing.T) {
	var mu sync.Mutex
	var called bool

	handler := callbacks.NewHandlerBuilder().
		OnStartFn(func(ctx context.Context, info *callbacks.RunInfo, input callbacks.CallbackInput) context.Context {
			mu.Lock()
			called = true
			mu.Unlock()
			t.Logf("OnStart called: name=%s component=%v", info.Name, info.Component)
			return ctx
		}).
		Build()

	callbacks.AppendGlobalHandlers(handler)

	// Simulate what the compose graph does: InitCallbacks + OnStart
	ctx := context.Background()
	info := &callbacks.RunInfo{
		Name:      "test-chat",
		Type:      "chat_model",
		Component: components.ComponentOfChatModel,
	}
	ctx = callbacks.InitCallbacks(ctx, info)
	ctx = callbacks.OnStart(ctx, map[string]any{"test": true})
	_ = ctx

	mu.Lock()
	wasCalled := called
	mu.Unlock()

	if !wasCalled {
		t.Fatal("Global handler OnStart was NOT called — callbacks pipeline is broken")
	}
	t.Log("Global handler OnStart was called successfully")
}
