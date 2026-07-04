# Spec: 8 项关键回归修复（2026-06-22）

> 创建：2026-06-22
> 状态：待用户确认 → 已确认
> 关联：用户报告 8 项问题 + Q1A/Q2B/Q3A/Q4(平铺复用顶层操作)/Q5A/Q6A/Q7C/Q8B 决策

---

## 一、Why

### 1.1 问题清单（用户 2026-06-22 报告）

| # | 问题 | 现状 |
|---|------|------|
| 1 | CI 8 个同类报错 | `tm.CreateWithCryptoParams undefined`（测试文件引用了不存在的 API） |
| 2 | 任务详情错误 UI 无详情 | 只显示标题，例如"source file not found" |
| 3 | 任务详情无性能指标 | 加密任务详情页未渲染 performanceSummary |
| 4 | 任务 UI 设计缺陷 | Pipeline 丢失 tree 视图；多任务不可用；任务无分组/筛选/搜索/删除；卡顿明显 |
| 5 | 任务卡片长按操作 | 不符合用户习惯，需恢复为左右滑 |
| 6 | 任务逃逸 | 1000+ 任务中 10+ 个没有 `runId/triggeredBy`；重启应用消失 |
| 7 | 报告下载失败 | "Unsupported url" 错误信息不足 |
| 8 | 任务类型错乱 | 文件长按删除创建的任务被显示为"解密"；"file task handler not configured" |

### 1.2 用户决策（2026-06-22 确认）

| Q | 决策 | 解读 |
|---|------|------|
| Q1 | **A** 改名映射 | 测试里 `CreateWithCryptoParams` → `CreateWithRunMeta` |
| Q2 | **B** errorDetail + analyzer 双轨 | 后端写 `errorDetail` JSON；前端折叠显示 + 分类建议 |
| Q3 | **A** 现成 summary | 任务详情用 `task.performanceSummary`；不补查 |
| Q4 | **B + 平铺复用顶层操作** | 合并 tab；平铺结构；筛选/搜索/批量作为顶层 action bar 的展开区；底部 sticky 批量栏；Pipeline 折叠为 DAG 树 |
| Q5 | **A** 纯 ion-item-sliding | 删除 touchstart/touchend 长按模拟 |
| Q6 | **A** 同步持久化 + 启动重建 | 同步 await persistPut；启动时 `reconcileWithBackend` |
| Q7 | **C** native Filesystem + web download | native 走 Filesystem+content://；web 保留 a.download |
| Q8 | **B** typeMap 查表 + 6 类型 UI | 6 种类型都有专属 UI |

---

## 二、What Changes

### 2.1 后端

#### 2.1.1 [task_manager_crypto_params_test.go](file:///workspace/internal/service/task_manager_crypto_params_test.go) — 改名映射
- `CreateWithCryptoParams(...)` → `CreateWithRunMeta(...)` × 4 处
- 注释同步

#### 2.1.2 [task_manager.go](file:///workspace/internal/service/task_manager.go) — ErrorDetail 写入
- 失败路径统一写 `task.ErrorDetail = JSON({phase, reason, ...details})`
- 加 helper `classifyError(phase, err) (category, errorDetail)`
- `CreateWithRunMeta` 末尾 `saveTasks()` → `saveTasksAsync()`（1000+ 并发不阻塞）
- 加 `saveTaskSingle(task)` 写单行

### 2.2 前端

#### 2.2.1 [TaskErrorSection.vue](file:///workspace/app/encv-mobile/src/components/TaskErrorSection.vue) — 重写
- 错误分类 chip（useErrorAnalyzer 12 类）
- 修复建议列表
- phase 时间链（6 phase 横道图）
- errorDetail JSON 折叠/展开
- 复制错误按钮

#### 2.2.2 [GroupDetail.vue](file:///workspace/app/encv-mobile/src/views/GroupDetail.vue) — 顶层 action bar 扩展
- 顶层 action bar：segment 切换（pipeline/tasks） + 搜索框 + 筛选按钮 + 多选 toggle + 导出
- 底部 sticky 批量操作栏：已选 N 项 + 取消选中/取消运行/删除/重试
- `exportGroupReport` 重写：native 走 Filesystem+Share.files；web 保留 a.download

#### 2.2.3 [TasksTab.vue](file:///workspace/app/encv-mobile/src/components/group-detail/TasksTab.vue) — 扩展
- 顶部 24px 筛选 chip 行
- 多选模式 toggle
- 选中视觉反馈

#### 2.2.4 [DiagnosticsTab.vue](file:///workspace/app/encv-mobile/src/components/group-detail/DiagnosticsTab.vue) — 合并入 TasksTab
- diagnostics = tasks 的 status ∈ {failed, cancelled} 筛选
- 移除独立 tab

#### 2.2.5 [PipelineTab.vue](file:///workspace/app/encv-mobile/src/components/group-detail/PipelineTab.vue) — 重写为折叠 DAG
- 默认折叠：仅显示"树视图"按钮
- 点击展开：递归渲染 JobRun → StepRun 树

#### 2.2.6 [Tasks.vue](file:///workspace/app/encv-mobile/src/views/Tasks.vue) — 改回滑动
- 删 `@contextmenu` + `@touchstart/@touchend`
- group card → `ion-item-sliding` 包裹（左滑取消 / 右滑置顶+删除）

#### 2.2.7 [taskStore.ts](file:///workspace/app/encv-mobile/src/stores/taskStore.ts) — 同步持久化 + 启动重建
- `applyTaskCreated` 持久化改为 `await persistPut`
- 新增 `reconcileWithBackend(serverTasks)`
- `submitAction` 返回后立即 `applyTaskCreated(task)` + 同步持久化

