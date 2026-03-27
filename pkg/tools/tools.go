package tools

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/cloudwego/eino/components/tool"
)

var logTools = slog.With("module", "tools")

// ToolConfig mirrors nanobot's ToolsConfig schema.
type ToolConfig struct {
	Workspace string

	Web struct {
		Proxy  string
		Search WebSearchConfig
	}

	Exec ShellConfig

	RestrictToWorkspace bool
	ExtraReadDirs       []string

	MCP []MCPServerConfig

	// OnMessage, when non-nil, enables the message tool.
	// Callers supply a closure wrapping their message delivery mechanism
	// (e.g. bus.PublishOutbound), keeping pkg/tools free of bus dependencies.
	OnMessage      SendMessageFunc
	DefaultChannel string
	DefaultChatID  string
}

func NewTools(ctx context.Context, cfg ToolConfig) ([]tool.InvokableTool, error) {
	var tools []tool.InvokableTool

	workspace := cfg.Workspace
	var allowedDir string
	if cfg.RestrictToWorkspace && workspace != "" {
		allowedDir = workspace
	}

	// 1. Web Search
	if cfg.Web.Search.Provider != "" {
		tools = append(tools, NewWebSearchTool(cfg.Web.Search))
	}

	// 2. Web Fetch
	tools = append(tools, NewWebFetchTool(WebFetchConfig{
		MaxChars: 50000,
		Proxy:    cfg.Web.Proxy,
	}))

	// 3. Filesystem Tools
	tools = append(tools, NewReadFileTool(workspace, allowedDir, cfg.ExtraReadDirs...))
	tools = append(tools, NewWriteFileTool(workspace, allowedDir))
	tools = append(tools, NewEditFileTool(workspace, allowedDir))
	tools = append(tools, NewListDirTool(workspace, allowedDir))

	// 4. Message Tool
	if cfg.OnMessage != nil {
		tools = append(tools, NewMessageTool(cfg.OnMessage, cfg.DefaultChannel, cfg.DefaultChatID))
	}

	// 5. Shell Tool
	shellCfg := cfg.Exec
	if shellCfg.Timeout == 0 {
		shellCfg.Timeout = 60 * time.Second
	}
	if shellCfg.MaxOutput == 0 {
		shellCfg.MaxOutput = 10000
	}
	shellCfg.RestrictToWorkspace = cfg.RestrictToWorkspace
	if shellCfg.WorkingDir == "" {
		shellCfg.WorkingDir = workspace
	}
	shellTool, err := NewShellTool(shellCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create shell tool: %w", err)
	}
	tools = append(tools, shellTool)

	// NOTE: cfg.MCP is intentionally not processed here.
	// In the agent path, NewAgent extracts cfg.MCP before calling NewTools and
	// connects MCP servers lazily via ensureMCPConnected on the first message.
	// cfg.MCP remains in ToolConfig solely as a transport field so callers can
	// pass MCP configuration through to NewAgent without a separate parameter.

	return tools, nil
}
