# Android 高版本 WorkManager 拆分 / 随时启停 Spec

> **核心原则（用户原话）**：所有改造**必须建立在真实后端运行的基础上**——"我不希望再看到什么不依赖真实后端之类的"。
>
> **严苛约束（用户原话）**：Android 高版本（targetSdk 36 / Android 16）+ WorkManager 拆分 + 随时启停。
>
> **不引入 mock backend**：所有 Cypress E2E 测试 + Android Worker 测试**必须打真实的 Go 后端 HTTP 端点**（127.0.0.1:2025-2035）。Worker 是 Kotlin 薄包装，业务逻辑全在 Go。

---

## 一、Why

### 1.1 现状诊断（来自调研报告）

| 维度 | 现状 | 风险 |
|------|------|------|
| Android WorkManager 依赖 | **未引入** | 无法做持久化 cancel/retry |
| Android Worker 子类 | **0 个** | 单任务级 cancel 在 Android 端无持久化 |
| `EncvGoService` | dataSync FGS，START_STICKY | Android 15+ DataSync 6h 上限 → 长任务被系统强杀 |
| ComboLite 16 个 HostService | **未声明 foregroundServiceType** | targetSdk 36 下 `startForegroundService` 必 crash（`MissingForegroundServiceTypeException`） |
| Android 端 cancel/pause | **只能杀整个 Go 进程** | 用户点"取消"按钮若 Go 进程已死 → 操作丢失 |
| Go TaskManager | cancel/resume HTTP 端点齐全 | 但 cancelFn 是 `context.CancelFunc` 内存态，重启即丢 |
| kernel Pool + FileJobLedger + Restore | **已实现但无调用方** | 半成品，TaskManager 没接入 |
| 重启后任务续跑 | **完全缺失** | task 记录持久化（显示用）但不续跑 |
| Cypress kernel 端点测试 | **完全缺失** | `/api/kernel/services` / `/api/kernel/health` 0 覆盖 |

### 1.2 用户原话驱动的目标分解

> "继续特色微服务内核改造，适配 cypress 测试和安卓高版本 workmanager 拆分随时启停等严苛要求"

拆解为 4 个目标：

1. **继续内核改造**：kernel Pool 接入 TaskManager，让 Pool.Restore() 在 server 启动时被调用
2. **适配 Cypress 测试**：新增 `/api/kernel/pools` 端点 + Cypress E2E 覆盖 kernel 端点（真实后端）
3. **Android 高版本 WorkManager 拆分**：ComboLite HostService 补 foregroundServiceType（targetSdk 36 硬约束）+ 引入 WorkManager 2.10+
4. **随时启停**：Android 端 `EncvTaskCancelWorker` 持久化 cancel 意图，Go 进程重启后重发

### 1.3 反模式（用户明令禁止）

- ❌ "不依赖真实后端"的 Worker（mock 任务执行）
- ❌ 用 WorkManager 替代 Go 后端业务逻辑（业务必须留在 Go）
- ❌ 在 Kotlin 端做加密/解密/索引重建（heavy lifting 必须在 Go）
- ❌ 引入 mock backend 跑 Cypress 测试（必须打真实 :2025-2035 端口）

---

## 二、Scope（分 3 个 Phase）

### Phase 1：Go 内核接入 + Cypress 真实后端测试（**核心交付**）

**目标**：让 kernel Pool + FileJobLedger + Restore 真正工作起来，Cypress 验证全流程。

| # | 任务 | 文件 |
|---|------|------|
| 1.1 | TaskManager 接入 kernel Pool（可选路径，guarded by feature flag） | `internal/service/task_manager_worker.go` |
| 1.2 | server 启动时调 `Pool.Restore()` 恢复未完成 job | `internal/server/server.go` |
| 1.3 | 新增 `/api/kernel/pools` 端点（列出 Pool + Ledger 状态 + Restore 计数） | `internal/server/kernel_api.go` |
| 1.4 | 新增 `/api/kernel/restore` 端点（POST 触发 Pool.Restore，dev only） | `internal/server/kernel_api.go` |
| 1.5 | Cypress E2E：`/api/kernel/services` + `/api/kernel/health` + `/api/kernel/pools` | `cypress/e2e/kernel-endpoints.cy.ts`（新建） |
| 1.6 | Cypress E2E：杀 Go 进程 → 重启 → 验证 Pool.Restore 续跑 | `cypress/e2e/kernel-restart-restore.cy.ts`（新建） |

