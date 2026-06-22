# Checklist

## 阶段一：后端 SQL 权威

- [ ] `task_manager.go` 新增 `GetRunSummary(runId string) RunSummary`
- [ ] `GetRunSummary` 优先走 SQLite store SQL（`GROUP BY status`）
- [ ] `GetRunSummary` 无 store 时降级为内存遍历
- [ ] `GET /api/runs/:runId/summary` 路由已注册
- [ ] `task_manager.go` 新增 `ListRuns() []RunInfo`
- [ ] `ListRuns` SQL：`GROUP BY run_id ORDER BY MIN(created_at) DESC`
- [ ] `ListRuns` 每个 run 带 summary（避免 N+1）
- [ ] `GET /api/runs` 路由已注册
- [ ] `ListPaginated` 优先走 `store.ListTasks(TaskFilter{RunID, Limit, Offset})`
- [ ] `ListPaginated` 无 store 时降级为内存过滤

## 阶段二：前端 store 拆分 + GroupDetail 独立加载

- [ ] `api/encv.ts` 新增 `getRunSummary(runId)` API
- [ ] `api/encv.ts` 新增 `listRuns()` API
- [ ] `composables/useRunSummaries.ts` 管理 run summary 数据
- [ ] WS `task:completed` 时刷新对应 runId 的 summary
- [ ] `useTaskStore`（Tasks 页）保留视图分页 + WS 守卫
- [ ] 新增 `useRunTasksStore`（GroupDetail 页）按 runId 独立加载
- [ ] `useRunTasksStore` WS 不守卫（当前 runId 的 task 全量 push）
- [ ] GroupDetail 进入时调 `GET /api/tasks?runId=xxx&offset=0&limit=100`
- [ ] GroupDetail `ion-infinite-scroll` 触发 `loadMore`（按 runId 分页）
- [ ] GroupDetail 复用 Tasks 页顶层的 filter/sort/viewMode 控件
- [ ] GroupDetail 删除自己的 `searchQuery` / `filterStatuses` / `filterPlugins`
- [ ] Tasks.vue 模板加 `ion-infinite-scroll` + `ion-infinite-scroll-content`
- [ ] Tasks.vue `@ionInfinite` 事件触发 `loadMore`
- [ ] Tasks.vue `hasMore=false` 时禁用 infinite-scroll

## 阶段三：虚拟滚动重构 + WS 上下文过滤

- [ ] `TaskVirtualList.vue` props 改为 `count` + `getItem(index)` + `getKey(index)`
- [ ] virtualizer 只调 `getItem` 获取可见窗口的 item
- [ ] 切换 viewMode 时只更新 `count` + `getItem` 引用
- [ ] virtualizer 复用 measure cache（key 稳定）
- [ ] `Tasks.vue` 适配新接口（displayedItems 改为 count + getItem）
- [ ] Tasks 页：WS `task:created` 不进 store（只触发 summary 刷新）
- [ ] Tasks 页：WS `task:update`/`task:completed` 只 patch 已加载的 task
- [ ] GroupDetail 页：WS 事件只处理当前 runId 的 task
- [ ] 离开视图时停止处理（onUnmounted 取消订阅）

## 阶段四：测试升级

- [ ] 挂载真 `Tasks.vue` + mock API + mock WS
- [ ] 验证 DOM 节点数 ≤ 30（虚拟滚动核心指标）
- [ ] 验证 `ion-infinite-scroll` 触发 `loadMore`
- [ ] 验证 1000+ task 时 group card 计数正确（从 summary 获取）
- [ ] 验证切换 viewMode 不卡顿（count + getItem 接口）
- [ ] 挂载真 `GroupDetail.vue` + mock API + mock WS
- [ ] 验证进入时调 `GET /api/tasks?runId=xxx`
- [ ] 验证 WS 事件只处理当前 runId 的 task
- [ ] 验证复用 Tasks 顶层 filter/sort/viewMode 控件
- [ ] 后端测试 `GetRunSummary` SQL 查询正确
- [ ] 后端测试 `ListRuns` 返回所有 run（带 summary）
- [ ] 后端测试 `ListPaginated` 走 SQL（不是内存过滤）

## 阶段五：全量回归

- [ ] `go build ./...` 通过
- [ ] `go test ./internal/service/...` 通过
- [ ] `vue-tsc --noEmit` 通过
- [ ] `pnpm test:run` 通过（pre-existing fail 除外）
