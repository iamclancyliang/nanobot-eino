package app

import (
	"context"
	"fmt"

	"github.com/wall/nanobot-eino/pkg/bus"
	"github.com/wall/nanobot-eino/pkg/config"
	"github.com/wall/nanobot-eino/pkg/tools"
)

func BuildToolConfig(cfg *config.Config, messageBus *bus.MessageBus, defaultChannel string) tools.ToolConfig {
	toolCfg := tools.ToolConfig{
		Workspace:           cfg.WorkspacePath(),
		RestrictToWorkspace: cfg.Tools.RestrictToWorkspace,
		ExtraReadDirs:       cfg.Tools.ExtraReadDirs,
		DefaultChannel:      defaultChannel,
		OnMessage: func(ctx context.Context, p tools.SendMessagePayload) {
			messageBus.PublishOutbound(ctx, &bus.OutboundMessage{
				Channel: p.Channel,
				ChatID:  p.ChatID,
				Content: p.Content,
				Media:   p.Media,
			})
		},
	}
	toolCfg.Web.Proxy = cfg.Tools.Web.Proxy
	toolCfg.Web.Search = tools.WebSearchConfig{
		Provider:   cfg.Tools.Web.Search.Provider,
		APIKey:     cfg.Tools.Web.Search.APIKey,
		BaseURL:    cfg.Tools.Web.Search.BaseURL,
		MaxResults: cfg.Tools.Web.Search.MaxResults,
	}
	toolCfg.Exec = tools.ShellConfig{
		Timeout:       cfg.Tools.Exec.Timeout.Duration,
		MaxOutput:     cfg.Tools.Exec.MaxOutput,
		DenyPatterns:  cfg.Tools.Exec.DenyPatterns,
		AllowPatterns: cfg.Tools.Exec.AllowPatterns,
		PathAppend:    cfg.Tools.Exec.PathAppend,
	}
	for _, mcp := range cfg.Tools.MCP {
		toolCfg.MCP = append(toolCfg.MCP, tools.MCPServerConfig{
			Name:         mcp.Name,
			Type:         mcp.Type,
			Command:      mcp.Command,
			Args:         mcp.Args,
			Env:          mcp.Env,
			URL:          mcp.URL,
			Headers:      mcp.Headers,
			ToolTimeout:  mcp.ToolTimeout.Duration,
			EnabledTools: mcp.EnabledTools,
		})
	}
	return toolCfg
}

func NewProgressHandler(messageBus *bus.MessageBus) tools.ToolProgressFunc {
	return func(ctx context.Context, toolName, status string) {
		pi := tools.GetProgressInfo(ctx)
		if pi == nil {
			logApp.Debug("Progress event without channel context",
				"tool", toolName, "status", status)
			return
		}
		messageBus.PublishOutbound(ctx, &bus.OutboundMessage{
			Channel:  pi.Channel,
			ChatID:   pi.ChatID,
			Content:  fmt.Sprintf("🔧 %s: %s", toolName, status),
			Metadata: map[string]any{"_progress": true},
		})
	}
}
