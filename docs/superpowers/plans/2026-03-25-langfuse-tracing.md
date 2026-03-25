# Langfuse Tracing Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add full-chain observability to Nanobot-Eino via Langfuse, with near-zero invasion to existing code.

**Architecture:** Register a Langfuse callback handler globally at startup via Eino's `callbacks.AppendGlobalHandlers()`. All ChatModel and Tool calls are automatically traced. Memory consolidation is manually instrumented. Trace context (session/user) is injected at the inbound message entry point.

**Tech Stack:** `github.com/cloudwego/eino-ext/callbacks/langfuse`, `github.com/cloudwego/eino/callbacks`, Langfuse v3 (Docker Compose)

**Spec:** `docs/superpowers/specs/2026-03-25-langfuse-tracing-design.md`

---

## File Structure

| File | Action | Responsibility |
|------|--------|---------------|
| `pkg/trace/trace.go` | Create | Init/Shutdown, global handler registration |
| `pkg/trace/span.go` | Create | Manual span helpers for non-Eino components |
| `pkg/trace/trace_test.go` | Create | Unit tests for trace package |
| `pkg/config/schema.go` | Modify | Add `Trace TracingConfig` field to Config |
| `cmd/nanobot/gateway.go` | Modify | Call `trace.Init()`, wire shutdown |
| `cmd/nanobot/agent.go` | Modify | Call `trace.Init()`, wire shutdown |
| `pkg/app/runloop.go` | Modify | Inject Langfuse trace context in processMessage |
| `pkg/memory/consolidator.go` | Modify | Add manual span around consolidation |
| `docker-compose.langfuse.yml` | Create | Langfuse + Postgres + ClickHouse + Redis + MinIO |
| `.env.langfuse.example` | Create | Example environment variables |
| `go.mod` / `go.sum` | Modify | Add langfuse dependency |

---

### Task 1: Add Langfuse dependency

**Files:**
- Modify: `go.mod`

- [ ] **Step 1: Add the eino-ext langfuse callback dependency**

```bash
cd /Users/wall/study/My-Repo
go get github.com/cloudwego/eino-ext/callbacks/langfuse@latest
```

- [ ] **Step 2: Verify dependency was added**

```bash
grep langfuse go.mod
```

Expected: line containing `github.com/cloudwego/eino-ext/callbacks/langfuse`

- [ ] **Step 3: Tidy modules**

```bash
go mod tidy
```

- [ ] **Step 4: Commit**

```bash
git add go.mod go.sum
git commit -m "deps: add eino-ext langfuse callback dependency"
```

---

### Task 2: Create `pkg/trace/trace.go` — Init and Shutdown

**Files:**
- Create: `pkg/trace/trace.go`

- [ ] **Step 1: Write the failing test**

Create `pkg/trace/trace_test.go`:

```go
package trace

import (
	"testing"

	"github.com/wall/nanobot-eino/pkg/config"
)

func TestInit_Disabled(t *testing.T) {
	cfg := config.TracingConfig{Enabled: false}
	shutdown, err := Init(cfg)
	if err != nil {
		t.Fatalf("Init with disabled config should not error: %v", err)
	}
	// shutdown should be a no-op, must not panic
	shutdown()
}

func TestInit_EnabledMissingFields(t *testing.T) {
	cfg := config.TracingConfig{Enabled: true}
	_, err := Init(cfg)
	if err == nil {
		t.Fatal("Init with enabled but empty config should return error")
	}
}

func TestInit_EnabledValid(t *testing.T) {
	cfg := config.TracingConfig{
		Enabled:   true,
		Endpoint:  "http://localhost:3000",
		PublicKey: "pk-lf-test",
		SecretKey: "sk-lf-test",
	}
	shutdown, err := Init(cfg)
	if err != nil {
		t.Fatalf("Init with valid config should not error: %v", err)
	}
	if shutdown == nil {
		t.Fatal("shutdown function should not be nil")
	}
	// Call shutdown to flush (no real server, just verify no panic)
	shutdown()
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd /Users/wall/study/My-Repo && go test ./pkg/trace/ -v -run TestInit
```

