# Android WorkManager 拆分 / 随时启停 — Checklist

> 跟踪 spec 各 Phase 完成情况。每完成一项打勾。

## Phase 1: Go 内核接入 + Cypress 真实后端测试

### 1.1 TaskManager 接入 kernel Pool
- [ ] `task_manager.go` 新增 `kernelPool *kernel.Pool` 字段 + `useKernelPool(pool)` 方法
- [ ] `task_manager_worker.go` Submit 路径双轨：if `kernelPool != nil` → `pool.Submit`，否则原 `go func`
- [ ] kernel Pool 新增 `CancelJob(jobID)` 方法（用于 task cancel）
- [ ] kernel Pool 新增 `Job.OnDone` 回调链路：完成后调 `taskManager.markDone(id, result)`
- [ ] feature flag：`ENCV_USE_KERNEL_POOL=1` 启用，默认关闭

### 1.2 server 启动时调 Pool.Restore
- [ ] `server.go` Start() 加 `pool.Restore(bootCtx)` 调用
- [ ] 日志 INFO `kernel: pool restored N jobs`
- [ ] Restore 失败不阻塞启动，WARN 日志
- [ ] 无 Ledger 时跳过 Restore（DEBUG 日志）

### 1.3 `/api/kernel/pools` 端点
- [ ] `kernel_api.go` 新增 `handleKernelPools`
- [ ] 返回 `count / pools[]` JSON
- [ ] 每个 pool 含 `name / workers / queueSize / queueDepth / ledgerEnabled / ledgerRoot / lastRestoreCount / lastRestoreAt`

### 1.4 `/api/kernel/restore` 端点（dev only）
- [ ] `kernel_api.go` 新增 `handleKernelRestore`
- [ ] dev 模式校验（`ENCV_DEV=1` 或 `ENCV_DEV_PREVIEW=1`）
- [ ] 非 dev 返回 403

### 1.5 Cypress E2E: kernel-endpoints.cy.ts
- [ ] 测试 `GET /api/kernel/services` 返回 3 个 service
- [ ] 测试 `GET /api/kernel/health` 全绿
- [ ] 测试 `GET /api/kernel/pools` 返回 Pool 状态
- [ ] 真实后端（无 cy.intercept mock）

### 1.6 Cypress E2E: kernel-restart-restore.cy.ts
- [ ] 启动后端，POST `/api/files/search-fulltext/rebuild` 创建 FTS 任务
- [ ] 等待 `running` + 进度 30%+
- [ ] POST `/api/dev/kill-backend`（dev only）模拟杀进程
- [ ] 等 2s + 重启后端
- [ ] 验证 `Pool.Restore` 日志
- [ ] 验证 task 续跑（状态 `running`，进度从 30%+ → 100%）
- [ ] 验证 cancelled task 不被 Restore 续跑

## Phase 2: Android 高版本硬约束修复

### 2.1 ComboLite HostService foregroundServiceType
- [ ] Manifest 16 个 HostService 补 `android:foregroundServiceType="specialUse"`
- [ ] 每个 HostService 加 `<property android:name="android.app.PROPERTY_SPECIAL_USE_FGS_SUBTYPE" android:value="encv-plugin-host" />`
- [ ] 验证 `FOREGROUND_SERVICE_SPECIAL_USE` 权限已声明（Manifest L19）
- [ ] `./gradlew :app:processDebugManifest` 通过

### 2.2 EncvGoService 评估
- [ ] 评估是否从 dataSync 改 specialUse（长任务 >6h 场景）
- [ ] 若改，加 specialUse property 描述
- [ ] 若不改，记录决策原因

## Phase 3: WorkManager 引入 + 随时启停

### 3.1 WorkManager 依赖
- [ ] `libs.versions.toml` 加 `work-runtime-ktx = "2.10.1"`
- [ ] `build.gradle.kts` 加 `implementation(libs.work.runtime.ktx)`
- [ ] `./gradlew :app:assembleDebug` 通过

### 3.2 EncvTaskCancelWorker
- [ ] 新建 `workers/EncvTaskCancelWorker.kt`
- [ ] 继承 `CoroutineWorker`
- [ ] `doWork()`：先 `/health` 探活，存活则 POST `/api/tasks/:id/cancel`
- [ ] 失败 retry，LinearBackoff(10s)，max 10 次
- [ ] 持久化 taskId 通过 `WorkRequest inputData`

### 3.3 GoProcessRestartReceiver
- [ ] 新建 `workers/GoProcessRestartReceiver.kt`
- [ ] 监听 `BROADCAST_BACKEND_READY`
- [ ] 调 `WorkManager.enqueueAllPending(EncvTaskCancelWorker.class)`

### 3.4 useTaskCancel composable
- [ ] 新建 `composables/useTaskCancel.ts`
- [ ] `cancel(taskId)` 双写：HTTP + Capacitor Worker
- [ ] HTTP 成功则忽略 Worker 结果
- [ ] HTTP 失败则依赖 Worker 重试

### 3.5 Android Instrumented Test
- [ ] 新建 `androidTest/.../EncvTaskCancelWorkerTest.kt`
- [ ] 真实 Go 后端（`Runtime.exec`）
- [ ] 测试场景：Go 存活 → 立即取消
- [ ] 测试场景：Go 死亡 → 持久化 → 重启 → 重发

## 验收

- [ ] Phase 1 全部通过（go build / vet / test + Cypress 真实后端）
- [ ] Phase 2 全部通过（Manifest 修改 + gradle 验证）
- [ ] Phase 3 代码骨架完成（androidTest 标记待跑）
- [ ] spec 文档更新（如有变更）
- [ ] 规则文档更新（android.md 加 WorkManager 章节）
