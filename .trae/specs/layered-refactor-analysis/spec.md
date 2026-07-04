# 分层重构分析 Spec（前端重点）

## Why

经过本轮管线自洽性重构（产物路径从文件系统遍历改为管线返回值）和时间线可展开详情实现，前端层累积的架构债尤为突出：

- **`Tasks.vue` 725 行**：列表渲染、过滤/搜索/排序、刷新、eventBus 多重订阅、modal 触发全部糅在一个组件
- **`TaskDetailModal.vue` 440+ 行**：基本信息、时间线、产物、错误、警告、按钮全部在单文件
- **phase 字符串散落**：后端/前端通过裸字符串耦合（`"encrypting"`/`"completed"` 等），无类型保护
- **inline modal / 跨 tab eventBus**：曾修复一处，仍需全局复查
- **eventBus 监听器在 Tasks.vue 中多达 5+ 个**，部分可能违反 §2.1 铁律

后端部分本轮已通过**管线自洽性重构**（`findEncryptedOutputFile`/`findDecryptedOutputFile` 消除、Plugin 接口统一返回产物路径）解决了核心矛盾，**其他后端重构本 spec 暂不纳入**，按"小改"或"延后"处理。

## What Changes

新增本 spec 作为**前端重构元索引**：
- 列出前端各层的高优先级重构项（含严重程度、范围、影响）
- 后端部分只列出**小改清单**（如统一 phase 常量）
- 重大后端重构（Plugin 实现去重、TaskManager mutation API 化、CI 去重等）**明确延后到后续 spec**
- **不改变**任何运行时行为

## Impact

- Affected specs: 无（纯重构元 spec）
- Affected code（按层列出**全部候选**清单）：

### 前端 (Vue/Ionic) 层 — **本 spec 重点**

| 层级 | 候选文件 | 现状 | 严重程度 |
|------|---------|------|---------|
| View | `app/encv-mobile/src/views/Tasks.vue` | 725 行，承担：列表渲染、过滤/搜索/排序、刷新、eventBus 多重订阅、modal 触发、UI 状态机 | **P1** |
| View | `app/encv-mobile/src/views/Tasks.vue` | `onMounted` 中注册 5+ 个 eventBus 监听器，部分可能在跨 tab 场景下违反 §2.1 铁律 | **P1** |
| View | `app/encv-mobile/src/views/Files.vue` | 仍然使用 `eventBus.emit` 触发跨组件操作（之前修复了 modal 跨 tab，但其他事件流是否还有反模式？） | P2 |
| Component | `app/encv-mobile/src/components/TaskDetailModal.vue` | 440+ 行，承担：基本信息、时间线、产物展示、错误展示、警告展示、操作按钮 | **P1** |
| Component | `app/encv-mobile/src/components/TaskDetailModal.vue` | `phaseLabel` 映射函数 7 行 case 散落在 script 块中，可独立为 `usePhaseLabel.ts` composable | P2 |
| Component | `app/encv-mobile/src/components/NewTaskModal.vue` | 加密/解密双模式 if-else 大量重复，可考虑拆为 `<EncryptBody>` + `<DecryptBody>` 子组件 | P2 |
| Composable | `app/encv-mobile/src/composables/useTaskDetail.ts` | 文件存在但只暴露几个简单函数，可能未被使用或职责不清 | P2 |
| Composable | `app/encv-mobile/src/composables/useTaskForm.ts` | `doPredict` 内部有 500ms 防抖 + API 调用，但外层 `useNewTaskModal` 不知道这个时序（依赖外层 await），存在时序耦合 | P2 |
| Feature/action | `app/encv-mobile/src/features/alist-encrypt/actions.ts` | actions.ts 还在用 `router.push` 做导航（与 §1.4 modal 铁律相关） | P2 |
| API client | `app/encv-mobile/src/api/encv.ts` | 仍有 `fetch` 直接调用，未统一走 axios/ofetch | P2 |
| Type | `app/encv-mobile/src/types/task.ts` + `api/encv.ts` | `EncvTask` 类型与 `MobileTask` 后端类型分散两处 | P2 |

### 后端 (Go) 层 — **小改清单（已纳入本 spec）**

| 层级 | 候选文件 | 现状 | 处理方式 |
|------|---------|------|---------|
| 类型 | `internal/service/task_manager.go` 中的 phase 字符串 | `"encrypting"`/`"completed"`/`"failed"` 等裸字符串散落 | **小改**：新增 `internal/service/phase.go` 集中常量定义 |
| 阶段时间 | `task_manager.go` 的 `updateProgress` | 已记录 Steps 字段，但 phase 仍为字符串 | **小改**：复用上方 Phase 常量 |

### 后端 (Go) 层 — **延后清单（不纳入本 spec）**

