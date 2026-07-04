# 跨进程 IPC 重构 — 实施任务分解

> 每条任务都是"原子"操作，可以独立 PR / commit。
> 任务依赖关系用 [Task X] 显式标注。

---

## Phase 0: 准备（不破坏现有功能）

### Task 0.1: 备份当前实现
- [ ] 提交当前所有改动（带 tag `pre-ipc-refactor`）
- [ ] 创建分支 `refactor/cross-process-ipc`
- [ ] 跑 baseline 测试，记录当前行为

### Task 0.2: 加特性开关
- [ ] `config.user.json` 加 `mobile.ipc.use_http_health_only: false`（默认 false，即用旧机制）
- [ ] Go 端 `internal/config/config.go` 加 `Mobile.IPC.UseHttpHealthOnly bool` 字段
- [ ] 提交

---

## Phase 1: Go 端 `/api/runtime` 端点（独立可测）

### Task 1.1: 定义 RuntimeInfo
- 依赖：[Task 0.2]
- 文件：`internal/server/runtime_api.go`（新）
- 步骤：
  1. 定义 `RuntimeInfo` struct
  2. 定义 `handleRuntimeAPI` 函数签名
- 验收：`go build` 通过
- 提交

### Task 1.2: 注册 /api/runtime 路由
- 依赖：[Task 1.1]
- 文件：`internal/server/server.go`
- 步骤：
  1. `Server.Route()` 中加 `r.GET("/api/runtime", s.handleRuntimeAPI)`
- 验收：启 Go，`curl :2025/api/runtime` 返回 404（handler 还没实现）
- 提交

### Task 1.3: 实现 handleRuntimeAPI
- 依赖：[Task 1.2]
- 文件：`internal/server/runtime_api.go`
- 步骤：
  1. 加 `runtimeInfo` + `runtimeInfoMu` 字段到 `Server` struct
  2. `Start()` 末尾填充 `runtimeInfo`
  3. 实现 `handleRuntimeAPI` 返回 JSON
- 验收：`curl :2025/api/runtime | jq .` 返回完整 JSON
- 提交

### Task 1.4: RuntimeInfo 单元测试
- 依赖：[Task 1.3]
- 文件：`internal/server/runtime_api_test.go`（新）
- 步骤：
  1. TestHandleRuntime_ReturnsValidJSON
  2. TestHandleRuntime_ReflectsServingDir
- 验收：`go test ./internal/server/...` 全过
- 提交

---

## Phase 2: Go 端心跳改写（独立可测）

### Task 2.1: 加 lastHeartbeatMs 原子字段
- 依赖：[Task 0.2]
- 文件：`internal/server/server.go`
- 步骤：
  1. `Server` struct 加 `lastHeartbeatMs atomic.Int64`
  2. 初始化为 `time.Now().UnixMilli()`
- 验收：`go build` 通过
- 提交

### Task 2.2: 实现 startHeartbeatLoopInMemory
- 依赖：[Task 2.1]
- 文件：`internal/server/server.go`
- 步骤：
  1. 实现 `startHeartbeatLoopInMemory(ctx)`，2s tick 写 `lastHeartbeatMs`
  2. `Start()` 中调用
- 验收：Go 启动后 `lastHeartbeatMs` 持续更新
- 提交

### Task 2.3: /health 端点扩展
- 依赖：[Task 2.2]
- 文件：`internal/server/server.go`（`handleHealth` 方法）
- 步骤：
  1. 响应改 JSON
  2. 加 `heartbeat_age_ms`、`heartbeat_ok` 字段
  3. **保持向后兼容**：`status: "ok"` 仍然存在
- 验收：`curl :2025/health | jq .heartbeat_ok` 返回 true
- 提交

### Task 2.4: Heartbeat 单元测试
- 依赖：[Task 2.3]
- 文件：`internal/server/health_test.go`（新）
- 步骤：
  1. TestHealth_HeartbeatOK_WhenFresh
  2. TestHealth_HeartbeatStale_WhenOld
- 验收：测试全过
- 提交

---

## Phase 3: 删除 Go 端旧心跳文件逻辑

