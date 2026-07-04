# 统一工作流任务服务与时间线 UI 重构 Spec

## Why

当前任务系统存在三套并行实现（`useWorkflowEngine` / `useAutomationTests` / `useWebDavAutomationTests`），WS 4 件套订阅逻辑、状态机、持久化、类型定义各自重复；`useTaskEventBridge` 形同虚设被绕过。三个 UI 区域（FFMPEG 流程日志、测试报告树、任务卡片时间线）样式混乱、抽象不足、视觉割裂（测试报告时间线在底部固定，step 详情与树分离；任务时间线仅竖线+圆点无进度/耗时）。

需要重构为统一的 `WorkflowTaskService`（编排+事件+持久化），并抽取通用 `UnifiedTreeView` + `UnifiedTimelineCard` 组件，再应用于三个 UI 区域，同步将前后端 phase 字符串枚举化。

## What Changes

### 服务层
- **新增** `useWorkflowTaskService` composable —— 统一的编排+事件+持久化入口，三套实现改为调用该服务
- **强化** `useTaskEventBridge` 为唯一 WS 4 件套入口（`task:created` / `task:update` / `task:progress` / `task:completed`），删除三套实现中重复的 `startListening` / `stopListening`
- **新增** 持久化 key `encv_workflow_tasks_v1`（不迁移旧数据，旧 key `encv_automation_results_v1` / `encv-workflow-runs` / `encv_webdav_automation_results_v2` 自然衰减）
- **退役** `useWorkflowEngine` / `useAutomationTests` / `useWebDavAutomationTests`，调用方迁移到 `useWorkflowTaskService`

### 类型层
- **新增** `UnifiedTreeNode` 类型 —— 通用树节点抽象（id / label / status / progress / phase / speed / eta / duration / icon / children / detailSlots），FFMPEG 日志卡和测试报告树共用
- **新增** `UnifiedTimelineEntry` 类型 —— 通用时间线条目（phase / icon / time / duration / progress / speed / eta / expandDetail），任务时间线和 FFMPEG 日志条目共用
- **新增** `Phase` 枚举（前后端同步）—— 替代散落的裸字符串 `'analyzing'` / `'encrypting'` / `'packing'` 等
- **扩展** `lib/workflow/types.ts` 增加上述类型；保留 `WorkflowRun` / `JobRun` / `StepRun` 作为运行时类型基础

### UI 组件层
- **重构** `components/automation/TreeView.vue` → 通用 `UnifiedTreeView`，接受 `UnifiedTreeNode[]` props + slot-based 详情渲染
- **新增** `components/shared/UnifiedTimelineCard.vue` —— 通用时间线卡片骨架（phase 图标 + 状态色 + 进度条 + 速率 + ETA + 耗时跨度 + 卡片化展开详情），FFMPEG 日志条目和任务时间线事件共用
- **新增** `components/shared/PhaseIcon.vue` + `components/shared/PhaseBadge.vue` —— phase 图标和徽章（基于 `Phase` 枚举）
- **抽取** `components/developer/MockGenLogCard.vue` —— 从 `PluginTestsDetail.vue` 抽取 FFMPEG 流程日志卡，使用 `UnifiedTimelineCard` 骨架
- **抽取** `composables/useMockGenLog.ts` —— FFMPEG 日志状态 + SSE 回调（5 个事件）独立 composable

### UI 应用层
- **修改** 测试报告树：移除底部固定时间线，step 展开节点内联显示该 step 的时间线（phase 序列 + 耗时）；保留 `StepDetailPanel` 作为"深度诊断"二级展开
- **修改** 任务卡片时间线（`TaskTimeline.vue`）：使用 `UnifiedTimelineCard`，加 phase 图标 + 进度条 + 速率 + ETA + 耗时跨度 + 卡片化展开
- **修改** FFMPEG 日志卡：使用 `UnifiedTimelineCard` 骨架，与任务时间线视觉统一
- **修改** `Tasks.vue` / `TaskBasicInfo.vue`：抽取 `useSectionDerivation` composable 消除 section 派生重复

### 后端
- **修改** `internal/.../task_manager.go`（及关联处）phase 字符串 → `Phase` 常量；前后端枚举值一致

## Impact

