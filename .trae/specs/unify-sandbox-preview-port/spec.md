# Spec: 沙箱统一预览端口（Unified Sandbox Preview Gateway）

> **核心目标**：构建 preview-gateway Node 项目监听 :16666，对外链 `外网 → :16000 (OpenPreview) → :16666 (preview-gateway) → 4 个 upstream`，统一暴露主 app / openlist plugin / openlist 后端 / encv-go。Vite 退化为纯净的 SPA dev server，监听 :8100（vite.config.ts 已声明）作为 preview-gateway 的一个上游。
>
> **关键反转**：用户决策 D1——`16000 → agent-tool-host → :16666 (gateway) 好记`。preview-gateway 用独立 :16666 端口（不是 :5173、不是 :16000），让 agent-tool-host 现有链路 :16000 → 默认 upstream 在 agent-browser 首次 navigate :16666 时被自动改写。

---

## 一、动机与现状

### 1.1 沙箱当前预览形态（4 端口 + 1 代理）

```
浏览器（沙箱内 Chrome agent-browser 或 外网用户）
  ↓ 走 :18080 强制代理（沙箱内） / 走 OpenPreview（外网）
预览入口 :16000 (agent-tool-host / OpenPreview)
  ↓ 反向代理 :16000 → :5173
:5173 vite (encv-mobile dev server)               ← 主 app
  + 内部 plugin: /openlist-ui/ → :5174
  + 内部 api:    /api/, /openlist/, /p/, /play/ → :2025 (encv-go) → :5244 (OpenList Go)
```

### 1.2 当前痛点

| # | 痛点 | 现象 | 已踩过的坑 |
|---|------|------|-----------|
| **P1** | **外网只能访问主 app** | `:16000 → :5173 (vite)` 只服务主 app；/openlist-ui/、/api/、/openlist/ 这些路径 vite 内部才有 plugin/代理，但 vite 的内部代理对外不直接可见 | 用户原话："预览端口需要沙箱代理，我无法访问远程ip" |
| **P2** | **沙箱 :18080 内部代理 bug** | Chrome agent-browser 走 :18080，body 截断 310 字节 | curl 经 :16000 拿 14645 字节，Chrome 经 :16000 拿 14335 字节 |
| **P3** | **Origin 头被改写** | vite 看到 `Origin: http://localhost:5173`（不是 :16000），回显 `Access-Control-Allow-Origin: http://localhost:5173`，ESM `import()` 跨域失败 | 用户看到"openlist 扩展 ui 空白" |
| **P4** | **vite HMR 跨端口失效** | 主 app 改代码，HMR 通过 :5173 ws 推送，但浏览器当前 origin 是 :16000，HMR 客户端拒绝连 | Vite 默认 ws 端口 = 同 server port，跨端口失效 |
| **P5** | **iframe 同源策略** | 原本 OpenListView 用 iframe 加载 :5174，:16000 同源但跨子域跨端口 → iframe 父子通信需要 postMessage | 当前 OpenListView 已用 Capacitor 嵌入绕开此问题，但 WebView 路径仍需 iframe |
| **P6** | **vite 内部胶水多** | vite.config.ts 累加 `cors: '*'` + `openlistUiProxy` plugin + `server.proxy` 块 → 单点反向代理职责过重 | 用户原话："你的修复太胶水了，需要从根本上解决" |

### 1.3 目标形态（preview-gateway 监听 :16666）

```
外网用户 (外网)               本地 dev / agent-browser
  ↓ http://<sandbox>/:16000/   ↓ http://localhost:16666/
agent-tool-host (OpenPreview)
  ↓ :16000 → :16666 (首次 agent-browser navigate :16666 后自动改写)
预览网关 :16666 (新项目, Node + http-proxy)  ← 用户决策 D1
  ├── /                  → encv-mobile SPA (:8100)            ← vite 监听 :8100
  ├── /openlist-ui/      → plugin-openlist/web (:5174)        ← SPA
  ├── /openlist/         → encv-go (:2025) → OpenList (:5244) ← reverse proxy
  ├── /api/              → encv-go (:2025)
  ├── /p/                → encv-go (:2025)
  ├── /play              → encv-go (:2025)
  ├── /src/* /@vite/*    → vite 资源直转（保持 HMR 端口）
  └── /ws                → vite HMR WebSocket 转发（关键！解决 P4）
```

**核心收益**：
- **外网单端口 :16000 直通全部上游**（首次 agent-browser navigate 触发后）：`外网 → :16000 → agent-tool-host → :16666 (gateway) → :8100/:5174/:2025`
- **本地 dev / agent-browser 直接用 :16666**（不需要 :16000 转发）：`http://localhost:16666/...`
- **网关是唯一路由权威**：vite :8100 是纯 dev server，零胶水
- **Origin 头不变量**：所有 :16666 请求都带 `Origin: http://localhost:16666`，vite 看到的 Origin 永远是 :16666 → CORS 一致
- **HMR 走转发**：WebSocket 升级握手走网关 → vite `:8100/ws` 正常 HMR
- **零胶水 vite**：vite :8100 是纯净的 vite dev server，不挂任何 reverse-proxy plugin

---

## 二、架构

### 2.1 三层 + 1 网关

```
Layer 0: 沙箱预览网关 (新, :5173, Node http-proxy)
  └── 接管原 vite 的 :5173 端口；agent-tool-host :16000 → :5173 链路零改动
  └── 纯转发；零业务逻辑

Layer 1: encv-mobile (:8100) + plugin-openlist/web (:5174) — Vite dev
  └── 沙箱内部，仍由 Vite 各自起；网关转发浏览器请求
  └── **vite :8100 不再承担任何反向代理职责**（用户决策 D9，vite.config.ts 已声明 :8100）

Layer 2: encv-go (:2025) + OpenList (:5244) — Go server
  └── 沙箱内部；纯后端 API

Layer 3: 浏览器
  ├── 外网用户 → http://<sandbox>/:16000/... → agent-tool-host → :16666 (preview-gateway) → 全部上游
  └── 本地 dev / agent-browser → http://localhost:16666/... → preview-gateway → 全部上游
```

### 2.2 关键决策

