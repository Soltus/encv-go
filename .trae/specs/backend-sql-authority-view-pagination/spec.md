# Spec: 后端 SQL 权威 + 前端视图分页 + GroupDetail 独立加载

> 创建：2026-06-23
> 起因：用户反馈"自动化插件测试 1000+ 任务聚合不稳定显示，初始和切换会导致虚拟滚动坍缩到 100 任务，而且虚拟滚动设计下切换到平铺依旧卡顿"
> 核心洞察（用户原话）："后端才是唯一权威，查询使用 sql，任务系统 api 设计上能够提供给第三方调用的。前端计数和渲染又不需要保持一致"

---

## 一、问题根因

### 1.1 虚拟滚动坍缩到 100 任务

**现象**：自动化测试 1000+ task 后，Tasks 列表只显示 100 个 task，切换 viewMode/sortBy 时 store 还是 100 个。

**根因链路**：
1. `taskStore.ts` `MAX_LOADED_TASKS=100` 守卫在 `hydrated` 后截断 WS `task:created`
2. `useTasksList.ts` `fetchTasks` 首屏只加载 100 个（`offset=0 limit=100`）
3. `Tasks.vue` 模板没有 `ion-infinite-scroll`，用户滚动到底部不会触发 `loadMore`
4. 结果：1000+ task 的 run 永远只显示 100 个

### 1.2 聚合计数错误

**现象**：group card 显示 `· 12 个任务` / `passed 8 / failed 2`，但实际 run 有 1000+ task。

**根因**：`useTasksList.ts` `buildGroupDisplayData` 靠 `store.tasks` 算计数，但 store 只持有 100 个，1000+ task 的 run 计数会错。

### 1.3 虚拟滚动切换平铺卡顿

**现象**：group 模式切换到 flat 模式时卡顿。

**根因**：`displayedItems` computed 在 viewMode 切换时重新遍历 store 构建 item 数组（O(N)），即使虚拟滚动只渲染可见行，computed 本身的遍历也会卡顿。

### 1.4 GroupDetail 用伪搜索筛选控件

**现象**（用户反馈）："当前 group 详情的任务视图依旧使用单独的伪搜索筛选等控件，而不是使用任务 tab 顶层的控件，这不合理"

**根因**：`GroupDetail.vue` 自己维护 `searchQuery` / `filterStatuses` / `filterPlugins`，没有复用 Tasks 页顶层的 filter/sort/viewMode 控件。

### 1.5 模拟测试是摆设

**现象**（用户反馈）："你的模拟测试完全是摆设"

**根因**：
- `TaskListDiagSimulator.tsx` 用 `v-for` 渲染所有 item（不虚拟滚动）
- 没验证 DOM 节点数 ≤ 30
- 没验证 `loadMore` 接入 UI
- 没验证 WS 守卫不截断正在运行的 run

---

## 二、设计原则

### 2.1 后端是唯一权威

- 任务系统 API 提供给第三方调用，必须支持 SQL 查询
- `GET /api/tasks?runId=&offset=&limit=` + `X-Total-Count` 响应头（已存在）
- 聚合计数由后端 SQL `COUNT` + `GROUP BY status` 出，不依赖前端 store
- 前端 store 只持有"当前视图需要的"task，不是所有 task

### 2.2 前端计数和渲染不需要保持一致

- **聚合计数**：从后端 `/runs/:runId/summary` 获取（SQL COUNT），独立于 store.tasks
- **渲染**：虚拟滚动只画可见行（~20 个 DOM 节点），独立于 store.tasks.length
- 两者本就不该耦合

### 2.3 WS 事件按视图上下文过滤

- **Tasks 列表页**：WS `task:created` 不进 store（只触发 summary 刷新）；`task:update`/`task:completed` 只 patch 已加载的 task
- **GroupDetail 页**：WS 事件只处理当前 runId 的 task（全量 push + patch）
- 离开视图时停止处理

### 2.4 GroupDetail 复用 Tasks 顶层控件

- GroupDetail 不再自己维护 `searchQuery` / `filterStatuses` / `filterPlugins`
- 复用 Tasks 页顶层的 filter/sort/viewMode 控件（通过共享 store 状态）
- GroupDetail 只额外提供 runId 过滤（路由参数）

---

## 三、后端设计

### 3.1 新增 `GET /api/runs/:runId/summary`

**用途**：返回指定 run 的聚合计数（SQL COUNT + GROUP BY status）

**响应**：
```json
{
  "runId": "run-xxx",
  "total": 1063,
  "passed": 1040,
  "failed": 15,
  "running": 5,
  "pending": 3,
  "cancelled": 0,
  "percent": 99
}
```

