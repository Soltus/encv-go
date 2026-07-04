# UI 丑化修复 + Tasks 性能优化 + 加解密任务适配 Spec v2

> 本 spec 是 `unify-workflow-task-service` 的第二轮修复，合并原 spec 已完成工作并修复用户反馈的丑化、性能、状态机、加解密适配问题。

## Why

第一轮 `unify-workflow-task-service` spec 完成了服务统一、类型枚举化、组件抽取，但用户反馈：

1. **UI 丑化（核心问题）**：UnifiedTimelineCard 的 4px 状态色左边框过重、emoji（⚡⏳⚙📄）与 ion-icon 混用违和、卡片化展开详情间距过大+嵌套卡片、暗黑模式卡片背景过透明（0.04）、PhaseBadge 暗黑模式抹平所有状态色、TreeView 暗黑模式破坏米色档案主题、MockGenLogCard 强制深色终端背景与页面主题冲突、5 种图标系统混用（emoji/ion-icon/Unicode/PhaseIcon/StepMiniBadge）。用户明确说"我要的是美化，实际效果是丑化"，并指定以**重构前 FFMPEG 日志卡深色终端风格**为现代审美基准（唯一问题是默认黑色未适配主题）。

2. **Tasks.vue 未真正重构**：Task 16 仅做了 `showAutomationReports` 数据源迁移，未解决核心问题：
   - **扁平状态更新越界**：`useTasksList.applyTaskUpdate` 无终态保护，`idx=-1` 时静默丢弃；`applyTaskCompleted` 同样无保护；`fetchTasks` 整体替换丢失实时状态；WS 事件乱序到达（`task:update` 先于 `task:created`）时直接丢失。
   - **加载大量任务性能差**：无虚拟滚动，200-500 task 全量 DOM；`displayedItems` computed 最坏 O(n²)，内层 `find` 退化；`allGroups.sort` 每次比较都重新创建 Date 对象 + flatMap；同一 task id 调 `getTriggeredBy` 4 次 + `getTriggeredByColor/Icon` 各 1 次（每 task 每 render 6 次 localStorage 查找）；3 个 debug computed + grouping-debug-bar 调试栏仍在生产代码中。

3. **加解密任务未适配新架构**：`EncvTask` 接口（`encv.ts` L567-595）不含 `cipherMode` / `compressionMode` / `extraFields` 字段；`submitAction`（`useWorkflowTaskService.ts` L600-617）传递这些参数给 createTask API 但返回的 `EncvTask` 丢失；`TaskTimeline` / `TaskBasicInfo` / `Tasks.vue` 卡片均不显示加解密参数；刷新页面/WS 重连后参数无法回显。

## What Changes

### 一、UI 丑化修复（以重构前 FFMPEG 日志卡风格为基准）

#### 1.1 设计语言基准（重构前 MockGenLogCard 原始样式）

以 `MockGenLogCard.vue` 重构前样式为现代审美基准：

```css
/* 基准：深色终端渐变背景 */
background: linear-gradient(180deg, #0F1419 0%, #0A0E12 100%);
border-radius: 8px;
border: 1px solid rgba(255, 255, 255, 0.06);
font-family: ui-monospace, 'SF Mono', Menlo, Consolas, monospace;
color: #E0E0E0;

/* 状态色 runner 标识（紫/绿/灰） */
.runner--ffmpeg { background: rgba(139, 92, 246, 0.15); color: #8B5CF6; }
.runner--mediacodec { background: rgba(34, 197, 94, 0.15); color: #22C55E; }
.runner--static { background: rgba(100, 116, 139, 0.15); color: #64748B; }
```

**核心特征**：深色终端渐变背景、等宽字体、极细半透明边框、8px 圆角、紧凑布局、状态色 runner 标识、卡片化详情。

**唯一问题**：硬编码 `#0F1419` 黑色，light 模式下也是黑色，未适配主题。

