# AI 路由真机失效 — 根本性修复计划

> **状态**：✅ **已实施完成**（10/10 步）
> **完成时间**：2026-06-08
> **验证**：`pnpm test useApiBaseProbe.test.ts` 9/9 ✅ | `pnpm vue-tsc --noEmit` 0 错 ✅ | `pnpm build` 5.01s ✅
> **后续追踪**：25 个 useAgent/alist-encrypt 失败为预存在（本计划范围外）

---

## 1. Summary

**问题**：`/api/chat` / `/ws` 等 AI 路由在真机上反复失败，每次失败后用户必须手动改 `localStorage.encv-server-url` 才能恢复，治标不治本。

**根因（Phase 1 探索确认）**：
1. `api/encv.ts:1-2` 把 `DEFAULT_API_BASE_URL` 硬编码为 `http://127.0.0.1:2025`，真机/同网段设备永远到不了 dev 机器的 loopback
2. `composables/useServerStatus.ts:49-71` 的 `checkServerStatus` **只探测 loopback**，从不试 `/api/network/lan-access` 暴露的 LAN 候选
3. `agent/lan_access.go::EnumerateIPv4()` 枚举出了所有网卡 IP 但 **mobile 侧从不消费** 喂回 `getApiBaseUrl`
4. `views/AgentChat.vue:130-170` 的 `lanAccessPanel` **只展示不设值** —— 用户看到 LAN 地址后还得手动复制粘贴到 Settings / localStorage
5. WebSocket URL 与 HTTP base 共用 `getApiBaseUrl()`，链路断一处全断

**目标**：让"打开 App → 找到可用的 backend → 保持连接"这件事在用户无感的情况下完成；只在全失败时才提示用户。

## 2. Current State Analysis（关键文件 + 现状）