### Task 3.1: 删除 ffmpeg.writeHeartbeat 文件版本
- 依赖：[Task 2.4]
- 文件：`internal/utils/ffmpeg/worker_client.go`
- 步骤：
  1. 改 `writeHeartbeat()` 为写内存（通过 `server.GetServer().TouchHeartbeat()`）
  2. 或者直接删除（由 startHeartbeatLoopInMemory 兜底）
- 验收：`go build` 通过
- 提交

### Task 3.2: 删除 ffmpeg.StartHeartbeatLoop 文件版本
- 依赖：[Task 2.2]
- 文件：`internal/utils/ffmpeg/worker_client.go`
- 步骤：
  1. 整个函数体删除（或重写为 noop + deprecation warning）
- 验收：`go build` 通过
- 提交

### Task 3.3: 删除 heartbeat_test.go
- 依赖：[Task 3.2]
- 文件：`internal/utils/ffmpeg/heartbeat_test.go`
- 步骤：
  1. 整个文件删除
- 验收：`go test ./internal/utils/ffmpeg/` 无 heartbeat 相关测试
- 提交

### Task 3.4: 删除 ENCV_HEARTBEAT_PATH env var 引用
- 依赖：[Task 3.3]
- 文件：所有引用 `ENCV_HEARTBEAT_PATH` 的文件
- 步骤：
  1. `grep -r ENCV_HEARTBEAT_PATH internal/`
  2. 删除所有引用
- 验收：`grep -r ENCV_HEARTBEAT_PATH internal/` 0 结果
- 提交

---

## Phase 4: Kotlin 端改造（分多个小 PR）

### Task 4.1: 加 checkHeartbeatOk 函数
- 依赖：[Phase 3 完成]
- 文件：`EncvGoService.kt`
- 步骤：
  1. 新增 `private fun checkHeartbeatOk(port: Int): Boolean`
  2. 实现 HTTP GET + JSON 解析
- 验收：编译通过
- 提交

### Task 4.2: 切换 mtime → HTTP（特性开关控制）
- 依赖：[Task 4.1]
- 文件：`EncvGoService.kt`
- 步骤：
  1. `startProcessAliveMonitor` 加 if 开关
  2. `if (cfg.mobile.ipc.useHttpHealthOnly) { ... } else { 旧 mtime 逻辑 }`
- 验收：开关 true 走 HTTP，false 走旧 mtime
- 提交

### Task 4.3: Kotlin EncvGoServiceTest
- 依赖：[Task 4.1]
- 文件：`EncvGoServiceTest.kt`（新）
- 步骤：
  1. TestCheckHeartbeatOk_TrueWhen200
  2. TestCheckHeartbeatOk_FalseOnTimeout
- 验收：测试全过
- 提交

---

## Phase 5: 删除 Kotlin 端旧逻辑

### Task 5.1: 删除 resolvedHeartbeatFile + heartbeatFile
- 依赖：[Task 4.2 灰度验证完成]
- 文件：`EncvGoService.kt`
- 步骤：
  1. 删除 `private val heartbeatFile: File by lazy` 字段
  2. 删除 `resolvedHeartbeatFile()` 方法
  3. 删 `startGoProcess` 里的 `ENCV_HEARTBEAT_PATH` env
  4. 删 `startGoProcess` 里的 `touchHeartbeat()` 调用
- 验收：编译通过，真机启动 OK
- 提交

### Task 5.2: 删除 resolveServingDir 系列
- 依赖：[Task 5.1]
- 文件：`EncvGoService.kt`
- 步骤：
  1. 删 `resolveServingDir()`、`readMobileServerDirFromConfig()`、`updateMobileServerDirInConfig()`、`probeDirWritable()` 4 个方法
  2. 删 `startGoProcess` 里的 `ENCV_SERVING_DIR` env
  3. 删 `startGoProcess` 里的 `servingDir = resolveServingDir(configPath)` 调用
- 验收：编译通过，真机启动 OK
- 提交

### Task 5.3: 删除 startProcessAliveMonitor 里的 mtime 分支
- 依赖：[Task 5.2]
- 文件：`EncvGoService.kt`
- 步骤：
  1. 删 `if (mtimeMs == 0L)` 分支
  2. 删 `if (ageMs > HEARTBEAT_STALE_MS)` 分支
  3. 改用 `checkHeartbeatOk` 替代