| 候选 | 原因 |
|------|------|
| 6 个 V4 容器插件 PostEncryptProcessor 公共 helper | 改动面大，风险中，延后到独立 spec |
| 6 个 V4 容器插件 Decrypt 公共 helper | 同上 |
| TaskManager mutation API 化 | 改动面大，依赖业务方全部迁移 |
| HTTP Handler 模板化 | 改动面大，收益暂不明确 |
| Physical 打包层去重 | 性能影响需评估 |
| Service 层 `GetFileInfo` 简化 | ContainerHandle 已统一，分支已少 |
| CI 去重 | 独立工作流 |
| Makefile 入口统一 | 工具链重构 |

### 横切关注点

| 层级 | 候选 | 处理方式 |
|------|------|---------|
| Phase 字符串 | 后端 + 前端双份硬编码 | **小改**：后端常量 + 前端枚举 |
| i18n | 1299 行单文件 | **延后**：评估拆分价值低，暂不重构 |
| Error code | 字符串 + 字符串映射 | **延后**：需要独立 spec 设计 schema |

---

## ADDED Requirements

### Requirement: 分层重构 spec 入口

本 spec SHALL 作为后续前端重构任务的**索引入口**，每个 Phase 完成后**勾选 checklist 对应项**。

#### Scenario: 用户批准 Phase 1 后开始
- **WHEN** 用户批准本 spec 并指定"开始 Phase 1"
- **THEN** 实施对应原子任务（见 tasks.md 各项）
- **AND** 完成后**只勾选 Phase 1 对应的 checklist 项**
- **AND** 不擅自开始后续 Phase

#### Scenario: 跨 Phase 依赖
- **WHEN** Phase N 的原子任务完成且 checklist 全部勾选
- **THEN** 才允许开始 Phase N+1
- **AND** Phase 1 完成后，必须先在 test 环境验证再进入 Phase 2

### Requirement: 优先级评估准则

每个原子重构任务 SHALL 包含以下字段：

| 字段 | 含义 |
|------|------|
| **严重程度** | P0 (崩溃/数据丢失) / P1 (重复代码 >200行) / P2 (可读性/可测试性) |
| **改动行数预估** | 净增/净减行数 |
| **影响面** | 修改的文件数 + 公共 API 变更数 |
| **验证方式** | vue-tsc + 端到端测试 + 视觉回归 |

#### Scenario: 优先级判定
- **WHEN** 一个候选重构项被评估
- **THEN** P0 项立即处理
- **AND** P1 项列入下个 Phase
- **AND** P2 项累积到后续 Phase

---

## MODIFIED Requirements

### Requirement: TaskManager phase 字符串使用常量

后端 `task_manager.go` SHALL 使用 `internal/service/phase.go` 中定义的常量：

```go
// internal/service/phase.go
package service

const (
    PhaseCreated       = "created"
    PhaseAnalyzing     = "analyzing"
    PhaseInitializing  = "initializing"
    PhasePreprocessing = "preprocessing"
    PhaseEncrypting    = "encrypting"
    PhaseDecrypting    = "decrypting"
    PhasePacking       = "packing"
    PhaseVerifying     = "verifying"
    PhaseCompleted     = "completed"
    PhaseFailed        = "failed"
    PhaseCancelled     = "cancelled"
)
```

#### Scenario: 调用方
- **WHEN** task_manager.go 中出现裸字符串 `"encrypting"` / `"completed"` 等
- **THEN** 替换为 `service.PhaseEncrypting` / `service.PhaseCompleted`
- **AND** JSON 序列化值保持不变（仍是小写字符串）

### Requirement: 前端 Phase 枚举化

前端 SHALL 通过 `app/encv-mobile/src/types/phase.ts` 集中 phase 名称：

```typescript
export const Phase = {
  Created: 'created',
  Analyzing: 'analyzing',
  Initializing: 'initializing',
  Preprocessing: 'preprocessing',
  Encrypting: 'encrypting',
  Decrypting: 'decrypting',
  Packing: 'packing',
  Verifying: 'verifying',
  Completed: 'completed',
  Failed: 'failed',
  Cancelled: 'cancelled',
} as const

export type PhaseValue = typeof Phase[keyof typeof Phase]
```

#### Scenario: 引用方
- **WHEN** Tasks.vue / TaskDetailModal.vue 中出现裸字符串 `'encrypting'` / `'completed'`
- **THEN** 替换为 `Phase.Encrypting` / `Phase.Completed`
- **AND** 与后端 `service.Phase*` 常量值保持一一对应

### Requirement: i18n 键名派生自 Phase

`useI18n.ts` 中 `tasks.phaseEncrypting` / `tasks.phaseCompleted` 等键 SHALL 由 Phase 枚举派生：

```typescript
// 派生方式（示意）
const phaseLabelKey = (phase: PhaseValue) => `tasks.phase${phase[0].toUpperCase()}${phase.slice(1)}`
```

