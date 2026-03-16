# Nexus Agent Go

Nexus Agent Go 是一个基于 Go + Gin 的轻量监控系统，用于集中展示多台服务器的 CPU、内存、网络、GPU 与活跃进程信息。

项目由两部分组成：

1. `Agent`：部署在被监控机器上，负责采集指标并提供 `/metrics` 接口。
2. `Gateway`：部署在你的管理机上，提供 Web 看板、节点配置管理和代理访问能力。

## 1. 项目结构

```text
nexus-agent-go/
├── cmd/
│   ├── agent/                 # Agent 程序入口
│   └── gateway/               # Gateway 程序入口
├── internal/
│   ├── agent/                 # 采集逻辑（CPU/内存/网络/GPU/进程）
│   ├── gateway/               # 网关逻辑（配置、代理、静态路由）
│   └── model/                 # 前后端共享数据结构
├── web/                       # 前端页面与静态资源
├── deploy/
│   ├── Dockerfile.agent       # Agent 镜像构建文件
│   ├── Dockerfile.gateway     # Gateway 镜像构建文件
│   └── systemd/               # systemd 服务单元
├── go.mod
├── go.sum
└── readme.md
```

## 2. 核心工作流程

### 2.1 数据流

1. 浏览器打开 Gateway 页面（默认端口 `3000`）。
2. 前端先请求 `GET /api/config` 获取节点列表。
3. 前端按周期请求每个节点的 `http://<agent>/metrics`（优先可走 `/api/proxy`）。
4. Agent 返回结构化系统指标。
5. 前端将指标渲染为卡片、GPU 用户视图等。

### 2.2 配置流

1. 在页面新增/删除节点时，前端调用 `POST /api/config`。
2. Gateway 校验 `X-PIN=MMDD` 后写入 `web/config.json`。
3. 后续刷新页面自动读取最新配置。

## 3. API 说明

### 3.1 Gateway API

1. `GET /api/version`
2. `GET /api/config`
3. `POST /api/config`（需 `X-PIN`，格式为当天 `MMDD`）
4. `GET /api/proxy?url=http://...`

### 3.2 Agent API

1. `GET /`：运行状态
2. `GET /metrics`：完整监控数据

## 4. 本地开发运行

### 4.1 环境要求

1. Go 版本建议 `1.22+`
2. 如需 GPU 指标，机器需安装 NVIDIA 驱动并可执行 `nvidia-smi`

### 4.2 安装依赖

```bash
go mod tidy
```

### 4.3 启动 Agent

```bash
go run ./cmd/agent
```

可选环境变量：

```bash
PORT=8005 METRICS_INTERVAL=2s go run ./cmd/agent
```

### 4.4 启动 Gateway

```bash
go run ./cmd/gateway
```

可选环境变量：

```bash
PORT=3000 WEB_DIR=web CONFIG_FILE=web/config.json go run ./cmd/gateway
```

### 4.5 访问看板

浏览器打开：

```text
http://127.0.0.1:3000
```

## 5. 页面使用说明

1. 点击右上角 `+` 新增节点。
2. `Node Name` 填显示名（例如 `Server-A100`）。
3. `Agent URL` 填节点地址（例如 `http://10.16.87.183:8005`）。
4. `Security PIN` 填当天日期 `MMDD`（例如 3 月 5 日是 `0305`）。
5. 保存后等待轮询刷新即可看到指标。

## 6. 二进制构建与部署

### 6.1 构建

```bash
go build -o bin/nexus-agent-go-agent ./cmd/agent
go build -o bin/nexus-agent-go-gateway ./cmd/gateway
```

### 6.2 不安装 Go 的服务器如何部署

1. 在本地构建 Linux 二进制：

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o bin/nexus-agent-go-agent ./cmd/agent
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o bin/nexus-agent-go-gateway ./cmd/gateway
```

2. 上传到服务器：

```bash
scp bin/nexus-agent-go-agent <user>@<server>:/opt/nexus-agent-go/bin/
scp bin/nexus-agent-go-gateway <user>@<server>:/opt/nexus-agent-go/bin/
scp -r web <user>@<server>:/opt/nexus-agent-go/
```

3. 直接运行：

```bash
/opt/nexus-agent-go/bin/nexus-agent-go-agent
/opt/nexus-agent-go/bin/nexus-agent-go-gateway
```

## 7. systemd 部署（推荐生产）

项目已提供服务单元：

1. `deploy/systemd/nexus-agent-go-agent.service`
2. `deploy/systemd/nexus-agent-go-gateway.service`

部署步骤：

```bash
sudo cp deploy/systemd/nexus-agent-go-agent.service /etc/systemd/system/
sudo cp deploy/systemd/nexus-agent-go-gateway.service /etc/systemd/system/

sudo systemctl daemon-reload
sudo systemctl enable nexus-agent-go-agent nexus-agent-go-gateway
sudo systemctl start nexus-agent-go-agent nexus-agent-go-gateway
```

状态与日志：

```bash
sudo systemctl status nexus-agent-go-agent nexus-agent-go-gateway
sudo journalctl -u nexus-agent-go-agent -u nexus-agent-go-gateway -f
```

## 8. Docker 部署

### 8.1 构建镜像

```bash
docker build -f deploy/Dockerfile.agent -t nexus-agent-go-agent .
docker build -f deploy/Dockerfile.gateway -t nexus-agent-go-gateway .
```

### 8.2 运行容器

```bash
docker run -d --name nexus-agent-go-agent --network host --restart always nexus-agent-go-agent
docker run -d --name nexus-agent-go-gateway --network host --restart always nexus-agent-go-gateway
```

## 9. 多机监控部署建议

1. 每台被监控服务器只部署 Agent。
2. 选一台管理机部署 Gateway 和前端。
3. 在 Gateway 页面统一录入所有 Agent 地址。
4. 若网络隔离，可用 SSH 隧道或内网代理打通 `agent:8005`。

## 10. 常见问题排查

### 10.1 页面显示节点离线

1. 先在 Gateway 机器执行：

```bash
curl http://<agent-ip>:8005/metrics
```

2. 不通则检查防火墙/安全组是否放行 `8005`。

### 10.2 显示的 IP 地址不是预期

Agent 通过对外路由探测 IP，可能命中虚拟网卡（如 Docker/VPN）。不影响核心监控。

### 10.3 保存节点提示 PIN 错误

`X-PIN` 必须是当天 `MMDD`，例如 12 月 3 日为 `1203`。

### 10.4 无 GPU 数据

1. 确认服务器有 NVIDIA 驱动。
2. 确认 `nvidia-smi` 可执行。
3. 容器部署时需正确透传 GPU（如 `--gpus all`，按环境调整）。

## 11. 安全建议

1. `POST /api/config` 当前为简化 PIN 机制，仅适合内网使用。
2. 生产环境建议配合 Nginx + HTTPS + 访问控制。
3. `/api/proxy` 建议后续增加目标地址白名单，降低代理滥用风险。

## 12. 快速验证清单

```bash
curl http://127.0.0.1:8005/
curl http://127.0.0.1:8005/metrics
curl http://127.0.0.1:3000/api/version
curl http://127.0.0.1:3000/api/config
```

如果以上都正常，页面即可稳定使用。