### 受影响代码
- `app/encv-mobile/src/composables/useWorkflowEngine.ts`（退役）
- `app/encv-mobile/src/composables/useAutomationTests.ts`（退役）
- `app/encv-mobile/src/composables/useWebDavAutomationTests.ts`（退役）
- `app/encv-mobile/src/composables/useTaskEventBridge.ts`（强化为唯一入口）
- `app/encv-mobile/src/composables/useWorkflowStore.ts`（被新服务取代或保留为底层）
- `app/encv-mobile/src/composables/useTaskTrigger.ts`（保留，已被三套共享）
- `app/encv-mobile/src/components/automation/TreeView.vue`（重构为通用）
- `app/encv-mobile/src/components/automation/StepDetailPanel.vue`（集成内联时间线）
- `app/encv-mobile/src/components/TaskTimeline.vue`（美化 + 用 UnifiedTimelineCard）
- `app/encv-mobile/src/components/TaskDetailModal.vue`（消费新 TaskTimeline）
- `app/encv-mobile/src/views/PluginTestsDetail.vue`（抽取 FFMPEG 日志卡，1535 行瘦身）
- `app/encv-mobile/src/views/Tasks.vue`（用新服务 + useSectionDerivation）
- `app/encv-mobile/src/lib/workflow/types.ts`（扩展 UnifiedTreeNode / UnifiedTimelineEntry / Phase）
- 后端 `internal/.../task_manager.go` 及关联（phase 枚举化）

### 受影响 specs
- `layered-refactor-analysis`（Phase 4.1 phase 枚举化对齐）
- `automation-workflow` 规则（4 件套订阅、动态工作流构建、本地持久化规范）

## ADDED Requirements

### Requirement: WorkflowTaskService 通用工作流任务服务

系统 SHALL 提供统一的 `useWorkflowTaskService` composable，作为工作流任务的编排、事件、持久化唯一入口。

#### Scenario: 三套实现统一调用
- **WHEN** 插件测试 / WebDAV 测试 / 通用工作流任务需要执行
- **THEN** 调用方通过 `useWorkflowTaskService` 提交任务、订阅事件、读取运行记录
- **AND** 不再直接订阅 eventBus 的 `task:*` 4 件套事件

#### Scenario: WS 4 件套统一入口
- **WHEN** 后端推送 `task:created` / `task:update` / `task:progress` / `task:completed`
- **THEN** `useTaskEventBridge` 接收并转发给 `useWorkflowTaskService`
- **AND** `useWorkflowTaskService` 更新对应 StepRun 的 status / progress / phase / speed / eta
- **AND** 终态保护：已终态的 StepRun 不再被覆盖

#### Scenario: 持久化统一
- **WHEN** 任务提交或完成
- **THEN** `useWorkflowTaskService` 写入 localStorage key `encv_workflow_tasks_v1`
- **AND** 最多保留 50 次运行记录（按 `startedAt` 倒序裁剪）
- **AND** 旧 key 数据不迁移、不读取

#### Scenario: 取消运行
- **WHEN** 调用方调用 `cancelRun(runId)`
- **THEN** 服务将运行中 step 标记为 `cancelling` → `cancelled`
- **AND** 调用后端 cancel API

### Requirement: UnifiedTreeNode 通用树节点类型

系统 SHALL 提供 `UnifiedTreeNode` 类型作为通用树节点抽象。

```typescript
interface UnifiedTreeNode {
  id: string
  label: string
  status: StepStatus
  progress?: number          // 0-100
  phase?: Phase
  speed?: string             // "12.5 MB/s"
  eta?: string               // "约 30s"
  duration?: string          // "12.3s"
  icon?: string              // 自定义图标覆盖
  meta?: string              // 副标题（如 "[3/12] · ffmpeg"）
  errorHint?: string
  children?: UnifiedTreeNode[]
  detailSlots?: string[]     // 声明可渲染的 detail slot 名（如 ['ffmpegArgs','stderr','diagnosis']）
}
```

#### Scenario: FFMPEG 日志卡使用 UnifiedTreeNode
- **WHEN** FFMPEG 日志条目渲染
- **THEN** 每个 `MockGenLogEntry` 转换为 `UnifiedTreeNode`，`detailSlots = ['ffmpegArgs', 'stderr', 'context']`
- **AND** `status` 映射自 `entry.status`（'ok'→'success', 'failed'→'failure', 'pending'→'running'）

