# Tasks — UI 丑化修复 + Tasks 性能优化 + 加解密任务适配 v2

> 本 tasks 是 `unify-workflow-task-service` 第二轮修复的任务分解。前置依赖：第一轮 spec 已完成（服务统一、类型枚举化、组件抽取）。

## Task 1: 创建 design token 基础（timeline-tokens.css）

**目的**：为所有时间线/树/卡片组件提供 light/dark 双主题变量。

**修改**：
- 新增 `app/encv-mobile/src/styles/timeline-tokens.css`，定义 `:root` 和 `body.dark` 的 design token：
  - `--card-bg-gradient-start` / `--card-bg-gradient-end`（卡片背景渐变）
  - `--card-border`（极细半透明边框）
  - `--card-text-primary` / `--card-text-secondary`（文字颜色）
  - `--card-font-mono`（等宽字体栈）
  - `--card-radius`（8px 圆角）
  - `--state-color-*`（9 个 phase 状态色 + failed/cancelled）
- 在 `app/encv-mobile/src/main.ts` 引入该 CSS

**验收**：
- light 模式：`--card-bg-gradient-start: #FAFBFC`，`--card-text-primary: #1A1D21`
- dark 模式：`--card-bg-gradient-start: #0F1419`，`--card-text-primary: #E0E0E0`
- CSS 变量在浏览器 DevTools 可见

---

## Task 2: PhaseIcon 全用 ion-icon（删除 emoji/Unicode）

**目的**：统一图标体系，删除所有 emoji（⚡⏳⚙📄）和 Unicode 符号（●○▲▼）。

**修改**：
- `app/encv-mobile/src/components/shared/PhaseIcon.vue`：phase → ion-icon 映射改为：
  - created → `cloud-upload-outline`
  - analyzing → `search-outline`
  - initializing → `play-outline`
  - preprocessing → `code-slash-outline`
  - encrypting → `lock-closed-outline`
  - decrypting → `lock-open-outline`
  - packing → `cube-outline`
  - verifying → `shield-checkmark-outline`
  - completed → `checkmark-circle-outline`
  - failed → `close-circle-outline`
  - cancelled → `ban-outline`
- 删除 emoji / Unicode 分支
- size 通过 props 控制（默认 16px）

**验收**：
- 所有 phase 渲染为 ion-icon，无 emoji/Unicode
- 单测覆盖 11 个 phase 映射

---

## Task 3: PhaseBadge 暗黑模式保留状态色

**目的**：修复暗黑模式抹平所有状态色的问题。

**修改**：
- `app/encv-mobile/src/components/shared/PhaseBadge.vue`：
  - 背景改为 `rgba(var(--state-color-rgb), 0.15)`（light/dark 一致）
  - 文字用状态色（`var(--state-color)`）
  - 删除 dark 模式抹平为灰色的覆盖样式

**验收**：
- dark 模式下 encrypting 仍为紫色，completed 仍为绿色
- 单测覆盖 light/dark 双主题

---

## Task 4: UnifiedTimelineCard 样式重写

**目的**：修复 4px 左边框过重、嵌套卡片、暗黑模式背景过透明等问题。

**修改**：
- `app/encv-mobile/src/components/shared/UnifiedTimelineCard.vue`：
  - 删除左侧 4px 状态色边框，改为顶部 2px 渐变状态色条
  - 背景改为 `linear-gradient(180deg, var(--card-bg-gradient-start), var(--card-bg-gradient-end))`
  - 边框改为 `1px solid var(--card-border)`
  - 展开详情去除嵌套卡片，改为左侧 2px 边线 + padding
  - 进度条样式与 TestReportHeader 统一（高度 4px，圆角 2px）
  - 暗黑模式背景不透明度提升（不用 0.04）

**验收**：
- light/dark 双主题视觉一致
- 顶部 2px 渐变条可见
- 展开详情无嵌套卡片
- 单测覆盖样式 class

---

## Task 5: TreeView 暗黑模式保留档案主题

**目的**：修复暗黑模式破坏米色档案主题的问题。

**修改**：
- `app/encv-mobile/src/components/automation/TreeView.vue`：
  - dark 模式背景改为 `#1A1D21`（非纯黑）
  - 文字改为 `#E0E0E0`
  - 展开图标用 ion-icon `chevron-forward` / `chevron-down`
  - 删除 Unicode 展开符号（▶▼）

**验收**：
- dark 模式下背景为深灰（#1A1D21），非纯黑
- 展开图标为 ion-icon
- 单测覆盖展开/折叠状态

---