### Phase 2：Android 高版本硬约束修复（**targetSdk 36 必修**）

**目标**：让 APK 在 Android 14+/15+/16 上不 crash。

| # | 任务 | 文件 |
|---|------|------|
| 2.1 | ComboLite 16 个 HostService 补 `foregroundServiceType="specialUse"` + `<property>` 描述 | `android/app/src/main/AndroidManifest.xml` |
| 2.2 | `EncvGoService` 评估是否从 dataSync 改 specialUse（长任务 >6h 场景） | `android/app/src/main/AndroidManifest.xml` |
| 2.3 | 验证 `FOREGROUND_SERVICE_SPECIAL_USE` 权限已声明（Manifest L19 ✓） | — |

### Phase 3：WorkManager 引入 + 随时启停（**Android 端新增**）

**目标**：单任务级 cancel/pause 持久化到 WorkManager，Go 进程重启后自动重发。

| # | 任务 | 文件 |
|---|------|------|
| 3.1 | 引入 `androidx.work:work-runtime-ktx:2.10.1` 依赖 | `android/gradle/libs.versions.toml` + `android/app/build.gradle.kts` |
| 3.2 | `EncvTaskCancelWorker`（CoroutineWorker）：persisted cancel intent + 重试 HTTP `/api/tasks/:id/cancel` | `android/app/src/main/java/com/encvgo/app/workers/EncvTaskCancelWorker.kt`（新建） |
| 3.3 | `GoProcessRestartReceiver`：监听 `BROADCAST_BACKEND_READY`，触发 `EncvTaskCancelWorker.enqueueAllPending()` | `android/app/src/main/java/com/encvgo/app/workers/GoProcessRestartReceiver.kt`（新建） |
| 3.4 | 前端 `useTaskCancel` composable：取消任务时**双写** — HTTP `/api/tasks/:id/cancel` + WorkManager `EncvTaskCancelWorker.enqueue()` | `app/encv-mobile/src/composables/useTaskCancel.ts`（新建） |
| 3.5 | Android Instrumented Test：`EncvTaskCancelWorkerTest`（真机/模拟器，打真实 Go 后端） | `android/app/src/androidTest/java/com/encvgo/app/workers/EncvTaskCancelWorkerTest.kt`（新建） |

---

## 三、ADDED Requirements

### Requirement: kernel Pool 接入 TaskManager（Phase 1.1）

系统 SHALL 提供 `TaskManager.useKernelPool(pool *kernel.Pool)` 方法，启用后所有任务通过 `pool.Submit(ctx, job)` 提交，替代直接 `go func() + cancelFn`。

#### Scenario: 启用 kernel Pool 后任务正常执行
- **WHEN** `taskManager.useKernelPool(pool)` 被调用，且 `taskManager.CreateWithRunMeta(...)` 创建新任务
- **THEN** 任务通过 `pool.Submit()` 进入 worker 队列
- **AND** 任务的 `cancelFn` 注册为 `pool.CancelJob(jobID)`（kernel Pool 新增方法）
- **AND** 任务执行完毕后 `OnDone` 回调更新 task 状态

#### Scenario: 未启用 kernel Pool（默认）保持兼容
- **WHEN** `useKernelPool` 未被调用
- **THEN** TaskManager 走原有 `go func() + cancelFn` 路径
- **AND** 行为与改造前完全一致

### Requirement: server 启动时调 Pool.Restore（Phase 1.2）

系统 SHALL 在 `Server.Start()` 完成路由注册后、监听端口前，调用 `pool.Restore(bootCtx)`，恢复所有未完成 job。

#### Scenario: 重启后续跑未完成任务
- **WHEN** server 启动且 `pool.cfg.Ledger` 非 nil
- **THEN** 调用 `pool.Restore(bootCtx)` 返回 `(restored int, err error)`
- **AND** 恢复的 job 重新进入 `jobCh` 队列
- **AND** 日志记录 `restored=N`（INFO 级别）
- **AND** 若 Restore 失败，server 启动**继续**（不阻塞），但日志 WARN + Step Summary（CI 场景）

