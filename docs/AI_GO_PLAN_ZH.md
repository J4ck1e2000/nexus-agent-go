# Nexus Agent Go: AI + Go 技术栈升级详细计划（2026-03-17）

## 1. 目标和原则

### 1.1 总目标
- 让当前监控系统从“展示指标”升级为“可对话、可分析、可告警、可追溯”的 AI Ops 平台。
- 在不推翻现有架构（Agent + Gateway + Web）的前提下，增量接入主流 AI Agent 能力。
- 同步完成 Go 工程化升级，让项目具备持续演进和稳定上线能力。

### 1.2 执行原则
- 先可用再智能：优先交付可上线的 MVP，再做多 Agent 增强。
- 先治理再扩展：先补安全、观测、测试基础，再叠加 AI 功能。
- 人机协同：AI 给建议，关键动作必须人工确认。

## 2. 当前项目基线（基于仓库现状）

### 2.1 已有优势
- 后端已拆分 `Agent` 与 `Gateway`，职责清晰。
- 采集链路完整，包含 CPU/内存/网络/GPU/进程。
- 前端已具备节点管理和指标看板基础能力。

### 2.2 主要短板
- 没有历史时序数据层，AI 很难做趋势和异常分析。
- `/api/proxy` 当前缺少目标白名单，存在 SSRF 风险。
- 配置鉴权是 `MMDD PIN`，仅适合内网临时使用。
- 缺少标准化观测（Prometheus/OTel）、测试基线和 CI 质量闸门。

## 3. AI 化建议（先做这 4 个能力）

### 3.1 AI 运维助手（MVP）
- 在 Gateway 增加一个 `/api/ai/chat` 接口。
- 用户可自然语言提问：
  - “哪个节点 CPU 连续 10 分钟 > 85%？”
  - “GPU 占用高的进程是谁？”
  - “帮我总结当前风险并给排障步骤。”

### 3.2 告警解释 Agent
- 对触发告警的节点自动生成“原因猜测 + 证据 + 优先级 + 建议动作”。
- 输出统一结构化 JSON，便于前端渲染和后续自动化。

### 3.3 运维知识库问答（RAG）
- 导入内部 runbook、常见故障 SOP、部署说明。
- 结合文件检索和工具调用，让回答既有“文档证据”也有“实时指标证据”。

### 3.4 周报/巡检报告 Agent
- 定时汇总节点健康度、异常波峰、资源浪费项、建议优化列表。
- 先做“只读报告”，不自动执行任何变更。

## 4. OpenAI 能力接入策略（面向 Go 项目）

### 4.1 推荐 API 形态
- 统一采用 `Responses API` 作为 Agent 编排入口。
- 使用 `tools` 同时接入：
  - 内置工具（如 web search/file search，按场景开启）
  - 自定义函数工具（查询节点、拉取历史、执行诊断脚本）

### 4.2 会话与异步
- 多轮上下文：优先使用 `previous_response_id` 或 `conversation`。
- 长任务（如深度分析、批量巡检）：使用 `background=true` + 轮询/回调。
- 回调通知：使用 webhook 处理 `response.completed` 事件。

### 4.3 Go 与 Agents SDK 的关系
- 现阶段 Agents SDK 文档主线在 Python/TypeScript。
- 当前 Go 项目建议：直接通过 OpenAI API（Go SDK 或 HTTP）实现编排层。
- 未来如果需要多 Agent 可视化追踪，再评估引入独立 Python/TS Agent 控制平面。

## 5. Go 主流技术栈完善建议（与你项目匹配）

### 5.1 运行时与基础库
- Go 版本从 `1.23` 升级到 `1.26.x`（当前稳定主线）。
- 保留 Gin（已是成熟主流），重点补中间件和治理能力。

### 5.2 观测与日志
- 日志：统一 `log/slog`（结构化字段 + trace_id）。
- 指标：引入 `prometheus/client_golang`，区分业务指标和系统指标。
- 链路：接入 OpenTelemetry（Trace/Metrics），导出到 OTel Collector。

### 5.3 安全与稳定性
- `govulncheck` 纳入 CI，按周扫描依赖。
- 增加 API 限流、超时、重试、熔断策略。
- 对 `/api/proxy` 增加白名单与私网网段拦截。

### 5.4 测试与质量门禁
- 单元测试：覆盖采集器解析、配置存储、API handler。
- 集成测试：网关 + mock agent 端到端验证。
- Fuzz：针对 URL 解析、JSON 输入、GPU 输出解析做 fuzz。
- Lint：`golangci-lint` + `go test -race` + `govulncheck` 进入 CI 必过项。

### 5.5 发布与部署
- 保留现有 systemd 与 Docker 方案。
- 增加 `docker-compose`（本地一键拉起：gateway/agent/prometheus/otel-collector）。
- 建议引入 `goreleaser` 规范二进制发布流程。

## 6. 10 周执行里程碑（可直接照此推进）

## 周期总览
- W1: 2026-03-18 ~ 2026-03-24
- W2: 2026-03-25 ~ 2026-03-31
- W3: 2026-04-01 ~ 2026-04-07
- W4: 2026-04-08 ~ 2026-04-14
- W5: 2026-04-15 ~ 2026-04-21
- W6: 2026-04-22 ~ 2026-04-28
- W7: 2026-04-29 ~ 2026-05-05
- W8: 2026-05-06 ~ 2026-05-12
- W9: 2026-05-13 ~ 2026-05-19
- W10: 2026-05-20 ~ 2026-05-26