#### 1.2 主题适配方案

引入 design token，light/dark 双主题：

```css
/* UnifiedTimelineCard 主题变量 */
:root {
  --card-bg-gradient-start: #FAFBFC;
  --card-bg-gradient-end: #F4F6F8;
  --card-border: rgba(0, 0, 0, 0.08);
  --card-text-primary: #1A1D21;
  --card-text-secondary: #5F6B7A;
  --card-font-mono: ui-monospace, 'SF Mono', Menlo, Consolas, monospace;
}
body.dark {
  --card-bg-gradient-start: #0F1419;
  --card-bg-gradient-end: #0A0E12;
  --card-border: rgba(255, 255, 255, 0.06);
  --card-text-primary: #E0E0E0;
  --card-text-secondary: #8B95A5;
}
```

#### 1.3 图标体系统一（全 ion-icon）

**删除**：所有 emoji（⚡⏳⚙📄✅❌⚠️🔄📦🔍）、Unicode 符号（●○▲▼）、StepMiniBadge 自定义 SVG。

**统一为 ion-icon**，phase 映射：

| Phase | ion-icon name | 说明 |
|---|---|---|
| created | `cloud-upload-outline` | 已提交 |
| analyzing | `search-outline` | 分析中 |
| initializing | `play-outline` | 初始化 |
| preprocessing | `code-slash-outline` | 预处理 |
| encrypting | `lock-closed-outline` | 加密中 |
| decrypting | `lock-open-outline` | 解密中 |
| packing | `cube-outline` | 打包中 |
| verifying | `shield-checkmark-outline` | 校验中 |
| completed | `checkmark-circle-outline` | 已完成 |
| failed | `close-circle-outline` | 失败 |
| cancelled | `ban-outline` | 已取消 |

#### 1.4 组件样式重写清单

| 组件 | 重写内容 |
|---|---|
| `UnifiedTimelineCard.vue` | 删除 4px 状态色左边框（改为顶部 2px 渐变条）；背景改为 design token 渐变；展开详情去除嵌套卡片（改为左侧 2px 边线 + padding）；进度条样式与 TestReportHeader 统一 |
| `PhaseBadge.vue` | 暗黑模式保留状态色（不抹平）；背景改为 `rgba(stateColor, 0.15)`；文字用状态色 |
| `PhaseIcon.vue` | 全用 ion-icon（删除 emoji/Unicode）；size 通过 props 控制 |
| `TreeView.vue`（UnifiedTreeView） | 暗黑模式保留米色档案主题（`#1A1D21` 背景 + `#E0E0E0` 文字，不强制纯黑）；展开图标用 `chevron-forward` / `chevron-down` |
| `StepInlineTimeline.vue` | 用 UnifiedTimelineCard 骨架；删除 StepMiniBadge 自定义 SVG |
| `MockGenLogCard.vue` | 背景改为 design token（light 模式浅色渐变，dark 模式深色终端渐变）；runner 标识保留状态色但适配主题 |
| `TaskTimeline.vue` | 用 UnifiedTimelineCard 骨架；删除 emoji；进度条+速率+ETA 样式与 FFMPEG 日志卡统一 |

### 二、Tasks.vue 状态机修复（扁平状态更新越界）

#### 2.1 终态保护

复用 `lib/workflow/state-machine.ts` 的 `applyTerminalGuard`：

