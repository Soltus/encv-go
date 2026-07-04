# OpenList 架构正名 + 运行时集成（combolite 扩展，非 Capacitor 插件）

## Why
OpenList 是 **combolite 扩展**（独立 APK，由 aar2apk 把 aar 转成 apk，作为独立软件包安装到 Android）。其与宿主 encv-go app 的关系是：
- 宿主通过 `com.combo.core.runtime.PluginManager`（combolite-core 2.x）管理插件安装/启用/加载
- 扩展内 `OpenListPluginEntry : IPluginEntryClass` 是 combolite 框架识别的入口
- 扩展内 `OpenListService` 是**独立进程的前台 Service**（aar2apk 把 `applicationId` 改成 `com.encvgo.plugin.openlist`，所以 service 在独立进程下运行）
- 宿主通过 `BaseHostActivity` 跨进程调起扩展的 activity（proxy intent 机制）

**之前 spec 的根本性错误**：
1. 把 OpenList 当成 Capacitor 插件 → 误用 `GoProcess.getPluginFullState({ pluginId })` 等 Capacitor 注册的方法
2. 把 `useOpenListBridge.ts` 写成 3 秒轮询 `getOpenListFullState` ——但 `getPluginFullState` 返回的 `status: "ready"` 只代表"插件已加载进 PluginManager"，**不代表 OpenList 进程在跑**。PluginManager 加载完 OpenList 的 APK 后，仅触发 `IPluginEntryClass.onLoad(context)`，具体 OpenList 进程的启停是 plugin 内部的事
3. 把"port 冲突""进程崩溃"等状态当成能从 Capacitor 一侧读到——这些信息**只在 plugin APK 自己的进程内**

**真实架构**（待本 spec 实现）：
- OpenList 进程内：`OpenListBridge` 通过 `openlistlib`（gomobile bind）拿到 PID/port/heartbeat
- 跨进程通信：plugin APK → 宿主 APK。要么走 binder/aidl，要么走最务实的 `LocalBroadcastManager`（Android 文档明确支持跨进程本地广播），要么走 ContentProvider
- 选 **ContentProvider** 路线最干净：plugin 暴露 `ContentProvider`，宿主通过 `ContentResolver.query()` 读状态，`ContentResolver.insert()` 写命令。零权限/零依赖/同步易用

## What Changes
- **撤掉** `src/composables/useOpenListBridge.ts`（基于错误假设的轮询桥）
- **撤掉** `GoProcess.ts` 里的 `OpenListFullState` / `getOpenListFullState()` 错误桥接
- **OpenListBridge.kt** 改造为同时是查询端（被宿主 ContentProvider 调）和事件源（向宿主广播）
- **新增** `OpenListStatusProvider`（ContentProvider）让宿主用 URI `content://com.encvgo.plugin.openlist.provider/status` 读 JSON
- **combolite-host 模块新增** `OpenListStatusBridge`（封装 ContentResolver 调用），暴露 Kotlin API 给 GoProcess 插件
- **GoProcess 插件新增** 真正的 `getOpenListRuntime()` Capacitor 方法（不是 `getPluginFullState` 误用），走 content provider 读 OpenList 真实运行时状态
- **TS 端新增** `getOpenListRuntime()` typed 函数（替代错误版 `getOpenListFullState`）
- **饱和度调试日志** 保留，跨 ContentProvider 边界两侧都打

## Impact
- Affected specs: wire-openlist-runtime-and-ui（上轮产物需要全删/全改）、integrate-openlist-as-combolite-plugin
- Affected code:
  - **删除**: `src/composables/useOpenListBridge.ts`, `GoProcess.ts:322-344`
  - **修改**: `src/components/LocalOpenListStatusCard.vue`（从 eventBus 模式改回"调 `getOpenListRuntime()` + 定时刷新"，因为 OpenList 进程独立运行宿主侧读不到实时事件，只能轮询 content provider；这才是独立 APK 扩展的真实约束）
  - **修改**: `src/composables/useEventBus.ts`（保留 typed event 但只有 `openlist:log` 用于宿主动态化注入）
  - **修改**: `src/views/ExtensionsPage.vue`（已有 combolite flow 走的是对的，不要动）
  - **修改**: plugin-openlist Kotlin 全套（Service/Bridge/Provider/Entry）

## ADDED Requirements