| # | 决策 | 取值 | 理由 |
|---|------|------|------|
| **D1** | 网关占用哪个端口？ | **`:16666`**（用户决策："好记"） | 避开 :16000 (agent-tool-host 占用) 和 :5173 (vite 老端口) 的语义冲突；:16666 独立、易记 |
| **D2** | 网关用什么实现？ | **Node `http-proxy`** | 成熟；`ws: true` 模式自动处理 WebSocket upgrade（HMR 关键） |
| **D3** | 网关放哪个仓库？ | **`app/preview-gateway/` 独立项目** | 单一职责；可独立 pm2 进程；可独立测试 |
| **D4** | 网关要鉴权吗？ | **不要** | 沙箱内本地 dev + agent-tool-host 内部转发用，无外部鉴权需求 |
| **D5** | CORS / Origin 处理 | **网关层不做任何改写**（`changeOrigin: false`） | 让上游 vite / Go 自行处理；网关是 transparent proxy |
| **D6** | HMR WebSocket 怎么转？ | **`http-proxy` 的 `ws: true` 自动 upgrade** | vite dev 默认 ws path = `/?token=...`；`http-proxy` 自动处理 Upgrade 头 |
| **D7** | 出错兜底？ | **502 + JSON 错误** | 任意 upstream 不可达 → 网关返回 `{ error: "upstream_unavailable", upstream: "..." }` 便于 DevLogs 诊断 |
| **D8** | 健康检查端点？ | **`/__gateway/health`** | 返回所有 upstream 的 ping 结果；pm2 readiness 用 |
| **D9** | vite 承担什么角色？ | **纯净 SPA dev server，监听 :8100**（用户决策） | 之前的所有 vite 胶水（workspacePackageRewrite / alias / fs.allow / proxy 块 / cors '*'）已撤销；vite 是纯 dev server |
| **D10** | agent-tool-host 的默认 upstream 怎么改？ | **需要 IDE OpenPreview 工具显式注册（无自动机制）** | 实测发现 preview-proxy 内部 :80 register 端点 requires_auth=true，普通 agent-browser navigate 不会触发。Register 仅在 `OpenPreview(command_id=..., preview_url=...)` 调用时由 PreviewManager 执行。终态须用 OpenPreview(preview_url="http://localhost:16666/") 显式激活 |
| **D11** | 沙箱 dev 下 plugin SPA 子资源（`/src/...`、`/@fs/...`、`/node_modules/...`）如何路由回 :5174？ | **VITE_BASE + Vite injectBaseHref plugin + fs.allow + preview-gateway pathRewrite + Set-Cookie 注入** 五层修复 | Vite 8 (rolldown) dev 模式不基于 `<base href>` 改写 import 路径，所有 import 解析为绝对根（如 `/node_modules/.vite/deps/vue.js?v=hash`）。裸 base 失效，必须用 (1) `transformIndexHtml order:'post'` 替换 placeholder → `<base>` + 改 `@vite/client` 的 src；(2) `server.fs.allow` 扩展到 `/workspace` 解决 `/@fs/...` 返回 SPA fallback 的 text/html；(3) preview-gateway pathRewrite 剥 `/openlist-ui` 前缀；(4) Set-Cookie `__plugin_spa=1` 标记后续子资源请求 |
| **D12** | 沙箱 dev 怎么识别"用户从 `/openlist-ui/` 进入的子资源请求"？ | **Cookie 路由（Referer 不可靠）** | Trae IDE 沙箱 Chrome 默认 `referrer-policy = strict-origin-when-cross-origin`，network requests 中 `document.referrer` 为空。改用 Set-Cookie 注入（HTML 响应时） + 后续请求带 cookie 头判定。Cookie 路径 `Path=/` 覆盖所有子路由，Max-Age=3600 限制作用域，SameSite=Lax 兼顾安全 |
| **D13** | 防御性 UI：路由不匹配 / 组件渲染崩溃时怎么显示？ | **catch-all 404 + onErrorCaptured 错误边界 + 根级 rootError fallback 三层防护** | 任意 vue-router 未匹配路由 → `/:pathMatch(.*)*` → `NotFoundView`（列出全部已知路由 + 诊断信息 + 返回/重载按钮）；任意子组件 render 异常 → `onErrorCaptured` 捕获 → 父级降级渲染 + 显示堆栈；根级异常（mount 失败）→ `rootError` 响应式状态 → 整个 app 显示错误屏 + reload 按钮 |
| **D14** | 沙箱 dev 浏览器 console 报 `[vite] failed to connect to websocket` 怎么修？ | **vite 5+ allowedHosts 拒绝 + HMR host 锁定 localhost + @vite/client 占位符替换 三个根因 + 五件套修复** | (a) vite 5+ 默认 `server.allowedHosts` 锁 localhost，外部 trae.cn 域名直接 403 拒绝 → 设 `allowedHosts: true`；(b) vite `server.hmr.host` 默认从 `server.host` 派生（0.0.0.0 → 退回 localhost），浏览器在远程沙箱无法连接 → 用 `enforce:'pre'` + `transform` 钩子拦截 `@vite/client`，替换 `__HMR_HOSTNAME__` / `__HMR_PORT__` / `__HMR_PROTOCOL__` / `__HMR_BASE__` 四个占位符为 auto-detected 外部 host + 网关端口 16666；(c) vite 8 WS upgrade 监听器要求 `Sec-WebSocket-Protocol: vite-hmr` 才响应（`@vite/client` 实际会发，但 curl 测试要带）；(d) preview-gateway 已有的 cookie 路由让 WS upgrade 在根路径也能正确路由到 :5174；(e) `__WS_TOKEN__` 不要替换（vite 生成，gateway 透传） |

### 2.3 与现有 OpenPreview 关系

- `:16000` 是 `agent-tool-host` (OpenPreview) 实际监听的 TCP 端口
- 它的默认 upstream 是 `:5173`（旧 vite），现在 vite 已搬到 :8100
- preview-gateway 监听 `:16666`，独立于 agent-tool-host
- **首次 agent-browser navigate `http://localhost:16666/`** 时，preview-proxy 自动注册 :16666 为新默认 upstream（D10）
- 之后外网用户 → :16000 → agent-tool-host → :16666 (gateway) → 4 个 upstream

**等价关系（终态）**：
```
外网用户               → :16000
                          │
                          └─ OpenPreview (agent-tool-host, 已存在, 不动)
                                └─ :16666 preview-gateway (本 spec 引入)
                                      ├─ :8100 encv-mobile vite (vite.config.ts 已声明)
                                      ├─ :5174 plugin-openlist/web
                                      └─ :2025 encv-go → :5244 OpenList

本地 / agent-browser  → :16666 (直接命中 preview-gateway, 同上)
```

---

## 三、关键技术细节

### 3.1 网关 HTTP 路由表

