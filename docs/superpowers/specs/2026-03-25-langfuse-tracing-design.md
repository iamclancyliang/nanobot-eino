# Langfuse Tracing Design for Nanobot-Eino

## Overview

Add full-chain observability to Nanobot-Eino using Langfuse as the tracing backend, integrated through Eino's global callback system. The goal is to enable developers to visualize every step of the Agent's reasoning process — LLM calls, tool executions, and memory consolidation — in Langfuse's web UI for debugging and prompt optimization.

## Requirements

- **Use case:** Personal development debugging
- **Deployment:** Local Docker self-hosted Langfuse
- **Granularity:** Full trace chain (LLM calls, tool execution, memory consolidation, prompt building)
- **Constraint:** Near-zero invasion to existing business code

## Architecture

```
┌─────────────────────────────────────────────────────┐
│                   Nanobot-Eino                       │
│                                                     │
│  Channel ──▶ Agent.ChatStream ──▶ ReAct Loop        │
│                                    ├── LLM Call     │
│                                    ├── Tool Exec    │
│                                    └── Memory Cons. │
│                                         │           │
│  Eino Callback System                   │           │
│  (Global Handler at startup)            │           │
│             │                           │           │
│  ┌──────────▼───────────┐               │           │
│  │  pkg/trace/           │◀─────────────┘           │
│  │  - langfuse handler   │                          │
│  │  - config             │                          │
│  │  - manual span helper │                          │
│  └──────────┬───────────┘                           │
└─────────────┼───────────────────────────────────────┘
              │ HTTP (batch)
              ▼
┌─────────────────────────┐
│  Langfuse (Docker)       │
│  - PostgreSQL            │
│  - ClickHouse            │
│  - Redis                 │
│  - MinIO                 │
│  - Web UI (:3000)        │
└─────────────────────────┘
```

**Core mechanism:** Register a Langfuse callback handler globally via `callbacks.AppendGlobalHandlers()` at startup. All Eino components (ChatModel, Tool, Retriever, etc.) are automatically traced without modifying existing business code.

## Trace Data Collected

### LLM Calls (ChatModel)

| Field | Source |
|-------|--------|
| Model name / Provider | `RunInfo.Name` + `RunInfo.Type` |
| Input messages | `model.ConvCallbackInput().Messages` |
| Output message | `model.ConvCallbackOutput().Message` |
| Token usage | `model.ConvCallbackOutput().TokenUsage` (prompt/completion/total) |
| Tools declared | `model.ConvCallbackInput().Tools` |
| Latency | OnStart → OnEnd time delta |
| Streaming output | OnEndWithStreamOutput async collection |

### Tool Execution

| Field | Source |
|-------|--------|
| Tool name | `RunInfo.Name` |
| Input args JSON | `tool.ConvCallbackInput().ArgumentsInJSON` |
| Result | `tool.ConvCallbackOutput().Result` |
| Latency | OnStart → OnEnd time delta |
| Error | OnError capture |

### Memory Consolidation

Memory consolidation is custom logic in `pkg/memory/` that calls LLM directly outside the Eino compose graph. Handled via manual instrumentation using `trace.StartSpan` / `trace.EndSpan` helper functions that invoke `callbacks.OnStart` / `callbacks.OnEnd` to stay consistent with the global handler data flow.

### Trace Hierarchy (Langfuse UI)

```
Trace: "chat-session-{sessionID}"
├── Generation: "LLM Call #1"
│   ├── input: messages[], tools[]
│   ├── output: assistant message + tool_calls
│   ├── usage: {prompt: 1200, completion: 350, total: 1550}
│   └── latency: 2.3s
├── Span: "Tool: web_search"
│   ├── input: {"query": "..."}
│   ├── output: "search results..."
│   └── latency: 1.1s
├── Span: "Tool: read_file"
│   ├── input: {"path": "/..."}
│   ├── output: "file content..."
│   └── latency: 0.02s
├── Generation: "LLM Call #2"
│   ├── usage: {prompt: 2800, completion: 500, total: 3300}
│   └── latency: 3.1s
└── Span: "Memory Consolidation"
    ├── Generation: "consolidation LLM call"
    └── latency: 1.8s
```

## Package Design: `pkg/trace/`

```
pkg/trace/
├── trace.go    # Init / Shutdown / global handler registration
├── config.go   # TracingConfig struct + Viper loading
└── span.go     # Manual span helpers for non-Eino components
```

