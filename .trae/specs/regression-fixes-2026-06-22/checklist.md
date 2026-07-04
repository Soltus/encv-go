# Checklist: 8 项关键回归修复（2026-06-22）

## Q1A CI 修复
- [ ] task_manager_crypto_params_test.go: 4 处 CreateWithCryptoParams → CreateWithRunMeta
- [ ] 测试注释更新
- [ ] go test ./internal/service/... 全绿

## Q2B 错误详情双轨
- [ ] task_manager.go: classifyError helper
- [ ] task_manager.go: 失败路径写 task.ErrorDetail
- [ ] task_manager.go: saveTaskSingle 单行写
- [ ] TaskErrorSection.vue: useErrorAnalyzer 分类 chip
- [ ] TaskErrorSection.vue: 修复建议列表
- [ ] TaskErrorSection.vue: phase 时间链
- [ ] TaskErrorSection.vue: errorDetail 折叠
- [ ] TaskErrorSection.vue: 复制错误按钮

## Q3A 性能指标 summary
- [ ] TaskDetailModal.vue: 集成 TaskPerformanceSection
- [ ] 完成态任务显示 performanceSummary
- [ ] 运行中显示"性能指标将在完成后显示"

## Q4 GroupDetail UI 重构（平铺 + 顶层复用）
- [ ] GroupDetail.vue: 顶层 action bar 含 segment/搜索/筛选/多选/导出
- [ ] GroupDetail.vue: 底部 sticky 批量操作栏
- [ ] TasksTab.vue: 顶部筛选 chip 行
- [ ] TasksTab.vue: 多选模式 toggle
- [ ] TasksTab.vue: 选中视觉反馈
- [ ] DiagnosticsTab.vue: 合并入 TasksTab
- [ ] PipelineTab.vue: 折叠 DAG 树
- [ ] useTaskFiltering composable
- [ ] useBatchOperations composable
- [ ] TaskVirtualList 复用
- [ ] 选中 Set<string> O(1)
- [ ] ion-segment 4 → 2

## Q5A 滑动操作
- [ ] Tasks.vue: 删 @contextmenu.prevent
- [ ] Tasks.vue: 删 @touchstart/@touchend
- [ ] Tasks.vue: 删 onGroupTouchStart/End
- [ ] Tasks.vue: group card 用 ion-item-sliding
- [ ] Tasks.vue: 左滑取消 / 右滑置顶+删除
- [ ] Tasks.vue: openGroupActionSheet 保留 a11y

## Q6A 逃逸根治
- [ ] taskStore.ts: applyTaskCreated 改 await persistPut
- [ ] taskStore.ts: reconcileWithBackend
- [ ] useTasksList.ts: onMounted 调 reconcileWithBackend
- [ ] taskPersistence.ts: persistPut 重试 3 次
- [ ] submitAction 后立即 applyTaskCreated

## Q7C 报告下载
- [ ] GroupDetail.vue: exportGroupReport 重写
- [ ] native: Filesystem.writeFile + getUri + Share.files
- [ ] web: 保留 a.download
- [ ] 失败 fallback

## Q8B taskType 查表
- [ ] 新建 taskTypeLabel.ts
- [ ] i18n/tasks.ts: 12 个新 key
- [ ] Tasks.vue L396 替换
- [ ] useTasksList.ts L377 替换
- [ ] TaskBasicInfo.vue L85-86 替换
- [ ] TasksTab.vue L27,61 替换
- [ ] NewTaskModal.vue 替换
- [ ] TaskDetailModal.vue 替换
- [ ] switch 加 default
- [ ] 6 类型专属 UI

## 验证
- [ ] go test ./internal/service/... 通过
- [ ] go build ./... 通过
- [ ] vue-tsc --noEmit 通过
- [ ] vite build 通过
- [ ] e2e: 1000+ 任务无逃逸
- [ ] e2e: native 报告下载
- [ ] e2e: 滑动操作
- [ ] e2e: 12 种 taskType 显示
