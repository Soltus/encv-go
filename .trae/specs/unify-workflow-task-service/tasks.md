# Tasks

按"类型 → 服务 → 迁移 → UI"顺序推进。每个 Task 完成后必须打勾。

## Phase 1: 类型层（基础）

- [x] Task 1: 扩展 `lib/workflow/types.ts` 增加 `Phase` 枚举 + `UnifiedTreeNode` + `UnifiedTimelineEntry` 类型
  - [x] SubTask 1.1: 定义 `Phase` 枚举（9 个值：created / analyzing / initializing / preprocessing / encrypting / decrypting / packing / verifying / completed）
  - [x] SubTask 1.2: 定义 `UnifiedTreeNode` 接口（id / label / status / progress / phase / speed / eta / duration / icon / meta / errorHint / children / detailSlots）
  - [x] SubTask 1.3: 定义 `UnifiedTimelineEntry` 接口（phase / icon / time / duration / progress / speed / eta / expandDetail）
  - [x] SubTask 1.4: 定义 `UnifiedRunRecord` 持久化接口（id / startedAt / completedAt / totalCases / passed / failed / skipped / results / workflowRun）
  - [x] SubTask 1.5: 单元测试覆盖类型守卫函数 `isUnifiedTreeNode` / `isUnifiedTimelineEntry`

- [x] Task 2: 后端 phase 字符串枚举化
  - [x] SubTask 2.1: 在 `internal/.../task_manager.go`（或合适位置）定义 Go 端 `Phase` 常量（字符串值与前端枚举一致）
  - [x] SubTask 2.2: 替换所有 phase 裸字符串引用为 `PhaseXXX` 常量
  - [x] SubTask 2.3: 后端单测覆盖 phase 常量值与前端枚举一致（生成共享 JSON 契约文件或断言）

## Phase 2: 服务层（核心）

- [x] Task 3: 强化 `useTaskEventBridge` 为唯一 WS 4 件套入口
  - [x] SubTask 3.1: 在 `useTaskEventBridge.ts` 中确保 4 件套全订阅（`task:created` / `task:update` / `task:progress` / `task:completed`），onMounted 自动订阅 / onUnmounted 自动取消
  - [x] SubTask 3.2: 增加终态保护工具函数 `applyTerminalGuard(stepRun, update)` —— 已终态的 StepRun 不被覆盖
  - [x] SubTask 3.3: 增加状态机校验 `validateTransition(from, to)` —— 基于 `VALID_TRANSITIONS`
  - [x] SubTask 3.4: 单测覆盖终态保护 + 状态转换校验

- [x] Task 4: 新建 `useWorkflowTaskService` composable
  - [x] SubTask 4.1: 创建 `composables/useWorkflowTaskService.ts`，定义服务接口（submitRun / cancelRun / getRun / listRuns / clearRuns / subscribeRun）
  - [x] SubTask 4.2: 实现编排逻辑（DAG 分层 + Promise.all + worker 池并发），从 `useWorkflowEngine` 迁移 `runWorkflow` / `executeJob` / `checkJobCompletion` / `scheduleDependentJobs` / `submitAction` 核心逻辑
  - [x] SubTask 4.3: 实现事件订阅 —— 内部调用 `useTaskEventBridge`，将 4 件套事件路由到对应 StepRun 更新
  - [x] SubTask 4.4: 实现持久化 —— localStorage key `encv_workflow_tasks_v1`，按 `startedAt` 倒序保留 50 条；提交阶段 + 运行阶段 + 末尾三处持久化（双保险）
  - [x] SubTask 4.5: 实现取消逻辑 —— `cancelRun(runId)` 标记 running step 为 cancelling → cancelled + 调用后端 cancel API
  - [x] SubTask 4.6: 单测覆盖编排 / 事件路由 / 持久化裁剪 / 取消

