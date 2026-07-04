# Checklist — UI 丑化修复 + Tasks 性能优化 + 加解密任务适配 v2

> 本 checklist 是 `unify-workflow-task-service` 第二轮修复的验证清单。每个 checkpoint 需在对应 Task 完成后勾选。

## 一、UI 丑化修复

### Design Token 基础
- [ ] `app/encv-mobile/src/styles/timeline-tokens.css` 已创建
- [ ] `:root` 定义 light 模式变量（`--card-bg-gradient-start: #FAFBFC` 等）
- [ ] `body.dark` 定义 dark 模式变量（`--card-bg-gradient-start: #0F1419` 等）
- [ ] `main.ts` 已引入 timeline-tokens.css
- [ ] 浏览器 DevTools 可见 CSS 变量

### 图标体系统一
- [ ] `PhaseIcon.vue` 全用 ion-icon，无 emoji/Unicode
- [ ] 11 个 phase 映射正确（created/analyzing/initializing/preprocessing/encrypting/decrypting/packing/verifying/completed/failed/cancelled）
- [ ] `StepInlineTimeline.vue` 无 StepMiniBadge 自定义 SVG
- [ ] `TaskTimeline.vue` 无 emoji（⚡⏳⚙📄）
- [ ] `TreeView.vue` 无 Unicode 展开符号（▶▼），用 ion-icon chevron-forward/chevron-down
- [ ] 全局搜索无 emoji 在时间线/树/卡片组件中

### PhaseBadge 暗黑模式
- [ ] dark 模式下 encrypting 仍为紫色（rgba(139, 92, 246, 0.15) 背景 + #8B5CF6 文字）
- [ ] dark 模式下 completed 仍为绿色
- [ ] dark 模式下 failed 仍为红色
- [ ] 无"抹平为灰色"的覆盖样式

### UnifiedTimelineCard 样式
- [ ] 顶部 2px 渐变状态色条（非左侧 4px 边框）
- [ ] 背景为 design token 渐变（light: #FAFBFC→#F4F6F8，dark: #0F1419→#0A0E12）
- [ ] 边框为 1px solid var(--card-border)
- [ ] 展开详情无嵌套卡片（左侧 2px 边线 + padding）
- [ ] 进度条样式与 TestReportHeader 统一（4px 高，2px 圆角）
- [ ] dark 模式背景不透明度不过低（不用 0.04）

### TreeView 暗黑模式
- [ ] dark 模式背景为 #1A1D21（非纯黑）
- [ ] dark 模式文字为 #E0E0E0
- [ ] 展开图标为 ion-icon chevron-forward / chevron-down

### MockGenLogCard 主题适配
- [ ] light 模式背景为浅色渐变，文字深色
- [ ] dark 模式背景为深色终端渐变，文字浅色
- [ ] runner 标识状态色保留（紫/绿/灰）
- [ ] 字体保留 var(--card-font-mono)

### TaskTimeline 统一
- [ ] 用 UnifiedTimelineCard 骨架
- [ ] 进度条+速率+ETA 样式与 FFMPEG 日志卡统一
- [ ] 无 emoji

## 二、Tasks.vue 状态机修复

### 终态保护
- [ ] `applyTaskUpdate` 加 `isTerminalStatus` 检查
- [ ] `applyTaskCompleted` 加 `isTerminalStatus` 检查
- [ ] `applyTaskProgress` 加 `isTerminalStatus` 检查
- [ ] 终态任务收到 update 事件 → 状态不变（单测验证）
- [ ] 终态任务收到 completed 事件 → 状态不变（单测验证）

### 乱序缓冲
- [ ] `pendingEvents` Map 已实现
- [ ] `applyTaskUpdate` idx=-1 时缓存到 pendingEvents
- [ ] `applyTaskProgress` idx=-1 时缓存到 pendingEvents
- [ ] `applyTaskCompleted` idx=-1 时缓存到 pendingEvents
- [ ] `applyTaskCreated` 创建后回放 pendingEvents
- [ ] 回放后清除该 task id 的缓存
- [ ] 单测覆盖：task:update 先于 task:created → created 后状态正确

### fetchTasks 保留实时状态
- [ ] `fetchTasks` 合并本地实时状态与远端状态
- [ ] 本地状态优先（progress/phase/speed/eta 不丢失）
- [ ] 单测覆盖：任务运行中 fetchTasks → 实时状态保留

