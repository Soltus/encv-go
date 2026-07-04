# 任务组 / 子任务 UI 折叠铁律

> **核心原则：后端当前**无 GroupID / ParentID 概念**，"任务组"是纯前端 UI 概念。**
> **不要在 task_manager.go 或后端 API 上加 group 字段（没必要，污染 schema）。**
> **自动/批量产生的 task（自动化测试 / AI agent）必须在 UI 层折叠成 group card。**

---

## 一、后端 task model 现状

[internal/service/task_manager.go](file:///workspace/internal/service/task_manager.go) `Task` 结构：

```go
type Task struct {
    ID          string
    Type        string  // encrypt / decrypt
    Status      string
    Progress    int
    Container   string
    ContainerVersion string
    SourcePath  string
    OutputPath  string
    Error       string
    Warning     string
    PluginName  string
    CreatedAt   time.Time
    StartedAt   *time.Time
    CompletedAt *time.Time
    Steps       []TaskStep  // 这里是子结构，≠ subTask
}
```

**没有** `GroupID / ParentID / Tags / BatchID` 字段。后端 API 路由：

```
GET    /api/tasks                    列出所有 task
POST   /api/tasks                    创建单 task
GET    /api/tasks/:id                查单 task
POST   /api/tasks/:id/cancel
POST   /api/tasks/:id/retry
DELETE /api/tasks/:id
DELETE /api/tasks                    清空（已完成 / 已取消）
```

**没有 `/api/tasks/batch` / `/api/task-groups` / `/api/tasks/:id/subtasks`**。**不要尝试加这些后端 API**——后端设计里 task 就是 flat list。

## 二、批量产生 task 的入口（已知清单）

| 入口 | 数量 | 触发表 | 文件 |
|------|------|--------|------|
| [useAutomationTests.runTests()](file:///workspace/app/encv-mobile/src/composables/useAutomationTests.ts) | 1+（一个 test case 一个） | `triggeredBy: 'automation'` | [src/composables/useAutomationTests.ts](file:///workspace/app/encv-mobile/src/composables/useAutomationTests.ts) |
| useAgent 流式多任务 | 1+ | `triggeredBy: 'ai_agent'` | [src/composables/useAgent.ts](file:///workspace/app/encv-mobile/src/composables/useAgent.ts) |

两处都串行 `for` 循环逐个调 `createTask()`，触发 N 张独立 task 卡片，污染 task 列表 UI。

## 三、UI 折叠方案（推荐）

**不改后端**。在 [src/views/Tasks.vue](file:///workspace/app/encv-mobile/src/views/Tasks.vue) 加 `displayedItems` computed：

```ts
type DisplayItem =
  | { kind: 'group'; key: string; groupKey: string; tasks: EncvTask[]; summary: {...} }
  | { kind: 'task'; key: string; task: EncvTask }

const displayedItems = computed<DisplayItem[]>(() => {
  const result: DisplayItem[] = []
  const tasks = filteredTasks.value
  let i = 0
  while (i < tasks.length) {
    const cur = tasks[i]
    const curBy = getTriggeredBy(cur.id)
    if (curBy === 'user') {
      result.push({ kind: 'task', key: cur.id, task: cur })
      i++; continue
    }
    // 收集连续同 triggeredBy 区段
    const seg: EncvTask[] = [cur]
    let j = i + 1
    while (j < tasks.length && getTriggeredBy(tasks[j].id) === curBy) {
      seg.push(tasks[j]); j++
    }
    if (seg.length < GROUP_FOLD_THRESHOLD) {
      // 不足阈值 → 全部展开
      for (const t of seg) result.push({ kind: 'task', key: t.id, task: t })
    } else {
      // ≥2 个非用户 task → 折叠成 group card
      const groupKey = `${curBy}-${seg[0].id}`
      const expanded = expandedGroupKeys.value.has(groupKey)
      result.push(buildGroupItem(groupKey, seg))
      if (expanded) {
        for (const t of seg) result.push({ kind: 'task', key: t.id, task: t })
      }
    }
    i = j
  }
  return result
})
```

**GROUP_FOLD_THRESHOLD = 2**（1 个不折叠，避免 UI 抖动）。

## 四、Group Card 显示

- 触发器 icon（cogOutline for automation / hardwareChipOutline for ai_agent）
- 标题：`自动化测试 · N 个任务` / `AI agent · N 个任务`
- 状态徽章：✓ 通过 / ✗ 失败 / ▶ 运行中 / ⋯ 待处理
- 时间戳：最后创建时间
- 右侧 chevron 按钮（chevronForward = 折叠，chevronBack = 展开）

## 五、阈值与边界

| 情况 | 行为 |
|------|------|
| 1 个非用户 task | 不折叠，正常显示 |
| ≥2 个连续同 triggeredBy | 折叠成 1 张 group card |
| 混合 user + automation 段 | 分别处理（user 独立卡片 + automation 折叠） |
| 用户搜/筛时 | 折叠前数据 (filteredTasks) 不变，displayedItems 重算 |
| 用户已展开 group | 临时插入 N 张 task，expandedGroupKeys 存 localStorage（可选） |

## 六、为什么不改后端

| 反对后端 group 字段的理由 | 说明 |
|--------------------------|------|
| **破坏 flat list 性能假设** | 后端 task store 假设 task 是平铺的，O(1) list；加 group 需 groupId 索引 + 重新组织 |
| **要同步改 WebSocket 推送** | task:update 事件需带 groupId 字段，Ai agent 端也要改 |
| **自动化测试和 AI agent 已是**显式**会话** | 自动化测试有 useAutomationTestsStore.results 维护状态；AI agent 有 useAgentStore；后端 group 是冗余 |
| **UI 层能完全解决** | triggeredBy + localStorage 标记已有，纯前端折叠成本极低 |
| **未来真要加后端** | 用 `Tags []string` 字段 + `POST /api/tasks/batch` 创建，**不要用 groupId**（flat + tag 更灵活） |

## 七、跨层参考

| 主题 | 文档位置 |
|------|---------|
| 任务触发者标记 (triggeredBy) | [src/composables/useTaskTrigger.ts](file:///workspace/app/encv-mobile/src/composables/useTaskTrigger.ts) |
| 自动化测试 entry | [src/composables/useAutomationTests.ts](file:///workspace/app/encv-mobile/src/composables/useAutomationTests.ts) |
| Tasks 列表 UI | [src/views/Tasks.vue](file:///workspace/app/encv-mobile/src/views/Tasks.vue) |
| 任务管理后端 | [internal/service/task_manager.go](file:///workspace/internal/service/task_manager.go) |

## 八、扩展铁律

> **任何批量创建 task 的代码（for 循环调 createTask）必须在调用前**显式 mark triggeredBy**。**
> **否则 task 列表默认会按时间排序，1 张 task 卡片 ≠ 1 个用户操作 → 用户看不懂**。
