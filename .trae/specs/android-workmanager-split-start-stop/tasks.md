# Android WorkManager 拆分 / 随时启停 — Tasks

> 详细任务列表，按 Phase 排序。每项任务对应 checklist 中的勾选项。

---

## Phase 1: Go 内核接入 + Cypress 真实后端测试

### Task 1.1.1: kernel Pool 新增 CancelJob + PoolStats 方法
- **文件**：`internal/kernel/pool.go`
- **动作**：
  - 新增 `func (p *Pool) CancelJob(jobID string) bool` — 取消指定 job（标记 + 从队列移除）
  - 新增 `func (p *Pool) Stats() PoolStats` — 返回 `{Name, Workers, QueueSize, QueueDepth, LedgerEnabled, LedgerRoot, LastRestoreCount, LastRestoreAt}`
  - 新增 `func (p *Pool) LastRestoreInfo() (count int, at time.Time)` — 内部记录
- **测试**：`internal/kernel/pool_test.go` 新增 5 个测试（CancelJob 命中/未命中/已 done/已 cancel + Stats 字段完整性）

### Task 1.1.2: TaskManager 接入 kernel Pool
- **文件**：`internal/service/task_manager.go` + `task_manager_worker.go`
- **动作**：
  - `TaskManager` struct 新增字段 `kernelPool *kernel.Pool`
  - 新增方法 `func (tm *TaskManager) UseKernelPool(pool *kernel.Pool)` — 注入 Pool
  - `task_manager_worker.go` 的 `runTask` 入口加分支：`if tm.kernelPool != nil { tm.kernelPool.Submit(ctx, job); return } else { go func() {...}() }`
  - Job 的 `OnDone` 回调调 `tm.markTaskDone(id, result)`
  - `Cancel(id)` 时若 `kernelPool != nil`，调 `kernelPool.CancelJob(jobID)`
- **feature flag**：环境变量 `ENCV_USE_KERNEL_POOL=1` 启用，默认关闭
- **测试**：`internal/service/task_manager_kernel_test.go` 新建，5 个集成测试

### Task 1.2.1: server.Start 调 Pool.Restore
- **文件**：`internal/server/server.go`
- **动作**：
  - `Start()` 方法在路由注册后、监听端口前调 `s.kernelPool.Restore(bootCtx)`
  - 日志 INFO `kernel: pool restored N jobs`
  - Restore 失败 WARN 但不阻塞启动
  - 无 Ledger 时 DEBUG `kernel: pool has no ledger, skip restore`
- **测试**：`internal/server/server_kernel_restore_test.go` 新建，3 个测试

### Task 1.3.1: `/api/kernel/pools` 端点
- **文件**：`internal/server/kernel_api.go` + `routes.go`
- **动作**：
  - 新增 `handleKernelPools(c *gin.Context)` — 返回所有 Pool 状态
  - 路由 `GET /api/kernel/pools`
- **测试**：`internal/server/kernel_api_test.go` 加 2 个测试

### Task 1.4.1: `/api/kernel/restore` 端点（dev only）
- **文件**：`internal/server/kernel_api.go` + `routes.go`
- **动作**：
  - 新增 `handleKernelRestore(c *gin.Context)` — 手动触发 Restore
  - dev 模式校验（isDevMode helper）
  - 非 dev 返回 403
  - 路由 `POST /api/kernel/restore`
- **测试**：`internal/server/kernel_api_test.go` 加 2 个测试（dev 通过 + 非 dev 403）

### Task 1.5.1: Cypress E2E kernel-endpoints.cy.ts
- **文件**：`app/encv-mobile/cypress/e2e/kernel-endpoints.cy.ts`（新建）
- **动作**：
  - 启动真实后端（前置脚本）
  - 测试 `GET /api/kernel/services` 返回 3 个 service
  - 测试 `GET /api/kernel/health` 全绿
  - 测试 `GET /api/kernel/pools` 返回 Pool 状态
  - **禁止** cy.intercept mock
- **验证**：`xvfb-run -a npx cypress run --e2e --spec cypress/e2e/kernel-endpoints.cy.ts`

### Task 1.6.1: Cypress E2E kernel-restart-restore.cy.ts
- **文件**：`app/encv-mobile/cypress/e2e/kernel-restart-restore.cy.ts`（新建）
- **动作**：
  - 启动后端
  - POST `/api/files/search-fulltext/rebuild` 创建 FTS 任务
  - 等 `running` + 进度 30%+
  - POST `/api/dev/kill-backend`（dev only，需新增端点）模拟杀进程
  - 等 2s + 重新启动后端
  - 验证 `Pool.Restore` 日志（GET /api/kernel/pools 看 `lastRestoreCount`）
  - 验证 task 续跑（GET /api/tasks/:id 状态 running，进度从 30%+ → 100%）
  - 验证 cancelled task 不被 Restore 续跑（另起一个 case）
- **依赖**：需新增 `/api/dev/kill-backend` 端点（dev only）
- **验证**：`xvfb-run -a npx cypress run --e2e --spec cypress/e2e/kernel-restart-restore.cy.ts`

### Task 1.6.2: 新增 `/api/dev/kill-backend` 端点（dev only）
- **文件**：`internal/server/dev_api.go`（新建）+ `routes.go`
- **动作**：
  - 新增 `handleKillBackend(c *gin.Context)` — 触发 `os.Exit(1)` 或 `signal.Notify SIGTERM`
  - dev 模式校验
  - 路由 `POST /api/dev/kill-backend`
- **测试**：手动验证（端点会自杀，自动化测试不适用）

---

## Phase 2: Android 高版本硬约束修复

