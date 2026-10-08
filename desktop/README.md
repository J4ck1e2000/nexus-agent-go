# Nexus Desktop

Nexus GPU Cluster 桌面客户端:Electron + React + TypeScript + Vite + Tailwind CSS,通过 Electron Forge 打包。它通过 HTTP/SSE 读取 Go Gateway 管理的节点、预约和历史数据;交互式终端在本机启动用户的 OpenSSH,不承载业务数据库逻辑。

```
Renderer (React, sandboxed)
   │ window.nexus.* (contextBridge)
Preload
   │ typed IPC
Electron Main (GatewayClient, TokenStore, SettingsStore, SSE, local OpenSSH/PTY)
   ├─ HTTP + SSE + JWT ── Go Gateway ── Redis / MySQL / Qdrant / Pi Runtime
   └─ local OpenSSH using this desktop user's keys/agent ── GPU nodes
```

## 环境要求

- Node.js ≥ 20(含 npm);建议与开发机一致(Node 24)
- macOS / Windows / Linux 均可开发;当前阶段重点保证 macOS(开发机)与 Linux 的 Forge maker 可用
- 首次 `npm install` 会下载 Electron 二进制;网络受限时可设置镜像:

```bash
export ELECTRON_MIRROR=https://npmmirror.com/mirrors/electron/
```

## 安装与开发

```bash
cd desktop
npm install
npm run dev     # electron-forge start:同时启动 Vite dev server 与 Electron
```

- 渲染层改动由 Vite HMR 热更新;主进程改动通过 `hotRestart` 自动重启应用
- 也可以在 `electron-forge start` 终端里输入 `rs` 手动重启
- DevTools:开发模式可用 `Cmd/Ctrl+Shift+I` 打开;生产包不会自动打开

## Gateway 配置

- 默认网关地址:`http://127.0.0.1:3000`
- 登录页右上角 “Gateway / 网关” 面板或应用内 “Settings” 页均可:
  - 修改 Gateway URL
  - Test Connection(调用 `GET /api/version`,5 秒超时)
  - Save(持久化到 `app.getPath("userData")/settings.json`)
- Renderer 不直接访问配置文件,一切经 IPC 读写。

## 空闲 GPU 与个人预约

- “空闲 GPU”页按可用显存、系统内存、GPU/CPU 型号、GPU 利用率上限、活跃 GPU 进程和持续空闲时间筛选；历史窗口不足或采样缺口太大时不会判断为稳定空闲。
- 预约由 Gateway 按当前平台用户保存,支持暂停/恢复、一次或重复提醒以及到期时间。它只触发提醒,不会锁定 GPU；其他用户仍可在同一张卡上启动任务。
- 提醒匹配与系统告警在桌面客户端登录运行期间执行；关闭客户端后不发送通知。

## 资源历史与系统通知

- Redis 提供近 25 小时高频样本;Gateway 每小时保存一条紧凑快照到 MySQL,默认保留 90 天。
- 节点详情提供 1、7、30、90 天历史范围,展示 CPU/系统内存概况和逐 GPU 利用率热力图。新数据从 Gateway 启动后开始积累,迁移前不会凭空补齐历史。
- 离线、GPU 过热、持续空闲、持续高负载、显存释放和 GPU 进程结束通知由 Electron 发送;通知阈值保存在本机偏好中。

## 本地 OpenSSH 终端

- 所有已登录用户都可以从节点详情打开终端,输入自己的 Linux 用户名,或使用本机 `~/.ssh/config` 别名。服务器账号本身决定实际命令权限。
- Electron 主进程以 `node-pty` 启动本机 `ssh`/`ssh.exe`;私钥、SSH Agent 和密码提示留在本机,不会经过 Gateway 或写入应用数据库。
- OpenSSH 配置解析结果必须匹配当前选中节点的主机和端口;Host Key 确认沿用本机 `known_hosts`。用户电脑需要能通过本地网络/VPN 访问目标服务器。
- Windows 启动环境保留 `ProgramData` 等 OpenSSH 系统路径变量，并显式使用 `TERM=xterm-256color`；应用/模型密钥不会传给 SSH。长文本输入按 UTF-8 字节限制分块串行发送，连接后与窗口缩放时同步终端尺寸。