- [x] Task 5: 抽取 `useTestCaseGeneration` composable（消除硬编码）
  - [x] SubTask 5.1: 创建 `composables/useTestCaseGeneration.ts`，从 `useAutomationTests` 迁移 `generateTestCases` 逻辑
  - [x] SubTask 5.2: 改为按 `plugin.taskOptions.extraFields` 派生笛卡尔积（消除 cipherMode / compressionMode 硬编码，对齐 automation-workflow 规则 §三）
  - [x] SubTask 5.3: 按 `plugin.supportedExtensions[0]` 选源（避免笛卡尔积爆炸）
  - [x] SubTask 5.4: 单测覆盖笛卡尔积展开 + ext → category 映射

- [x] Task 6: 抽取 `useSectionDerivation` composable
  - [x] SubTask 6.1: 创建 `composables/useSectionDerivation.ts`，合并 `Tasks.vue` (571-621) 与 `TaskBasicInfo.vue` (153-188) 的 `deriveSubSection` 逻辑
  - [x] SubTask 6.2: 导出 `deriveSubSection(task, dimension)` 函数（dimension: 'plugin' | 'type' | 'category' | 'none'）
  - [x] SubTask 6.3: `Tasks.vue` 与 `TaskBasicInfo.vue` 改为调用该 composable，删除各自实现
  - [x] SubTask 6.4: 单测覆盖 4 种 dimension 派生

## Phase 3: 迁移（退役旧实现）

- [x] Task 7: 迁移 `useWorkflowEngine` 调用方到 `useWorkflowTaskService`
  - [x] SubTask 7.1: 查找所有 `useWorkflowEngine` 调用方（`PluginTestsDetail.vue` 等）
  - [x] SubTask 7.2: 替换为 `useWorkflowTaskService` 调用，保持 UI 行为不变
  - [x] SubTask 7.3: 删除 `useWorkflowEngine.ts`（确认无引用后）
  - [x] SubTask 7.4: 手动验证插件测试页流程正常

- [x] Task 8: 迁移 `useAutomationTests` 调用方
  - [x] SubTask 8.1: 查找所有 `useAutomationTests` 调用方
  - [x] SubTask 8.2: 替换为 `useWorkflowTaskService` + `useTestCaseGeneration`
  - [x] SubTask 8.3: 删除 `useAutomationTests.ts`
  - [x] SubTask 8.4: 手动验证自动化测试流程

- [x] Task 9: 迁移 `useWebDavAutomationTests` 调用方
  - [x] SubTask 9.1: 查找所有 `useWebDavAutomationTests` 调用方
  - [x] SubTask 9.2: WebDAV 8 module 协调逻辑作为 `workflowDefinition` 模板注册到 `useWorkflowTaskService`
  - [x] SubTask 9.3: 替换调用方
  - [x] SubTask 9.4: 删除 `useWebDavAutomationTests.ts`
  - [x] SubTask 9.5: 手动验证 WebDAV 测试流程

## Phase 4: UI 组件层（通用组件）

- [x] Task 10: 新建 `PhaseIcon.vue` + `PhaseBadge.vue`
  - [x] SubTask 10.1: 创建 `components/shared/PhaseIcon.vue` —— props: `phase: Phase`，根据枚举返回 ion-icon（analyzing=searchOutline, encrypting=lockClosedOutline, packing=cubeOutline 等）
  - [x] SubTask 10.2: 创建 `components/shared/PhaseBadge.vue` —— props: `phase: Phase`，返回带颜色背景的徽章
  - [x] SubTask 10.3: 暗黑模式适配
  - [x] SubTask 10.4: 单测覆盖 9 个 Phase 值的图标/颜色映射

- [x] Task 11: 新建 `UnifiedTimelineCard.vue`
  - [x] SubTask 11.1: 创建 `components/shared/UnifiedTimelineCard.vue` —— props: `entry: UnifiedTimelineEntry`，slot: `detail`（卡片化展开内容）
  - [x] SubTask 11.2: 渲染 phase 图标（用 PhaseIcon）+ 状态色边框 + 时间 + meta
  - [x] SubTask 11.3: 渲染进度条（0-100%）+ 速率 + ETA（字段缺失时隐藏）
  - [x] SubTask 11.4: 渲染耗时跨度（duration 文本，最长耗时高亮标记由父组件控制）
  - [x] SubTask 11.5: 展开/收起动画 + 卡片化布局（圆角、内边距、状态色边框）
  - [x] SubTask 11.6: 暗黑模式适配
  - [x] SubTask 11.7: 单测覆盖渲染 + 字段缺失 + 展开交互