- 验收：编译通过，真机 WS 30 分钟长稳
- 提交

---

## Phase 6: 灰度 + 验证

### Task 6.1: 真机灰度
- 依赖：[Phase 5 完成]
- 步骤：
  1. 真机安装 APK
  2. Logcat 确认 `Go heartbeat OK via HTTP` 日志
  3. 30 分钟长稳测试
- 验收：WS 7s 必死不再复现
- 提交

### Task 6.2: 沙箱 dev 灰度
- 依赖：[Phase 5 完成]
- 步骤：
  1. `pm2 restart` 重启 preview-gateway
  2. `curl :2025/api/runtime | jq .` 验证
  3. 完整 dev 流程跑通
- 验收：preview-gateway 正常启动 encv-go
- 提交

### Task 6.3: Desktop dev 灰度
- 依赖：[Phase 5 完成]
- 步骤：
  1. `go run ./cmd/encv/ serve` 启动
  2. `curl :2025/health | jq .heartbeat_ok` 验证
- 验收：JSON 包含 `heartbeat_ok` 字段
- 提交

### Task 6.4: 删除特性开关
- 依赖：[Task 6.1 + 6.2 + 6.3 全部通过]
- 文件：`config.user.json`、Go config
- 步骤：
  1. 删除 `mobile.ipc.use_http_health_only` 字段
  2. 删除 Go 端对应 if 分支
  3. 删除 Kotlin 端对应 if 分支
- 验收：所有代码路径都走 HTTP-only
- 提交

---

## Phase 7: 文档

### Task 7.1: 更新 AGENTS.md / CLAUDE.md
- 依赖：[Phase 6 完成]
- 步骤：
  1. 加"parent ↔ child IPC 铁律"章节
  2. 引用本文档
- 提交

### Task 7.2: 更新 backend-crash-websocket-1006-fix.md
- 依赖：[Phase 6 完成]
- 步骤：
  1. 加"重构后状态"章节
  2. 标注旧 bug 已被根治
- 提交

---

## Task Dependencies 图

```
Phase 0 (0.1 → 0.2)
   ↓
Phase 1 (1.1 → 1.2 → 1.3 → 1.4)
   ↓                          
Phase 2 (2.1 → 2.2 → 2.3 → 2.4)
   ↓
Phase 3 (3.1 → 3.2 → 3.3 → 3.4)
   ↓
Phase 4 (4.1 → 4.2 → 4.3)  (与 Phase 5 灰度可并行)
   ↓
Phase 5 (5.1 → 5.2 → 5.3)
   ↓
Phase 6 (6.1 + 6.2 + 6.3 并行) → 6.4
   ↓
Phase 7 (7.1 → 7.2)
```

## 总任务数

- Phase 0: 2 任务
- Phase 1: 4 任务
- Phase 2: 4 任务
- Phase 3: 4 任务
- Phase 4: 3 任务
- Phase 5: 3 任务
- Phase 6: 4 任务
- Phase 7: 2 任务
- **总计 26 任务**

## 预估工作量

| Phase | 任务数 | 工作量 |
|-------|--------|--------|
| Phase 0 | 2 | 0.5h |
| Phase 1 | 4 | 2h |
| Phase 2 | 4 | 2h |
| Phase 3 | 4 | 1h |
| Phase 4 | 3 | 2h |
| Phase 5 | 3 | 1.5h |
| Phase 6 | 4 | 2h（含 30min 真机测试）|
| Phase 7 | 2 | 1h |
| **合计** | **26** | **12h** |

## 风险点

| Phase | 风险 | 缓解 |
|-------|------|------|
| Phase 2 | 改 /health 响应格式破坏 preview-gateway 探活 | 保持 `code==200` 语义，加 JSON body 字段 |
| Phase 4 | 灰度开关写错导致双逻辑并存 | 每个 if 分支独立可测 |
| Phase 5 | 删除代码时漏删引用 | `grep -r "heartbeatFile\|resolvedHeartbeat\|resolveServingDir" android/` 验证 |
| Phase 6 | 真机 WebView 拦截 localhost | 已在 Capacitor 配置中 allowlist localhost |
