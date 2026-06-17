# Checklist

## Phase 1: 类型层
- [x] `lib/workflow/types.ts` 包含 `Phase` 枚举（9 个值）
- [x] `lib/workflow/types.ts` 包含 `UnifiedTreeNode` 接口（含 id / label / status / progress / phase / speed / eta / duration / icon / meta / errorHint / children / detailSlots）
- [x] `lib/workflow/types.ts` 包含 `UnifiedTimelineEntry` 接口
- [x] `lib/workflow/types.ts` 包含 `UnifiedRunRecord` 持久化接口
- [x] 类型守卫函数 `isUnifiedTreeNode` / `isUnifiedTimelineEntry` 单测通过
- [x] 后端 Go 端 `Phase` 常量定义且字符串值与前端枚举一致
- [x] 后端所有 phase 裸字符串引用替换为常量
- [x] 后端单测验证 phase 常量值与前端枚举一致

## Phase 2: 服务层
- [x] `useTaskEventBridge.ts` 4 件套全订阅（task:created / task:update / task:progress / task:completed）
- [x] `useTaskEventBridge` onMounted 自动订阅 / onUnmounted 自动取消
- [x] `applyTerminalGuard` 工具函数实现 + 单测覆盖（终态不被覆盖）
- [x] `validateTransition` 状态机校验实现 + 单测覆盖
- [x] `useWorkflowTaskService.ts` 创建，接口含 submitRun / cancelRun / getRun / listRuns / clearRuns / subscribeRun
- [x] 编排逻辑（DAG 分层 + Promise.all + worker 池）从 useWorkflowEngine 迁移
- [x] 事件路由：4 件套事件 → StepRun 更新
- [x] 持久化 key 为 `encv_workflow_tasks_v1`，按 startedAt 倒序保留 50 条
- [x] 持久化三处触发（提交阶段 + 运行阶段 + 末尾）
- [x] 取消逻辑实现（cancelling → cancelled + 后端 cancel API）
- [x] useWorkflowTaskService 单测全绿
- [x] `useTestCaseGeneration.ts` 创建，按 `plugin.taskOptions.extraFields` 派生笛卡尔积（无硬编码 cipherMode）
- [x] `useTestCaseGeneration` 按 `plugin.supportedExtensions[0]` 选源
- [x] `useTestCaseGeneration` 单测覆盖笛卡尔积 + ext → category 映射
- [x] `useSectionDerivation.ts` 创建，导出 `deriveSubSection(task, dimension)`
- [x] `Tasks.vue` 与 `TaskBasicInfo.vue` 改用 `useSectionDerivation`，各自实现删除
- [x] `useSectionDerivation` 单测覆盖 4 种 dimension

## Phase 3: 迁移
- [x] `useWorkflowEngine` 所有调用方迁移到 `useWorkflowTaskService`
- [x] `useWorkflowEngine.ts` 文件删除（无引用）
- [x] `useAutomationTests` 所有调用方迁移到 `useWorkflowTaskService` + `useTestCaseGeneration`
- [x] `useAutomationTests.ts` 文件删除
- [x] `useWebDavAutomationTests` 所有调用方迁移，WebDAV 8 module 协调作为 workflowDefinition 模板注册
- [x] `useWebDavAutomationTests.ts` 文件删除
- [x] 插件测试页流程手动验证正常
- [x] 自动化测试流程手动验证正常
- [x] WebDAV 测试流程手动验证正常

## Phase 4: UI 组件层
- [x] `components/shared/PhaseIcon.vue` 创建，9 个 Phase 值图标映射正确
- [x] `components/shared/PhaseBadge.vue` 创建，9 个 Phase 值颜色映射正确
- [x] PhaseIcon / PhaseBadge 暗黑模式适配
- [x] PhaseIcon / PhaseBadge 单测全绿
- [x] `components/shared/UnifiedTimelineCard.vue` 创建，props: entry + slot: detail
- [x] UnifiedTimelineCard 渲染 phase 图标 + 状态色边框 + 时间 + meta
- [x] UnifiedTimelineCard 渲染进度条 + 速率 + ETA（字段缺失时隐藏）
- [x] UnifiedTimelineCard 渲染耗时跨度
- [x] UnifiedTimelineCard 展开/收起动画 + 卡片化布局
- [x] UnifiedTimelineCard 暗黑模式适配
- [x] UnifiedTimelineCard 单测全绿
- [x] `components/automation/TreeView.vue` 重构为通用 UnifiedTreeView，props: `nodes: UnifiedTreeNode[]`
- [x] UnifiedTreeView slot-based 详情渲染（detailSlots 决定渲染哪些 slot）
- [x] UnifiedTreeView 节点显示 progress / phase / speed / eta / duration
- [x] UnifiedTreeView 保留搜索过滤 + 默认展开失败 job
- [x] UnifiedTreeView 暗黑模式适配（保留米色"档案"主题 light + 新增暗黑变体）
- [x] UnifiedTreeView 单测全绿