```typescript
// app/preview-gateway/src/server.ts
import http from 'node:http'
import httpProxy from 'http-proxy'

const routes: Array<{ match: string, target: string, name: string }> = [
  { match: '/openlist-ui',  target: 'http://127.0.0.1:5174', name: 'plugin-openlist-web' },
  { match: '/openlist/',    target: 'http://127.0.0.1:2025', name: 'encv-go' },
  { match: '/api',          target: 'http://127.0.0.1:2025', name: 'encv-go' },
  { match: '/p',            target: 'http://127.0.0.1:2025', name: 'encv-go' },
  { match: '/play',         target: 'http://127.0.0.1:2025', name: 'encv-go' },
  // 默认 fallthrough → encv-mobile vite :8100
]

const proxies = new Map<string, httpProxy>()
for (const r of routes) {
  proxies.set(r.name, httpProxy.createProxyServer({ target: r.target, changeOrigin: false }))
}
const mainApp = httpProxy.createProxyServer({ target: 'http://127.0.0.1:8100', ws: true, changeOrigin: false })

const server = http.createServer((req, res) => {
  const route = routes.find(r => req.url?.startsWith(r.match))
  if (route) {
    proxies.get(route.name)!.web(req, res)
  } else {
    mainApp.web(req, res)  // fallthrough → encv-mobile vite :8100
  }
})

// WebSocket：识别 Upgrade 头，转发到 :8100 (主 app HMR) / :5174 (plugin HMR)
server.on('upgrade', (req, socket, head) => {
  if (req.url?.startsWith('/openlist-ui/')) {
    proxies.get('plugin-openlist-web')!.ws(req, socket, head)
  } else {
    mainApp.ws(req, socket, head)
  }
})

server.listen(Number(process.env.PORT ?? 16666), '0.0.0.0', () => {
  console.log(`[preview-gateway] listening on :16666 (D1: "好记")`)
})
```

### 3.2 沙箱 :18080 代理兼容性

`agent-browser` Chrome 强制走 `http://127.0.0.1:18080` 沙箱代理，触发 body 截断 310 字节 bug。
- **本 spec 不能修复** :18080 bug
- **本 spec 缓解**：
  - 外网用户走 :16000 → agent-tool-host → :5173 (gateway) → 上游；与 :18080 无关
  - 本地 agent-browser 调试时直接用 `http://localhost:5173`（沙箱内直连，绕过 :18080）
- **body 完整传递** 由 Node http-proxy 保证（实测经 Node 转发的响应 Content-Length 完整）

### 3.3 CORS / Origin 一致性

- 网关**不**改 `Origin` / `Host` 头（`changeOrigin: false`）
- 浏览器请求 :16666 → 网关转发 → 上游看到 `Origin: http://localhost:16666`
- vite 5+ 默认 `cors: true` 回显 Origin → 浏览器收到 `Access-Control-Allow-Origin: http://localhost:16666` → 匹配 → CORS 通过
- **撤销**之前在 vite.config.ts 硬编码的 `cors: { origin: '*' }`（dev 配置可以是回显 Origin）
- **不**在 vite.config.ts 中挂任何 reverse-proxy plugin（D9 决策）

### 3.4 HMR WebSocket 转发

- vite dev 默认 ws path: `/?token=...`（无独立路径）
- 浏览器看到 ws URL: `ws://localhost:16666/?token=...`（基于当前 origin :16666）
- 网关在 `upgrade` 事件中识别 `Upgrade: websocket` + `Connection: Upgrade` 头
- 转发到 `ws://127.0.0.1:8100/?token=...`（保持 query string 不变）
- vite HMR 客户端 ws 握手成功 → 正常推送 hot module updates

### 3.5 错误兜底与可观测

每个 upstream 不可达时：

```json
HTTP/1.1 502 Bad Gateway
Content-Type: application/json

{
  "error": "upstream_unavailable",
  "upstream": "plugin-openlist-web",
  "target": "http://127.0.0.1:5174",
  "path": "/openlist-ui/@login",
  "hint": "Check pm2 status for plugin-openlist-vite"
}
```

`/__gateway/health`：

```json
{
  "ok": true,
  "upstreams": {
    "encv-mobile":        { "url": "http://127.0.0.1:5173", "alive": true, "latency_ms": 12 },
    "plugin-openlist-web":{ "url": "http://127.0.0.1:5174", "alive": true, "latency_ms": 8  },
    "encv-go":            { "url": "http://127.0.0.1:2025", "alive": true, "latency_ms": 5  },
    "openlist":           { "url": "http://127.0.0.1:5244", "alive": true, "latency_ms": 23 }
  }
}
```

---

## 四、改动清单

| # | 改动 | 文件 | 性质 | 风险 |
|---|------|------|------|------|
| **G1** | 新建 `app/preview-gateway/` 项目（package.json + tsconfig + src/server.ts） | 新 | TS | 低 |
| **G2** | pm2 注册 `preview-gateway` 进程（端口 :16666） | `ecosystem.config.cjs` (改) | JS | 低 |
| **G3** | setup-sandbox-env.sh 加 preview-gateway 启动步骤 | `scripts/setup-sandbox-env.sh` (改) | shell | 低 |
| **G4** | vite.config.ts 撤销 `cors: { origin: '*' }` 硬编码 + 撤销 proxy 块 + 撤销 openlistUiProxy plugin | `app/encv-mobile/vite.config.ts` (改) | TS | 中（需端到端验证） |
| **G5** | **start-preview.sh 显式指定 vite 监听 :8100**（vite.config.ts 已声明 :8100） | `app/encv-mobile/scripts/start-preview.sh` (改) | bash | 中（启动顺序耦合） |
| **G6** | 首次 agent-browser navigate :16666 触发 preview-proxy 自动注册 | 无代码 | — | — |
| **G7** | 文档：沙箱预览拓扑图更新 | `app/preview-gateway/README.md` (新) | md | 低 |

### 4.1 撤销列表（修复后清理）

| 之前"胶水" | 撤销原因 | 撤销方式 |
|------------|---------|---------|
| `vite.config.ts` 的 `cors: { origin: '*' }` | 网关层不做 Origin 改写，vite 回显 Origin 即可 | 改回默认 `cors: true` 或不显式设置 |
| `vite.config.ts` 的 `server.proxy` 块 | D9 决策：vite 不再承担反向代理 | 删除 `proxy: { '/api': 2025, '/openlist/': 2025, '/p': 2025, '/play': 2025 }` |
| `vite.config.ts` 的 `openlistUiProxy` plugin | D9 决策：vite 不再处理子路径 | 移除 plugin 引用 |
| `vite.config.ts` 的 `server.host: '0.0.0.0'` | 网关层转发即可 | 保留（保险；vite 仍可监听 0.0.0.0） |

**已撤销（之前的胶水重构已完成）**：
- ✅ 删除 `/workspace/app/encv-mobile/packages/`（假 monorepo 共享）
- ✅ 创建 `/workspace/app/encv-mobile/src/components-shared/`（主 app 复本）
- ✅ plugin-openlist/web 创建本地 `components-shared/`
- ✅ `pnpm-workspace.yaml` 删 `packages/*`
- ✅ 两个 `package.json` 删 `@encvgo/components` 依赖
- ✅ `vite.config.ts` 撤销 `workspacePackageRewrite` plugin / `resolve.alias` / `fs.allow`

