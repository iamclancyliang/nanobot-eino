// Package reactutil provides shared helpers for building Eino react agents.
// It is a dependency-free utility layer imported by both pkg/agent and pkg/subagent,
// avoiding a circular import between those two packages.
package reactutil

import (
	"context"
	"fmt"
	"io"

	emodel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/flow/agent/react"
	"github.com/cloudwego/eino/schema"
)

// AgentOptions holds optional per-caller overrides for react agent creation.
type AgentOptions struct {
	// UnknownToolsHandler is called when the LLM invokes a tool that doesn't exist.
	UnknownToolsHandler func(ctx context.Context, name, input string) (string, error)
	// ToolArgumentsHandler pre-processes raw JSON arguments before tool invocation.
	ToolArgumentsHandler func(ctx context.Context, name, arguments string) (string, error)
}

// NewReactAgent creates a react.Agent with the standard StreamToolCallChecker
// and automatic model-type detection (ToolCallingChatModel vs ChatModel).
// chatModel may be either emodel.ChatModel or emodel.ToolCallingChatModel —
// both satisfy emodel.BaseChatModel.
// Optional handlers in opts are applied when non-nil.
func NewReactAgent(ctx context.Context, chatModel emodel.BaseChatModel, allTools []tool.BaseTool, maxStep int, opts *AgentOptions) (*react.Agent, error) {
	agentCfg := &react.AgentConfig{
		ToolsConfig: compose.ToolsNodeConfig{
			Tools: allTools,
		},
		MaxStep:               maxStep,
		StreamToolCallChecker: StreamToolCallChecker,
	}

	if opts != nil {
		if opts.UnknownToolsHandler != nil {
			agentCfg.ToolsConfig.UnknownToolsHandler = opts.UnknownToolsHandler
		}
		if opts.ToolArgumentsHandler != nil {
			agentCfg.ToolsConfig.ToolArgumentsHandler = opts.ToolArgumentsHandler
		}
	}

	if tcm, ok := chatModel.(emodel.ToolCallingChatModel); ok {
		agentCfg.ToolCallingModel = tcm
	} else if cm, ok := chatModel.(emodel.ChatModel); ok {
		agentCfg.Model = cm
	} else {
		return nil, fmt.Errorf("reactutil: chatModel must implement emodel.ChatModel or emodel.ToolCallingChatModel")
	}

	return react.NewAgent(ctx, agentCfg)
}

// StreamToolCallChecker is the standard Eino StreamToolCallChecker:
// it drains the stream and returns true if any message contains tool calls.
func StreamToolCallChecker(ctx context.Context, sr *schema.StreamReader[*schema.Message]) (bool, error) {
	defer sr.Close()
	for {
		msg, err := sr.Recv()
		if err == io.EOF {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		if len(msg.ToolCalls) > 0 {
			return true, nil
		}
	}
}

// IsToolCallingChatModel reports whether the given model implements
// emodel.ToolCallingChatModel. Exposed for testing and diagnostics.
func IsToolCallingChatModel(m emodel.BaseChatModel) bool {
	_, ok := m.(emodel.ToolCallingChatModel)
	return ok
}
