# Realtime Transport 托管层重构（优雅解决沙箱预览 WS 不可用）

> **目标**：消除「每个用到 WS 的地方都得自己判断 isOpenPreviewBrowser / isSandboxBrowser」的散落模式，
> 把 transport 选择 + fallback 集中到**一个**单例 useRealtimeTransport()，
> 消费方**零修改**（继续走 eventBus），新增 WS 事件类型不用动业务代码。

---

## 一、问题背景（用户原话复述）

> /plan 你忘了沙箱预览ws不可用吗？考虑到有很多用到ws的地方，为了避免每个地方都fallback兼容维护地狱，需要重构一种优雅的托管方案

**沙箱预览 WS 不可用的根因**（已实测，[preview-management.md §三.1](file:///workspace/.trae/rules/preview-management.md)）：

```
外网用户浏览器
  ↓
:16000 (agent-tool-host)  ← 唯一外网入口
  ↓ 内部 preview-proxy
:16666 (preview-gateway)   ← 沙箱内统一入口
  ↓
:8100 (encv-mobile-vite)   ← 主 app
```

trae 反代 :16000 **不支持 WebSocket upgrade**（实测 WebSocket upgrade → 502）。
OpenPreview 工具激活后 → 浏览器必须走 trae 域名 → 所有 WS 必失败。

**当前散落兼容**（痛点来源）：

| 文件 | 行 | 判断 | 作用 |
|------|----|------|------|
| [useWebSocket.ts](file:///workspace/app/encv-mobile/src/composables/useWebSocket.ts) | L100-108 | `isOpenPreviewBrowser()` | 不连 WS，只 emit `server:status online:true` |
| [useServerStatus.ts](file:///workspace/app/encv-mobile/src/composables/useServerStatus.ts) | L41-44 | `isSandboxBrowser` | 抑制 `server:connection-error` 误显 |
| [useFrontendLogs.ts](file:///workspace/app/encv-mobile/src/composables/useFrontendLogs.ts) | L75-92 | 独立硬编码 | 降级 vite HMR WS noise（仅 console.error 过滤） |
| [DevLogs.vue](file:///workspace/app/encv-mobile/src/views/DevLogs.vue) | L303-308 | onMounted 重复 `ws.connect()` | 与 App.vue 的 connect 形成 2 次尝试 |

**散落的代价**：
- 新增 1 个 WS 事件类型 → 至少改 4 个文件
- `useTaskEventBridge` 已经走 eventBus（OK），但 `useServerStatus` 自己也写 `eventBus.on('server:connection-error')` 绕过 transport
- HMR noise 降级只覆盖 console.error，没覆盖 console.warn
- `useWebSocket.ts` 暴露 `connectionState` ref，但 `useServerStatus` 又有自己的 `isOnline` ref（两份数据源）

---

## 二、目标架构（用户已确认）

### 2.1 核心设计

```
                 ┌─────────────────────────────────┐
                 │   useRealtimeTransport() (singleton) │
                 │                                  │
                 │   选举 transport 模式:           │
                 │   - isNative()          → native-bridge
                 │   - isOpenPreviewBrowser() → http-poll
                 │   - 否则                → ws        │
                 │                                  │
                 │   内部持有:                       │
                 │   - WsBackend (现有 useWebSocket 行为)
                 │   - HttpPollBackend (新)          │
                 │   - NativeBridgeBackend (APK)     │
                 │                                  │
                 │   统一 emit eventBus:            │
                 │   task:update/progress/created/  │
                 │   completed/refresh,             │
                 │   server:status/connection-error,│
                 │   ws:message, log:message        │
                 └─────────────────────────────────┘
                              ↓ emit
                       ┌──────────────┐
                       │   eventBus   │ (useEventBus 现有)
                       └──────────────┘
                              ↓ on
              ┌──────────┬──────────┬──────────┐
              │ Tasks.vue│ DevLogs  │ useServerStatus │
              │ useWF    │ useFrontendLogs        │
              │ useAGUIP │ useAutomationTests     │
              └──────────┴──────────┴──────────┘
```

**关键约束**（用户已选）：
1. **fallback 协议 = HTTP 轮询**（用现有 `GET /api/tasks` diff，后端零改动）
2. **消费方 API 粒度 = transport 内部 emit eventBus**（消费方零迁移）

### 2.2 HttpPollBackend 核心算法

```ts
// 节流：active=2s, idle=5s, hidden=30s
// 错误 backoff：1s → 2s → 4s → 8s → 30s
// 后端 GET /api/tasks 返回 tasks 数组
// 前端维护 lastSnapshot: Map<id, {status, progress, phase, speed, eta, error}>
// 每一轮 diff：
//   - 新 id          → emit('task:created', {id, type, sourcePath})
//   - status 变      → emit('task:update',   {id, type, status, progress})
//   - progress/phase  → emit('task:progress', {id, progress, phase, speed, eta})
//   - 终态 (success/failure/cancelled) → emit('task:completed', {id, error})
//   - 后端列表少 id   → emit('task:completed', {id, error: 'server-list-missing'})（不常见，容错）
```

### 2.3 transport 模式选择（按优先级）

```ts
function electTransportMode(): TransportMode {
  if (isNative()) return 'native-bridge'   // APK: 设备本地 backend, native plugin 桥
  if (isOpenPreviewBrowser()) return 'http-poll'  // trae 域名: 反代 :16000 不支持 WS upgrade
  return 'ws'                               // 沙箱本地 dev (localhost:16666) / 真机浏览器
}
```

**关键**：transport 一旦选举，**整个 session 不变**（不动态切换）。避免运行时反复重连。

---

## 三、实施计划

### 3.1 Phase A：新建 useRealtimeTransport 单例（核心）

**新建** `/workspace/app/encv-mobile/src/composables/useRealtimeTransport.ts`：

```ts
// 核心导出
export type TransportMode = 'ws' | 'http-poll' | 'native-bridge' | 'unknown'
export interface RealtimeTransport {
  connect(): void
  disconnect(): void
  forceReconnect(): Promise<void>
  connectionState: Readonly<Ref<ConnectionState>>
  transportMode: Readonly<Ref<TransportMode>>
  isSandboxBrowser: Readonly<Ref<boolean>>  // OpenPreview 浏览器标记（只读）
  /** 内部方法（仅测试用）：模拟 transport 模式强制切换 */
  __forceMode(mode: TransportMode): void
}
export function useRealtimeTransport(): RealtimeTransport
```

**模块级单例**（参照 `useApiBaseProbe` 实现），确保整个 app 共享同一个 transport 实例。

### 3.2 Phase B：HttpPollBackend 实现

**新建** `/workspace/app/encv-mobile/src/composables/realtime/HttpPollBackend.ts`：

```ts
export function createHttpPollBackend(emit: EventEmitter): Backend {
  // 状态
  let pollTimer: number | null = null
  let backoffMs = 1000
  const lastSnapshot = new Map<string, TaskSnapshot>()
  const BASELINE_INTERVAL_MS = 2000  // active
  const IDLE_INTERVAL_MS = 5000      // 用户 idle (无 UI 交互)
  const HIDDEN_INTERVAL_MS = 30000   // document hidden

  // 触发条件
  function intervalMs(): number {
    if (document.visibilityState === 'hidden') return HIDDEN_INTERVAL_MS
    return BASELINE_INTERVAL_MS
  }

  // 轮询 + diff + emit
  async function tick(): Promise<void> {
    try {
      const tasks = await fetchTasks()
      diffAndEmit(tasks)
      backoffMs = 1000  // 成功重置
      pollTimer = window.setTimeout(tick, intervalMs())
    } catch (e) {
      // 错误：emit server:connection-error（按 transport 层静默）
      pollTimer = window.setTimeout(tick, backoffMs)
      backoffMs = Math.min(backoffMs * 2, 30000)
    }
  }

  function diffAndEmit(tasks: EncvTask[]): void {
    const seen = new Set<string>()
    for (const t of tasks) {
      seen.add(t.id)
      const prev = lastSnapshot.get(t.id)
      if (!prev) {
        lastSnapshot.set(t.id, snapshotOf(t))
        emit('task:created', { id: t.id, type: t.type, sourcePath: t.sourcePath })
        continue
      }
      if (prev.status !== t.status) {
        emit('task:update', { id: t.id, type: t.type, status: t.status, progress: t.progress })
      }
      if (prev.progress !== t.progress || prev.phase !== t.phase) {
        emit('task:progress', { id: t.id, progress: t.progress, phase: t.phase, speed: t.speed, eta: t.eta })
      }
      if (isTerminal(t.status)) {
        emit('task:completed', { id: t.id, error: t.error })
      }
      lastSnapshot.set(t.id, snapshotOf(t))
    }
    // 防御：snapshot 有但 server 列表没了（罕见：后端被重启）→ 标 completed
    for (const id of lastSnapshot.keys()) {
      if (!seen.has(id) && !isTerminal(lastSnapshot.get(id)!.status)) {
        emit('task:completed', { id, error: 'server-list-missing' })
        lastSnapshot.delete(id)
      }
    }
  }

  return {
    start(): void { if (!pollTimer) tick() },
    stop(): void { /* clearTimeout + clear snapshot */ },
  }
}
```

### 3.3 Phase C：重构 useWebSocket.ts 为内部 backend

**改造** `/workspace/app/encv-mobile/src/composables/useWebSocket.ts`：

- **保留** 现有 WS 连接管理逻辑，但**不再**直接被 App.vue / DevLogs.vue 调用
- 改为暴露 `createWsBackend(emit)` factory（参考 useApiBaseProbe 的 createProbe 模式）
- 内部仍用 `getWebSocketUrl()` / `isOpenPreviewBrowser()` 判断
- 删除 `useWebSocket()` 公共导出（防止新代码误用）

**或更激进**：删除 useWebSocket.ts，把代码内联到 `useRealtimeTransport.ts` 的 `createWsBackend`。**倾向第二种**（更聚合，避免遗留 2 套 API）。

### 3.4 Phase D：消费方迁移

| 文件 | 改动 | 行数估计 |
|------|------|---------|
| [App.vue](file:///workspace/app/encv-mobile/src/App.vue) | L133 `const { connect, disconnect } = useWebSocket()` → `useRealtimeTransport()`；L371 `connect()` → `transport.connect()`；L377 `disconnect()` → `transport.disconnect()` | 3 处 |
| [useServerStatus.ts](file:///workspace/app/encv-mobile/src/composables/useServerStatus.ts) | L17 删除 `isSandboxBrowser` ref；L41-44 删除 isSandboxBrowser 判断；L69/L75/L116/L128/L161/L204/L223 全部 `useWebSocket().xxx` → `useRealtimeTransport().xxx`；L237 `useWebSocket()` → `useRealtimeTransport()`；L245-247 transportMode 选举改调 transport.electMode | 8 处 |
| [DevLogs.vue](file:///workspace/app/encv-mobile/src/views/DevLogs.vue) | L116 `useWebSocket()` → `useRealtimeTransport()`；L303-308 删重复 `ws.connect()`（transport 已自动管理） | 2 处 |
| [useFrontendLogs.ts](file:///workspace/app/encv-mobile/src/composables/useFrontendLogs.ts) | L75-92 HMR noise 降级保留（vite HMR 客户端噪声不依赖 transport，跟 OpenPreview 无关） | 0 处 |

**消费方零迁移清单**（已经走 eventBus，自动跟随 transport 切换）：
- [useTaskEventBridge.ts](file:///workspace/app/encv-mobile/src/composables/useTaskEventBridge.ts) — 不动
- [useWorkflowEngine.ts](file:///workspace/app/encv-mobile/src/composables/useWorkflowEngine.ts) — 不动
- [useAutomationTests.ts](file:///workspace/app/encv-mobile/src/composables/useAutomationTests.ts) — 不动
- [useTasksList.ts](file:///workspace/app/encv-mobile/src/composables/useTasksList.ts) — 不动
- [Tasks.vue](file:///workspace/app/encv-mobile/src/views/Tasks.vue) — 不动

### 3.5 Phase E：transport 模块边界收敛

为了让「transporter 选什么模式 / 怎么 fallback」的逻辑**只有一处**：

- [useRealtimeTransport.ts](file:///workspace/app/encv-mobile/src/composables/useRealtimeTransport.ts) **唯一**持有 `isOpenPreviewBrowser()` / `isNative()` 判断
- [useApiBaseProbe.ts](file:///workspace/app/encv-mobile/src/composables/useApiBaseProbe.ts) 的 isSandboxBrowserOrigin 仅用于**baseUrl 探测**（跟 transport 无关）
- [useWebSocket.ts](file:///workspace/app/encv-mobile/src/composables/useWebSocket.ts) **删除**整个文件（代码内联到 createWsBackend）

---

## 四、关键代码模式

### A. Transport 单例 + 内部 election

```ts
// useRealtimeTransport.ts
import { isOpenPreviewBrowser, getApiBaseUrl } from '@/api/encv'
import { isNative } from '@/plugins/GoProcess'
import { createWsBackend, type Backend } from './realtime/WsBackend'
import { createHttpPollBackend } from './realtime/HttpPollBackend'
import { createNativeBridgeBackend } from './realtime/NativeBridgeBackend'
import { eventBus } from './useEventBus'

export type TransportMode = 'ws' | 'http-poll' | 'native-bridge' | 'unknown'
export type ConnectionState = 'connecting' | 'connected' | 'disconnected'

let _instance: RealtimeTransport | null = null
let _forcedMode: TransportMode | null = null  // 仅测试

function electMode(): TransportMode {
  if (_forcedMode) return _forcedMode
  if (isNative()) return 'native-bridge'
  if (isOpenPreviewBrowser()) return 'http-poll'
  return 'ws'
}

function createTransport() {
  const connectionState = ref<ConnectionState>('disconnected')
  const transportMode = ref<TransportMode>('unknown')
  const isSandboxBrowser = ref(isOpenPreviewBrowser())
  let backend: Backend | null = null

  function ensureBackend(): Backend {
    if (backend) return backend
    const mode = electMode()
    transportMode.value = mode
    const emit = (type: string, data: any) => eventBus.emit(type as any, data)
    switch (mode) {
      case 'native-bridge': backend = createNativeBridgeBackend(emit); break
      case 'http-poll':     backend = createHttpPollBackend(emit); break
      case 'ws':            backend = createWsBackend(emit); break
    }
    return backend!
  }

  return {
    connect() {
      connectionState.value = 'connecting'
      ensureBackend().start()
      // WsBackend 自己会在 onopen 置 'connected'；HttpPollBackend 在首次 tick 成功置 'connected'
    },
    disconnect() { backend?.stop(); connectionState.value = 'disconnected'; backend = null },
    forceReconnect() { this.disconnect(); this.connect() },
    connectionState: connectionState as Readonly<Ref<ConnectionState>>,
    transportMode: transportMode as Readonly<Ref<TransportMode>>,
    isSandboxBrowser: isSandboxBrowser as Readonly<Ref<boolean>>,
    __forceMode(mode: TransportMode | null) { _forcedMode = mode; backend = null },
  }
}

export function useRealtimeTransport() {
  if (!_instance) _instance = createTransport()
  return _instance
}
```

### B. Backend 统一接口

```ts
// realtime/Backend.ts
export interface Backend {
  /** 启动 / 重连 */
  start(): void
  /** 停止（保留 backend 实例，可被 start 复用） */
  stop(): void
  /** 强制完全重置（backend 内部状态清空） */
  reset?(): void
}

export type EventEmitter = (type: string, data: any) => void
```

### C. HttpPollBackend 自适应节流

```ts
function intervalMs(): number {
  if (typeof document !== 'undefined' && document.visibilityState === 'hidden') {
    return HIDDEN_INTERVAL_MS
  }
  return BASELINE_INTERVAL_MS
}
```

### D. WsBackend 改造（保留所有现有能力）

```ts
// realtime/WsBackend.ts — 现有 useWebSocket.ts 的 connect/heartbeat/reconnect 全内联
// 唯一变化：把 emit('server:status') 改为走 EventEmitter，不再直接读 eventBus
export function createWsBackend(emit: EventEmitter): Backend {
  let ws: WebSocket | null = null
  let reconnectTimer: number | null = null
  let reconnectDelay = 1000
  let heartbeatTimer: number | null = null

  function start() {
    if (isOpenPreviewBrowser()) {
      // 防御：理论上 electMode 已分流到 http-poll，到这里说明 _forcedMode 或 bug
      console.warn('[WsBackend] called in OpenPreview; emitting online:true only')
      emit('server:status', { online: true })
      return
    }
    const url = getWebSocketUrl()
    ws = new WebSocket(url)
    ws.onopen = () => { emit('server:status', { online: true }); startHeartbeat() }
    ws.onclose = (e) => { emit('server:status', { online: false }); if (!e.wasClean) scheduleReconnect() }
    ws.onerror = () => emit('server:connection-error', { error: `failed to connect to ${url}` })
    ws.onmessage = (msg) => {
      try { const { type, data } = JSON.parse(msg.data); emit(type, data) }
      catch { /* ignore */ }
    }
  }
  function stop() { /* close ws + clear timers */ }
  function scheduleReconnect() { /* exp backoff */ }
  function startHeartbeat() { /* 每 30s ping */ }

  return { start, stop }
}
```

---

## 五、验证步骤

### 5.1 类型 + 单元测试
- `vue-tsc --noEmit` 必须 0 错误
- 新增 `useRealtimeTransport.test.ts`：
  - electMode：isNative / isOpenPreviewBrowser / 默认三分支
  - HttpPollBackend：模拟 fetch 返 tasks 列表 → 验证 emit `task:created` / `task:update` / `task:completed` 序列
  - HttpPollBackend：错误 backoff（mock fetch throw → backoffMs 累加）
  - HttpPollBackend：document hidden 切到 30s 节流
- 现有 `useWebSocket.test.ts` / `useServerStatus.test.ts` / `useApiBaseProbe.test.ts` 不能挂

### 5.2 后端 + go test
- `go test ./internal/...` 全部通过（理论上不涉及后端改动）
- `go build ./cmd/encv` 通过

### 5.3 运行时验证（3 个 mode 各跑一次）
1. **WS 模式**：沙箱本地 dev（直接打开 http://localhost:16666/）→ DevTools console 看 `[ENCV-WS] connected to ws://...`
2. **HTTP-poll 模式**：用 OpenPreview 工具激活外网 URL → DevTools console 看 `[HttpPoll] tick` 日志（每 2s 一次）→ 跑自动化测试看 task 卡片实时从 running → completed
3. **Native-bridge 模式**：APK 真机（本期无法实测，留 TODO）

### 5.4 回归检查清单
- [ ] Tasks 页面切回 tab 自动刷新（onIonViewWillEnter）仍工作
- [ ] 自动化测试 200 个 case 跑完状态正确（不再卡 running）
- [ ] DevLogs backend 标签能继续接 log 事件（http-poll 模式下 task 事件 + 控制台日志都正常）
- [ ] vite HMR noise 仍被降级（不影响）
- [ ] 沙箱 OpenPreview 浏览器不再有 `WebSocket error: url=ws://... readyState=3`（HttpPollBackend 根本不发 WS）

---

## 六、关键文件清单

### 新建
- `/workspace/app/encv-mobile/src/composables/useRealtimeTransport.ts` — 单例 transport 选举 + 生命周期
- `/workspace/app/encv-mobile/src/composables/realtime/Backend.ts` — Backend 接口 + EventEmitter 类型
- `/workspace/app/encv-mobile/src/composables/realtime/WsBackend.ts` — 从 useWebSocket.ts 提取的 WS 实现
- `/workspace/app/encv-mobile/src/composables/realtime/HttpPollBackend.ts` — 新增 HTTP 轮询实现
- `/workspace/app/encv-mobile/src/composables/realtime/NativeBridgeBackend.ts` — APK native bridge 桥（占位，先 throw + TODO）
- `/workspace/app/encv-mobile/src/composables/__tests__/useRealtimeTransport.test.ts` — 单测
- `/workspace/app/encv-mobile/src/composables/__tests__/realtime/HttpPollBackend.test.ts` — diff 算法 + 节流 + backoff 单测

### 修改
- `/workspace/app/encv-mobile/src/App.vue` — useWebSocket → useRealtimeTransport
- `/workspace/app/encv-mobile/src/composables/useServerStatus.ts` — 删 isSandboxBrowser，迁 transport
- `/workspace/app/encv-mobile/src/views/DevLogs.vue` — useWebSocket → useRealtimeTransport，删重复 connect

### 删除
- `/workspace/app/encv-mobile/src/composables/useWebSocket.ts` — 内容内联到 WsBackend.ts

### 不动（验证清单）
- [useTaskEventBridge.ts](file:///workspace/app/encv-mobile/src/composables/useTaskEventBridge.ts)
- [useWorkflowEngine.ts](file:///workspace/app/encv-mobile/src/composables/useWorkflowEngine.ts)
- [useAutomationTests.ts](file:///workspace/app/encv-mobile/src/composables/useAutomationTests.ts)
- [useTasksList.ts](file:///workspace/app/encv-mobile/src/composables/useTasksList.ts)
- [Tasks.vue](file:///workspace/app/encv-mobile/src/views/Tasks.vue)
- [useApiBaseProbe.ts](file:///workspace/app/encv-mobile/src/composables/useApiBaseProbe.ts)
- [useFrontendLogs.ts](file:///workspace/app/encv-mobile/src/composables/useFrontendLogs.ts)（保留 HMR noise 降级）
- [useEventBus.ts](file:///workspace/app/encv-mobile/src/composables/useEventBus.ts)

---

## 七、风险 & 决策

### 风险 R1：HttpPollBackend 实时性比 WS 差（2-5s vs 实时）
- **影响**：自动化测试报告状态更新慢几秒
- **缓解**：BASELINE_INTERVAL_MS = 2s（active 状态）；用户看 task 卡片刷新体验可接受
- **可接受度**：✅ 沙箱预览是开发期调试，实时性要求低于真机

### 风险 R2：后端 GET /api/tasks 单次返回数据量随时间增长
- **影响**：500+ 历史 task 时每次 poll 都拉全量
- **缓解**：短期不优化（沙箱场景 task 数 < 100）；长期可加 `?since=ts` 后端 query param（前置 diff 给前端）
- **可接受度**：✅ 短期 OK，加 TODO 注释

### 风险 R3：transport 模式 election 是一次性的，不会动态切换
- **影响**：用户在沙箱本地 dev 启动 → 选举 ws → 切到 OpenPreview 不会自动改 transport
- **缓解**：符合预期（同一个 session 不会从 127.0.0.1 跳到 trae 域名）；如要支持，需要 visibilitychange 重新 election（**暂不做**，留 TODO）
- **可接受度**：✅ 实际场景不会切 origin

### 决策 D1：保留 useApiBaseProbe 的 isSandboxBrowserOrigin（**不删**）
- **理由**：baseUrl 探测是 transport 之**前**的逻辑，需要独立判断
- **可接受度**：✅ 职责不同，不重复

### 决策 D2：useFrontendLogs 的 HMR noise 降级**保留**（不并入 transport）
- **理由**：vite HMR 客户端是 vite 自己的 WS，跟业务 transport 无关
- **可接受度**：✅ 边界清晰

---

## 八、跨层参考

| 主题 | 文档位置 |
|------|---------|
| 沙箱 OpenPreview 拓扑 | [.trae/rules/preview-management.md §三](file:///workspace/.trae/rules/preview-management.md) |
| WS 协议 + trae 反代不支持 upgrade | [useApiBaseProbe.ts:75-115](file:///workspace/app/encv-mobile/src/composables/useApiBaseProbe.ts) |
| eventBus 4 件套契约 | [useEventBus.ts](file:///workspace/app/encv-mobile/src/composables/useEventBus.ts) |
| 4 件套订阅铁律 | [.trae/rules/automation-workflow.md §二](file:///workspace/.trae/rules/automation-workflow.md) |
| 现有 WS 实现 | [useWebSocket.ts](file:///workspace/app/encv-mobile/src/composables/useWebSocket.ts) |
| 现有 baseUrl 探测 | [useApiBaseProbe.ts](file:///workspace/app/encv-mobile/src/composables/useApiBaseProbe.ts) |
| server:status 抑制逻辑 | [useServerStatus.ts:39-44](file:///workspace/app/encv-mobile/src/composables/useServerStatus.ts) |
| DevLogs.vue 重复 connect | [DevLogs.vue:303-308](file:///workspace/app/encv-mobile/src/views/DevLogs.vue) |