#### Scenario: 无 Ledger 时跳过 Restore
- **WHEN** `pool.cfg.Ledger == nil`
- **THEN** 跳过 Restore，日志 DEBUG `kernel: pool has no ledger, skip restore`

### Requirement: `/api/kernel/pools` 端点（Phase 1.3）

系统 SHALL 提供 `GET /api/kernel/pools` 端点，返回所有 kernel Pool 的运行时状态。

#### Scenario: 列出所有 Pool
- **WHEN** 客户端 GET `/api/kernel/pools`
- **THEN** 返回 200 + JSON：
```json
{
  "count": 1,
  "pools": [
    {
      "name": "task-manager",
      "workers": 4,
      "queueSize": 64,
      "queueDepth": 3,
      "ledgerEnabled": true,
      "ledgerRoot": "/data/data/com.encvgo.app/files/kernel-ledger",
      "lastRestoreCount": 2,
      "lastRestoreAt": "2026-07-03T00:37:03Z"
    }
  ]
}
```

#### Scenario: Pool 未启用 Ledger
- **WHEN** Pool 的 `Ledger == nil`
- **THEN** `ledgerEnabled=false`，`ledgerRoot=""`，`lastRestoreCount=0`

### Requirement: `/api/kernel/restore` 端点（Phase 1.4，dev only）

系统 SHALL 提供 `POST /api/kernel/restore` 端点（仅 dev 模式可用），手动触发 Pool.Restore，用于 Cypress E2E 测试。

#### Scenario: dev 模式手动触发 Restore
- **WHEN** 客户端 POST `/api/kernel/restore` 且 `ENCV_DEV=1` 或 `ENCV_DEV_PREVIEW=1`
- **THEN** 调用 `pool.Restore(bootCtx)`
- **AND** 返回 200 + JSON：`{"restored": N, "error": ""}`

#### Scenario: 非 dev 模式拒绝访问
- **WHEN** 客户端 POST `/api/kernel/restore` 且非 dev 模式
- **THEN** 返回 403 + JSON：`{"error": "kernel: restore endpoint is dev-only"}`

### Requirement: Cypress E2E kernel 端点覆盖（Phase 1.5）

Cypress SHALL 提供真实后端 E2E 测试，覆盖 kernel 诊断端点。

#### Scenario: 验证 3 个 kernel service 已注册
- **WHEN** Cypress 测试启动真实后端（:2025）
- **THEN** `GET /api/kernel/services` 返回 200
- **AND** `body.services` 包含 `["search.vector", "ws.hub", "fts.rebuilder"]`
- **AND** `body.count === 3`

#### Scenario: 验证 kernel health 全绿
- **WHEN** Cypress 测试 `GET /api/kernel/health`
- **THEN** `body.ok === true`
- **AND** 每个 service 的 `ok === true`
- **AND** 每个 service 的 `latency < 5000`（5s 上限）

#### Scenario: 验证 /api/kernel/pools 返回 Pool 状态
- **WHEN** Cypress 测试 `GET /api/kernel/pools`
- **THEN** `body.count >= 0`（可能为 0，若未启用 Pool）
- **AND** 若 `count > 0`，每个 pool 包含 `name / workers / queueSize / ledgerEnabled` 字段

### Requirement: Cypress E2E 重启续跑（Phase 1.6）

Cypress SHALL 提供真实后端 E2E 测试，验证杀进程 → 重启 → 任务续跑。

#### Scenario: FTS 重建任务中断后续跑
- **WHEN** Cypress 启动后端，POST `/api/files/search-fulltext/rebuild` 创建 FTS 重建任务
- **AND** 等待任务进入 `running` 状态（进度 30%-50%）
- **AND** 通过 `POST /api/dev/kill-backend`（dev only）模拟"杀进程"
- **AND** 等待 2s，重新启动后端（PM2 自动拉起 或 手动 `go run`）
- **THEN** 后端启动后调 `Pool.Restore()`，日志 `kernel: pool restored N jobs`
- **AND** `GET /api/tasks/:id` 返回 task 状态从 `running` → `running`（续跑，非 failed）
- **AND** 任务最终 `completed`，进度从 30%+ 继续 → 100%

