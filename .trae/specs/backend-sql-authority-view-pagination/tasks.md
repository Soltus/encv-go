# Tasks

## 阶段一：后端 SQL 权威（聚合 API + 分页走 SQL）

- [ ] Task 1: 后端新增 `GET /api/runs/:runId/summary`
  - [ ] SubTask 1.1: `task_manager.go` 新增 `GetRunSummary(runId string) RunSummary`
  - [ ] SubTask 1.2: 优先走 SQLite store SQL：`SELECT status, COUNT(*) FROM tasks WHERE run_id=? GROUP BY status`
  - [ ] SubTask 1.3: 无 store 时降级为内存遍历（向后兼容）
  - [ ] SubTask 1.4: `mobile_api.go` 新增 `handleGetRunSummaryGin` handler
  - [ ] SubTask 1.5: `server.go` 注册 `GET /api/runs/:runId/summary` 路由
  - [ ] SubTask 1.6: 后端测试 — `GetRunSummary` SQL 查询正确
- [ ] Task 2: 后端新增 `GET /api/runs`（run 列表带 summary）
  - [ ] SubTask 2.1: `task_manager.go` 新增 `ListRuns() []RunInfo`
  - [ ] SubTask 2.2: SQL：`SELECT run_id, MIN(created_at), triggered_by FROM tasks WHERE run_id != '' GROUP BY run_id ORDER BY MIN(created_at) DESC`
  - [ ] SubTask 2.3: 每个 run 再调 `GetRunSummary(runId)` 拿计数（或 JOIN 避免 N+1）
  - [ ] SubTask 2.4: `mobile_api.go` 新增 `handleListRunsGin` handler
  - [ ] SubTask 2.5: `server.go` 注册 `GET /api/runs` 路由
  - [ ] SubTask 2.6: 后端测试 — `ListRuns` 返回所有 run（带 summary）
- [ ] Task 3: `ListPaginated` 改走 SQL
  - [ ] SubTask 3.1: `task_manager.go` `ListPaginated` 优先走 `store.ListTasks(TaskFilter{RunID, Limit, Offset})`
  - [ ] SubTask 3.2: 无 store 时降级为内存过滤（向后兼容）
  - [ ] SubTask 3.3: 后端测试 — `ListPaginated` 走 SQL 验证

## 阶段二：前端 store 拆分 + GroupDetail 独立加载

- [ ] Task 4: 前端新增 `useRunSummaries` composable
  - [ ] SubTask 4.1: `api/encv.ts` 新增 `getRunSummary(runId)` + `listRuns()` API
  - [ ] SubTask 4.2: `composables/useRunSummaries.ts` 管理 run summary 数据
  - [ ] SubTask 4.3: WS `task:completed` 时刷新对应 runId 的 summary
- [ ] Task 5: 前端 `taskStore` 拆分
  - [ ] SubTask 5.1: `useTaskStore`（Tasks 页）保留视图分页 + WS 守卫
  - [ ] SubTask 5.2: 新增 `useRunTasksStore`（GroupDetail 页）按 runId 独立加载
  - [ ] SubTask 5.3: `useRunTasksStore` WS 不守卫（当前 runId 的 task 全量 push）
- [ ] Task 6: GroupDetail 重构
  - [ ] SubTask 6.1: 进入时调 `GET /api/tasks?runId=xxx&offset=0&limit=100` 加载
  - [ ] SubTask 6.2: `ion-infinite-scroll` 触发 `loadMore`（按 runId 分页）
  - [ ] SubTask 6.3: 复用 Tasks 页顶层的 filter/sort/viewMode 控件（共享 store 状态）
  - [ ] SubTask 6.4: 删除 GroupDetail 自己的 `searchQuery` / `filterStatuses` / `filterPlugins`
- [ ] Task 7: Tasks.vue 接入 `ion-infinite-scroll`
  - [ ] SubTask 7.1: 模板加 `ion-infinite-scroll` + `ion-infinite-scroll-content`
  - [ ] SubTask 7.2: `@ionInfinite` 事件触发 `loadMore`
  - [ ] SubTask 7.3: `hasMore=false` 时禁用 infinite-scroll

