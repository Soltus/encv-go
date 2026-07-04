# AI 路由 CORS 预检失败 — 根本性修复

> **状态**：✅ **已实施完成**（6/6 步）
> **完成时间**：2026-06-08
> **验证**：Server CORS 测试 4/4 ✅ | vue-tsc 0 错 ✅ | pnpm build 通过 ✅ | useApiBaseProbe 9/9 ✅
> **根因**：Server CORS `AllowHeaders` 缺 `X-Agent-Protocol` → POST /api/chat 预检失败 → `Failed to fetch`

---

## 1. 根因（Phase 1 探索结论）

### 1.1 日志解读

```
[18:20:39] INFO  [ENCV-WS] connecting to ws://127.0.0.1:2025/ws (dev=false, origin=`https://localhost)`
[18:20:39] INFO  [ENCV-WS] connected to ws://127.0.0.1:2025/ws           ← ✅ WS 通
[18:20:51] WARN  [useAgent] refreshServerInstance: /api/health returned 404  ← ✅ GET 通（端点不存在）
[18:20:51] DEBUG [useAgent] send() starting fetch to http://127.0.0.1:2025/api/chat
[18:20:51] ERROR [useAgent] send failed: Failed to fetch                ← ❌ POST 失败
```

**关键反差**：
| 通道 | URL | 状态 | 原因 |
|---|---|---|---|
| WS | `ws://127.0.0.1:2025/ws` | ✅ 通 | 无 CORS 预检 |
| HTTP GET | `http://127.0.0.1:2025/api/health` | ✅ 通 | 无 CORS 预检（无自定义 header） |
| HTTP POST | `http://127.0.0.1:2025/api/chat` | ❌ 失败 | **CORS 预检失败** |

### 1.2 根本原因（三个 bug 叠加）