```typescript
// useTasksList.ts
import { applyTerminalGuard, isTerminalStatus } from '@/lib/workflow/state-machine'

function applyTaskUpdate(data: Partial<EncvTask> & { id: string }) {
  const idx = tasks.value.findIndex(t => t.id === data.id)
  if (idx === -1) {
    // 乱序缓冲：created 未到达时缓存事件
    pendingEvents.set(data.id, [...(pendingEvents.get(data.id) ?? []), { type: 'update', data }])
    return
  }
  const current = tasks.value[idx]
  // 终态保护：已终态的任务不被 update 覆盖
  if (isTerminalStatus(current.status)) return
  tasks.value[idx] = { ...current, ...data }
}

function applyTaskCompleted(data: { id: string; error?: string }) {
  const idx = tasks.value.findIndex(t => t.id === data.id)
  if (idx === -1) {
    pendingEvents.set(data.id, [...(pendingEvents.get(data.id) ?? []), { type: 'completed', data }])
    return
  }
  const current = tasks.value[idx]
  if (isTerminalStatus(current.status)) return
  tasks.value[idx] = {
    ...current,
    status: data.error ? 'failed' : 'completed',
    completedAt: new Date().toISOString(),
    error: data.error,
  }
}

function applyTaskCreated(data: EncvTask) {
  // 去重检查
  if (tasks.value.some(t => t.id === data.id)) return
  tasks.value.unshift(data)
  // 回放乱序缓冲的事件
  const pending = pendingEvents.get(data.id)
  if (pending) {
    pendingEvents.delete(data.id)
    for (const evt of pending) {
      if (evt.type === 'update') applyTaskUpdate(evt.data)
      else if (evt.type === 'completed') applyTaskCompleted(evt.data)
    }
  }
}
```

#### 2.2 乱序缓冲（pendingEvents Map）

```typescript
const pendingEvents = new Map<string, Array<{ type: 'update' | 'completed' | 'progress'; data: any }>>()
```

`task:update` / `task:progress` / `task:completed` 先于 `task:created` 到达时，缓存到 `pendingEvents`，等 `task:created` 到达后回放。

#### 2.3 fetchTasks 保留实时状态

```typescript
async function fetchTasks() {
  const remote = await api.getTasks()
  // 合并而非替换：保留本地实时状态（progress/phase/speed/eta）
  const localMap = new Map(tasks.value.map(t => [t.id, t]))
  tasks.value = remote.map(t => {
    const local = localMap.get(t.id)
    return local ? { ...t, ...local } : t  // 本地状态优先（实时性更高）
  })
}
```

### 三、Tasks.vue 性能优化

#### 3.1 虚拟滚动（@tanstack/vue-virtual）

**方案**：`@tanstack/vue-virtual`（与 DevLogs 一致，复用 `VirtualLogList` 的 `scrollEl` 获取逻辑）。

**实现要点**：
- 复用 `DevLogs.vue` 的 `ensureScrollEl()` + ResizeObserver 兜底逻辑（获取 ion-content shadowRoot 的 `.inner-scroll`）
- `useVirtualizer` 配置：`estimateSize: () => 80`（折叠态默认高度）、`overscan: 20`（加大防止白屏）、`measureElement` 自动测量展开态高度
- 任务卡片展开/折叠时高度变化自动 re-measure（`measureElement` ref 绑定）
- 白屏优化：`overscan: 20` + `content-visibility: auto` + `contain-intrinsic-size: 80px`

**新增组件**：`components/tasks/TaskVirtualList.vue` —— 封装虚拟滚动逻辑，接受 `tasks` + `scrollEl` props。

#### 3.2 删除调试代码

- 删除 `grouping-debug-bar`（L146-162）
- 删除 3 个 debug computed（L984-1004）
- 删除 `showAutomationReports` + `reportAutomationToBackend`（L1035-1131，已迁移到 `useWorkflowTaskService`）

#### 3.3 shallowRef + 预构建索引