- [x] Task 12: 重构 `TreeView.vue` → 通用 `UnifiedTreeView`
  - [x] SubTask 12.1: 修改 `components/automation/TreeView.vue` props 为 `nodes: UnifiedTreeNode[]` + `stepNames?` / `jobDisplayNames?`（保留兼容）
  - [x] SubTask 12.2: 实现 slot-based 详情渲染（`detailSlots` 决定渲染哪些 slot）
  - [x] SubTask 12.3: 节点显示 progress / phase / speed / eta / duration（字段缺失时隐藏）
  - [x] SubTask 12.4: 保留搜索过滤 + 默认展开失败 job 的行为
  - [x] SubTask 12.5: 暗黑模式适配（保留米色"档案"主题作为 light，新增暗黑变体）
  - [x] SubTask 12.6: 单测覆盖渲染 + 搜索 + 展开

## Phase 5: UI 应用层（三个区域）

- [x] Task 13: 抽取 `MockGenLogCard.vue` + `useMockGenLog` composable
  - [x] SubTask 13.1: 创建 `composables/useMockGenLog.ts` —— 迁移 `PluginTestsDetail.vue` 417-455 状态 + 608-707 SSE 回调 + 751-795 交互函数
  - [x] SubTask 13.2: 创建 `components/developer/MockGenLogCard.vue` —— 使用 `UnifiedTimelineCard` 渲染每个 `MockGenLogEntry`（转换为 `UnifiedTimelineEntry`）
  - [x] SubTask 13.3: `MockGenLogEntry` → `UnifiedTimelineEntry` 转换器：runner 图标（⚡mediacodec / ⚙ffmpeg / 📄static）+ path + encoder + exit code 作为 meta；ffmpegArgs / stderr / context 作为 detail slot
  - [x] SubTask 13.4: `PluginTestsDetail.vue` 改为 `<MockGenLogCard :log="mockGenLog" :summary="mockGenLogSummary" />`，删除 106-187 行内联模板 + 1360-1476 行样式
  - [x] SubTask 13.5: 验证 FFMPEG 日志卡视觉与任务时间线统一

- [x] Task 14: 测试报告树时间线移入展开节点
  - [x] SubTask 14.1: 修改 `UnifiedTreeView`（原 TreeView）—— step 节点展开时内联渲染该 step 的时间线（用 `UnifiedTimelineCard` 列表）
  - [x] SubTask 14.2: 时间线条目从 `StepRun` 派生（phase 序列 + 耗时 + 进度）
  - [x] SubTask 14.3: 移除底部固定时间线（`PluginTestsDetail.vue` 测试报告区底部时间线组件）
  - [x] SubTask 14.4: 保留底部进度概览（非时间线，如总进度条 + 通过率）
  - [x] SubTask 14.5: `StepDetailPanel` 改为"深度诊断"二级展开（默认折叠，点击内联时间线下方按钮展开 5 个诊断区块）
  - [x] SubTask 14.6: 验证 step 多时不再需要滚到底部看时间线

- [x] Task 15: 任务卡片时间线美化
  - [x] SubTask 15.1: 修改 `TaskTimeline.vue` —— 改为渲染 `UnifiedTimelineCard` 列表
  - [x] SubTask 15.2: `timelineEvents` computed 改为输出 `UnifiedTimelineEntry[]`，phase 字段使用 `Phase` 枚举
  - [x] SubTask 15.3: 从 `StepRun` 派生 progress / speed / eta / duration（之前未展示的字段）
  - [x] SubTask 15.4: 计算最长耗时 phase 并高亮（加粗或不同背景色）
  - [x] SubTask 15.5: 展开详情卡片化 —— 输出路径 / 错误 / 加密参数作为独立卡片区块（不再是 label:value 网格）
  - [x] SubTask 15.6: `getPhaseLabel` 改为基于 `Phase` 枚举的映射表（i18n key）
  - [x] SubTask 15.7: 验证任务卡片详情时间线视觉与 FFMPEG 日志卡统一