Expected: FAIL (Init not defined)

- [ ] **Step 3: Write the implementation**

Create `pkg/trace/trace.go`:

```go
package trace

import (
	"fmt"
	"log/slog"

	"github.com/cloudwego/eino-ext/callbacks/langfuse"
	"github.com/cloudwego/eino/callbacks"
	"github.com/wall/nanobot-eino/pkg/config"
)

// Init creates a Langfuse callback handler and registers it globally.
// When cfg.Enabled is false, returns a no-op shutdown function with zero overhead.
// The returned shutdown function MUST be called before process exit to flush pending traces.
func Init(cfg config.TracingConfig) (shutdown func(), err error) {
	if !cfg.Enabled {
		return func() {}, nil
	}

	if cfg.Endpoint == "" || cfg.PublicKey == "" || cfg.SecretKey == "" {
		return nil, fmt.Errorf("trace: enabled but missing required fields (endpoint, publicKey, secretKey)")
	}

	handler, flusher := langfuse.NewLangfuseHandler(&langfuse.Config{
		Host:      cfg.Endpoint,
		PublicKey: cfg.PublicKey,
		SecretKey: cfg.SecretKey,
	})

	callbacks.AppendGlobalHandlers(handler)

	slog.Info("Tracing enabled", "module", "trace", "endpoint", cfg.Endpoint)

	return flusher, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
cd /Users/wall/study/My-Repo && go test ./pkg/trace/ -v -run TestInit
```

Expected: all 3 tests PASS

- [ ] **Step 5: Commit**

```bash
git add pkg/trace/trace.go pkg/trace/trace_test.go
git commit -m "feat(trace): implement Init/Shutdown with Langfuse handler registration"
```

---

### Task 3: Create `pkg/trace/span.go` — Manual span helpers

**Files:**
- Create: `pkg/trace/span.go`

- [ ] **Step 1: Add tests to `pkg/trace/trace_test.go`**

Append to `pkg/trace/trace_test.go`:

```go
func TestStartSpan_EndSpan_NoPanic(t *testing.T) {
	ctx := context.Background()
	ctx = StartSpan(ctx, "test-span", map[string]any{"key": "value"})
	// EndSpan should not panic even without a registered handler
	EndSpan(ctx, map[string]any{"result": "ok"}, nil)
}

func TestEndSpan_WithError(t *testing.T) {
	ctx := context.Background()
	ctx = StartSpan(ctx, "test-span", nil)
	EndSpan(ctx, nil, fmt.Errorf("test error"))
}
```

Also add `"context"` and `"fmt"` to the test imports.

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd /Users/wall/study/My-Repo && go test ./pkg/trace/ -v -run TestStartSpan
```

Expected: FAIL (StartSpan not defined)

- [ ] **Step 3: Write the implementation**

Create `pkg/trace/span.go`:

```go
package trace

import (
	"context"
	"time"

	"github.com/cloudwego/eino/callbacks"
	"github.com/cloudwego/eino/components"
)

type spanKey struct{}

type spanData struct {
	name      string
	startTime time.Time
}

// StartSpan creates a manual span for non-Eino components (e.g., memory consolidation).
// The returned context carries span state for EndSpan.
func StartSpan(ctx context.Context, name string, input map[string]any) context.Context {
	info := &callbacks.RunInfo{
		Name:      name,
		Type:      "custom",
		Component: components.ComponentOfToolNode,
	}
	ctx = callbacks.OnStart(ctx, info, input)
	return context.WithValue(ctx, spanKey{}, &spanData{
		name:      name,
		startTime: time.Now(),
	})
}

