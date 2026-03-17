# Nexus Agent Go 详细使用说明（小白版）

本文档目标：你不需要懂 Go、不需要懂前端，只要按步骤执行，就能把 `nexus-agent-go` 跑起来并监控服务器。

## 1. 这项目是干什么的

`nexus-agent-go` 是一个“监控看板”项目，用来查看多台服务器的实时状态，主要包括：

1. CPU 使用率
2. 内存使用率
3. 网络收发累计与估算速率
4. GPU 温度/功耗/利用率/显存
5. 活跃进程（重点显示 GPU 进程）

它由两个程序组成：

1. `Agent`：部署在被监控机器上，采集本机指标，提供 `GET /metrics`
2. `Gateway`：部署在你管理机上，提供网页看板、节点配置 API、代理转发 API

简单理解：

1. 每台被监控机器 = 跑一个 Agent
2. 管理机器 = 跑一个 Gateway
3. 浏览器只访问 Gateway

## 2. 项目目录怎么理解

核心目录如下：

1. `cmd/agent/main.go`：Agent 入口
2. `cmd/gateway/main.go`：Gateway 入口
3. `internal/agent/`：系统指标采集逻辑（CPU/内存/网络/GPU/进程）
4. `internal/gateway/`：Gateway API、静态文件、配置读写、代理逻辑
5. `internal/model/types.go`：前后端共享数据结构
6. `web/index.html`：前端页面（React UMD + Tailwind CDN）
7. `web/config.json`：节点配置文件（Gateway 读写）
8. `deploy/systemd/`：systemd 服务模板
9. `deploy/Dockerfile.*`：Docker 构建模板

## 3. 运行前准备

## 3.1 最低要求

1. Go `1.23+`（`go.mod` 当前是 `go 1.23.0`）
2. Linux/macOS 任一可运行 Go 的环境
3. 端口可用：
1. Agent 默认 `8005`
2. Gateway 默认 `3000`
4. 如果你要看 GPU 指标：
1. 机器要有 NVIDIA GPU
2. 系统里 `nvidia-smi` 命令可执行

## 3.2 快速检查环境

```bash
go version
nvidia-smi
```

如果 `nvidia-smi` 报错，不影响 CPU/内存/网络监控，只是 GPU 区域会显示无数据。

## 4. 本地最小可用启动（先跑起来）

在项目根目录执行：

```bash
go mod tidy
go run ./cmd/agent
```

再开一个终端执行：

```bash
go run ./cmd/gateway
```

浏览器打开：

```text
http://127.0.0.1:3000
```

## 4.1 验证接口是否正常

```bash
curl http://127.0.0.1:8005/
curl http://127.0.0.1:8005/metrics
curl http://127.0.0.1:3000/api/version
curl http://127.0.0.1:3000/api/config
```

只要这四个请求能成功，项目基本就正常。

## 5. 页面怎么用（一步一步）

进入网页后：

1. 点击右上角 `+`
2. `Node Name` 填节点名字（例如 `A100-01`）
3. `Agent URL` 填被监控机地址（例如 `http://10.0.0.21:8005`）
4. `Security PIN` 填当天日期 `MMDD`
1. 例如 3 月 6 日就是 `0306`
5. 点击 `Connect Agent`

删除节点时也会要求你输入 PIN。

## 5.1 PIN 机制说明（非常重要）

Gateway 保存配置时会校验请求头 `X-PIN`，必须等于“当天月日”：

1. 1 月 2 日：`0102`
2. 12 月 31 日：`1231`

对应后端逻辑在 `internal/gateway/handler.go`：

1. `POST /api/config` 时比对 `X-PIN == time.Now().Format("0102")`

注意：这只是轻量保护，不是强安全认证。

## 6. 环境变量说明（Agent）

Agent 入口：`cmd/agent/main.go`

可用变量：