## Task 6: StepInlineTimeline 用 UnifiedTimelineCard 骨架

**目的**：统一 step 内联时间线视觉。

**修改**：
- `app/encv-mobile/src/components/automation/StepInlineTimeline.vue`：
  - 用 UnifiedTimelineCard 骨架渲染每个 phase 条目
  - 删除 StepMiniBadge 自定义 SVG
  - 用 PhaseIcon 替代自定义图标

**验收**：
- step 展开后内联时间线与任务时间线视觉统一
- 无自定义 SVG
- 单测覆盖

---

## Task 7: MockGenLogCard 背景适配主题

**目的**：修复强制深色终端背景与页面主题冲突的问题。

**修改**：
- `app/encv-mobile/src/components/developer/MockGenLogCard.vue`：
  - 背景改为 `linear-gradient(180deg, var(--card-bg-gradient-start), var(--card-bg-gradient-end))`
  - runner 标识保留状态色但适配主题（light 模式用深色文字，dark 模式用浅色文字）
  - 字体保留 `var(--card-font-mono)`

**验收**：
- light 模式下背景为浅色渐变，文字深色
- dark 模式下背景为深色终端渐变，文字浅色
- runner 标识状态色保留

---

## Task 8: TaskTimeline 用 UnifiedTimelineCard 骨架

**目的**：统一任务时间线视觉，删除 emoji。

**修改**：
- `app/encv-mobile/src/components/TaskTimeline.vue`：
  - 用 UnifiedTimelineCard 骨架渲染每个 phase 条目
  - 删除 emoji（⚡⏳⚙📄）
  - 进度条+速率+ETA 样式与 FFMPEG 日志卡统一
  - 用 PhaseIcon 替代 emoji

**验收**：
- 任务时间线与 FFMPEG 日志卡视觉统一
- 无 emoji
- 单测覆盖

---

## Task 9: useTasksList 终态保护

**目的**：修复扁平状态更新越界问题（applyTaskUpdate/applyTaskCompleted 无终态保护）。

**修改**：
- `app/encv-mobile/src/composables/useTasksList.ts`：
  - `applyTaskUpdate`：加 `isTerminalStatus(current.status)` 检查，终态时丢弃事件
  - `applyTaskCompleted`：同上加终态保护
  - `applyTaskProgress`：同上加终态保护
  - 复用 `lib/workflow/state-machine.ts` 的 `isTerminalStatus`

**验收**：
- 已终态任务不被 update/completed/progress 覆盖
- 单测覆盖：终态任务收到 update 事件 → 状态不变

---

## Task 10: useTasksList 乱序缓冲（pendingEvents Map）

**目的**：修复 WS 事件乱序到达（task:update 先于 task:created）时事件丢失。

**修改**：
- `app/encv-mobile/src/composables/useTasksList.ts`：
  - 新增 `pendingEvents: Map<string, Array<{ type: 'update' | 'progress' | 'completed'; data: any }>>`
  - `applyTaskUpdate` / `applyTaskProgress` / `applyTaskCompleted`：idx=-1 时缓存到 pendingEvents
  - `applyTaskCreated`：创建后回放 pendingEvents 中该 task id 的所有缓存事件，然后清除

**验收**：
- task:update 先于 task:created 到达 → created 后状态正确
- 单测覆盖：乱序场景

---

## Task 11: useTasksList fetchTasks 保留实时状态

**目的**：修复 fetchTasks 整体替换丢失实时状态（progress/phase/speed/eta）。

**修改**：
- `app/encv-mobile/src/composables/useTasksList.ts`：
  - `fetchTasks`：合并本地实时状态与远端状态，本地状态优先
  - 实现：`const localMap = new Map(tasks.value.map(t => [t.id, t]))`，`tasks.value = remote.map(t => localMap.get(t.id) ? { ...t, ...localMap.get(t.id) } : t)`

**验收**：
- 任务运行中 fetchTasks → progress/phase/speed/eta 不丢失
- 单测覆盖

---

## Task 12: useTasksList shallowRef + 预构建索引

**目的**：优化性能，避免深层响应式和重复计算。

**修改**：
- `app/encv-mobile/src/composables/useTasksList.ts`：
  - `tasks` 从 `ref` 改为 `shallowRef`
  - 新增 `tasksByRunId` computed（Map<runId, EncvTask[]>）
  - 新增 `triggeredByCache` computed（Map<taskId, { by, color, icon }>，避免每 render 6 次 localStorage 查找）
  - `applyTaskProgress` 局部 patch：直接修改数组元素字段，通过 `tasks.value = [...tasks.value]` 触发 shallowRef 更新