#### Scenario: 取消的 task 不被 Restore 续跑
- **WHEN** 任务状态为 `cancelled`（用户主动取消）
- **THEN** Pool.Restore **不**重新提交该任务
- **AND** `GET /api/tasks/:id` 状态保持 `cancelled`

### Requirement: ComboLite HostService foregroundServiceType（Phase 2.1）

Manifest SHALL 为 16 个 HostService 声明 `foregroundServiceType`，避免 targetSdk 36 下 `startForegroundService` crash。

#### Scenario: HostService 启动为前台 service 不 crash
- **WHEN** ComboLite 框架调用 `startForegroundService(HostService1Intent)`
- **THEN** 系统不抛 `MissingForegroundServiceTypeException`
- **AND** service 正常启动并调用 `startForeground()`

#### Scenario: specialUse 类型属性声明
- **WHEN** Manifest 声明 `foregroundServiceType="specialUse"`
- **THEN** 同一 `<service>` 节点必须包含 `<property android:name="android.app.PROPERTY_SPECIAL_USE_FGS_SUBTYPE" android:value="..." />`
- **AND** value 描述真实用途（例：`"encv-plugin-host"`）

### Requirement: WorkManager 依赖引入（Phase 3.1）

`android/app/build.gradle.kts` SHALL 引入 `androidx.work:work-runtime-ktx:2.10.1`。

#### Scenario: 依赖可解析
- **WHEN** `./gradlew :app:dependencies` 执行
- **THEN** `androidx.work:work-runtime-ktx:2.10.1` 出现在 dependency tree
- **AND** 间接依赖 `androidx.lifecycle:lifecycle-service:2.8.x` 兼容

### Requirement: EncvTaskCancelWorker（Phase 3.2）

系统 SHALL 提供 `EncvTaskCancelWorker`，持久化 cancel 意图，Go 进程重启后重发 HTTP。

#### Scenario: Go 进程存活时立即取消
- **WHEN** 前端调 `useTaskCancel.cancel(taskId)`，且 Go 进程存活（`/health` 200）
- **THEN** Worker 立即 POST `/api/tasks/:id/cancel`
- **AND** 返回 200 → Worker 标记 `Result.success()`
- **AND** WorkInfo 状态为 SUCCEEDED

#### Scenario: Go 进程死亡时持久化 cancel 意图
- **WHEN** 前端调 `useTaskCancel.cancel(taskId)`，且 Go 进程不响应（`/health` 失败 3 次）
- **THEN** Worker 标记 `Result.retry()`，WorkInfo 状态为 ENQUEUED + retryCount=1
- **AND** cancel 意图持久化到 WorkManager 内部 SQLite
- **AND** 退避策略 `LinearBackoff(10s)`，最多 10 次重试

#### Scenario: Go 进程重启后重发 cancel
- **WHEN** Go 进程重启并广播 `BROADCAST_BACKEND_READY`
- **THEN** `GoProcessRestartReceiver` 接收广播
- **AND** 调 `WorkManager.enqueueAllPending(EncvTaskCancelWorker.class)`
- **AND** 所有 pending cancel Worker 立即重试
- **AND** Go 端收到 cancel 请求，task 状态 → `cancelled`

#### Scenario: cancel 意图超过 10 次重试仍失败
- **WHEN** Worker retryCount >= 10 且仍失败
- **THEN** Worker 标记 `Result.failure()`
- **AND** WorkInfo 状态为 FAILED
- **AND** 前端通过 `useTaskCancel.pollCancelStatus(taskId)` 得知失败，弹 toast 提示用户手动重试

### Requirement: useTaskCancel 前端 composable（Phase 3.4）

前端 SHALL 提供 `useTaskCancel` composable，**双写** HTTP + WorkManager。

#### Scenario: 取消任务时双写
- **WHEN** 用户点击 UI 取消按钮，触发 `useTaskCancel.cancel(taskId)`
- **THEN** 同时发起：
  1. HTTP `POST /api/tasks/:id/cancel`（同步，期望立即生效）
  2. Capacitor 调 `GoProcessPlugin.enqueueCancelWorker(taskId)`（异步，兜底持久化）
