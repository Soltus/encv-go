# WS 时序修复 + 批量吞吐 + 10 万任务性能 Spec

> 创建：2026-06-23
> 状态：待用户批准
> 前置：regression-fixes-2026-06-22（批量创建 API 已落地，但暴露 WS 时序 bug）

---

## Why

批量创建 API（`POST /api/tasks/batch`）落地后，用户真机日志暴露了**任务逃逸真凶**：WS `task:created` 在 `RunId` 设置**之前**广播，导致前端收到 `runId=""` 的 task → 孤儿 group → 一段时间后 WS `task:update` 推带 `runId` 的 task → 孤子消失正常聚会。

同时用户反馈两个性能问题：
1. **toast 延迟**：`submitRun` 阻塞在 `await batchCreateTasks(1056 specs)` 上，toast "Workflow started: 1063 steps" 要等几秒才显示
2. **取消卡顿**：`cancelRun` 逐个调 `cancelTask` API × 1000+ 次，取消整个 workflow 卡卡的

目标：根治孤儿 group + submitRun 非阻塞 + 批量取消 + 10 万任务虚拟滚动，分三阶段交付。

---

## What Changes

### 阶段一：WS 时序修复（根治孤儿 group）

- **后端** `task_manager.go` `Create()`：删除 `broadcaster.Broadcast("task:created", task)`（移到 `CreateWithRunMeta` 末尾）
- **后端** `task_manager.go` `CreateWithRunMeta()`：末尾加 `Broadcast("task:created", task)`（所有字段设置后）
- **后端** `task_manager.go` `Create()`：`saveTasks()` → `saveTaskSingle()`（O(1) 单行写，不再全表写）
- **后端** 5 个直接 `Create()` 调用方补广播：`admin_handlers.go`（rename/copy/move）、`mobile_api.go`（delete）、`rollback_manager.go`（rollback）

### 阶段二：批量吞吐 + 非阻塞 submitRun + 批量取消

- **后端** `task_manager.go`：新增 `CancelByRunId(runId string) error`（按 runId 批量取消）
- **后端** `mobile_api.go`：新增 `POST /api/runs/:runId/cancel` handler
- **后端** `server.go`：注册路由
- **前端** `api/encv.ts`：新增 `cancelRun(runId: string)` API
- **前端** `useWorkflowTaskService.ts` `submitRun()`：先创建 run + 显示 toast → 立即返回 run；`executeJob` 改 fire-and-forget
- **前端** `useWorkflowTaskService.ts` `cancelRun()`：改为调 `cancelRun(runId)` 一次 API（不再逐个 cancelTask）

### 阶段三：10 万任务虚拟滚动 + 分页加载

- **后端** `mobile_api.go` `handleGetTasksGin`：加 `?runId=&offset=&limit=` 分页参数
- **前端** `api/encv.ts` `getTasks()`：加分页参数
- **前端** `useTasksList.ts`：按 runId 分页加载，首屏只加载前 100 个 task
- **前端** `Tasks.vue` / `TaskVirtualList.vue`：虚拟滚动优化（只渲染可见行）
- **前端** `taskStore.ts`：WS 事件只 patch 已加载的 task，未加载的走增量加载

### 测试：完善已有模拟渲染真实测试

- **前端** `buildDynamicWorkflow.pre-population.test.ts`：扩展为 3 阶段全覆盖（WS 时序 + 非阻塞 + 批量取消 + 10 万虚拟滚动）
- **前端** `TaskListDiagSimulator.tsx`：补取消按钮 + 分页控件 1:1 复刻
- **后端** `task_manager_crypto_params_test.go`：新增 WS 时序测试（`Create` 不广播、`CreateWithRunMeta` 广播）
- **后端** 新增 `CancelByRunId` 测试

---

## Impact

### 受影响代码

**后端**：
- `internal/service/task_manager.go` — `Create` / `CreateWithRunMeta` / `CreateBatch` / 新增 `CancelByRunId`
- `internal/server/mobile_api.go` — `handleCreateTaskGin` / `handleCreateTaskBatchGin` / 新增 `handleCancelRunGin` / `handleGetTasksGin` 加分页
- `internal/server/server.go` — 注册 `POST /api/runs/:runId/cancel`
- `internal/server/admin_handlers.go` — 3 处 `Create()` 后补广播
- `internal/service/rollback_manager.go` — `Create()` 后补广播

