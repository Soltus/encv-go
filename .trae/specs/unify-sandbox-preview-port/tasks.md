# Tasks

> Spec: [spec.md](file:///workspace/.trae/specs/unify-sandbox-preview-port/spec.md)
> 目标：preview-gateway 监听 :16666（用户决策 D1："好记"），外网链 `外网 → :16000 (OpenPreview) → :16666 (preview-gateway) → 4 个 upstream`，本地 dev / agent-browser 直接访问 :16666。Vite 监听 :8100（vite.config.ts 已声明），是 preview-gateway 的 fallthrough 上游。
>
> **追加目标（本轮）**：修复 `/openlist-ui/` 空白 + 主 app `/tabs/openlist` 空白 + 完善防御性 UI（404 兜底 + 错误边界）。

---

## 基础架构

- [x] **Task 1**: 改造 preview-gateway 监听 :16666
  - [x] SubTask 1.1: 修改 `app/preview-gateway/src/server.ts` 默认监听端口 `:16000` → `:16666`
  - [x] SubTask 1.2: 修改 fallthrough 目标 `:5173` → `:8100`（vite 新端口）
  - [x] SubTask 1.3: 修改 WebSocket upgrade 转发目标 `:5173` → `:8100`
  - [x] SubTask 1.4: 重新 `pnpm build` 验证编译通过
  - [x] SubTask 1.5: 修改启动日志（"D1: 好记"）

- [x] **Task 2**: start-preview.sh 显式指定 vite 监听 :8100
  - [x] SubTask 2.1: 找到 `app/encv-mobile/scripts/start-preview.sh` 中 vite 启动命令
  - [x] SubTask 2.2: 把 `--port 5174 --strictPort` 等显式参数改为 `--port 8100 --strictPort`
  - [x] SubTask 2.3: 确认 pm2 重启后 vite 在 :8100 监听
  - [x] SubTask 2.4: 现有 pm2 进程 :5173 占用需要先 kill（避免端口冲突）

- [x] **Task 3**: 撤销 vite.config.ts 的所有反向代理胶水（D9 决策）
  - [x] SubTask 3.1: 撤销 `cors: { origin: '*' }` 硬编码
  - [x] SubTask 3.2: 删除 `server.proxy: { '/api': 2025, '/openlist/': 2025, '/p': 2025, '/play': 2025 }` 块
  - [x] SubTask 3.3: 移除 `openlistUiProxy` plugin 引用
  - [x] SubTask 3.4: 端到端验证：:16666/tabs/openlist 仍能加载 OpenListView

- [x] **Task 4**: pm2 注册 preview-gateway 进程（端口 :16666）
  - [x] SubTask 4.1: `ecosystem.config.cjs` 加 `preview-gateway` app（PORT=16666）
  - [x] SubTask 4.2: `setup-sandbox-env.sh` 加启动步骤（`pnpm install` + `pnpm build` + `pm2 start`）
  - [x] SubTask 4.3: 启动顺序：preview-gateway 必须在 vite 之前起来（先占 :16666，vite 监听 :8100 无冲突）

- [x] **Task 5**: 端到端验证（spec.md §八 J1-J14）
  - [x] SubTask 5.1: J1-J5 本地 :16666 网关 + 4 上游
  - [x] SubTask 5.2: J6 WebSocket upgrade 转发到 :8100 vite
  - [x] SubTask 5.3: J7 health 端点
  - [x] SubTask 5.4: J8-J9 agent-browser 浏览器实测 :16666/tabs/openlist
  - [x] SubTask 5.5: J10 pm2 重启自愈
  - [x] SubTask 5.6: J11-J12 vite.config.ts 干净度
  - [x] SubTask 5.7: J13 vite :8100 + :5173 端口空闲
  - [x] SubTask 5.8: J14 agent-browser navigate :16666 触发 preview-proxy 自动注册

- [x] **Task 6**: 文档（app/preview-gateway/README.md）
  - [x] SubTask 6.1: 架构图
  - [x] SubTask 6.2: 启动步骤
  - [x] SubTask 6.3: 故障排查
  - [x] SubTask 6.4: 端口决策说明

---

## 本轮新增：修复 /openlist-ui/ 空白 + 防御性 UI（D11 + D12 + D13）

- [x] **Task 7**: 修复 vite.config.ts 沙箱 dev 资源路由（D11）
  - [x] SubTask 7.1: `app/encv-mobile/plugin-openlist/web/index.html` 加 `<!--VITE-BASE-HREF-PLACEHOLDER-->` 占位符到 `<head>` 顶部
  - [x] SubTask 7.2: vite.config.ts 加 `injectBaseHref` plugin（`order: 'post'` 替换 placeholder + 改 `@vite/client` src）
  - [x] SubTask 7.3: vite.config.ts 扩展 `server.fs.allow` 到 `/workspace`、`/workspace/app/encv-mobile` 等（修复 `/@fs/...` 返回 text/html）
  - [x] SubTask 7.4: `scripts/dev-openlist-web.sh` 加 `export VITE_BASE="/openlist-ui/"`
  - [x] SubTask 7.5: `ecosystem.config.cjs` plugin-openlist-vite 加 `VITE_BASE: '/openlist-ui/'` 环境变量

- [x] **Task 8**: 修复 preview-gateway 子资源路由（D11 + D12）
  - [x] SubTask 8.1: `Upstream` interface 加 `pathRewrite?: (path: string) => string` 字段
  - [x] SubTask 8.2: `/openlist-ui` upstream 加 `pathRewrite: (p) => p.replace(/^\/openlist-ui(?=\/|$)/, '') || '/'`
  - [x] SubTask 8.3: HTTP proxy.web 应用 `pathRewrite`（修改 `req.url`）
  - [x] SubTask 8.4: WebSocket upgrade 转发也应用 `pathRewrite`（HMR 路径）
  - [x] SubTask 8.5: `pickUpstream` 加 `cookie` 参数 + Cookie 路由识别 plugin SPA 来源
  - [x] SubTask 8.6: `proxy.on('proxyRes')` 钩子对 `text/html` 响应注入 `Set-Cookie: __plugin_spa=1; Path=/; SameSite=Lax; Max-Age=3600`
  - [x] SubTask 8.7: WebSocket upgrade handler 也传 `req.headers.cookie` 给 `pickUpstream`

- [x] **Task 9**: 修复 plugin-openlist-web vue-router 路由（D11）
  - [x] SubTask 9.1: `src/router/index.ts` `createWebHashHistory('/openlist-ui/')`（base 参数关键）
  - [x] SubTask 9.2: 路由表加 `/:pathMatch(.*)*` catch-all → `NotFoundView`

- [x] **Task 10**: 主 app 防御性 UI（D13 Layer 1 + Layer 3）
  - [x] SubTask 10.1: 新建 `app/encv-mobile/src/views/NotFoundView.vue`（ionic UI + 已知路由列表 + debug 面板）
  - [x] SubTask 10.2: `app/encv-mobile/src/router/index.ts` 加 catch-all `/:pathMatch(.*)*` 路由
  - [x] SubTask 10.3: `app/encv-mobile/src/App.vue` 加 `onErrorCaptured` + `rootError` 状态 + 错误屏 UI
  - [x] SubTask 10.4: import `bugOutline` 图标 + CSS 样式

- [x] **Task 11**: plugin-openlist-web 防御性 UI（D13 Layer 1 + Layer 3）
  - [x] SubTask 11.1: 新建 `app/encv-mobile/plugin-openlist/web/src/views/NotFoundView.vue`（纯 Vue + 已知路由列表 + debug 面板）
  - [x] SubTask 11.2: 路由表加 catch-all（Task 9.2 已完成）
  - [x] SubTask 11.3: `app/encv-mobile/plugin-openlist/web/src/App.vue` 加 `onErrorCaptured` + `rootError` 状态 + 错误屏 UI

- [x] **Task 12**: 端到端验证 6+ 场景
  - [x] SubTask 12.1: `curl -sI http://localhost:16666/` → 200 vite HTML（主 app fallthrough）
  - [x] SubTask 12.2: `curl -sI http://localhost:16666/openlist-ui/` → 200 + Set-Cookie: `__plugin_spa=1`
  - [x] SubTask 12.3: `curl -sI -H "Cookie: __plugin_spa=1" http://localhost:16666/src/views/OpenListHome.vue` → text/javascript（cookie 路由）
  - [x] SubTask 12.4: `curl -sI http://localhost:16666/src/views/OpenListHome.vue` → text/html（无 cookie fallthrough）
  - [x] SubTask 12.5: `curl -s http://localhost:16666/openlist-ui/ | grep /openlist-ui/@vite` → 验证 injectBaseHref 工作
  - [x] SubTask 12.6: `pm2 list` → preview-gateway / plugin-openlist-vite / start-preview / openlist 全部 online
  - [x] SubTask 12.7: `ss -tlnp | grep -E "(:16666|:8100|:5174|:2025|:5244)"` → 5 端口全部监听

- [x] **Task 13**: 文档同步
  - [x] SubTask 13.1: spec.md 追加 D11/D12/D13 决策（§2.2 表格追加 3 行）
  - [x] SubTask 13.2: spec.md 追加 §十「防御性 UI 设计」（三层防护架构 + NotFoundView + onErrorCaptured）
  - [x] SubTask 13.3: spec.md 追加 §十一「沙箱 dev 资源路由：Cookie 标记机制」（五层修复链 + 请求流 + 验证命令）
  - [x] SubTask 13.4: tasks.md 追加 Task 7-13（本文件）
  - [x] SubTask 13.5: checklist.md 追加 D11/D12/D13 验证项
  - [x] SubTask 13.6: preview-gateway/README.md 追加「沙箱 dev 资源路由：Cookie 标记机制」章节

---

# Task Dependencies

- [Task 1] depends on 现有 preview-gateway 项目
- [Task 2] depends on [Task 1]
- [Task 3] depends on [Task 2]
- [Task 4] depends on [Task 1]
- [Task 5] depends on [Task 3] + [Task 4]
- [Task 6] depends on [Task 5]
- [Task 7] depends on [Task 4]（vite 已搬到 :8100 才能做沙箱 dev 配置）
- [Task 8] depends on [Task 7]（vite 侧改完才能在 gateway 配 pathRewrite）
- [Task 9] depends on [Task 8]（gateway 路由修好才能用 base=/openlist-ui/）
- [Task 10] depends on [Task 4]（主 app 已 :8100）
- [Task 11] depends on [Task 9]（plugin 路由修好才能加防御 UI）
- [Task 12] depends on [Task 10] + [Task 11]
- [Task 13] depends on [Task 12]

# Parallelizable

- [Task 7] + [Task 8] 可与 [Task 10] + [Task 11] 并行
- [Task 13] 文档可与 [Task 12] 部分并行
