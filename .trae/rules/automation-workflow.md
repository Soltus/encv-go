# 自动化测试工作流规则

> 任何"测试报告 / 工作流运行"类 UI 必须监全 4 件套 WS 事件。批量执行必须实时持久化。动态生成测试用例必须从 plugin 元数据派生。
>
> **完整内容 + 14 个 bug 详情 + 5 段代码模式 + 类型扩展**：[详情文档](../rule-library/automation-workflow.md)

## 一、核心原则

> 自动化测试**不能**硬编码 `cipherMode` / `compressionMode` / `sourcePath` ——必须从 `plugin.taskOptions.extraFields` + `plugin.supportedExtensions` 派生。
> WS 状态变化必须全链路同步（`task:update` / `task:progress` / `task:created` / `task:completed` 4 件套全监）。
> 测试结果必须持久化到 localStorage（刷新页面 / 关 App 不丢失）。

---

## 二、4 件套事件监听铁律

```ts
// 缺一不可（修复前只监 task:completed）
function startListening() {
  eventBus.on('task:completed', onTaskCompleted)
  eventBus.on('task:update', onTaskUpdate)        // 🆕 状态机升级 pending→queued→running
  eventBus.on('task:progress', onTaskProgress)    // 🆕 进度% / phase / speed / eta
  eventBus.on('task:created', onTaskCreated)      // 🆕 确认后端已收
}
```

**后端推送的 task 事件**：

| 事件 | payload | 触发时机 |
|------|---------|---------|
| `task:created` | `{id, type, sourcePath}` | submit 后立即 |
| `task:update` | `{id, status, type, progress}` | status 变化（queued/running/cancelling/cancelled） |
| `task:progress` | `{id, progress, phase, speed, eta}` | 进度推（每 N% / 每 Ns） |
| `task:completed` | `{id, error?}` | 终态 |

**前端的 useWebSocket 透传**：`message.type` → `eventBus.emit(type, data)`。所以**任意前端的 `useTaskEventBridge` / `useWorkflowEngine` / `useAutomationTests` 都必须全订阅 4 件套**。

