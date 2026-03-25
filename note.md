# Nanobot-Eino 记忆管理方案

基于 Python nanobot 的记忆系统严格复刻到 Go/Eino 版本。

---

## 架构概览

```
┌─────────────────────────────────────────────────┐
│              MemoryConsolidator                 │
│  (策略层: 何时整理? 整理多少? 并发控制)           │
│                                                 │
│  ┌───────────────────────────────────────────┐  │
│  │             MemoryStore                   │  │
│  │  (存储层: 文件读写 + LLM 总结)             │  │
│  │                                           │  │
│  │  MEMORY.md ← 长期记忆 (覆写更新)           │  │
│  │  HISTORY.md ← 历史日志 (追加写入)           │  │
│  └───────────────────────────────────────────┘  │
│                                                 │
│  Session.Messages ──[LastConsolidated]──► 待整理 │
└─────────────────────────────────────────────────┘
```

**Python → Go 对应关系：**

| Python nanobot | Go nanobot-eino | 文件 |
|---|---|---|
| `MemoryStore` | `MemoryStore` | `pkg/memory/memory.go` |
| `MemoryConsolidator` | `MemoryConsolidator` | `pkg/memory/consolidator.go` |
| `Session` | `Session` | `pkg/session/manager.go` |
| `SessionManager` | `SessionManager` | `pkg/session/manager.go` |
| `AgentLoop._process_message` | `Agent.ChatStream` | `pkg/agent/agent.go` |

---

## 数据流转

```
用户发消息 → InboundMessage(channel, chatID)
    │
    ▼
sessionID = "feishu:oc_xxx"
    │
    ▼
SessionManager.GetOrCreate(sessionID)
  → 缓存命中? 返回
  → 磁盘加载? 反序列化 JSONL → Session 对象
  → 都没有? 新建 Session
    │
    ▼
MaybeConsolidateByTokens(session)  ← 处理前检查
    │
    ▼
session.GetHistory(0) → Messages[LastConsolidated:] → 给 LLM
    │
    ▼
React Agent Stream 运行 (LLM + 工具调用循环)
    │
    ▼
session.AddMessage(user + assistant) → 追加到 Messages
sessions.Save(session) → 重写 JSONL 到磁盘
    │
    ▼
MaybeConsolidateByTokens(session)  ← 处理后再检查
  → 超窗口? → 取旧消息块 → LLM 总结 → 写 MEMORY.md/HISTORY.md
  → 更新 LastConsolidated → sessions.Save()
```

---

## 核心组件

### 1. MemoryStore (`pkg/memory/memory.go`)

纯文件存储层，无 SQLite 依赖（与 Python 版完全一致）：

- **MEMORY.md**：长期记忆 —— 当前有效的事实和上下文，每次整理时**整体覆写**
- **HISTORY.md**：历史日志 —— 按时间追加的摘要条目，**只追加不修改**，可 grep 搜索

核心方法：
- `Consolidate()` — 调用 LLM 让其使用 `save_memory` 工具，提取结构化的摘要和记忆更新
- `failOrRawArchive()` — 连续 3 次 LLM 整理失败后，直接将原始消息文本归档，不丢数据
- 覆写前比较：如果 `memory_update` 与当前 MEMORY.md 内容相同，跳过写入

### 2. MemoryConsolidator (`pkg/memory/consolidator.go`)

策略控制层，决定**何时整理**、**整理多少**：

- **Token 估算**：`estimatePromptTokens()` — 系统 prompt 基础开销 + 记忆上下文 + 未整理消息，按 chars/3 近似（nanobot使用openai的tiktoken算法估算）
- **边界选择**：`PickConsolidationBoundary()` — 在 user 消息处切割，不破坏对话轮次
- **多轮整理**：`MaybeConsolidateByTokens()` — 目标压缩到上下文窗口的 **50%**，最多 5 轮
- **Per-session 锁**：`sync.Map` + `sync.Mutex`，防止同一 session 的并发整理

### 3. Session (`pkg/session/manager.go`)

消息的单一事实源（Single Source of Truth）：

- `AddMessage()` — 追加消息并更新时间戳
- `GetHistory(maxMessages)` — 返回 `Messages[LastConsolidated:]`，对齐到 user 轮次起点
- `Clear()` — 重置消息和 LastConsolidated（用于 `/new` 命令）
- `LastConsolidated` 指针 — 分隔"已归档"和"活跃"部分，消息列表只追加不删除

### 4. Agent (`pkg/agent/agent.go`)

消息生命周期管理：

```go
ChatStream(sessionID, input):
  1. Per-session Lock (sync.Mutex)
  2. GetOrCreate(session)
  3. MaybeConsolidateByTokens(session)  // 处理前
  4. session.GetHistory(0)
  5. buildMessages(system + skills + memory + history + input)
  6. reactAgent.Stream(messages)
  7. goroutine: 读取流 → 收集响应 → 保存轮次 → MaybeConsolidateByTokens  // 处理后
  8. 释放锁
```

---

## 关键设计决策

### 1. 移除 SQLite，Session 作为唯一消息源

**旧方案（有问题）**：消息同时写入 SQLite + Session JSONL，双源不一致，Consolidator 读 SQLite 但 `LastConsolidated` 在 Session 上。

**新方案（对齐 Python）**：Session.Messages 是唯一消息存储，JSONL 持久化。SQLite 完全移除。

### 2. 基于 Token 的自动整理

旧方案用 `totalChars/4 < tokenLimit` 做粗糙判断且只整理一次。新方案：
- 估算完整 prompt token 数（系统 prompt + 记忆上下文 + 未整理历史）
- 只在超过上下文窗口时触发
- 目标压缩到窗口的 50%，留足余量

### 3. 多轮整理循环（最多 5 轮）

单次整理可能不够（超长对话），每轮整理后重新估算 token，循环直到达标或无法继续。

### 4. 用户轮次边界对齐

`PickConsolidationBoundary()` 只在 `role=user` 的消息处切割，确保不会丢失一轮对话的上下文。

### 5. 三级降级

1. LLM 正常调用 `save_memory` 工具 → 结构化归档
2. LLM 未调用工具 → 计入失败计数
3. 连续 3 次失败 → 原始消息直接追加到 HISTORY.md（[RAW ARCHIVE] 标记）