```typescript
// useTasksList.ts
const tasks = shallowRef<EncvTask[]>([])  // 替换 ref

// 预构建索引（tasks 变化时重建，O(n) 一次）
const tasksByRunId = computed(() => {
  const map = new Map<string, EncvTask[]>()
  for (const t of tasks.value) {
    if (t.runId) {
      const arr = map.get(t.runId) ?? []
      arr.push(t)
      map.set(t.runId, arr)
    }
  }
  return map
})

// 预构建 triggeredBy 索引（避免每 render 6 次 localStorage 查找）
const triggeredByCache = computed(() => {
  const map = new Map<string, { by: string; color: string; icon: string }>()
  for (const t of tasks.value) {
    if (!map.has(t.id)) {
      const by = getTriggeredBy(t.id)
      map.set(t.id, {
        by,
        color: getTriggeredByColor(by),
        icon: getTriggeredByIcon(by),
      })
    }
  }
  return map
})
```

#### 3.4 progress 局部 patch（不触发 sortedTasks 重排）

```typescript
function applyTaskProgress(data: { id: string; progress: number; phase?: string; speed?: string; eta?: string }) {
  const idx = tasks.value.findIndex(t => t.id === data.id)
  if (idx === -1) {
    pendingEvents.set(data.id, [...(pendingEvents.get(data.id) ?? []), { type: 'progress', data }])
    return
  }
  const current = tasks.value[idx]
  if (isTerminalStatus(current.status)) return
  // 局部 patch：直接修改数组元素的字段，不触发 sortedTasks 重排
  const updated = { ...current, ...data }
  tasks.value[idx] = updated
  // 触发 shallowRef 更新（浅层引用变化）
  tasks.value = [...tasks.value]
}
```

#### 3.5 局部重排（排序方式在局部也生效）

```typescript
// 排序索引（tasks 变化时重建）
const sortedIndices = computed(() => {
  const indices = tasks.value.map((_, i) => i)
  indices.sort((a, b) => {
    const ta = tasks.value[a]
    const tb = tasks.value[b]
    return sortByCreatedAt(ta, tb)  // 按 createdAt 降序
  })
  return indices
})

// displayedItems 用 sortedIndices 映射，避免每次比较都重新创建 Date
const displayedItems = computed(() => {
  return sortedIndices.value.map(i => tasks.value[i])
})
```

#### 3.6 分组逻辑优化（消除 O(n²)）

```typescript
// 旧：displayedItems 内层 find 退化 O(n²)
// 新：用 tasksByRunId 索引 O(n) 分组
const groupedItems = computed(() => {
  const groups: Array<{ runId: string; tasks: EncvTask[] }> = []
  const userTasks: EncvTask[] = []
  const seenRunIds = new Set<string>()

  for (const t of displayedItems.value) {
    if (!t.runId || t.triggeredBy === 'user') {
      userTasks.push(t)
      continue
    }
    if (!seenRunIds.has(t.runId)) {
      seenRunIds.add(t.runId)
      groups.push({ runId: t.runId, tasks: tasksByRunId.value.get(t.runId) ?? [] })
    }
  }
  return { groups, userTasks }
})
```

### 四、加解密任务适配新架构（前后端联动）

#### 4.1 后端扩展 EncvTask 返回字段

**修改** `internal/service/task_manager.go`：

```go
type Task struct {
    // ... 原有字段
    CipherMode      string            `json:"cipherMode,omitempty"`
    CompressionMode string            `json:"compressionMode,omitempty"`
    ExtraFields     map[string]string `json:"extraFields,omitempty"`
}
```

- `CipherMode` / `CompressionMode` 在 `createTask` 时从请求参数提取并持久化到 task 元数据
- `ExtraFields` 同样持久化
- `getTasks` / `getTask` 返回时包含这些字段

#### 4.2 前端 EncvTask 接口扩展

**修改** `app/encv-mobile/src/api/encv.ts`：

```typescript
export interface EncvTask {
  // ... 原有字段
  cipherMode?: string         // 🆕 加密模式（如 "0", "1"）
  compressionMode?: string    // 🆕 压缩模式（如 "none", "zstd"）
  extraFields?: Record<string, string>  // 🆕 额外参数
}
```

#### 4.3 前端展示加解密参数

**修改** `TaskBasicInfo.vue` / `TaskTimeline.vue` / `Tasks.vue` 卡片：