**实现**：
- `task_manager.go` 新增 `GetRunSummary(runId string) RunSummary`
- 优先走 SQLite store SQL 查询：`SELECT status, COUNT(*) FROM tasks WHERE run_id=? GROUP BY status`
- 无 store 时降级为内存遍历（`tm.List()` filter runId + count by status）

### 3.2 新增 `GET /api/runs`

**用途**：返回所有 run 列表（带 summary，避免 N+1 查询）

**响应**：
```json
{
  "runs": [
    {
      "runId": "run-xxx",
      "startedAt": "2026-06-23T10:00:00Z",
      "triggeredBy": "automation",
      "summary": { "total": 1063, "passed": 1040, "failed": 15, ... }
    },
    ...
  ]
}
```

**实现**：
- `task_manager.go` 新增 `ListRuns() []RunInfo`
- SQL：`SELECT run_id, MIN(created_at), triggered_by FROM tasks WHERE run_id != '' GROUP BY run_id ORDER BY MIN(created_at) DESC`
- 每个 run 再调 `GetRunSummary(runId)` 拿计数（或用一条 SQL JOIN 避免 N+1）

### 3.3 ListPaginated 改走 SQL

**现状**：`ListPaginated` 是内存过滤（`tm.List()` 拿全部再 filter）

**改进**：优先走 SQLite store SQL 查询
- `store.ListTasks(TaskFilter{RunID: runId, Limit: limit, Offset: offset})` 已支持
- 无 store 时降级为内存过滤（向后兼容）

---

## 四、前端设计

### 4.1 taskStore 拆分

**现状**：单一 `useTaskStore` 持有所有 task，Tasks 页和 GroupDetail 页共享

**改进**：
- `useTaskStore`（Tasks 页）：视图分页，WS 守卫保留
- `useRunTasksStore`（GroupDetail 页）：按 runId 独立加载，WS 不守卫

### 4.2 Tasks 页

- `fetchTasks`：调 `GET /api/tasks?offset=0&limit=100` 加载首屏
- `loadMore`：调 `GET /api/tasks?offset=N&limit=100` 加载下一页
- `ion-infinite-scroll`：滚动到底部触发 `loadMore`
- group card 计数：调 `GET /api/runs/:runId/summary` 获取（不靠 store.tasks 算）
- WS `task:created`：守卫保留（store 满后不 push）
- WS `task:update`/`task:completed`：只 patch 已加载的 task

### 4.3 GroupDetail 页

- 独立路由 `/tabs/tasks/group/:runId`
- 进入时调 `GET /api/tasks?runId=xxx&offset=0&limit=100` 加载该 run 的 task
- `ion-infinite-scroll` 触发 `loadMore`（按 runId 分页）
- 复用 Tasks 页顶层的 filter/sort/viewMode 控件（通过共享 store 状态）
- WS 事件只处理当前 runId 的 task（全量 push + patch）

### 4.4 TaskVirtualList 重构

**现状**：接收 `items` 数组（O(N) 遍历）

**改进**：接收 `count` + `getItem(index)` 接口
- virtualizer 只调 `getItem` 获取可见窗口的 ~20 个 item
- 切换 viewMode 时只更新 `count` + `getItem` 引用
- virtualizer 复用 measure cache（key 稳定）

### 4.5 聚合计数独立

- `useRunSummaries` composable：管理 run summary 数据
- group card 显示 `summary.total` / `summary.passed` / `summary.failed`（不靠 store.tasks 算）
- WS `task:completed` 时刷新对应 runId 的 summary（调 `GET /api/runs/:runId/summary`）

---

## 五、测试设计

### 5.1 完全用真机组件测试

- 挂载真 `Tasks.vue` + mock API + mock WS
- 挂载真 `GroupDetail.vue` + mock API + mock WS
- 验证：
  - DOM 节点数 ≤ 30（虚拟滚动核心指标）
  - `ion-infinite-scroll` 触发 `loadMore`
  - WS 守卫不截断正在运行的 run（GroupDetail 页）
  - 1000+ task 时 group card 计数正确（从 summary 获取）
  - 切换 viewMode 不卡顿（count + getItem 接口）

### 5.2 后端测试

- `GetRunSummary` SQL 查询正确
- `ListRuns` 返回所有 run（带 summary）
- `ListPaginated` 走 SQL（不是内存过滤）

---

## 六、迁移路径

1. 后端新增 `GetRunSummary` + `ListRuns` + `ListPaginated` 走 SQL
2. 前端新增 `useRunSummaries` composable
3. 前端 `taskStore` 拆分（Tasks 页 + GroupDetail 页）
4. 前端 `GroupDetail` 重构（独立加载 + 复用顶层控件）
5. 前端 `Tasks.vue` 接入 `ion-infinite-scroll`
6. 前端 `TaskVirtualList` 重构为 `count + getItem`
7. 前端 WS 事件按视图上下文过滤
8. 测试升级（真机组件测试）
