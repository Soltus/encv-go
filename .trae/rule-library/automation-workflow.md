# automation-workflow 详情

> 本文件为 [automation-workflow.md](../rules/automation-workflow.md) 的详情文档。
>
> 索引位于 [`.trae/rules/automation-workflow.md`](../rules/automation-workflow.md)。本文件汇总索引未包含的 5 + 5 + 4 = 14 个 bug 完整表 + 5 段代码模式 A-E + 完整新实现 60 行代码 + DAG 拆 2 job + 实时持久化双保险 + 任务组 UI 完整规范。

---

## 一、5 个历史 bug（v1 - 2026-06-10 同日连修）

| # | 症状 | 根因 | 修复 |
|---|------|------|------|
| **#1** | 测试报告状态不刷新（running 期间 progress=0%） | [useWorkflowEngine.ts](file:///workspace/app/encv-mobile/src/composables/useWorkflowEngine.ts) `startListening()` **只监听 `task:completed`** | 加 `task:update` / `task:progress` / `task:created` 监听，引入 `findStepByTaskId()` 状态机升级 |
| **#2** | 测试结果刷新页面丢失 | [useAutomationTests.ts](file:///workspace/app/encv-mobile/src/composables/useAutomationTests.ts) results 只在内存 | 新增 `persistCurrentRun()` + `getPersistedRuns()` + `clearPersistedRuns()`，localStorage key `encv_automation_results_v1`，最多 50 次 run |
| **#3** | 任务组 group card 简陋、跟普通 task 区分度低 | [Tasks.vue](file:///workspace/app/encv-mobile/src/views/Tasks.vue) group card 是简单 ion-item | 加 tone (`automation` / `ai_agent`)、icon-bubble、左侧 4px 渐变 border、group-progress-track、% 文本 + checkmark icon |
| **#4** | 所有 plugin 的加解密任务都走 sample.mp4 | `buildDynamicWorkflow()` **写死** `sourcePath: DEFAULT_AUTOMATION_SOURCE` | 加 `categoryForExt()` ext→目录分类映射，按 `plugin.supportedExtensions[0]` 选源 |
| **#5** | 遍历加密选项承诺没生效 | `buildDynamicWorkflow()` 硬编码 `cipherMode=[0,1] / compressionMode=['none','zstd']`（v4 only） | 改为遍历 `plugin.taskOptions.extraFields`：type=select 笛卡尔积、type=bool 2^N，**删 v4 硬编码** |

---

## 二、test 报告状态同步 — 4 件套监听铁律

```ts
// 缺一不可（修复前只监 task:completed）
function startListening() {
  eventBus.on('task:completed', onTaskCompleted)
  eventBus.on('task:update', onTaskUpdate)        // 🆕 状态机升级 pending→queued→running
  eventBus.on('task:progress', onTaskProgress)    // 🆕 进度% / phase / speed / eta
  eventBus.on('task:created', onTaskCreated)      // 🆕 确认后端已收
}
```