- 折叠态：在 task 卡片副标题显示 `cipherMode` / `compressionMode` 摘要（如 `AES-256 | zstd`）
- 展开态：在 `TaskBasicInfo` 增加加解密参数区块，显示完整 `cipherMode` / `compressionMode` / `extraFields`

#### 4.4 useTestCaseGeneration 笛卡尔积扩展

**修改** `useTestCaseGeneration.ts`：

当前只派生 `extraFields` 笛卡尔积。由于 `EncryptBody.vue` 中 `cipherMode` / `compressionMode` 是独立控件（不走 extraFields），需要：

- 从 plugin 元数据派生 `cipherMode` 候选值（如 plugin 支持 `[0, 1]`）
- 从 plugin 元数据派生 `compressionMode` 候选值（如 `['none', 'zstd']`）
- 笛卡尔积展开时包含 `cipherMode` × `compressionMode` × `extraFields`

**注意**：需先调查 plugin 元数据是否暴露 `cipherMode` / `compressionMode` 候选值。如果未暴露，则保持现状（只派生 extraFields），加解密参数仅用于展示回显。

## Impact

### 受影响代码

**UI 丑化修复**：
- `app/encv-mobile/src/components/shared/UnifiedTimelineCard.vue`（重写样式）
- `app/encv-mobile/src/components/shared/PhaseBadge.vue`（暗黑模式保留状态色）
- `app/encv-mobile/src/components/shared/PhaseIcon.vue`（全用 ion-icon）
- `app/encv-mobile/src/components/automation/TreeView.vue`（暗黑模式保留米色档案主题）
- `app/encv-mobile/src/components/automation/StepInlineTimeline.vue`（用 UnifiedTimelineCard 骨架）
- `app/encv-mobile/src/components/developer/MockGenLogCard.vue`（背景改 design token）
- `app/encv-mobile/src/components/TaskTimeline.vue`（用 UnifiedTimelineCard 骨架）
- `app/encv-mobile/src/App.vue` 或新增 `src/styles/timeline-tokens.css`（design token 定义）

**Tasks.vue 状态机修复**：
- `app/encv-mobile/src/composables/useTasksList.ts`（终态保护 + 乱序缓冲 + fetchTasks 合并）

**Tasks.vue 性能优化**：
- `app/encv-mobile/src/views/Tasks.vue`（虚拟滚动 + 删除调试 + shallowRef + 预构建索引 + 局部重排）
- `app/encv-mobile/src/components/tasks/TaskVirtualList.vue`（新增，封装虚拟滚动）
- `app/encv-mobile/src/composables/useTasksList.ts`（shallowRef + 预构建索引）

**加解密任务适配**：
- `internal/service/task_manager.go`（扩展 Task 结构体 + 持久化 + 返回）
- `app/encv-mobile/src/api/encv.ts`（EncvTask 接口扩展）
- `app/encv-mobile/src/components/TaskBasicInfo.vue`（展示加解密参数）
- `app/encv-mobile/src/components/TaskTimeline.vue`（折叠态摘要）
- `app/encv-mobile/src/views/Tasks.vue`（卡片副标题摘要）
- `app/encv-mobile/src/composables/useTestCaseGeneration.ts`（笛卡尔积扩展，视 plugin 元数据而定）

### 受影响 specs
- `unify-workflow-task-service`（第一轮，本 spec 合并并修复其遗留问题）
- `automation-workflow` 规则（4 件套订阅、状态机、持久化规范）

### 受影响规则
- `.trae/rules/automation-workflow.md`（状态机终态保护、乱序缓冲需补充到规则）

## ADDED Requirements

### Requirement: UI 设计语言统一为重构前 FFMPEG 日志卡风格

系统 SHALL 以重构前 `MockGenLogCard` 深色终端风格为现代审美基准，统一所有时间线/树/卡片组件的视觉语言，并适配 light/dark 主题。