### Requirement: ContentProvider as cross-process IPC
The OpenList extension SHALL expose a `ContentProvider` at authority `com.encvgo.plugin.openlist.provider` (declared in its AndroidManifest) with:
- URI `content://com.encvgo.plugin.openlist.provider/status` → `query()` returns a `MatrixCursor` with one row containing columns `running (int)`, `port (int)`, `pid (int)`, `data_size_bytes (long)`, `last_error (string)`, `last_update_ts (long)`
- URI `content://com.encvgo.plugin.openlist.provider/control` → `insert()` accepts a `ContentValues` with `action` key (`start` | `stop` | `force_db_sync` | `set_admin_password`) and dispatches to OpenListBridge

#### Scenario: Host reads OpenList status
- **WHEN** host app calls `ContentResolver.query("content://com.encvgo.plugin.openlist.provider/status", null, null, null, null)` from its own process
- **THEN** the OpenList provider returns a `MatrixCursor` with the current `running/port/pid/data_size_bytes/last_error/last_update_ts` snapshot
- **AND** if the OpenList process is not running, all values are 0/empty and `last_error` says "openlist process not running"

#### Scenario: Host triggers OpenList start
- **WHEN** host calls `ContentResolver.insert("content://com.encvgo.plugin.openlist.provider/control", ContentValues("action" -> "start"))`
- **THEN** the OpenList provider's `insert()` invokes `OpenListBridge.start()` on the plugin side
- **AND** returns a Uri like `content://com.encvgo.plugin.openlist.provider/control/result/<txnId>`

#### Scenario: Provider declared in plugin AndroidManifest
- **WHEN** the OpenList APK is installed and Android resolves its `AndroidManifest.xml`
- **THEN** the manifest contains a `<provider>` element with `android:authorities="com.encvgo.plugin.openlist.provider"`, `android:exported="true"`, `android:grantUriPermissions="true"`
- **AND** the provider class is `com.encvgo.plugin.openlist.OpenListStatusProvider`

### Requirement: OpenListBridge becomes the data source
`OpenListBridge` (Kotlin object in plugin-openlist) SHALL be the single source of truth for OpenList runtime state, queried by `OpenListStatusProvider`. It SHALL:
- Track `running: Boolean`, `port: Int`, `pid: Int`, `dataSizeBytes: Long`, `lastError: String?`, `lastUpdateTs: Long`
- Update `pid` from the native Go process after `Openlistlib.start()` returns
- Update `dataSizeBytes` on every `onLog` tick with level=INFO and a `data_size=` prefix
- Set `lastError` whenever `onStartError(type, msg)` or `onProcessExit(exitCode)` fires

#### Scenario: Bridge exposes snapshot via synchronized getter
- **WHEN** the ContentProvider's `query()` calls `OpenListBridge.snapshot()`
- **THEN** it returns a `Bundle` with `running, port, pid, dataSizeBytes, lastError, lastUpdateTs` keys, all read under a single `synchronized(lock)` block

### Requirement: Combolite-host OpenList bridge (host side)
`combolite-host` module SHALL add `OpenListStatusBridge` Kotlin object that:
- Wraps `ContentResolver` calls to `content://com.encvgo.plugin.openlist.provider/{status,control}`
- Returns `OpenListRuntime` data class (typed)
- Has safe fallback when provider is unavailable (returns `OpenListRuntime.NotInstalled`)

#### Scenario: Host GoProcess plugin reads runtime
- **WHEN** the Capacitor `getOpenListRuntime()` method is called from TypeScript
- **THEN** `GoProcessPlugin` Kotlin implementation calls `OpenListStatusBridge.read()` which performs the ContentResolver query
- **AND** returns a JSON object `{running, port, pid, dataSizeBytes, lastError, lastUpdateTs, isInstalled}`

#### Scenario: Host unavailable provider
- **WHEN** the OpenList extension is not installed (no provider in PackageManager)
- **THEN** `OpenListStatusBridge.read()` catches `IllegalArgumentException` from `ContentResolver.query()` returning null
- **AND** returns `OpenListRuntime.NotInstalled`

### Requirement: TS API correction
`GoProcess.ts` SHALL replace the previous erroneous `getOpenListFullState()` with a new `getOpenListRuntime()` function that:
- Calls the new Capacitor `getOpenListRuntime()` (not the old `getPluginFullState`)
- Returns `OpenListRuntime` interface `{running, port, pid, dataSizeBytes, lastError, lastUpdateTs, isInstalled}`