### 6. 处理前 + 处理后双向整理

**处理前**：防止历史消息已过长导致本轮 LLM 调用失败
**处理后**：本轮新增的消息可能又把上下文撑爆了

### 7. 流式响应场景下的锁管理

锁在 `ChatStream` 开始时获取，在流式传输的 goroutine 结束后释放（`defer lock.Unlock()`），确保消息保存和后置整理都在锁的保护下完成。

---

## LLM Prompt Cache 优化策略

### 背景：Prompt Caching 原理

LLM 提供商（OpenAI、Anthropic、火山方舟等）支持 Prompt Caching：如果连续请求的 prompt 前缀相同，后端复用 KV cache，显著降低延迟和 input token 费用。

一个典型的 prompt 结构：

```
[system prompt] + [history msg1] + [msg2] + ... + [msgN] + [new user input]
```

每轮对话只在末尾追加消息，**前缀不变** → 缓存命中 → 费用降低。

### 三种场景的缓存影响

| 场景 | 缓存 | 频率 | 说明 |
|------|------|------|------|
| 正常对话 | **命中** | 每轮 | Messages append-only，前缀稳定 |
| 自动整理 (`MaybeConsolidateByTokens`) | **未命中** | 低频 | `LastConsolidated` 推进 → history 变化；MEMORY.md 更新 → system prompt 变化 |
| `/new` 清空 | **完全未命中** | 极低频 | 全部消息清空，仅保留 system prompt + 新 memory |

### 已采用的优化措施（与 Python nanobot 一致）

**1. 消息列表 append-only**

`Session.Messages` 只追加不修改。整理时 `LastConsolidated` 指针前移、`GetHistory()` 返回子切片，但原始列表不变。这保证了正常对话中 prompt 前缀的稳定性，每轮都命中缓存。

Python nanobot 的 Session docstring 明确写到：
> "Messages are append-only for LLM cache efficiency."

**2. System prompt 不含时间变量**

Python nanobot 将时间戳放在 user message 中（`_RUNTIME_CONTEXT_TAG`），而非 system prompt 里。专门有测试保证：

```python
# test_context_prompt_cache.py
def test_system_prompt_stays_stable_when_clock_changes():
    """System prompt should not change just because wall clock minute changes."""
    prompt1 = builder.build_system_prompt()  # 13:59
    prompt2 = builder.build_system_prompt()  # 14:00
    assert prompt1 == prompt2
```

Go 实现同样：`prompt.Loader.BuildSystemMessages()` 读取静态文件（SOUL.md、USER.md 等），不含任何时间变量。

**3. 整理阈值足够高，减少触发频率**

只有当估算 token 超过上下文窗口的 50%（如 65536 的一半 = 32768）时才触发整理。大多数对话永远不会达到此阈值，因此绝大多数请求都是纯 append-only → 缓存命中。

**4. MEMORY.md 变更不频繁**

MEMORY.md 内容注入到 system prompt 中。由于只在整理时更新，system prompt 在正常对话轮次间完全稳定。

### 不可避免的 Cache Miss 场景

以下 cache miss 是设计上的有意取舍：

- **自动整理**：`LastConsolidated` 推进后 history 前缀变化 + MEMORY.md 更新导致 system prompt 变化。这是必须的——不整理就会 context overflow。
- **`/new` 命令**：用户主动触发，一次性 cache miss。旧消息已归档到 MEMORY.md/HISTORY.md，新会话通过 `GetMemoryContext()` 注入长期记忆实现跨会话延续。

**结论**：cache miss 的额外费用 ≈ 一次完整 system prompt 的 input token（通常几千 token），远低于持续发送无限增长消息列表的费用。这是以极低频的 cache miss 换取持续可控的上下文窗口。

---

## 文件变更清单

| 文件 | 变更 |
|---|---|
| `pkg/session/manager.go` | 新增 `AddMessage`/`GetHistory`/`Clear` 方法，修复 `load()` 类型断言安全性 |
| `pkg/memory/memory.go` | **完全重写** → `MemoryStore`（移除 SQLite，纯文件存储 + LLM 整理） |
| `pkg/memory/consolidator.go` | **完全重写** → `MemoryConsolidator`（Token 估算、边界选择、多轮循环） |
| `pkg/agent/agent.go` | **重构** → 接管 SessionManager，处理前后双向整理，流式保存轮次 |
| `cmd/server/main.go` | **简化** → 移除 SQLite 初始化和双写逻辑，消息保存下沉到 Agent |

---

# Agent 设计开发：难点、方案 Tradeoff 与实现

基于 OpenClaw 和 Claude Code 两个主流 Agent 框架的最佳实践，对 nanobot-eino 在 Agent 设计开发中遇到的关键问题进行总结和对比分析。

---

## 一、三个框架的架构对比

### 1.1 整体架构分层

| 层次 | OpenClaw | Claude Code | nanobot-eino |
|------|----------|-------------|--------------|
| **上下文治理** | MEMORY.md + daily logs + SQLite embeddings 索引 | CLAUDE.md + rules + skills（分 always-loaded 和 on-demand） | SOUL.md + USER.md + MEMORY.md（全静态文件，无 embedding） |
| **行动层** | 内建工具 + TOOLS.md 定义 + Skills | 内建工具（Bash/Edit/Grep）+ MCP + Skills | Eino tool.InvokableTool + MCP + Skill Manager |
| **控制层** | 权限沙箱 + 工具黑白名单 | Hooks（enforced behaviors）+ 权限模型 | RestrictToWorkspace + Shell deny/allow patterns |
| **隔离层** | 无明确子 agent 层 | Subagents（context-isolated workers） | SubagentManager（goroutine 级隔离） |
| **验证层** | 无内建验证循环 | Verifiers（tests / lint / CI 验证闭环） | 无（依赖用户工具链） |

### 1.2 Agent Loop 核心差异

**OpenClaw** 采用标准 ReAct 循环：Load context → Call LLM → Parse response → Execute tool → Append result → Loop。每轮迭代向上下文追加 token，直到 LLM 输出最终文本。特点是**纯顺序执行**，工具调用无并行。