与 RackTop 的终端能力对照：

| 能力 | Nexus 当前实现 |
| --- | --- |
| 本地 OpenSSH + 原生 PTY + xterm | 已实现，Windows 原生验证 |
| SSH Config 别名、个人 Linux 用户、密钥/Agent/密码提示 | 已实现；密钥和密码交互已用临时服务验证，真实个人 Agent 由本机 OpenSSH 管理 |
| Host Key 确认与变化阻止 | 沿用本机 OpenSSH 策略；已验证首次确认及指纹变化拒绝 |
| 输入/输出、连接就绪握手、缩放、退出、重新连接 | 已实现；主进程链路已通过真实终端管理器验证 |
| 大段文本粘贴 | 已实现 Unicode 分块并保持发送顺序；xterm 处理 bracketed paste |
| 指定 GPU 的终端与 CUDA 命令绑定检查 | 尚未迁移；当前入口是服务器通用终端 |
| RackTop 托管密码/SSH_ASKPASS | 不采用；按本项目确认的身份模型，用户在自己的 OpenSSH 会话中认证 |

## SSH Node Setup

Add Node 弹窗默认使用 **SSH(agentless)** 采集方式。管理员填写服务器地址、SSH 端口和服务器上已有账号的用户名；首次连接时可输入该账号密码，让 Gateway 把自己的公钥一次性加入 `authorized_keys`。密码随后丢弃，指标采集改用 Gateway 专用 SSH 密钥。

- Gateway 的 Ed25519 身份默认保存在 Docker 的 `ssh-key-data` 持久卷中；不要删除该卷。也可用 `SSH_PRIVATE_KEY_PATH` 配置现有未加密私钥。
- Desktop 会把管理员电脑 `~/.ssh/known_hosts` 中匹配的主机密钥用于首次信任；无预置信任记录时自动固定首次观察到的主机密钥。之后密钥变化会阻止连接。
- 首次引导密码只通过 HTTPS 发送到远程 Gateway。本机 `127.0.0.1` / `localhost` 的 loopback HTTP 可用；Gateway 也会执行服务端传输检查。
- 服务器无需安装 Agent，需提供 `sshd`、普通 shell、`/proc` 和 `ps`；读取 NVIDIA GPU 指标时还需 `nvidia-smi`。
- 已登录用户都可查看全局节点指标；添加和删除节点仅限管理员。管理员身份不拥有节点，所有管理员对同一份节点列表操作。旧版 Agent(URL 方式)仍可在 Collector 下拉框中选择。

## npm scripts

| 命令 | 作用 |
| --- | --- |
| `npm run dev` / `start` | 启动开发环境(Vite + Electron) |
| `npm run typecheck` | `tsc --noEmit`,strict 模式,必须始终通过 |
| `npm test` | vitest 单元测试(SSE、节点逻辑、空闲 GPU 稳定窗口、通知偏好等) |
| `npm run test:terminal-smoke` | Electron 原生 PTY/OpenSSH 回环测试(需要 Go 和本机 OpenSSH;临时密钥自动清理) |
| `npm run build` | 渲染层生产构建(自检用;打包时 Forge 会自行构建) |
| `npm run package` | 生成平台可执行包,输出到 `out/` |
| `npm run make` | 生成发布物(zip/dmg、Linux deb/rpm、Windows squirrel),输出到 `make/` |

打包产物:`out/Nexus Desktop-<platform>-<arch>/Nexus Desktop.app`(macOS)或对应目录;可执行文件名为 `nexus-desktop`。`out/` 与 `make/` 均已 gitignore。

当前阶段未包含:代码签名、公证、自动更新、系统托盘完整功能(见任务计划的“明确不实现”清单)。

## 目录结构

