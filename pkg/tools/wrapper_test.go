package tools

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

type mockTool struct {
	name   string
	result string
	err    error
}

func (m *mockTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{Name: m.name}, nil
}

func (m *mockTool) InvokableRun(_ context.Context, _ string, _ ...tool.Option) (string, error) {
	return m.result, m.err
}

func TestWrapTools_Truncation(t *testing.T) {
	longResult := strings.Repeat("x", 200)
	inner := &mockTool{name: "test_tool", result: longResult}

	wrapped := WrapTools([]tool.InvokableTool{inner}, 100, nil)
	if len(wrapped) != 1 {
		t.Fatalf("expected 1 wrapped tool, got %d", len(wrapped))
	}

	result, err := wrapped[0].InvokableRun(context.Background(), "{}")
	if err != nil {
		t.Fatalf("InvokableRun error: %v", err)
	}

	if !strings.Contains(result, "truncated") {
		t.Error("result should indicate truncation")
	}
	if !strings.Contains(result, "100 of 200") {
		t.Errorf("truncation message should show sizes, got: %s", result)
	}
}

func TestWrapTools_NoTruncation(t *testing.T) {
	inner := &mockTool{name: "test_tool", result: "short result"}

	wrapped := WrapTools([]tool.InvokableTool{inner}, 100, nil)
	result, _ := wrapped[0].InvokableRun(context.Background(), "{}")

	if result != "short result" {
		t.Errorf("short result should pass through unchanged, got %q", result)
	}
}

func TestWrapTools_ProgressCallback(t *testing.T) {
	inner := &mockTool{name: "my_tool", result: "ok"}
	var events []string

	progress := func(_ context.Context, toolName, status string) {
		events = append(events, toolName+":"+status)
	}

	wrapped := WrapTools([]tool.InvokableTool{inner}, 0, progress)
	wrapped[0].InvokableRun(context.Background(), "{}")

	if len(events) != 2 {
		t.Fatalf("expected 2 progress events, got %d: %v", len(events), events)
	}
	if events[0] != "my_tool:running" {
		t.Errorf("first event = %q, want %q", events[0], "my_tool:running")
	}
	if events[1] != "my_tool:completed" {
		t.Errorf("second event = %q, want %q", events[1], "my_tool:completed")
	}
}

func TestWrapTools_ProgressCallback_OnError(t *testing.T) {
	inner := &mockTool{name: "fail_tool", result: "", err: fmt.Errorf("boom")}
	var events []string

	progress := func(_ context.Context, toolName, status string) {
		events = append(events, toolName+":"+status)
	}

	wrapped := WrapTools([]tool.InvokableTool{inner}, 0, progress)
	result, err := wrapped[0].InvokableRun(context.Background(), "{}")
	if err != nil {
		t.Fatalf("expected wrapped tool to downgrade errors, got err=%v", err)
	}
	if !strings.Contains(result, "Error executing fail_tool: boom") {
		t.Fatalf("unexpected downgraded error result: %q", result)
	}
	if !strings.Contains(result, "[Analyze the error above and try a different approach.]") {
		t.Fatalf("missing retry hint in downgraded error result: %q", result)
	}

	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if events[1] != "fail_tool:failed" {
		t.Errorf("error event = %q, want %q", events[1], "fail_tool:failed")
	}
}

func TestWrapTools_AppendsHintForErrorStringResult(t *testing.T) {
	inner := &mockTool{name: "web_fetch", result: "Error: request timeout"}

	wrapped := WrapTools([]tool.InvokableTool{inner}, 0, nil)
	result, err := wrapped[0].InvokableRun(context.Background(), "{}")
	if err != nil {
		t.Fatalf("InvokableRun error: %v", err)
	}
	if !strings.Contains(result, "Error: request timeout") {
		t.Fatalf("expected original error result to be preserved, got: %q", result)
	}
	if !strings.Contains(result, "[Analyze the error above and try a different approach.]") {
		t.Fatalf("missing retry hint for error-like tool result: %q", result)
	}
}

func TestWrapTools_InfoPassthrough(t *testing.T) {
	inner := &mockTool{name: "passthrough_tool", result: "ok"}

	wrapped := WrapTools([]tool.InvokableTool{inner}, 0, nil)
	info, err := wrapped[0].Info(context.Background())
	if err != nil {
		t.Fatalf("Info error: %v", err)
	}
	if info.Name != "passthrough_tool" {
		t.Errorf("Info.Name = %q, want %q", info.Name, "passthrough_tool")
	}
}

func TestWrapTools_ZeroMaxChars_NoTruncation(t *testing.T) {
	longResult := strings.Repeat("x", 100000)
	inner := &mockTool{name: "big_tool", result: longResult}

	wrapped := WrapTools([]tool.InvokableTool{inner}, 0, nil)
	result, _ := wrapped[0].InvokableRun(context.Background(), "{}")

	if len(result) != 100000 {
		t.Errorf("with maxChars=0, result should not be truncated, got len=%d", len(result))
	}
}