**Claude Code** 的核心理念是 **Model-Driven Autonomy**——由模型决定下一步，而非硬编码脚本。架构上强调"五个关键表面"（Context / Action / Control / Isolation / Verification）的治理。Loop 本身与 OpenClaw 类似，但增加了 auto-compaction（自动压缩）和 semantic search 保护上下文。

**nanobot-eino** 基于 Eino React Agent 实现同构的 ReAct 循环，但关键创新在于：
- **双向整理**（pre/post consolidation）：处理前后各检查一次是否需要整理
- **流式管道 + goroutine**：`schema.Pipe` + goroutine 实现异步消费，锁在 goroutine 结束后释放
- **MCP 延迟连接**：`sync.Once` 保证首次消息时才连接 MCP，避免启动阻塞

---

## 二、关键设计难点与解决思路

### 2.1 上下文窗口管理

**难点**：长对话中 token 持续增长，超出上下文窗口导致 API 报错或被截断。这是所有 Agent 框架的核心挑战。

**OpenClaw 方案**：
- 多层记忆（MEMORY.md + daily logs + SQLite embeddings）
- 基于 Ebbinghaus 遗忘曲线的 decay-based forgetting
- Similarity 阈值 0.85 合并重复记忆
- Hooks 在 `onPostTurn`、`onSessionEnd`、定时 Cron 三个生命周期点触发整理
- 近期引入 Zettelkasten + PersonalizedPageRank 做关联记忆检索

**Claude Code 方案**：
- Auto-compaction 机制
- Subagent 返回结果是"摘要"而非完整中间过程（只有 final result 回流到 parent）
- 但存在已知问题：多个 subagent 并行返回大结果可能撑爆 parent 上下文（Issue #23463 记录了 7 个并行子任务产生 ~150K 字符导致会话崩溃）

**nanobot-eino 方案**：
- **Token 估算**：`chars/3` 近似（不依赖 tiktoken，避免 Go 生态缺少精确 tokenizer 的问题）
- **50% 阈值触发**：只有估算超过上下文窗口一半才触发，留足余量给工具调用和回复
- **多轮循环**：单次整理可能不够，最多 5 轮，每轮重新估算
- **用户边界对齐**：`PickConsolidationBoundary()` 只在 `role=user` 处切割，不破坏对话轮次
- **三级降级**：LLM 成功 → 累计失败 → 3 次后原始归档，保证不丢数据

**Tradeoff**：
- `chars/3` vs tiktoken：精度换取零依赖、零延迟。实测大部分场景偏差 <15%，对于"是否需要整理"的判断足够准确。只有当消息量刚好在阈值边界时会有偏差，但多轮循环兜底了这种情况。
- 50% 阈值 vs 更激进的整理：过早整理导致更多 cache miss 和 LLM 调用费用，过晚整理有 overflow 风险。50% 是 OpenClaw 验证过的经验值。
- 无 embedding 索引：OpenClaw 用 SQLite + 向量检索做记忆搜索，但代价是依赖 embedding 模型和复杂部署。nanobot-eino 选择纯文件方案，用户可通过 grep HISTORY.md 搜索历史，复杂度极低但查询能力有限。

### 2.2 流式响应 + 并发锁管理

**难点**：Agent 的流式响应在 goroutine 中异步消费，但必须保证消息保存和后置整理在同一 session 锁的保护下完成。锁释放时机是核心问题。

**OpenClaw 方案**：per-session lane 序列化，session 写锁在流式传输前获取，streaming 完成后释放。

**Claude Code 方案**：单用户场景（CLI），并发问题不突出。Subagent 通过独立 context window 天然隔离。

**nanobot-eino 方案**：

```go
lock.Lock()
// ... build messages, start stream ...
go func() {
    defer lock.Unlock()  // goroutine 结束后释放
    // 消费 stream → 收集响应 → 保存轮次 → 后置整理
}()
return stream  // 立即返回给调用方
```

关键设计：
- `sessionLocks sync.Map` 存储 per-session 的 `sync.Mutex`
- 锁在 `ChatStream` 开始时获取，但 **在 goroutine 中 defer 释放**
- 整个流式传输期间锁都被持有，保证同一 session 的消息不会交叉
- 后置整理也在锁的保护下完成，防止并发整理竞争

**Tradeoff**：
- 锁在整个流式传输期间持有意味着同一 session 的并发消息会排队等待，牺牲了吞吐量换取一致性。对个人助手场景完全可接受（用户不会同时在一个对话中发多条消息），但多用户高并发场景需要重新设计。
- `sync.Map` 的 per-session 粒度比全局锁细得多，不同 session 完全并行。

### 2.3 Subagent 子任务系统

**难点**：后台执行长耗时任务时，如何隔离上下文、限制工具权限、安全取消、通知完成。

**OpenClaw 方案**：无明确的 Subagent 层，通过 Hook + Cron 实现后台行为。

**Claude Code 方案**：
- 完整的 Subagent 架构，每个子 agent 拥有独立的 context window
- 只有 final result 回流到 parent，中间步骤不污染上下文
- 支持并行调度，但有已知问题：多个子任务的结果合并可能导致 parent context overflow
- Subagent 类型化（Explore / Bash / Plan / General-purpose），明确职责边界

**nanobot-eino 方案**：
- `SubagentManager` 管理后台 goroutine，每个子任务创建独立的 React Agent 实例
- **工具集受限**：子 agent 只能用 filesystem + shell + web，**不能用 message/spawn/cron/MCP**，防止递归 spawn 和意外副作用
- **通知机制**：完成后通过 `bus.PublishInbound` 注入 `channel: "system"` 消息，主 agent 在下次交互时感知到
- **取消支持**：`context.WithCancel` + `runningTasks sync.Map` + `sessionTasks sync.Map`，支持按 session 批量取消

```go
// 工具集隔离的核心逻辑
func NewSubagentManager(...) *SubagentManager {
    toolCfg.MessageBus = nil  // 禁止消息发送
    toolCfg.MCP = nil         // 禁止 MCP 调用
    // spawn tool 本身也不注入子 agent，天然防递归
}
```

