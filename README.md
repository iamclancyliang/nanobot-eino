# Nanobot-Eino

Go 语言重写的个人 AI 助手，基于 [Cloudwego Eino](https://github.com/cloudwego/eino) 框架，复刻 [nanobot](https://github.com/openclaw/openclaw) 的核心架构。

---

## 特性

- **ReAct Agent Loop** — 基于 Eino React Agent 实现多步推理 + 工具调用循环
- **持久化记忆** — Token 感知的自动记忆整理，MEMORY.md + HISTORY.md 双层存储
- **多渠道接入** — 飞书 WebSocket 实时通信，可扩展其他渠道
- **丰富工具集** — 文件系统、Shell、Web 搜索/抓取、MCP 协议、定时任务、消息发送
- **子任务系统** — 后台 Subagent 独立执行长耗时任务，完成后自动通知
- **定时任务** — Cron / 一次性定时任务，自然语言创建和管理
- **心跳唤醒** — 定期读取 HEARTBEAT.md，LLM 自主决定是否行动
- **技能扩展** — 与 Python nanobot 对齐的 Skill 系统，8 个内建技能，支持 always-on 和按需加载
- **MCP 延迟连接** — MCP 服务器首次收到消息时才建立连接，避免启动阻塞
- **Prompt Cache 友好** — Append-only 消息列表 + 静态 System Prompt，最大化 LLM 缓存命中

---

## 架构

```
┌──────────────────────────────────────────────────────────────────┐
│                     Channels (飞书 WebSocket)                     │
│                  onMessage → PublishInbound                       │
└──────────────────────────────────────────────────────────────────┘
                                │
                                ▼
┌──────────────────────────────────────────────────────────────────┐
│                        MessageBus                                │
│            Inbound → Agent.ChatStream                            │
│            Outbound → Channel.Send                               │
└──────────────────────────────────────────────────────────────────┘
                                │
                                ▼
┌──────────────────────────────────────────────────────────────────┐
│                    Agent (Eino React)                             │
│  Session → Memory Consolidation → Prompt Building → LLM Stream   │
└──────────────────────────────────────────────────────────────────┘
        │              │              │              │
        ▼              ▼              ▼              ▼
   ┌─────────┐  ┌───────────┐  ┌──────────┐  ┌────────────┐
   │ Session  │  │  Memory   │  │   Cron   │  │ Heartbeat  │
   │ (JSONL)  │  │ (MD文件)  │  │ (调度器)  │  │  (定期)    │
   └─────────┘  └───────────┘  └──────────┘  └────────────┘
```

---

## 快速开始

### 前置条件

- Go 1.23+
- LLM API Key（OpenAI / 通义千问 / 硅基流动 / 火山方舟 / Google Gemini / Ollama）

### 安装

```bash
git clone https://github.com/wall/nanobot-eino.git
cd nanobot-eino
go mod download
```

### 初始化

```bash
go run ./cmd/nanobot onboard
```

创建 `~/.nanobot-eino/` 目录和默认配置文件。

### 交互对话

```bash
go run ./cmd/nanobot agent
```

直接在终端与 Agent 对话，支持历史记录、Markdown 渲染。

### 启动完整服务

```bash
go run ./cmd/nanobot gateway
```

启动飞书 Channel + Heartbeat + Cron 全套服务。

---

## CLI 命令

| 命令 | 说明 |
|------|------|
| `go run ./cmd/nanobot gateway` | 启动完整服务（渠道 + Agent + 心跳 + 定时任务） |
| `go run ./cmd/nanobot agent` | 交互式对话（`-m "msg"` 单次模式） |
| `go run ./cmd/nanobot onboard` | 初始化配置和 workspace |
| `go run ./cmd/nanobot status` | 显示当前配置和状态 |
| `go run ./cmd/nanobot version` | 版本信息 |

对话中可用：`/new` 新会话、`/stop` 终止任务、`/help` 帮助。

---

## 配置

默认配置文件位于 `~/.nanobot-eino/config.*`（如 `config.yaml` / `config.json`），支持 YAML / JSON / TOML 格式。
建议只保留一个配置文件，避免多个同名配置并存导致生效值混淆。可通过 `go run ./cmd/nanobot status` 确认当前实际生效配置。

```yaml
model:
  type: openai          # openai / qwen / siliconflow / ark / google(gemini) / ollama
  baseUrl: ""
  apiKey: "sk-..."
  model: "gpt-4o"

agent:
  contextWindowTokens: 65536
  maxStep: 20

channels:
  feishu:
    appId: "cli_xxx"
    appSecret: "xxx"

tools:
  workspace: "nanobot-eino"
  web:
    search:
      provider: "tavily"
      apiKey: "tvly-xxx"
  exec:
    timeout: "60s"
  mcp:
    - name: "my-mcp"
      command: "npx"
      args: ["-y", "@my/mcp-server"]

gateway:
  heartbeat:
    interval: "30m"
  cron:
    storePath: "./data/jobs.json"
```

Google Gemini 配置示例（`config.yaml`）：

```yaml
model:
  type: google          # 或 gemini
  # Gemini 官方 API 一般不需要 baseUrl，留空即可
  baseUrl: ""
  # 推荐留空并通过环境变量 NANOBOT_MODEL_API_KEY 注入
  apiKey: ""
  # 可按需替换为你有权限的 Gemini 模型
  model: "gemini-2.5-flash"
```

最小可运行环境变量：

```bash
export NANOBOT_MODEL_TYPE=google
export NANOBOT_MODEL_API_KEY="your-google-api-key"
export NANOBOT_MODEL_NAME="gemini-2.5-flash"
```

硅基流动（SiliconFlow）配置示例（`config.yaml`）：

```yaml
model:
  type: siliconflow
  # 需要在配置中显式填写 SiliconFlow OpenAI 兼容地址
  baseUrl: "https://api.siliconflow.cn/v1"
  apiKey: "sk-xxxx"
  model: "Qwen/Qwen3-8B"
```

环境变量覆盖（`NANOBOT_` 前缀）始终可用。

---

## 工具

| 工具 | 功能 |
|------|------|
| `read_file` / `write_file` / `edit_file` / `list_dir` | 文件读写和目录浏览 |
| `shell` | 执行 Shell 命令（可配置超时和黑白名单） |
| `web_search` | Web 搜索（Tavily / Brave / Jina） |
| `web_fetch` | 抓取网页并转为 Markdown |
| `message` | 通过 MessageBus 向渠道发送消息 |
| `cron` | 创建 / 删除 / 列出定时任务 |
| `spawn` | 启动后台 Subagent 子任务 |
| MCP | 通过 Model Context Protocol 接入外部工具 |

---

## 技能系统

Skill 系统支持 8 个内建技能：

| 技能 | 说明 | 依赖 |
|------|------|------|
| memory | 双层记忆系统（MEMORY.md + HISTORY.md） | — (always-on) |
| weather | 天气查询（curl + wttr.in） | curl |
| summarize | URL/文件/视频摘要 | summarize CLI |
| skill-creator | 创建/更新 Agent 技能 | — |
| github | 通过 gh CLI 操作 GitHub | gh |
| cron | 定时提醒和周期任务 | — |
| tmux | 远程控制 tmux 会话 | tmux |
| clawhub | 从 ClawHub 搜索安装技能 | — |

技能加载优先级：workspace (`{workspace}/skills/`) > builtin (`configs/skills/`)。

技能以 SKILL.md 文件定义，包含 YAML frontmatter（name、description、metadata）和 Markdown 正文。`always: true` 的技能全文注入 system prompt；其他技能以 XML 摘要注入，agent 按需用 `read_file` 加载。

---

## 记忆系统

- **MEMORY.md** — 长期记忆，由 LLM 使用 `save_memory` 工具整理，每次覆写
- **HISTORY.md** — 对话历史摘要日志，只追加不修改

整理策略：
- Token 估算超过上下文窗口 50% 时自动触发
- 在 user 消息边界处切割，不破坏对话轮次
- 多轮整理（最多 5 轮），目标压缩到窗口一半
- 连续 3 次 LLM 整理失败自动降级为原始归档

---

## 项目结构

```
nanobot-eino/
├── cmd/
│   ├── nanobot/             # Cobra CLI 入口
│   │   ├── main.go          #   根命令
│   │   ├── gateway.go       #   gateway 子命令
│   │   ├── agent.go         #   agent 子命令
│   │   ├── onboard.go       #   onboard 子命令
│   │   └── status.go        #   status 子命令
│   ├── server/main.go       # 独立服务入口（legacy）
│   └── cli/main.go          # 简易 REPL（legacy）
├── pkg/
│   ├── agent/               # Agent Loop（核心）
│   ├── bus/                  # MessageBus（入站/出站路由）
│   ├── channels/            # 渠道适配（飞书）
│   ├── config/              # 配置加载（Viper）
│   ├── cron/                # 定时任务（robfig/cron）
│   ├── heartbeat/           # 心跳服务
│   ├── memory/              # 记忆存储 + 整理器
│   ├── model/               # LLM 模型工厂
│   ├── prompt/              # Prompt 模板加载
│   ├── session/             # 会话管理（JSONL）
│   ├── skill/               # 技能管理器
│   ├── subagent/            # 子任务管理器
│   ├── tools/               # 工具实现 + 封装
│   └── workspace/           # Workspace 模板同步
├── configs/
│   ├── prompts/             # 默认 Prompt（SOUL / USER / TOOLS / AGENTS / HEARTBEAT）
│   └── skills/              # 内建技能（memory / weather / summarize / skill-creator / github / cron / tmux / clawhub）
├── data/                    # 运行时数据（sessions / memory / jobs）
├── go.mod
├── Dockerfile
└── note.md                  # 设计笔记
```

---

## 核心依赖

| 依赖 | 用途 |
|------|------|
| [cloudwego/eino](https://github.com/cloudwego/eino) | AI 应用框架（React Agent / Schema / Compose） |
| eino-ext/model/* | LLM 模型扩展（OpenAI / Ollama / Ark / Gemini） |
| eino-ext/tool/mcp | MCP 工具集成 |
| [spf13/cobra](https://github.com/spf13/cobra) | CLI 框架 |
| [spf13/viper](https://github.com/spf13/viper) | 配置管理 |
| [charmbracelet/glamour](https://github.com/charmbracelet/glamour) | 终端 Markdown 渲染 |
| [peterh/liner](https://github.com/peterh/liner) | 终端行编辑 / 历史记录 |
| [robfig/cron](https://github.com/robfig/cron) | Cron 调度器 |
| [larksuite/oapi-sdk-go](https://github.com/larksuite/oapi-sdk-go) | 飞书 SDK |
| [modelcontextprotocol/go-sdk](https://github.com/modelcontextprotocol/go-sdk) | MCP 协议 SDK |

---

## Docker

```bash
docker build -t nanobot-eino .
docker run -v $(pwd)/data:/root/data nanobot-eino
```

---

## 链路监控（Tracing）

基于 Eino 框架的全局 Callback 机制，集成 [Langfuse](https://langfuse.com) 实现完整链路追踪，覆盖 LLM 调用、工具执行、记忆整理全流程。

### 启动 Langfuse

```bash
docker compose -f docker-compose.langfuse.yml up -d
```

等待服务启动后访问 `http://localhost:3000`，注册账号并在 Settings → API Keys 创建密钥。

### 配置

在 `~/.nanobot-eino/config.yaml` 中添加：

```yaml
trace:
  enabled: true
  endpoint: "http://localhost:3000"
  publicKey: "pk-lf-..."
  secretKey: "sk-lf-..."
```

或通过环境变量：

```bash
export NANOBOT_TRACE_ENABLED=true
export NANOBOT_TRACE_ENDPOINT=http://localhost:3000
export NANOBOT_TRACE_PUBLICKEY=pk-lf-...
export NANOBOT_TRACE_SECRETKEY=sk-lf-...
```

### 使用

配置完成后，正常启动 agent 或 gateway 即可：

```bash
go run ./cmd/nanobot agent          # 交互模式
go run ./cmd/nanobot agent -m "hi"  # 单次模式
go run ./cmd/nanobot gateway        # 完整服务
```

打开 Langfuse UI (`http://localhost:3000`) → Traces 页面，可以看到：

- **LLM 调用**：模型名称、输入/输出消息、Token 用量（prompt/completion/total）、延迟
- **工具执行**：工具名称、入参 JSON、返回结果、延迟、错误信息
- **记忆整理**：consolidation 触发时机、处理消息数、成功/失败状态

每条 Trace 关联 Session ID 和 User ID，支持按会话、用户筛选。

### 关闭 Tracing

设置 `trace.enabled: false`（或不配置 trace 段）即可完全关闭，零运行时开销。

### 架构

```
Nanobot-Eino
  │
  ├── Eino Global Callback ──▶ Langfuse Handler
  │     (自动追踪所有 ChatModel / Tool)
  │
  ├── pkg/trace/
  │     ├── trace.go   # Init/Shutdown，全局 handler 注册
  │     └── span.go    # 手动埋点辅助（用于记忆整理等非 Eino 组件）
  │
  └── docker-compose.langfuse.yml
        ├── Langfuse Web UI (:3000)
        ├── PostgreSQL
        ├── ClickHouse
        ├── Redis
        └── MinIO
```

---

## 致谢

- [OpenClaw](https://github.com/openclaw/openclaw) — 灵感来源
- [nanobot](https://github.com/nanobot/nanobot) — Python 原版实现
- [Cloudwego Eino](https://github.com/cloudwego/eino) — Go AI 应用框架