**验收**：
- shallowRef 生效（Vue DevTools 确认非深层响应式）
- tasksByRunId 索引正确
- triggeredByCache 缓存命中
- 单测覆盖

---

## Task 13: Tasks.vue 删除调试代码

**目的**：删除生产代码中的调试栏和 debug computed。

**修改**：
- `app/encv-mobile/src/views/Tasks.vue`：
  - 删除 `grouping-debug-bar`（L146-162）
  - 删除 3 个 debug computed（L984-1004）
  - 删除 `showAutomationReports` + `reportAutomationToBackend`（L1035-1131，已迁移到 useWorkflowTaskService）

**验收**：
- 生产代码无调试栏
- 无 debug computed
- 功能不受影响（showAutomationReports 已迁移）

---

## Task 14: 新增 TaskVirtualList 组件（@tanstack/vue-virtual）

**目的**：封装虚拟滚动逻辑，复用 DevLogs 的 scrollEl 获取模式。

**修改**：
- 新增 `app/encv-mobile/src/components/tasks/TaskVirtualList.vue`：
  - 接受 `tasks` + `scrollEl` props
  - 用 `useVirtualizer` 配置：`estimateSize: () => 80`、`overscan: 20`、`measureElement` 自动测量
  - 白屏优化：`content-visibility: auto` + `contain-intrinsic-size: 80px`
  - 暴露 `forceMeasure()` 给父级兜底
  - 参考 `VirtualLogList.vue` 的实现模式

**验收**：
- 200-500 task 渲染时 DOM 节点恒定（视口内 + overscan 20）
- 任务卡片展开/折叠时高度自动 re-measure
- 单测覆盖

---

## Task 15: Tasks.vue 集成虚拟滚动 + 局部重排

**目的**：集成 TaskVirtualList，优化分组/排序性能。

**修改**：
- `app/encv-mobile/src/views/Tasks.vue`：
  - 复用 DevLogs 的 `ensureScrollEl()` + ResizeObserver 兜底逻辑获取 ion-content shadowRoot 的 `.inner-scroll`
  - 用 `TaskVirtualList` 替换原 `ion-list` + `v-for`
  - 新增 `sortedIndices` computed（预计算排序索引，避免每次比较重新创建 Date）
  - `displayedItems` 用 `sortedIndices` 映射
  - `groupedItems` 用 `tasksByRunId` 索引 O(n) 分组（消除内层 find 退化 O(n²)）

**验收**：
- 200-500 task 滚动流畅（120Hz 真机无白屏）
- 排序/分组性能提升（DevTools Performance 无长任务）
- 单测覆盖 sortedIndices / groupedItems

---

## Task 16: 后端扩展 Task 结构体（加解密参数持久化）

**目的**：后端持久化 cipherMode/compressionMode/extraFields，返回时包含。

**修改**：
- `internal/service/task_manager.go`：
  - `Task` 结构体新增 `CipherMode string` / `CompressionMode string` / `ExtraFields map[string]string`（json tag）
  - `createTask` 时从请求参数提取并持久化
  - `getTasks` / `getTask` 返回时包含这些字段
- 关联文件（如 task 持久化存储）同步修改

**验收**：
- createTask 传入 cipherMode/compressionMode/extraFields → 持久化
- getTasks 返回的 task 包含这些字段
- Go 单测覆盖

---

## Task 17: 前端 EncvTask 接口扩展

**目的**：前端 EncvTask 接口同步扩展加解密参数字段。

**修改**：
- `app/encv-mobile/src/api/encv.ts`：
  - `EncvTask` 接口新增 `cipherMode?: string` / `compressionMode?: string` / `extraFields?: Record<string, string>`
- `app/encv-mobile/src/composables/useWorkflowTaskService.ts`：
  - `submitAction` 确保传递 cipherMode/compressionMode/extraFields 给 createTask API（已有逻辑，确认无遗漏）

**验收**：
- EncvTask 接口包含新字段
- TypeScript 编译通过
- 单测覆盖

---

## Task 18: 任务卡片展示加解密参数

**目的**：在任务卡片折叠态/展开态展示加解密参数。

**修改**：
- `app/encv-mobile/src/components/TaskBasicInfo.vue`：
  - 新增加解密参数区块，显示 `cipherMode` / `compressionMode` / `extraFields`
  - 用 UnifiedTimelineCard 的 design token 样式
- `app/encv-mobile/src/components/TaskTimeline.vue`：
  - 折叠态副标题显示 `cipherMode` / `compressionMode` 摘要（如 `AES-256 | zstd`）
