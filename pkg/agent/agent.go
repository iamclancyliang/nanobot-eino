package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/cloudwego/eino/components/tool"
	emodel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/flow/agent/react"
	"github.com/cloudwego/eino/schema"

	"github.com/wall/nanobot-eino/pkg/cron"
	"github.com/wall/nanobot-eino/pkg/memory"
	"github.com/wall/nanobot-eino/pkg/model"
	"github.com/wall/nanobot-eino/pkg/prompt"
	"github.com/wall/nanobot-eino/pkg/reactutil"
	"github.com/wall/nanobot-eino/pkg/session"
	"github.com/wall/nanobot-eino/pkg/skill"
	"github.com/wall/nanobot-eino/pkg/subagent"
	"github.com/wall/nanobot-eino/pkg/tools"
)

var logAgent = slog.With("module", "agent")

const toolRecoveryHint = "\n\n[Analyze the error above and try a different approach.]"

type Agent struct {
	mu           sync.RWMutex
	reactAgent   *react.Agent
	consolidator *memory.MemoryConsolidator
	promptLoader *prompt.Loader
	cronService  *cron.CronService
	skillManager *skill.Manager
	sessions     *session.SessionManager
	toolCfg      tools.ToolConfig

	// activeTasks stores per-session cancel functions for /stop support.
	// Per-session serialization is handled by RunInboundLoop's worker goroutines;
	// callers are responsible for not calling ChatStream concurrently on the same session.
	activeTasks     sync.Map // map[string]context.CancelFunc
	subagentManager *subagent.SubagentManager

	// OnProgress is called when a tool starts or finishes execution.
	// Set by the caller (e.g. server/gateway) to route progress info.
	OnProgress tools.ToolProgressFunc

	// MCP lazy loading: configs are stored at init, connected on first message.
	mcpConfigs []tools.MCPServerConfig
	mcpOnce    sync.Once
	baseTools  []tool.BaseTool

	// Stored for react agent recreation after MCP tools are connected.
	toolCallingModel emodel.ToolCallingChatModel
	chatModel        emodel.ChatModel
	maxStep          int
}

func NewAgent(
	ctx context.Context,
	modelCfg model.Config,
	toolCfg tools.ToolConfig,
	memStore *memory.MemoryStore,
	promptDir string,
	builtinSkillsDir string,
	cronService *cron.CronService,
	sessionMgr *session.SessionManager,
	contextWindowTokens int,
	maxStep int,
	subagentMgr *subagent.SubagentManager,
) (*Agent, error) {
	chatModel, err := model.NewChatModel(ctx, modelCfg)
	if err != nil {
		return nil, err
	}

	skillManager := skill.NewManager(toolCfg.Workspace, builtinSkillsDir)
	if err := skillManager.LoadSkills(); err != nil {
		return nil, err
	}

	// Store MCP configs for lazy loading; remove from toolCfg so NewTools skips them.
	mcpConfigs := toolCfg.MCP
	toolCfg.MCP = nil

	toolCfg.ExtraReadDirs = append(toolCfg.ExtraReadDirs, builtinSkillsDir)
	invokableTools, err := tools.NewTools(ctx, toolCfg)
	if err != nil {
		return nil, err
	}

	if cronService != nil {
		cronTool, err := tools.NewCronTool(cronService, toolCfg.DefaultChannel, toolCfg.DefaultChatID)
		if err == nil {
			invokableTools = append(invokableTools, cronTool)
		}
	}

	if subagentMgr != nil {
		invokableTools = append(invokableTools, tools.NewSpawnTool(subagentMgr))
	}

	if maxStep <= 0 {
		maxStep = 20
	}

	promptLoader := prompt.NewLoader(promptDir)
	consolidator := memory.NewMemoryConsolidator(
		memStore, chatModel, sessionMgr,
		contextWindowTokens, 2000,
	)

	a := &Agent{
		consolidator:    consolidator,
		promptLoader:    promptLoader,
		cronService:     cronService,
		skillManager:    skillManager,
		sessions:        sessionMgr,
		toolCfg:         toolCfg,
		mcpConfigs:      mcpConfigs,
		maxStep:         maxStep,
		chatModel:       chatModel,
		subagentManager: subagentMgr,
	}

	if tcm, ok := chatModel.(emodel.ToolCallingChatModel); ok {
		a.toolCallingModel = tcm
		logAgent.Info("Model supports ToolCallingChatModel interface")
	}

	// Wrap tools with truncation + progress reporting.
	progressFn := func(ctx context.Context, toolName, status string) {
		if a.OnProgress != nil {
			a.OnProgress(ctx, toolName, status)
		}
	}
	wrappedTools := tools.WrapTools(invokableTools, tools.ToolResultMaxChars, progressFn)

	baseTools := make([]tool.BaseTool, len(wrappedTools))
	for i, t := range wrappedTools {
		baseTools[i] = t
	}
	a.baseTools = baseTools

	agent, err := a.newReactAgent(ctx, baseTools)
	if err != nil {
		return nil, err
	}
	a.reactAgent = agent

	return a, nil
}

