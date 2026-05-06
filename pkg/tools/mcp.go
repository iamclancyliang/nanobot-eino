package tools

import (
	"context"
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/cloudwego/eino-ext/components/tool/mcp/officialmcp"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// MCPServerConfig configures one MCP (Model Context Protocol) server
// connection. Type is auto-detected from Command/URL when left empty.
type MCPServerConfig struct {
	Name         string
	Type         string // "stdio", "sse", "streamableHttp"; auto-detected if empty
	Command      string
	Args         []string
	Env          map[string]string
	URL          string
	Headers      map[string]string
	ToolTimeout  time.Duration
	EnabledTools []string // ["*"] = all, empty = none
}

type mcpSessionEntry struct {
	serverName string
	session    *mcp.ClientSession
}

var (
	mcpSessionsMu sync.Mutex
	mcpSessions   []mcpSessionEntry
)

// ConnectMCPServer dials an MCP server, registers the underlying session
// for later cleanup, and returns the tools it advertises that match
// cfg.EnabledTools.
func ConnectMCPServer(ctx context.Context, cfg MCPServerConfig) ([]tool.InvokableTool, error) {
	transportType := cfg.Type
	if transportType == "" {
		switch {
		case cfg.Command != "":
			transportType = "stdio"
		case cfg.URL != "":
			if strings.HasSuffix(strings.TrimRight(cfg.URL, "/"), "/sse") {
				transportType = "sse"
			} else {
				transportType = "streamableHttp"
			}
		default:
			return nil, fmt.Errorf("MCP server '%s': no command or url configured", cfg.Name)
		}
	}

	if cfg.ToolTimeout == 0 {
		cfg.ToolTimeout = 30 * time.Second
	}

	var transport mcp.Transport

	switch transportType {
	case "stdio":
		cmd := exec.Command(cfg.Command, cfg.Args...)
		if len(cfg.Env) > 0 {
			env := cmd.Environ()
			for k, v := range cfg.Env {
				env = append(env, fmt.Sprintf("%s=%s", k, v))
			}
			cmd.Env = env
		}
		transport = &mcp.CommandTransport{Command: cmd}

	case "sse":
		t := &mcp.SSEClientTransport{
			Endpoint: cfg.URL,
		}
		if len(cfg.Headers) > 0 {
			t.HTTPClient = &http.Client{
				Transport: &headerTransport{
					base:    http.DefaultTransport,
					headers: cfg.Headers,
				},
			}
		}
		transport = t

	case "streamableHttp":
		t := &mcp.StreamableClientTransport{
			Endpoint: cfg.URL,
		}
		if len(cfg.Headers) > 0 {
			t.HTTPClient = &http.Client{
				Transport: &headerTransport{
					base:    http.DefaultTransport,
					headers: cfg.Headers,
				},
			}
		}
		transport = t

	default:
		return nil, fmt.Errorf("MCP server '%s': unknown transport type '%s'", cfg.Name, transportType)
	}

	client := mcp.NewClient(&mcp.Implementation{
		Name:    "nanobot-eino",
		Version: "0.1.0",
	}, nil)

	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		return nil, fmt.Errorf("MCP server '%s': failed to connect: %w", cfg.Name, err)
	}
	registerMCPSession(cfg.Name, session)

	enabledSet := make(map[string]bool)
	allowAll := false
	for _, t := range cfg.EnabledTools {
		if t == "*" {
			allowAll = true
			break
		}
		enabledSet[t] = true
	}
	if len(cfg.EnabledTools) == 0 {
		allowAll = true
	}

	var toolNameList []string
	if !allowAll {
		for name := range enabledSet {
			n := name
			if strings.HasPrefix(n, "mcp_"+cfg.Name+"_") {
				n = strings.TrimPrefix(n, "mcp_"+cfg.Name+"_")
			}
			toolNameList = append(toolNameList, n)
		}
	}

	baseTools, err := officialmcp.GetTools(ctx, &officialmcp.Config{
		Cli:          session,
		ToolNameList: toolNameList,
	})
	if err != nil {
		return nil, fmt.Errorf("MCP server '%s': failed to list tools: %w", cfg.Name, err)
	}

	var invokableTools []tool.InvokableTool
	for _, bt := range baseTools {
		if it, ok := bt.(tool.InvokableTool); ok {
			invokableTools = append(invokableTools, &mcpToolWrapper{
				inner:       it,
				serverName:  cfg.Name,
				toolTimeout: cfg.ToolTimeout,
			})
		}
	}

	logTools.Info("MCP server connected", "server", cfg.Name, "tool_count", len(invokableTools))
	return invokableTools, nil
}

func registerMCPSession(serverName string, session *mcp.ClientSession) {
	mcpSessionsMu.Lock()
	defer mcpSessionsMu.Unlock()
	mcpSessions = append(mcpSessions, mcpSessionEntry{
		serverName: serverName,
		session:    session,
	})
}

// CloseMCPConnections closes all active MCP sessions created via ConnectMCPServer.
// It returns the number of sessions that were attempted to close.
func CloseMCPConnections() int {
	mcpSessionsMu.Lock()
	sessions := mcpSessions
	mcpSessions = nil
	mcpSessionsMu.Unlock()

	for _, entry := range sessions {
		if entry.session == nil {
			continue
		}
		if err := entry.session.Close(); err != nil {
			logTools.Warn("MCP server close failed", "server", entry.serverName, "error", err)
		}
	}
	return len(sessions)
}

type mcpToolWrapper struct {
	inner       tool.InvokableTool
	serverName  string
	toolTimeout time.Duration
}

func (w *mcpToolWrapper) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return w.inner.Info(ctx)
}

func (w *mcpToolWrapper) InvokableRun(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
	tCtx, cancel := context.WithTimeout(ctx, w.toolTimeout)
	defer cancel()

	result, err := w.inner.InvokableRun(tCtx, argumentsInJSON, opts...)
	if tCtx.Err() == context.DeadlineExceeded {
		logTools.Warn("MCP tool timed out", "server", w.serverName, "timeout", w.toolTimeout)
		return fmt.Sprintf("(MCP tool call timed out after %v)", w.toolTimeout), nil
	}
	if err != nil {
		logTools.Warn("MCP tool failed", "server", w.serverName, "error", err)
		return fmt.Sprintf("(MCP tool call failed: %v)", err), nil
	}
	if result == "" {
		return "(no output)", nil
	}
	return result, nil
}

type headerTransport struct {
	base    http.RoundTripper
	headers map[string]string
}

func (t *headerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	for k, v := range t.headers {
		req.Header.Set(k, v)
	}
	return t.base.RoundTrip(req)
}