#### Scenario: 测试报告树使用 UnifiedTreeNode
- **WHEN** WorkflowRun 渲染为树
- **THEN** Job 转换为父节点，Step 转换为子节点
- **AND** `detailSlots = ['timeline', 'diagnosis', 'encryptionParams']`（timeline 为内联时间线）

### Requirement: UnifiedTimelineCard 通用时间线卡片

系统 SHALL 提供 `UnifiedTimelineCard` 组件作为通用时间线卡片骨架。

#### Scenario: 任务时间线使用
- **WHEN** 任务卡片详情显示时间线
- **THEN** `TaskTimeline.vue` 渲染 `UnifiedTimelineCard` 列表
- **AND** 每个事件显示 phase 图标 + 状态色 + 时间 + 进度条 + 速率 + ETA + 耗时跨度
- **AND** 展开详情以卡片化布局呈现（不再是 label:value 网格）

#### Scenario: FFMPEG 日志条目使用
- **WHEN** FFMPEG 日志卡渲染条目
- **THEN** 每个 `MockGenLogEntry` 渲染为 `UnifiedTimelineCard`
- **AND** 显示 runner 图标（⚡mediacodec / ⚙ffmpeg / 📄static）+ 路径 + encoder + exit code
- **AND** 展开详情卡片化显示 ffmpeg args / stderr / context

#### Scenario: 视觉统一
- **WHEN** 同一应用内出现 FFMPEG 日志卡和任务时间线
- **THEN** 两者使用相同的卡片骨架（圆角、内边距、状态色边框、展开动画）
- **AND** 通过 slot 区分内容差异

### Requirement: Phase 枚举化（前后端同步）

系统 SHALL 将 phase 字符串改为 `Phase` 枚举常量，前后端值一致。

```typescript
enum Phase {
  Created = 'created',
  Analyzing = 'analyzing',
  Initializing = 'initializing',
  Preprocessing = 'preprocessing',
  Encrypting = 'encrypting',
  Decrypting = 'decrypting',
  Packing = 'packing',
  Verifying = 'verifying',
  Completed = 'completed',
}
```

#### Scenario: 前端使用 Phase 枚举
- **WHEN** 前端代码引用 phase 值
- **THEN** 使用 `Phase.Encrypting` 等枚举常量，不使用裸字符串 `'encrypting'`

#### Scenario: 后端使用 Phase 常量
- **WHEN** 后端 `task_manager.go` 推送 phase
- **THEN** 使用 Go 端 `Phase` 常量（字符串值与前端枚举一致）

#### Scenario: PhaseIcon / PhaseBadge 映射
- **WHEN** UI 渲染 phase
- **THEN** `PhaseIcon` 组件根据 `Phase` 枚举返回对应 ion-icon（如 `Phase.Encrypting` → `lockClosedOutline`）
- **AND** `PhaseBadge` 组件根据 `Phase` 枚举返回对应颜色

### Requirement: 测试报告时间线移入展开节点

系统 SHALL 将测试报告的时间线从底部固定位置移入 step 展开节点内联显示。

#### Scenario: step 展开显示内联时间线
- **WHEN** 用户在测试报告树中展开某个 step 节点
- **THEN** 节点下方内联显示该 step 的时间线（phase 序列 + 耗时 + 进度）
- **AND** 不再显示底部固定的全局时间线

#### Scenario: 保留深度诊断二级展开
- **WHEN** 用户点击内联时间线下方的"深度诊断"按钮
- **THEN** 展开 `StepDetailPanel` 的 5 个诊断区块（DIAGNOSIS / ERROR CHAIN / REMEDIATION / METADATA / RAW ERROR）
- **AND** 默认折叠，避免视觉过载

### Requirement: useSectionDerivation 派生逻辑抽取

系统 SHALL 抽取 `useSectionDerivation` composable 消除 `Tasks.vue` 与 `TaskBasicInfo.vue` 中重复的 section 派生逻辑。

#### Scenario: 统一派生
- **WHEN** `Tasks.vue` 或 `TaskBasicInfo.vue` 需要派生 section（plugin / type / category / none）
- **THEN** 调用 `useSectionDerivation` 的 `deriveSubSection(task)` 函数
- **AND** 两处不再各自实现 `deriveSubSection`