// newReactAgent creates a react.Agent with the given tools.
// Delegates to reactutil.NewReactAgent for shared boilerplate (StreamToolCallChecker,
// model-type detection), then adds Agent-specific error handlers.
func (a *Agent) newReactAgent(ctx context.Context, allTools []tool.BaseTool) (*react.Agent, error) {
	toolNames := listToolNames(ctx, allTools)
	opts := &reactutil.AgentOptions{
		UnknownToolsHandler: func(ctx context.Context, name, input string) (string, error) {
			available := strings.Join(toolNames, ", ")
			if available == "" {
				available = "(none)"
			}
			return fmt.Sprintf(
				"Error: Tool '%s' not found. Available tools: %s.%s",
				name,
				available,
				toolRecoveryHint,
			), nil
		},
		ToolArgumentsHandler: func(ctx context.Context, name, arguments string) (string, error) {
			normalized := normalizeToolArguments(arguments)
			if normalized == "" {
				return "{}", nil
			}
			return normalized, nil
		},
	}

	// Use toolCallingModel if available, otherwise fall back to plain chatModel.
	// Both satisfy emodel.BaseChatModel which reactutil.NewReactAgent accepts.
	var cm emodel.BaseChatModel
	if a.toolCallingModel != nil {
		cm = a.toolCallingModel
	} else {
		cm = a.chatModel
	}
	return reactutil.NewReactAgent(ctx, cm, allTools, a.maxStep, opts)
}

func listToolNames(ctx context.Context, allTools []tool.BaseTool) []string {
	names := make([]string, 0, len(allTools))
	for _, t := range allTools {
		info, err := t.Info(ctx)
		if err != nil || info == nil || strings.TrimSpace(info.Name) == "" {
			continue
		}
		names = append(names, strings.TrimSpace(info.Name))
	}
	sort.Strings(names)
	return names
}

func normalizeToolArguments(arguments string) string {
	trimmed := strings.TrimSpace(arguments)
	if trimmed == "" {
		return "{}"
	}
	trimmed = stripJSONCodeFence(trimmed)
	if trimmed == "" {
		return "{}"
	}

	var parsed any
	if err := json.Unmarshal([]byte(trimmed), &parsed); err != nil {
		fallback, _ := json.Marshal(map[string]any{
			"_tool_argument_error": "invalid_json",
			"_raw_arguments":       truncateToolArgument(trimmed, 400),
		})
		return string(fallback)
	}
	return trimmed
}

