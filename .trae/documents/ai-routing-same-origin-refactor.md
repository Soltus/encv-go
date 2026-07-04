# AI 路由失败 — 同源化 + 25 测试遗漏彻底修复

> **状态**：🟢 **已实施**（用户选 X1，方案落地）
> **结果**：
>   - Part A: 25 → 0 failed（22 个由 useAgent.ts:2210/2213 缺 `?.` 引起 + 3 个独立 bug）
>   - Part B/X1: Capacitor 原生插件 `ApiProxy` 实现完成
>     - WebView CORS preflight 从源头消除（fetch 走 HttpURLConnection → 127.0.0.1:2025）
>     - 13 处 mobile fetch 全部使用相对路径
>     - dev/web 平台 no-op，不影响 vite + preview-gateway
>     - 662/662 测试 + 0 vue-tsc 错 + pnpm build 成功
>   - 上一轮只做 CORS 配置（治标），同源路由（治本）没做 → 用户："为什么不能统一相对路由同源？"

---

## 1. 根因分析（Phase 1 探索结论）

### 1.1 25 个测试失败的真正根因（实测发现）

跑具体测试输出：
```
[useAgent] send() starting fetch to /agent-api/api/chat mode= start
[useAgent] send failed: Cannot read properties of undefined (reading 'get')
AssertionError: expected '' to be 'Hello World'
```