- `app/encv-mobile/src/views/Tasks.vue`：
  - 任务卡片副标题显示摘要

**验收**：
- 折叠态显示摘要
- 展开态显示完整参数
- 刷新页面后参数回显（后端已持久化）
- 单测覆盖

---

## Task 19: useTestCaseGeneration 笛卡尔积扩展（视 plugin 元数据而定）

**目的**：调查 plugin 元数据是否暴露 cipherMode/compressionMode 候选值，若暴露则扩展笛卡尔积。

**修改**：
- 调查 `plugin.taskOptions` 是否包含 `cipherMode` / `compressionMode` 候选值
- 若暴露：
  - `app/encv-mobile/src/composables/useTestCaseGeneration.ts`：笛卡尔积展开包含 `cipherMode` × `compressionMode` × `extraFields`
- 若未暴露：
  - 保持现状（只派生 extraFields），加解密参数仅用于展示回显
  - 在 spec 备注说明

**验收**：
- 调查结论记录在 tasks.md
- 若扩展：单测覆盖笛卡尔积包含 cipherMode/compressionMode
- 若未扩展：备注原因

**调查结论（2026-06-18）**：**未暴露**，保持现状。

后端 7 个插件（video / audio / image / pdf / wps / text / alistencrypt）的 `GetTaskOptions()` 返回的 `ExtraFields` 均不包含 `cipherMode` / `compressionMode` 字段：

| 插件 | ExtraFields keys |
|------|------------------|
| video | stream_preset, encrypt_filename, fn_rounds, fn_charset, fn_deconfuse, fn_structured（6 个） |
| audio / image / pdf / wps / text | encrypt_filename, fn_rounds, fn_charset, fn_deconfuse, fn_structured（各 5 个） |
| alistencrypt | plugin_password, encode_filename, enc_type（3 个） |

`cipherMode` / `compressionMode` 实际处理位置：
- **后端**：作为 `MobileTask` 结构体顶层字段（`task_manager.go` L54-59），通过 `CreateWithCryptoParams()` 持久化，API 请求体也是顶层字段（`mobile_api.go` L460-462）
- **前端**：`NewTaskModal` / `EncryptBody.vue` 中硬编码 radio group（AES-128/AES-256、none/zstd），仅 v4 容器显示，不通过 `extraFields` 动态渲染

**决策**：保持 `useTestCaseGeneration` 现状（仅从 `extraFields` 派生笛卡尔积）。`cipherMode` / `compressionMode` 仅用于任务详情展示回显（Task 18 已实现）。

**已知限制**：自动化测试不会覆盖 `cipherMode` × `compressionMode` 维度组合。如未来需要覆盖，有两种路径：
1. 后端插件 `GetTaskOptions()` 暴露这两个字段为 `select` 类型（对齐 automation-workflow 规则 §三）
2. 前端 `useTestCaseGeneration` 硬编码 v4 容器的 cipher/compression 维度（违背"零硬编码"原则，不推荐）

当前选择路径 0（不覆盖），因为加解密参数的正确性已由后端单测 + 真机手动验证覆盖。

---

## Task 20: 真机验证 + 回归测试

**目的**：真机验证虚拟滚动无白屏，主题一致性，加解密参数回显。

**修改**：
- 真机验证（120Hz 可变刷新率）：
  - Tasks.vue 滚动 200-500 task，快速滚动无白屏
  - light/dark 双主题切换，所有组件视觉一致
  - 加解密任务提交后，刷新页面参数回显
- 回归测试：
  - 运行 `pnpm test`（前端单测）
  - 运行 `bash scripts/test-go.sh ./internal/service`（后端单测）
  - 运行 lint / typecheck

**验收**：
- 真机无白屏
- 主题一致
- 加解密参数回显
- 所有单测通过
- lint / typecheck 通过

---

## 执行顺序

```
Task 1 (design token)
  ↓
Task 2-8 (UI 样式重写，可并行)
  ↓
Task 9-11 (状态机修复，可并行)
  ↓
Task 12 (shallowRef + 索引)
  ↓
Task 13 (删除调试)
  ↓
Task 14 (TaskVirtualList 组件)
  ↓
Task 15 (Tasks.vue 集成虚拟滚动)
  ↓
Task 16-17 (加解密前后端扩展，可并行)
  ↓
Task 18 (展示加解密参数)
  ↓
Task 19 (笛卡尔积扩展)
  ↓
Task 20 (真机验证 + 回归)
```