**前端**：
- `app/encv-mobile/src/api/encv.ts` — `getTasks` 加分页 / 新增 `cancelRun`
- `app/encv-mobile/src/composables/useWorkflowTaskService.ts` — `submitRun` 非阻塞 / `cancelRun` 批量
- `app/encv-mobile/src/composables/useTasksList.ts` — 分页加载
- `app/encv-mobile/src/stores/taskStore.ts` — WS 事件 patch 逻辑
- `app/encv-mobile/src/views/Tasks.vue` — 虚拟滚动
- `app/encv-mobile/src/components/tasks/TaskVirtualList.vue` — 虚拟滚动优化

**测试**：
- `app/encv-mobile/src/lib/workflow/__tests__/buildDynamicWorkflow.pre-population.test.ts` — 扩展
- `app/encv-mobile/src/lib/workflow/__tests__/fixtures/TaskListDiagSimulator.tsx` — 补控件
- `internal/service/task_manager_crypto_params_test.go` — 新增 WS 时序测试

### 受影响 specs
- `regression-fixes-2026-06-22`（批量创建 API 已落地，本 spec 修复其暴露的 WS 时序 bug）
- `unify-workflow-task-service`（submitRun / cancelRun 架构变更）

---

## ADDED Requirements

### Requirement: WS task:created 广播时序正确

系统 SHALL 在 task 所有字段（含 `RunId` / `TriggeredBy` / `CipherMode` / `CompressionMode`）设置完毕后才广播 `task:created` WS 事件。

#### Scenario: CreateWithRunMeta 广播带 runId
- **WHEN** 后端通过 `CreateWithRunMeta` 创建 task（含 runId 参数）
- **THEN** WS `task:created` 事件的 payload 中 `runId` 字段非空
- **AND** 前端 `taskStore.appendTask` 收到的 task `runId` 非空
- **AND** 不产生孤儿 group

#### Scenario: Create 不广播（移到 CreateWithRunMeta）
- **WHEN** 后端 `Create()` 被直接调用（admin_handlers / delete / rollback）
- **THEN** `Create()` 内部不广播 `task:created`
- **AND** 调用方在设置完 `RunId` 后自行广播

#### Scenario: Create 改用 saveTaskSingle
- **WHEN** 后端 `Create()` 创建 task
- **THEN** 持久化走 `saveTaskSingle()`（O(1) 单行写）
- **AND** 不走 `saveTasks()`（O(N) 全表写）

### Requirement: submitRun 非阻塞（toast 秒显）

系统 SHALL 在 `submitRun` 创建 `WorkflowRun` 后立即返回 run 对象，不阻塞在 `batchCreateTasks` API 调用上。

#### Scenario: toast 秒显
- **WHEN** 用户点击"运行自动化插件测试"
- **THEN** `submitRun` 在 < 50ms 内返回 run 对象
- **AND** toast "Workflow started: N steps" 立即显示
- **AND** `executeJob` 在后台异步执行（fire-and-forget）

#### Scenario: executeJob 异步执行
- **WHEN** `submitRun` 返回后
- **THEN** `executeJob` 继续在后台收集 specs → 调 `batchCreateTasks` → push 到 store
- **AND** UI 通过 reactive 自动更新（不需要 await）

### Requirement: 批量取消（POST /api/runs/:runId/cancel）

系统 SHALL 提供 `POST /api/runs/:runId/cancel` endpoint，一次性取消指定 runId 下所有非终态 task。

#### Scenario: 一次 API 取消 1000+ task
- **WHEN** 前端调用 `cancelRun(runId)` API
- **THEN** 后端 `CancelByRunId(runId)` 遍历所有 `task.RunId == runId` 且非终态的 task
- **AND** 逐个调 `cancelTask` 内部方法（不走 HTTP）
- **AND** 返回 `200 OK`
- **AND** 前端只调 1 次 API（不是 1000+ 次）

#### Scenario: 前端 cancelRun 改批量
- **WHEN** 前端 `useWorkflowTaskService.cancelRun(runId)` 被调用
- **THEN** 调用 `cancelRun(runId)` API 一次
- **AND** 不再逐个调 `cancelTask(taskId)` × N

### Requirement: 10 万任务分页加载

系统 SHALL 支持按 `runId` 分页加载 task，首屏只加载前 100 个 task。

#### Scenario: GET /api/tasks 分页
- **WHEN** 前端调用 `getTasks({ runId, offset, limit })`
- **THEN** 后端返回指定 runId 下 offset~offset+limit 范围的 task
- **AND** 默认 limit=100

#### Scenario: 首屏只加载 100 个 task
- **WHEN** 用户打开 Tasks tab
- **THEN** 前端只加载前 100 个 task 到 store
- **AND** 滚动到底部时增量加载下一页