> onTaskCompleted 放宽校验（接受任何非终态 step）+ onTaskUpdate 终态保护 + 实时持久化双保险 → [详情文档 §三.1/§四.D](../rule-library/automation-workflow.md#四代码模式)

---

## 三、动态工作流构建（消除硬编码）

> 修复前 `buildDynamicWorkflow()` 写死 `sourcePath: DEFAULT_AUTOMATION_SOURCE` + `cipherMode=[0,1]` / `compressionMode=['none','zstd']` (v4 only)。
> 修复后按 `plugin.supportedExtensions[0]` 选源 + 遍历 `plugin.taskOptions.extraFields` 派生笛卡尔积。

```ts
// 1. 按 plugin.supportedExtensions 选源
const sourceExt = plugin.supportedExtensions[0]
const sourcePath = `${mockRoot.value}01-plain-media/${categoryForExt(sourceExt)}/sample.${sourceExt}`

// 2. 按 plugin.taskOptions.extraFields 派生笛卡尔积
const selectFields: { field: any; values: string[] }[] = []
const boolFields: { field: any }[] = []
for (const f of opts.extraFields ?? []) {
  if (f.type === 'select' && Array.isArray(f.options) && f.options.length > 1) {
    selectFields.push({ field: f, values: f.options })
  } else if (f.type === 'bool') {
    boolFields.push({ field: f })
  }
}

// 3. 笛卡尔积展开（详见详情文档）
```

**ext → 目录分类映射**：

| ext | category | sample 文件 |
|-----|----------|------------|
| mp4 / mkv / avi / mov / webm / flv / wmv | `video` | `sample.mp4` |
| mp3 / flac / ogg / m4a / wav / aac / opus | `audio` | `sample.mp3` |
| png / jpg / jpeg / gif / webp / bmp / tiff | `image` | `sample.png` |
| pdf | `pdf` | `sample.pdf` |
| doc/docx/xls/xlsx/ppt/pptx | `wps` | `sample.docx` |
| txt/md/rtf/log | `text` | `sample.txt` |
| encv / ae | `alist-encrypted` | `sample.encv` |
| 其他 | `misc` | — |

**策略**：每个 plugin 取 `supportedExtensions[0]`（避免笛卡尔积爆炸）。未来要遍历所有 ext 把 `supportedExts[0]` 改为 `for (const sourceExt of supportedExts)`。

> 完整新实现 60 行代码 + 旧 vs 新对比 → [详情文档 §三](../rule-library/automation-workflow.md#三动态工作流构建-消除硬编码)

---

## 四、StepRun 字段扩展

`StepRun` 加：

```ts
export interface StepRun {
  // ...原有
  progress?: number   // 🆕 由 task:update / task:progress 驱动
  phase?: string      // 🆕 task phase label
  speed?: string      // 🆕 速率（"12.5 MB/s"）
  eta?: string        // 🆕 剩余时间
}
```

`StepStatus` 加 `'cancelling'`（区分取消中 vs 已取消），`VALID_TRANSITIONS` 同步更新：

```ts
running: new Set(['cancelling', 'success', 'failure', 'cancelled', 'timed_out']),
cancelling: new Set(['cancelled', 'failure', 'success']),
```

`EncvTaskActionParams` 加 `extraFields?: Record<string, string>`，并把 `useWorkflowEngine.submitAction` 第 7 个参数从 `{}` 改为 `spec.params.extraFields ?? {}`。

---

## 五、本地持久化规范

**localStorage key**：`encv_automation_results_v1`（带版本号，方便未来 schema 迁移）

**数据格式**：

```ts
interface PersistedRun {
  id: string
  startedAt: string
  completedAt?: string
  totalCases: number
  passed: number
  failed: number
  skipped: number
  results: TestCaseResult[]
}
```

**裁剪策略**：按 `startedAt` 倒序，最多保留 50 次。防止 localStorage 撑爆。

**触发时机**：`runTests()` 所有 case 提交完后立即 `persistCurrentRun()`（不等 WS 回调）；`onTaskCompleted` 每个完成事件也立即 `persistCurrentRun()`（双保险）。

**清空**：`clearPersistedRuns()` 仅清测试历史，**不动** workflow definition / runs / triggeredBy 标记。

---

## 六、任务组 group card UI 规范

> **2026-06-10 修复 v2**（详见 §八）：按 `workflow run.id` 分组（不再按"连续同 triggeredBy 区段"）

**displayedItems 改用 `getRunIdForTask()` 按 runId 索引到 `Map<runId, Group>`**：

```ts
const groupsByRun = new Map<string, Group>()
for (const t of tasks) {
    const by = getTriggeredBy(t.id)
    if (by === 'user') { userTasks.push(t); continue }
    const runId = getRunIdForTask(t.id)
    if (runId) {
        const g = groupsByRun.get(runId)
        if (g) g.tasks.push(t)
        else groupsByRun.set(runId, { runId, tone: ..., tasks: [t] })
    }
    // fallback: legacy {triggeredBy 聚拢}
}
```

`useTaskTrigger` 扩展 `{ triggeredBy, runId?, recordedAt }` 字段；`useWorkflowEngine.executeJob` / `scheduleDependentJobs` 把 `run.id` 传给 `recordTriggeredBy`。

| 元素 | 样式 | 用意 |
|------|------|------|
| 左侧 border | 4px 渐变（automation 蓝 / ai_agent 紫） | 一眼区分触发器 |
| icon-bubble | 40×40 圆形填充 | 比纯 ion-icon 大气 |
| title | `自动化测试 · 12 个任务` | trigger 名 + N |
| chevron | 折叠=chevronForward | 直观 |

> UI 完整规范表（5 类元素 + tone='ai_agent' 紫色）+ type union + group key 锚定逻辑 → [详情文档 §七](../rule-library/automation-workflow.md#七任务组-group-card-ui-规范)

---

## 七、测试用例数估算（防爆）

| plugin 数 | supportedExts 策略 | taskType | version | select 笛卡尔积 | bool 2^N | 总数 |
|----------|-------------------|---------|---------|---------------|---------|------|
| 7 | 1 ext each | encrypt+decrypt | v2+v3+v4 | 0~3 fields × 2~4 options | 0~3 bool | **典型 200-500** |
| 7 | 全部 ext 展开 | encrypt+decrypt | v2+v3+v4 | ... | ... | **典型 2000+** |

**当前策略**（1 ext per plugin）= 200-500 case，并行 `max: 5`，后端可承载。

---

## 八、扩展铁律（核心 5 条）

> 任何"执行"类代码（for-await / .then 链）必须显式并行：layer 用 `Promise.all`、step 用 worker 池、case 用并发队列。
> 任何"创建 run/job/step"代码必须立即 push 到 reactive 容器，让 UI 立刻看到。
> 任何"文件操作"必须先 `os.Stat` 判断是文件还是目录。
> 任何"DAG 拆 job"必须用 `needs` 字段。
> 任何"WS 事件"消费方必须放宽状态匹配 + 加终态保护。
> 任何"批量执行"系统必须实时持久化（提交阶段 + 运行阶段 + 末尾）。
> 任何"任务组"UI 必须按 `workflow run.id` 分组（不是 triggeredBy）。

> 14 个 bug 完整表（5 v1 + 5 v2 + 4 v3）+ 5 段代码模式 A-E + DAG 拆 2 job + 实时持久化双保险 → [详情文档 §九/§十/§十一/§十二](../rule-library/automation-workflow.md)
## 九、相关规则

- [task-group-collapse.md](task-group-collapse.md) — 任务组折叠
- [mock-data-architecture.md](mock-data-architecture.md) — mock 架构
- [saturation-debugging.md](saturation-debugging.md) — 饱和调试
- [test-master-plan.md](test-master-plan.md) — **测试体系总纲**：Cypress 性能测试方法论、层级定位
- [test-orchestration.md](test-orchestration.md) — 测试编排守卫、合法入口

> 拆分：2026-06-11
> 更新：2026-07-01（新增 test-master-plan / test-orchestration 引用）
