package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

const ToolResultMaxChars = 16000
const toolFailureHint = "\n\n[Analyze the error above and try a different approach.]"

// ToolProgressFunc is called when a tool starts or finishes execution.
// The context carries session ID and progress info for routing.
type ToolProgressFunc func(ctx context.Context, toolName, status string)

type wrappedTool struct {
	inner      tool.InvokableTool
	maxChars   int
	onProgress ToolProgressFunc
}

// WrapTools wraps each tool with result truncation and progress reporting.
func WrapTools(invokableTools []tool.InvokableTool, maxChars int, onProgress ToolProgressFunc) []tool.InvokableTool {
	wrapped := make([]tool.InvokableTool, len(invokableTools))
	for i, t := range invokableTools {
		wrapped[i] = &wrappedTool{inner: t, maxChars: maxChars, onProgress: onProgress}
	}
	return wrapped
}

func (w *wrappedTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return w.inner.Info(ctx)
}

func (w *wrappedTool) InvokableRun(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
	info, _ := w.inner.Info(ctx)
	toolName := "unknown"
	if info != nil {
		toolName = info.Name
	}

	if w.onProgress != nil {
		w.onProgress(ctx, toolName, "running")
	}

	result, err := w.inner.InvokableRun(ctx, argumentsInJSON, opts...)

	failed := err != nil || isToolErrorResult(result)
	if w.onProgress != nil {
		status := "completed"
		if failed {
			status = "failed"
		}
		w.onProgress(ctx, toolName, status)
	}

	if err != nil {
		// Normalize execution errors into tool-result text so the next LLM step
		// can recover by trying different parameters or tools.
		result = fmt.Sprintf("Error executing %s: %v", toolName, err)
		err = nil
	}
	if isToolErrorResult(result) {
		result = appendToolFailureHint(result)
	}

	if w.maxChars > 0 && len(result) > w.maxChars {
		originalLen := len(result)
		result = result[:w.maxChars] + fmt.Sprintf("\n\n... (truncated, showing %d of %d characters)", w.maxChars, originalLen)
		logTools.Debug("Tool result truncated", "tool", toolName, "original", originalLen, "truncated", w.maxChars)
	}

	return result, nil
}

func isToolErrorResult(result string) bool {
	return strings.HasPrefix(strings.TrimSpace(result), "Error")
}

func appendToolFailureHint(result string) string {
	if strings.Contains(result, toolFailureHint) {
		return result
	}
	return result + toolFailureHint
}