#### 2.2.8 [useTasksList.ts](file:///workspace/app/encv-mobile/src/composables/useTasksList.ts) — 启动重建
- onMounted 调 `reconcileWithBackend(getTasks())`

#### 2.2.9 [taskPersistence.ts](file:///workspace/app/encv-mobile/src/lib/taskPersistence.ts) — 失败重试
- `persistPut` try/catch + 重试 3 次（50/100/200ms）

#### 2.2.10 新建 [taskTypeLabel.ts](file:///workspace/app/encv-mobile/src/lib/taskTypeLabel.ts)
- `getTaskTypeLabel(t, tFunc)` / `getTaskTypeIcon(t)` / `getTaskTypeColor(t)`

#### 2.2.11 [i18n/tasks.ts](file:///workspace/app/encv-mobile/src/i18n/tasks.ts) — 加 12 个 key
- `tasks.type.encrypt/decrypt/move/copy/rename/delete`
- `tasks.type.rollback_encrypt/decrypt/move/copy/rename/delete`
- `tasks.type.unknown`

#### 2.2.12 6 处硬二分替换
- [Tasks.vue#L396](file:///workspace/app/encv-mobile/src/views/Tasks.vue#L396)
- [useTasksList.ts#L377](file:///workspace/app/encv-mobile/src/composables/useTasksList.ts#L377)
- [TaskBasicInfo.vue#L85-86](file:///workspace/app/encv-mobile/src/components/TaskBasicInfo.vue#L85-86)
- [TasksTab.vue#L27, 61](file:///workspace/app/encv-mobile/src/components/group-detail/TasksTab.vue#L27)
- [NewTaskModal.vue](file:///workspace/app/encv-mobile/src/components/NewTaskModal.vue)
- [TaskDetailModal.vue](file:///workspace/app/encv-mobile/src/components/TaskDetailModal.vue)

#### 2.2.13 [TaskDetailModal.vue](file:///workspace/app/encv-mobile/src/components/TaskDetailModal.vue) — 6 类型专属 UI
- delete: originalPath + 删除时间
- move: fromPath + targetPath
- copy: sourcePath + targetPath
- rename: originalPath + targetPath
- 6 类型切换 badge 文案/图标/颜色统一查表

---

## 三、Impact

### 3.1 风险评估
- 低风险：Q1（改名）/ Q2（增量）/ Q3（不改）/ Q5（回归）/ Q6（增量）/ Q7（条件分支）/ Q8（统一）
- 中风险：Q4（UI 重构，组件多）

### 3.2 兼容性
- WS payload 扩展加字段，向后兼容
- `EncvTask.type` 取值集合不变（仍 12 种），仅前端不再硬二分

### 3.3 性能
- `saveTaskSingle` 比 `saveTasks` 写全表快 100x（1000+ 并发无 IO 抖动）
- `reconcileWithBackend` O(n) 一次
- 任务列表虚拟滚动 + 筛选 memoized

---

## 四、ADDED Requirements

1. CI 测试文件所有 `CreateWithCryptoParams` 引用替换为 `CreateWithRunMeta`
2. 任务失败时 `task.ErrorDetail` 必须有结构化 JSON
3. TaskErrorSection 必须显示 useErrorAnalyzer 分类 + phase 链
4. GroupDetail 顶层 action bar 包含：segment 切换 + 搜索 + 筛选 + 多选 + 导出
5. GroupDetail 底部 sticky 批量操作栏：选中 N 项时显示
6. 任务列表支持：状态/类型/plugin 多选筛选 + 文本搜索 + 批量删除/取消/重试
7. Pipeline 树视图：默认折叠，点击展开
8. Tasks.vue 任务卡片仅支持 ion-item-sliding（左滑取消/右滑置顶+删除）
9. submitAction 后必须 `await persistPut(task)`
10. 启动时 `reconcileWithBackend` 强制同步后端权威层
11. native 报告下载走 Filesystem → content:// URI → Share.files
12. 12 种 taskType 全部走 typeMap 查表（无硬二分）
13. delete/move/copy/rename 任务在 TaskDetailModal 显示专属元数据

---

## 五、MODIFIED Requirements

1. DiagnosticsTab.vue → 并入 TasksTab（作为 status 筛选的预设）
2. GroupDetail ion-segment 从 4 个（pipeline/tasks/diagnostics/performance）→ 2 个（pipeline/tasks）
3. PerformanceTab 保留在 tasks tab 内作为 collapsible section

---

## 六、REMOVED Requirements

1. Tasks.vue 移除 `@contextmenu.prevent` + `@touchstart/@touchend` 模拟长按
2. TaskErrorSection 移除纯字符串展示（旧版 `task.error` 单行）

---

## 七、验收

- [ ] `go test ./internal/service/...` 通过（CreateWithRunMeta 4 个 test 全绿）
- [ ] `go build ./...` 通过
- [ ] `vue-tsc --noEmit` 通过
- [ ] `vite build` 通过
- [ ] TaskDetailModal 错误详情显示分类 + 建议 + phase 链
- [ ] TaskDetailModal 加密任务显示 performanceSummary
- [ ] GroupDetail 顶层 action bar 含 segment/搜索/筛选/多选/导出
- [ ] GroupDetail 选中后底部出现 sticky 批量操作栏
- [ ] Pipeline 折叠为 DAG 树
- [ ] Tasks.vue 滑动操作可用
- [ ] 1000+ 任务无逃逸
- [ ] native 真机报告下载走 Filesystem
- [ ] 12 种 taskType 全部正确显示（无"删除→解密"错乱）