### Requirement: 虚拟滚动只渲染可见行

系统 SHALL 在任务列表中使用虚拟滚动，只渲染可见行（~20 行），不渲染全部 task。

#### Scenario: 10 万 task 流畅滚动
- **WHEN** store 中有 10 万个 task
- **THEN** DOM 中只渲染可见的 ~20 行 task card
- **AND** 滚动流畅（60fps）
- **AND** 内存只保留当前页 task 对象

### Requirement: 模拟渲染真实测试全覆盖

系统 SHALL 在 `buildDynamicWorkflow.pre-population.test.ts` 中覆盖三阶段所有场景，使用 `TaskListDiagSimulator` 1:1 复刻真机 UI 验证。

#### Scenario: 阶段一 WS 时序测试
- **WHEN** `CreateWithRunMeta` 创建 task
- **THEN** mock broadcaster 收到的 `task:created` 事件 payload `runId` 非空
- **AND** `Create()` 直接调用不触发 broadcaster

#### Scenario: 阶段二 非阻塞 submitRun 测试
- **WHEN** `submitRun` 被调用
- **THEN** run 对象在 < 50ms 内返回
- **AND** `batchCreateTasks` 在后台异步完成
- **AND** UI 通过 reactive 自动显示 task

#### Scenario: 阶段二 批量取消测试
- **WHEN** `cancelRun(runId)` 被调用
- **THEN** 只调 1 次 `cancelRun` API
- **AND** 所有非终态 task 标记为 cancelled

#### Scenario: 阶段三 10 万虚拟滚动测试
- **WHEN** store 中有 10 万个 task
- **THEN** `TaskListDiagSimulator` 渲染的 DOM task card 数 <= 30（可见 + overscan）
- **AND** 滚动后正确渲染新可见行

---

## MODIFIED Requirements

### Requirement: CreateWithRunMeta 广播 task:created

`CreateWithRunMeta` SHALL 在所有字段设置完毕后（`saveTaskSingle` 之后）广播 `task:created` WS 事件。

#### Scenario: 广播时序
- **WHEN** `CreateWithRunMeta` 执行完毕
- **THEN** `task.RunId` / `task.TriggeredBy` / `task.CipherMode` / `task.CompressionMode` 已全部设置
- **AND** `broadcaster.Broadcast("task:created", task)` 在 `saveTaskSingle` 之后调用

### Requirement: Create 不广播 task:created

`Create` SHALL 不再广播 `task:created` WS 事件，广播责任移交给调用方。

#### Scenario: Create 静默
- **WHEN** `Create()` 执行
- **THEN** 不调用 `broadcaster.Broadcast`
- **AND** 调用方（`CreateWithRunMeta` / `admin_handlers` / `mobile_api delete` / `rollback_manager`）负责在字段设置完毕后广播

### Requirement: submitRun 非阻塞

`submitRun` SHALL 立即返回 run 对象，`executeJob` 改为 fire-and-forget。

#### Scenario: submitRun 立即返回
- **WHEN** `submitRun({ workflow, triggeredBy })` 被调用
- **THEN** 创建 `WorkflowRun` + push 到 runs 列表 + 持久化
- **AND** 立即返回 run 对象（不等 `executeJob`）
- **AND** `executeJob` 在后台异步执行

### Requirement: cancelRun 批量取消

`cancelRun` SHALL 调用 `POST /api/runs/:runId/cancel` 一次 API，不再逐个调 `cancelTask`。

#### Scenario: 批量取消
- **WHEN** `cancelRun(runId)` 被调用
- **THEN** 调用 `cancelRun(runId)` API 一次
- **AND** 标记所有非终态 step 为 cancelling → cancelled
- **AND** 不逐个调 `cancelTask(taskId)`

---

## REMOVED Requirements

### Requirement: Create 内部广播 task:created
**Reason**: `Create` 在 `RunId` 设置前广播，导致 WS `task:created` payload `runId=""` → 孤儿 group
**Migration**: 广播移到 `CreateWithRunMeta` 末尾；5 个直接 `Create` 调用方补广播

### Requirement: cancelRun 逐个调 cancelTask
**Reason**: 1000+ task 逐个调 API 卡顿
**Migration**: 改为 `POST /api/runs/:runId/cancel` 一次 API

### Requirement: submitRun 阻塞在 executeJob
**Reason**: `await batchCreateTasks(1056 specs)` 阻塞 toast 显示
**Migration**: `submitRun` 立即返回 run；`executeJob` fire-and-forget