**Tradeoff**：
- **goroutine vs 独立进程**：Claude Code 的 subagent 是独立进程，资源隔离更彻底但开销大。nanobot-eino 用 goroutine，零开销启动但共享进程内存。对于个人助手的子任务（读文件、执行命令），goroutine 足够。
- **通知延迟**：子任务完成后通过 MessageBus 注入系统消息，但用户必须发起下一次交互才会被 agent 看到。这不如 Claude Code 的即时回流，但避免了主动推送中断用户的问题。
- **MaxStep = 15**：子 agent 硬限 15 步（主 agent 20 步），防止失控循环。Claude Code 的 Issue #31689 证明了这种限制的必要性——无限制的子任务会耗尽 token 预算。

### 2.4 Tool Result 截断

**难点**：工具返回结果可能非常大（文件内容、命令输出），直接塞入上下文会浪费 token 甚至导致溢出。

**OpenClaw 方案**：工具输出在 Agent Loop 层面截断，但具体阈值和策略在运行时配置中。

**Claude Code 已知问题**（2026 年 Issues）：
- 工具输出无统一截断机制，依赖各工具自行限制
- grep 搜索 session 文件导致递归膨胀，session 文件从 KB 膨胀到数百 MB（Issue #23196）
- Subagent 不感知 `MAX_OUTPUT_TOKENS` 环境变量，Write 工具在非默认设置下静默截断（Issue #31689）

**nanobot-eino 方案**：
- **统一 Wrapper 层**：`tools.WrapTools()` 给所有工具套上截断 + 进度报告
- **ToolResultMaxChars = 16000**：统一阈值，截断后附带原始长度信息
- **Web Fetch 单独限制**：`MaxChars: 50000`（HTML 转 Markdown 后截断）
- **Shell MaxOutput**：命令输出独立限制（默认 10000 字符）

```go
// 截断逻辑在 wrapper 层统一处理
if w.maxChars > 0 && len(result) > w.maxChars {
    result = result[:w.maxChars] + fmt.Sprintf(
        "\n\n... (truncated, showing %d of %d characters)",
        w.maxChars, originalLen)
}
```

**Tradeoff**：
- 16000 字符的硬截断可能丢失关键信息，但比 Claude Code 无截断导致 session 膨胀好得多
- Wrapper 层统一处理 vs 每个工具自行截断：前者代码简洁、不会遗漏，但丧失了 per-tool 的智能截断能力（如只保留 JSON 的 key 而非 value）

### 2.5 MCP 延迟连接

**难点**：MCP 服务器可能启动时不可用（网络问题、进程未就绪），导致 Agent 启动阻塞或失败。

**OpenClaw 方案**：启动时连接，连接失败直接报错。

**Claude Code 方案**：MCP 服务器声明式配置，按需连接。

**nanobot-eino 方案**：
- MCP 配置在 `NewAgent` 时保存，但不连接
- 首次收到真实消息时 `sync.Once` 触发连接
- 连接成功后重建 React Agent（因为 Eino 的 `react.Agent` 在创建时绑定工具列表，不支持动态添加）
- 连接失败的 MCP 服务器被跳过（warning log），不阻塞其他工具

```go
func (a *Agent) ensureMCPConnected(ctx context.Context) {
    a.mcpOnce.Do(func() {
        // 逐个连接，失败的跳过
        // 成功后重建 reactAgent
    })
}
```

**Tradeoff**：
- **延迟连接 vs 启动时连接**：延迟连接让服务启动更快、更健壮，但首次消息处理会有额外延迟
- **重建 Agent vs 动态工具注入**：Eino 框架限制导致必须重建 Agent 实例。如果 Eino 支持运行时工具注册，可以避免这个开销。目前通过 `sync.Once` 保证只重建一次。

### 2.6 Message 去重与 TurnContext

**难点**：Agent 可能在一轮对话中通过 `message` 工具主动发送了回复，如果 server 端又发送最终回复，用户会收到重复消息。

**OpenClaw 方案**：`_sent_in_turn` 标记跟踪当前轮次是否已发送消息。

**Claude Code 方案**：CLI 场景，不存在此问题。

**nanobot-eino 方案**：
- `TurnContext` 通过 `context.Value` 在工具调用链中传递
- `atomic.Bool` 标记 `messageSent`，message tool 发送后设置
- Server 端检查 `WasMessageSent()` 决定是否发送最终回复

```go
type TurnContext struct {
    messageSent atomic.Bool
}
```

**Tradeoff**：
- `context.Value` 传递 vs 全局状态：context 方式更 Go-idiomatic，天然线程安全，但每个需要感知状态的地方都必须从 context 中提取。
- `atomic.Bool` vs `sync.Mutex`：单个 bool 标记用 atomic 即可，无需 mutex 的开销。

---

## 三、框架选型与 Eino 生态的影响

### 3.1 为什么选 Eino 而非 LangChain-Go / LlamaIndex-Go

**决策因素**：
- Eino 是字节跳动开源的 Go AI 框架，国内生态支持好（火山方舟 Ark、通义千问 Qwen 原生适配）
- 提供开箱即用的 React Agent（`flow/agent/react`），不需要手写 ReAct 循环
- Schema 层（`schema.Message`、`schema.StreamReader`）与 OpenAI API 对齐
- MCP 支持通过 `eino-ext/components/tool/mcp` 集成

**Eino 带来的约束**：
- `react.Agent` 创建时绑定工具列表，不支持运行时动态修改 → 导致 MCP 延迟连接必须重建 Agent
- `ToolCallingChatModel` 接口不是所有模型都支持 → 需要 `if tcm, ok := chatModel.(ToolCallingChatModel)` 的运行时类型判断
- `StreamToolCallChecker` 需要手动消费 StreamReader 来判断是否有工具调用 → 实现略冗余

### 3.2 vs OpenClaw 的 TypeScript 生态

OpenClaw 用 TypeScript 实现（占比 87.8%），天然享受：
- npm 生态的海量工具包
- `async/await` 的异步编程模型比 Go 的 goroutine + channel 更直觉
- 社区活跃（320K stars），问题响应快