- **AND** HTTP 成功则忽略 Worker 结果（Worker 内部去重：先查 task 状态，已 cancelled 则 noop）
- **AND** HTTP 失败则依赖 Worker 重试

---

## 四、Non-Goals

- ❌ **不**用 WorkManager 替代 Go 后端业务逻辑（加密/解密/索引重建必须留在 Go）
- ❌ **不**在 Kotlin 端实现任何加密算法
- ❌ **不**用 WorkManager 替代 `EncvGoService`（FGS 仍是 Go 进程宿主，WorkManager 只管 cancel 意图持久化）
- ❌ **不**在 Phase 1 引入 ExpeditedWorkRequest（Phase 4 stretch，需评估 quota）
- ❌ **不**为 Cypress 测试引入 mock backend（必须打真实 :2025）
- ❌ **不**改 ComboLite 框架内部代码（只补 Manifest 声明）

---

## 五、Impact

### 5.1 受影响的代码

**Go 端**：
- `internal/service/task_manager.go` — 新增 `useKernelPool` + `kernelPool` 字段
- `internal/service/task_manager_worker.go` — Submit 路径双轨（goroutine / pool.Submit）
- `internal/server/server.go` — Start() 调 Pool.Restore()
- `internal/server/kernel_api.go` — 新增 `/api/kernel/pools` + `/api/kernel/restore`
- `internal/server/routes.go` — 注册新路由

**Cypress**：
- `app/encv-mobile/cypress/e2e/kernel-endpoints.cy.ts`（新建）
- `app/encv-mobile/cypress/e2e/kernel-restart-restore.cy.ts`（新建）

**Android**：
- `app/encv-mobile/android/app/src/main/AndroidManifest.xml` — 16 HostService 补 foregroundServiceType
- `app/encv-mobile/android/gradle/libs.versions.toml` — 加 work-runtime-ktx 版本
- `app/encv-mobile/android/app/build.gradle.kts` — 加依赖
- `app/encv-mobile/android/app/src/main/java/com/encvgo/app/workers/EncvTaskCancelWorker.kt`（新建）
- `app/encv-mobile/android/app/src/main/java/com/encvgo/app/workers/GoProcessRestartReceiver.kt`（新建）
- `app/encv-mobile/android/app/src/androidTest/java/com/encvgo/app/workers/EncvTaskCancelWorkerTest.kt`（新建）

**前端**：
- `app/encv-mobile/src/composables/useTaskCancel.ts`（新建）

### 5.2 受影响的 specs

- `refactor-mobile-backend-service` — TaskManager 改造的延续
- `cross-process-ipc-refactor` — Go 进程死亡/重启的协调机制
- `unify-workflow-task-service` — 任务系统统一

### 5.3 受影响的规则文档（需后续更新）

- `.trae/rules/android.md` — 新增 WorkManager 章节
- `.trae/rules/automation-workflow.md` — 4 件套 WS 事件在 WorkManager 重启场景下的语义
- `.trae/rules/capacitor.md` — Modal 跨 tab 在 WorkManager 通知场景下的行为

---

## 六、验收标准（**用户硬约束**）

> 用户原话："平均启动就绪耗时不超过0.5秒，停止不超过0.2秒，正确处理停止后续处理委托，内存指标优秀，满足不消耗额外端口的设计目标"

### 6.0 kernel 生命周期硬指标（**首要验收**）

| 指标 | 目标 | 验收方式 |
|------|------|---------|
| **kernel.Start() 就绪耗时** | ≤ 500ms（avg） | Go bench + HTTP `/api/kernel/lifecycle/start` 计时 |
| **kernel.Stop() 停止耗时** | ≤ 200ms（avg） | Go bench + HTTP `/api/kernel/lifecycle/stop` 计时 |
| **停止时 in-flight 请求** | 全部收到响应或 graceful error（无 hang） | Go bench 持续打 traffic + 中途 Stop |
| **停止后 pending job 委托** | 100% 写入 Ledger（可 Restore 续跑） | `pool_restore_test.go` 加压测场景 |
| **内存守卫** | Start/Stop 1000 次循环后 RSS 增长 ≤ 10% | Go bench + runtime.MemStats |
| **不消耗端口** | 0 个 TCP 端口监听（纯进程内调用） | `lsof -iTCP -sTCP:LISTEN` 在 kernel 启停前后对比 |
| **持续业务消费** | 启停过程中业务请求 100ms 间隔不间断，失败率 ≤ 5%（其余为 graceful error） | Go bench `BenchmarkKernelLifecycleUnderLoad` |