#### Scenario: 深色终端渐变背景适配主题
- **WHEN** 用户在 light 模式查看 UnifiedTimelineCard / MockGenLogCard / TaskTimeline
- **THEN** 卡片背景为浅色渐变（`#FAFBFC` → `#F4F6F8`），文字为深色（`#1A1D21`）
- **AND** 边框为极细半透明深色（`rgba(0, 0, 0, 0.08)`）
- **WHEN** 用户切换到 dark 模式
- **THEN** 卡片背景为深色终端渐变（`#0F1419` → `#0A0E12`），文字为浅色（`#E0E0E0`）
- **AND** 边框为极细半透明浅色（`rgba(255, 255, 255, 0.06)`）

#### Scenario: 图标全统一 ion-icon
- **WHEN** 渲染任何 phase 图标、状态图标、操作图标
- **THEN** 使用 ion-icon（如 `lock-closed-outline` 表示 encrypting）
- **AND** 不使用 emoji（⚡⏳⚙📄）、Unicode 符号（●○▲▼）、自定义 SVG

#### Scenario: UnifiedTimelineCard 样式重写
- **WHEN** 渲染 UnifiedTimelineCard
- **THEN** 顶部为 2px 渐变状态色条（非左侧 4px 边框）
- **AND** 展开详情为左侧 2px 边线 + padding（非嵌套卡片）
- **AND** 进度条样式与 TestReportHeader 统一
- **AND** 暗黑模式背景不透明度过低（不用 0.04）

#### Scenario: PhaseBadge 暗黑模式保留状态色
- **WHEN** 在 dark 模式渲染 PhaseBadge
- **THEN** 背景为 `rgba(stateColor, 0.15)`
- **AND** 文字为状态色（不抹平为灰色）

#### Scenario: TreeView 暗黑模式保留档案主题
- **WHEN** 在 dark 模式渲染 UnifiedTreeView
- **THEN** 背景为 `#1A1D21`（非纯黑），文字为 `#E0E0E0`
- **AND** 展开图标用 `chevron-forward` / `chevron-down`

### Requirement: Tasks.vue 状态机终态保护与乱序缓冲

系统 SHALL 对 `useTasksList` 的扁平状态更新加终态保护，并对乱序到达的 WS 事件做缓冲回放。

#### Scenario: 终态保护
- **WHEN** 后端推送 `task:update` 或 `task:completed` 给已终态（completed/failed/cancelled）的任务
- **THEN** 事件被丢弃，不覆盖终态状态
- **AND** 不抛出错误

#### Scenario: 乱序缓冲
- **WHEN** `task:update` / `task:progress` / `task:completed` 先于 `task:created` 到达
- **THEN** 事件缓存到 `pendingEvents` Map
- **WHEN** `task:created` 到达后
- **THEN** 回放 `pendingEvents` 中该 task id 的所有缓存事件
- **AND** 清除该 task id 的缓存

#### Scenario: fetchTasks 保留实时状态
- **WHEN** `fetchTasks` 从后端拉取任务列表
- **THEN** 合并本地实时状态（progress/phase/speed/eta）与远端状态
- **AND** 本地状态优先（实时性更高）

### Requirement: Tasks.vue 虚拟滚动与性能优化

系统 SHALL 对 Tasks.vue 列表实施虚拟滚动，并优化分组/排序/索引性能。

#### Scenario: 虚拟滚动
- **WHEN** Tasks.vue 渲染 200-500 个任务
- **THEN** 使用 `@tanstack/vue-virtual` 虚拟滚动
- **AND** DOM 节点数恒定（视口内 + overscan 20）
- **AND** 任务卡片展开/折叠时高度自动 re-measure
- **AND** 快速滚动不白屏（overscan 20 + content-visibility: auto）

