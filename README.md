# nexus-agent-go

AI 问答固定由 Pi Runtime 执行；Docker Compose 会启动 Gateway、Pi Runtime 和后端依赖。

## 项目简介

Nexus 是一套分布式 GPU 集群监控与调度辅助平台:

- **Agentless SSH 采集(推荐)**:Gateway 通过 SSH 直接登录 Linux 服务器执行 `/proc` / `ps` / `nvidia-smi` 采集,目标服务器无需安装任何 Nexus 程序(详见下文 [Agentless SSH Monitoring](#agentless-ssh-monitoring));
- **旧版 Go Agent(可选兼容)**:仅供已有 `collector_type=agent` 节点使用;Docker Compose 默认不启动,如确需兼容可运行 `docker compose --profile legacy-agent up -d agent`;新节点建议使用 SSH 采集。
- **Go Gateway** 是控制面:认证(JWT)、节点配置与聚合状态(Redis 热状态 + MySQL 持久化)、管理 API、AI 问答入口(SSE 流式),并对外提供 Web Dashboard;
- **空闲 GPU 工作台**:Electron 按显存、系统内存、型号、GPU 进程和持续空闲时间筛选资源;预约是按用户隔离的提醒,不会锁定硬件;
- **资源历史与通知**:Gateway 通过 Redis 提供 25 小时高频状态,并在 MySQL 保留最多 90 天小时样本;Electron 显示 GPU 热力图并发送本机系统告警;
- **本地 SSH 终端**:Electron 使用用户电脑上的 OpenSSH 直连登记节点,沿用个人密钥、Agent、`~/.ssh/config`、ProxyJump 和 `known_hosts`;凭据不会经过 Gateway;
- **Pi Runtime**(TypeScript)是唯一的 AI 执行器,经工具网关回调 Gateway 读取集群快照,流式生成回答;
- **Qdrant** 提供知识库向量检索(可回退本地 markdown 检索);
- **Web Dashboard**(`web/`,CDN React 单文件)保留为现有行为基准;**Desktop Client**(`desktop/`,Electron + React + TS)使用相同 Gateway 数据,并承载本机 OpenSSH 终端和桌面通知。

## 整体架构

```text
Browser (web/)            Electron (desktop/)
       │ REST / SSE / JWT        │ preload IPC → Main
       └──────────┬──────────────┘
                  ▼
            Go Gateway  ──── Redis(节点热状态/25h历史)
                  │         MySQL(用户/节点/预约/90d小时历史)
                  │         Qdrant(知识向量)
                  ▼
            Pi Runtime(AI 执行器,SSE 透传)
                  ▲
        CollectorRouter
        ┌────────┴─────────┐
  SSH Collector        Go Agent(legacy-agent profile)
  (agentless,每台         (每台 GPU 节点,
   Linux 服务器)           指标上报)
```

Electron 的交互式 SSH 终端在用户电脑上启动 OpenSSH,直接连接已登记节点;Gateway 的 SSH 私钥只用于指标采集,不用于桌面用户的终端会话。

## 仓库结构

```text
├── cmd/                  # Go 入口:gateway / agent / ssh-smoke / knowledge-sync / rag-eval
├── internal/
│   ├── gateway/          # HTTP API、认证、节点状态、AI 适配层
│   ├── collector/        # 节点采集抽象:agenthttp(旧版)/ sshcollector(agentless)
│   ├── ai/               # 意图识别、检索(RAG)、工具、知识同步
│   ├── runtime/          # Pi Runtime 客户端与运行凭据
│   ├── agent/            # 节点采集 agent
│   ├── model/            # 数据模型(AgentConfig、SystemMetrics...)
│   └── tools/            # 通用工具
├── pi-runtime/           # TypeScript AI 执行器(Node 24 原生 TS 运行)
├── web/                  # 旧 Web 前端(CDN React,保留作行为基准)
├── desktop/              # Electron 桌面客户端(见 desktop/README.md)
├── knowledge/            # 知识库 markdown(可同步到 Qdrant)
├── deploy/               # Dockerfile 与 systemd 单元
├── scripts/              # knowledge-run 等运维脚本
└── docker-compose.yml    # 一键启动 Gateway / Pi Runtime / MySQL / Redis / Qdrant
```

## 组件速览与本地启动

| 组件 | 目录 | 本地启动 | 说明 |
| --- | --- | --- | --- |
| Gateway | `cmd/gateway` | `go run ./cmd/gateway`(需先起 Pi Runtime 并设 `PI_RUNTIME_URL`) | 默认 `:3000`,同时服务 `web/` 静态页 |
| Agent | `cmd/agent` | `go run ./cmd/agent` | 旧版节点兼容；Compose 需显式启用 `legacy-agent` profile |
| Pi Runtime | `pi-runtime/` | `npm install && npm start` | 需要 `AI_API_KEY` 等,见下文 |
| 知识同步 | `cmd/knowledge-sync` | `go run ./cmd/knowledge-sync` 或 `scripts/knowledge-run.cmd` | markdown → Qdrant |
| Web 前端 | `web/` | 由 Gateway 静态托管,无需单独构建 | 保留不删 |
| 桌面客户端 | `desktop/` | `npm install && npm run dev` | 见 [desktop/README.md](desktop/README.md) |

`AGENTS.md` 面向 AI 编码代理,汇总了各组件的构建/测试命令与工程约定。

## Agentless SSH Monitoring

Nexus 支持通过 SSH 对 Linux 服务器做无代理(agentless)监控:Gateway 直接登录目标服务器执行一段固定脚本,从 `/proc`、`ps`、`nvidia-smi` 读取指标,转换为与旧版 Agent 完全一致的 `SystemMetrics`。

远端 Linux 服务器只需要具备:

- `sshd`(OpenSSH Server；首次引导时需允许账号密码登录，或预先授权 Gateway 公钥；引导后使用密钥认证);
- `/proc` 与 `ps`(任何标准 Linux 发行版都有);
- `nvidia-smi`(NVIDIA Driver 自带)——无 GPU 的纯 CPU 服务器同样可以采集 CPU / 内存 / 网络。

监控链路:

```text
Gateway → CollectorRouter → SSH Collector → SSH(长连接复用)
        → /proc + ps + nvidia-smi → SystemMetrics → NodeState → Redis
```

旧版 Go Agent HTTP 采集仍为存量节点提供兼容。Compose 自带的 Agent 容器默认不启动；只有要把运行 Compose 的这台 GPU 主机本身作为 Agent 节点时，才需用 `docker compose --profile legacy-agent up -d agent` 启动。其他服务器上已部署的 Agent 不受此 profile 影响。

### 添加 SSH 节点

只有管理员可以添加或管理节点。Desktop 管理员打开 **Add Node**，选择 SSH 并填写节点名称、服务器地址、SSH 端口和远端账号用户名。该账号必须是管理员在服务器上已有的账号，能通过 SSH 登录并运行普通 shell 命令；不需要安装 Nexus Agent，也不需要 `sudo`。建议使用权限足以读取指标的普通账号，而不是直接使用 root。

首次连接时，如果 Gateway 专用公钥还没有授权到该账号，管理员可以填写该账号的服务器密码。Gateway 会用密码建立 SSH 会话，只把自己的公钥追加到该账号的 `~/.ssh/authorized_keys`，随后立即改用密钥认证并采集首份数据。密码不会保存到节点配置或 MySQL，应用也不会把它写入日志；如果公钥已授权，密码可以留空。目标服务器需在首次引导时允许密码认证，后续可按服务器策略关闭密码认证。

管理接口使用 `POST /api/config/enroll-ssh`，普通的 `POST /api/config` 会拒绝 SSH 节点，避免绕过安全引导。节点按 `host:port` 在全平台唯一，和由哪位平台管理员添加、使用哪个 Linux 用户名无关；所有管理员共享同一份节点列表。若该服务器已存在但本次填写了另一个 Linux 用户名，系统会提示已添加，不会创建副本或静默切换采集账号。需要更换远端登录账号时，先删除全局节点再重新添加。修改后的主机指纹不会静默覆盖已有指纹；若服务器合法轮换主机密钥，先核验并更新管理员电脑或 Gateway 上过期的 `known_hosts` 记录，再删除旧节点并重新添加以重置数据库固定值。

所有已登录用户都可以读取全局节点状态 `GET /api/nodes/overview`。节点配置、添加、删除及 SSH 凭据引导都仅限管理员。

### SSH 身份与凭据

- Gateway 首次启动时生成独立的 Ed25519 SSH 密钥。Docker Compose 将它保存在 `ssh-key-data` 持久卷中；不要删除该卷，否则目标服务器授权的公钥与 Gateway 身份会失配。也可以通过 `SSH_PRIVATE_KEY_PATH` 提供现有未加密私钥。
- 添加节点时，Desktop 会查找管理员电脑 `~/.ssh/known_hosts` 中与目标匹配的记录，并把匹配的主机公钥作为信任提示交给 Gateway。也可以给 Gateway 配置 `SSH_KNOWN_HOSTS_PATH`。如果没有可用的预置信任记录，系统自动信任首次收到的主机公钥并将公钥和 `SHA256` 指纹存入节点配置；此后公钥发生变化会拒绝连接，不会要求管理员逐次确认。
- 自动信任首次连接的主机公钥是 TOFU。它能识别后续密钥变化，但无法证明首次连接时没有网络中间人。需要更强的首次连接保证时，请先把经过独立核验的主机公钥放入管理员电脑或 Gateway 的 `known_hosts`。
- Gateway 的远端公钥带有 `restrict` 选项，关闭端口转发、代理转发、X11 转发和 PTY。该选项不限制远程命令本身；Gateway 密钥可按远端账号权限执行命令，因此建议使用具备必要读取权限的低权限账号，避免直接使用 root。
- SSH 私钥、密码不会下发到浏览器或普通用户端；SSH 节点按 `host:port` 全局唯一，所选 Linux 用户仅作为该节点的 Gateway 登录账号。连接按主机地址、端口、登录账号和固定主机公钥复用。连接和命令超时、keepalive 均可配置。

### 密码传输

首次引导密码只允许通过 HTTPS 提交到远程 Gateway。Gateway 会再次执行服务端检查；仅管理员 Desktop 的客户端校验不足以绕过此检查。Docker Compose 默认把 Gateway 端口绑定到本机 `127.0.0.1`，所以本机 Desktop 可通过 loopback HTTP 使用首次密码。若把 Gateway 暴露给其他机器，请配置 HTTPS 并将 `SSH_BOOTSTRAP_ALLOW_INSECURE_HTTP=false`。TLS 在可信反向代理终止时，可将 `SSH_BOOTSTRAP_TRUST_PROXY_TLS=true`，前提是代理覆盖 `X-Forwarded-Proto` 且外部不能绕过代理直连 Gateway。

### 环境变量与 Docker 部署

```env
# 默认使用 Docker 持久卷中的 Gateway 密钥；known_hosts 可缺省并在首次连接时 TOFU 固定
SSH_PRIVATE_KEY_PATH=/var/lib/nexus-agent/ssh/id_ed25519
SSH_KNOWN_HOSTS_PATH=
# 如需挂载自有密钥/known_hosts，需同时设置宿主机与容器路径：
# SSH_PRIVATE_KEY_HOST_PATH=/home/user/.ssh/nexus_id_ed25519
# SSH_PRIVATE_KEY_PATH=/run/secrets/nexus_ssh_key
# SSH_KNOWN_HOSTS_HOST_PATH=/home/user/.ssh/known_hosts
# SSH_KNOWN_HOSTS_PATH=/app/config/known_hosts

SSH_CONNECT_TIMEOUT_SEC=5
SSH_COMMAND_TIMEOUT_SEC=5
SSH_KEEPALIVE_SEC=15
NODE_POLL_MAX_CONCURRENCY=16
GATEWAY_HOST_BIND=127.0.0.1
SSH_BOOTSTRAP_ALLOW_INSECURE_HTTP=true
SSH_BOOTSTRAP_TRUST_PROXY_TLS=false
```

```bash
docker compose up -d --build gateway
```

### 采集自检(ssh-smoke)

不启动 Gateway 也可以验证一台服务器能否被 agentless 采集:

```bash
SSH_HOST=10.0.0.15 \
SSH_PORT=22 \
SSH_USER=renhaokun \
SSH_PRIVATE_KEY_PATH=/home/me/.ssh/id_ed25519 \
SSH_KNOWN_HOSTS_PATH=/home/me/.ssh/known_hosts \
go run ./cmd/ssh-smoke
```

输出一次采集的 JSON 摘要(hostname / CPU / RAM / GPU / 进程数)后退出;失败时输出稳定的 `ssh_*` 错误码并以非零码退出。

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

Docker Compose 默认生成并持久保存 Gateway 的 SSH 身份；添加 SSH 节点无需在宿主机预先创建私钥。旧版 Agent(URL 方式)仍可继续使用。

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

除了浏览器访问 `http://127.0.0.1:3000`,仓库还提供正式的桌面客户端,位于 [`desktop/`](desktop/README.md)。空闲 GPU 预约、系统通知、长周期历史热力图和本机 OpenSSH 终端目前由桌面端提供;旧 Web Dashboard 保留既有功能作为行为基准:

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
