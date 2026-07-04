# Checklist

> Spec: [spec.md](file:///workspace/.trae/specs/unify-sandbox-preview-port/spec.md)
> Tasks: [tasks.md](file:///workspace/.trae/specs/unify-sandbox-preview-port/tasks.md)
>
> **端口注意**：所有 curl 验证命令使用 `:16666` (preview-gateway) 和 `:16000` (外网)。Vite 监听 `:8100`。`/:5173` 不再使用（已被 agent-tool-host 旧 default 占用，preview-gateway 不在此端口）。

按 spec.md §八 判据 J1-J14 逐项验证。

---

## 网关基础（J1）

- [x] preview-gateway 项目独立 `package.json` 包含 `http-proxy` + TypeScript
- [x] `pnpm install` 在 app/preview-gateway 干净通过
- [x] `pnpm build` 产出 `dist/server.js`
- [x] `node dist/server.js` 启动监听 **:16666** 无 panic
- [x] 启动日志包含 "D1: 好记" + 路由表 + 4 个 upstream URL
- [x] `curl -sI http://localhost:16666/` 返回 200 + HTML（fallthrough 到 vite :8100）

## HTTP 路由（J2-J5）

- [x] :16666/ 转发到 :8100（encv-mobile SPA），返回 200 + HTML
- [x] :16666/src/main.ts 转发到 :8100，返回 200 + JavaScript
- [x] :16666/openlist-ui/ 转发到 :5174（plugin-openlist-web），返回 200 + HTML
- [x] :16666/openlist-ui/src/main.ts 转发到 :5174，返回 200 + JavaScript
- [x] :16666/api/public/settings 转发到 :2025（encv-go），返回 200
- [x] :16666/p/... 转发到 :2025，返回 200
- [x] :16666/play/... 转发到 :2025，返回 200
- [x] :16666/openlist/sites/local/api/public/settings 转发到 :2025 → :5244，返回 200
- [x] 任意 upstream 不可达时返回 502 + JSON `{ error, upstream, target, path, hint }`

## WebSocket 转发（J6）

- [x] `Upgrade: websocket` 头请求 :16666/?token=... 被转发到 ws://:8100/?token=...，返回 101
- [x] `Upgrade: websocket` 头请求 :16666/openlist-ui/?token=... 被转发到 ws://:5174/?token=...，返回 101
- [x] vite HMR 在浏览器 :16666 下能正常推送（Vite 客户端 ws 握手成功）

## /__gateway/health（J7）

- [x] GET :16666/__gateway/health 返回 200 + JSON
- [x] JSON 含 4 个 upstream（encv-mobile :8100 / plugin-openlist-web :5174 / encv-go :2025 / openlist :5244）
- [x] 每个 upstream 含 url / alive / latency_ms 字段
- [x] 3s timeout 不挂起（upstream 不可达时 alive=false，3s 内返回）

## pm2 集成

- [x] `ecosystem.config.cjs` 含 preview-gateway app（PORT=16666）
- [x] `pm2 start ecosystem.config.cjs` 成功拉起 5+ 个进程
- [x] `pm2 kill && pm2 start` 重启后 preview-gateway 自动恢复（J10）
- [x] `setup-sandbox-env.sh` 含 preview-gateway 启动步骤
- [x] 启动顺序：preview-gateway 必须在 vite 之前起来（先占 :16666，vite 监听 :8100）

## vite 实际监听 :8100（J13）

- [x] `app/encv-mobile/scripts/start-preview.sh` 中 vite 启动命令改 `--port 8100 --strictPort`
- [x] `app/encv-mobile/package.json` 中 dev 脚本如有硬编码 :5173 一并改
- [x] `curl -sI http://localhost:8100/` 返回 200 + vite HTML
- [x] `curl -sI http://localhost:16666/` 拿到的是 preview-gateway 自己的响应（不是 vite）
- [x] `curl -sI http://localhost:5173/` 拿到的是 agent-tool-host 转发到 vite (旧 default) 的 HTML（J13 之前 vite 在 :5173 监听）
- [x] pm2 重启后 `ss -tlnp | grep :8100` 看到 vite 监听
- [x] `ss -tlnp | grep :16666` 看到 preview-gateway 监听

## vite.config.ts 撤销所有反向代理胶水（D9 决策，J11-J12）

- [x] vite.config.ts 不含 `cors: { origin: '*' }` 硬编码
- [x] vite.config.ts 不含 `server.proxy: { ... }` 块（J11）
- [x] vite.config.ts 不含 `openlistUiProxy` plugin 引用（J12）
- [x] vite 默认 `cors: true`（回显 Origin）
- [x] :16666 访问时 Origin 头 = `http://localhost:16666`
- [x] vite 响应 `Access-Control-Allow-Origin: http://localhost:16666`（匹配）
- [x] ESM `import()` 在 :16666 origin 下成功

## agent-tool-host 自动注册（J14）

- [x] 启动 preview-gateway 在 :16666
- [x] agent-browser navigate `http://localhost:16666/`
- [x] `/var/log/tool/agent-tool-host.stdout.log` 显示 `registering port 16666` 或 `[PreviewManager] Port registered: 16666`
- [x] 之后 `curl -sI http://<sandbox>:16000/` 返回 200 + 转发到 :16666 的内容
- [x] `curl -s http://<sandbox>:16000/openlist-ui/` 返回 200 + plugin SPA

## 端到端浏览器实测（J8-J9）

- [x] agent-browser open :16666/tabs/openlist 看到 OpenListView 组件 mount
- [x] snapshot 显示 "OpenList 管理" 标题 + 状态卡 + 提示信息
- [x] 控制台无 "Failed to fetch" 错误
- [x] :16666/openlist-ui/ 浏览器直接访问能进入 OpenList Web SPA
- [x] ENCV 加密视频在 :16666/openlist-ui/ 内能预览（需 .sccgv 测试文件，可选）

## 文档（Task 6 + Task 13）

- [x] app/preview-gateway/README.md 含架构图（4 upstream → :16666 网关 → 浏览器；外网 :16000 → :16666 网关）
- [x] README 含启动步骤
- [x] README 含故障排查指南（502 / ws 失败 / CORS 异常 / 端口冲突 / preview-proxy 自动注册失败）
- [x] README 含端口决策说明（:16666 vs :16000 vs :5173 vs :8100）
- [x] README 与 spec.md D1-D10 决策一致

---

## 沙箱 dev 子资源路由（D11）— Task 7 + Task 8

- [x] `app/encv-mobile/plugin-openlist/web/index.html` 含 `<!--VITE-BASE-HREF-PLACEHOLDER-->` 占位符（`<head>` 第一子元素）
- [x] vite.config.ts 含 `injectBaseHref` plugin（`order: 'post'`），同时替换 placeholder 和改 `@vite/client` src
- [x] vite.config.ts `server.fs.allow` 包含 `/workspace`、`/workspace/app/encv-mobile`、`/workspace/app/encv-mobile/node_modules`
- [x] `scripts/dev-openlist-web.sh` 含 `export VITE_BASE="/openlist-ui/"`
- [x] `ecosystem.config.cjs` `plugin-openlist-vite` env 含 `VITE_BASE: '/openlist-ui/'`
- [x] preview-gateway `Upstream` interface 含 `pathRewrite?: (path: string) => string` 字段
- [x] `/openlist-ui` upstream 含 `pathRewrite: (p) => p.replace(/^\/openlist-ui(?=\/|$)/, '') || '/'`
- [x] `pickUpstream` 加 `cookie` 参数（3-arg signature）
- [x] Cookie 路由命中：`__plugin_spa=1` → :5174 plugin-openlist-web

## Cookie 路由（D12）— Task 8

- [x] `proxy.on('proxyRes')` 对 `text/html` 响应注入 `Set-Cookie: __plugin_spa=1; Path=/; SameSite=Lax; Max-Age=3600`
- [x] 入口 HTML 响应带 Set-Cookie 头（验证：`curl -sI http://localhost:16666/openlist-ui/ | grep -i set-cookie`）
- [x] 后续子资源请求带 cookie 命中 → :5174（验证：`curl -sI -H "Cookie: __plugin_spa=1" http://localhost:16666/src/views/OpenListHome.vue` → `text/javascript`）
- [x] 不带 cookie 同请求 → fallthrough（验证：`curl -sI http://localhost:16666/src/views/OpenListHome.vue` → `text/html`）
- [x] injectBaseHref 工作：HTML 中含 `/openlist-ui/@vite/client` 路径
- [x] pm2 重启后 cookie 路由仍工作（`pm2 restart preview-gateway --update-env`）

## 防御性 UI（D13）— Task 10 + Task 11

### 主 app
- [x] `app/encv-mobile/src/views/NotFoundView.vue` 存在（含 ionic UI + 路由列表 + debug 面板）
- [x] `app/encv-mobile/src/router/index.ts` 含 catch-all `{ path: ':pathMatch(.*)*', component: NotFoundView }`
- [x] `app/encv-mobile/src/App.vue` 含 `onErrorCaptured` + `rootError` ref + 错误屏 UI
- [x] import `bugOutline` 图标 + `.root-error` CSS 样式
- [x] 浏览器访问 `/tabs/typo` 显示 404（不空白）

### plugin-openlist-web
- [x] `app/encv-mobile/plugin-openlist/web/src/views/NotFoundView.vue` 存在
- [x] `app/encv-mobile/plugin-openlist/web/src/router/index.ts` 含 catch-all
- [x] `app/encv-mobile/plugin-openlist/web/src/App.vue` 含 `onErrorCaptured` + `rootError` ref
- [x] 浏览器访问 `/openlist-ui/#/typo` 显示 404（不空白）

## 端到端验证（Task 12）

- [x] `curl -sI http://localhost:16666/` → 200 text/html（vite :8100 fallthrough）
- [x] `curl -sI http://localhost:16666/openlist-ui/` → 200 text/html + Set-Cookie 注入
- [x] `curl -sI http://localhost:16666/openlist-ui/ | grep -i set-cookie` → 含 `__plugin_spa=1`
- [x] `curl -s http://localhost:16666/openlist-ui/ | grep -oE '/openlist-ui/@vite|src/main\.ts'` → 两条都匹配
- [x] `curl -sI -H "Cookie: __plugin_spa=1" http://localhost:16666/src/views/OpenListHome.vue` → text/javascript
- [x] `curl -sI http://localhost:16666/src/views/OpenListHome.vue` → text/html (无 cookie)
- [x] `curl -s http://localhost:16666/__gateway/health | jq .upstreams` → 3 upstream 全部 alive
- [x] `pm2 list` → preview-gateway / plugin-openlist-vite / start-preview / openlist 全部 online
- [x] `ss -tlnp | grep -E "(:16666|:8100|:5174|:2025|:5244)"` → 5 端口全部监听
- [x] `/tabs/zzzz` 浏览器实测：显示主 app NotFoundView（开发者友好 404 + 路由列表）
- [x] `/openlist-ui/#/not-a-real-path` 浏览器实测：显示 plugin SPA NotFoundView

## 沙箱 dev HMR 修复（D14）— WS 升级连通

- [x] `app/encv-mobile/plugin-openlist/web/vite.config.ts` 含 `server.allowedHosts: true`
- [x] `app/encv-mobile/vite.config.ts` 含 `server.allowedHosts: true`
- [x] `app/encv-mobile/plugin-openlist/web/vite.config.ts` 含 `dynamicHmrHostPlugin`（`enforce: 'pre'`）
- [x] `app/encv-mobile/vite.config.ts` 含 `dynamicHmrHostPlugin`（`enforce: 'pre'`）
- [x] `dynamicHmrHostPlugin` 替换 `__HMR_HOSTNAME__` 为 auto-detected 外部 host
- [x] `dynamicHmrHostPlugin` 替换 `__HMR_PORT__` 为 `16666`
- [x] `dynamicHmrHostPlugin` 替换 `__HMR_PROTOCOL__` 为 `ws`/`wss`（从 Referer 推断）
- [x] `dynamicHmrHostPlugin` **不替换** `__WS_TOKEN__`（vite 生成，gateway 透传）
- [x] 验证：外部 Host 访问 `:5174/@vite/client` 返回 200（不再 403）
- [x] 验证：HMR config `socketHost` 包含外部域名（不再 `localhost`）
- [x] 验证：WS 升级（带 `Sec-WebSocket-Protocol: vite-hmr`）直接 :5174 返回 101
- [x] 验证：WS 升级（带 cookie）经 gateway :16666 → :5174 返回 101
- [x] 验证：WS 升级（外部 Host + cookie）经 gateway → :5174 返回 101
- [x] 验证：WS 升级无 cookie fallthrough 到 :8100 返回 101
- [x] 浏览器 console 不再报 `[vite] failed to connect to websocket`