// EndSpan ends the current span. If err is non-nil, records the error.
func EndSpan(ctx context.Context, output map[string]any, err error) {
	info := &callbacks.RunInfo{
		Name:      "custom-span",
		Type:      "custom",
		Component: components.ComponentOfToolNode,
	}
	if sd, ok := ctx.Value(spanKey{}).(*spanData); ok {
		info.Name = sd.name
	}
	if err != nil {
		callbacks.OnError(ctx, info, err)
		return
	}
	callbacks.OnEnd(ctx, info, output)
}
```

- [ ] **Step 4: Run all trace tests**

```bash
cd /Users/wall/study/My-Repo && go test ./pkg/trace/ -v
```

Expected: all tests PASS

- [ ] **Step 5: Commit**

```bash
git add pkg/trace/span.go pkg/trace/trace_test.go
git commit -m "feat(trace): add manual span helpers for non-Eino components"
```

---

### Task 4: Add TracingConfig to main config schema

**Files:**
- Modify: `pkg/config/schema.go:44-52` (Config struct)

- [ ] **Step 1: Add TracingConfig and Trace field to Config struct**

In `pkg/config/schema.go`, add the `TracingConfig` struct (after `DataConfig`, before the methods):

```go
// TracingConfig holds Langfuse tracing settings.
type TracingConfig struct {
	Enabled   bool   `json:"enabled"`
	Endpoint  string `json:"endpoint"`
	PublicKey string `json:"publicKey"`
	SecretKey string `json:"secretKey"`
}
```

Then add a `Trace` field to the `Config` struct (after `Data`):

```go
type Config struct {
	Agent     AgentConfig                `json:"agent"`
	Model     ModelConfig                `json:"model"`
	Providers map[string]ProviderConfig  `json:"providers,omitempty"`
	Channels  ChannelsConfig             `json:"channels"`
	Gateway   GatewayConfig              `json:"gateway"`
	Tools     ToolsConfig                `json:"tools"`
	Data      DataConfig                 `json:"data"`
	Trace     TracingConfig              `json:"trace"`
}
```

No new imports needed — `TracingConfig` is defined in the same package.

- [ ] **Step 2: Verify it compiles**

```bash
cd /Users/wall/study/My-Repo && go build ./pkg/config/
```

- [ ] **Step 3: Run existing config tests**

```bash
cd /Users/wall/study/My-Repo && go test ./pkg/config/ -v
```

Expected: all existing tests PASS

- [ ] **Step 4: Commit**

```bash
git add pkg/config/schema.go
git commit -m "feat(config): add Trace field to Config schema"
```

---

### Task 5: Wire trace.Init into gateway command

**Files:**
- Modify: `cmd/nanobot/gateway.go:34-128` (runGateway function)

- [ ] **Step 1: Add trace initialization after config loading**

In `cmd/nanobot/gateway.go`, add after `cfg := mustLoadConfig()` (line 40):

```go
	traceShutdown, err := trace.Init(cfg.Trace)
	if err != nil {
		return fmt.Errorf("init tracing: %w", err)
	}
	defer traceShutdown()
```

Add import: `"github.com/wall/nanobot-eino/pkg/trace"`

- [ ] **Step 2: Verify it compiles**

```bash
cd /Users/wall/study/My-Repo && go build ./cmd/nanobot/
```

- [ ] **Step 3: Commit**

```bash
git add cmd/nanobot/gateway.go
git commit -m "feat(gateway): wire trace.Init at startup"
```

---

### Task 6: Wire trace.Init into agent command

**Files:**
- Modify: `cmd/nanobot/agent.go:46-108` (runAgent function)

- [ ] **Step 1: Add trace initialization after config loading**

In `cmd/nanobot/agent.go`, add after `cfg := mustLoadConfig()` (line 56):

```go
	traceShutdown, err := trace.Init(cfg.Trace)
	if err != nil {
		return fmt.Errorf("init tracing: %w", err)
	}
	defer traceShutdown()
```

Add import: `"github.com/wall/nanobot-eino/pkg/trace"`

- [ ] **Step 2: Verify it compiles**

```bash
cd /Users/wall/study/My-Repo && go build ./cmd/nanobot/
```

- [ ] **Step 3: Commit**

```bash
git add cmd/nanobot/agent.go
git commit -m "feat(agent): wire trace.Init at startup"
```

---

### Task 7: Inject Langfuse trace context in processMessage

**Files:**
- Modify: `pkg/app/runloop.go:76-152` (processMessage function)

- [ ] **Step 1: Add trace context injection**

In `pkg/app/runloop.go`, in the `processMessage` function, add after line 93 (`"chat_id", targetChatID,` log line closing paren) and before `turnCtx, turnFlag := tools.NewTurnContext(ctx)` (line 94):

```go
	ctx = langfuse.SetTrace(ctx,
		langfuse.WithSessionID(sessionID),
		langfuse.WithUserID(m.SenderID),
		langfuse.WithName("chat"),
		langfuse.WithMetadata(map[string]string{
			"channel": targetChannel,
			"chat_id": targetChatID,
		}),
	)