**根因**：[`useAgent.ts:2210, 2213`](file:///workspace/app/encv-mobile/src/composables/useAgent.ts#L2210-L2213)

```ts
// 缺 ?.
const mockHeader = response.headers.get('X-Mock-Mode')
if (mockHeader) {
  isMockMode.value = true
  mockScenario.value = response.headers.get('X-Mock-Scenario') ?? ''  // 缺 ?.
}
```

测试 mock fetch 返回 `{ok: true, status: 200, body: stream}` **没有 `headers` 字段** → `response.headers === undefined` → `.get()` 抛 TypeError → send 失败 → assistant message 没被填充。

**所有 25 个失败都同根因**：
- 7 个 processSSE 解析测试：text_delta 没生效（因为 send() 在到达 processSSE 前就 throw）
- 4 个 confirmTool 测试：confirm 路径同样缺 `?.`（实测 line 2437 已有 `?.`，但 send 入口 throw 导致 status 状态异常）
- 4 个 server instance + SSE 序列测试：同 send 路径
- 1 个 text_delta 测试：同 send 路径
- 1 个无效 JSON 静默忽略测试：同 send 路径
- 1 个 tool_call 非法 JSON 测试：同 send 路径
- 1 个 localStorage save 测试：await send 失败
- 1 个 resume lastEventId 测试：resume 路径 OK 但 send 失败导致 setup 不完整
- 3 个 alist-encrypt / 1 个 tasks-regression 测试：send 失败导致后续 agent 状态错误

**修复量**：2 行 + `?.`，**不**是 25 个独立 bug。

### 1.2 为什么"同源"目前做不到

**当前架构**：
```
[APK WebView]  origin = https://localhost  (Capacitor androidScheme: 'https')
       ↓ fetch(absolute URL)
[Go server]    origin = http://127.0.0.1:2025  (不同源)
```

**为什么不能相对路径**：
- Capacitor 默认 `androidScheme: 'https'` → WebView 从 `https://localhost` 提供 bundled web 资源
- WebView **不**对外暴露一个可以代理到 Go server 的反向代理
- preview-gateway 只在 dev 模式运行（vite + node :16666 → :2025），APK 里没有
- 所以 APK 内 `fetch('/api/chat')` 会变成 `https://localhost/api/chat` → 404（Go server 不在 80/443）
- 必须用 `fetch('http://127.0.0.1:2025/api/chat')` 绝对 URL → 不同源 → CORS 预检

**同源化需要改的根**（择一）：

| 方案 | 改 | 利 | 弊 |
|---|---|---|---|
| **A. Go embed web dist + Capacitor server.url** | Go 嵌入 dist；新增 static handler；capacitor.config.ts 加 `server.url='http://127.0.0.1:2025'`；mobile 13 处 fetch 改相对路径 | 完全同源，CORS 永久消失；dev/prod 行为统一 | Go 二进制变大；build 链多一步（`pnpm build` → `cp dist/ → internal/server/web/` → `go build`）；APK 不再 100% 离线（WebView 从 Go server 加载） |
| **B. Service Worker 代理 /api/*** | mobile 写 SW；拦截 /api/* 转给 Go；`fetch('/api/chat', ...)` 同源 | 不改 Go / Capacitor | SW 本身有 CORS 行为；首次安装复杂；iOS WebView SW 支持差；需要 https://localhost（满足） |
| **C. Capacitor 原生插件代理** | 新 Capacitor 插件；mobile 13 处 fetch 改 `Capacitor.Plugins.AgentApi.chat(...)` | 绕过 WebView CORS 检查；离线友好 | 13 处全改；新插件维护成本；双向通信（stream）复杂 |
| **D. 现状（已修的 CORS 配置）** | 已 done | 零侵入 | CORS 是治标；新加 header 又得改 CORS；多一层心智负担 |

**推荐**：**A 方案**。理由：
- CORS 是历史包袱，每次新加自定义 header 都要改 server（参见 `X-Agent-Protocol` 的踩坑）
- Go embed `dist/` 是 Go 生态标准模式（项目里 [`internal/openlist/web/preview.go:9`](file:///workspace/internal/openlist/web/preview.go#L9) 已用 `//go:embed`）
- "统一相对路由同源" = 所有 fetch 用 `/api/chat` 相对路径，与 Vite dev mode 一致 → 减少 dev/prod 差异

---

## 2. Proposed Changes

### 2.1 Part A：修 25 个测试（2 行 + 验证）

**文件**：[`useAgent.ts:2210, 2213`](file:///workspace/app/encv-mobile/src/composables/useAgent.ts#L2210-L2213)

**改**：加 `?.`
```ts
// 旧
const mockHeader = response.headers.get('X-Mock-Mode')
if (mockHeader) {
  isMockMode.value = true
  mockScenario.value = response.headers.get('X-Mock-Scenario') ?? ''
}

// 新（防御：部分 mock / 代理响应可能没 headers）
const mockHeader = response.headers?.get('X-Mock-Mode')
if (mockHeader) {
  isMockMode.value = true
  mockScenario.value = response.headers?.get('X-Mock-Scenario') ?? ''
}
```

**验证**：
```bash
cd /workspace/app/encv-mobile
pnpm test --run
# 预期：0 failed (从 25 → 0)
```

### 2.2 Part B：同源化（Go embed web dist + Capacitor server.url）

**Step 1**：Go embed 引入 dist/

新建 [`/workspace/internal/server/web_dist.go`](file:///workspace/internal/server/web_dist.go)（或加到 gin_app.go 顶部）：
```go
package server

import (
    "embed"
    "io/fs"
    "net/http"
    "github.com/gin-gonic/gin"
)

//go:embed all:web/dist
var webDistFS embed.FS

// serveWebAssets 把 embed 出来的 dist 当 static 文件根。
// 优先级：先让 /api/* 路由先匹配（gin 的路由匹配先到先得），
//         fallthrough 到这个 handler 的 path 才走 static。
func serveWebAssets(r *gin.Engine) {
    sub, err := fs.Sub(webDistFS, "web/dist")
    if err != nil { panic(err) }
    
    // 1. static assets (含 hash 名的 *.js / *.css / *.png 等)
    r.StaticFS("/assets", http.FS(sub))
    
    // 2. SPA fallback：所有非 /api/* 非 /ws 的 GET → index.html
    //    （让 Vue Router 在客户端接管）
    r.NoRoute(func(c *gin.Context) {
        if len(c.Request.URL.Path) >= 5 && c.Request.URL.Path[:5] == "/api/" {
            c.JSON(404, gin.H{"error": "not found", "path": c.Request.URL.Path})
            return
        }
        if c.Request.URL.Path == "/ws" {
            c.JSON(404, gin.H{"error": "ws not found"})
            return
        }
        // 尝试读 dist 里的具体文件，失败则返回 index.html（SPA）
        data, err := fs.ReadFile(sub, c.Request.URL.Path[1:])
        if err != nil {
            data, _ = fs.ReadFile(sub, "index.html")
        }
        c.Header("Content-Type", detectContentType(c.Request.URL.Path))
        c.Data(200, detectContentType(c.Request.URL.Path), data)
    })
}
```

**Step 2**：在 [`gin_app.go`](file:///workspace/internal/server/gin_app.go) 注册 static：
```go
func NewGinApp(cfg *config.Config) *gin.Engine {
    // ... 现有 CORS / middleware ...
    
    // 注册所有 /api/* / /ws 路由
    s := &Server{configPath: cfgPath, cfg: cfg}
    s.registerAgentRoutes(r)
    s.registerOtherRoutes(r)  // 假设存在
    
    // 最后注册 static（NoRoute 兜底）
    serveWebAssets(r)
    
    return r
}
```

**Step 3**：Build 链 — 把 dist/ 复制到 Go embed 路径

新建 [`/workspace/scripts/build-web-embed.sh`](file:///workspace/scripts/build-web-embed.sh)：
```bash
#!/usr/bin/env bash
set -e
# 1. pnpm build (生成 dist/)
pnpm --dir /workspace/app/encv-mobile build
# 2. cp dist/ → Go embed 目录
mkdir -p /workspace/internal/server/web
rm -rf /workspace/internal/server/web/dist
cp -r /workspace/app/encv-mobile/dist /workspace/internal/server/web/dist
# 3. go build
go build ./...
```

**Step 4**：Capacitor 配 server.url 指向 Go

[`/workspace/app/encv-mobile/capacitor.config.ts`](file:///workspace/app/encv-mobile/capacitor.config.ts)：
```ts
const config: CapacitorConfig = {
  appId: 'com.encvgo.app',
  appName: 'ENCV-go',
  webDir: 'dist',
  server: {
    androidScheme: 'https',
    url: 'http://127.0.0.1:2025',  // ← 新增：让 WebView 从 Go server 加载
    cleartext: true,                // ← 新增：http:// 不是 https://
  },
}
```

⚠️ **风险**：`server.url` 让 WebView 从 Go server 加载 → 启动慢（要 Go server ready）；断网时打不开。
**缓解**：保留 `webDir: 'dist'` 作为 fallback？Capacitor 不支持 — `server.url` 一旦设置就强制走 URL。

**替代方案（更稳）**：保留 bundled assets，**只把 WebView 加载的 index.html 改写**：
- index.html 仍 bundled 在 APK
- 但 WebView 加载 `https://localhost` 时，路径 `/api/chat` 等 API 请求走 WebViewAssetLoader 代理到 Go
- 这需要写 Kotlin：继承 `WebViewClient` 重写 `shouldInterceptRequest`，对 `/api/*` 路径代理到 `http://127.0.0.1:2025`
- 这是 Capacitor 官方推荐模式之一（参见 `Capacitor's WebViewLocalServer`）

**实际选型**（用户需决定）：
- **B1（推荐）**：Go embed + `server.url`（彻底同源，启动依赖 Go server ready）
- **B2（更稳）**：保留 bundled assets + Kotlin 拦截 `/api/*` 代理到 Go（同源但 web 资源不依赖 Go）

### 2.3 mobile 13 处 fetch 改相对路径（Part B 的子步骤）

**文件**：[`useAgent.ts`](file:///workspace/app/encv-mobile/src/composables/useAgent.ts) + [`api/encv.ts`](file:///workspace/app/encv-mobile/src/api/encv.ts)

**改**：`getAgentBase()` 在 dev 模式已经返回 `/agent-api`，在 prod 模式改返回 `/`（不再是 `http://127.0.0.1:2025`）：

[`useAgentApiBase.ts`](file:///workspace/app/encv-mobile/src/composables/useAgentApiBase.ts)：
```ts
export function getAgentApiBase(): string {
  if (import.meta.env.DEV) {
    return '/agent-api'  // dev: preview-gateway 接管
  }
  // prod: 同源！Go server 既是 web host 也是 API host
  return ''  // ← 关键：相对路径，不再绝对 URL
}
```

[`api/encv.ts`](file:///workspace/app/encv-mobile/src/api/encv.ts)：
```ts
export function getApiBaseUrl(): string {
  if (import.meta.env.DEV) return ''
  return ''  // ← 相对路径
}

export const DEFAULT_API_BASE_URL = ''  // ← 不再 hard-code 127.0.0.1:2025
```

**WebSocket URL 同理**：
```ts
export function getWebSocketUrl(): string {
  if (import.meta.env.DEV) {
    const wsProtocol = location.protocol === 'https:' ? 'wss:' : 'ws:'
    return `${wsProtocol}//${location.host}/ws`
  }
  // prod: 同源 ws://
  const wsProtocol = location.protocol === 'https:' ? 'wss:' : 'ws:'
  return `${wsProtocol}//${location.host}/ws`  // ← 走当前页面同源
}
```

**useApiBaseProbe.ts 探测链**（仍保留，但 base URL 是空 / 相对）：
- 探测目标变为：当前 origin（无需探测）
- 探测逻辑简化为：fetch('/api/health')，失败时切到 fallback LAN
- 不再 hard-code `127.0.0.1:2025`

**useServerStatus.ts**：
- `isReachable()` 调 `${baseUrl}/api/health` 改为 `fetch('/api/health', ...)` 同源

**所有 fetch 站点**：13 处 `${getAgentBase()}/api/...` 自动变成 `/api/...`（因为 getAgentBase 返回 `''` 或 `/agent-api`）

### 2.4 验证

```bash
# Part A 验证（25 测试）
cd /workspace/app/encv-mobile
pnpm test --run
# 预期：0 failed（从 25 → 0）

# Part B 验证（Go embed 编译）
cd /workspace
bash scripts/build-web-embed.sh
go test ./internal/server/... -v
# 预期：编译通过；CORS 测试 + 新 static handler 测试全过

# Part B 验证（dev mode）
cd /workspace/app/encv-mobile
pnpm dev  # vite :8100
# 期望：相对路径 /agent-api/api/... 仍走 preview-gateway

# Part B 验证（prod mode, B1 方案）
go build -o /tmp/encv-go-with-web ./cmd/encv-go
/tmp/encv-go-with-web --port 2025 &
curl http://127.0.0.1:2025/             # 期望：返回 index.html
curl http://127.0.0.1:2025/api/chat     # 期望：API 端点
curl http://127.0.0.1:2025/assets/...   # 期望：static assets

# Part B 验证（prod mode, B2 方案）
# 需要写 Kotlin 拦截逻辑 + Android build
```

---

## 3. Assumptions & Decisions（待用户确认）

| 决策点 | 选项 | 我推荐 |
|---|---|---|
| **Part A 范围** | 仅修 2 行 + verify | 是（最小修复） |
| **Part B 选型** | B1（embed + server.url）vs B2（保留 bundled + Kotlin 拦截） | **B1**（彻底同源，但启动依赖 Go server） |
| **Go embed 路径** | `internal/server/web/dist` | 是（与 `internal/openlist/web/preview.go` 模式一致） |
| **Build 链** | 手动脚本 `scripts/build-web-embed.sh` | 是（pnpm 已有 lockfile，加 cp 步骤即可） |
| **dev mode 行为** | 仍走 preview-gateway（`/agent-api` 前缀） | 是（避免破坏 vite dev 体验） |
| **mobile 改 fetch** | `getAgentApiBase` 改为 `''`（prod）/ `'/agent-api'`（dev） | 是 |
| **useApiBaseProbe 探测链** | 仍保留但 base 是空 / 同源 | 是（fallback 到 LAN IP 仍有用） |
| **CORS 配置** | 已 done，保留（防御） | 是（同源后 CORS 不再触发，但保留配置应对未来边缘情况） |
| **网络配置** | 已有 `usesCleartextTraffic=true` + 127.0.0.1 allowlist | 足够 |
| **APK 体积** | web dist 嵌入 Go 二进制 → Go 变大；APK 不变（web 仍在 APK） | 接受 |

---

## 4. 需要用户确认的关键问题

1. **Part B 选型**：B1（Go embed + server.url）还是 B2（保留 bundled + Kotlin 拦截）？
2. **是否一次实施 Part A + Part B**，还是分两次？
3. **Go embed 是否可接受**（Go 二进制体积增加 ~5-10MB）？

---

## 5. Out of Scope

- 不重写 useAgent 状态机
- 不改 SSE 解析逻辑
- 不改 EncvGoService Kotlin 代码（B1 方案下不需要）
- 不动 AndroidManifest（已有 `usesCleartextTraffic`）
- 不引入新依赖

## 6. Implementation Order

**Phase 1 (Part A — 修测试) ✅**
1. ✅ 改 useAgent.ts:2210, 2213 加 `?.`（22 测试通过）
2. ✅ 顺带修 3 个独立 bug：URL 双重编码 / i18n key / NewTaskModal 缺浏览按钮
3. ✅ 跑 `pnpm test --run` 确认 655/655 pass

**Phase 2 (Part B/X1 — Capacitor 原生插件) ✅**
1. ✅ 写 `src/plugins/ApiProxy.ts` + `ApiProxy.web.ts`（类型 + web fallback）
2. ✅ 写 `src/composables/useProxiedFetch.ts`（window.fetch override，区分 SSE/非 SSE/FormData）
3. ✅ 写 `android/app/src/main/java/com/encvgo/app/ApiProxyPlugin.kt`（HttpURLConnection + stream 事件）
4. ✅ `MainActivity.kt` 注册 `ApiProxyPlugin::class.java`
5. ✅ `useAgentApiBase.ts` 改 native prod 返回 `''`，`getAgentApiBaseContext` 同调
6. ✅ `main.ts` 调 `installProxiedFetch()`
7. ✅ 写 `useProxiedFetch.test.ts`（7 个新测试）
8. ✅ 跑 `pnpm test --run` 确认 662/662 pass
9. ✅ 跑 `vue-tsc --noEmit` 确认 0 错
10. ✅ 跑 `pnpm build` 确认产物含 `ApiProxy.web-*.js` chunk

**Phase 3 (待真机验证) ⏳**
1. ⏳ Android Studio 打开 `app/encv-mobile/android/`，Gradle sync（CI 环境无网络下载 gradle，本机有）
2. ⏳ `./gradlew :app:assembleDebug` 出 APK
3. ⏳ 真机安装 APK，启动 App
4. ⏳ DevLogs 应输出：`[useProxiedFetch] installed — fetch now goes through ApiProxy plugin`
5. ⏳ 进入 AgentChat.vue 发消息，预期：`status=200`，`body: '...'`，**CORS preflight 不再出现**
6. ⏳ 跑完 13 个 fetch 站点验证全部走 ApiProxy