nanobot-eino 选 Go 的优势：
- 单二进制部署，无 Node.js 运行时依赖
- goroutine 天然并发，无 callback hell
- 内存占用远低于 Node.js（对自部署场景重要）
- 类型系统更严格，编译时捕获更多错误

---

## 四、尚未解决的问题与未来方向

### 4.1 记忆搜索能力不足

当前 HISTORY.md 只能通过 `grep` 搜索，无法做语义检索。OpenClaw 的 embedding + 向量搜索方案能力更强，但部署复杂度也高得多。可能的中间方案：
- 轻量级的 BM25 全文索引（Go 原生实现，如 bleve）
- 或直接依赖 LLM 的长上下文能力（Claude 4.6 已支持 200K+ token 上下文，记忆整理的必要性在降低）

### 4.2 验证闭环缺失

Claude Code 的 Verification Surface（tests / lint / CI）在 nanobot-eino 中没有对应实现。当 Agent 执行文件写入或代码生成后，没有自动化验证机制。未来可通过 Skill 实现（如 `lint-check` skill），但缺少框架级别的 Hook 支持。

### 4.3 Subagent 结果回流策略

当前子任务通过 MessageBus 注入系统消息，但时机不可控（依赖用户下次交互）。更好的方案可能是：
- 实时推送到渠道（飞书消息）通知用户
- 或将结果写入文件，在下次 prompt 构建时自动加载

### 4.4 多 Agent 协作

Claude Code 2026 引入了实验性的 Agent Teams（多个 agent 共享任务列表、直接通信）。nanobot-eino 目前只有 parent-child 单向关系。若需要多 agent 协作，MessageBus 的 pub/sub 模型天然支持扩展，但需要设计 agent 间的任务分配和冲突解决机制。

---

## 五、运行实操补充（2026-03）

### 5.1 Workspace 命名建议

为了与项目名保持一致，建议在配置中显式设置：

```json
"tools": {
  "workspace": "nanobot-eino"
}
```

不要依赖默认值（默认仍可能是 `data`），否则 `status` 输出与预期目录命名会不一致。

### 5.2 配置文件并存的排查建议

`~/.nanobot-eino/` 下若同时存在多个 `config.*`（例如 `config.yaml` 与 `config.json`），可能出现“改了文件但状态没变”的错觉。实操建议：

1. 同时检查 `config.yaml` 和 `config.json`
2. 优先确保只保留一个主配置文件
3. 每次改完后执行 `go run ./cmd/nanobot status` 验证生效值

---

## 六、工具处理链路优化沉淀（2026-03-18）

### 6.1 目标

将工具调用失败从“中断主流程”改为“可回喂给 LLM 的结构化失败信息”，让模型在下一轮自动进行降级决策（重试参数、换工具、改方案），并补齐用户侧兜底体验。

### 6.2 方案总览

本次落地分两层：

1. **工具层统一降级（wrapper）**
   - 文件：`pkg/tools/wrapper.go`
   - 核心点：
     - 对 `InvokableRun` 的 `error` 统一转为结果文本：`Error executing <tool>: ...`
     - 对以 `Error` 开头的工具结果统一视为失败结果
     - 所有失败结果自动追加引导：
       `[Analyze the error above and try a different approach.]`
     - 进度状态统一：异常或失败文本都标记为 `failed`