## Phase 5: UI 应用层
- [x] `composables/useMockGenLog.ts` 创建，封装 5 个 SSE 回调 + 状态 + 交互函数
- [x] `components/developer/MockGenLogCard.vue` 创建，使用 UnifiedTimelineCard 渲染
- [x] `MockGenLogEntry` → `UnifiedTimelineEntry` 转换器实现（runner 图标 + path + encoder + exit code）
- [x] `PluginTestsDetail.vue` 改为 `<MockGenLogCard />`，删除 106-187 行内联模板 + 1360-1476 行样式
- [x] FFMPEG 日志卡视觉与任务时间线统一
- [x] 测试报告树 step 展开节点内联显示时间线（用 UnifiedTimelineCard 列表）
- [x] 时间线条目从 StepRun 派生（phase 序列 + 耗时 + 进度）
- [x] 底部固定时间线移除
- [x] 底部进度概览保留（总进度条 + 通过率）
- [x] `StepDetailPanel` 改为"深度诊断"二级展开（默认折叠）
- [x] step 多时不再需要滚到底部看时间线
- [x] `TaskTimeline.vue` 改为渲染 UnifiedTimelineCard 列表
- [x] `timelineEvents` 输出 `UnifiedTimelineEntry[]`，phase 用 `Phase` 枚举
- [x] 从 StepRun 派生 progress / speed / eta / duration
- [x] 最长耗时 phase 高亮
- [x] 展开详情卡片化（输出路径 / 错误 / 加密参数作为独立卡片区块）
- [x] `getPhaseLabel` 改为基于 Phase 枚举的映射表
- [x] 任务卡片详情时间线视觉与 FFMPEG 日志卡统一
- [x] `Tasks.vue` 使用 `useWorkflowTaskService` 替换直接订阅
- [x] `Tasks.vue` 使用 `useSectionDerivation` 替换 571-621 行
- [x] `TaskBasicInfo.vue` 使用 `useSectionDerivation` 替换 153-188 行
- [x] 任务列表分组 + 详情显示正常

## Phase 6: 验证
- [x] 插件测试页 FFMPEG 日志卡渲染正常 + 与任务时间线视觉统一
- [x] 测试报告树展开 step 显示内联时间线 + 深度诊断二级展开
- [x] 任务卡片详情时间线显示 phase 图标 + 进度 + 速率 + ETA + 耗时 + 卡片化展开
- [x] 三套自动化测试流程均通过 useWorkflowTaskService 正常执行
- [x] 持久化 —— 刷新页面 / 关 App 后运行记录从 `encv_workflow_tasks_v1` 恢复
- [x] 暗黑模式 —— 三个 UI 区域暗黑模式样式正常
- [x] 前后端 phase 枚举值一致 —— 后端推送的 phase 在前端正确渲染图标/颜色
- [x] `pnpm lint` 通过（N/A：项目未配置 eslint，无 lint script）
- [x] `pnpm typecheck`（或 `vue-tsc`）通过（0 errors）
- [x] `pnpm test`（vitest）通过，新增单测全绿（1404 passed / 4 pre-existing failed 与 spec 无关）
- [x] 后端 `go test` 通过
- [x] 旧 key 数据不读取验证（删除 localStorage 旧 key 后功能正常）

## 铁律合规
- [x] WS 4 件套全订阅（task:created / task:update / task:progress / task:completed）—— 对齐 automation-workflow 规则 §二
- [x] 动态工作流构建无硬编码 cipherMode / compressionMode / sourcePath —— 对齐 automation-workflow 规则 §三
- [x] 本地持久化规范 —— key 带版本号 + 按 startedAt 倒序裁剪 50 条 + 提交阶段双保险 —— 对齐 automation-workflow 规则 §五
- [x] 任务组按 workflow run.id 分组 —— 对齐 automation-workflow 规则 §六
- [x] Phase 枚举化对齐 layered-refactor-analysis spec Phase 4.1
- [x] 不创建不必要文件 —— 通用组件复用，旧实现删除
- [x] 不写文档文件（除非用户明确要求）
