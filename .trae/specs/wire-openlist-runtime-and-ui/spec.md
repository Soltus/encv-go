# OpenList Runtime Wiring & Developer Diagnostics Spec

## Why
OpenList 扩展的构建骨架（AAR 编译、aar2apk 打包、CI 流水线）已全部就绪，但运行时存在一个空洞：OpenListBridge.kt 全是 TODO placeholder，不做任何真实的 openlistlib 调用。前端虽已有 `LocalOpenListStatusCard.vue` 和 `ExtensionsPage.vue` 的 OpenList 入口，但所有状态都走 HTTP API（需要 Go 后端转发），缺乏从原生层直接桥接的实时事件通道。运行时一旦出错，开发者只有 Android logcat，前端无感知。需要通过真实桥接 + 饱和调试日志 + 开发者友好错误 UI 三层防御实现"让所有运行时错误无所遁形"。

## What Changes
- **OpenListBridge.kt** — 替换全部 placeholder stub，接入真实 openlistlib（gomobile bind 输出的 openlistlib.Openlistlib / Event / LogCallback）
- **OpenListPluginEntry.kt** — onLoad 桥接 Koin DI 注入 real OpenListBridge 单例
- **饱和度调试日志** — 全链路加 `Log.e(TAG, "[SAT-DBG] ...")`：线程池创建、startupSequence 每步、Bridge 每个回调、Broadcast 发送前后
- **前端事件总线** — 新增 `openlist:status` / `openlist:log` / `openlist:error` 三个 typed 事件，与 WebSocket `log:message` 并列
- **LocalOpenListStatusCard.vue** — 3 种 error state 升级：not_installed / port_conflict / crash_loop → 各自对应独立 UI 卡片 + 一键诊断按钮
- **ExtensionsPage.vue** — OpenList 卡片增加实时状态指示器（dot + status text），不依赖轮询而是订阅 eventBus
- **Capacitor bridge** — 新增 `openlist:status` → eventBus 的转发通道，让原生 Broadcast 实时进入 Vue 层
- **GoProcess.ts** — 新增 `getOpenListFullState()` typed 接口

## Impact
- Affected specs: integrate-openlist-as-combolite-plugin（R3 扩展实现）
- Affected code: `plugin-openlist/` (Kotlin), `src/composables/useEventBus.ts`, `src/components/LocalOpenListStatusCard.vue`, `src/views/ExtensionsPage.vue`, `src/plugins/GoProcess.ts`

## ADDED Requirements

### Requirement: Real openlistlib Bridge
OpenListBridge SHALL replace all TODO placeholder stubs with real `openlistlib.Openlistlib` calls from the gomobile-bind AAR. The Bridge SHALL implement `openlistlib.Event` and `openlistlib.LogCallback` interfaces directly (no local stubs).

#### Scenario: Bridge initialization
- **WHEN** `OpenListBridge.init(context)` is called
- **THEN** `Openlistlib.setConfigData(dataDir)` is invoked with the configured data directory
- **AND** the port from `OpenListConfig` is applied via `Openlistlib.setPort(port)`

#### Scenario: Bridge start
- **WHEN** `OpenListBridge.start()` is called
- **THEN** `Openlistlib.start()` is invoked on a dedicated background thread
- **AND** on success, `isRunning` is set to true and `broadcastStatus(port, true)` is emitted

#### Scenario: Bridge shutdown with progress timeout
- **WHEN** `OpenListBridge.shutdown(timeoutMs)` is called
- **THEN** `Openlistlib.shutdown(timeoutMs.toInt())` is invoked
- **AND** a 500ms grace timer is started; if the native process hasn't exited by then, a force-kill signal is sent

#### Scenario: Bridge admin password
- **WHEN** `OpenListBridge.setAdminPassword(pwd)` is called with non-empty string
- **THEN** `Openlistlib.setAdminPassword(pwd)` is invoked

#### Scenario: Bridge force DB sync
- **WHEN** `OpenListBridge.forceDbSync()` is called
- **THEN** `Openlistlib.forceDBSync()` is invoked and any exception is caught + logged to [SAT-DBG]

### Requirement: Saturation Debug Logging (SAT-DBG)
Every significant runtime event in the OpenList plugin SHALL emit a debug log at `Log.e` (error level) with the prefix `[SAT-DBG][OpenList]` so it appears as a red-highlighted entry in DevLogs. This applies to:

#### Scenario: Service lifecycle logging
- **WHEN** `OpenListService.onCreate()` / `onStartCommand()` / `onDestroy()` fires
- **THEN** a `[SAT-DBG][OpenList]` log is emitted with the event name, timestamp, and thread name

#### Scenario: Startup sequence step logging
- **WHEN** `startupSequence()` executes each step (port check, config load, bridge init, bridge start)
- **THEN** a `[SAT-DBG][OpenList]` log is emitted BEFORE and AFTER each step, including elapsed ms

#### Scenario: Bridge callback logging
- **WHEN** `onShutdown()` / `onStartError()` / `onProcessExit()` / `onLog()` is invoked by the native Go layer
- **THEN** a `[SAT-DBG][OpenList]` log is emitted with the full callback payload

