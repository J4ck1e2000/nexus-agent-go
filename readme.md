# nexus-agent-go

这个项目推荐用 `scripts/knowledge-run` 启动，默认就把 AI + RAG 常用配置准备好。

## Docker 部署（推荐）

### 1. 复制环境变量模板

```cmd
copy .env.example .env
```

或 PowerShell:

```powershell
Copy-Item .env.example .env
```

### 2. 修改 `.env`

至少修改这些项：

- `MYSQL_ROOT_PASSWORD`
- `JWT_SECRET`
- `AI_API_KEY`

### 3. 一键启动

```cmd
docker compose up -d --build
```

查看状态和日志：

```cmd
docker compose ps
docker compose logs -f gateway
```

启动后访问：`http://127.0.0.1:3000`

### 4. 知识库改动后同步（可选）

```cmd
scripts\knowledge-run.cmd -Task sync-reload -Backend qdrant -GatewayURL http://127.0.0.1:3000 -Username admin -Password admin123
```

## 快速启动（推荐）

### 1. 准备最少环境变量（CMD）

```cmd
set "MYSQL_DSN=root:password@tcp(127.0.0.1:3306)/nexus_agent?charset=utf8mb4&parseTime=True&loc=Local"
set "JWT_SECRET=your-jwt-secret"

set AI_ENABLED=true
set AI_MODE=agent
set AI_PROVIDER=qwen
set AI_MODEL=qwen3.5-122b-a10b
set AI_BASE_URL=https://dashscope.aliyuncs.com/compatible-mode/v1
set AI_API_KEY=your-api-key
```

### 2. 一条命令启动 Gateway

```cmd
scripts\knowledge-run.cmd -Task gateway -Backend auto
```

启动后访问：`http://127.0.0.1:3000`

## 常用启动模式

- `auto`（推荐）：优先 Qdrant，失败自动回退本地知识检索
- `local`：只用本地 markdown 检索（不依赖向量库）
- `qdrant`：只用 Qdrant 向量检索（要求向量配置完整）

示例：

```cmd
scripts\knowledge-run.cmd -Task gateway -Backend auto
scripts\knowledge-run.cmd -Task gateway -Backend local
scripts\knowledge-run.cmd -Task gateway -Backend qdrant
```

## 知识库维护（改了 knowledge/*.md 之后）

### 同步到 Qdrant

```cmd
scripts\knowledge-run.cmd -Task sync -Backend qdrant -KnowledgeDir knowledge/anomalies
```

### 同步并热重载（推荐）

```cmd
scripts\knowledge-run.cmd -Task sync-reload -Backend qdrant -GatewayURL http://127.0.0.1:3000 -Username admin -Password admin123
```

### 仅热重载（不重新向量化）

```cmd
scripts\knowledge-run.cmd -Task reload -GatewayURL http://127.0.0.1:3000 -Username admin -Password admin123
```

## 可选：本地启动 Qdrant

```powershell
docker run -d --name qdrant `
  -p 6333:6333 -p 6334:6334 `
  qdrant/qdrant:latest
```

## 可选：检索效果评估

```cmd
scripts\knowledge-run.cmd -Task eval -Backend auto -TopK 5
```

## 如果你不想用脚本（手动启动）

也可以继续用：

```cmd
go run .\cmd\gateway\
```

但你需要自己维护 `KNOWLEDGE_*` 等环境变量；脚本版更省心。

## 可选：Pi Runtime 执行器（AI_EXECUTOR=pi）

AI 问答支持两种执行器，通过 `AI_EXECUTOR` 切换：

- `legacy`（默认）：进程内 Eino ChatModelAgent，行为与历史版本一致。
- `pi`：把模型 Loop 委托给独立的 TypeScript Pi Runtime（`pi-runtime/`，基于 `@earendil-works/pi-coding-agent`），Go 侧只保留认证、Run 生命周期、工具网关与 SSE。Runtime 不可达时自动回退 legacy。

详细设计见 `pi-runtime/DESIGN.md`。

### Docker 启动（compose profiles: pi）

```cmd
docker compose --profile pi up -d --build
```

compose 会额外启动 `pi-runtime` 服务，gateway 通过内部工具网关 `/internal/api/tools/*` 供其回调（共享密钥 + Run 凭证双向鉴权）。

### 本地手动启动 Runtime

```cmd
cd pi-runtime
npm install
set PI_RUNTIME_PORT=8010
set PI_RUNTIME_TOKEN=nexus-pi-internal-token
set GATEWAY_TOOL_URL=http://127.0.0.1:3000
set GATEWAY_INTERNAL_TOKEN=nexus-pi-internal-token
set AI_BASE_URL=https://dashscope.aliyuncs.com/compatible-mode/v1
set AI_API_KEY=your-api-key
set AI_MODEL=qwen3.5-122b-a10b
node src/server.ts
```

再以 `AI_EXECUTOR=pi`、`PI_RUNTIME_URL=http://127.0.0.1:8010` 启动 gateway 即可。前端与 `/api/ai/query` 的 SSE 契约完全不变。

### 验证（无需真实模型 Key）

```cmd
cd pi-runtime
npm run test:e2e
go test ./...
```

`npm run test:e2e` 用脚本化 mock 模型服务器驱动真实 Pi SDK 完整跑通「模型 → 工具回调 → 结果反馈 → JSON 终稿」链路；`go test ./internal/gateway -run TestPiRuntimeFullStack` 验证 Go ↔ Runtime 全链路。
