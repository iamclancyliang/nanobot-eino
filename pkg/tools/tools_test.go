package tools

import (
	"context"
	"testing"
)

// TestNewTools_IgnoresMCPConfigs verifies that NewTools does NOT attempt to
// connect MCP servers even when cfg.MCP is populated.
//
// In the agent flow, NewAgent extracts cfg.MCP before calling NewTools and
// connects them lazily via ensureMCPConnected. The MCP block inside NewTools
// is therefore dead code and should be removed.
//
// This test points an unreachable MCP address; if the block were still present,
// NewTools would return an error. After the fix it must succeed.
func TestNewTools_IgnoresMCPConfigs(t *testing.T) {
	cfg := ToolConfig{
		Workspace: t.TempDir(),
		MCP: []MCPServerConfig{
			{
				Name:    "unreachable-server",
				Type:    "stdio",
				Command: "this-binary-does-not-exist",
			},
		},
	}

	_, err := NewTools(context.Background(), cfg)
	if err != nil {
		t.Errorf("NewTools should ignore cfg.MCP (agent handles MCP via lazy loading), got error: %v", err)
	}
}