### `config.go`

```go
type TracingConfig struct {
    Enabled   bool   // Master switch, default false
    Endpoint  string // Langfuse API URL, default "http://localhost:3000"
    PublicKey string // Langfuse public key
    SecretKey string // Langfuse secret key
}
```

Configuration via Viper (same pattern as existing config):

```yaml
trace:
  enabled: true
  endpoint: "http://localhost:3000"
  publicKey: "pk-lf-..."
  secretKey: "sk-lf-..."
```

Environment variable override supported: `NANOBOT_TRACE_ENABLED`, `NANOBOT_TRACE_ENDPOINT`, etc.

### `trace.go`

```go
// Init reads config, creates Langfuse handler, registers globally.
// Returns shutdown function to flush pending trace data.
// When enabled=false, returns no-op shutdown with zero overhead.
func Init(cfg TracingConfig) (shutdown func(), err error)
```

- Calls `langfuse.NewLangfuseHandler()` to create the handler
- Registers via `callbacks.AppendGlobalHandlers()`
- Returned `shutdown` wraps `flusher()` to ensure data is sent on exit

### `span.go`

```go
// StartSpan creates a manual span for non-Eino components (e.g., memory consolidation).
func StartSpan(ctx context.Context, name string, input map[string]any) context.Context

// EndSpan ends the current span.
func EndSpan(ctx context.Context, output map[string]any, err error)
```

Thin wrapper over Eino's `callbacks.OnStart` / `callbacks.OnEnd` / `callbacks.OnError`.

## Trace Context Injection

Langfuse traces require session/user context. Injected at the inbound message entry point:

```
User message arrives
    │
    ▼
ConsumeInbound (pkg/app/runloop.go)
    │
    ├── langfuse.SetTrace(ctx,
    │     WithTraceID(messageID),
    │     WithSessionID(sessionID),
    │     WithUserID(route),
    │     WithMetadata(channelType),
    │   )
    │
    ▼
Agent.ChatStream(ctx, input)   ← ctx carries trace context
                                  all subsequent callbacks associate to the same trace
```

## Integration Points (Changes to Existing Code)

| File | Change | Invasiveness |
|------|--------|-------------|
| `pkg/config/schema.go` | Add `TracingConfig` field to config schema | Low (add field) |
| `cmd/nanobot/gateway.go` | Call `trace.Init()` in `runGateway()`, inject shutdown into graceful shutdown | Low (~5 lines) |
| `pkg/app/runloop.go` | Inject trace context in `ConsumeInbound` entry | Low (~5 lines) |
| `pkg/memory/consolidator.go` | Add `StartSpan/EndSpan` around consolidation LLM call | Low (~6 lines) |
| `go.mod` | Add `eino-ext/callbacks/langfuse` dependency | 1 line |
| **All other files** | **No changes** | **Zero** |

## Docker Compose Deployment

New file `docker-compose.langfuse.yml` at project root:

```yaml
# Start: docker compose -f docker-compose.langfuse.yml up -d
services:
  langfuse:    # Web UI + API (:3000)
  postgres:    # Metadata storage (:5432)
  clickhouse:  # Analytics storage (:8123)
  redis:       # Cache (:6379)
  minio:       # Object storage (:9000)
```

- Internal network, only Langfuse UI port 3000 exposed
- Data persisted to Docker volumes
- `.env.langfuse.example` provided with default keys

## Design Principles

1. **Zero invasion:** ChatModel and Tool tracing is fully automatic via Eino global callbacks. No changes to `pkg/agent/`, `pkg/reactutil/`, `pkg/tools/`.
2. **Opt-in:** Controlled by `trace.enabled` config switch. When disabled, zero runtime overhead.
3. **Graceful shutdown:** `shutdown` function flushes pending trace data before process exits.
4. **Consistent data flow:** Manual spans (memory consolidation) use the same callback mechanism as automatic spans.

## New Dependencies

- `github.com/cloudwego/eino-ext/callbacks/langfuse` — Eino Langfuse callback handler

## Estimated Code Volume

| Component | Lines |
|-----------|-------|
| `pkg/trace/` (new) | ~150 |
| Config changes | ~10 |
| Bootstrap integration | ~5 |
| Runloop context injection | ~5 |
| Memory manual instrumentation | ~6 |
| Docker Compose + env example | Config files |
| **Total new Go code** | **~176** |