1. `PORT`：监听端口，默认 `8005`
2. `METRICS_INTERVAL`：采集间隔，默认 `2s`
1. 支持 Go duration，如 `500ms`、`2s`
2. 也支持纯数字秒，如 `5`

示例：

```bash
PORT=9005 METRICS_INTERVAL=1s go run ./cmd/agent
```

## 7. 环境变量说明（Gateway）

Gateway 入口：`cmd/gateway/main.go`

可用变量：

1. `PORT`：监听端口，默认 `3000`
2. `WEB_DIR`：前端目录，默认 `web`
3. `CONFIG_FILE`：配置文件路径，默认 `web/config.json`
4. `VERSION_NAME`：版本名展示，默认 `V2.0`
5. `CHANGELOG`：更新日志文本，前端版本检测弹窗会显示

示例：

```bash
PORT=3100 WEB_DIR=web CONFIG_FILE=web/config.json VERSION_NAME=V2.1 CHANGELOG="修复代理超时" go run ./cmd/gateway
```

## 8. 实际采集逻辑（你看到的数据从哪里来）

采集入口：`internal/agent/collector.go`

1. 主机信息：`gopsutil/host`
2. CPU 使用率和核心数：`gopsutil/cpu`
3. 内存：`gopsutil/mem`
4. 网络累计收发：`gopsutil/net`
5. 进程：`gopsutil/process`
6. GPU：调用 `nvidia-smi`（见 `internal/agent/gpu.go`）

### 8.1 进程筛选规则

1. 非 GPU 进程：CPU 必须 `> 2.0%` 才保留
2. GPU 进程：即使 CPU 低也优先保留
3. 最终按“是否 GPU + CPU 占用”排序
4. 最多返回 10 条

### 8.2 网络速度怎么来的

后端只返回累计值：

1. `net_sent_mb`
2. `net_recv_mb`

前端每 2 秒轮询一次，然后自己用“本次累计 - 上次累计 / 时间差”计算速率。

## 9. Gateway 的关键接口

来自 `internal/gateway/handler.go`：

1. `GET /api/version`：前端每 5 秒检查版本变化
2. `GET /api/config`：读取节点配置
3. `POST /api/config`：保存节点配置（需要 `X-PIN`）
4. `GET /api/proxy?url=http://...`：代理请求，解决跨域/网络可达性问题

前端拉取指标时优先走：

1. `/api/proxy?url=<agent>/metrics`
2. 代理失败后回退直连 `<agent>/metrics`

## 10. 构建二进制并部署

## 10.1 本机构建

```bash
go build -o bin/nexus-agent-go-agent ./cmd/agent
go build -o bin/nexus-agent-go-gateway ./cmd/gateway
```

## 10.2 构建 Linux 产物

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o bin/nexus-agent-go-agent ./cmd/agent
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o bin/nexus-agent-go-gateway ./cmd/gateway
```

上传并运行：

```bash
scp bin/nexus-agent-go-agent <user>@<host>:/opt/nexus-agent-go/bin/
scp bin/nexus-agent-go-gateway <user>@<host>:/opt/nexus-agent-go/bin/
scp -r web <user>@<host>:/opt/nexus-agent-go/