#### Scenario: Broadcast emission logging
- **WHEN** a `LocalBroadcastManager.sendBroadcast()` is called for any OpenList event
- **THEN** a `[SAT-DBG][OpenList]` log is emitted with the intent action and extras

#### Scenario: Port conflict detection logging
- **WHEN** `isPortOccupied()` returns true
- **THEN** a `[SAT-DBG][OpenList]` log is emitted with the conflicting port and socket connect elapsed ms

### Requirement: Developer-Friendly Error UI
The front-end SHALL render distinct error states for OpenList rather than generic "something went wrong" messages.

#### Scenario: Plugin not installed
- **WHEN** the Capacitor plugin reports OpenList as `not_installed`
- **THEN** `LocalOpenListStatusCard` shows a gray card with:
  - Title: "本地 OpenList 未安装"
  - Description: "前往扩展管理页面安装 OpenList 扩展"
  - Action button: "前往扩展管理" → navigates to ExtensionsPage

#### Scenario: Port conflict
- **WHEN** the service broadcasts `BROADCAST_PORT_CONFLICT` with port N
- **THEN** `LocalOpenListStatusCard` shows an orange card with:
  - Title: "端口 {N} 被占用"
  - Description: "请修改 OpenList 端口或关闭占用该端口的程序"
  - Action button: "修改端口" → navigates to Settings > Plugin Settings > OpenList

#### Scenario: Startup crash (loop detection)
- **WHEN** `BROADCAST_STATUS_CHANGED` transitions running→stopped→running→stopped within 10 seconds (3+ cycles)
- **THEN** `LocalOpenListStatusCard` shows a red card with:
  - Title: "OpenList 反复崩溃"
  - Description: "过去 10 秒内 OpenList 进程反复重启 {count} 次，请查看诊断日志"
  - Action button: "查看诊断日志" → navigates to DevLogs filtered by "OpenList"

#### Scenario: Generic runtime error
- **WHEN** `onStartError(type, msg)` is invoked with unrecognized error type
- **THEN** `LocalOpenListStatusCard` shows a red card with:
  - Title: "OpenList 启动失败"
  - Description: error type + first 120 chars of message
  - Action button: "重试启动" → calls `ensurePluginLoaded('openlist')`

### Requirement: Capacitor Bridge for OpenList Status
The Capacitor `GoProcess` plugin interface SHALL expose a method to query OpenList-specific status without going through the Go backend HTTP API.

#### Scenario: Query full OpenList state
- **WHEN** `getOpenListFullState()` is called from TypeScript
- **THEN** the native GoProcess plugin returns `{ running: boolean, port: number, pid: number, dataSizeBytes: number, lastStartError: string | null }`

#### Scenario: Event forwarding from Android Broadcast to Vue eventBus
- **WHEN** the Android OpenListService sends a LocalBroadcast with action `BROADCAST_STATUS_CHANGED` / `BROADCAST_PORT_CONFLICT` / `BROADCAST_PROCESS_EXIT`
- **THEN** the Capacitor GoProcess plugin intercepts it (via a BroadcastReceiver registered in onLoad) and forwards it to the web layer via `notifyListeners('openlist:status', payload)`

### Requirement: EventBus Extension for OpenList
The `useEventBus.ts` type definitions SHALL include OpenList-specific events alongside existing `task:*` and `server:*` events.

#### Scenario: OpenList status change event
- **WHEN** the Capacitor layer sends `openlist:status`
- **THEN** `EncvEvents['openlist:status']` is typed as `{ running: boolean; port: number; pid: number; dataSizeBytes: number }`
- **AND** `LocalOpenListStatusCard` and `ExtensionsPage` subscribe to it reactively

#### Scenario: OpenList log event
- **WHEN** the Capacitor layer sends `openlist:log`
- **THEN** `EncvEvents['openlist:log']` is typed as `{ level: number; message: string; timestamp: number }`
- **AND** subscribers can forward it to `DevLogs` for display alongside WebSocket `log:message` events

#### Scenario: OpenList error event
- **WHEN** the Capacitor layer sends `openlist:error`
- **THEN** `EncvEvents['openlist:error']` is typed as `{ type: string; message: string; code?: number }`
- **AND** the error UI layer (toast / card) picks it up reactively

## MODIFIED Requirements

### Requirement: OpenListPluginEntry onLoad (was: placeholder start service)
The `onLoad()` method SHALL do more than just start a foreground service:
1. Register a `BroadcastReceiver` for OpenList service broadcasts
2. Create a Koin singleton binding for `OpenListBridge` (so the Capacitor plugin can call bridge methods)
3. Forward `BROADCAST_STATUS_CHANGED` / `BROADCAST_PORT_CONFLICT` / `BROADCAST_PROCESS_EXIT` to the Capacitor plugin's `notifyListeners` bridge
4. Log every step with `[SAT-DBG][OpenList]`

### Requirement: OpenListBridge as Koin Singleton
OpenListBridge SHALL be registered as a Koin `single` in `OpenListPluginEntry.pluginModule` to allow the Capacitor GoProcess plugin to call `OpenListBridge.isRunning()` / `OpenListBridge.forceDbSync()` / etc. from outside the service context.