```
desktop/
├── electron/
│   ├── main.ts               # 窗口、生命周期、服务装配
│   ├── preload.ts            # contextBridge 只暴露业务 API
│   ├── env.d.ts              # Forge Vite 注入的构建常量类型
│   ├── ipc/                  # auth / settings / nodes / reservations / notifications / terminal / users / ai
│   ├── lib/                  # 纯逻辑:SSE 增量解析器、参数校验(可单测)
│   ├── services/             # Gateway client、本地 OpenSSH PTY、token/settings store、错误规范化
│   └── types/ipc.ts          # 主进程/渲染层共享的 IPC 契约(NexusAPI)
├── src/                      # React 渲染层
│   ├── pages/                # LoginPage / DashboardPage / SettingsPage
│   ├── components/           # nodes / gpu / ai / admin / settings / common / layout
│   ├── hooks/                # useAuth / useNodes(2s 轮询)/ useAIStream / useAdminUsers ...
│   ├── context/              # Auth / Language / Toast
│   ├── i18n/                 # en/zh 字典 + desktop 补充 + login 字典
│   └── lib/                  # node 派生逻辑、格式化、AI 文本清洗
├── tests/                    # vitest 单元测试
└── scripts/extract-dict.mjs  # 从旧 web 前端复现 i18n 字典提取
```

## IPC API(window.nexus)

所有方法返回统一信封 `{ ok: true, data } | { ok: false, error: { code, detail?, status?, message } }`;事件订阅返回取消函数。

| 命名空间 | 方法 | 说明 |
| --- | --- | --- |
| `auth` | `login` `register` `logout` `me` `onAuthExpired` | JWT 只存主进程;401 广播过期 |
| `nodes` | `overview` `history` `list` `add` `remove` `testSSH` | `/api/nodes/overview`、`/api/nodes/:id/history` 与 `/api/config`;`testSSH` 走 `POST /api/config/test-ssh`(仅管理员) |
| `idleReservations` | `list` `create` `setStatus` `evaluate` `remove` | `/api/idle-reservations`;只读写当前平台用户自己的提醒 |
| `notifications` | `show` | Electron 主进程发送本机系统通知 |
| `terminal` | `start` `attach` `write` `resize` `close` `onOutput` `onExit` | 本机 OpenSSH PTY;attach 就绪握手保留首次输出和提前退出事件;远端 SSH 凭据不经过 Gateway |
| `users` | `list` `create` `remove` `setRole` `resetPassword` | 管理员接口 |
| `ai` | `start(requestId, query)` `cancel(requestId)` `onEvent` | SSE 在主进程解析,事件类型 `start/status/delta/meta/done/error` |
| `settings` | `get` `update` `testConnection` | 网关地址管理 |

## Electron 安全模型

- `nodeIntegration: false`,`contextIsolation: true`,`sandbox: true`
- Preload 通过 `contextBridge` 只暴露 `window.nexus.{auth,nodes,idleReservations,notifications,terminal,users,ai,settings}`;不暴露 `ipcRenderer`
- JWT 仅保存在主进程(`safeStorage` 加密,无 OS keyring 的 Linux 上明文回退,文件头有版本标记),Renderer 永远拿不到 token
- 所有 IPC 参数在主进程做运行时校验(renderer 输入不可信);Gateway 响应同样经过 sanitize 再进入 Renderer
- 错误信息经规范化后展示;控制台日志不打印 token / 密码 / 内部凭据
- 401 时主进程清除 token 并广播 `auth:expired`,由 AuthContext 复位会话
- 交互终端由主进程以 `node-pty` 启动用户本机 OpenSSH;Host/Port 必须匹配当前节点,不使用 Gateway 采集私钥,不把输入、私钥或密码转发给 Gateway
- 空闲 GPU 预约保存在 Gateway MySQL 并按平台用户隔离;预约只保存提醒条件,不提供 GPU 独占锁
- 历史视图合并 Redis 近 25 小时高频样本和 MySQL 90 天小时样本;新部署的长周期数据从 Gateway 启动后开始积累

## 行为基准与 i18n

