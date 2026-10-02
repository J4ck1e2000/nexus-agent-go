# AGENTS.md

面向 AI 编码代理(及新成员)的工程指南。项目全貌见 [readme.md](readme.md);桌面客户端细节见 [desktop/README.md](desktop/README.md)。

## 项目是什么

分布式 GPU 集群监控与调度辅助平台:

- **Go Gateway**(`cmd/gateway`,`internal/gateway`):JWT 认证、节点配置与聚合状态(Redis + MySQL)、管理 API、AI 问答 SSE 入口,并托管 `web/` 静态页;
- **Go Agent**(`cmd/agent`):跑在 GPU 节点上采集 CPU/RAM/GPU/进程指标;
- **Pi Runtime**(`pi-runtime/`,TypeScript):**唯一** AI 执行器,经工具网关回调 Gateway,`/api/ai/query` 的 SSE 由它透传;
- **web/**(CDN React 单文件):旧 Web 前端,**保留**,是一切前端迁移的行为基准;
- **desktop/**(Electron + React + TS + Vite + Tailwind + Forge):新桌面客户端,与 web/ 功能对齐。

架构:Browser/Electron → Gateway → {Redis, MySQL, Qdrant, Pi Runtime} ← GPU Agents。

## 常用命令

所有命令在仓库根执行,除非注明。

### Go(Backend)

```bash
go build ./...            # 编译
go test ./...             # 全量单测
go test ./internal/gateway -run TestNodesOverview -v   # 单包单个用例
go run ./cmd/gateway      # 本地起 Gateway(需先起 Pi Runtime 并设 PI_RUNTIME_URL)
go vet ./...
```

### desktop/(Electron)

```bash
cd desktop
npm install
npm run dev        # Vite + Electron 一键起
npm run typecheck  # tsc --noEmit(strict),必须保持通过
npm test           # vitest(51+ 用例)
npm run build      # renderer 生产构建自检
npm run package    # 打包到 out/
npm run make       # 发布物到 make/
```

### pi-runtime/

```bash
cd pi-runtime
npm install
npm run typecheck
npm run test:e2e   # mock 模式 e2e
npm start          # 需要 PI_RUNTIME_TOKEN / AI_API_KEY 等(见 readme)
```

### 本地后端栈

```bash
docker compose up -d gateway pi-runtime   # 附带 mysql/redis/qdrant
curl http://127.0.0.1:3000/api/version
# 注意:agent 服务声明了 GPU capability,在 macOS 上起不来,属预期
```

`.env` 必改项:`MYSQL_ROOT_PASSWORD`、`JWT_SECRET`、`AI_API_KEY`。测试账号:`admin/admin123`。

## 架构不变量(改代码前必读)

1. **Gateway API 兼容**:不重命名/不删除既有端点与 JSON 字段(web/ 与 desktop/ 双客户端依赖)。Go 结构体 json tag 是跨端契约,`nil` slice 会序列化为 `null`,TS 类型必须如实标注可空。
2. **Pi Runtime 是唯一 AI 执行器**:不要恢复规则回答路径,不要绕过 runtime 直接调用 LLM。
3. **不要动的算法**:节点轮询、Availability Score/Tier、调度器、Redis 热状态层。
4. **web/ 保留不删**:它是行为基准;删除属于未来的破坏性迁移。
5. **desktop 安全模型**:`nodeIntegration=false`、`contextIsolation=true`、`sandbox=true`;preload 只经 contextBridge 暴露 `window.nexus.*`;JWT 只存主进程(safeStorage),Renderer 不持有;所有 IPC 参数在主进程校验;错误文案本地化,日志不打 token/密码。
6. **desktop 新增功能须中英双语**:en/zh 字典同步,键在 `desktop/src/i18n/desktop.ts`(AI 面板/设置/错误码)或提取字典。

## 代码风格

- Go:标准 `gofmt`;注释与领域命名沿用现有中文注释风格;错误用 sentinel error + `errors.Is`。
- TypeScript(desktop 与 pi-runtime):strict,禁止大面积 `any`;共享 IPC 契约只写在 `desktop/electron/types/ipc.ts`;渲染层禁止直接 fetch(一律走 IPC)。
- 提交信息:Conventional Commits,如 `feat(desktop): ...`、`fix(gateway): ...`、`test(pi-runtime): ...`。
- 分支:开发在 `feature/*` 分支;**不要直接提交 main**,合并走 PR。

## 测试指引

- Go:各包内 `*_test.go`,Gateway handler 测试在 `internal/gateway/*_test.go`(含 AI 流、节点概览、管理员的表驱动用例)。
- desktop:`desktop/tests/*.test.ts`(vitest)——SSE 解析器、URL/凭据校验、TokenStore/SettingsStore、节点派生逻辑、AI 文本清洗。新增纯逻辑(可函数化的业务规则)应补测试。
- pi-runtime:`npm run test:e2e`(mock)。
- 前端行为改动的验证基准:同场景下旧 web 前端的行为(排序方向、格式化、轮询节奏、SSE 事件处理)。

## 已知坑

- **Forge Vite 插件**(desktop):main/preload 产物是 `.vite/build/*.cjs`;dev server URL 是构建期 define 注入的**裸全局常量** `MAIN_WINDOW_VITE_DEV_SERVER_URL`(不是 `import.meta.env.*`),类型在 `desktop/electron/env.d.ts`;`vite.*.config.ts` 保持极简,输出目录约定归插件管。
- **desktop i18n 字典是生成的**:`desktop/src/i18n/{en,zh,login}.ts` 来自 `desktop/scripts/extract-dict.mjs`(源:web/dashboard L588–1147、web/login L95–244),手工改动会被下次提取覆盖——桌面专属键放 `desktop/src/i18n/desktop.ts`。
- **JSON 可空性**:Go 的 `*int`/`omitempty`/nil slice 会让字段缺失或为 null,TS 侧用 `? | null` 并在渲染层做兜底。
- **SSE 事件契约**:`start/status/delta/meta/done/error`,`data:` 为 JSON;meta/done 携带 `reasoning_summary`、`related_nodes`、`tool_calls`、`warnings`(snake_case,渲染层兼容 camelCase 别名)。
- **端口**:Gateway `:3000`,Pi Runtime 默认 `:8010`;5173 被占时 Vite 自动换端口,desktop 主进程会拿到实际地址。