2. **Agent 层优雅处理（Eino ToolsNodeConfig）**
   - 文件：`pkg/agent/agent.go`（`newReactAgent`）
   - 利用 Eino 原生能力：
     - `UnknownToolsHandler`：模型幻觉工具名时，不直接硬失败；返回可恢复错误 + 可用工具列表 + 引导提示
     - `ToolArgumentsHandler`：统一参数预处理（空参数归一为 `{}`、剥离 ```json 围栏、非法 JSON 降级为可解析 payload）

3. **非工具层兜底（runloop）**
   - 文件：`pkg/app/runloop.go`
   - 当 `ChatStream` 启动失败，或流接收异常且无有效内容时，统一返回：
     - `Sorry, I encountered an error.`
   - 目的：避免静默失败，确保用户始终有反馈。

### 6.3 设计取舍

- **优先连续性**：即使工具失败，也尽量让主回路继续，让 LLM 自主恢复。
- **错误信息可消费**：失败文本可直接进入下一轮上下文，避免“只有日志有错、模型看不到错”。
- **最小侵入**：复用 Eino `ToolsNodeConfig`，不自建冗余调度框架。
- **兼容现有工具**：当前各工具中大量 `return "Error: ...", nil` 的风格可以直接受益。

### 6.4 测试与验证

本次变更已通过：

- `go test ./pkg/tools ./pkg/app ./pkg/agent`

新增/更新了以下测试覆盖：

- `pkg/tools/wrapper_test.go`
  - 工具异常降级为错误文本 + 引导提示
  - `Error:` 结果自动追加引导提示
- `pkg/agent/agent_test.go`
  - 参数归一逻辑（空参数、json fence、非法 JSON）
  - 工具名收集与排序
- `pkg/app/runloop_test.go`
  - `ChatStream` 失败时用户兜底回复
  - 流式接收异常且无内容时兜底回复

### 6.5 后续可选增强

- 将失败引导提示抽成可配置项（配置文件开关 + 文案）。
- 对 `ToolArgumentsHandler` 增加“按工具 schema 的细粒度参数校验”。
- 对错误结果打统一机器可读标签（如 `metadata.tool_error=true`）便于可观测平台统计。

---

# Skills 系统设计开发笔记

## 7. Skills 系统对齐 Python nanobot

### 7.1 背景

Python nanobot 的 Skills 是扩展 agent 能力的核心机制：每个 Skill 是一个目录，包含 `SKILL.md` 文件（YAML frontmatter + Markdown body），告诉 LLM 何时、如何使用某项能力。Skills 不是工具本身，而是**工具使用指南**——LLM 根据 Skill 描述决定是否调用对应工具（如 `exec`、`read_file`、`gh` CLI 等）。

nanobot-eino 需要与 Python 版严格对齐，确保行为一致。

### 7.2 难点

1. **Go 无 YAML frontmatter 标准库**
   - Python 用正则 `^---\n(.*?)\n---` 匹配即可，Go 也采用了同样的正则方案（`regexp.MustCompile`），而非引入 YAML 库（如 `gopkg.in/yaml.v3`），因为 frontmatter 字段简单，手动 `split(":", 1)` 即可解析。
   - 权衡：引入 YAML 库可支持更复杂的 frontmatter（嵌套、多行值），但当前字段全是单行 key-value，手动解析更轻量。

2. **metadata 字段内嵌 JSON**
   - `metadata` 字段值是一个 JSON 字符串（`{"nanobot":{"emoji":"🐙","requires":{"bins":["gh"]}}}`），frontmatter 解析只拿到第一层字符串，再用 `encoding/json` 二次解码。
   - 权衡：可以把 metadata 拆成多个 frontmatter 字段（`emoji`、`requires_bins` 等），但这会偏离 Python 版的 SKILL.md 格式，破坏兼容性。

3. **两级加载优先级**
   - workspace skills > builtin skills，同名 skill workspace 覆盖 builtin。
   - 实现：先扫描 workspace 目录，再扫描 builtin 目录，已存在的 name 跳过。
   - 难点在于 Go map 无序遍历，但 LoadSkills 总是先 workspace 后 builtin，保证优先级正确。

4. **依赖检查跨平台一致性**
   - `requires.bins` 用 `exec.LookPath`（等价 Python `shutil.which`），`requires.env` 用 `os.Getenv`。
   - macOS/Linux 行为一致；Windows 下 `LookPath` 需注意 `.exe` 后缀，但 nanobot 主要跑在 Unix 环境。

5. **System Prompt 拼接方式的微妙差异**
   - Python 用 `"\n\n---\n\n".join(parts)` 拼接各段（identity、active skills、skills summary、memory）。
   - Go 原版用 `+=` 直接拼接，段间无分隔符。
   - 对齐后改为 `strings.Join(parts, "\n\n---\n\n")`。这个分隔符让 LLM 更清晰地区分各段，是 Python 版的设计意图。

6. **Weather Tool 方案选择**
   - Python 版 weather 是纯 SKILL.md 方案：告诉 agent 用 `curl wttr.in` 获取天气，不依赖专用工具。
   - Go 版原来有一个专用的 `weather.NewWeatherTool()` Go 工具，直接 HTTP 调用 wttr.in。
   - 对齐选择：删除 Go 专用工具，改为纯 SKILL.md + `exec` 工具方案，与 Python 一致。
   - 权衡：专用工具更可靠（不依赖系统 `curl`），但不符合 Skills 设计哲学——Skills 应引导 agent 使用已有通用工具，而非为每个能力写专用工具。

### 7.3 方案 Tradeoff

| 方案 | 优势 | 劣势 | 选择 |
|------|------|------|------|
| 手动解析 frontmatter | 零依赖，轻量 | 不支持复杂 YAML（多行值、嵌套） | ✅ 采用 |
| 引入 YAML 库 | 完整 YAML 支持 | 多一个依赖，当前不需要 | ❌ |
| metadata 拆成独立字段 | 解析更简单 | 破坏与 Python 的 SKILL.md 兼容性 | ❌ |
| metadata 内嵌 JSON | 与 Python 完全兼容 | 需要二次 JSON 解码 | ✅ 采用 |
| Weather 专用 Go 工具 | 更可靠，不依赖 curl | 偏离 Python 设计哲学 | ❌ 删除 |
| Weather 纯 SKILL.md | 与 Python 一致，agent 自主使用 curl | 依赖系统 curl | ✅ 采用 |
| `LoadSkill` 返回 parsed content | 内存效率高 | 与 Python API 不一致 | ❌ 改为读文件 |
| `LoadSkill` 返回 raw file | 与 Python 一致 | 每次调用读一次磁盘 | ✅ 采用 |

### 7.4 实现清单

共完成 8 项任务：

1. **修复硬编码路径** — `NewAgent` 新增 `builtinSkillsDir` 参数，删除 `agent.go` 中硬编码的 `"configs/skills"` 和 `"data"`，改用 `toolCfg.Workspace` 和配置值。同步更新 6 个调用点（4 个主入口 + 2 个测试入口）。

2. **对齐 system prompt 拼接** — `buildMessages` 改为 `strings.Join(parts, "\n\n---\n\n")`，与 Python `ContextBuilder.build_system_prompt` 一致。

3. **补齐内建 Skills** — 新增 `github`、`cron`、`tmux`、`clawhub` 4 个 SKILL.md，从 Python 版复制适配。内建 skills 从 4 个增加到 8 个。

4. **对齐 Weather Skill** — 删除 `pkg/skill/weather/weather.go` 专用工具，更新 `configs/skills/weather/SKILL.md` 为纯 curl 方案，新增 `requires.bins: ["curl"]`。

5. **新增 `ListAvailableSkills`** — 对齐 Python `list_skills(filter_unavailable=True)`，只返回依赖满足的 skills。

6. **新增 `GetSkillMetadata`** — 对齐 Python `get_skill_metadata(name)`，返回 frontmatter 字段的 map。

7. **优化 `stripFrontmatter`** — 不再重复读文件，改为直接操作内容字符串。`LoadSkill` 改为返回 raw file content（含 frontmatter），对齐 Python。

8. **测试覆盖** — 新增 `TestListAvailableSkills`、`TestGetSkillMetadata`、`TestLoadSkillReturnsRawContent`、`TestStripFrontmatter`；更新 `TestLoadRealConfigSkills` 验证 8 个内建 skills 及 weather 的 curl 依赖。

### 7.5 变更文件清单

| 文件 | 操作 | 说明 |
|------|------|------|
| `pkg/agent/agent.go` | 修改 | 新增 `builtinSkillsDir` 参数、对齐 prompt 拼接、移除 weather 工具注册和 import |
| `pkg/skill/manager.go` | 修改 | 新增 `ListAvailableSkills`、`GetSkillMetadata`、优化 `stripFrontmatter`、`LoadSkill` 返回 raw content |
| `pkg/skill/manager_test.go` | 修改 | 新增 4 个测试、更新集成测试验证 8 个 skills |
| `pkg/skill/weather/` | 删除 | 移除 weather 专用工具（weather.go + weather_test.go） |
| `cmd/nanobot/agent.go` | 修改 | 传入 `cfg.Agent.BuiltinSkillsDir` |
| `cmd/nanobot/gateway.go` | 修改 | 传入 `cfg.Agent.BuiltinSkillsDir` |
| `cmd/server/main.go` | 修改 | 传入 `cfg.Agent.BuiltinSkillsDir` |
| `cmd/cli/main.go` | 修改 | 传入 `cfg.Agent.BuiltinSkillsDir` |
| `test/cmd/test_agent/main.go` | 修改 | 传入 `"configs/skills"` |
| `test/cmd/test_skills/main.go` | 修改 | 传入 `"configs/skills"` |
| `configs/skills/weather/SKILL.md` | 修改 | 替换为 Python 版 curl 方案 |
| `configs/skills/github/SKILL.md` | 新增 | 从 Python 版适配 |
| `configs/skills/cron/SKILL.md` | 新增 | 从 Python 版适配 |
| `configs/skills/tmux/SKILL.md` | 新增 | 从 Python 版适配 |
| `configs/skills/clawhub/SKILL.md` | 新增 | 从 Python 版适配 |

### 7.6 验证

- `go build ./...` 编译通过
- `go test ./pkg/... -count=1` 全部 15 个包测试通过
- `go run ./cmd/nanobot agent -m "你有哪些skills？"` 正确列出 6 个可用 + 2 个不可用 skills

---

## 8. web_fetch 超长结果处理方案

### 8.1 问题背景

web_fetch 抓取网页内容时，页面可能非常长（几十万字符），但 LLM context window 有限。当前 nanobot-eino 采用两层简单截断：
1. **web_fetch 层**（`pkg/tools/web.go`）：Jina Reader 返回上限 50,000 字符
2. **wrapper 层**（`pkg/tools/wrapper.go`）：所有工具结果统一截断到 16,000 字符（`ToolResultMaxChars`）

截断导致大量信息直接丢失，用户无法获取页面后半部分的内容。

### 8.2 业界调研

| 产品 | 处理方式 |
|------|----------|
| **Claude Code** | 子模型摘要：抓取全文（上限 ~10MB / 100K markdown），用快速小模型 + 用户 prompt 做定向摘要 |
| **Python nanobot** | 简单截断，无分页无摘要 |

### 8.3 三种可选方案

#### 方案 A：增强截断（Enhanced Truncation）

**思路**：保持截断机制不变，但改进截断策略和信息提示。

**改动点**：
- 截断时保留头部 + 尾部内容（而非只保留头部）
- 返回结果中明确告知总字符数、已截断比例
- 可选：返回页面目录/大纲（如提取所有 `<h1>`-`<h3>` 标签）帮助用户定位

**优点**：实现最简单，零额外成本，无额外延迟
**缺点**：信息丢失严重，用户无法获取中间部分内容
**复杂度**：低
**适合场景**：快速改进，对准确性要求不高的场景

#### 方案 B：子模型摘要（Sub-model Summarization）⭐ 推荐

**思路**：当内容超过阈值（如 32K 字符）时，将全文 + 用户 prompt 发给轻量 LLM 做定向摘要提取。

**数据流**：
```
用户调用 web_fetch(url, prompt?)
    │
    ▼