### 6.0.1 反模式（用户明令禁止）

- ❌ 启动后只 ping 一次 `/health` 就停（必须**持续**业务请求）
- ❌ 用 `time.Sleep` 模拟业务消费（必须真实调 `kernel.Call`）
- ❌ 临时测试脚本（必须放进 `internal/kernel/*_test.go` 或 `cmd/...`）
- ❌ kernel 启动监听 TCP 端口（违反"不消耗端口"原则）

### 6.0.2 内核启停机制设计

新增 `kernel.Lifecycle` 概念：

```go
// internal/kernel/lifecycle.go
type Lifecycle struct {
    pools      []*Pool
    services   []Service
    ledger     JobLedger
    store      CheckpointStore
    ready      atomic.Bool
    startedAt  time.Time
    stoppedAt  time.Time
    memGuard   *MemoryGuard
}

func (l *Lifecycle) Start(ctx context.Context) error      // 注册 + 启动所有 pool + Restore
func (l *Lifecycle) Stop(graceful time.Duration) error    // drain pool + delegate pending to ledger
func (l *Lifecycle) Ready() bool                          // 快速检查
func (l *Lifecycle) Stats() LifecycleStats                // 启停耗时 + 内存 + 队列深度
```

**Stop 的 graceful delegation 流程**：
1. 标记 `ready=false`（新请求立即返回 `ErrKernelNotReady`）
2. 给 in-flight job 一个 `graceful` 时间窗（默认 200ms）自然完成
3. 时间窗结束后未完成的 job → 写入 Ledger（status=submitted，下次 Restore 续跑）
4. 关闭所有 worker goroutine
5. 释放 MemoryGuard
6. 记录 `stoppedAt`，返回

### 6.1 Phase 1 验收

- [ ] `go build ./...` 通过
- [ ] `go vet ./internal/...` 通过
- [ ] `bash scripts/test-go.sh ./internal/kernel/...` 全过（含新加的 lifecycle benchmark）
- [ ] `go test -bench=KernelLifecycle -benchmem ./internal/kernel/...` 跑通，输出启停耗时
- [ ] `bash scripts/test-go.sh ./internal/service/...` 全过
- [ ] Cypress `kernel-endpoints.cy.ts` 真实后端通过
- [ ] Cypress `kernel-restart-restore.cy.ts` 真实后端通过
- [ ] 后端启动日志可见 `kernel: pool restored N jobs`（若 N=0 也要打印 DEBUG）
- [ ] **启停硬指标验证**：HTTP `POST /api/kernel/lifecycle/start` + `POST /api/kernel/lifecycle/stop` 100 次循环，avg 启动 < 500ms，avg 停止 < 200ms
- [ ] **内存守卫验证**：1000 次循环后 RSS 增长 ≤ 10%

### 6.2 Phase 2 验收

- [ ] `./gradlew :app:processDebugManifest` 通过
- [ ] APK 在 Android 14+ 模拟器启动 EncvGoService 不 crash
- [ ] ComboLite 加载 plugin-openlist 时 HostService 启动不 crash
- [ ] Manifest 合并工具不报 `MissingForegroundServiceType` 警告

### 6.3 Phase 3 验收

- [ ] `./gradlew :app:assembleDebug` 通过（依赖可解析）
- [ ] `EncvTaskCancelWorkerTest`（androidTest）在真机/模拟器通过（打真实 :2025）
- [ ] 前端 `useTaskCancel.cancel(taskId)` 同时发 HTTP + 触发 Worker
- [ ] 杀 Go 进程后重启，pending cancel 意图自动重发，task 状态正确变为 `cancelled`