- [x] Task 16: `Tasks.vue` 应用新服务 + composable
  - [x] SubTask 16.1: `Tasks.vue` 改为使用 `useWorkflowTaskService`（替换原 `useTaskEventBridge` 直接订阅）
  - [x] SubTask 16.2: 使用 `useSectionDerivation` 替换 571-621 行 `deriveSubSection`
  - [x] SubTask 16.3: `TaskBasicInfo.vue` 同步使用 `useSectionDerivation` 替换 153-188 行
  - [x] SubTask 16.4: 验证任务列表分组 + 详情显示正常

## Phase 6: 验证

- [x] Task 17: 端到端验证
  - [x] SubTask 17.1: 插件测试页 FFMPEG 日志卡渲染正常 + 与任务时间线视觉统一
  - [x] SubTask 17.2: 测试报告树展开 step 显示内联时间线 + 深度诊断二级展开
  - [x] SubTask 17.3: 任务卡片详情时间线显示 phase 图标 + 进度 + 速率 + ETA + 耗时 + 卡片化展开
  - [x] SubTask 17.4: 三套自动化测试流程（插件测试 / WebDAV 测试 / 通用工作流）均通过 `useWorkflowTaskService` 正常执行
  - [x] SubTask 17.5: 持久化 —— 刷新页面 / 关 App 后运行记录从 `encv_workflow_tasks_v1` 恢复
  - [x] SubTask 17.6: 暗黑模式 —— 三个 UI 区域暗黑模式样式正常
  - [x] SubTask 17.7: 前后端 phase 枚举值一致 —— 后端推送的 phase 在前端正确渲染图标/颜色

- [x] Task 18: Lint + 类型检查 + 测试
  - [x] SubTask 18.1: `pnpm lint` 通过
  - [x] SubTask 18.2: `pnpm typecheck`（或 `vue-tsc`）通过
  - [x] SubTask 18.3: `pnpm test`（vitest）通过，新增单测全绿
  - [x] SubTask 18.4: 后端 `go test` 通过（phase 枚举化相关）
  - [x] SubTask 18.5: 旧 key 数据不读取验证（删除 localStorage 旧 key 后功能正常）

# Task Dependencies

- Task 1（类型）→ 所有后续 Task 依赖
- Task 2（后端 phase 枚举）独立可并行，但 Task 17 验证依赖
- Task 3（useTaskEventBridge 强化）→ Task 4（useWorkflowTaskService）依赖
- Task 4 → Task 7/8/9（迁移）依赖
- Task 5（useTestCaseGeneration）→ Task 8 依赖
- Task 6（useSectionDerivation）独立可并行
- Task 10（PhaseIcon/Badge）→ Task 11（UnifiedTimelineCard）依赖
- Task 11 → Task 12（UnifiedTreeView 用 UnifiedTimelineCard 渲染时间线）依赖
- Task 11 → Task 13/15 依赖
- Task 12 → Task 14 依赖
- Task 7/8/9（迁移完成）→ Task 16（Tasks.vue 应用新服务）依赖
- 所有 Task → Task 17/18 验证依赖

可并行分组：
- Phase 1: Task 1 + Task 2 并行
- Phase 2: Task 3 + Task 5 + Task 6 并行（Task 4 依赖 Task 3）
- Phase 4: Task 10 独立；Task 11 依赖 Task 10；Task 12 依赖 Task 11
- Phase 5: Task 13/14/15 部分可并行（Task 13 依赖 Task 11；Task 14 依赖 Task 12；Task 15 依赖 Task 11）
