# 跨进程 IPC 重构 — 实施检查清单

> 每条检查项都是"完成"的明确标准，**不允许模糊判断**。

## 一、Go 端

### 1.1 新增 /api/runtime 端点

- [ ] `internal/server/runtime_api.go` 创建
  - [ ] 定义 `RuntimeInfo` struct（PID, Version, ServingDir, Port, StartedAt, Mobile, ConfigPath, HeartbeatOK）
  - [ ] 实现 `handleRuntimeAPI(w, r)` HTTP handler
  - [ ] 加 `/api/runtime` 路由注册到 `server.go` 的 `Route()` 方法
  - [ ] 字段访问用 `sync.RWMutex` 保护
- [ ] `internal/server/server.go` 修改
  - [ ] `Server` struct 加 `runtimeInfo RuntimeInfo` 字段
  - [ ] `Server` struct 加 `runtimeInfoMu sync.RWMutex` 字段
  - [ ] `Start()` 中填充 `runtimeInfo`（PID、servingDir、port、startedAt、mobile、configPath）
  - [ ] `Start()` 中注册 `/api/runtime` 路由

### 1.2 心跳机制改写：内存中

- [ ] `internal/utils/ffmpeg/worker_client.go` 修改
  - [ ] 删除 `writeHeartbeat()` 函数（写文件版本）
  - [ ] 删除 `touchHeartbeatFile()` 函数
  - [ ] 删除 `ENCV_HEARTBEAT_PATH` env var 引用
  - [ ] 删除 `heartbeatTickInterval` 常量
  - [ ] 删除 `StartHeartbeatLoop()` 函数（或重写为内存版本）
  - [ ] 删除 `ResetHeartbeatLoopForTesting()` 函数
  - [ ] 删除 `startHeartbeatLoopOnce` 变量
  - [ ] 删除 `heartbeatLoops` 变量
  - [ ] 删除 `heartbeatLoopsMu` 变量
  - [ ] 删除 `heartbeatLoop()` goroutine 函数
  - [ ] 删除 `log/slog` import（如果不再用）
- [ ] `internal/server/server.go` 修改
  - [ ] `Server` struct 加 `lastHeartbeatMs atomic.Int64` 字段
  - [ ] `Start()` 中启 `startHeartbeatLoopInMemory(ctx)` goroutine
  - [ ] `startHeartbeatLoopInMemory` 每 2s `atomic.StoreInt64(&lastHeartbeatMs, time.Now().UnixMilli())`
- [ ] `internal/server/server.go` 修改 `/health` 端点
  - [ ] 响应从 `text/plain "ok"` 改为 `application/json`
  - [ ] JSON 包含 `status`, `heartbeat_age_ms`, `heartbeat_ok` 字段
  - [ ] `heartbeat_ok = (heartbeat_age_ms < 30000)`

### 1.3 测试

- [ ] `internal/server/runtime_api_test.go`（新）
  - [ ] TestHandleRuntime_ReturnsValidJSON
  - [ ] TestHandleRuntime_ReflectsServingDir
  - [ ] TestHandleRuntime_HeartbeatOKWithin30s
  - [ ] TestHandleRuntime_HeartbeatStaleReportsFalse
- [ ] `internal/utils/ffmpeg/heartbeat_test.go` 修改
  - [ ] 全部删除（mtime-based 测试不再适用）
- [ ] 跑 `go test ./internal/server/... ./internal/utils/ffmpeg/...` 全过

---

## 二、Kotlin 端

### 2.1 删除项

- [ ] `EncvGoService.kt` 删除
  - [ ] `private val heartbeatFile: File by lazy { resolvedHeartbeatFile() }` 字段
  - [ ] `private fun resolvedHeartbeatFile(): File` 方法（30 行）
  - [ ] `private fun resolveServingDir(configPath: String): String` 方法（30 行）
  - [ ] `private fun readMobileServerDirFromConfig(configPath: String): String?` 方法
  - [ ] `private fun updateMobileServerDirInConfig(configPath: String, newDir: String)` 方法
  - [ ] `private fun probeDirWritable(dirPath: String): Boolean` 方法
  - [ ] `startGoProcess` 中的 `ENCV_HEARTBEAT_PATH` env var
  - [ ] `startGoProcess` 中的 `ENCV_SERVING_DIR` env var
  - [ ] `startGoProcess` 中的 `servingDir = resolveServingDir(configPath)` 调用
  - [ ] `startGoProcess` 中的 `touchHeartbeat()` 调用
  - [ ] `private fun touchHeartbeat()` 方法