---

## 七、测试策略（**真实后端铁律**）

> 用户原话："我不希望再看到什么不依赖真实后端之类的"

### 7.1 Cypress E2E（必须真实后端）

- 启动方式：`PM2_HOME=/tmp/cypress-pm2 pnpm dev` + `ENCV_MOBILE=1 ENCV_DEV_PREVIEW=1 go run ./cmd/encv start`
- 端口：`:2025`（Go 后端）+ `:8100`（Vite）
- **禁止** `cy.intercept()` mock 任何 `/api/*` 端点
- **禁止** `cy.stub()` 替代 Go 后端响应
- 测试间用 `POST /api/dev/reset-state`（dev only）重置状态，**不**重启后端

### 7.2 Go 单元测试

- kernel Pool + Ledger 已有 32 个测试（pool_restore_test.go + tool_wrapper_test.go）
- TaskManager 接入 Pool 后新增 5-10 个集成测试

### 7.3 Android Instrumented Test（androidTest）

- 必须在真机/模拟器跑（CI 暂不支持，手动验证）
- `EncvTaskCancelWorkerTest` 启动真实 Go 后端（`Runtime.exec("libencv-go.so", "start")`）
- 不允许 mock Go 后端 HTTP 响应

---

## 八、风险与回退

### 8.1 风险

| 风险 | 概率 | 影响 | 缓解 |
|------|------|------|------|
| TaskManager 接入 Pool 引入回归 | 中 | 高 | feature flag 默认关闭，灰度开启 |
| Pool.Restore 在生产环境恢复过多 job 撑爆队列 | 低 | 中 | Restore 时检查 `queueDepth + pending <= queueSize`，超限 WARN + 截断 |
| ComboLite specialUse 类型被 Google Play 拒 | 低 | 低 | 保留 dataSync 作为 fallback；Play Console 提交时声明用途 |
| WorkManager 2.10.1 与现有 lifecycle 版本冲突 | 低 | 中 | dependency analysis 提前验证 |
| Cypress 重启测试在 CI 不稳定 | 中 | 中 | 加 5s sleep + retry 3 次；本地必跑 |

### 8.2 回退策略

- Phase 1 回退：关闭 `useKernelPool` feature flag，TaskManager 回到 goroutine 路径
- Phase 2 回退：Manifest 改回 dataSync（仅 EncvGoService）
- Phase 3 回退：移除 WorkManager 依赖 + 删除 Worker 类，前端 `useTaskCancel` 退化到纯 HTTP

---

## 九、Phase 排期与优先级

| Phase | 优先级 | 工作量 | 必须在本轮交付 |
|-------|-------|-------|---------------|
| Phase 1 | P0 | 中 | ✅ 必须交付 |
| Phase 2 | P1 | 小 | ✅ 必须交付（Manifest 修改） |
| Phase 3 | P2 | 中 | 交付代码骨架 + 1 个 Worker 类，androidTest 标记为待跑 |

---

## 十、引用

- 调研报告：见 spec 文档同目录 `research.md`（如需补充可加）
- 相关规则：
  - [android.md](../../rules/android.md) — WorkManager 章节待新增
  - [automation-workflow.md](../../rules/automation-workflow.md) — 4 件套 WS 事件
  - [capacitor.md](../../rules/capacitor.md) — Modal 跨 tab 行为
  - [ci-workflow.md](../../rules/ci-workflow.md) — 三层 CI 职责
- 相关 specs：
  - [refactor-mobile-backend-service](../refactor-mobile-backend-service/spec.md)
  - [cross-process-ipc-refactor](../cross-process-ipc-refactor/spec.md)
- 关键代码：
  - [internal/kernel/pool.go](../../../internal/kernel/pool.go)
  - [internal/kernel/pool_restore.go](../../../internal/kernel/pool_restore.go)
  - [internal/service/task_manager.go](../../../internal/service/task_manager.go)
  - [internal/server/kernel_api.go](../../../internal/server/kernel_api.go)
  - [android/app/src/main/java/com/encvgo/app/EncvGoService.kt](../../../app/encv-mobile/android/app/src/main/java/com/encvgo/app/EncvGoService.kt)
