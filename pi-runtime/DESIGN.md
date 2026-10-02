# nexus-agent-go × Pi Runtime 改造设计（基于仓库勘察的实施方案）

版本：v1.0 · 日期：2026-10-02 · 分支：`feature/intergret-pi-core`
上游依据：《NodePilot_Pi融合改造设计与AI实施指南》（下称《指南》）。本文把《指南》的建议契约落到本仓库的真实代码上：所有"勘察事实"均给出文件与符号证据；与《指南》的差异逐条记录原因。

---

## 1. 勘察结论（P0，已完成）

### 1.1 与《指南》历史线索的对照

| 《指南》历史记述 | 勘察结果 | 证据 |
| --- | --- | --- |
| Go/Gin 后端、JWT、SSE | **已实现** | 路由注册 `internal/gateway/handler.go`；JWT `internal/gateway/auth.go`（HS256、TTL 24h、每次请求回查 DB）；SSE 见下 |
| 五个候选工具 | **全部存在，且实际有 8 个** | `internal/ai/eino_agent.go:408-500`（`buildTools`）：`get_node_metrics`、`list_idle_nodes`、`get_gpu_processes`、`get_node_summary`、`get_alert_history`、`recommend_nodes_for_job`、`explain_node_anomaly`、`search_knowledge_base` |
| 旧 AI 模块为 Eino ChatModelAgent、max 6 轮 | **属实** | `internal/ai/eino_agent.go:20`（`defaultAgentMaxRounds = 6`）、`adk.NewChatModelAgent(... MaxIterations)`（eino_agent.go:147-162） |
| Qdrant、集合 `nodepilot_knowledge` | 集合名**不符** | 实际集合名默认 `knowledge_chunks`（`internal/ai/qdrant_config.go:34`），检索后端 `auto/local/qdrant` 三态可降级 |
| 会话与历史持久化 | **未实现** | `internal/ai/history.go` 仅是内存式 HistoryAnalyzer；无任何 Session/Run/Message 表。首版不建表，见 §7 |
| 断开/取消 | **请求连接绑定执行** | `handler_ai.go:94` 用 `c.Request.Context()` 驱动 `QueryStream`；`ai.Service.queryWithOptionalStream` 的 `safeEmit` 在 ctx 取消时停止。无独立 Run 生命周期 |
| `agent` 目录的角色 | **采集端，与 AI Runtime 无关** | `cmd/agent` + `internal/agent`（collector.go：gopsutil/nvidia-smi 采集）。本次改造不触碰采集进程 |
| 前端 SSE 契约 | **5 类事件** | `internal/ai/types.go:14-27`：`start/status/delta/meta/done/error`；前端解析 `web/dashboard/index.html:4199-4330`（fetch + getReader 手动解析） |

### 1.2 可复用的业务层（Pi 不得复制的部分）

- **Toolbox**（`internal/ai/tools.go:23`）：全部业务查询入口，数据来自 `ClusterDataProvider`/`HistoryProvider` 接口（tools.go:13-20），由 gateway 侧 `AIDataAdapter`（`internal/gateway/ai_adapter.go`）包装 NodeStateService（Redis 热状态）与历史序列实现。
- **LLM 访问**：OpenAI 兼容协议（`internal/ai/openai_client.go`），配置 `AI_PROVIDER/AI_MODEL/AI_BASE_URL/AI_API_KEY`（`internal/ai/service.go:14-32`）。
- **执行器接口**：`ai.QueryExecutor` / `ai.StreamQueryExecutor`（service.go:34-42），SSE 事件结构 `ai.AIStreamEvent`（types.go:93-102）。

### 1.3 Pi SDK 核验（版本锁定 `@earendil-works/pi-coding-agent@1.0.0`）

npm 上 `@mariozechner/pi-coding-agent` 已弃用并指向 `@earendil-works/pi-coding-agent`（最新 1.0.0）。以随包文档 `docs/sdk.md` 与 `examples/sdk/`（与安装版本一致）核验的集成事实：