## 三、Tasks.vue 性能优化

### 虚拟滚动
- [ ] `TaskVirtualList.vue` 组件已创建
- [ ] 使用 `@tanstack/vue-virtual` 的 `useVirtualizer`
- [ ] `estimateSize: () => 80`（折叠态默认高度）
- [ ] `overscan: 20`（加大防止白屏）
- [ ] `measureElement` 自动测量展开态高度
- [ ] `content-visibility: auto` + `contain-intrinsic-size: 80px` 白屏优化
- [ ] 暴露 `forceMeasure()` 给父级兜底
- [ ] 复用 DevLogs 的 `ensureScrollEl()` + ResizeObserver 兜底逻辑
- [ ] 200-500 task 渲染时 DOM 节点恒定（视口内 + overscan 20）
- [ ] 任务卡片展开/折叠时高度自动 re-measure

### 删除调试代码
- [ ] `grouping-debug-bar` 已删除
- [ ] 3 个 debug computed 已删除
- [ ] `showAutomationReports` / `reportAutomationToBackend` 已删除（已迁移到 useWorkflowTaskService）
- [ ] 生产代码无调试栏

### shallowRef + 预构建索引
- [ ] `tasks` 从 `ref` 改为 `shallowRef`
- [ ] `tasksByRunId` computed 已实现（Map<runId, EncvTask[]>）
- [ ] `triggeredByCache` computed 已实现（Map<taskId, { by, color, icon }>）
- [ ] Vue DevTools 确认 tasks 为非深层响应式
- [ ] triggeredByCache 缓存命中（每 render 不再 6 次 localStorage 查找）

### progress 局部 patch
- [ ] `applyTaskProgress` 局部 patch 任务字段
- [ ] 通过 `tasks.value = [...tasks.value]` 触发 shallowRef 更新
- [ ] 不触发 sortedTasks 全量重排

### 局部重排
- [ ] `sortedIndices` computed 已实现（预计算排序索引）
- [ ] `displayedItems` 用 `sortedIndices` 映射
- [ ] 排序时不每次比较重新创建 Date

### 分组逻辑优化
- [ ] `groupedItems` 用 `tasksByRunId` 索引 O(n) 分组
- [ ] 无内层 `find` 退化 O(n²)
- [ ] DevTools Performance 无长任务

## 四、加解密任务适配

### 后端扩展
- [ ] `Task` 结构体新增 `CipherMode` / `CompressionMode` / `ExtraFields` 字段
- [ ] `createTask` 时从请求参数提取并持久化
- [ ] `getTasks` / `getTask` 返回时包含这些字段
- [ ] Go 单测覆盖

### 前端接口扩展
- [ ] `EncvTask` 接口新增 `cipherMode?` / `compressionMode?` / `extraFields?`
- [ ] `useWorkflowTaskService.submitAction` 确保传递参数给 createTask API
- [ ] TypeScript 编译通过

### 任务卡片展示
- [ ] `TaskBasicInfo.vue` 新增加解密参数区块
- [ ] `TaskTimeline.vue` 折叠态副标题显示摘要（如 `AES-256 | zstd`）
- [ ] `Tasks.vue` 任务卡片副标题显示摘要
- [ ] 刷新页面后参数回显（后端已持久化）

### useTestCaseGeneration 笛卡尔积
- [ ] 调查 plugin 元数据是否暴露 cipherMode/compressionMode 候选值
- [ ] 若暴露：笛卡尔积展开包含 cipherMode × compressionMode × extraFields
- [ ] 若未暴露：备注原因，保持现状

## 五、真机验证 + 回归测试

### 真机验证（120Hz 可变刷新率）
- [ ] Tasks.vue 滚动 200-500 task，快速滚动无白屏
- [ ] light/dark 双主题切换，所有组件视觉一致
- [ ] 加解密任务提交后，刷新页面参数回显
- [ ] 任务卡片展开/折叠滚动流畅

### 回归测试
- [ ] `pnpm test` 前端单测全部通过
- [ ] `bash scripts/test-go.sh ./internal/service` 后端单测通过
- [ ] lint 通过（`pnpm lint`）
- [ ] typecheck 通过（`pnpm typecheck`）

### 规则更新
- [ ] `.trae/rules/automation-workflow.md` 补充状态机终态保护、乱序缓冲规范