/opt/nexus-agent-go/bin/nexus-agent-go-agent
/opt/nexus-agent-go/bin/nexus-agent-go-gateway
```

## 11. systemd 部署（推荐）

已有模板：

1. `deploy/systemd/nexus-agent-go-agent.service`
2. `deploy/systemd/nexus-agent-go-gateway.service`

安装：

```bash
sudo cp deploy/systemd/nexus-agent-go-agent.service /etc/systemd/system/
sudo cp deploy/systemd/nexus-agent-go-gateway.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable nexus-agent-go-agent nexus-agent-go-gateway
sudo systemctl start nexus-agent-go-agent nexus-agent-go-gateway
```

查看状态：

```bash
sudo systemctl status nexus-agent-go-agent nexus-agent-go-gateway
sudo journalctl -u nexus-agent-go-agent -u nexus-agent-go-gateway -f
```

## 12. Docker 部署

构建：

```bash
docker build -f deploy/Dockerfile.agent -t nexus-agent-go-agent .
docker build -f deploy/Dockerfile.gateway -t nexus-agent-go-gateway .
```

运行：

```bash
docker run -d --name nexus-agent-go-agent --network host --restart always nexus-agent-go-agent
docker run -d --name nexus-agent-go-gateway --network host --restart always nexus-agent-go-gateway
```

如果要采 GPU，请根据宿主机环境增加 `--gpus all` 和 NVIDIA runtime 配置。

## 13. 常见问题排查

## 13.1 页面显示节点离线

在 Gateway 所在机器执行：

```bash
curl http://<agent-ip>:8005/metrics
```

如果不通，通常是：

1. Agent 没启动
2. 端口没放行
3. URL 填错（协议、IP、端口）

## 13.2 保存节点失败（PIN 错误）

确认你输入的是当天 `MMDD`，例如 2026-03-06 是 `0306`。

## 13.3 没有 GPU 信息

检查：

1. `nvidia-smi` 在 Agent 机器是否可执行
2. 是否是 AMD/Intel GPU（当前逻辑仅支持 NVIDIA 的 `nvidia-smi`）

## 13.4 网页能开但一直不刷新

前端脚本依赖外部 CDN：

1. `cdn.tailwindcss.com`
2. `unpkg.com`（React/ReactDOM/Babel）

如果机器访问外网受限，页面可能样式或脚本加载失败。

## 14. 安全与风险提示（生产必看）

当前版本有以下安全边界：

1. `POST /api/config` 只靠“当天 PIN”，很容易被猜到
2. `/api/proxy` 没有目标白名单，存在被滥用风险
3. Agent 默认全开放 CORS `*`
4. 没有内建 TLS/认证/审计

建议至少放在内网，并在前面加反向代理访问控制。

## 15. 改进说明（建议按优先级执行）

## 15.1 P0（优先立刻改）

1. 给 Gateway 增加正式认证
1. 用固定强口令/JWT/OIDC 替代当天 PIN
2. 给 `/api/proxy` 增加白名单
1. 只允许请求 `config.json` 中登记的 Agent 域名/IP
3. 给 Agent/Gateway 增加基础访问控制
1. 至少支持 IP 白名单或 Basic Auth

## 15.2 P1（稳定性）

1. 增加健康检查接口（`/healthz`）
2. 前端轮询策略从固定 2 秒改为可配置
3. 给 `fetch` 增加退避重试和失败统计
4. 增加结构化日志（JSON）和请求追踪 ID

## 15.3 P1（可维护性）

1. 前端从单文件 `web/index.html` 拆分为工程化项目
1. 例如 Vite + React + TypeScript
2. 移除运行时 Babel CDN 依赖，改为构建产物
3. 补齐后端单元测试：
1. `ConfigStore` 读写
2. `safeJoin` 路径安全
3. PIN 校验与代理参数校验

## 15.4 P2（功能增强）

1. 增加磁盘指标（容量、读写速率）
2. 增加告警机制（CPU/GPU 温度阈值）
3. 增加历史时序存储（Prometheus/InfluxDB）
4. 增加多角色权限（只读/管理员）

## 16. 给新同事的 5 分钟上手版

1. 在每台机器启动 Agent：`go run ./cmd/agent`
2. 在管理机启动 Gateway：`go run ./cmd/gateway`
3. 浏览器打开 `http://<gateway-ip>:3000`
4. 点 `+` 添加 Agent URL 和当天 PIN（`MMDD`）
5. 看板开始 2 秒轮询刷新，绿色在线、离线会标红

---

如果你只想做“最快上线”，推荐先按以下组合：

1. Linux 二进制 + systemd
2. 内网部署
3. Nginx 反代 Gateway 并加 HTTPS + 访问控制
4. 尽快落实 `15.1` 的 P0 改进项