**后端推送的 task 事件**（[internal/service/task_manager.go](file:///workspace/internal/service/task_manager.go)）：

| 事件 | payload | 触发时机 |
|------|---------|---------|
| `task:created` | `{id, type, sourcePath}` | submit 后立即 |
| `task:update` | `{id, status, type, progress}` | status 变化（queued/running/cancelling/cancelled） |
| `task:progress` | `{id, progress, phase, speed, eta}` | 进度推（每 N% / 每 Ns） |
| `task:completed` | `{id, error?}` | 终态 |

**前端的 useWebSocket 透传**：message.type → eventBus.emit(type, data)。所以**任意前端的 useTaskEventBridge / useWorkflowEngine / useAutomationTests 都必须全订阅 4 件套**。

---

## 三、动态工作流构建 — 消除硬编码

### 3.1 旧实现（❌ 硬编码 v4 cipher + 写死 sample.mp4）

```ts
for (const plugin of plugins.value) {
  for (const taskType of ['encrypt', 'decrypt'] as const) {
    for (const version of versions) {
      const isV4 = version === 4
      const cipherModes = isV4 && taskType === 'encrypt' ? [0, 1] : [undefined]  // ❌ 硬编码
      const compressionModes = isV4 && taskType === 'encrypt' ? ['none', 'zstd'] : [undefined]  // ❌ 硬编码
      steps.push({
        action: { params: { sourcePath: DEFAULT_AUTOMATION_SOURCE, ... } }  // ❌ 一刀切
      })
    }
  }
}
```

### 3.2 新实现（✅ 动态遍历）

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

// 3. 笛卡尔积展开
for (const taskType of ['encrypt', 'decrypt'] as const) {
  for (const version of versions) {
    // 按 taskType 过滤 ExtraFields（Condition='encrypt' 字段只 encrypt 用）
    const taskSelectFields = selectFields.filter((sf) => !sf.field.condition || sf.field.condition === taskType)
    const taskBoolFields = boolFields.filter((bf) => !bf.field.condition || bf.field.condition === taskType)
    const selectCombos = cartesianExpand(taskSelectFields.map((sf) => sf.values))
    const boolCombos: boolean[][] = (taskBoolFields.length === 0)
      ? [[]]
      : Array.from({ length: 1 << taskBoolFields.length }, (_, mask) =>
          Array.from({ length: taskBoolFields.length }, (_, i) => Boolean(mask & (1 << i))))

    for (const selectCombo of selectCombos) {
      for (const boolCombo of boolCombos) {
        const extraFields: Record<string, string> = {}
        taskSelectFields.forEach((sf, i) => { extraFields[sf.field.key] = selectCombo[i] })
        taskBoolFields.forEach((bf, i) => { extraFields[bf.field.key] = boolCombo[i] ? 'true' : 'false' })

        steps.push({
          action: {
            type: 'encv_task',
            taskType,
            pluginName: plugin.name,
            params: { sourcePath, password, version, extraFields },
          },
        })
      }
    }
  }
}
```

### 3.3 ext → 目录分类映射（避免一刀切 sample.mp4）

| ext | category | sample 文件 |
|-----|----------|------------|
| mp4 / mkv / avi / mov / webm / flv / wmv | `video` | `sample.mp4` / `comedy.mkv` |
| mp3 / flac / ogg / m4a / wav / aac / opus | `audio` | `sample.mp3` / `sample.flac` |
| png / jpg / jpeg / gif / webp / bmp / tiff | `image` | `sample.png` / `sample.jpg` |
| pdf | `pdf` | `sample.pdf` |
| doc / docx / xls / xlsx / ppt / pptx | `wps` | `sample.docx` |
| txt / md / rtf / log | `text` | `sample.txt` |
| encv / ae | `alist-encrypted` | `sample.encv` |
| 其他 | `misc` | （需要时再补） |

**策略**：每个 plugin 取 `supportedExtensions[0]`（避免笛卡尔积爆炸）。如果未来要遍历所有 ext，把 `const sourceExt = supportedExts[0]` 改为 `for (const sourceExt of supportedExts)` 即可。

---

## 四、代码模式 A：runWorkflow layer 内真正并行 + jobRun 立即 push

```ts
// ❌ 旧实现（串行）：第一个 job 卡住 → 后续所有 job 都不显示
for (const layerJobIds of layers) {
  for (const jobId of layerJobIds) {
    const jobRun = await executeJob(jobDef, def.env ?? {})  // 串行
    run.jobs.push(jobRun)  // 延后 push
  }
}

// ✅ 新实现：layer 内 Promise.all + 立即 push
for (const layerJobIds of layers) {
  const jobPromises = layerJobIds.map((jobId) => {
    const jobDef = def.jobs.find((j) => j.id === jobId)!
    const jobRun: JobRun = { id: genId(), jobDefId: jobDef.id, status: 'running', startedAt: new Date().toISOString(), steps: [] }
    run.jobs.push(jobRun)  // 🆕 立即 push（UI 立刻可见）
    return executeJob(jobDef, def.env ?? {}, jobRun).then(() => jobRun)
  })
  await Promise.all(layerJobPromises)  // 全部并发启动
}
```

---

## 四、代码模式 B：executeJob 内部 worker 池 + 限流

```ts
// 1. 先构造所有 stepRun → 立即 push（UI 立刻可见）
const executableSteps: ExecutableStep[] = []
for (const exec of stepExecutions) {
  const stepRun: StepRun = { id: genId(), stepDefId: stepDef.id, status: 'pending', startedAt: new Date().toISOString(), matrixVars: binding }
  jobRun.steps.push(stepRun)  // 立即 push
  executableSteps.push({ stepDef, binding, stepRun })
}

// 2. N 个 worker 共享 cursor 限流
const max = (jobDef.strategy && 'max' in jobDef.strategy) ? jobDef.strategy.max : 5
let cursor = 0
const worker = async (): Promise<void> => {
  while (cursor < executableSteps.length) {
    const idx = cursor++
    const ex = executableSteps[idx]
    if (ex) await runOneStep(ex)  // 提交 action
  }
}
const workerCount = Math.min(max, executableSteps.length)
await Promise.all(Array.from({ length: workerCount }, () => worker()))
```

---

## 四、代码模式 C：DAG 拆 2 job + unique subdir

```ts
// 🆕 safeId = 唯一子目录名
const makeSafeId = (extraFields: Record<string, string>): string => {
  const sortedKeys = Object.keys(extraFields).sort()
  const parts: string[] = [plugin.name, `v${version}`, sourceExt]
  for (const k of sortedKeys) parts.push(`${k}-${extraFields[k]}`)
  return parts.join('_').replace(/[^\w.-]/g, '_').replace(/_+/g, '_')
}

// encrypt: targetPath = 唯一子目录
encryptSteps.push({
  action: { type: 'encv_task', taskType: 'encrypt', params: {
    sourcePath,
    targetPath: `${mockRoot}02-test-output/${safeId}/`,  // 🆕 唯一
  }},
})

// decrypt: sourcePath = encrypt 产物路径
decryptSteps.push({
  action: { type: 'encv_task', taskType: 'decrypt', params: {
    sourcePath: `${mockRoot}02-test-output/${baseSafeId}/sample.${ext}.${plugin.containerExtension}`,
    targetPath: `${mockRoot}02-test-output/dec_${baseSafeId}/`,
  }},
})

// jobs: 拆 2 个 + needs
jobs: [
  { id: 'encrypt-all', strategy: { type: 'parallel', max: 5 }, steps: encryptSteps },
  { id: 'decrypt-all', needs: ['encrypt-all'], strategy: { type: 'parallel', max: 5 }, steps: decryptSteps },
]
```

---

## 四、代码模式 D：实时持久化双保险

```ts
// useAutomationTests.onTaskCompleted：每收到一个就写
function onTaskCompleted(data) {
    // ...改 result.status
    persistCurrentRun()  // 🆕 实时写
}

// useAutomationTests.runTests：每个 case 提交完也立即写一次
for (const spec of specs) {
    // ...createTask 提交
    persistCurrentRun()  // 🆕 提交阶段也不丢
}
```

---

## 四、代码模式 E：onTaskCompleted 放宽 step.status 校验

```ts
// ❌ 旧：强校验 status === 'running'（WS 时序错乱时 step 永远卡住）
if (step.taskId !== data.id || step.status !== 'running') continue

// ✅ 新：接受任何非终态 step
if (step.taskId !== data.id) continue
if (isTerminalStep(step.status)) continue
// 推到 success / failure / cancelled
```

---

## 七、任务组 group card UI 规范

| 元素 | 样式 | 用意 |
|------|------|------|
| 左侧 border | 4px 渐变（automation 蓝 / ai_agent 紫） | 一眼区分触发器 |
| icon-bubble | 40×40 圆形填充（automation primary / ai_agent secondary） | 比纯 ion-icon 大气 |
| title | `自动化测试 · 12 个任务` / `AI agent · 8 个任务` | 中文 trigger 名 + N |
| 徽章 | ✓ N / ✗ N / ▶ N（spinner）/ N | running 用 spinner 图标（动态感） |
| 进度条 | 6px 高度，渐变 fill | 跟 task 卡片对齐 |
| % 文本 | 右上角 monospace 数字 | 一眼读完成度 |
| chevron | 折叠=chevronForward，展开=chevronBack | 直观 |
| **tone='ai_agent'** | 紫色 secondary 色 | 区分 automation（蓝） |

**type 字段**（`DisplayItem` union）：

```ts
{ kind: 'group'; key: string; groupKey: string; tone: 'automation' | 'ai_agent'; tasks: EncvTask[]; summary: { passed; failed; running; pending; percent; latestCreatedAt } }
{ kind: 'task'; key: string; task: EncvTask }
```

---

## 八、5 个新 bug（v2 - 2026-06-10 同日连修）

| # | 症状 | 根因 | 修复 |
|---|------|------|------|
| **#1** | 文件夹长按菜单缺少删除操作 | [Files.vue:1041](file:///workspace/app/encv-mobile/src/views/Files.vue#L1040-L1052) `if (!file.isDirectory)` 阻止文件夹删除；后端 [mobile_service.go:186](file:///workspace/internal/service/mobile_service.go#L186-L195) `os.Remove` 只能删文件不能删非空目录 | 1) Files.vue 去掉 `!file.isDirectory` 条件；2) 后端检测是目录用 `os.RemoveAll`，文件继续 `os.Remove` |
| **#1b** | 重置 mock 数据没有任何效果 | [mock_generator.go:240-269](file:///workspace/internal/server/mock_generator.go#L238-L303) `handleMockResetGin` 只 `os.Remove` `generateMockSpecs("all")` 列出的具体文件；02-test-output 等子目录根本不删 | 改用 `filepath.WalkDir` 递归遍历 5 个已知子目录（01-plain-media / 02-alist-encrypt / 03-encv-containers / 04-boundary-test / **02-test-output**），保留目录结构，删除其中所有文件 |
| **#2** | 任务组显示混乱 | [Tasks.vue:465-507](file:///workspace/app/encv-mobile/src/views/Tasks.vue) `displayedItems` 用"连续同 triggeredBy 区段"折叠 → 当 user task 穿插在 automation task 中间时区段被切散成多个 group card | 不再依赖区段。扫描所有 task，**所有** triggeredBy != 'user' 的 task 归为 1 个 group；group key 锚定到最早 createdAt 的 task.id（稳定）；user task 永远单独展示 |
| **#3** | 测试报告不显示运行的 job | [useWorkflowEngine.ts:243-249](file:///workspace/app/encv-mobile/src/composables/useWorkflowEngine.ts) `runWorkflow` layer 内 `for layerJobIds: const jobRun = await executeJob(...)` 串行 → 第一个 job 卡住，下面所有 job 都不显示；`executeJob` 内部 `for-await submitAction` 也串行 → 200 个 case 全排队 | 1) `runWorkflow` 改 `Promise.all(layerJobIds.map(...))` 真正并行；2) `jobRun` 构造后**立即 push** 到 `run.jobs`（UI 立刻可见，不等 await）；3) `executeJob` 内部 N 个 worker 共享 cursor 轮转拉取（按 `jobDef.strategy.max` 限流） |
| **#4** | 承诺的工作流没有兑现，所有任务全部平铺并行不合理 | [AutomationTestsDetail.vue](file:///workspace/app/encv-mobile/src/views/AutomationTestsDetail.vue) `buildDynamicWorkflow` 只有 1 个 job `'test-all'` parallel max:5，所有 encrypt+decrypt step 平铺，没有 DAG 依赖 | 拆 2 个 job：`encrypt-all` (parallel max:5) + `decrypt-all` (parallel max:5, **needs: ['encrypt-all']**)；`resolveExecutionOrder` 自动排层；`scheduleDependentJobs` 监听 encrypt-all 完成事件启动 decrypt-all |
| **#5** | 遍历加密选项导致的产物冲突（同一 sourcePath 多次加密覆盖） | [AutomationTestsDetail.vue](file:///workspace/app/encv-mobile/src/views/AutomationTestsDetail.vue) `buildDynamicWorkflow` 所有 encrypt step 共享同一 `sourcePath`，不传 `targetPath` → 加密后 `sample.{ext}.{containerExt}` 多次覆盖 | 1) 每个 (plugin, version, ext, extraFields) 组合 → **唯一 safeId** = `plugin_v{ver}_{ext}_{k1-v1_k2-v2...}`（特殊字符替换为 `_`）；2) encrypt step `targetPath = ${mockRoot}02-test-output/${safeId}/`；3) decrypt step `sourcePath = ${mockRoot}02-test-output/${baseSafeId}/sample.${ext}.${plugin.containerExtension}`（baseSafeId 不含 extraFields，确保 decrypt 读 encrypt 产物） |

---

## 十、跨层参考

| 主题 | 文档位置 |
|------|---------|
| 工作流引擎 | [useWorkflowEngine.ts](file:///workspace/app/encv-mobile/src/composables/useWorkflowEngine.ts) |
| 工作流存储 | [useWorkflowStore.ts](file:///workspace/app/encv-mobile/src/composables/useWorkflowStore.ts) |
| 自动化测试入口 | [useAutomationTests.ts](file:///workspace/app/encv-mobile/src/composables/useAutomationTests.ts) |
| 自动化测试 UI | [AutomationTestsDetail.vue](file:///workspace/app/encv-mobile/src/views/AutomationTestsDetail.vue) |
| 类型定义 | [workflow/types.ts](file:///workspace/app/encv-mobile/src/lib/workflow/types.ts) |
| 状态机 | [workflow/stateMachine.ts](file:///workspace/app/encv-mobile/src/lib/workflow/stateMachine.ts) |
| 任务组折叠 | [task-group-collapse.md](../rules/task-group-collapse.md) |
| Mock 数据生成 | [mock-data-architecture.md](../rules/mock-data-architecture.md) |

---

## 十一、4 个新 bug（v3 - 2026-06-10 同日连修）

### Bug 列表

| # | 症状 | 根因 | 修复 |
|---|------|------|------|
| **#1** | 长按菜单删除失败 500 错误，逻辑过于简单没有考虑边界情况与安全防御 | 前端 [encv.ts:240](file:///workspace/app/encv-mobile/src/api/encv.ts) 只 throw `HTTP error! status: 500` 没读 response body；后端 [mobile_service.go:175](file:///workspace/internal/service/mobile_service.go) 没区分 NotFound/Permission/Other；absPath == servingDir 会被一锅端 | **后端**：先 stat、os.IsNotExist/IsPermission 显式分支、log fileCount/size、servingDir 根拒绝；**前端**：读 response body 透传 error 到 toast、文件夹二次确认、根目录拒绝 |
| **#2** | 任务组显示混乱（前两次修复 v1/v2 都不够） | v1：所有 non-user task 归 1 group → 新 run 抢老 group；v2：按 triggeredBy 粗聚拢 → 跨 run 仍混。根本问题：没按 **workflow run 维度** 分组 | **架构重构**：useTaskTrigger 扩展 runId 字段；useWorkflowEngine.executeJob / scheduleDependentJobs 把 run.id 传给 recordTriggeredBy；Tasks.vue displayedItems 改用 `getRunIdForTask()` 按 runId 索引到 Map<runId, Group>，O(n) 单次扫描；user task 永远单独；无 runId 走 legacy fallback |
| **#3** | 任务卡片显示已完成的任务一直显示运行中，两种视图模式无脑排列看不到状态更新 | useWorkflowEngine.onTaskCompleted line 154 强校验 `step.status === 'running'` 才更新 → 后端没发 task:update 时 step 永远停留在 'pending'，task:completed 找不到匹配 step | **放宽校验**：接受任何非终态 step（pending / queued / running / cancelling / skipped）都会被推到 success / failure / cancelled；onTaskUpdate 加**终态保护**（已 success/failure 的 step 不被降级到 running） |
| **#4** | 测试报告文件保存逻辑：是否实时写入？（用户自己看自己发现问题） | useAutomationTests.onTaskCompleted 改 result.status 后**没立即 persistCurrentRun**；runTests 循环也只在末尾调一次 → 200 case 跑 5 分钟期间，用户刷新/关 App 数据全丢 | **实时持久化双保险**：每个 task:completed 收到就 persistCurrentRun 一次；runTests 每个 case 提交完也立即写一次 → 提交阶段 + 运行阶段都不丢；useWorkflowEngine 路径本身已经每个 onTaskCompleted 调 store.updateRun → 实时持久化（OK） |

---

## 十二、关键代码模式（v3 实战）

#### A. 后端 DeleteFile 细化错误 + 安全防御

```go
// 🆕 2026-06-10：禁止删根 + 详细 stat + permission 区分
if absPath == s.servingDir {
    return &ForbiddenError{Err: errors.New("cannot delete the root of the serving directory")}
}

info, statErr := os.Stat(absPath)
if statErr != nil {
    if os.IsNotExist(statErr) { return &NotFoundError{Err: statErr} }
    if os.IsPermission(statErr) { return &PermissionError{Err: statErr} }
    return statErr
}

if info.IsDir() {
    // 删除前统计 fileCount（详细日志）
    fileCount := 0
    filepath.WalkDir(absPath, func(_ string, d os.DirEntry, _ error) error {
        if !d.IsDir() { fileCount++ }
        return nil
    })
    slog.Warn("removing directory", "fileCount", fileCount)
    err = os.RemoveAll(absPath)
} else {
    slog.Warn("removing file", "size", info.Size())
    err = os.Remove(absPath)
}
```

#### B. 前端 deleteFile 读 response body error

```ts
// ❌ 旧：丢了后端 error message
if (!response.ok) {
    throw new Error(`HTTP error! status: ${response.status}`)
}

// ✅ 新：读 JSON {error: ...} 透传给用户
let detail = ''
try {
    const data = await response.json()
    detail = data?.error || data?.message || ''
} catch {
    detail = (await response.text()).slice(0, 200)
}
throw new Error(detail || `HTTP error! status: ${response.status}`)
```

#### C. onTaskCompleted 放宽 step.status 校验

```ts
// ❌ 旧：强校验 status === 'running'（WS 时序错乱时 step 永远卡住）
if (step.taskId !== data.id || step.status !== 'running') continue

// ✅ 新：接受任何非终态 step
if (step.taskId !== data.id) continue
if (isTerminalStep(step.status)) continue
// 推到 success / failure / cancelled
```

#### D. 任务组按 workflow runId 分组（架构重构）

```ts
// useTaskTrigger 扩展：{ triggeredBy, runId?, recordedAt }
export function recordTriggeredBy(
  taskId: string,
  triggeredBy: TriggeredBy,
  runId?: string,  // 🆕
) { ... }

// useWorkflowEngine.executeJob 接收 runIdOverride
const _runId = _runIdOverride || jobRun.id
// ... recordTriggeredBy(task.id, 'automation', _runId)

// Tasks.vue displayedItems O(n) Map 索引
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
// group 按最早 createdAt 排序，活跃在前
// group key = `${tone}-${runId}`（稳定）
```

#### E. 测试报告实时持久化

```ts
// useAutomationTests.onTaskCompleted：每收到一个就写
function onTaskCompleted(data) {
    // ...改 result.status
    persistCurrentRun()  // 🆕 实时写
}

// useAutomationTests.runTests：每个 case 提交完也立即写
for (const spec of specs) {
    // ...createTask 提交
    persistCurrentRun()  // 🆕 提交阶段也不丢
}
```

---

## 十三、扩展铁律（来自 14 个 bug 实战）

> 任何"文件删除"API 必须前后端双重防御：
> - 后端：`os.IsNotExist` / `os.IsPermission` 显式分支、stat 先于 remove、servingDir 根拒绝
> - 前端：读 response body error 透传 toast、文件夹二次确认、根目录客户端拦截
> 
> 缺一 → 500 黑箱 + 用户不知道为啥删不掉 / 误删根目录

> 任何"WS 事件"消费方必须放宽状态匹配 + 加终态保护：
> - onTaskCompleted 接受任何非终态 step（不只 `running`）— 抗 WS 时序错乱
> - onTaskUpdate 加 isTerminalStep 早返回 — 抗 task:completed 后 task:update 降级
> 
> 强校验 `status === 'running'` → 后端漏发 task:update 时 step 永远卡住

> 任何"批量执行"系统必须实时持久化：
> - 提交阶段（每个 case 提交完）→ 立即写
> - 运行阶段（每个 task:completed）→ 立即写
> - 末尾（runTests 收尾）→ 再写一次
> 
> 只在末尾写 → 200 case 跑 5 分钟期间任何崩溃都丢数据

> 任何"任务组"UI 必须按** `workflow run.id` **分组**（不是 triggeredBy）。
> - triggeredBy 太粗：同一 run 的 task 跟其他 run 的 task 区分不开
> - runId 精细：每个 workflow run 一个 group card，跨 run 互不干扰
> 
> 扩展：useTaskTrigger 必须存 runId 字段；recordTriggeredBy 接受 runId 参数；Tasks.vue 用 getRunIdForTask() 索引

> 拆分：2026-06-11
