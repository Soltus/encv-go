# Tasks: 8 项关键回归修复（2026-06-22）

> 共 8 个 Phase，~50 个 Task

## Phase 1: CI 修复（Q1A）

- [ ] T1.1 改 [task_manager_crypto_params_test.go](file:///workspace/internal/service/task_manager_crypto_params_test.go) 中 4 个 test 的 `CreateWithCryptoParams` → `CreateWithRunMeta`
- [ ] T1.2 改测试注释
- [ ] T1.3 `go test ./internal/service/...` 通过

## Phase 2: 任务详情错误 + 性能指标（Q2B + Q3A）

- [ ] T2.1 后端 [task_manager.go](file:///workspace/internal/service/task_manager.go) 加 `classifyError(phase, err) (category, errorDetail JSON string)`
- [ ] T2.2 失败路径统一写 `task.ErrorDetail`
- [ ] T2.3 加 `saveTaskSingle(task)` 高频单行写
- [ ] T2.4 `CreateWithRunMeta` 改用 `saveTaskSingle`
- [ ] T2.5 重写 [TaskErrorSection.vue](file:///workspace/app/encv-mobile/src/components/TaskErrorSection.vue)：分类 chip + 修复建议 + phase 链 + errorDetail 折叠
- [ ] T2.6 确认 [TaskDetailModal](file:///workspace/app/encv-mobile/src/components/TaskDetailModal.vue) 集成 TaskPerformanceSection + 显示 `task.performanceSummary`
- [ ] T2.7 运行中任务显示"性能指标将在任务完成后显示"

## Phase 3: GroupDetail UI 重构（Q4 平铺 + 顶层复用）

- [ ] T3.1 扩展 [GroupDetail.vue](file:///workspace/app/encv-mobile/src/views/GroupDetail.vue) 顶层 action bar：segment 切换（pipeline/tasks）+ 搜索框 + 筛选按钮 + 多选 toggle + 导出
- [ ] T3.2 加底部 sticky 批量操作栏：已选 N 项 + 取消选中/取消运行/删除/重试
- [ ] T3.3 扩展 [TasksTab.vue](file:///workspace/app/encv-mobile/src/components/group-detail/TasksTab.vue)：顶部筛选 chip 行 + 多选模式 toggle + 选中视觉反馈
- [ ] T3.4 合并 [DiagnosticsTab.vue](file:///workspace/app/encv-mobile/src/components/group-detail/DiagnosticsTab.vue) → TasksTab
- [ ] T3.5 重写 [PipelineTab.vue](file:///workspace/app/encv-mobile/src/components/group-detail/PipelineTab.vue) 为折叠 DAG 树
- [ ] T3.6 加 `useTaskFiltering` composable（O(n) + memoized）
- [ ] T3.7 加 `useBatchOperations` composable（删除/取消/重试/置顶）
- [ ] T3.8 性能：复用 TaskVirtualList + 选中 Set<string> O(1)
- [ ] T3.9 ion-segment 从 4 → 2（pipeline/tasks），PerformanceTab 移到 tasks tab 内 collapsible

## Phase 4: 任务卡片滑动（Q5A）

- [ ] T4.1 [Tasks.vue](file:///workspace/app/encv-mobile/src/views/Tasks.vue) 删 `@contextmenu.prevent` + `@touchstart` + `@touchend`
- [ ] T4.2 删 `onGroupTouchStart/End` 函数
- [ ] T4.3 group card → `ion-item-sliding` 包裹（左滑取消 / 右滑置顶+删除）
- [ ] T4.4 单 task card 仅 click 跳转详情
- [ ] T4.5 保留 `openGroupActionSheet`（a11y 备用）

## Phase 5: 任务逃逸根治（Q6A）

- [ ] T5.1 [taskStore.ts](file:///workspace/app/encv-mobile/src/stores/taskStore.ts) `applyTaskCreated` 改 `await persistPut`
- [ ] T5.2 加 `reconcileWithBackend(serverTasks: EncvTask[])`
- [ ] T5.3 [useTasksList.ts](file:///workspace/app/encv-mobile/src/composables/useTasksList.ts) onMounted 调 `reconcileWithBackend`
- [ ] T5.4 [taskPersistence.ts](file:///workspace/app/encv-mobile/src/lib/taskPersistence.ts) `persistPut` 加 try/catch + 重试 3 次
- [ ] T5.5 submitAction 后立即 `applyTaskCreated` + 同步持久化

## Phase 6: 报告下载（Q7C）

- [ ] T6.1 [GroupDetail.vue](file:///workspace/app/encv-mobile/src/views/GroupDetail.vue) `exportGroupReport` 重写
- [ ] T6.2 native: Filesystem.writeFile + getUri + Share.files
- [ ] T6.3 web: 保留 URL.createObjectURL + `<a download>`
- [ ] T6.4 失败 fallback: toast + 复制到剪贴板

## Phase 7: taskType 硬二分修复（Q8B）

- [ ] T7.1 新建 [taskTypeLabel.ts](file:///workspace/app/encv-mobile/src/lib/taskTypeLabel.ts)
- [ ] T7.2 [i18n/tasks.ts](file:///workspace/app/encv-mobile/src/i18n/tasks.ts) 加 12 个 key（中英文）
- [ ] T7.3 替换 6 处硬二分（Tasks.vue / useTasksList.ts / TaskBasicInfo.vue / TasksTab.vue / NewTaskModal.vue / TaskDetailModal.vue）
- [ ] T7.4 所有 `switch (taskType)` 加 default 分支
- [ ] T7.5 [TaskDetailModal.vue](file:///workspace/app/encv-mobile/src/components/TaskDetailModal.vue) 6 类型专属 UI
- [ ] T7.6 TasksTab.vue 显示任务时也走 typeMap（删除任务显示"删除"图标）

## Phase 8: 验证

- [ ] T8.1 `go test ./internal/service/...` 通过
- [ ] T8.2 `go build ./...` 通过
- [ ] T8.3 `vue-tsc --noEmit` 通过
- [ ] T8.4 `vite build` 通过
- [ ] T8.5 手动端到端：1000+ 任务逃逸测试
- [ ] T8.6 手动端到端：报告下载 native 真机
- [ ] T8.7 手动端到端：滑动操作
- [ ] T8.8 手动端到端：12 种 taskType 显示正确