```

Add import: `langfuse "github.com/cloudwego/eino-ext/callbacks/langfuse"`

- [ ] **Step 2: Verify it compiles**

```bash
cd /Users/wall/study/My-Repo && go build ./pkg/app/
```

- [ ] **Step 3: Run existing runloop tests**

```bash
cd /Users/wall/study/My-Repo && go test ./pkg/app/ -v -run TestRunInbound
```

Expected: existing tests still PASS

- [ ] **Step 4: Commit**

```bash
git add pkg/app/runloop.go
git commit -m "feat(runloop): inject Langfuse trace context per message"
```

---

### Task 8: Add manual span to memory consolidation

**Files:**
- Modify: `pkg/memory/consolidator.go:119-170` (MaybeConsolidateByTokens method)

- [ ] **Step 1: Wrap ConsolidateMessages call with manual span**

In `pkg/memory/consolidator.go`, in the `MaybeConsolidateByTokens` method, replace line 156:

```go
		if !c.ConsolidateMessages(ctx, chunk) {
```

with:

```go
		ctx = trace.StartSpan(ctx, "Memory Consolidation", map[string]any{
			"session":    s.Key,
			"round":      round,
			"chunk_msgs": len(chunk),
			"estimated":  estimated,
			"target":     target,
		})
		consolidated := c.ConsolidateMessages(ctx, chunk)
		if !consolidated {
			trace.EndSpan(ctx, nil, fmt.Errorf("consolidation failed"))
			return
		}
		trace.EndSpan(ctx, map[string]any{"success": true}, nil)
```

And replace `if !c.ConsolidateMessages(ctx, chunk) {` block (lines 156-158) accordingly.

Add import: `"github.com/wall/nanobot-eino/pkg/trace"`

- [ ] **Step 2: Verify it compiles**

```bash
cd /Users/wall/study/My-Repo && go build ./pkg/memory/
```

- [ ] **Step 3: Run existing memory tests**

```bash
cd /Users/wall/study/My-Repo && go test ./pkg/memory/ -v
```

Expected: existing tests still PASS

- [ ] **Step 4: Commit**

```bash
git add pkg/memory/consolidator.go
git commit -m "feat(memory): add tracing span around consolidation"
```

---

### Task 9: Create Docker Compose for Langfuse

**Files:**
- Create: `docker-compose.langfuse.yml`
- Create: `.env.langfuse.example`

- [ ] **Step 1: Create docker-compose.langfuse.yml**

```yaml
# Langfuse local deployment for Nanobot-Eino tracing.
# Usage: docker compose -f docker-compose.langfuse.yml up -d
# UI: http://localhost:3000

services:
  langfuse:
    image: langfuse/langfuse:3
    ports:
      - "3000:3000"
    environment:
      DATABASE_URL: "postgresql://langfuse:langfuse@postgres:5432/langfuse"
      CLICKHOUSE_URL: "http://clickhouse:8123"
      CLICKHOUSE_MIGRATION_URL: "clickhouse://clickhouse:9000"
      REDIS_HOST: "redis"
      REDIS_PORT: "6379"
      SALT: "nanobot-eino-local-salt"
      ENCRYPTION_KEY: "0000000000000000000000000000000000000000000000000000000000000000"
      NEXTAUTH_SECRET: "nanobot-eino-local-secret"
      NEXTAUTH_URL: "http://localhost:3000"
      LANGFUSE_S3_MEDIA_UPLOAD_ENABLED: "true"
      LANGFUSE_S3_MEDIA_UPLOAD_ENDPOINT: "http://minio:9000"
      LANGFUSE_S3_MEDIA_UPLOAD_ACCESS_KEY_ID: "minioadmin"
      LANGFUSE_S3_MEDIA_UPLOAD_SECRET_ACCESS_KEY: "minioadmin"
      LANGFUSE_S3_MEDIA_UPLOAD_BUCKET: "langfuse"
      LANGFUSE_S3_MEDIA_UPLOAD_REGION: "us-east-1"
      LANGFUSE_S3_MEDIA_UPLOAD_FORCE_PATH_STYLE: "true"
    depends_on:
      postgres:
        condition: service_healthy
      redis:
        condition: service_healthy
      clickhouse:
        condition: service_healthy
      minio:
        condition: service_started

  postgres:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: langfuse
      POSTGRES_PASSWORD: langfuse
      POSTGRES_DB: langfuse
    volumes:
      - langfuse-postgres:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U langfuse"]
      interval: 5s
      timeout: 5s
      retries: 5

  clickhouse:
    image: clickhouse/clickhouse-server:24
    volumes:
      - langfuse-clickhouse:/var/lib/clickhouse
    healthcheck:
      test: ["CMD", "clickhouse-client", "--query", "SELECT 1"]
      interval: 5s
      timeout: 5s
      retries: 5

  redis:
    image: redis:7-alpine
    volumes:
      - langfuse-redis:/data
    healthcheck:
      test: ["CMD", "redis-cli", "ping"]
      interval: 5s
      timeout: 5s
      retries: 5

  minio:
    image: minio/minio:latest
    command: server /data --console-address ":9001"
    environment:
      MINIO_ROOT_USER: minioadmin
      MINIO_ROOT_PASSWORD: minioadmin
    volumes:
      - langfuse-minio:/data
    ports:
      - "9090:9001"

volumes:
  langfuse-postgres:
  langfuse-clickhouse:
  langfuse-redis:
  langfuse-minio:
```

- [ ] **Step 2: Create .env.langfuse.example**

```env
# Langfuse tracing config for Nanobot-Eino
# Copy to your config.yaml trace section or set as environment variables.

# Enable tracing (set to true to activate)
NANOBOT_TRACE_ENABLED=true

# Langfuse server URL (default: local Docker deployment)
NANOBOT_TRACE_ENDPOINT=http://localhost:3000

# Langfuse API keys (create at http://localhost:3000 → Settings → API Keys)
NANOBOT_TRACE_PUBLICKEY=pk-lf-...
NANOBOT_TRACE_SECRETKEY=sk-lf-...
```

- [ ] **Step 3: Commit**

```bash
git add docker-compose.langfuse.yml .env.langfuse.example
git commit -m "infra: add Langfuse Docker Compose for local tracing"
```

---

### Task 10: End-to-end verification

- [ ] **Step 1: Build the project**

```bash
cd /Users/wall/study/My-Repo && go build ./...
```

Expected: no errors

- [ ] **Step 2: Run all tests**

```bash
cd /Users/wall/study/My-Repo && go test ./... -count=1
```

Expected: all tests PASS

- [ ] **Step 3: Verify trace disabled path (no Langfuse server needed)**

```bash
cd /Users/wall/study/My-Repo && go test ./pkg/trace/ -v -run TestInit_Disabled
```

Expected: PASS — confirms zero overhead when tracing is disabled

- [ ] **Step 4: (Manual) Start Langfuse and test end-to-end**

```bash
# Terminal 1: Start Langfuse
docker compose -f docker-compose.langfuse.yml up -d

# Wait for services, then open http://localhost:3000
# Create account, go to Settings → API Keys, copy pk/sk

# Terminal 2: Run agent with tracing
# Add to config.yaml:
#   trace:
#     enabled: true
#     endpoint: "http://localhost:3000"
#     publicKey: "pk-lf-..."
#     secretKey: "sk-lf-..."

./nanobot agent -m "hello"

# Open Langfuse UI → Traces: verify trace appears with LLM generation span
```

- [ ] **Step 5: Final commit (if any fixes needed)**

```bash
git add -A && git commit -m "fix: address issues found during e2e verification"
```