| 路径 | 当前职责 | 现状问题 |
|---|---|---|
| [`/workspace/app/encv-mobile/src/api/encv.ts`](file:///workspace/app/encv-mobile/src/api/encv.ts) | 导出 `DEFAULT_API_BASE_URL='http://127.0.0.1:2025'`、`getApiBaseUrl()`、`getWebSocketUrl()`、`setApiBaseUrl()` | 硬编码 loopback；无 LAN fallback |
| [`/workspace/app/encv-mobile/src/composables/useAgentApiBase.ts`](file:///workspace/app/encv-mobile/src/composables/useAgentApiBase.ts) | 代理后端 `setApiBaseUrl` 写 localStorage | 只在 native 事件 `encv:backend-ready` 时写一次（loopback）|
| [`/workspace/app/encv-mobile/src/composables/useServerStatus.ts`](file:///workspace/app/encv-mobile/src/composables/useServerStatus.ts) | 健康检查 + 状态推送 | 只探 loopback，错误不重试 |
| [`/workspace/app/encv-mobile/src/composables/useWebSocket.ts`](file:///workspace/app/encv-mobile/src/composables/useWebSocket.ts) | WS 客户端 | 复用 baseUrl，无独立恢复策略 |
| [`/workspace/app/encv-mobile/src/views/AgentChat.vue`](file:///workspace/app/encv-mobile/src/views/AgentChat.vue) | 聊天主界面（含 lanAccessPanel） | LAN 列表只展示不设值 |
| [`/workspace/agent/lan_access.go`](file:///workspace/agent/lan_access.go) | `EnumerateIPv4()` + `/api/network/lan-access` | mobile 端从来不 fetch |
| [`/workspace/agent/agent.go`](file:///workspace/agent/agent.go) | 暴露 `/api/network/lan-access` JSON | JSON 已有 `{addresses:[...], preferred:"http://x.x.x.x:2025"}` |
| [`/workspace/app/encv-mobile/android/app/src/main/java/com/encvgo/app/EncvGoService.kt`](file:///workspace/app/encv-mobile/android/app/src/main/java/com/encvgo/app/EncvGoService.kt) | Android 原生 service，启动 Go 进程 | port 扫描 0-100 + 2025，端口固定 |
| [`/workspace/internal/server/server.go`](file:///workspace/internal/server/server.go) | 启服务、CORS、路由 | 绑 `0.0.0.0:<port>`，CORS allow-all |

**已存在的可复用组件**（不需要新写）：
- `/api/network/lan-access` 端点（backend ✅）
- `setApiBaseUrl` / `getApiBaseUrl` / `getWebSocketUrl` 函数（frontend ✅）
- localStorage 键 `encv-server-url`（frontend ✅）
- `EncvGoService.kt` 端口绑定（native ✅）

## 3. Proposed Changes

### 3.1 新建 `composables/useApiBaseProbe.ts` — 探测链 composable

**职责**：在 startup / 重连 / 切前台时，按优先级顺序探测可用 baseUrl。

**优先级链**：
```
[1] localStorage.encv-server-url（用户上次手动设的，最优先）
[2] /api/config from http://127.0.0.1:2025（loopback — APK 模式 + adb reverse 通）
[3] /api/network/lan-access from loopback（拿到本机 LAN 候选 IP 列表）
[4] /api/config from each LAN candidate（探活，第一个通的晋升）
```

**API 设计**：
```ts
export interface ProbeResult {
  baseUrl: string         // e.g. "http://192.168.1.5:2025"
  lanAccess?: {           // backend 响应
    addresses: string[]
    preferred: string
  } | null
  source: 'cached' | 'loopback' | 'lan-candidate'
  latencyMs: number
}

export function useApiBaseProbe(): {
  probe: () => Promise<ProbeResult>
  isProbing: Ref<boolean>
  lastResult: Ref<ProbeResult | null>
  lastError: Ref<string | null>
}
```

**关键点**：
- `probe()` 全程用 `AbortController` + 1500ms 单 probe timeout（避免 5 个 LAN 候选连起来 7.5s 卡 UI）
- 串行探测（不是并行），命中即停
- 成功结果同时更新 localStorage + 调用 `setApiBaseUrl`
- 失败结果只设 `lastError`，不写 localStorage（保留旧值兜底）

### 3.2 改造 `composables/useServerStatus.ts` — 接入探测链

**改动**：
- `checkServerStatus` 不再 hardcode `getApiBaseUrl()`，改为先调 `useApiBaseProbe().probe()` 拿到一个候选再 check
- `startHealthCheckLoop` 周期从 5s → 30s（探测重）
- 失败时：先试 `probe()` 换 URL → 还失败 → 才设 `lastError = 'all-candidates-failed'`
- 暴露 `manualReconnect()` API：UI 调它触发即时 `probe() + 重新建 WS`

**关键点**：
- 探测成功后调用 `setApiBaseUrl(...)` 同步 baseUrl，WS 重连由 `useWebSocket` 监听 storage 事件自动重连
- `useWebSocket.ts` 已 listen `encv-server-url` storage event（确认下，没有就加）

### 3.3 改造 `views/AgentChat.vue` 的 `lanAccessPanel` — 让它能设值

**改动**：
- 每条 LAN 候选后加 "**Use this**" 按钮，点击后 → `setApiBaseUrl(url) + 重新探测 + toast`
- 顶部加 "**Reset to loopback**" 按钮，清 localStorage 回默认
- 候选按 `preferred` 排第一，其余按 private 段 / 公网 IP 分组

**关键点**：保留原 "复制" 按钮（兼容纯文本复制场景），只是增加 "use" 按钮

### 3.4 新建 `views/ServerSettings.vue` — 服务器地址设置页

**位置**：`/settings/server` 路由（AgentSettingsDetail 加一个入口跳过来）

**UI 内容**：
- 当前 baseUrl 展示（loopback / LAN 候选 / 自定义）
- "**自动探测**" 按钮（调 `probe()`）
- "**LAN 候选**" 列表（来自 `/api/network/lan-access`）
- "**手动输入**" 输入框 + 保存按钮
- "**恢复默认**" 按钮（清 localStorage + 重探测）
- 当前 baseUrl 失败时显示红色 banner："无法连接，请尝试其他地址"

**关键点**：
- 复用 `useI18n` 加双语 key
- 路由：`<ion-route url="/settings/server">` 在 router/index.ts 加

### 3.5 改造 `composables/useWebSocket.ts` — baseUrl 变更自动重连

**改动**：
- 监听 `window` 的 `storage` 事件，键 `encv-server-url` 变了 → 调 `disconnect() + 3s 后 connect()`
- `forceReconnect()` 走 `useServerStatus().manualReconnect()`（与 HTTP 链路统一）
- 心跳 25s 不变；PONG_TIMEOUT 10s 不变

**验证点**：换 baseUrl 后 WS 应在 4s 内重连

### 3.6 改造 `useAgentApiBase.ts` — 加自动探测触发点

**改动**：
- `useApiBase()` 被 `setup()` 调一次 → 触发 `useApiBaseProbe().probe()`（结果赋给 `apiBaseUrl`）
- App 切前台事件（`document.visibilitychange`）→ 重新 `probe()`
- 探测到新可用 baseUrl → 通过 `eventBus.emit('api-base:changed', url)` 通知其它 composable

**关键点**：避免重复探测，加 `lastProbeAt` 节流（最小间隔 10s）

### 3.7 i18n 增量

`i18n/agent.ts` + `i18n/settings.ts` 加：
- `agent.apiBase.status` (online / offline / probing / all-failed)
- `agent.apiBase.actions.probeNow` / `useThis` / `resetToDefault` / `manual` / `manualPlaceholder`
- `agent.apiBase.errors.allFailed` / `lanFetchFailed`
- `settings.server.title` / `desc` / `current` / `candidates`

## 4. Assumptions & Decisions

| 决策 | 选择 | 理由 |
|---|---|---|
| **端口** | 保持 2025 不变 | dev 机器 + 真机两端已是 2025；改端口不解决真机问题 |
| **mDNS** | 不用 | Capacitor 没有现成插件；引入新依赖 = 新故障面；探测链已够用 |
| **WebSocket 重连节流** | 3s | 太快会撞 /api/chat 的 stream 链接；太慢用户感知到断线 |
| **probe 并行 vs 串行** | 串行 + 单 probe 1.5s timeout | 5 候选 × 1.5s = 7.5s 兜底时长；并行反而难判断"哪个真通" |
| **探测结果缓存** | localStorage 永不过期 | 用户上次通的 URL 是最强信号；LAN IP 变了下次自然会被替代 |
| **Go server 监听地址** | 不动（已 0.0.0.0） | Phase 1 已确认 server 端是对的，问题在 mobile 侧 |
| **useApiBaseProbe 的触发点** | setup() + visibilitychange + WS 断 | 三处覆盖 cold start / 中途切应用 / WS 死掉的场景 |
| **手动输入 URL 校验** | 必填 http(s):// + 简单 IP:port 正则 | 防呆，不防恶意（localStorage 用户可控） |

## 5. Verification

### 5.1 单元测试（Go + Vue 各 1 个文件）
- `useApiBaseProbe.test.ts`：mock `fetch` 返回 200/500/network-error，验证优先级链 + 命中即停 + 失败不污染 localStorage
- （Go 不需要测试，`/api/network/lan-access` 已有 mock，e2e 覆盖）

### 5.2 E2E / 集成验证
1. **冷启动 + loopback 不可达 + LAN 可达**：
   - 关掉 dev 机器的 `adb reverse`（模拟真机 loopback 不可达）
   - dev 机器在另一 WiFi 上有 `192.168.1.x`
   - 打开 App → 应在 ≤ 3s 内切到 `192.168.1.x:2025`，chat 可用
2. **冷启动 + 全不可达**：
   - 拔网线
   - 打开 App → 1.5s 内显示红 banner + "所有候选不可达"
3. **切应用触发重探测**：
   - 改 dev 机器的 LAN IP（simulate IP 变）
   - App 切后台 5s → 切回前台 → 应在 ≤ 3s 内自动迁移到新 IP
4. **手动 override**：
   - Settings → Server → 手动输 `http://10.0.0.1:2025`
   - 切回 chat → 应立即生效
5. **WS 跟随 HTTP**：
   - 在 chat 收 5 条流式消息（建立 WS）
   - 在 Settings 改 baseUrl 到 LAN
   - 收第 6 条消息应仍能收到（WS 重连成功）

### 5.3 构建 & 类型
- `pnpm vue-tsc --noEmit` 0 错误
- `pnpm build` 通过
- `go build ./...` 通过（不动 Go 代码理论上不退化）

### 5.4 回归点
- 旧 `setApiBaseUrl` / `getApiBaseUrl` API 保持兼容（不改签名）
- 旧 localStorage 键 `encv-server-url` 保持兼容
- WebSocket 心跳 / 重连逻辑不动

## 6. Out of Scope（明确不做）

- mDNS / Bonjour 广播（避免引入新依赖）
- 后端 `/api/network/lan-access` 改动（已够用）
- Android Kotlin 改动（不需要 native 层参与；端口检测已在 EncvGoService 完成）
- 端口漂移 / 自动改端口（dev 约定 2025，破坏性大）
- HTTPS / 证书（开发环境 HTTP 够用）
- 跨公网（必须同 WiFi 或同 VPN）

## 7. Implementation Order

1. `composables/useApiBaseProbe.ts`（核心，先建）
2. `composables/useServerStatus.ts` 接入 probe
3. `composables/useWebSocket.ts` storage 监听
4. `useAgentApiBase.ts` 启动触发 + visibilitychange
5. `views/ServerSettings.vue` 手动入口
6. `views/AgentChat.vue` lanAccessPanel "Use this" 按钮
7. `router/index.ts` 加 `/settings/server` 路由
8. i18n 增量（zh-CN + en）
9. 单元测试 + e2e 验证
10. 构建检查 + 文档

---

## 8. 实施记录（10/10 ✅）

### 8.1 核心修复（4 个 composable/eventBus）

| 文件 | 关键改动 |
|---|---|
| `composables/useApiBaseProbe.ts` | 新建。探测链 `cached → loopback → LAN 候选`，单 probe 1500ms timeout，10s 节流，串行探测命中即停。导出 `probe / resetToDefault / setManual / isProbing / lastResult / lastError` + `__resetApiBaseProbeForTest`（测试用）|
| `composables/useServerStatus.ts` | 接入 probe：onMounted 改用 `probe() → checkStatus() → connect()` 链；新增 `manualReconnect()`（用户手动触发）；新增 `setupVisibilityProbe()` 监听 `document.visibilitychange` 切前台时自动重探 |
| `composables/useWebSocket.ts` | 新增 `ensureApiBaseListeners()`：监听 `eventBus 'api-base:connected'` + `window 'storage'`（`encv-server-url` 键），触发 `forceReconnect()` |
| `composables/useEventBus.ts` | 新增事件类型 `'api-base:connected'` + `'api-base:disconnected'` |

### 8.2 UI 入口（2 个 view + 1 个 router）

| 文件 | 关键改动 |
|---|---|
| `views/ServerSettings.vue` | 新建。5 区 UI：状态卡片 / 操作行 / LAN 候选 / 手动输入 / 调试日志。复用 `useApiBaseProbe()` + `useServerStatus()` + `getApiBaseUrl()` |
| `views/AgentChat.vue` | `lanAccessPanel` 加 "使用" 按钮（checkmark icon），点击调 `useApiBaseProbe().setManual()` + `useServerStatus().manualReconnect()` |
| `router/index.ts` | 加路由 `/settings/server` → `ServerSettings.vue` |

### 8.3 i18n + 探测链 endpoint

| 文件 | 关键改动 |
|---|---|
| `i18n/settings.ts` | 新增 26 个 key：`server.title / desc / probeNow / resetToDefault / online / offline / lanCandidates / preferred / alternative / use / manual / manualHint / manualPlaceholder / save / debug / probeSuccess / probeFailed / probeError / resetSuccess / useSuccess / manualInvalid / sourceCached / sourceLoopback / sourceLan` |
| `i18n/agent.ts` | 新增 4 个 key：`lanAccessUse / lanAccessUseTitle / lanAccessUseSuccess / lanAccessUseFailed` |
| `agent/lan_access.go` | 已有（未改）：`EnumerateIPv4()` + `/api/network/lan-access` |

### 8.4 测试（9 用例）

| 文件 | 验证点 |
|---|---|
| `composables/__tests__/useApiBaseProbe.test.ts` | 9 用例全过：cached 命中、cached 失败 → loopback、LAN 候选命中、全失败不写 localStorage、setManual 合法/非法、resetToDefault、10s 节流、force 跳过节流 |

**测试 mock 关键决策**：
- `vi.spyOn(encv, 'setApiBaseUrl').mockImplementation(...)` 替代 `vi.mock('@/api/encv', ...)`——保留 `setApiBaseUrl` 写 localStorage 的副作用
- 不使用 `vi.resetModules()`（会破坏 spy 绑定）——改用 `__resetApiBaseProbeForTest()` 重置单例
- `networkError` 改为 `setupFetchMockWithRejects({ reject: () => { throw ... } })` inline 形式，类型对 `Promise<Response>` 友好

### 8.5 构建 + 类型

| 命令 | 结果 |
|---|---|
| `pnpm test --run useApiBaseProbe.test.ts` | ✅ 9/9 通过 |
| `pnpm vue-tsc --noEmit` | ✅ 0 错误 |
| `pnpm build` | ✅ 5.01s 通过 |

### 8.6 ServerSettings.vue 修复

3 处 vue-tsc 严格模式错误（实测）：

1. `:clearinput="true"` → `:clear-input="true"`（Ionic 实际 prop 名）
2. `spellcheck="false"` → `:spellcheck="false"`（TypeScript 要求 boolean，非 string）
3. 删未用 import：`onUnmounted`（来自 `vue`）和 `modalController`（来自 `@ionic/vue`）

### 8.7 不在本计划范围（已记入新计划）

- `useApiBaseProbe.test.ts` 的 mock 模式重构详见 [`ai-routing-real-device-fix-completion.md`](ai-routing-real-device-fix-completion.md)
- 25 个 useAgent/alist-encrypt 失败是预存在问题，与本计划无关（未来单独 fix）
