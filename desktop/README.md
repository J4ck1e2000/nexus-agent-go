# Nexus Desktop

Nexus GPU Cluster 桌面客户端:Electron + React + TypeScript + Vite + Tailwind CSS,通过 Electron Forge 打包。它替代旧的 CDN 版 Web Dashboard(`web/`,保留在原处作为行为基准),通过 HTTP/SSE 访问现有 Go Gateway,不承载任何业务数据库逻辑。

```
Renderer (React, sandboxed)
   │ window.nexus.* (contextBridge)
Preload
   │ typed IPC
Electron Main (GatewayClient, TokenStore, SettingsStore, SSE)
   │ HTTP + SSE + JWT
Go Gateway  ──  Redis / MySQL / Qdrant / Pi Runtime ── GPU Agents
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
| `npm test` | vitest 单元测试(SSE 解析、URL 校验、TokenStore、节点逻辑、AI 文本) |
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
│   ├── ipc/                  # auth / settings / nodes / users / ai 处理器
│   ├── lib/                  # 纯逻辑:SSE 增量解析器、参数校验(可单测)
│   ├── services/             # gateway client、token-store、settings-store、错误规范化
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
| `nodes` | `overview` `list` `add` `remove` `testSSH` | `/api/nodes/overview` 与 `/api/config`;`testSSH` 走 `POST /api/config/test-ssh`(仅管理员) |
| `users` | `list` `create` `remove` `setRole` `resetPassword` | 管理员接口 |
| `ai` | `start(requestId, query)` `cancel(requestId)` `onEvent` | SSE 在主进程解析,事件类型 `start/status/delta/meta/done/error` |
| `settings` | `get` `update` `testConnection` | 网关地址管理 |

## Electron 安全模型

- `nodeIntegration: false`,`contextIsolation: true`,`sandbox: true`
- Preload 通过 `contextBridge` 只暴露 `window.nexus.{auth,nodes,users,ai,settings}`;不暴露 `ipcRenderer`
- JWT 仅保存在主进程(`safeStorage` 加密,无 OS keyring 的 Linux 上明文回退,文件头有版本标记),Renderer 永远拿不到 token
- 所有 IPC 参数在主进程做运行时校验(renderer 输入不可信);Gateway 响应同样经过 sanitize 再进入 Renderer
- 错误信息经规范化后展示;控制台日志不打印 token / 密码 / 内部凭据
- 401 时主进程清除 token 并广播 `auth:expired`,由 AuthContext 复位会话

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

覆盖:`electron/lib/validate.ts`(Gateway URL、用户名/密码/requestId 规则,与 Gateway 的 Go 校验一致)、`electron/lib/sse.ts`(跨 chunk、CRLF、多行 data、注释行)、`token-store`(加密/明文回退/损坏容错)、`settings-store`(默认值/回写)、`src/lib/node-logic.ts`(GPU 汇总、可用性评分、排序、URL 规范化)、`src/lib/ai-text.ts`(AI 文本清洗、meta 归一化、错误本地化)。

## 端到端冒烟清单

前提:`docker compose up -d gateway pi-runtime`(macOS 上 `agent` 服务因无 GPU 驱动无法启动,属预期)。

1. 打开应用 → 登录页右上角 Gateway 面板 → Test Connection 显示版本号 → Save
2. 登录(`admin/admin123` 或注册新账号)→ 进入 Dashboard
3. 节点列表每 2 秒刷新;点击节点查看 GPU 矩阵与内联详情
4. 管理员:添加节点(重复 URL 会被拦截)、删除节点、用户管理(创建/改角色/重置密码/删除)
5. AI 助手:输入“哪个节点现在最空闲?”→ 观察 thinking 文案 → 流式增量 → 详情块(推理摘要/相关节点/警告)→ 再发一条并点 Stop
6. 退出登录 → 重启应用 → 会话应保持(重启前已登录时);登出后 token 已清除

## 常见问题

- **Electron 下载慢/失败**:设置 `ELECTRON_MIRROR` 后删除 `node_modules/electron` 重新 `npm install`
- **5173 端口被占**:Vite 会自动换端口,主进程通过构建期常量拿到实际地址,无需干预
- **Linux 上 token 明文落盘**:无 OS keyring 时 `safeStorage` 不可用,TokenStore 明文回退(文件头标记区分);如有 keyring(如 gnome-keyring)则自动加密
- **打包版首次打开被 Gatekeeper 拦截**:未签名/公证所致,右键 →「打开」
- **改了 `electron/` 没生效**:dev 模式下主进程会自动热重启;若被 IDE 冻结可手动输入 `rs`

## 与旧 Web 前端的关系

`web/` 旧前端完整保留并由 Gateway 继续托管,桌面客户端与其功能对齐、可同时使用;在桌面端未明确覆盖的细节(如移动端样式)不迁移。删除 `web/` 属于未来的破坏性迁移,不在本阶段范围内。