#### Scenario: 派生后
- **WHEN** 重构时
- **THEN** `getPhaseLabel(phase)` 函数从 `if-else 链` 改为 `phaseLabelKey + t()` 派生
- **AND** 减少 7+ 行 case 散落

---

## MODIFIED Requirements（前端重点）

### Requirement: Tasks.vue 拆分为 store + 视图

`Tasks.vue` 725 行的状态管理逻辑 SHALL 迁移到 Pinia store（如项目已引入）或 `useTasksStore` composable。

#### Scenario: 拆分目标
- **WHEN** 重构时
- **THEN** 提取 `useTasksStore()`：负责列表、过滤、刷新、eventBus 订阅
- **AND** `Tasks.vue` 只剩 UI 渲染 + 用户事件分发
- **AND** `<template>` 部分行数 < 200

#### Scenario: eventBus 审查
- **WHEN** 拆分 store 时
- **THEN** 重新审查 `Tasks.vue` 中注册的 5+ 个 eventBus 监听
- **AND** 跨 tab 的迁移到 composable 调用（同 §1.4 铁律）
- **AND** 同 tab 自消费的保留在 store 内
- **AND** 文档化每个监听器的"是否跨 tab"决策

### Requirement: TaskDetailModal 子组件化

`TaskDetailModal.vue` 440+ 行 SHALL 拆分为：
- `<TaskBasicInfo>` — 文件名/类型/插件/容器版本
- `<TaskTimeline>` — 时间线（含已完成的 steps 展开逻辑）
- `<TaskOutputInfo>` — 产物展示
- `<TaskErrorSection>` — 错误展示
- `<TaskWarningSection>` — 警告展示
- `<TaskActionButtons>` — 取消/重试/移除

#### Scenario: 拆分后
- **WHEN** 拆分完成
- **THEN** `TaskDetailModal.vue` < 100 行，只做组合
- **AND** 每个子组件独立可测
- **AND** i18n 键名仍统一在 `useI18n.ts`

### Requirement: NewTaskModal 加密/解密模式拆分

`NewTaskModal.vue` SHALL 评估拆分为：
- `<EncryptBody>` — 加密表单（密码 + 插件选项）
- `<DecryptBody>` — 解密表单（密码 + 输出路径）

#### Scenario: 拆分条件
- **WHEN** 加密/解密分支各自 > 100 行模板代码
- **AND** 分支间 prop 不同
- **THEN** 拆分为子组件
- **ELSE** 保留单文件但提取公共部分为 composable

### Requirement: Composable 时序解耦

`useTaskForm.ts` 的 `doPredict` 防抖时序 SHALL 与 `useNewTaskModal` 的打开逻辑解耦：

#### Scenario: 当前耦合
- **WHEN** `useNewTaskModal.openNewTask()` 内部 `await doPredict()`
- **AND** `doPredict` 内部有 500ms setTimeout 防抖
- **THEN** 调用方需要 `await`，但不知道内部有 500ms 防抖
- **AND** 时序依赖隐式、不易测试

#### Scenario: 解耦后
- **WHEN** 重构时
- **THEN** `doPredict` 返回 `Promise<PredictResult>`，调用方选择 await 或不 await
- **AND** 防抖时序作为实现细节隐藏在 `useTaskForm` 内部
- **AND** 单元测试可 mock `doPredict` 返回固定结果

### Requirement: API client 统一

`app/encv-mobile/src/api/encv.ts` 中散落的 `fetch` 调用 SHALL 统一为单一 HTTP 客户端：

#### Scenario: 统一前
- **WHEN** 出现 `await fetch(url, { ... })` 直调
- **THEN** 改为 `await httpClient.get<T>(url)` 或 `await apiGet('/path', params)`

#### Scenario: 统一后
- **WHEN** 引入 ofetch/axios 统一实例
- **THEN** 集中处理：baseURL、auth header、错误格式、retry 策略
- **AND** 测试可 mock httpClient 拦截请求

### Requirement: 事件总线全局复查

项目 SHALL 全面审查 eventBus 使用，识别所有跨 tab 反模式：

| 事件名 | 发射者 | 监听者 | 跨 tab? | 决策 |
|--------|--------|--------|---------|------|
| （待审查填充） | | | | |

#### Scenario: 审查流程
- **WHEN** 审查时
- **THEN** 列出所有 `eventBus.emit/on` 调用
- **AND** 对每条标注"同组件/同 tab 跨组件/跨 tab"
- **AND** 跨 tab 的迁移到 composable 直接调用（§2.1 铁律）
- **AND** 审查结果写入 `checklist.md` Phase 0 表格

---

## REMOVED Requirements

无。本 spec 是元 spec，不删除任何功能，只是规划后续前端原子重构任务。
