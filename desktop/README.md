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

- Node.js ≥ 20(含 npm)
- 首次安装会下载 Electron 二进制;网络受限时可设置镜像,例如 `ELECTRON_MIRROR=https://npmmirror.com/mirrors/electron/`

## 安装与开发

```bash
cd desktop
npm install
npm run dev     # 同时启动 Vite dev server 与 Electron
```

## Gateway 配置

- 默认网关地址:`http://127.0.0.1:3000`
- 登录页右上角 “Gateway / 网关” 面板或应用内 “Settings” 页均可:
  - 修改 Gateway URL
  - Test Connection(调用 `GET /api/version`)
  - Save(持久化到 `app.getPath("userData")/settings.json`)
- Renderer 不直接访问配置文件,一切经 IPC 读写。

## 生产构建与打包

```bash
npm run build        # renderer 生产构建(自检用)
npm run typecheck    # tsc --noEmit(strict)
npm test             # vitest:SSE parser / URL 校验 / TokenStore / 节点逻辑 / AI 文本
npm run package      # 输出 out/<platform> 可执行包
npm run make         # 生成发布物(zip/dmg;Linux deb/rpm;Windows squirrel)
```

当前阶段重点保证 macOS(开发机)与 Linux 的 Forge maker 配置可用;未包含签名、公证与自动更新。

## 目录结构

```
desktop/
├── electron/
│   ├── main.ts               # 窗口、生命周期、服务装配
│   ├── preload.ts            # contextBridge 只暴露业务 API
│   ├── env.d.ts              # Forge Vite 注入的构建常量
│   ├── ipc/                  # auth / settings / nodes / users / ai 处理器
│   ├── lib/                  # 纯逻辑:sse 解析器、参数校验
│   ├── services/             # gateway client、token-store、settings-store、错误规范化
│   └── types/ipc.ts          # 主进程/渲染层共享的 IPC 契约
├── src/                      # React 渲染层
│   ├── pages/                # LoginPage / DashboardPage / SettingsPage
│   ├── components/           # nodes / gpu / ai / admin / common / layout
│   ├── hooks/                # useAuth / useNodes / useAIStream / useAdminUsers ...
│   ├── context/              # Auth / Language / Toast
│   ├── i18n/                 # en/zh 字典(从 web/dashboard 提取)+ desktop 补充
│   └── lib/                  # node 派生逻辑、格式化、AI 文本清洗
├── tests/                    # vitest 单元测试
└── scripts/extract-dict.mjs  # 从旧 web 前端复现 i18n 字典提取
```

## Electron 安全模型

- `nodeIntegration: false`,`contextIsolation: true`,`sandbox: true`
- Preload 通过 `contextBridge` 只暴露 `window.nexus.{auth,nodes,users,ai,settings}`;不暴露 `ipcRenderer`
- JWT 仅保存在主进程(`safeStorage` 加密,无 keyring 时明文回退),Renderer 永远拿不到 token
- 所有 IPC 参数在主进程做运行时校验;Gateway 响应也经过 sanitize 再进入 Renderer
- 401 时主进程清除 token 并广播 `auth:expired`,由 AuthContext 复位会话

## 行为基准

迁移以 `web/dashboard/index.html` 的真实行为为准:2 秒节点轮询、搜索/四种排序、GPU 矩阵与内联详情、Availability Score/Tier、GPU Users 抽屉、中英文双语、AI 流式问答(`status/delta/meta/done/error` + Stop 取消)。`scripts/extract-dict.mjs` 可从旧前端复现字典提取。
