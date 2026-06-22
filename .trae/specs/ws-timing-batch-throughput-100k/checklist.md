# Checklist

## 阶段一：WS 时序修复

- [x] `Create()` 内部不调用 `broadcaster.Broadcast("task:created", task)`
- [x] `Create()` 持久化走 `saveTaskSingle()`（不是 `saveTasks()`）
- [x] `CreateWithRunMeta()` 末尾（`saveTaskSingle` 之后）调用 `Broadcast("task:created", task)`
- [x] `CreateWithRunMeta()` 广播的 task payload 中 `runId` 非空
- [x] `admin_handlers.go` 3 处（rename/copy/move）`Create()` 后补 `RunId` 兜底 + `Broadcast`
- [x] `mobile_api.go` delete handler `Create()` 后补 `RunId` 兜底 + `Broadcast`
- [x] `rollback_manager.go` `Create()` 后补 `RunId` 兜底 + `Broadcast`
- [x] 后端测试验证 `Create()` 不广播 / `CreateWithRunMeta()` 广播带 runId

## 阶段二：批量吞吐 + 非阻塞 + 批量取消

- [x] 后端 `CancelByRunId(runId string) error` 方法存在
- [x] `CancelByRunId` 取消指定 runId 下所有非终态 task
- [x] `POST /api/runs/:runId/cancel` 路由已注册
- [x] 前端 `cancelRun(runId: string)` API 函数存在
- [x] 前端 `useWorkflowTaskService.cancelRun()` 只调 1 次 `cancelRun` API（不逐个 cancelTask）
- [x] 前端 `submitRun()` 在 < 50ms 内返回 run 对象
- [x] `submitRun()` 返回后 toast 立即显示
- [x] `executeJob` 改为 fire-and-forget（不阻塞 submitRun）
- [x] 后端测试验证 `CancelByRunId` 正确取消

## 阶段三：10 万任务虚拟滚动 + 分页

- [x] `GET /api/tasks` 支持 `?runId=&offset=&limit=` 参数
- [x] 前端 `getTasks()` 支持分页参数
- [x] `useTasksList.ts` 首屏只加载 100 个 task
- [x] 滚动到底部增量加载下一页
- [x] `TaskVirtualList.vue` 虚拟滚动只渲染可见行（~20 行）
- [x] 10 万 task 滚动流畅（60fps）
- [x] WS 事件只 patch 已加载的 task

## 测试：模拟渲染真实测试

- [x] `buildDynamicWorkflow.pre-population.test.ts` 覆盖阶段一 WS 时序
- [x] `buildDynamicWorkflow.pre-population.test.ts` 覆盖阶段二非阻塞 submitRun
- [x] `buildDynamicWorkflow.pre-population.test.ts` 覆盖阶段二批量取消
- [x] `buildDynamicWorkflow.pre-population.test.ts` 覆盖阶段三 10 万虚拟滚动
- [x] `TaskListDiagSimulator.tsx` 补取消按钮控件
- [x] `TaskListDiagSimulator.tsx` 补分页加载指示器
- [x] `TaskListDiagSimulator.tsx` 补虚拟滚动容器标记
- [x] 后端测试 `Create` 不广播 / `CreateWithRunMeta` 广播带 runId
- [x] 后端测试 `CancelByRunId` 正确取消

## 全量回归

- [ ] `go test ./internal/service/...` 通过
- [ ] `go build ./...` 通过
- [ ] `vue-tsc --noEmit` 通过
- [ ] `pnpm test:run` 通过（pre-existing fail 除外）