#### Scenario: `useOpenListBridge` rewritten
- **WHEN** the corrected composable runs
- **THEN** it polls `getOpenListRuntime()` every 3 seconds (acceptable: cross-process query is fast <10ms)
- **AND** emits `eventBus.emit('openlist:status', ...)` to the existing typed event
- **AND** the previous `useOpenListBridge.ts` is deleted and replaced

### Requirement: OpenList runtime error UI (preserved from previous spec)
The 4-card-variant `LocalOpenListStatusCard.vue` is preserved. The mapping is corrected:

| status | card |
|--------|------|
| `isInstalled: false` | not_installed (gray) |
| `running: true` | running (green) |
| `port: 0, lastError: contains "port"` | port_conflict (orange) |
| `running: false, snapshot within 10s after running: true` ≥3 times | crash_loop (red) |
| `running: false, lastError: "openlist process not running"` | stopped (gray) |

### Requirement: SAT-DBG logging on cross-process boundary
Every ContentProvider call (host→plugin) and every event reply (plugin→host) SHALL log:
- `[SAT-DBG][OpenList][Provider]` on plugin side (ContentProvider.query/insert)
- `[SAT-DBG][OpenList][HostBridge]` on host side (OpenListStatusBridge.read/control)
- `[SAT-DBG][OpenList][Capacitor]` on GoProcess plugin Kotlin side
- `[SAT-DBG][OpenList][Frontend]` on TS side

## MODIFIED Requirements

### Requirement: OpenListPluginEntry onLoad (corrected)
`onLoad(context)` SHALL additionally register the `OpenListStatusProvider` as part of `com.combo.core.runtime.PluginManager` lifecycle. The provider is auto-registered by AndroidManifest declaration, but onLoad SHALL:
1. Call `OpenListBridge.init(context)`
2. NOT auto-start OpenList (was a previous mistake — auto-start blocks plugin load)
3. Log SAT-DBG with step details

### Requirement: LocalOpenListStatusCard.vue (corrected data flow)
The card subscribes to eventBus events **AND** directly invokes `getOpenListRuntime()` on mount + on eventBus tick. Polling is acceptable (3s) because the IPC is local and fast.

## REMOVED Requirements

### Removed: Previous `useOpenListBridge.ts` polling `getPluginFullState`
**Reason**: `getPluginFullState` returns combolite `PluginFullState` (status: `ready`/`not_installed`/etc.) which says nothing about OpenList process runtime. The `port/pid/dataSizeBytes` are **fabricated** (always 0 from `getPluginFullState` because combolite-host doesn't expose those — they live inside the OpenList plugin process).
**Migration**: Use new `getOpenListRuntime()` that goes through ContentProvider.

### Removed: Previous `OpenListFullState` interface in GoProcess.ts
**Reason**: Misleading — claimed `port`/`pid`/`dataSizeBytes` fields but never populated correctly.
**Migration**: New `OpenListRuntime` interface with `isInstalled` flag added.

### Removed: Previous `getOpenListFullState()` function
**Reason**: Calls wrong Capacitor method, returns misleading data.
**Migration**: New `getOpenListRuntime()`.

### Removed: Previous claim that `OpenListService` can forward broadcasts to host
**Reason**: LocalBroadcastManager does NOT cross processes. Plugin APK runs in its own process; its broadcasts are invisible to the host APK. The `OpenListPluginEntry` `BroadcastReceiver` registered in onLoad fires in the plugin process, not the host process. The spec author confused this with the encv-go backend (which is in-process, all broadcasts stay within the same process).
**Migration**: ContentProvider replaces broadcast-based cross-process IPC.

# Spec Doc Diff vs Previous (`wire-openlist-runtime-and-ui/spec.md`)

| Previous (wrong) | Corrected (this spec) |
|------------------|----------------------|
| OpenList is a Capacitor plugin | OpenList is a combolite extension (independent APK) |
| `GoProcess.getPluginFullState()` returns runtime | `getPluginFullState` returns combolite lifecycle state, not process state |
| LocalBroadcastManager from plugin → host works | Broadcasts do not cross processes; this is impossible |
| 3s polling of plugin full state sufficient | Need cross-process IPC → ContentProvider is the answer |
| `OpenListService` runs in host process | `OpenListService` runs in plugin APK's own process (aar2apk sets `applicationId` → separate process) |
| `OpenListBridge` reads `dataSizeBytes` from polling | `dataSizeBytes` is reported by the Go process via `onLog` callback |