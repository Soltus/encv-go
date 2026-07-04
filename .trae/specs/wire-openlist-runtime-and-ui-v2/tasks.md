# Tasks

## 阶段 0: 撤回错误实现

- [x] T0.1 删除 `src/composables/useOpenListBridge.ts`（基于错误假设的轮询桥）
- [x] T0.2 删除 `GoProcess.ts:322-344` 的 `OpenListFullState` interface + `getOpenListFullState()` 函数
- [x] T0.3 修改 `LocalOpenListStatusCard.vue` — 暂时回退到上上轮的"HTTP 轮询 backend API"模式（因为新桥还没建好），加 SAT-DBG 标记
- [x] T0.4 删除 `useEventBus.ts` 中误加的 3 个 `openlist:*` 事件类型（等阶段 2 用真实的再补）
  - **Validation**: `vue-tsc --noEmit` 通过，0 type error；`git diff` 干净

## 阶段 1: plugin 侧 — ContentProvider + 真实 Bridge

- [x] T1.1 修改 `OpenListBridge.kt` 维持单例（`object`），新增 `snapshot()` 返回 `Bundle`（含 running/port/pid/dataSizeBytes/lastError/lastUpdateTs），所有字段读写在 `synchronized(lock)` 块
- [x] T1.2 改写 `OpenListBridge` 让 PID 来自 `openlistlib.start()` 返回值；`dataSizeBytes` 在 `onLog` 收到 `data_size=<bytes>` 日志时更新
- [x] T1.3 `OpenListBridge.onStartError(type, msg)` 和 `onProcessExit(exitCode)` 写入 `lastError` 字段
- [x] T1.4 新建 `OpenListStatusProvider.kt`，继承 `ContentProvider`：
  - [x] `onCreate()` 调 `OpenListBridge.init(context)` 一次
  - [x] `query()` URI=`status` → 返回 `MatrixCursor(running, port, pid, data_size_bytes, last_error, last_update_ts)`
  - [x] `insert()` URI=`control` → 读 `action` 字段分派到 `OpenListBridge.start/stop/forceDbSync/setAdminPassword`
  - [x] SAT-DBG 日志
- [x] T1.5 修改 `plugin-openlist/src/main/AndroidManifest.xml` 加 `<provider>` 声明，authorities=`com.encvgo.plugin.openlist.provider`，`exported=true`，`grantUriPermissions=true`
- [x] T1.6 修改 `OpenListPluginEntry.onLoad()`：调 `OpenListBridge.init(context)`，**不**自动 start（避免阻塞 PluginManager 加载），SAT-DBG 全程
- [x] T1.7 保留全链路 SAT-DBG 日志（Kotlin 层 `Log.e("[SAT-DBG][OpenList]...")`）覆盖 Service 生命周期、Provider query/insert、Bridge 各方法
  - **Validation**: `./gradlew :plugin-openlist:compileDebugKotlin` 通过；apk 反编译可见 `OpenListStatusProvider` 类

## 阶段 2: 宿主侧 — combolite-host OpenListStatusBridge

- [x] T2.1 在 `combolite-host` 模块新建 `OpenListStatusBridge.kt`：
  - [x] `data class OpenListRuntime(...)` 含 isInstalled/running/port/pid/dataSizeBytes/lastError/lastUpdateTs
  - [x] `fun read(context: Context): OpenListRuntime` 调 `ContentResolver.query("content://com.encvgo.plugin.openlist.provider/status")`，捕获 `IllegalArgumentException`/`null cursor` → 返回 `NotInstalled`
  - [x] `fun control(context: Context, action: String, args: Map<String,Any>)` 调 `ContentResolver.insert`
  - [x] SAT-DBG 日志
- [x] T2.2 修改 `GoProcessPlugin.kt`（在 app 模块）：新增 `@PluginMethod fun getOpenListRuntime()` → 调 `OpenListStatusBridge.read`，返回 `JSObject` JSON
  - **Validation**: TS 端可调用 `Capacitor.Plugins.GoProcess.getOpenListRuntime()`

## 阶段 3: TS 端 — 真实 runtime 接口

- [x] T3.1 `GoProcess.ts`: 新增 `OpenListRuntime` interface + `getOpenListRuntime()` async function
  - `running, port, pid, dataSizeBytes, lastError, lastUpdateTs, isInstalled`
- [x] T3.2 新建 `src/composables/useOpenListBridge.ts`（**重写**），结构：
  - [x] `onMounted`: 调 `getOpenListRuntime()` 一次 + 每 3 秒 setInterval
  - [x] 每次结果 `eventBus.emit('openlist:status', ...)`
  - [x] 错误 `eventBus.emit('openlist:error', ...)`
  - [x] SAT-DBG 日志
  - [x] `onUnmounted`: clearInterval
- [x] T3.3 `useEventBus.ts` 重新加 3 个 typed 事件 `openlist:status` / `openlist:log` / `openlist:error`
- [x] T3.4 `LocalOpenListStatusCard.vue` 切回 eventBus 模式，保留 4 种卡片变体（running/port_conflict/crash_loop/not_installed）
  - **Validation**: `vue-tsc --noEmit` 通过；dev 跑起来验证轮询能拿到 isInstalled=true 之后能拿 running/port

## 阶段 4: 集成验证

- [ ] T4.1 在模拟器/真机装宿主 APK + OpenList APK
- [ ] T4.2 验证 ContentProvider query 链路：宿主侧 `getOpenListRuntime()` 返回真实 port/pid
- [ ] T4.3 验证 ContentProvider insert 链路：宿主侧触发 `action=start` → OpenList 进程起 → 前端卡片转 running
- [ ] T4.4 验证 SAT-DBG 日志在 DevLogs 红色高亮区可见
- [ ] T4.5 验证 4 种卡片变体在对应状态下正确显示

# Task Dependencies
- 阶段 0 必须先完成（删除错误实现）
- 阶段 1 独立（Kotlin 改动）
- 阶段 2 依赖 阶段 1（host bridge 需要 plugin 侧 provider 存在）
- 阶段 3 依赖 阶段 2（TS 需要真实数据源）
- 阶段 4 依赖 阶段 1+2+3 全部完成