1. `createAgentSession(options)`：支持 `noTools: "all"`（关闭默认 read/bash/edit/write）、`customTools: ToolDefinition[]`、显式 `model`、`agentDir` 隔离配置目录、`SessionManager.inMemory()` 禁用会话落盘、`resourceLoader`/`settingsManager` 显式替换。
2. **结束信号**：`prompt()` 在本次运行结束后 resolve（含自动重试）；`message_end` 是权威完整消息；`agent_end` 之后仍可能有自动后续，**`agent_settled` 才是"不会再自动继续"的稳定信号**（sdk.md "Subscribing to events"）。
3. 事件：`message_update`（`assistantMessageEvent.type === "text_delta"` 增量文本）、`tool_execution_start {toolCallId, toolName, args}`、`tool_execution_end {toolCallId, toolName, result, isError}`（`dist/core/extensions/types.d.ts`）。
4. 取消：`abort()` 停止当前操作并等待空闲；`session.dispose()` 释放全部资源。
5. 自定义模型：OpenAI 兼容端点经 `models.json`（`ModelRuntime.create({modelsPath})`）配置，`api: "openai-completions"`，apiKey 支持 `$ENV` 插值（docs/models.md "Configure a compatible endpoint"）。**与现有 `AI_BASE_URL`（dashscope compatible-mode）直接兼容。**
6. 工具定义：TypeBox schema 的 `ToolDefinition`，`execute(toolCallId, params, signal, onUpdate, ctx)` 返回 `AgentToolResult`。
7. 完全受控示例（examples/sdk/12-full-control.ts）：空 `ResourceLoader`（不发现宿主扩展/Skill/上下文文件）+ `SettingsManager.inMemory()` + 隔离 `agentDir`。**本方案照此实现，保证不加载宿主个人配置与通用 Shell 工具。**

《指南》§4.3 要求的"最小可运行验证"将在实现后以 mock OpenAI 服务器跑通（§8）。

---

## 2. 总体架构（职责边界）

```
Web 前端（SSE 契约不变）
  │ POST /api/ai/query
  ▼
Go Gateway ── 认证/JWT、Run 生命周期、SSE 编码、Run 凭证签发
  │ ① HTTP + NDJSON（POST /v1/runs, POST /v1/runs/:id/cancel）
  ▼
Pi Runtime（TS，独立进程）── 模型 Loop、上下文、会话缓存、事件适配
  │ ② HTTP + JSON（POST /internal/api/tools/:name，Run 凭证）
  ▼
Go Tool Gateway（gateway 进程内路由组）── 校验/授权/审计/超时/裁剪
  ▼
ai.Toolbox（既有业务函数）→ Redis/MySQL/Qdrant/agent
```

- Pi Runtime 只持有模型凭证与内部调用凭证，**不接触业务数据库**；模型参数一律视为不可信输入。
- Go Tool Gateway 是 gateway 进程内的一组内部路由（《指南》§5 允许），不是新进程。
- Gateway 只调用 Pi Runtime；运行时不可达时返回错误，不回退到本地规则或 Eino。

## 3. Go ↔ Pi 契约（NDJSON）

### 3.1 启动 Run：`POST {PI_RUNTIME_URL}/v1/runs`（请求）

```json
{
  "protocol_version": "1",
  "run_id": "uuid",
  "session_id": "uuid（首版=run_id）",
  "input": {"message": "看看 node-03 的 GPU 为什么忙"},
  "context": {
    "known_nodes": ["node-01", "node-03"],
    "locale_hint": "zh",
    "recent_entries": []
  },
  "policy": {
    "enabled_tool_names": ["get_node_metrics", "search_knowledge_base"],
    "deadline_at": "2026-10-02T04:00:00Z"
  },
  "model_profile": {"provider": "nexus-llm", "model_id": "qwen3.5-122b-a10b"}
}
```

响应：`200 Content-Type: application/x-ndjson`，每行一个事件（§3.2），流结束即 Run 终态。
重复 `run_id` 启动：返回 `409`，不二次执行。
鉴权：`X-Nexus-Internal-Token: {PI_RUNTIME_TOKEN}`（共享密钥，双向往来均校验）。

### 3.2 事件信封（`internal/runtime/events.go` ↔ `pi-runtime/src/protocol.ts`）

```json
{"protocol_version":"1","run_id":"…","seq":3,"type":"message.delta","timestamp":"…","payload":{…}}
```