Jina Reader 抓取完整页面（上限 ~100K）
    │
    ▼
内容 ≤ 32K？──是──▶ 直接返回原文
    │
   否
    ▼
构造摘要请求，调用轻量模型 Generate()
    │
    ▼
返回摘要结果（1K-5K 字符），失败则 fallback 到截断
```

**改动点**：
- `webFetchArgs` 新增 `Prompt string` 可选参数
- `ToolConfig` 新增 `SummaryModel` 字段
- 超阈值时调用摘要模型，失败 fallback 到截断
- 无 prompt 时使用默认通用提取 prompt

**关键设计决策**：
1. **摘要模型**：默认复用主模型，可选配置覆盖为更便宜的模型
2. **摘要阈值**：32K 字符（Claude Code 用 100K，nanobot 当前 wrapper 截断 16K，取中间值）
3. **无 prompt 时**：用默认 prompt「提取主要内容、核心观点、关键数据」
4. **摘要失败**：fallback 到简单截断，不影响主流程
5. **wrapper 层截断**：保留作为最后兜底

**优点**：信息保留率最高（~90%+ 关键信息），对齐 Claude Code 成熟实践，用户体验好
**缺点**：每次超长请求多一次 LLM 调用（+2-5s 延迟 + 额外 token 成本），摘要质量依赖 prompt
**复杂度**：中
**适合场景**：追求信息质量，对齐业界最佳实践

#### 方案 C：分页读取（Pagination with startIndex）

**思路**：允许用户通过 `startIndex` 参数多次调用 web_fetch，每次获取一个固定窗口的内容。

**改动点**：
- `webFetchArgs` 新增 `StartIndex int` 参数（从第 N 个字符开始读取）
- 每次返回固定窗口（如 16K 字符）+ 剩余字符数提示
- LLM 可自主决定是否继续翻页读取

**示例交互**：
```
第1次: web_fetch(url) → 返回 0-16K + "剩余 48K 字符，使用 startIndex=16000 继续"
第2次: web_fetch(url, startIndex=16000) → 返回 16K-32K + "剩余 32K 字符"
第3次: web_fetch(url, startIndex=32000) → 返回 32K-48K + ...
```

**优点**：信息零丢失（理论上可读完全文），LLM 自主决定何时停止，实现较简单
**缺点**：多次调用增加总延迟和 token 消耗，LLM 可能不知道何时停止翻页，每页独立截断可能切断语义
**复杂度**：中
**适合场景**：需要完整内容但不确定哪部分重要的场景

### 8.4 方案对比

| 维度 | 方案 A 增强截断 | 方案 B 子模型摘要 | 方案 C 分页读取 |
|------|----------------|------------------|----------------|
| 实现复杂度 | 低 | 中 | 中 |
| 信息保留率 | ~20% | ~90% | ~100%（多次调用） |
| 额外延迟 | 无 | +2-5s（一次 LLM 调用） | +N×原始延迟（N 次翻页） |
| 额外成本 | 无 | 一次轻量模型调用 | N 次 fetch + 更多 context token |
| 业界采用 | 少 | Claude Code | 部分 RAG 系统 |
| 对齐 Python 版 | 是（当前行为） | 超越对齐 | 超越对齐 |

### 8.5 面试要点

- **问**：web_fetch 结果超长怎么处理？
- **答**：调研了三种方案。推荐方案 B（子模型摘要），这是 Claude Code 的做法——抓取全文后用轻量模型 + 用户 prompt 做定向提取，信息保留率最高。实现上在 `ToolConfig` 中注入摘要模型，超阈值时调用，失败 fallback 到截断。方案 A（增强截断）最简单但信息丢失多，方案 C（分页）信息零丢失但多轮调用成本高。
- **关键词**：两层截断、子模型摘要、graceful fallback、prompt 引导提取

---

# Nanobot-Eino 链路监控方案选型

## 1. 背景

需要给 Nanobot-Eino 添加链路监控能力，追踪 Agent 每步推理过程（LLM 调用、工具执行、记忆整理），用于个人开发调试、prompt 优化和问题排查。

## 2. Eino 框架 Callback 机制

Eino 提供了完整的 Callback 系统（`github.com/cloudwego/eino/callbacks`），支持 5 个生命周期时点：

| 时点 | 触发场景 |
|------|---------|
| `OnStart` | 组件处理前（非流式输入） |
| `OnEnd` | 处理成功完成（非流式输出） |
| `OnError` | 组件返回错误 |
| `OnStartWithStreamInput` | 流式输入变体 |
| `OnEndWithStreamOutput` | 流式输出变体 |

注册方式：
- **全局注册**：`callbacks.AppendGlobalHandlers(handler)` — 启动时调用一次，所有 Eino 组件自动被追踪
- **按次注册**：`compose.WithCallbacks(handler)` — 单次图执行
- **按节点注册**：`compose.WithCallbacks(handler).DesignateNode("nodeName")`

## 3. eino-ext 官方 Tracing 后端

eino-ext 提供 4 个官方 callback handler：

| 后端 | 包路径 | 说明 |
|------|--------|------|
| **Langfuse** | `callbacks/langfuse` | 开源 MIT，功能最丰富（batching、采样、脱敏） |
| **CozeLoop** | `callbacks/cozeloop` | 字节跳动平台，Eino 生态最紧密 |
| **LangSmith** | `callbacks/langsmith` | LangChain 生态，闭源服务器 |
| **APMPlus** | `callbacks/apmplus` | 基于 OpenTelemetry，火山引擎 APM |

## 4. 方案对比

| 维度 | Langfuse | CozeLoop | LangSmith | OTel (自定义) |
|------|----------|----------|-----------|--------------|
| **开源协议** | MIT | Apache 2.0 | 闭源 (仅 SDK) | CNCF 标准 |
| **GitHub Stars** | 23.7k | 5.4k | ~814 (SDK) | N/A |
| **自部署** | Docker Compose (PG+CH+Redis) | Docker/Helm | 仅 Enterprise (K8s, 16C/64G+) | 无 UI |
| **云服务** | 免费层 50k 事件/月 | 免费 OSS | $39/seat 起 | 无 |
| **Token 追踪** | 内置 | 内置 | 内置 | 需自己实现 |
| **成本分析** | 内置 | 有限 | 内置 | 需 DIY |
| **Prompt 管理** | 版本化 + Playground | 调试/评估 | Prompt Hub | 无 |
| **评估能力** | LLM-as-Judge + 人工标注 | 自动化评估引擎 | 完整套件 | 无 |
| **Eino Callback** | eino-ext 有实现 (最丰富) | eino-ext 有实现 (最紧密) | eino-ext 有实现 (基础) | 需自己写 Handler |
| **社区活跃度** | 极高 (ClickHouse 收购) | 中等 (2025.7 开源) | 高 (LangChain 生态) | 极高 (CNCF) |
| **锁定风险** | 低 (MIT + OTel 兼容) | 中 (字节生态) | 高 (闭源) | 无 |
| **维护复杂度** | 中 (自部署组件多) | 中 (项目年轻) | 低(云)/极高(自部署) | 低 (只是库) |

## 5. 最终选择：Langfuse

**选择理由：**

1. **个人调试场景最匹配** — Langfuse 的 Trace 可视化（嵌套调用树 + Token 用量 + 延迟）完全覆盖需求
2. **社区最大、MIT 开源** — 23.7k stars，2026.1 被 ClickHouse 收购保证长期维护
3. **eino-ext callback 实现最完善** — 支持 batching、采样、脱敏，开箱即用
4. **OTel 兼容** — 原生支持 OTLP 协议，将来切换后端无需改代码
5. **本地 Docker 部署** — 数据全在本地，隐私可控

**排除理由：**

- **CozeLoop**：项目只开源 8 个月，社区以中文为主，生产验证较少
- **LangSmith**：闭源 + 高价 + 自部署门槛极高，与 Eino 生态无天然亲和力
- **OTel 自定义**：GenAI Semantic Conventions 仍实验阶段，无现成 UI，个人调试性价比低

## 6. 实现方案

**核心思路：** 利用 Eino 全局 Callback 机制，在程序启动时注册 Langfuse handler，所有 ChatModel 和 Tool 调用自动被追踪，现有业务代码零侵入。

**新增包：** `pkg/trace/` — 封装 Init/Shutdown 和手动埋点辅助

**追踪粒度：**
- LLM 调用（自动）：模型名、输入/输出消息、Token 用量、延迟
- 工具执行（自动）：工具名、入参、返回结果、延迟、错误
- 记忆整理（手动埋点）：通过 `trace.StartSpan/EndSpan` 包裹 consolidation

**改动量：** ~180 行新 Go 代码，现有文件改动 < 25 行

**关键 API：**
- `langfuse.NewLangfuseHandler(cfg) → (handler, flusher)` — 创建 handler
- `callbacks.AppendGlobalHandlers(handler)` — 全局注册
- `langfuse.SetTrace(ctx, opts...)` — 注入 trace 上下文（session/user）

**配置：** 通过 `trace.enabled` 开关控制，不配置时零开销