func stripJSONCodeFence(raw string) string {
	s := strings.TrimSpace(raw)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```JSON")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	return strings.TrimSpace(s)
}

func truncateToolArgument(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	return s[:max]
}

// ensureMCPConnected lazily connects MCP servers on first message,
// recreating the react agent with the additional tools.
func (a *Agent) ensureMCPConnected(ctx context.Context) {
	a.mcpOnce.Do(func() {
		if len(a.mcpConfigs) == 0 {
			return
		}

		logAgent.Info("Lazy-connecting MCP servers", "count", len(a.mcpConfigs))
		var mcpInvokableTools []tool.InvokableTool
		for _, cfg := range a.mcpConfigs {
			mcpTools, err := tools.ConnectMCPServer(ctx, cfg)
			if err != nil {
				logAgent.Warn("MCP server connection failed, skipped", "server", cfg.Name, "error", err)
				continue
			}
			mcpInvokableTools = append(mcpInvokableTools, mcpTools...)
		}

		if len(mcpInvokableTools) == 0 {
			return
		}

		progressFn := func(ctx context.Context, toolName, status string) {
			if a.OnProgress != nil {
				a.OnProgress(ctx, toolName, status)
			}
		}
		wrappedMCPTools := tools.WrapTools(mcpInvokableTools, tools.ToolResultMaxChars, progressFn)

		newBaseTools := make([]tool.BaseTool, 0, len(a.baseTools)+len(wrappedMCPTools))
		newBaseTools = append(newBaseTools, a.baseTools...)
		for _, t := range wrappedMCPTools {
			newBaseTools = append(newBaseTools, t)
		}

		newAgent, err := a.newReactAgent(ctx, newBaseTools)
		if err != nil {
			logAgent.Warn("Failed to recreate agent with MCP tools", "error", err)
			return
		}

		a.mu.Lock()
		a.reactAgent = newAgent
		a.baseTools = newBaseTools
		a.mu.Unlock()

		logAgent.Info("MCP tools connected", "new_tools", len(wrappedMCPTools))
	})
}

// ChatStream processes a message within a session: pre-consolidation → build prompt →
// run agent loop → stream response → save turn → post-consolidation.
func (a *Agent) ChatStream(ctx context.Context, sessionID, input string) (*schema.StreamReader[*schema.Message], error) {
	cmd := strings.TrimSpace(strings.ToLower(input))
	if cmd == "/new" || cmd == "new" || cmd == "新会话" {
		logAgent.Info("New session command received", "input", input, "session", sessionID)
		return a.handleNewSession(ctx, sessionID)
	}
	if cmd == "/help" || cmd == "help" {
		return stringToStream(commandHelpText()), nil
	}
	if cmd == "/stop" {
		return a.handleStop(sessionID)
	}
	if cmd == "/restart" {
		return a.handleRestart()
	}

	// Lazy-connect MCP servers on first real message.
	a.ensureMCPConnected(ctx)

	// Create cancellable context for /stop support.
	// Per-session serialization is handled by RunInboundLoop's per-session workers.
	taskCtx, taskCancel := context.WithCancel(ctx)
	a.activeTasks.Store(sessionID, taskCancel)

	// Inject sessionID for progress callbacks.
	taskCtx = tools.ContextWithSessionID(taskCtx, sessionID)

	sess := a.sessions.GetOrCreate(sessionID)

	// Pre-consolidation: trim context if it exceeds window.
	a.consolidator.MaybeConsolidateByTokens(taskCtx, sess)

	history := sess.GetHistory(0)
	messages, err := a.buildMessages(taskCtx, history, input)
	if err != nil {
		a.activeTasks.Delete(sessionID)
		taskCancel()
		return nil, err
	}

	logAgent.Debug("Sending to LLM", "session", sessionID, "message_count", len(messages))
	for i, m := range messages {
		preview := m.Content
		runes := []rune(preview)
		if len(runes) > 200 {
			preview = string(runes[:200]) + "..."
		}
		logAgent.Debug("LLM message", "index", i, "role", m.Role, "preview", preview)
	}

	a.mu.RLock()
	currentAgent := a.reactAgent
	a.mu.RUnlock()

	// WithMessageFuture captures all intermediate messages produced during the agent
	// loop (model turns with tool_calls + tool result messages), so the session stores
	// the full conversation turn, not just the final text.
	futureOpt, future := react.WithMessageFuture()

	stream, err := currentAgent.Stream(taskCtx, messages, futureOpt)
	if err != nil {
		a.activeTasks.Delete(sessionID)
		taskCancel()
		return nil, err
	}

	// Pipe the stream, capture full response, then save turn and run post-consolidation.
	pipeReader, pipeWriter := schema.Pipe[*schema.Message](10)
	go func() {
		defer func() {
			a.activeTasks.Delete(sessionID)
			taskCancel()
			pipeWriter.Close()
		}()

		// Collect all intermediate messages (tool_calls, tool results, final response)
		// concurrently with the main stream consumption. GetMessageStreams() blocks until
		// the graph starts, which happens when stream.Recv() is first called below.
		var collectedMsgs []*schema.Message
		var collectWg sync.WaitGroup
		collectWg.Add(1)
		go func() {
			defer collectWg.Done()
			sIter := future.GetMessageStreams()
			for {
				msgSR, hasNext, iterErr := sIter.Next()
				if iterErr != nil || !hasNext {
					break
				}
				msg, concatErr := schema.ConcatMessageStream(msgSR)
				if concatErr != nil {
					logAgent.Error("Failed to concat message stream", "session", sessionID, "error", concatErr)
					break
				}
				collectedMsgs = append(collectedMsgs, msg)
			}
		}()

		var fullResponse strings.Builder
		for {
			msg, recvErr := stream.Recv()
			if recvErr != nil {
				if recvErr == io.EOF || taskCtx.Err() == context.Canceled {
					break
				}
				pipeWriter.Send(nil, recvErr)
				break
			}
			fullResponse.WriteString(msg.Content)
			pipeWriter.Send(msg, nil)
		}
		stream.Close()

		// Wait for message collection goroutine to finish before saving.
		collectWg.Wait()

		// Build session turn: save input message + all collected messages.
		// collectedMsgs includes assistant tool_calls turns, tool result turns, and the
		// final assistant response, giving the session a faithful replay of the agent loop.
		inputMsg := schema.UserMessage(input)
		if tools.InputRoleFromContext(taskCtx) == "assistant" {
			inputMsg = &schema.Message{Role: schema.Assistant, Content: input}
		}

		if len(collectedMsgs) > 0 {
			sess.AddMessage(inputMsg)
			for _, m := range collectedMsgs {
				sess.AddMessage(m)
			}
		} else {
			// Fallback: no future messages collected (e.g. graph error); save final text.
			responseText := strings.TrimSpace(fullResponse.String())
			if responseText == "" {
				return
			}
			sess.AddMessage(inputMsg)
			sess.AddMessage(&schema.Message{Role: schema.Assistant, Content: responseText})
		}

		if saveErr := a.sessions.Save(sess); saveErr != nil {
			logAgent.Error("Failed to save session", "session", sessionID, "error", saveErr)
		}

		// Post-consolidation (use background context since streaming may have cancelled ctx).
		a.consolidator.MaybeConsolidateByTokens(context.Background(), sess)
	}()

	return pipeReader, nil
}

func commandHelpText() string {
	return "🐈 nanobot commands:\n/new — Start a new conversation\n/stop — Stop the current task\n/restart — Restart the bot process\n/help — Show available commands"
}

func restartAckText() string {
	return "Restarting..."
}

// handleRestart performs an in-place process replacement using syscall.Exec,
// mirroring Python nanobot's os.execv restart. A short delay lets the ack
// message be sent before the process image is replaced.
func (a *Agent) handleRestart() (*schema.StreamReader[*schema.Message], error) {
	sr, sw := schema.Pipe[*schema.Message](1)
	go func() {
		sw.Send(&schema.Message{Role: schema.Assistant, Content: "Restarting..."}, nil)
		sw.Close()
		// Small delay to allow the ack message to be flushed to the client.
		time.Sleep(500 * time.Millisecond)
		if err := restartProcess(); err != nil {
			logAgent.Error("Restart failed", "error", err)
		}
	}()
	return sr, nil
}

// restartProcess replaces the current process image in-place via syscall.Exec.
// On success it never returns. On failure (e.g. unsupported OS) it returns an error.
func restartProcess() error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("cannot resolve executable path: %w", err)
	}
	// Resolve symlinks so we exec the real binary, not a symlink.
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return fmt.Errorf("cannot eval symlinks for executable: %w", err)
	}
	return syscall.Exec(exe, os.Args, os.Environ())
}

func (a *Agent) handleNewSession(ctx context.Context, sessionID string) (*schema.StreamReader[*schema.Message], error) {
	sess := a.sessions.GetOrCreate(sessionID)
	logAgent.Info("handleNewSession",
		"session", sessionID, "messages", len(sess.Messages), "last_consolidated", sess.LastConsolidated)

	if err := a.consolidator.ArchiveUnconsolidated(ctx, sess); err != nil {
		logAgent.Error("handleNewSession: archival failed", "session", sessionID, "error", err)
		return stringToStream("Memory archival failed, session not cleared. Please try again."), nil
	}

	logAgent.Info("handleNewSession: archival succeeded, clearing session", "session", sessionID)
	sess.Clear()
	if err := a.sessions.Save(sess); err != nil {
		logAgent.Error("Failed to save cleared session", "error", err)
	}
	a.sessions.Invalidate(sessionID)
	return stringToStream("🐈 New session started. Previous conversation has been archived to memory."), nil
}

// CancelAll cancels all in-flight tasks across all sessions.
// Returns the number of tasks cancelled.
func (a *Agent) CancelAll() int {
	cancelled := 0
	a.activeTasks.Range(func(key, value any) bool {
		if cancelFn, ok := value.(context.CancelFunc); ok {
			cancelFn()
			cancelled++
		}
		a.activeTasks.Delete(key)
		return true
	})
	return cancelled
}

func (a *Agent) handleStop(sessionID string) (*schema.StreamReader[*schema.Message], error) {
	stopped := false

	if cancelFn, ok := a.activeTasks.LoadAndDelete(sessionID); ok {
		cancelFn.(context.CancelFunc)()
		stopped = true
	}

	subCancelled := 0
	if a.subagentManager != nil {
		subCancelled = a.subagentManager.CancelBySession(sessionID)
	}

	if stopped || subCancelled > 0 {
		msg := "🛑 Current task has been stopped."
		if subCancelled > 0 {
			msg += fmt.Sprintf(" (%d subagent task(s) also cancelled)", subCancelled)
		}
		logAgent.Info("Task stopped", "session", sessionID, "subagents_cancelled", subCancelled)
		return stringToStream(msg), nil
	}
	return stringToStream("No active task to stop."), nil
}

// buildMessages constructs the full prompt: system + skills + memory + history + user input.
// Sections are joined with "\n\n---\n\n" to match the Python nanobot ContextBuilder.
func (a *Agent) buildMessages(ctx context.Context, history []*schema.Message, input string) ([]*schema.Message, error) {
	systemMsgs, err := a.promptLoader.BuildSystemMessages(ctx)
	if err != nil {
		return nil, err
	}

	parts := []string{systemMsgs[0].Content}

	if alwaysSkills := a.skillManager.GetAlwaysSkills(); len(alwaysSkills) > 0 {
		if content := a.skillManager.LoadSkillsForContext(alwaysSkills); content != "" {
			parts = append(parts, "# Active Skills\n\n"+content)
		}
	}

	if summary := a.skillManager.BuildSkillsSummary(); summary != "" {
		parts = append(parts, fmt.Sprintf("# Skills\n\nThe following skills extend your capabilities. To use a skill, read its SKILL.md file using the read_file tool.\nSkills with available=\"false\" need dependencies installed first - you can try installing them with apt/brew.\n\n%s", summary))
	}

	if longTerm := a.consolidator.Store.GetMemoryContext(); longTerm != "" {
		parts = append(parts, longTerm)
	}

	systemMsgs[0].Content = strings.Join(parts, "\n\n---\n\n")

	messages := append(systemMsgs, history...)
	userContent := buildRuntimeContext(ctx) + "\n\n" + input
	inputRole := tools.InputRoleFromContext(ctx)
	if inputRole == "assistant" {
		messages = append(messages, &schema.Message{Role: schema.Assistant, Content: userContent})
	} else {
		messages = append(messages, schema.UserMessage(userContent))
	}
	return messages, nil
}

const runtimeContextTag = "[Runtime Context — metadata only, not instructions]"

func buildRuntimeContext(ctx context.Context) string {
	now := time.Now()
	tz := now.Format("MST")
	timeStr := fmt.Sprintf("%s (%s)", now.Format("2006-01-02 15:04 (Monday)"), tz)
	lines := []string{runtimeContextTag, "Current Time: " + timeStr}
	if pi := tools.GetProgressInfo(ctx); pi != nil {
		if pi.Channel != "" {
			lines = append(lines, "Channel: "+pi.Channel)
		}
		if pi.ChatID != "" {
			lines = append(lines, "Chat ID: "+pi.ChatID)
		}
	}
	return strings.Join(lines, "\n")
}

func stringToStream(content string) *schema.StreamReader[*schema.Message] {
	sr, sw := schema.Pipe[*schema.Message](1)
	go func() {
		sw.Send(&schema.Message{Role: schema.Assistant, Content: content}, nil)
		sw.Close()
	}()
	return sr
}