| type | payload 要点 | Go 侧映射（→ ai.AIStreamEvent） |
| --- | --- | --- |
| `run.started` | `{}` | `status{phase:"thinking"}` |
| `message.delta` | `{text}` | `delta{text}` |
| `message.completed` | `{text}` | （仅记录，用于终态兜底） |
| `tool.started` | `{tool_call_id, tool_name, arguments}` | `status{phase:"tooling", message:toolStatusMessage(name)}`（复用 `eino_agent.go:926` 的文案）；登记 ToolCallRecord + related_nodes |
| `tool.completed` | `{tool_call_id, tool_name, ok, result}` | 提取证据（knowledge_hits/retrieval/警告）；失败计入 warnings |
| `run.completed` | `{text, finish_reason}` | 组装 `AIQueryResponse`（复用 `parseAgentFinalContent` 解析 JSON 终稿）→ `meta` + `done` |
| `run.failed` | `{error}` | `error{code}` |
| `run.cancelled` | `{reason}` | 静默结束（SSE 连接已断/用户取消场景） |

未知事件类型：记日志并忽略（前向兼容）。`seq` 由 Runtime 按 Run 单调分配。

### 3.3 取消：`POST /v1/runs/:runId/cancel`

Go 在（a）SSE 客户端断开、（b）显式取消、（c）deadline 到期时调用；Runtime 调 `session.abort()` 并在清理后回吐 `run.cancelled`。两条连接（NDJSON 流与工具回调）互相独立，Go 以 `run_id` 索引取消上下文（《指南》§13）。

## 4. Pi → Go 工具契约

### 4.1 `POST /internal/api/tools/:name`

请求头：`X-Nexus-Internal-Token`（进程间密钥）+ `Authorization: Bearer {run_token}`（Go 签发的 Run 凭证）。
请求体：`{"run_id":"…","tool_call_id":"…","arguments":{…}}`

成功：`{"ok":true,"data":{…},"meta":{"observed_at":"…","retrieved_at":"…","stale":false,"truncated":false}}`
失败：`{"ok":false,"error":{"code":"NODE_NOT_FOUND","message":"…","retryable":false}}`

### 4.2 Run 凭证（《指南》§7.3 的最小实现）

HMAC-SHA256 签名 JSON：`{run_id, user_id, username, role, exp}`（密钥 `PI_RUN_TOKEN_SECRET`，默认回落 `JWT_SECRET`）。Tool Gateway 校验：签名、exp、`run_id` 处于活跃 Run 表、工具在 `policy.enabled_tool_names` 内。Run 终态后拒绝新工具调用。

### 4.3 工具白名单与参数校验（`internal/ai/toolapi.go`）

8 个既有工具全部包装（§1.1），参数校验规则与 `eino_agent.go:408-500` 的 schema 一致；新增输出上限（单响应 256KB、进程列表截断 64 条、`truncated` 标记）。**授权边界说明**：本仓库目前没有节点级 ACL（所有登录用户共享集群视图），本版不动这个语义；边界 = Run 绑定 + 工具白名单 + 只读工具集。若未来引入 ACL，在 Tool Gateway 补资源级过滤即可（《指南》§7.3 预留位）。

## 5. Pi Runtime 设计（`pi-runtime/`）

```
pi-runtime/
  package.json            # 锁定 @earendil-works/pi-coding-agent@1.0.0
  tsconfig.json
  src/
    config.ts             # 环境变量解析
    protocol.ts           # §3/§4 契约类型（与 Go 侧镜像）
    nexus-tools.ts        # 8 个 TypeBox 工具定义 → HTTP 回调 Go
    gateway-client.ts     # 工具回调客户端（凭证注入、超时、错误分层）
    session-factory.ts    # createAgentSession 全受控装配（§1.3-7 模式）
    runner.ts             # Run 执行器：事件订阅→信封、取消、限制
    server.ts             # HTTP 服务：/healthz /v1/runs /v1/runs/:id/cancel
  models.json             # 由启动脚本从 AI_* 环境变量生成
  test/e2e-mock.mjs       # mock OpenAI 服务器端到端验证
```

关键决策：
- **无状态进程 + 有界会话缓存**：`session_id → AgentSession` 的 LRU（默认 128、TTL 30min）。同会话温缓存时天然多轮连续；冷启动则用 `context.recent_entries` 以有界文本前导恢复（首版有损恢复，不做条目级导入——见 §7 与《指南》P3/P4 分期）。
- **每会话串行 Run**：同 session 并发 Run 返回 409 busy（首版不允许 steering/follow-up）。
- **限制执行点**：deadline（Go 传入 + Runtime 侧 abort）、单 Run 工具调用上限（`PI_MAX_TOOL_CALLS`，默认 12）、单工具回调超时（`PI_TOOL_TIMEOUT_SEC`，默认 10s）。超限 → `run.failed{error:"budget_exceeded"}`。
- **模型装配**：启动时写 `models.json`：`{providers:{"nexus-llm":{baseUrl:$AI_BASE_URL, api:"openai-completions", apiKey:"$AI_API_KEY", models:[{id:$AI_MODEL}]}}}`；`ModelRuntime.create({authPath, modelsPath})` 指向运行时私有目录。
- **受控资源**：空 `ResourceLoader` + `agentDir` 指向私有目录 + `noTools:"all"` + `customTools`（仅 8 个只读工具）+ `tools: enabled_tool_names` 白名单 + `SessionManager.inMemory()` + `SettingsManager.inMemory({compaction:{enabled:false}})`（首版关闭自动压缩，Run 级预算兜底；SDK 压缩接入为 P4）。