迁移以 `web/dashboard/index.html` 的真实行为为准:2 秒节点轮询、搜索、四种排序(availability/name/cpu/users,其中 cpu 与 users 为“最空闲在前”的升序)、GPU 矩阵与内联详情、Availability Score/Tier、GPU Users 抽屉、确认/输入对话框、中英文切换(`localStorage["nexus_language"]`,en 为兜底语言)、AI 流式问答(`status/delta/meta/done/error` + Stop)。

- `src/i18n/{en,zh}.ts` 由 `scripts/extract-dict.mjs` 从 `web/dashboard/index.html`(L588–1147)自动提取;`src/i18n/login.ts` 来自 `web/login/login-app.js`(L95–244)
- 桌面新增文案(AI 面板、设置页、错误码)在 `src/i18n/desktop.ts`,新增键必须中英同步
- 修改提取规则后运行 `node desktop/scripts/extract-dict.mjs` 可复现生成

## 测试

```bash
npm test        # 全量 vitest
npm test -- sse # 单文件过滤
```

覆盖:`electron/lib/validate.ts`、`electron/lib/sse.ts`、token/settings store、节点派生逻辑、空闲 GPU 阈值/稳定时间窗口、通知偏好归一化和 AI 文本清洗。

`npm run test:terminal-smoke -- --full-electron` 编译并调用实际 `LocalSshTerminalManager`，用回环 SSH 服务验证系统配置预检、精简进程环境、密钥/密码交互、首次指纹确认、变化拒绝、输出握手、输入/缩放和退出/断开。夹具只给 SSH 调用增加 `-F` 指向临时配置，不替换生产环境构造函数或会话逻辑。打包后再传 `--pty-module <resources/app.asar/node_modules/node-pty 的绝对路径>` 核验 ASAR 加载与 worker。测试不会连接生产服务器，临时密钥在结束后清理。

## 端到端冒烟清单

前提:`docker compose up -d gateway pi-runtime`(macOS 上 `agent` 服务因无 GPU 驱动无法启动,属预期)。

1. 打开应用 → 登录页右上角 Gateway 面板 → Test Connection 显示版本号 → Save
2. 登录(`admin/admin123` 或注册新账号)→ 进入 Dashboard
3. 节点列表每 2 秒刷新;点击节点查看 GPU 矩阵与内联详情
4. 管理员:添加节点(重复 URL 会被拦截)、删除节点、用户管理(创建/改角色/重置密码/删除)
5. AI 助手:输入“哪个节点现在最空闲?”→ 观察 thinking 文案 → 流式增量 → 详情块(推理摘要/相关节点/警告)→ 再发一条并点 Stop
6. 打开空闲 GPU 页 → 设置显存/进程/稳定时长 → 保存个人提醒 → 暂停、恢复和删除提醒
7. 打开节点历史 → 切换 1/7/30/90 天 → 查看 CPU/内存趋势和 GPU 热力图
8. 打开节点终端 → 选择本机 SSH Config 或输入 Linux 用户名 → 核验本机 Host Key 提示 → 输入命令 → 关闭会话
9. 退出登录 → 重启应用 → 会话应保持(重启前已登录时);登出后 token 已清除

## 常见问题

- **Electron 下载慢/失败**:设置 `ELECTRON_MIRROR` 后删除 `node_modules/electron` 重新 `npm install`
- **5173 端口被占**:Vite 会自动换端口,主进程通过构建期常量拿到实际地址,无需干预
- **Linux 上 token 明文落盘**:无 OS keyring 时 `safeStorage` 不可用,TokenStore 明文回退(文件头标记区分);如有 keyring(如 gnome-keyring)则自动加密
- **打包版首次打开被 Gatekeeper 拦截**:未签名/公证所致,右键 →「打开」
- **改了 `electron/` 没生效**:dev 模式下主进程会自动热重启;若被 IDE 冻结可手动输入 `rs`

## 与旧 Web 前端的关系

`web/` 旧前端完整保留并由 Gateway 继续托管,仍是现有监控/API 行为基准。空闲算力预约、系统通知、本地 SSH 终端和长周期历史界面目前属于 Electron 桌面能力;Web 客户端不会访问本机 OpenSSH 或 SSH Agent。