### 2.2 保留并简化项

- [ ] `EncvGoService.kt` `startGoProcess()` 保留
  - [ ] `ENCV_CONFIG_PATH` env var
  - [ ] `ENCV_MOBILE=1` env var
  - [ ] `HOME` env var
  - [ ] `ENCV_LIB_DIR` env var
  - [ ] `ENCV_FFMPEG_WORKER` env var
  - [ ] `redirectErrorStream(true)`
  - [ ] `directory(filesDir)`

### 2.3 新增项

- [ ] `EncvGoService.kt` `startProcessAliveMonitor()` 修改
  - [ ] 删除 heartbeatFile 相关代码
  - [ ] 调用 `checkHeartbeatOk(currentPort)` 替代 mtime 检查
  - [ ] hang 判定改用 HTTP 连续失败计数（与现有 restartAttempts 配合）
- [ ] `EncvGoService.kt` 新增 `checkHeartbeatOk(port: Int): Boolean`
  - [ ] `GET http://127.0.0.1:$port/health`
  - [ ] 1s timeout
  - [ ] 解析 JSON 读 `heartbeat_ok` 字段
  - [ ] 异常 catch 返回 false

### 2.4 测试

- [ ] `EncvGoServiceTest.kt`（新）
  - [ ] TestCheckHeartbeatOk_TrueWhen200
  - [ ] TestCheckHeartbeatOk_FalseOnTimeout
  - [ ] TestCheckHeartbeatOk_FalseOnConnectionRefused
  - [ ] TestCheckHeartbeatOk_FalseOnInvalidJSON
- [ ] 跑 `./gradlew :app:compileDebugKotlin` 编译通过

---

## 三、前端（无需改）

- [ ] `app/encv-mobile/src/composables/realtime/WsBackend.ts` — 不变（HTTP/WS 走标准 API）
- [ ] `app/encv-mobile/src/composables/useRealtimeTransport.ts` — 不变
- [ ] `app/preview-gateway/src/children.ts` — 不变（已经 HTTP /health poll）

---

## 四、配置（无需改）

- [ ] `app/encv-mobile/android/app/src/main/assets/config.user.json`
  - [ ] `mobile.server.dir` 保留默认值 `/storage/emulated/0`
  - [ ] **Kotlin 不再修改此字段**

---

## 五、跨平台验收

### 5.1 Android 真机

- [ ] 真机启动 APK → 启动 Go 进程 → 1s 内 ready 广播
- [ ] 真机运行 30 分钟 → WS 不断（无 7s 必死）
- [ ] 真机拒存储权限 → 不影响 IPC（Kotlin 不再 probe）
- [ ] Logcat 显示 `Go heartbeat OK via HTTP :2025/health`

### 5.2 沙箱 dev

- [ ] `pm2 start ecosystem.config.cjs` 启动
- [ ] preview-gateway 启 encv-go → HTTP `/health` 探活 ready
- [ ] 沙箱里 `curl :2025/api/runtime | jq .` 返回 JSON
- [ ] 沙箱里 `curl :2025/health` 返回 JSON 包含 `heartbeat_ok`

### 5.3 Desktop dev

- [ ] Linux `go run ./cmd/encv/ serve` → `curl :2025/health` 返 200 + JSON
- [ ] macOS 同上
- [ ] Windows 同上

### 5.4 CI

- [ ] `go test ./...` 全过
- [ ] `./gradlew :app:assembleDebug` 编译过
- [ ] TypeScript `tsc --noEmit` 0 错误

---

## 六、文档更新

- [ ] `/workspace/.trae/documents/backend-crash-websocket-1006-fix.md` 加"重构后状态"章节
- [ ] `/workspace/.trae/rules/kotlin-android.md` 加"HTTP 探活模式"标准
- [ ] `/workspace/.trae/rules/capacitor.md` 不变（无 Android IPC 相关规则）
- [ ] `/workspace/CLAUDE.md` 或 `/workspace/AGENTS.md` 加"parent ↔ child IPC 铁律"

---

## 七、回归测试

- [ ] 真机 WS 长稳 30 分钟（基线：7s 必死）
- [ ] 真机 ffmpeg 调用 50 次（基线：mtime 误判 hang）
- [ ] 真机拒存储权限（基线：probe 失败导致启动卡住）
- [ ] 沙箱 dev preview 完整流程（vite + encv-go + plugin-openlist）
- [ ] Desktop dev 启动 + ffmpeg 调用