## 阶段三：虚拟滚动重构 + WS 上下文过滤

- [ ] Task 8: TaskVirtualList 重构为 `count + getItem`
  - [ ] SubTask 8.1: `TaskVirtualList.vue` props 改为 `count` + `getItem(index)` + `getKey(index)`
  - [ ] SubTask 8.2: virtualizer 只调 `getItem` 获取可见窗口的 item
  - [ ] SubTask 8.3: 切换 viewMode 时只更新 `count` + `getItem` 引用（virtualizer 复用 measure cache）
  - [ ] SubTask 8.4: `Tasks.vue` 适配新接口（displayedItems 改为 count + getItem）
- [ ] Task 9: WS 事件按视图上下文过滤
  - [ ] SubTask 9.1: Tasks 页：WS `task:created` 不进 store（只触发 summary 刷新）
  - [ ] SubTask 9.2: Tasks 页：WS `task:update`/`task:completed` 只 patch 已加载的 task
  - [ ] SubTask 9.3: GroupDetail 页：WS 事件只处理当前 runId 的 task
  - [ ] SubTask 9.4: 离开视图时停止处理（onUnmounted 取消订阅）

## 阶段四：测试升级（完全用真机组件测试）

- [ ] Task 10: 真机组件测试
  - [ ] SubTask 10.1: 挂载真 `Tasks.vue` + mock API + mock WS
  - [ ] SubTask 10.2: 验证 DOM 节点数 ≤ 30（虚拟滚动核心指标）
  - [ ] SubTask 10.3: 验证 `ion-infinite-scroll` 触发 `loadMore`
  - [ ] SubTask 10.4: 验证 1000+ task 时 group card 计数正确（从 summary 获取）
  - [ ] SubTask 10.5: 验证切换 viewMode 不卡顿（count + getItem 接口）
- [ ] Task 11: GroupDetail 真机组件测试
  - [ ] SubTask 11.1: 挂载真 `GroupDetail.vue` + mock API + mock WS
  - [ ] SubTask 11.2: 验证进入时调 `GET /api/tasks?runId=xxx`
  - [ ] SubTask 11.3: 验证 WS 事件只处理当前 runId 的 task
  - [ ] SubTask 11.4: 验证复用 Tasks 顶层 filter/sort/viewMode 控件
- [ ] Task 12: 后端测试
  - [ ] SubTask 12.1: `GetRunSummary` SQL 查询正确
  - [ ] SubTask 12.2: `ListRuns` 返回所有 run（带 summary）
  - [ ] SubTask 12.3: `ListPaginated` 走 SQL（不是内存过滤）

## 阶段五：全量回归验证

- [ ] Task 13: 全量回归
  - [ ] SubTask 13.1: `go build ./...` 通过
  - [ ] SubTask 13.2: `go test ./internal/service/...` 通过
  - [ ] SubTask 13.3: `vue-tsc --noEmit` 通过
  - [ ] SubTask 13.4: `pnpm test:run` 通过（pre-existing fail 除外）

# Task Dependencies

- Task 2 依赖 Task 1（ListRuns 调 GetRunSummary）
- Task 3 独立（可并行）
- Task 4 依赖 Task 1/2（前端 API 依赖后端路由）
- Task 5 依赖 Task 4（store 拆分依赖 summary composable）
- Task 6 依赖 Task 5（GroupDetail 依赖 useRunTasksStore）
- Task 7 独立（可并行）
- Task 8 依赖 Task 5/6（虚拟重构依赖 store 拆分）
- Task 9 依赖 Task 5/6/8（WS 过滤依赖 store 拆分 + 虚拟重构）
- Task 10/11 依赖 Task 1-9
- Task 12 依赖 Task 1/2/3
- Task 13 依赖 Task 1-12

# 可并行

- Task 1（后端 summary）与 Task 3（ListPaginated 走 SQL）可并行
- Task 4（前端 summary composable）与 Task 7（ion-infinite-scroll）可并行（都依赖后端）
- Task 8（虚拟重构）与 Task 9（WS 过滤）可并行（都依赖 store 拆分）