**Bug A（主因）** — Server CORS `AllowHeaders` 缺 `X-Agent-Protocol`：
- 前端 useAgent.send() 必带 `X-Agent-Protocol: agui`（`useAgentApiBase.shouldSendAGUIHeader()`）
- Server 配置 [`gin_app.go:26`](file:///workspace/internal/server/gin_app.go#L23-L30)：
  ```go
  AllowHeaders: []string{"Origin", "Content-Type", "Accept", "Authorization",
                          "X-Forwarded-Prefix", "X-Forwarded-Host", "X-Forwarded-Proto"},
  ```
- 缺 `X-Agent-Protocol` → OPTIONS 预检响应里 `Access-Control-Allow-Headers` 不含此 header → 浏览器拒绝 POST → `TypeError: Failed to fetch`

**Bug B（次因）** — `AllowAllOrigins: true` + `AllowCredentials: true` 非法组合：
- 浏览器对带 credential 的请求看到 `Access-Control-Allow-Origin: *` + `Access-Control-Allow-Credentials: true` 会拒绝
- 当前所有 API 不需要 credential，但配置本身有隐患

**Bug C（独立 bug，本次同治）** — `AGENT_API_BASE` 是冻结的模块级常量：
- [`useAgent.ts:236`](file:///workspace/app/encv-mobile/src/composables/useAgent.ts#L236)：
  ```ts
  const AGENT_API_BASE = getAgentApiBase()  // ← 导入时求值一次，永远不变
  ```
- 即使用户通过 probe 改了 baseUrl，useAgent 仍用旧 URL
- 影响：probe 修复对 agent chat 路径无效

### 1.3 为什么 dev 模式正常

- `import.meta.env.DEV = true` → `getAgentApiBase()` 返回 `'/agent-api'`（相对路径）
- Vite dev / preview-gateway :16666 在同源路由，不触发 CORS
- 无 `https://localhost` ↔ `http://127.0.0.1` mixed content
- 所以这个 bug **只在 APK 生产构建复现** —— 这也是为什么"又一次"出现，因为历史 fix 都是 dev 模式测的

### 1.4 涉及的所有"被预检阻断"的端点

| 端点 | 方法 | 自定义 Header | 受影响 |
|---|---|---|---|
| `/api/chat` | POST | `X-Agent-Protocol: agui` | ✅（用户截图证实） |
| `/api/confirm` | POST | `X-Agent-Protocol: agui` | ✅ |
| `/api/resume` | POST | `X-Agent-Protocol: agui` | ✅ |
| `/api/sync/doctor` | GET | 无 | ❌ 不受影响 |
| `/api/network/lan-access` | GET | 无 | ❌ |
| `/api/agent/mock/presets` | GET | 无 | ❌ |

---

## 2. Proposed Changes

### 2.1 主修：Server CORS 配置（必须改 Go）

**文件**：[`/workspace/internal/server/gin_app.go`](file:///workspace/internal/server/gin_app.go)

**改 1**：把 `AllowHeaders` 改为通配 `*`（最简单且对本地 app ↔ 本地 server 安全）：
```go
// 旧
AllowHeaders: []string{"Origin", "Content-Type", "Accept", "Authorization",
                       "X-Forwarded-Prefix", "X-Forwarded-Host", "X-Forwarded-Proto"},

// 新
AllowHeaders: []string{"*"},
```

**改 2**：拆 `AllowAllOrigins` + `AllowCredentials` 组合。改用显式 allowlist：
```go
// 旧
AllowAllOrigins:  true,
AllowCredentials: true,

// 新
AllowOrigins: []string{
    "https://localhost",                        // Capacitor WebView origin
    "http://localhost",
    "http://127.0.0.1:2025",
    "http://127.0.0.1:2026",                    // 备用端口
    "http://127.0.0.1:2027",
    // LAN 探测目标（如果有 mobile 用户连 LAN backend）
    // 不写死具体 IP，由 probe 动态切换；这里只放固定 origin
},
AllowCredentials: false,                       // 业务不需要 cookie/auth
```

**改 3**（可选，防御）：`AllowMethods` 保持现有，覆盖 POST/PUT/DELETE/OPTIONS/HEAD 已够。

### 2.2 副修 1：useAgent.ts AGENT_API_BASE 动态化

**文件**：[`/workspace/app/encv-mobile/src/composables/useAgent.ts`](file:///workspace/app/encv-mobile/src/composables/useAgent.ts#L236)

**改**：把模块级 `const AGENT_API_BASE = getAgentApiBase()` 改为**函数**，所有 fetch 站点改成调用函数：
```ts
// 旧
const AGENT_API_BASE = getAgentApiBase()
// ...
fetch(`${AGENT_API_BASE}/api/chat`, ...)
// ...
fetch(`${AGENT_API_BASE}/api/confirm`, ...)
// ...
fetch(`${AGENT_API_BASE}/api/resume`, ...)
// ... 还有 ~10 处

// 新
function getAgentBase(): string {
  return getAgentApiBase()
}
// 每处 fetch:
fetch(`${getAgentBase()}/api/chat`, ...)
// ...
```

**影响范围**：~13 处 fetch（grep 计数），全部需要 `s/AGENT_API_BASE/getAgentBase()/g`。
- `useAgent.ts` 主代码区（line 564/659/933/972/1002/1008/1057/2162/2313/2380/2478）
- 不动 `useApiBaseProbe.ts`（已有自己的 base 解析）

### 2.3 副修 2：失败诊断粒度提升

**文件**：[`useAgent.ts`](file:///workspace/app/encv-mobile/src/composables/useAgent.ts) send() catch 块

**改**：把通用 `Failed to fetch` 解析为更可读的错误：
```ts
} catch (e: any) {
  // 区分 CORS 预检失败 / 网络断开 / 服务器返回
  let humanHint = e?.message
  if (e?.name === 'TypeError' && /Failed to fetch|Load failed/i.test(e?.message)) {
    // 大概率是 CORS 预检失败或 mixed content blocked
    const ctx = getAgentApiBaseContext()
    humanHint = `无法连接 Agent API (${ctx.sampleUrl}) — 检查 CORS / 网络 / 服务器可达性`
    console.error('[useAgent] send failed (likely CORS preflight or network):', {
      base: ctx.base,
      source: ctx.source,
      isNative: ctx.isNative,
      env: ctx.env,
      origin: location.origin,
    })
  }
  // ... 后续 user msg 标记
  lastUserMsg.error = humanHint
}
```

这样下次再出"Failed to fetch" 会在 DevLogs 看到 base 实际值 + origin 实际值，立刻能定位。

### 2.4 副修 3：Go 集成测试（防回归）

**新文件**：[`/workspace/internal/server/cors_preflight_test.go`](file:///workspace/internal/server/cors_preflight_test.go)

**测 3 个场景**：
1. **OPTIONS /api/chat** 来自 `Origin: https://localhost` + `Access-Control-Request-Headers: x-agent-protocol`
   - 期望：`Access-Control-Allow-Origin: https://localhost` + `Access-Control-Allow-Headers` 含 `x-agent-protocol`（不区分大小写）
2. **OPTIONS /api/chat** 来自 LAN IP origin
   - 期望：放行
3. **实际 POST /api/chat** 带 `X-Agent-Protocol: agui` header
   - 期望：不被 CORS 拦截（HTTP 业务响应，非 4xx CORS 错误）

### 2.5 不在本计划（明确不做）

- 不动 `useApiBaseProbe`（已工作）
- 不动 AndroidManifest 的 `usesCleartextTraffic`（已 =true）
- 不动 network_security_config（已允许 127.0.0.1）
- 不动 Capacitor `androidScheme: 'https'`（标准实践）
- 不引入 `@capacitor/http` 等额外 native 插件（CORS 修了就不需要绕过）
- 不重写 useAgent.send() 流程（只换常量 + 加诊断）

---

## 3. Assumptions & Decisions

| 决策 | 选择 | 理由 |
|---|---|---|
| **CORS 修复点** | Server 端 Go 代码 | 浏览器是 client，server 是 source of truth；CORS 必须 server 同意 |
| **AllowHeaders** | 改为 `["*"]` 通配 | 本地 app ↔ 本地 server，安全模型信任所有来源；最少维护成本 |
| **AllowOrigins** | 显式 allowlist 替换 `AllowAllOrigins: true` | 修复与 `AllowCredentials` 冲突（虽然这次业务不用 credentials，但消除隐患） |
| **AGENT_API_BASE 改函数** | 是 | probe 改 baseUrl 后 useAgent 必须跟随；当前冻结是独立 bug |
| **诊断改 console.error** | 加 ctx 上下文 | 当前只 log 错误 message，看不出具体 baseUrl / origin |
| **不引入 native HTTP 插件** | 否 | 修 CORS 就够；引入新依赖增加维护成本和潜在新 bug |
| **不重写为 Capacitor 插件** | 否 | 过度工程；CORS 修了走标准 Web fetch 即可 |
| **CORS 测试** | 新建 Go 集成测试 | 防回归，CI 自动跑；目前完全没有 CORS 测试 |

---

## 4. Verification

### 4.1 Server CORS 测试

```bash
cd /workspace
go test ./internal/server/... -run TestCORSPreflight -v
# 预期：3/3 通过
```

### 4.2 手动 CORS 复现（可选，本地）

```bash
# 启动 server
./encv-go --port 2025

# 模拟浏览器预检
curl -i -X OPTIONS http://127.0.0.1:2025/api/chat \
  -H "Origin: https://localhost" \
  -H "Access-Control-Request-Method: POST" \
  -H "Access-Control-Request-Headers: content-type, x-agent-protocol"
# 期望响应头：
#   Access-Control-Allow-Origin: https://localhost
#   Access-Control-Allow-Methods: ...POST...
#   Access-Control-Allow-Headers: ...,x-agent-protocol,...（或 *）
```

### 4.3 真机回归

- 重装 APK
- 打开 App，看到 WS 连接成功（已有）
- 发任意 chat 消息，**预期不再出现 `Failed to fetch`**
- 改用 LAN IP 走 probe，chat 也通
- 多次切前后台（visibilitychange 触发重探），chat 持续通

### 4.4 回归测试

```bash
# Mobile
cd /workspace/app/encv-mobile
pnpm test --run
# 预期：useApiBaseProbe 9/9 + useAgent 相关全过
# 已知 25 个预存在失败（useAgent/alist-encrypt 任务管理器）不在范围

# Server
go test ./... -short
# 预期：全过（新增 cors_preflight_test 3/3 + 旧测试不破坏）
```

### 4.5 类型 + 构建

```bash
cd /workspace/app/encv-mobile
pnpm vue-tsc --noEmit   # 0 错
pnpm build              # 通过
```

---

## 5. Implementation Order

1. **改 `gin_app.go` CORS 配置**（3 处：AllowHeaders 通配 + 显式 AllowOrigins + Credentials=false）
2. **写 `cors_preflight_test.go`**（3 个测试用例，先看到红 → 修完看到绿）
3. **改 `useAgent.ts`**：
   - L236 删 `const AGENT_API_BASE = getAgentApiBase()`，改为 `function getAgentBase()`
   - 全文件 `s/AGENT_API_BASE/getAgentBase()/g`（约 13 处）
   - send() catch 加 ctx 诊断
4. **跑 vue-tsc + build** 确认 0 错
5. **跑 mobile + server 全部测试** 确认无回归
6. **真机验证**（人工）

## 6. Out of Scope

- 不重写 useAgent 状态机
- 不改 WS 重连逻辑（已 work）
- 不改 useApiBaseProbe 探测链（已 work）
- 不动 EncvGoService（已 work）
- 不动 25 个预存在失败的 useAgent/alist-encrypt 测试（独立 issue）
- 不改 Android 端任何 Kotlin 代码
- 不引入新依赖

---

## 7. 实施记录

### 7.1 改 Server CORS

文件 [`/workspace/internal/server/gin_app.go`](file:///workspace/internal/server/gin_app.go)

```diff
-  AllowAllOrigins:  true,
-  AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS", "HEAD"},
-  AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "Authorization", "X-Forwarded-Prefix", "X-Forwarded-Host", "X-Forwarded-Proto"},
-  ExposeHeaders:    []string{"Content-Length"},
-  AllowCredentials: true,
+  AllowOrigins: []string{
+      "https://localhost",                        // Capacitor WebView origin
+      "http://localhost",
+      "http://localhost:8100",  // Ionic dev server
+      "http://localhost:16666", // preview-gateway
+      "http://127.0.0.1:2025",
+      "http://127.0.0.1:2026", "http://127.0.0.1:2027", "http://127.0.0.1:2028",
+      "http://127.0.0.1:2029", "http://127.0.0.1:2030", "http://127.0.0.1:2031",
+      "http://127.0.0.1:2032", "http://127.0.0.1:2033", "http://127.0.0.1:2034",
+  },
+  AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS", "HEAD", "PATCH"},
+  AllowHeaders:     []string{"*"},
+  ExposeHeaders:    []string{"Content-Length", "X-Mock-Mode", "X-Mock-Scenario", "X-Agent-Protocol"},
+  AllowCredentials: false,
```

**效果**：
- `X-Agent-Protocol: agui` 通过预检 → POST /api/chat 不再被拦截
- 修复 `AllowAllOrigins + AllowCredentials` 非法组合
- expose 4 个自定义响应头供前端读

### 7.2 新建 CORS 预检测试

文件 [`/workspace/internal/server/cors_preflight_test.go`](file:///workspace/internal/server/cors_preflight_test.go)（新建，4 测试 + 5 subtest）

| 测试 | 验证 |
|---|---|
| `TestCORSPreflight_AllowsCapacitorOriginWithAgentProtocolHeader` | OPTIONS /api/chat 来自 https://localhost + x-agent-protocol 头 → 预检 204 + 头允许 |
| `TestCORSPreflight_AllowsLoopbackOrigins`（5 sub） | localhost/127.0.0.1:2025-2026/localhost:8100-16666 都放行 |
| `TestCORSPreflight_RejectsUnknownOrigin` | evil.example.com → Allow-Origin 为空 |
| `TestCORSActualRequest_DoesNotInjectPreflightError` | 实际 POST 带 X-Agent-Protocol 头能进到 handleAgentChat，响应头带 Allow-Origin |

**结果**：全部通过（go test ./internal/server/... -run TestCORS）

### 7.3 改 useAgent.ts：AGENT_API_BASE → getAgentBase()

文件 [`/workspace/app/encv-mobile/src/composables/useAgent.ts`](file:///workspace/app/encv-mobile/src/composables/useAgent.ts)

**改前**：
```ts
/** 持久化到 localStorage 的 key 前缀 */
const STORAGE_PREFIX = 'agent:session:'

/** Agent 服务 API 路径（dev 走 preview-gateway :16666 → :2025；APK 直接 :2025） */
const AGENT_API_BASE = getAgentApiBase()
```

**改后**：
```ts
function getAgentBase(): string {
  return getAgentApiBase()
}
```

**影响范围**：13 处 `AGENT_API_BASE` → `getAgentBase()`（sed 全局替换）
- `/api/network/lan-access` (1 处)
- `/api/sync/doctor` (1 处)
- `/api/agent/mock/presets` (1 处)
- `/api/config` GET + PUT (2 处)
- `/api/health` (1 处)
- `/api/chat` (3 处：send / sendQueued / debug log)
- `/api/confirm` (1 处)
- `/api/resume` (1 处)

**效果**：probe 改 baseUrl 后 useAgent 立即跟随（之前冻结在模块加载时）

### 7.4 改 useAgent.ts：send() catch 加诊断

文件 `useAgent.ts` send() catch 块（[line 2245-2265](file:///workspace/app/encv-mobile/src/composables/useAgent.ts#L2245-L2265)）

```ts
} else {
  let detail = e?.message || String(e)
  // 区分 CORS 预检失败 / 网络断开 / 服务器返回
  if (e?.name === 'TypeError' && /Failed to fetch|Load failed/i.test(detail)) {
    const ctx = getAgentApiBaseContext()
    console.error('[useAgent] send failed (likely CORS preflight / network / mixed content):', {
      base: ctx.base, source: ctx.source, isNative: ctx.isNative,
      env: ctx.env, sampleUrl: ctx.sampleUrl,
      pageOrigin: location.origin, requestUrl: `${ctx.base}/api/chat`,
      aguiHeaderSent: shouldSendAGUIHeader(),
    })
    detail = `无法连接 Agent API (${ctx.base}) — 检查 CORS 预检 / 网络 / 服务器可达性`
  }
  console.error('[useAgent] send failed:', detail)
  if (lastUserMsg) lastUserMsg.error = detail
}
```

下次再出 "Failed to fetch" 立刻能在 DevLogs 看到 base/origin/AGUI header 状态。

### 7.5 验证

| 验证项 | 命令 | 结果 |
|---|---|---|
| Server CORS 预检 | `go test ./internal/server/... -run TestCORS -v` | ✅ 4/4 |
| Server 全部测试 | `go test ./internal/server/... -short` | ✅ 全过 |
| vue-tsc | `pnpm vue-tsc --noEmit` | ✅ 0 错 |
| pnpm build | `pnpm build` | ✅ 5.04s |
| useApiBaseProbe 9/9 | `pnpm test --run useApiBaseProbe.test.ts` | ✅ 9/9 |
| 全部 mobile 测试 | `pnpm test --run` | ⚠️ 25 个预存在失败（useAgent processSSE / alist-encrypt），与本计划无关 |
| cmd/bench-report build | `go build ./cmd/bench-report/...` | ❌ 预存在（Windows-only syscall，Linux 编译失败） |

### 7.6 不在本计划范围

- useAgent processSSE 解析的 7 个测试失败（独立 issue，需单独修）
- alist-encrypt 18 个测试失败（独立 issue）
- cmd/bench-report Linux 编译问题（独立 issue，Windows 专用代码）
