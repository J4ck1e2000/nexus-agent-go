# nexus-agent-go

这个项目推荐用 `scripts/knowledge-run` 启动，默认就把 AI + RAG 常用配置准备好。

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
