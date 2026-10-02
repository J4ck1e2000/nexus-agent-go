# nexus-agent-go

AI 问答固定由 Pi Runtime 执行；Docker Compose 会启动 Gateway、Pi Runtime 和后端依赖。

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

## 快速启动

推荐使用上面的 Docker Compose 命令，它会启动 Gateway 和 Pi Runtime。单独运行 Gateway 时，必须先启动 Pi Runtime 并设置 PI_RUNTIME_URL，见本文末尾。
## 常用启动模式

这些命令用于调整知识检索后端。脚本直接启动 Gateway 前，请先启动 Pi Runtime 并设置 PI_RUNTIME_URL。

- `auto`（推荐）：优先 Qdrant，失败自动回退本地知识检索
- `local`：只用本地 markdown 检索（不依赖向量库）
- `qdrant`：只用 Qdrant 向量检索（要求向量配置完整）

示例：

```cmd
docker compose --env-file .env.example up -d --build
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

也可以用 go run 启动 Gateway。必须先启动 Pi Runtime，并设置 PI_RUNTIME_URL 指向该 Runtime。

```cmd
go run .\cmd\gateway\
```

但你需要自己维护 `KNOWLEDGE_*` 等环境变量；脚本版更省心。

## AI 问答运行时（Pi Runtime）

AI 问答固定由 TypeScript Pi Runtime 执行。规则模式和旧 Eino 查询执行器不再作为可选运行路径；Runtime 不可用时请求会返回错误，不会切换到规则回答。

### Docker 启动

~~~cmd
docker compose --env-file .env.example up -d --build
~~~

Gateway 通过内部工具网关调用 Pi Runtime，前端 SSE 契约保持不变。

### 本地手动启动 Runtime

~~~cmd
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
~~~

Gateway 启动时必须设置 PI_RUNTIME_URL，例如 http://127.0.0.1:8010。Docker Compose 会默认启动 Pi Runtime 服务。

## Desktop Client(Electron)

除了浏览器访问 `http://127.0.0.1:3000`,仓库还提供正式的桌面客户端,位于 [`desktop/`](desktop/README.md):

- 技术栈:Electron + React + TypeScript + Vite + Tailwind CSS,使用 Electron Forge 打包
- 与 Web 前端功能对齐:节点监控(2s 轮询)、GPU 详情、Availability、用户/节点管理、中英文、AI 流式问答(SSE 经主进程转发,支持 Stop)
- JWT 保存在主进程(safeStorage 加密),Renderer 不持有 token;`nodeIntegration=false`、`contextIsolation=true`、`sandbox=true`
- Gateway 地址可在登录页/设置页配置并测试连接,默认 `http://127.0.0.1:3000`

```bash
cd desktop
npm install
npm run dev      # 开发:同时启动 Vite 与 Electron
npm run package  # 打包
```

详见 [desktop/README.md](desktop/README.md)。旧 Web 前端(`web/`)保留不变。
