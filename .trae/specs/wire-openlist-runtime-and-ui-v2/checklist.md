# Checklist

## 阶段 0: 撤回错误实现

- [x] `src/composables/useOpenlistBridge.ts` 已删除
- [x] `GoProcess.ts:322-344` 的 `OpenListFullState` + `getOpenListFullState()` 已删除
- [x] `useEventBus.ts` 暂时不含 `openlist:*` 事件（阶段 3 重新加回）
- [x] `LocalOpenListStatusCard.vue` 暂时回退到 backend API 轮询模式（标 SAT-DBG 标记）
- [x] `vue-tsc --noEmit` 0 error（阶段 0 完成时点）
- [x] `git diff` 不残留错误的 OpenList 桥接代码

## 阶段 1: plugin 侧 — ContentProvider + 真实 Bridge

- [x] `OpenListBridge.kt`: `synchronized(lock)` 块保护所有 snapshot 字段
- [x] `OpenListBridge.kt`: PID 来自 `openlistlib.start()` 返回后 `Process.myPid()` 赋值（受限于 openlistlib 未暴露 getPid，未来需替换）
- [x] `OpenListBridge.kt`: `dataSizeBytes` 在 `onLog` 收到 `data_size=<bytes>` 时更新
- [x] `OpenListBridge.kt`: `lastError` 在 `onStartError` / `onProcessExit` / `onShutdown` 时写入
- [x] `OpenListStatusProvider.kt` 文件存在，extends `ContentProvider`
- [x] `OpenListStatusProvider.kt`: `query(status URI)` 返回正确列名的 MatrixCursor
- [x] `OpenListStatusProvider.kt`: `insert(control URI)` 接受 `action=start|stop|force_db_sync|set_admin_password` 并分派
- [x] `plugin-openlist/src/main/AndroidManifest.xml`: `<provider android:authorities="com.encvgo.plugin.openlist.provider" android:exported="true" android:grantUriPermissions="true">`
- [x] `OpenListPluginEntry.onLoad`: 不自动 start（避免阻塞 PluginManager 加载）
- [x] 全链路 SAT-DBG Kotlin 日志覆盖 Service 生命周期、Provider query/insert、Bridge 各方法
- [x] `./gradlew :plugin-openlist:compileDebugKotlin` 通过

## 阶段 2: 宿主侧 — combolite-host OpenListStatusBridge

- [x] `combolite-host` 模块新增 `OpenListStatusBridge.kt`
- [x] `OpenListRuntime` data class 含全部 7 字段（isInstalled/running/port/pid/dataSizeBytes/lastError/lastUpdateTs）
- [x] `OpenListStatusBridge.read()` 调 `ContentResolver.query` 正确处理 null cursor / IllegalArgumentException
- [x] `OpenListStatusBridge.control()` 调 `ContentResolver.insert`
- [x] GoProcess Capacitor plugin 新增 `@PluginMethod fun getOpenListRuntime()` 返回 JSObject
- [x] GoProcess Capacitor plugin 新增 `@PluginMethod fun controlOpenList(action)` 返回 JSObject
- [x] `combolite-host/build.gradle.kts` 改为 `implementation(project(":plugin-openlist"))`（compileOnly 会在 runtime 抛 NoClassDefFoundError 因 STATUS_URI 字段访问）
- [x] SAT-DBG 日志在 host bridge 路径

## 阶段 3: TS 端

- [x] `GoProcess.ts`: `OpenListRuntime` interface 定义完整（含 isInstalled）
- [x] `GoProcess.ts`: `getOpenListRuntime()` 函数 export
- [x] `GoProcess.ts`: `controlOpenList()` 函数 export
- [x] `src/composables/useOpenListBridge.ts` 重写：3 秒轮询 + eventBus emit + SAT-DBG
- [x] `useEventBus.ts`: `openlist:status` / `openlist:log` / `openlist:error` 三个 typed 事件（含 isInstalled 字段）
- [x] `LocalOpenListStatusCard.vue`: 4 卡片变体（running/port_conflict/crash_loop/not_installed）+ eventBus 订阅 + useOpenListBridge 调用
- [x] `vue-tsc --noEmit` 0 error

## 阶段 4: 集成验证

- [ ] 宿主 APK + OpenList APK 都在设备上（需要 CI 走完一遍）
- [ ] 宿主侧 `getOpenListRuntime()` 返回真实 port/pid
- [ ] 宿主侧 `action=start` 触发 OpenList 启动 + 卡片转 running
- [ ] SAT-DBG 日志在 DevLogs 红色高亮区可见（搜索 `[SAT-DBG][OpenList]`）
- [ ] 4 种卡片在对应状态下正确显示
- [ ] port_conflict、crash_loop 等异常状态可触发并显示对应红色/橙色卡片