### W1-W2（基础治理）
- 交付物
  - Go 升级到 1.26.x，依赖完成兼容验证。
  - 增加 `Makefile` 统一命令：`lint/test/race/vuln/build/run`。
  - 完成 `/api/proxy` SSRF 防护（白名单 + scheme + 私网拦截）。
  - 配置写入鉴权从 `MMDD PIN` 升级为 Token（最小可先静态 token）。
- 验收标准
  - CI 首次跑通，至少包含 lint + unit test + vulncheck。
  - 代理接口安全扫描无高危。

### W3-W4（观测与数据底座）
- 交付物
  - Gateway/Agent 暴露 Prometheus 指标。
  - 引入 OpenTelemetry Trace，关键路径打点（采集、代理、配置保存、AI 调用）。
  - 设计并落地历史数据存储（建议先 Prometheus，再扩展长期存储）。
- 验收标准
  - 可查询过去 24h 节点趋势。
  - 单次请求可从日志关联到 trace。

### W5-W7（AI MVP）
- 交付物
  - 新增 `internal/ai` 模块：
    - OpenAI Client
    - Tool Registry
    - Prompt 模板
    - 会话存储（response_id/conversation_id）
  - 新增 `/api/ai/chat` 与前端最小聊天面板。
  - 3 个核心工具函数：
    - `get_node_snapshot(node_id)`
    - `query_hot_nodes(window)`
    - `get_top_processes(node_id, by, limit)`
  - 输出结构化结果（摘要、风险级别、建议动作）。
- 验收标准
  - 对常见运维问题回答可用，且包含可验证证据。
  - 单轮平均响应时间、成功率达到预设阈值。

### W8-W9（Agent 增强）
- 交付物
  - 告警解释 Agent（可异步 background 执行）。
  - 定时巡检/周报 Agent（支持 webhook 完成通知）。
  - 加入 RAG：接入 runbook 文档检索。
- 验收标准
  - 告警到解释报告延迟稳定可控。
  - 报告可追溯到数据来源与文档来源。

### W10（上线与运营化）
- 交付物
  - 建立 AI 评测集（高频故障问答 + 标注答案）。
  - 成本监控与限额策略（按模型、按租户、按日限额）。
  - 灰度发布与回滚手册。
- 验收标准
  - 可灰度到 10%-30% 用户/节点。
  - 回滚演练在 30 分钟内完成。

## 7. 建议目录改造（计划性，不要求一次做完）

```text
nexus-agent-go/
├── cmd/
│   ├── agent/
│   ├── gateway/
│   └── ai-worker/                 # 可选：异步任务执行器
├── internal/
│   ├── ai/
│   │   ├── client.go
│   │   ├── orchestrator.go
│   │   ├── prompts/
│   │   ├── tools/
│   │   └── eval/
│   ├── telemetry/
│   ├── security/
│   ├── gateway/
│   ├── agent/
│   └── model/
├── deploy/
│   ├── docker-compose.observability.yml
│   └── otel/
└── docs/
    ├── ADR/
    └── RUNBOOK/
```

## 8. 风险与回滚策略

### 8.1 主要风险
- 模型幻觉导致错误建议。
- Token 成本失控。
- 工具调用可达范围过大带来安全隐患。
- 高峰时 AI 服务延迟影响用户体验。

### 8.2 控制措施
- 仅允许只读工具进入第一阶段；执行类操作必须人工批准。
- 所有 AI 输出附证据区块（数据来源、时间窗口、置信度）。
- 设置模型降级策略：`高性能模型 -> 经济模型 -> 本地规则`。
- 设置预算阈值和速率限制。

### 8.3 回滚
- 通过 feature flag 一键关闭 AI 路由，保留原监控功能不受影响。
- 保留稳定版本镜像和配置快照，支持快速回滚。

## 9. 成功指标（上线后 4 周评估）
- 平均故障定位时间（MTTD）下降 30% 以上。
- 告警处理平均耗时下降 25% 以上。
- 人工查询节点数据操作次数下降 40% 以上。
- AI 回答“可采纳率”（被运维采纳）达到 60% 以上。

## 10. 你现在最该先做的 3 件事
- 先做安全和治理底座（W1-W2），否则 AI 能力会放大风险。
- 先做历史数据与观测（W3-W4），否则 AI 只能看“当前快照”。
- 再做 AI MVP（W5-W7），优先交付“问答 + 告警解释”两条线。

## 11. 参考资料（用于选型与日期核对）
- OpenAI Responses + Tools: https://platform.openai.com/docs/guides/tools/tool-choice
- OpenAI Conversation State: https://platform.openai.com/docs/guides/conversation-state
- OpenAI Background mode: https://platform.openai.com/docs/guides/background
- OpenAI Webhooks: https://platform.openai.com/docs/webhooks
- OpenAI Assistants 迁移与下线日期: https://platform.openai.com/docs/guides/assistants
- OpenAI Libraries（含 Go）: https://platform.openai.com/docs/libraries
- OpenAI Agents SDK（Python/TypeScript）: https://platform.openai.com/docs/guides/agents-sdk
- Go Release History: https://go.dev/doc/devel/release
- OpenTelemetry Go 状态: https://opentelemetry.io/docs/languages/go/
- Prometheus Go Instrumentation: https://prometheus.io/docs/guides/go-application/
- Go Fuzzing: https://go.dev/doc/security/fuzz/
- Go Security & govulncheck: https://go.dev/doc/security/
