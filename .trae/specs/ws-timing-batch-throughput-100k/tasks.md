# Tasks

## 阶段一：WS 时序修复（根治孤儿 group）

- [x] Task 1: 后端 `Create()` 移除广播 + 改用 `saveTaskSingle`
  - [x] SubTask 1.1: `task_manager.go` `Create()` 删除 `broadcaster.Broadcast("task:created", task)`
  - [x] SubTask 1.2: `task_manager.go` `Create()` `saveTasks()` → `saveTaskSingle()`
  - [x] SubTask 1.3: `task_manager.go` `CreateWithRunMeta()` 末尾加 `Broadcast("task:created", task)`（在 `saveTaskSingle` 之后）
  - [x] SubTask 1.4: `admin_handlers.go` 3 处（rename/copy/move）`Create()` 后补 `RunId` 兜底 + `Broadcast`
  - [x] SubTask 1.5: `mobile_api.go` delete handler `Create()` 后补 `RunId` 兜底 + `Broadcast`
  - [x] SubTask 1.6: `rollback_manager.go` `Create()` 后补 `RunId` 兜底 + `Broadcast`
  - [x] SubTask 1.7: 后端测试 — `Create()` 不广播 / `CreateWithRunMeta()` 广播带 runId

## 阶段二：批量吞吐 + 非阻塞 submitRun + 批量取消

- [x] Task 2: 后端批量取消 API
  - [x] SubTask 2.1: `task_manager.go` 新增 `CancelByRunId(runId string) error`
  - [x] SubTask 2.2: `mobile_api.go` 新增 `handleCancelRunGin` handler
  - [x] SubTask 2.3: `server.go` 注册 `POST /api/runs/:runId/cancel` 路由
  - [x] SubTask 2.4: 后端测试 — `CancelByRunId` 取消指定 runId 所有非终态 task
- [x] Task 3: 前端 `submitRun` 非阻塞
  - [x] SubTask 3.1: `useWorkflowTaskService.ts` `submitRun()` 先创建 run + push + 持久化 → 立即返回 run
  - [x] SubTask 3.2: `executeJob` 改 fire-and-forget（不 await，后台异步执行）
  - [x] SubTask 3.3: `PluginTestsDetail.vue` toast 在 `submitRun` 返回后立即显示
- [x] Task 4: 前端 `cancelRun` 批量取消
  - [x] SubTask 4.1: `api/encv.ts` 新增 `cancelRun(runId: string)` API
  - [x] SubTask 4.2: `useWorkflowTaskService.ts` `cancelRun()` 改为调 `cancelRun(runId)` 一次 API
  - [x] SubTask 4.3: 删除逐个 `cancelTask` 循环

## 阶段三：10 万任务虚拟滚动 + 分页加载

- [x] Task 5: 后端分页 API
  - [x] SubTask 5.1: `mobile_api.go` `handleGetTasksGin` 加 `?runId=&offset=&limit=` 参数
  - [x] SubTask 5.2: 后端测试 — 分页返回正确范围
- [x] Task 6: 前端分页加载
  - [x] SubTask 6.1: `api/encv.ts` `getTasks()` 加分页参数
  - [x] SubTask 6.2: `useTasksList.ts` 按 runId 分页加载，首屏 100 个
  - [x] SubTask 6.3: 滚动到底部增量加载下一页
- [x] Task 7: 前端虚拟滚动优化
  - [x] SubTask 7.1: `TaskVirtualList.vue` 确认虚拟滚动只渲染可见行
  - [x] SubTask 7.2: `taskStore.ts` WS 事件只 patch 已加载的 task

## 测试：完善已有模拟渲染真实测试

- [x] Task 8: 扩展 `buildDynamicWorkflow.pre-population.test.ts` 三阶段全覆盖
  - [x] SubTask 8.1: 阶段一 — WS 时序测试（mock broadcaster 验证 `Create` 不广播 / `CreateWithRunMeta` 广播带 runId）
  - [x] SubTask 8.2: 阶段二 — 非阻塞 submitRun 测试（run 对象 < 50ms 返回 / batchCreateTasks 异步完成）
  - [x] SubTask 8.3: 阶段二 — 批量取消测试（只调 1 次 cancelRun API / 所有 task 标记 cancelled）
  - [x] SubTask 8.4: 阶段三 — 10 万虚拟滚动测试（DOM task card 数 <= 30）
- [x] Task 9: `TaskListDiagSimulator.tsx` 补控件
  - [x] SubTask 9.1: 补取消按钮（group card 上的取消控件）
  - [x] SubTask 9.2: 补分页加载指示器（底部 loading spinner）
  - [x] SubTask 9.3: 补虚拟滚动容器（data-testid 标记可见行数）
- [x] Task 10: 全量回归验证
  - [x] SubTask 10.1: `go test ./internal/service/...` 通过
  - [x] SubTask 10.2: `go build ./...` 通过
  - [x] SubTask 10.3: `vue-tsc --noEmit` 通过
  - [x] SubTask 10.4: `pnpm test:run` 通过（pre-existing fail 除外）

# Task Dependencies

- Task 2 依赖 Task 1（WS 时序修复后才能验证批量取消不产生孤儿）
- Task 3 依赖 Task 1（非阻塞 submitRun 需要 WS 时序正确才能保证 UI 一致性）
- Task 4 依赖 Task 2（前端 cancelRun 依赖后端 API）
- Task 5 独立（可并行）
- Task 6 依赖 Task 5（前端分页依赖后端 API）
- Task 7 依赖 Task 6（虚拟滚动依赖分页加载）
- Task 8 依赖 Task 1-7（测试覆盖所有阶段）
- Task 9 依赖 Task 8（模拟器补控件供测试使用）
- Task 10 依赖 Task 1-9

# 可并行

- Task 1（后端 WS 时序）与 Task 5（后端分页）可并行
- Task 3（前端非阻塞）与 Task 4（前端批量取消）可并行（都依赖 Task 1 完成后）