#### Scenario: 删除调试代码
- **WHEN** 生产环境运行 Tasks.vue
- **THEN** 不存在 `grouping-debug-bar`
- **AND** 不存在 3 个 debug computed
- **AND** 不存在 `showAutomationReports` / `reportAutomationToBackend`（已迁移）

#### Scenario: shallowRef + 预构建索引
- **WHEN** tasks 列表更新
- **THEN** 使用 `shallowRef` 避免深层响应式
- **AND** 预构建 `tasksByRunId` Map 索引（O(n) 一次）
- **AND** 预构建 `triggeredByCache` Map（避免每 render 6 次 localStorage 查找）

#### Scenario: progress 局部 patch
- **WHEN** 后端推送 `task:progress` 高频更新
- **THEN** 局部 patch 任务字段（不触发 sortedTasks 全量重排）
- **AND** 通过浅层引用变化触发 shallowRef 更新

#### Scenario: 局部重排
- **WHEN** 排序方式变化或新任务加入
- **THEN** 用 `sortedIndices` 预计算索引（避免每次比较重新创建 Date）
- **AND** `displayedItems` 用索引映射

#### Scenario: 分组逻辑消除 O(n²)
- **WHEN** 计算 `groupedItems`
- **THEN** 用 `tasksByRunId` 索引 O(n) 分组
- **AND** 不存在内层 `find` 退化

### Requirement: 加解密任务参数前后端联动

系统 SHALL 在后端持久化加解密参数（cipherMode/compressionMode/extraFields），前端 EncvTask 接口同步扩展，并在任务卡片/详情中展示。

#### Scenario: 后端持久化加解密参数
- **WHEN** 用户提交加解密任务（createTask API）
- **THEN** 后端从请求参数提取 `cipherMode` / `compressionMode` / `extraFields`
- **AND** 持久化到 task 元数据
- **WHEN** `getTasks` / `getTask` 返回任务
- **THEN** 包含 `cipherMode` / `compressionMode` / `extraFields` 字段

#### Scenario: 前端 EncvTask 接口扩展
- **WHEN** 前端定义 `EncvTask` 接口
- **THEN** 包含可选字段 `cipherMode?: string` / `compressionMode?: string` / `extraFields?: Record<string, string>`

#### Scenario: 任务卡片展示加解密参数
- **WHEN** 渲染任务卡片折叠态
- **THEN** 副标题显示 `cipherMode` / `compressionMode` 摘要（如 `AES-256 | zstd`）
- **WHEN** 渲染任务展开态（TaskBasicInfo）
- **THEN** 包含加解密参数区块，显示完整 `cipherMode` / `compressionMode` / `extraFields`

#### Scenario: 刷新页面后参数回显
- **WHEN** 用户刷新页面或 WS 重连
- **THEN** 任务卡片的加解密参数从后端返回数据回显
- **AND** 不丢失（因后端已持久化）

#### Scenario: useTestCaseGeneration 笛卡尔积扩展（视 plugin 元数据而定）
- **WHEN** plugin 元数据暴露 `cipherMode` / `compressionMode` 候选值
- **THEN** `useTestCaseGeneration` 笛卡尔积展开包含 `cipherMode` × `compressionMode` × `extraFields`
- **WHEN** plugin 元数据未暴露候选值
- **THEN** 保持现状（只派生 extraFields），加解密参数仅用于展示回显

## Constraints

1. **不破坏第一轮已完成工作**：服务统一、类型枚举化、组件抽取的架构保持不变，仅重写样式和修复 bug
2. **向后兼容**：旧 localStorage key（`encv_automation_results_v1` 等）自然衰减，不强制迁移
3. **测试覆盖**：所有修改的 composable / 组件需有单测；状态机终态保护、乱序缓冲、虚拟滚动需有单测
4. **真机验证**：虚拟滚动需在 120Hz 真机验证无白屏
5. **主题一致性**：light/dark 双主题均需验证，design token 覆盖所有时间线/树/卡片组件