### Task 2.1.1: ComboLite 16 HostService 补 foregroundServiceType
- **文件**：`app/encv-mobile/android/app/src/main/AndroidManifest.xml`
- **动作**：
  - 16 个 `<service android:name="com.encvgo.combolite.proxy.HostServiceN">` 加 `android:foregroundServiceType="specialUse"`
  - 每个加 `<property android:name="android.app.PROPERTY_SPECIAL_USE_FGS_SUBTYPE" android:value="encv-plugin-host" />`
- **验证**：`./gradlew :app:processDebugManifest` 通过

### Task 2.2.1: EncvGoService 评估 + 决策
- **文件**：`app/encv-mobile/android/app/src/main/AndroidManifest.xml`（如需改）
- **动作**：
  - 评估 EncvGoService 是否从 dataSync 改 specialUse
  - 决策：长任务（>6h FTS 重建）需要 specialUse，但短期看 EncvGoService 持续运行，dataSync 6h 上限会触发
  - 改为 specialUse + `<property android:value="encv-go-backend-host" />`
- **验证**：`./gradlew :app:processDebugManifest` 通过

---

## Phase 3: WorkManager 引入 + 随时启停

### Task 3.1.1: 引入 WorkManager 依赖
- **文件**：`app/encv-mobile/android/gradle/libs.versions.toml` + `app/build.gradle.kts`
- **动作**：
  - `libs.versions.toml` 加 `work-runtime-ktx = "2.10.1"`
  - `build.gradle.kts` 加 `implementation(libs.work.runtime.ktx)`
- **验证**：`./gradlew :app:dependencies | grep work-runtime` 看到 2.10.1

### Task 3.2.1: EncvTaskCancelWorker
- **文件**：`app/encv-mobile/android/app/src/main/java/com/encvgo/app/workers/EncvTaskCancelWorker.kt`（新建）
- **动作**：
  - 继承 `CoroutineWorker`
  - `doWork()`：
    1. 从 inputData 取 `taskId`
    2. `/health` 探活（HTTP GET，3s timeout）
    3. 存活 → POST `/api/tasks/:id/cancel` → 200 = `Result.success()` / 4xx = `Result.success()`（task 已不在）
    4. 不存活 → `Result.retry()`
  - 退避：`LinearBackoff(10s)`，max 10 次（在 enqueue 时配置）
- **依赖**：Task 3.1.1

### Task 3.3.1: GoProcessRestartReceiver
- **文件**：`app/encv-mobile/android/app/src/main/java/com/encvgo/app/workers/GoProcessRestartReceiver.kt`（新建）
- **动作**：
  - 继承 `BroadcastReceiver`
  - 监听 `BROADCAST_BACKEND_READY`（在 Manifest 注册或 EncvGoService 发送时注册）
  - `onReceive()`：`WorkManager.getInstance(context).enqueueUniqueWork(...)` 触发所有 pending cancel Worker
- **Manifest**：注册 receiver（`<action android:name="com.encvgo.app.BACKEND_READY" />`）

### Task 3.4.1: useTaskCancel composable
- **文件**：`app/encv-mobile/src/composables/useTaskCancel.ts`（新建）
- **动作**：
  - 导出 `useTaskCancel()` 返回 `{ cancel, cancelByRunId, pollCancelStatus }`
  - `cancel(taskId)`：
    1. HTTP `POST /api/tasks/:id/cancel`（fire-and-forget，不阻塞 UI）
    2. Capacitor `GoProcessPlugin.enqueueCancelWorker({ taskId })`（兜底持久化）
    3. HTTP 成功 → toast `已取消`，HTTP 失败 → toast `已加入取消队列，等后端就绪后重试`
  - `pollCancelStatus(taskId)`：周期查 WorkInfo 状态，UI 显示重试次数

### Task 3.5.1: EncvTaskCancelWorkerTest
- **文件**：`app/encv-mobile/android/app/src/androidTest/java/com/encvgo/app/workers/EncvTaskCancelWorkerTest.kt`（新建）
- **动作**：
  - 真机/模拟器跑（androidTest）
  - 启动真实 Go 后端（`Runtime.exec("libencv-go.so", "start")`）
  - 测试场景 A：Go 存活 → 立即取消 → task 状态 cancelled
  - 测试场景 B：Go 死亡 → enqueue cancel Worker → 重启 Go → Worker 重发 → task cancelled
- **标记**：`@Ignore("Requires real Android device + Go binary")` 默认跳过，手动验证

---

## 任务依赖图

```
Task 1.1.1 (Pool CancelJob + Stats)
  ↓
Task 1.1.2 (TaskManager 接入 Pool)  ←─── feature flag
  ↓
Task 1.2.1 (server.Start Restore)
  ↓
Task 1.3.1 (/api/kernel/pools) ──┐
Task 1.4.1 (/api/kernel/restore)─┤
                                 ↓
Task 1.5.1 (Cypress kernel-endpoints.cy.ts)
  ↓
Task 1.6.2 (/api/dev/kill-backend)
  ↓
Task 1.6.1 (Cypress kernel-restart-restore.cy.ts)

Task 2.1.1 (Manifest HostService type)     ←  独立
Task 2.2.1 (Manifest EncvGoService type)   ←  独立

Task 3.1.1 (WorkManager 依赖)
  ↓
Task 3.2.1 (EncvTaskCancelWorker) ←─ 也可独立写但跑不了
  ↓
Task 3.3.1 (GoProcessRestartReceiver)
  ↓
Task 3.4.1 (useTaskCancel composable)

Task 3.5.1 (androidTest) ←─ 依赖 Task 3.2.1 + 真机
```