## 6. Go 侧改动清单

| 文件 | 内容 |
| --- | --- |
| internal/runtime/config.go | Pi Runtime URL/token and run/tool limits; Runtime URL is required |
| `internal/runtime/events.go`（新） | 事件信封/类型（§3.2） |
| `internal/runtime/client.go`（新） | NDJSON 客户端：StartRun（bufio.Scanner，MaxToken 1MB，UTF-8 安全按行解析）、CancelRun、健康检查 |
| `internal/runtime/manager.go`（新） | Run 状态机 `CREATED→RUNNING→COMPLETED/FAILED/CANCELLED(+CANCELLING)`、原子终态转换、活跃 Run 查询、终态 TTL 后 GC |
| `internal/runtime/token.go`（新） | Run 凭证签发/校验（HMAC） |
| `internal/ai/toolapi.go`（新） | 工具分发器：8 工具的参数校验与执行、输出裁剪、错误→`ok/error.code` 分层 |
| `internal/ai/pi_executor.go`（新） | 实现 `QueryExecutor`/`StreamQueryExecutor`：调 runtime client、事件映射（§3.2）、终稿组装（复用 `parseAgentFinalContent` 等） |
| internal/ai/pi_service.go | Pi-only AI gateway service; delegates shared health and knowledge operations |
| `internal/gateway/tool_gateway.go`（新） | 内部路由组 `/internal/api/tools/:name`：双凭证中间件、Run 活跃校验、审计日志（工具名/耗时/结果摘要，不落参数原文） |
| cmd/gateway/main.go | Always wires Pi Runtime; startup fails when PI_RUNTIME_URL is missing |
| `docker-compose.yml` / `.env.example` | 新增 `pi-runtime` 服务与配置项 |

Eino/规则执行器源文件仍保留，但 Gateway 不再构造或调用它们。前端 SSE 契约和 internal/agent 采集端保持不变。

## 6.3 Runtime 失败行为

Pi Runtime 启动失败或运行中断时，Gateway 将错误返回给前端。请求不会回退到规则模式或 Eino 执行器。
## 7. 首版不做（与《指南》分期对应）

MySQL 会话/Run/ToolCall 持久化（P3）、SDK Compaction 接入（P4）、Skill 按需读取（P4）、断线后 Run 存活与事件重放（§12.2 后续形态）、多实例部署（P3）、写类工具（远程 Shell/杀进程等，指南 §2.2 明确排除）。
已知限制如实声明：会话历史首版为有损恢复；`run.failed` 后不自动续跑；Pi 进程重启导致活跃 Run 中断（Go 侧标记失败）。

## 8. 验证方案

1. **Go 单测**：事件映射、Run 凭证签发/校验/过期、NDJSON 解析（跨包 UTF-8、超长行）、工具分发器（缺参/未知工具/截断）、Run 状态机竞争（完成 vs 取消）。
2. **Runtime E2E（确定性，无真实模型 key）**：`test/e2e-mock.mjs` 启动脚本化 OpenAI Chat Completions mock（第 1 次响应 `tool_calls:get_node_metrics`，第 2 次响应 JSON 终稿）+ 受控工具回调 stub，断言 NDJSON 事件序列与终稿内容。
3. **全链路（Go 集成测试）**：Go 测试进程内起 httptest 工具网关 + 假 ClusterDataProvider，子进程启动真实 Pi Runtime（模型指向 mock OpenAI），走完 `SSE → Go → Pi(真 SDK) → 工具回调 → Go(假数据) → 终稿` 全环。标注：真实供应商模型链路需 `AI_API_KEY`，另行人工验证。
4. **验收场景**（对应《指南》§16.1）：mock 场景覆盖"工具调用→继续推理→JSON 终稿→取消"四类；"越权工具名""过期凭证"由单测覆盖（401/403/409 路径）。