---

## 五、执行顺序

```
P0 — 新建 preview-gateway 项目骨架 (G1)               ✅
P1 — 写 HTTP 路由 + WebSocket 转发                    ⏳ 需改端口从 :16001 → :5173
P2 — 写 /__gateway/health 端点                       ⏳ P1 后
P3 — pm2 注册 + setup-sandbox-env.sh 集成 (G2, G3)   ⏳ P2 后
P4 — vite 端口从 :5173 改到 :5175 (G5)               ⏳ P3 后
P5 — vite.config.ts 撤销 cors '*' + proxy 块 (G4)     ⏳ P4 后
P6 — 端到端验证 (外网 :16000 直通 4 个上游)          ⏳ P5 后
P7 — 文档 (G7)                                       ⏳ P6 后
```

---

## 六、风险登记

| # | 风险 | 缓解 |
|---|------|------|
| **R1** | 网关单点故障 | pm2 配 `autorestart: true`，挂掉自动拉起 |
| **R2** | WebSocket 转发性能 | `http-proxy` 的 `ws: true` 自动处理 upgrade；4 个 upstream 并发量极小（沙箱内单用户） |
| **R3** | 网关与 vite proxy 重复 | **D9 决策**：vite 不再有 proxy 块；只有 preview-gateway 是唯一代理 |
| **R4** | `:18080` 沙箱代理仍截断 body | 本地 agent-browser 改用 :5173（不走 :18080）；或经 Node http-proxy 中转，body 完整 |
| **R5** | 撤销 `cors: '*'` + proxy 块后其它 dev 场景破坏 | 仅 sandbox 预览路径用网关；其它本地 dev 仍 `pnpm dev` 直接 :5175（vite 默认 cors: true 已足够） |
| **R6** | vite 端口从 :5173 改到 :5175 影响其它工具链 | start-preview.sh 启动命令改 `--port 5175 --strictPort`；air 配置 / linter / IDE 调试端口不需要改 |
| **R7** | 外网链 :16000 → :5173 切到 gateway 后 vite 不可达 | 是 spec 接受的取舍：vite 仍然在 :5175 跑（pm2 拉起），仅 :5173 不再是 vite；外网链 :16000 → :5173 (gateway) → :5175 (vite) 完整工作 |
| **R8** | D9 决策导致 vite 老端口 :5173 路径下 /openlist-ui/ 不可达 | 是 spec 接受的取舍：所有 :5173 访问都经 preview-gateway，不再直连 vite |

---

## 七、Spec 自我一致性检查

- [x] 改动都有具体文件路径
- [x] 每个改动都有可执行命令（`pnpm add http-proxy ws`、`pm2 start ecosystem.config.cjs`）
- [x] 决策 D1-D8 都有理由
- [x] 风险 R1-R6 都有缓解
- [x] 不破坏现有 OpenList 嵌入逻辑
- [x] 不依赖 Trae IDE 内部代理（:18080）修复

---

## 八、Spec 完成判据