## MODIFIED Requirements

### Requirement: 任务卡片时间线美化

`TaskTimeline.vue` SHALL 使用 `UnifiedTimelineCard` 渲染时间线，并显示 phase 图标、进度条、速率、ETA、耗时跨度、卡片化展开详情。

#### Scenario: phase 图标 + 状态色
- **WHEN** 渲染时间线事件
- **THEN** 每个 phase 显示对应图标（analyzing=🔍, encrypting=🔐, packing=📦 等）
- **AND** 状态色：current=蓝, completed=绿, error=红

#### Scenario: 进度条 + 速率 + ETA
- **WHEN** StepRun 有 `progress` / `speed` / `eta` 字段
- **THEN** 时间线事件显示进度条（0-100%）+ 速率文本 + ETA 文本
- **AND** 字段缺失时不显示对应元素（不报错）

#### Scenario: 耗时跨度可视化
- **WHEN** 时间线事件有 `duration`
- **THEN** 事件间显示耗时（如 "12.3s"）
- **AND** 最长耗时 phase 高亮（如加粗或不同背景色）

#### Scenario: 展开详情卡片化
- **WHEN** 用户点击可展开的时间线事件
- **THEN** 展开内容以卡片化布局呈现（不再是 label:value 网格）
- **AND** 关键信息（输出路径、错误、加密参数）以独立卡片区块显示

### Requirement: FFMPEG 流程日志卡抽取

`PluginTestsDetail.vue` SHALL 将 FFMPEG 流程日志卡抽取为独立的 `MockGenLogCard.vue` 组件 + `useMockGenLog` composable。

#### Scenario: 组件抽取
- **WHEN** `PluginTestsDetail.vue` 渲染 FFMPEG 日志
- **THEN** 使用 `<MockGenLogCard :log="mockGenLog" :summary="mockGenLogSummary" />`
- **AND** 主文件不再内联 106-187 行的日志卡模板

#### Scenario: composable 抽取
- **WHEN** `PluginTestsDetail.vue` 需要 FFMPEG 日志状态
- **THEN** 调用 `useMockGenLog()` 获取 `mockGenLog` / `mockGenLogTotal` / `mockGenLogSummary` / `toggleMockGenLogEntry` / `copyMockGenLog`
- **AND** 5 个 SSE 回调（onSpecDiag / onSpecPlan / onProgress / onSpecFailed / onSkipped）封装在 composable 内

#### Scenario: 使用 UnifiedTimelineCard 骨架
- **WHEN** `MockGenLogCard` 渲染日志条目
- **THEN** 每个 `MockGenLogEntry` 转换为 `UnifiedTreeNode` 并通过 `UnifiedTimelineCard` 渲染
- **AND** 视觉与任务时间线统一

## REMOVED Requirements

### Requirement: useWorkflowEngine
**Reason**: 被 `useWorkflowTaskService` 取代，三套并行实现造成 WS 订阅、状态机、持久化重复
**Migration**: 调用方（`PluginTestsDetail.vue` 等）改用 `useWorkflowTaskService`；`WorkflowRun` / `JobRun` / `StepRun` 类型保留在 `lib/workflow/types.ts` 作为运行时类型基础

### Requirement: useAutomationTests
**Reason**: 被 `useWorkflowTaskService` 取代，与 `useWorkflowEngine` 逻辑重复且硬编码 cipherMode
**Migration**: 调用方改用 `useWorkflowTaskService`；测试用例生成逻辑（笛卡尔积）抽取为 `useTestCaseGeneration` composable，按 `plugin.taskOptions.extraFields` 派生（消除硬编码，对齐 automation-workflow 规则 §三）

### Requirement: useWebDavAutomationTests
**Reason**: 被 `useWorkflowTaskService` 取代，第三套独立实现加剧重复
**Migration**: 调用方改用 `useWorkflowTaskService`；WebDAV 8 module 协调逻辑作为 `useWorkflowTaskService` 的一个 `workflowDefinition` 模板注册

### Requirement: 测试报告底部固定时间线
**Reason**: step 多时底部时间线不方便查看，与展开节点分离造成视觉割裂
**Migration**: 时间线移入 step 展开节点内联显示；保留底部进度概览（非时间线）