| # | 判据 | 验证方式 |
|---|------|----------|
| **J1** | `pnpm dev` 起 preview-gateway，监听 :16666 | `curl -sI http://localhost:16666/ \| head -3`（拿到 vite HTML 即正确） |
| **J2** | 本地 :16666 走 preview-gateway → 主 app | `curl -s http://localhost:16666/ \| grep -c '<div id="app">'` |
| **J3** | :16666/openlist-ui/ 走网关 → :5174 plugin SPA | `curl -s -o /dev/null -w "%{http_code}" http://localhost:16666/openlist-ui/` |
| **J4** | :16666/api/* 走网关 → :2025 encv-go | `curl -s -o /dev/null -w "%{http_code}" http://localhost:16666/api/public/settings` |
| **J5** | :16666/openlist/sites/* 走网关 → :2025 → :5244 | `curl -s -o /dev/null -w "%{http_code}" http://localhost:16666/openlist/sites/local/api/public/settings` |
| **J6** | WebSocket 升级转发到 vite HMR (:8100) | `curl -i -N -H "Connection: Upgrade" -H "Upgrade: websocket" http://localhost:16666/?token=test` 期望 101 |
| **J7** | /__gateway/health 返回 4 个 upstream 状态 | `curl -s http://localhost:16666/__gateway/health \| jq .upstreams` |
| **J8** | 浏览器实测 :16666/tabs/openlist 正常渲染 | agent-browser open + snapshot |
| **J9** | 撤销 vite `cors: '*'` 后 :16666 仍 CORS 通过 | snapshot 显示 OpenListView 组件 |
| **J10** | pm2 拉起 preview-gateway 后自愈 | `pm2 kill && pm2 start ecosystem.config.cjs` |
| **J11** | vite.config.ts 不含 `proxy: { ... }` 块（D9 验证） | `grep -n "proxy:" app/encv-mobile/vite.config.ts` 应为空 |
| **J12** | vite.config.ts 不含 `openlistUiProxy` plugin 引用 | `grep -n "openlistUiProxy" app/encv-mobile/vite.config.ts` 应为空 |
| **J13** | vite 实际监听 :8100（D9 验证） | `curl -sI http://localhost:8100/ \| head -3`（拿到 vite HTML 即正确） |
| **J14** | OpenPreview 工具激活 :16666 后 :16000 默认 upstream 切换 | `OpenPreview(preview_url="http://localhost:16666/")` 显式注册；日志 `PreviewManager` Port registered: 16666 |

---

## 十、防御性 UI 设计（D13）

> **核心原则**：白屏 = 失败。**任何**路径不匹配 / 组件渲染异常 / 资源加载失败都必须显示**可诊断的 UI**，而不是空白页面。
>
> 动机：沙箱 dev 路径下 plugin-openlist SPA 经常因为：(1) vite 注入 `/@vite/client` 路径错；(2) 沙箱 Chrome 缺 referer 头导致资源 404；(3) 用户输入未知 hash 路由；(4) 子组件崩溃等场景空白。修复后**仍需**有兜底 UI 让开发者一眼看出"哪里错了"。

### 10.1 三层防护架构

```
┌────────────────────────────────────────────────────────┐
│ Layer 1: vue-router catch-all（路由级）                │
│   任何未匹配路径 → :pathMatch(.*)* → NotFoundView       │
│   - 列出所有已知路由（可点击跳转）                       │
│   - 显示 location.pathname / router base / currentRoute │
│   - 「返回 /home」「重新加载」按钮                       │
│   - <details> 折叠 debug 面板（黑底等宽字体）           │
└────────────────────────────────────────────────────────┘
            │ 路由匹配但组件崩溃？
            ▼
┌────────────────────────────────────────────────────────┐
│ Layer 2: onErrorCaptured（组件级）                      │
│   父组件捕获子组件 render 异常 → 显示局部错误 UI         │
│   - 错误标题 + 图标                                      │
│   - 错误堆栈（折叠）                                    │
│   - 「重新加载」「返回」按钮                              │
│   - 阻止异常冒泡到根（return false）                     │
└────────────────────────────────────────────────────────┘
            │ 根组件 mount 失败？
            ▼
┌────────────────────────────────────────────────────────┐
│ Layer 3: rootError fallback（应用级）                  │
│   App.vue 顶层 catch → 整 app 替换为错误屏               │
│   - 大图标 + 「应用启动失败」                            │
│   - 错误堆栈（可复制）                                  │
│   - 「重新加载」按钮                                    │
└────────────────────────────────────────────────────────┘
```

### 10.2 NotFoundView 设计（主 app + plugin-openlist-web 各一份）

**主 app 版本**（[NotFoundView.vue](file:///workspace/app/encv-mobile/src/views/NotFoundView.vue)）：

```vue
<template>
  <ion-page>
    <ion-header>
      <ion-toolbar color="warning">
        <ion-title>404 — 找不到这个页面</ion-title>
      </ion-toolbar>
    </ion-header>
    <ion-content class="ion-padding">
      <ion-card>
        <ion-card-header>
          <ion-icon :icon="alertCircleOutline" size="large" color="warning" />
          <ion-card-title>路由未匹配</ion-card-title>
          <ion-card-subtitle>path: <code>{{ $route.path }}</code></ion-card-subtitle>
        </ion-card-header>
        <ion-card-content>
          <p>可能是 URL 输错、路由未注册、或上游资源加载失败。</p>
          <ion-button @click="goHome">返回首页</ion-button>
          <ion-button @click="reload">重新加载</ion-button>
          <details>
            <summary>调试信息</summary>
            <pre>{{ debugInfo }}</pre>
          </details>
        </ion-card-content>
      </ion-card>
      <ion-list>
        <ion-list-header>已知路由</ion-list-header>
        <ion-item v-for="r in knownRoutes" :key="r.path" button @click="goTo(r.path)">
          <ion-label>{{ r.path }}</ion-label>
        </ion-item>
      </ion-list>
    </ion-content>
  </ion-page>
</template>
```

**plugin-openlist-web 版本**（[NotFoundView.vue](file:///workspace/app/encv-mobile/plugin-openlist/web/src/views/NotFoundView.vue)）—— 结构同主 app，但：
- 路由列表来自 plugin-openlist-web 自己的 `routes`
- 按钮：`返回 /home`（plugin SPA 内部 hash 路由）
- 不使用 ion-card（plugin SPA 是纯 Vue，无 Ionic 依赖）

### 10.3 vue-router 路由表 catch-all 注册

**主 app**（[router/index.ts](file:///workspace/app/encv-mobile/src/router/index.ts)）：

```ts
export const routes: RouteRecordRaw[] = [
  // ... 已有路由
  {
    path: ':pathMatch(.*)*',
    name: 'not-found',
    component: NotFoundView,
  },
]
```

**plugin-openlist-web**（[router/index.ts](file:///workspace/app/encv-mobile/plugin-openlist/web/src/router/index.ts)）：

```ts
export const router = createRouter({
  history: createWebHashHistory('/openlist-ui/'),  // ← base 参数关键
  routes: [
    { path: '/', redirect: '/home' },
    // ... 已有路由
    { path: '/:pathMatch(.*)*', name: 'not-found', component: NotFoundView },
  ],
})
```

### 10.4 App.vue 错误边界

**主 app** + **plugin-openlist-web** 的 `App.vue` 都加：

```vue
<script setup lang="ts">
import { onErrorCaptured, ref } from 'vue'
import { bugOutline } from 'ionicons/icons'

const rootError = ref<Error | null>(null)

onErrorCaptured((err) => {
  console.error('[App] captured render error:', err)
  rootError.value = err instanceof Error ? err : new Error(String(err))
  // 不冒泡到 Vue 顶层
  return false
})
</script>

<template>
  <!-- 根级异常：整 app 替换为错误屏 -->
  <div v-if="rootError" class="root-error">
    <ion-icon :icon="bugOutline" size="large" color="danger" />
    <h1>应用启动失败</h1>
    <pre>{{ rootError.stack ?? rootError.message }}</pre>
    <ion-button @click="location.reload()">重新加载</ion-button>
  </div>

  <!-- 正常渲染 -->
  <ion-app v-else>
    <ion-router-outlet />
  </ion-app>
</template>
```

### 10.5 为什么"白屏 = 失败"

| 场景 | 不防御的后果 | 防御后 |
|------|------------|-------|
| 用户手输 `/tabs/typo` | 整个 SPA 空白 | 显示 404 + 路由列表 + 返回按钮 |
| 沙箱 vite HMR 断开 | dev mode 整个 SPA 卡死 | 重新加载按钮可恢复 |
| `OpenListView` 组件 import 失败 | /tabs/openlist 空白 | rootError 屏 + 堆栈 + 重新加载 |
| 用户手输 `/openlist-ui/#/zzz` | 空白 | 显示 404 + 已知路由 |
| 上游 :5174 vite 挂掉 | 入口 HTML 502 → 浏览器空白 | preview-gateway 已返 502 JSON（仍可诊断） |

### 10.6 禁止的反模式

| 反模式 | 后果 |
|--------|------|
| ❌ inline `<ion-modal :is-open="showModal">` 用于跨 tab 操作 | tab 非活跃时 modal 不渲染（capacitor.md §1.1） |
| ❌ 把 catch-all 路由放在 routes 列表第一项 | 永远匹配不到其他路由 |
| ❌ `onErrorCaptured` 不 `return false` | 异常继续冒泡到根，被 Vue 默认 handler 吞掉 |
| ❌ `rootError` 状态用 `let` 不用 `ref` | Vue 不会响应式更新 |
| ❌ NotFoundView 不导出 debug 信息 | 出了问题无法诊断 |

---

## 十一、沙箱 dev 资源路由：Cookie 标记机制（D11 + D12）

> **核心问题**：Vite 8 (rolldown) dev 模式不基于 `<base href>` 改写 import 路径，所有 import 解析为**绝对根路径**（如 `/node_modules/.vite/deps/vue.js?v=hash`）。浏览器访问 `/openlist-ui/` 时：
>
> 1. 入口 HTML 中 `<script src="./src/main.ts">` → base href 把它解析为 `/openlist-ui/src/main.ts` → gateway 路由到 :5174 ✓
> 2. main.ts 内部 `import 'vue'` → vite dev 解析为 `/node_modules/.vite/deps/vue.js?v=hash`（绝对根，**不带 /openlist-ui 前缀**） → 浏览器请求 `http://localhost:16666/node_modules/...` → **fallthrough 到 :8100 (encv-mobile)** → 404 ✗

### 11.1 单一机制为什么不够

| 机制 | 单独作用 | 失败原因 |
|------|---------|---------|
| `<base href="/openlist-ui/">` | 让相对路径变 base-prefixed | Vite 8 dev 模式内部 import 不基于 base 改写 |
| `server.origin: 'http://...:16666/openlist-ui'` | 让 vite 输出完整 origin 路径 | Vite 8 仍输出绝对根路径（实测） |
| `enforce:'pre' + transform` 改 .ts/.vue 内路径 | 拦截 import 重写 | main.ts 入口文件不被 user transform 处理 |
| preview-gateway Referer 路由 | 用 referer 判定 | Trae IDE 沙箱 Chrome referer 为空 |

### 11.2 五层修复链

**Layer 1: VITE_BASE 环境变量**

```bash
# scripts/dev-openlist-web.sh
export VITE_BASE="/openlist-ui/"
```

**Layer 2: vite.config.ts — injectBaseHref plugin (order: 'post')**

```ts
function injectBaseHref(href: string): Plugin {
  return {
    name: 'inject-base-href',
    transformIndexHtml: {
      order: 'post',  // 必须在 Vite 注入 @vite/client 之后
      handler(html) {
        let result = html
        // 1) 占位符 → <base> 标签
        result = result.replace(
          '<!--VITE-BASE-HREF-PLACEHOLDER-->',
          `<base href="${href}" />`,
        )
        // 2) @vite/client 路径加前缀
        if (result.includes('<script type="module" src="/@vite/client">')) {
          result = result.replace(
            '<script type="module" src="/@vite/client">',
            `<script type="module" src="${href.replace(/\/$/, '')}/@vite/client">`,
          )
        }
        return result
      },
    },
  }
}
```

**Layer 3: vite.config.ts — fs.allow 扩展**

```ts
server: {
  fs: {
    allow: [
      path.resolve(__dirname),
      path.resolve(__dirname, '..', '..', '..'),  // encv-mobile root
      path.resolve('/workspace/app/encv-mobile'),
      path.resolve('/workspace/app/encv-mobile/node_modules'),
      path.resolve('/workspace'),
    ],
  },
}
```

**Layer 4: preview-gateway — pathRewrite 剥前缀**

```ts
const UPSTREAMS: Upstream[] = [
  {
    match: '/openlist-ui',
    target: 'http://127.0.0.1:5174',
    pathRewrite: (p) => p.replace(/^\/openlist-ui(?=\/|$)/, '') || '/',
  },
]
```

**Layer 5: preview-gateway — Set-Cookie 注入 + Cookie 路由**

```ts
// 1) 注入（HTML 响应时）
proxy.on('proxyRes', (proxyRes, _req) => {
  const ct = proxyRes.headers['content-type']
  if (ct && /text\/html/i.test(String(ct))) {
    const cookieLine = '__plugin_spa=1; Path=/; SameSite=Lax; Max-Age=3600'
    const existing = proxyRes.headers['set-cookie']
    proxyRes.headers['set-cookie'] = existing
      ? (Array.isArray(existing) ? [...existing, cookieLine] : [String(existing), cookieLine])
      : [cookieLine]
  }
})

// 2) 路由识别（pickUpstream 末尾的 fallback）
if (cookie && /(?:^|;\s*)__plugin_spa=1/.test(cookie)) {
  return UPSTREAMS.find((u) => u.match === '/openlist-ui') ?? DEFAULT_UPSTREAM
}
```

### 11.3 完整请求流（带 cookie 路由）

```
用户访问 http://localhost:16666/openlist-ui/
  ↓ gateway 收到
  pickUpstream: url="/openlist-ui/" 匹配 match="/openlist-ui" → :5174
  pathRewrite: "/openlist-ui/" → "/"
  ↓ proxy 到 :5174
  vite 返回 HTML（已含 <base href="/openlist-ui/"> + 改过的 @vite/client）
  ↓ proxyRes 钩子：content-type=text/html → Set-Cookie: __plugin_spa=1
  ↓ 浏览器收到 HTML + Set-Cookie: __plugin_spa=1
  浏览器执行 <script src="/openlist-ui/src/main.ts">  ← 相对路径，基于 base
  ↓ gateway 收到 /openlist-ui/src/main.ts + cookie
  pickUpstream: url 匹配 → :5174
  pathRewrite: "/openlist-ui/src/main.ts" → "/src/main.ts"
  ↓ proxy 到 :5174
  vite 返回 main.ts 内容（内部 import 是绝对根路径）
  浏览器执行 main.ts → 触发 import 'vue' → 解析为 /node_modules/.vite/deps/vue.js
  ↓ gateway 收到 /node_modules/.vite/deps/vue.js + cookie __plugin_spa=1
  pickUpstream: url 不匹配任何 prefix → 但 cookie 命中 → :5174
  ↓ proxy 到 :5174
  vite 返回正确的 vue.js ✓
```

### 11.4 验证命令

```bash
# 入口 HTML 应包含 /openlist-ui/@vite/client
curl -s http://localhost:16666/openlist-ui/ | grep -oE '/openlist-ui/@vite|src/main\.ts'
# 期望：/openlist-ui/@vite  src/main.ts

# Set-Cookie 注入
curl -sI http://localhost:16666/openlist-ui/ | grep -i set-cookie
# 期望：__plugin_spa=1; Path=/; SameSite=Lax; Max-Age=3600

# 子资源路由：带 cookie → text/javascript；不带 cookie → fallthrough (text/html)
curl -sI -H "Cookie: __plugin_spa=1" http://localhost:16666/src/views/OpenListHome.vue
# 期望：Content-Type: text/javascript
curl -sI http://localhost:16666/src/views/OpenListHome.vue
# 期望：Content-Type: text/html (encv-mobile SPA fallback)
```

---

## 十二、沙箱 dev HMR 修复（D14）

> **核心问题**：沙箱 dev 浏览器跑在外部 trae.cn 域名（如 `run-agent-...trae.cn:16666`），vite dev server 跑在沙箱内 `:5174` / `:8100`。浏览器无法直连 `localhost:5174`，HMR 一直报 `[vite] failed to connect to websocket`。

### 12.1 三个隐藏根因

**(1) vite 5+ `server.allowedHosts` 默认白名单**：

vite 5+ 引入安全特性：默认 `server.allowedHosts` 锁 `localhost / 127.0.0.1` 等白名单，外部 Host 头（如 `run-agent-...trae.cn`）直接返回 403 `Blocked request. This host is not allowed.`。

**复现**：
```bash
curl -s -o /dev/null -w "%{http_code}\n" \
  -H "Host: run-agent-test.trae.cn:16666" \
  http://127.0.0.1:5174/@vite/client
# 修复前：403
# 修复后：200
```

**修复**：
```ts
server: {
  allowedHosts: true,  // 允许所有 Host
}
```

**(2) vite `server.hmr.host` 默认锁定 localhost**：

vite `server.hmr.host` 默认从 `server.host` 派生。当 `server.host = '0.0.0.0'`（沙箱必须监听所有接口）时，HMR host 退回 `'localhost'`，但沙箱外浏览器连不上沙箱内 `localhost`。

**复现**：
```bash
curl -s http://127.0.0.1:5174/@vite/client | grep "socketHost ="
# 修复前：${"localhost" || importMetaUrl.hostname}:5174/
# 修复后：${"run-agent-test.trae.cn" || importMetaUrl.hostname}:16666/
```

**修复**（`dynamicHmrHostPlugin`）：

`enforce: 'pre'` + `transform` 钩子拦截 `@vite/client` 模块，替换 vite 注入的占位符：

| 占位符 | 注入值（修复前） | 替换值（修复后） |
|--------|----------------|----------------|
| `__HMR_HOSTNAME__` | `"localhost"` | auto-detected `req.headers.host` 剥端口 |
| `__HMR_PORT__` | `5174` | `16666`（preview-gateway port） |
| `__HMR_PROTOCOL__` | `"ws"` | `"wss"` 当 Referer 是 `https://`，否则 `"ws"` |
| `__HMR_BASE__` | `"/"` | `"/"`（保持） |
| `__WS_TOKEN__` | vite 随机 token | **不要替换**（gateway 透传给上游） |

**为什么 `enforce: 'pre'`？** vite 8 内部用 `code.replace('__HMR_HOSTNAME__', hostReplacement)` 替换占位符，`enforce: 'pre'` 保证我们的 transform 在 vite 内部替换之前先替换好。vite 内部再 replace 时找不到占位符就跳过。

**auto-detect 流程**：
```
1. 浏览器 GET http://run-agent-...trae.cn:16666/openlist-ui/
2. 网关剥前缀 → :5174 vite 收到 GET /
3. vite 中间件 (configureServer) 检测 req.headers.host = 'run-agent-...trae.cn:16666'
   → 存到 detectedHost / detectedProtocol
4. vite 响应 HTML (含 <script src="/openlist-ui/@vite/client">)
5. 浏览器 GET /openlist-ui/@vite/client → 网关剥前缀 → :5174 vite
6. transform @vite/client → dynamicHmrHostPlugin (enforce:'pre') 替换占位符
7. 浏览器收到 client.mjs，HMR client 连接 ws://run-agent-...trae.cn:16666/?token=...
8. 网关 WS upgrade → cookie 路由到 :5174 → vite 接受
```

**(3) vite 8 WS upgrade 监听器要求 `Sec-WebSocket-Protocol: vite-hmr`**：

vite 8 在 `if (wsServer)` 分支只对带 `vite-hmr` 或 `vite-ping` subprotocol 的 WS upgrade 调 `handleUpgrade`。`@vite/client` 实际 `new WebSocket(url, 'vite-hmr')` 会带，但 curl/wscat 测试要手动加：

```js
req.headers['Sec-WebSocket-Protocol'] = 'vite-hmr'  // 关键！
```

### 12.2 五件套修复清单

| # | 文件 | 改动 | 作用 |
|---|------|------|------|
| 1 | `app/encv-mobile/plugin-openlist/web/vite.config.ts` | 加 `server.allowedHosts: true` | 让 vite 接受外部 Host |
| 2 | `app/encv-mobile/plugin-openlist/web/vite.config.ts` | 加 `dynamicHmrHostPlugin` (enforce:'pre') | 替换 @vite/client 占位符为外部 host |
| 3 | `app/encv-mobile/vite.config.ts` | 同上（主 app 版本，端口 16666） | 同上 |
| 4 | `app/preview-gateway/src/server.ts` | （已存在）cookie 路由 + WS upgrade 透传 | 让根路径 WS upgrade 路由到正确 upstream |
| 5 | （浏览器端）`@vite/client` | 自动带 `Sec-WebSocket-Protocol: vite-hmr` | 触发 vite WS 处理 |

### 12.3 env var 覆盖

`dynamicHmrHostPlugin` 支持 env var 覆盖 auto-detect，优先级：**env > auto-detect > fallback**：

| env | 默认值 | 说明 |
|-----|--------|------|
| `HMR_HOST` | auto-detected | 外部 host（不含端口） |
| `HMR_PROTOCOL` | auto from Referer | `ws` 或 `wss` |
| `HMR_CLIENT_PORT` | `16666` | 浏览器连的端口（应等于 preview-gateway port） |

### 12.4 完整验证命令

```bash
# A) @vite/client 接受外部 Host + HMR config 正确
curl -s -H "Host: run-agent-test.trae.cn:16666" http://127.0.0.1:5174/@vite/client \
  | grep -E "(socketHost|hmrPort|socketProtocol) ="
# 期望：socketHost = ${"run-agent-test.trae.cn" || ...}:16666/

# B) 经 gateway 完整链路（cookie 路由）
curl -s -H "Host: run-agent-test.trae.cn:16666" \
  http://localhost:16666/openlist-ui/@vite/client \
  | grep -E "(socketHost|hmrPort) ="

# C) WS 升级测试（必须带 Sec-WebSocket-Protocol: vite-hmr）
node -e "
const http = require('http');
const req = http.request({
  host:'127.0.0.1', port:16666, path:'/?token=test', method:'GET',
  headers:{
    'Connection':'Upgrade','Upgrade':'websocket',
    'Sec-WebSocket-Key':'dGhlIHNhbXBsZSBub25jZQ==',
    'Sec-WebSocket-Version':'13',
    'Sec-WebSocket-Protocol':'vite-hmr',
    'Cookie':'__plugin_spa=1',
    'Host':'run-agent-test.trae.cn:16666',
  },
});
req.on('upgrade',(r)=>console.log('UPGRADE',r.statusCode));
req.on('response',(r)=>console.log('RESP',r.statusCode));
req.on('error',(e)=>console.log('ERR',e.message));
req.setTimeout(3000,()=>{console.log('TIMEOUT');process.exit(0)});
req.end();
"
# 期望：UPGRADE 101
```

### 12.5 已知限制

1. **auto-detect 是一次性的**：第一个请求的 Host 锁定后不变。如果用同一 vite 进程服务多个域名（罕见），需要每个域名单独启动 vite。
2. **HTTPS 场景必须靠 Referer 推断 protocol**：如果 referer 不可靠（如 strict-origin-when-cross-origin），可用 `HMR_PROTOCOL=wss` env 覆盖。
3. **__WS_TOKEN__ 不能改**：vite 启动时随机生成，注入 @vite/client。gateway 透传 token，vite 验证。

---

## 十三、沙箱 dev HMR 彻底禁掉（D15）

> **核心结论**：`server.hmr = false` **不够**。vite 8 (rolldown) 即便 hmr:false，**仍然**往 HTML 注入 `<script src="/@vite/client">`。必须用一个 `transformIndexHtml: { order: 'post' }` 的内联 plugin，**物理删除**这个 script 标签。

### 13.1 问题链路（再踩坑）

D14 已经验证了：sandbox dev 链路中 `agent-tool-host :16000` 不支持 WebSocket 升级。`server.hmr = false` 应该让 vite **完全不注入** HMR client 脚本，浏览器也就不会尝试连 WS。

**但实际**：vite 8 (rolldown) 走的是 `htmlRewritePlugin`，**与 `server.hmr` 配置解耦** —— hmr:false 只关 HMR 的 WS server，**不阻止** `<script src="/@vite/client">` 注入。结果：
- HTML 仍有 `<script src="/@vite/client">`
- 浏览器加载这个脚本 → 内部立刻 `new WebSocket(wss://...:16666/?token=...)` → 立即关闭 → console 抛红字
- 用户看到 `[vite] failed to connect to websocket (Error: WebSocket closed without opened.)`

### 13.2 修复方案：transformIndexHtml 物理删除

```ts
{
  name: 'remove-vite-client-sandbox-dev',
  transformIndexHtml: {
    order: 'post',  // 必须在 htmlRewritePlugin 之后，否则改的是空字符串
    handler(html: string) {
      return html.replace(
        /<script\s+type="module"\s+src="[^"]*\/@vite\/client"[^>]*><\/script>/g,
        '<!-- @vite/client removed (hmr disabled in sandbox dev) -->',
      )
    },
  },
}
```

**为什么 `order: 'post'`**：htmlRewritePlugin 在 transformIndexHtml 阶段（具体阶段名为 `transformIndexHtml`）注入 @vite/client。Vite 的 plugin 钩子按 order 排序：pre → 默认 → post。我们的删除必须在 htmlRewritePlugin 之后执行，所以用 `order: 'post'`。

**为什么是内联 plugin 而非顶层 `transformIndexHtml`**：vite 8 (rolldown) 的顶层 `transformIndexHtml` config 在某些情况下被忽略（实测），所以保险起见写在 `plugins: []` 数组里。

### 13.3 副带修复（plugin-openlist vite.config.ts）

修主 app 的同时，必须同步修 `app/encv-mobile/plugin-openlist/web/vite.config.ts`：

1. **`import { fileURLToPath, URL } from 'node:url'` 改为 `import path from 'node:path'`**
   - 根因：vite 8 bundler 对 `node:url` 的命名导入处理有 bug，runtime 抛 `ReferenceError: fileURLToPath is not defined`
   - 改为与主 app 一致的 `path.resolve(__dirname, 'src')` 即可

2. **`server.middlewares.use('/__openlist-health', ...)` 改为 `/openlist-ui/__openlist-health`**
   - 根因：preview-gateway 的 `/openlist-ui` upstream 去掉了 pathRewrite（见 13.4），请求 URL 保留完整前缀
   - vite 中间件必须挂到完整路径 `/openlist-ui/__openlist-health` 才能匹配

3. **删除多余的 `},` 闭包**
   - 之前留下的 parse error：`vite.config.ts:368:1 })` Unexpected token
   - 之前 fs.allow 块结尾多一个 `},`，defineConfig 被提前闭合

### 13.4 副带修复（preview-gateway pathRewrite）

`/openlist-ui` upstream **必须去掉** `pathRewrite: strip`：

| 旧行为 | 新行为 |
|--------|--------|
| `/openlist-ui/` → `/` (剥前缀) | `/openlist-ui/` → `/openlist-ui/` (identity) |
| `/openlist-ui/src/main.ts` → `/src/main.ts` | `/openlist-ui/src/main.ts` → `/openlist-ui/src/main.ts` |

**为什么**：vite 收 `/` 跟 `base: '/openlist-ui/'` 不匹配，立刻 302 跳 `/openlist-ui/`，形成无限重定向循环。保留前缀后 vite 收 `/openlist-ui/` 完整路径，匹配自己 base，零重定向。

### 13.5 验证结果（agent-browser 实测）

| 页面 | URL | appInnerLen | viteScriptCount | consoleErrors | viteErrors |
|------|-----|-------------|-----------------|---------------|------------|
| 主 app | `/tabs/home` | 4628 | 0 | [] | [] |
| plugin-openlist | `/openlist-ui/#/home` | 4953 | 0 | [] | [] |
| OpenList 后端 | `/openlist/` | (React) | n/a | [] | [] |

**0 个 `[vite] failed to connect to websocket` 错误，0 个 console error，三服务全部正常渲染。**

### 13.6 沙箱 dev 经验教训

> **你的经验在沙箱面前不值一提** —— 沙箱有自己的物理限制（agent-tool-host 不支持 WS 升级），不能套用本地 dev 的"开箱即用"经验。

沙箱 dev 与本地 dev 的差异清单：

| 维度 | 本地 dev | 沙箱 dev |
|------|---------|---------|
| HMR | ✅ vite 默认开 | ❌ agent-tool-host 不支持 WS → 必须禁 |
| 域名 | `localhost:5173` | trae 域名（hash 每次重启都变） |
| 多上游端口 | 浏览器直接连 | 必须统一过 preview-gateway :16666 |
| HMR 错误可见 | console warning | console **红字**（看起来像严重错误） |

---

## 九、相关 spec / 文档

- [openlist-frontend-extraction-and-sandbox-preview](file:///workspace/.trae/specs/openlist-frontend-extraction-and-sandbox-preview/spec.md) — OpenList 前端抽取 + 浏览器预览（上一轮）
- [wire-openlist-runtime-and-ui-v2](file:///workspace/.trae/specs/wire-openlist-runtime-and-ui-v2/spec.md) — OpenList 运行时 + UI 集成
- [app/preview-gateway/README.md](file:///workspace/app/preview-gateway/README.md) — 网关项目文档（待写）
