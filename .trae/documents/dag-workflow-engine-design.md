# DAG 工作流引擎设计文档

> **决策记录**
> - 编排模型：DAG 工作流引擎（GitHub Actions 对齐）
> - 状态机：全量对齐（pending → submitted → queued → running → success/failure/cancelled/skipped/timed_out）
> - 存储：混合模式（MVP 前端 localStorage + 后端 API 预留签名）
> - UI：可切换双视图（Pipeline 卡片默认 + 树形详情）

---

## 一、数据模型

### 1.1 核心类型层次

```
WorkflowDefinition (静态定义，可保存/加载)
├── WorkflowRun     (一次执行的运行实例)
│   ├── JobRun[]    (Job 的运行实例)
│   │   └── StepRun[]  (Step 的运行实例，每个 StepRun 对应一个 EncvTask)
```

### 1.2 WorkflowDefinition（工作流定义）

```typescript
interface WorkflowDefinition {
  id: string                    // UUID v4
  name: string                  // "自动化测试套件"
  description?: string
  createdAt: string             // ISO 8601
  updatedAt: string

  // 触发方式
  trigger: 'manual' | 'on_event' | 'schedule'

  // 全局变量 / 上下文注入
  env?: Record<string, string>  // { PASSWORD: 'automation-test-pwd' }

  // 并发策略
  concurrency:
    | { maxParallel: number }          // 最多 N 个 job 并行
    | { group: string; cancelInProgress: boolean }  // 同组互斥

  jobs: JobDefinition[]
}
```

### 1.3 JobDefinition（作业定义）

```typescript
interface JobDefinition {
  id: string              // 'test-encrypt', 'cleanup'
  name: string            // "加密测试"

  // === 依赖控制 ===
  needs?: string[]       // ['setup-mock'] — 等待这些 job 完成
  if?: ConditionExpr     // 条件执行（见 §1.6）

  // === 并行策略 ===
  strategy?:
    | { type: 'matrix'; axes: Record<string, string[]> }  // 笛卡尔积展开
    | { type: 'parallel'; max: number }                   // 固定并发数
    | { type: 'sequential' }                              // 严格顺序

  // === 超时 ===
  timeoutMinutes?: number

  steps: StepDefinition[]
}
```

### 1.4 StepDefinition（步骤定义）

```typescript
interface StepDefinition {
  id: string               // 'step-001'
  name: string             // "AES-256 + zstd 加密"

  // 动作类型
  action: ActionSpec

  // 条件
  if?: ConditionExpr

  // 是否允许失败（continue-on-error）
  continueOnError?: boolean

  // 超时
  timeoutSeconds?: number
}

type ActionSpec =
  | { type: 'encv_task'; taskType: TaskType; pluginName: string; params: TaskParams }
  | { type: 'shell'; command: string }           // 未来扩展
  | { type: 'http_request'; url: method; body }  // 未来扩展
```

### 1.5 运行时实例

```typescript
type StepStatus = 'pending' | 'queued' | 'running' | 'success' | 'failure' | 'cancelled' | 'skipped' | 'timed_out'
type JobStatus = StepStatus                      // 复用同一套状态
type WorkflowStatus = 'pending' | 'running' | 'success' | 'failure' | 'cancelled'

interface StepRun {
  id: string
  stepDefId: string        // 引用 StepDefinition.id
  status: StepStatus
  startedAt?: string
  completedAt?: string
  durationMs?: number
  taskId?: string          // 关联的 EncvTask.id
  error?: string
  errorAnalysis?: ErrorAnalysis
  output?: Record<string, any>
}

interface JobRun {
  id: string
  jobDefId: string         // 引用 JobDefinition.id
  status: JobStatus
  conclusion?: 'success' | 'failure' | 'skipped' | 'cancelled'
  startedAt?: string
  completedAt?: string
  steps: StepRun[]
  matrixVars?: Record<string, string>  // matrix 展开时的变量快照
}

interface WorkflowRun {
  id: string
  workflowDefId: string
  status: WorkflowStatus
  triggeredBy: TriggeredBy
  createdAt: string
  startedAt?: string
  completedAt?: string
  durationMs?: number
  jobs: JobRun[]
}
```

### 1.6 条件表达式（ConditionExpr）

参考 GitHub Actions `if:` 语法，简化版：

```typescript
// AST 节点
type ConditionExpr =
  | { op: 'always' }
  | { op: 'success' }            // 上一步成功
  | { op: 'failure' }            // 上一步失败
  | { op: 'eq'; left: string; right: string }
  | { op: 'neq'; left: string; right: string }
  | { op: 'and'; children: ConditionExpr[] }
  | { op: 'or'; children: ConditionExpr[] }
  | { op: 'not'; child: ConditionExpr }

// 示例：
// "如果上一步失败则跳过"
{ op: 'not', child: { op: 'success' } }

// "如果是 encrypt 类型且版本为 4"
{
  op: 'and',
  children: [
    { op: 'eq', left: '${{ taskType }}', right: 'encrypt' },
    { op: 'eq', left: '${{ version }}', right: '4' },
  ]
}
```

---

## 二、状态机

### 2.1 状态转换图

```
                    ┌─────────────┐
                    │   pending   │ ← 创建但未开始
                    └──────┬──────┘
                           │ run()
                           ▼
                    ┌─────────────┐
                    │  submitted  │ ← 已提交到后端队列
                    └──────┬──────┘
                           │ 后端接受
                           ▼
                    ┌─────────────┐
              ┌────▶│   queued    │ ◀─── 依赖等待中
              │     └──────┬──────┘
              │            │ 调度器分配
              │            ▼
              │     ┌─────────────┐
              │     │   running   │
              │     └──┬────┬────┘
              │        │    │
              │   成功│    │失败/超时/取消
              │        ▼    ▼
              │   ┌────────┐ ┌──────────┐
              │   │ success │ │ failure  │
              │   └────────┘ └──────────┘
              │                     │
              │  continueOnError=true│
              │                     ▼
              │              ┌──────────┐
              └─────────────▶│ skipped  │
                             └──────────┘
```

### 2.2 合法转换表

| 当前状态 | 可转入 |
|---------|--------|
| pending | submitted, cancelled |
| submitted | queued, cancelled |
| queued | running, cancelled |
| running | success, failure, cancelled, timed_out |
| success | （终态）|
| failure | （终态）|
| cancelled | （终态）|
| skipped | （终态）|
| timed_out | （终态）|

### 2.3 Job 结论映射（conclusion）

Job 的最终结论由其 Steps 决定：

| 规则 | conclusion |
|------|-----------|
| 所有 step = success | **success** |
| 任一 step = failure 且 continueOnError=false | **failure** |
| 任一 step = timed_out | **failure** |
| 任一 step = cancelled | **cancelled** |
| 有 step = skipped（其余 success） | **success** |

---

## 三、DAG 编排引擎

### 3.1 架构

```
useWorkflowEngine (composable)
├── Scheduler（调度器）
│   ├── resolveDAG()      — 拓扑排序，计算可并行层
│   ├── tick()            — 每轮检查依赖、派发就绪 job
│   └── onStepCompleted() — WS 回调触发状态转移
├── Executor（执行器）
│   ├── executeStep()     — 调用 createTask() 提交到后端
│   └── handleMatrix()    — matrix 展开为多个 StepRun
├── Store（存储）
│   ├── saveDefinition()  — localStorage / IndexedDB
│   ├── loadDefinition()  — 从本地或后端加载
│   └── saveRun()         — 运行历史
└── Observer（响应式）
    └── runs / currentRun / status — Vue ref 驱动 UI
```

### 3.2 调度算法

```
function resolveExecutionOrder(jobs: JobDefinition[]): string[][] {
  // 1. 构建邻接表（needs → depends on）
  // 2. Kahn's algorithm 拓扑排序
  // 3. 按层级分组（同层可并行）
  // 返回：[[job-id-1, job-id-2], [job-id-3], ...]
}
```

### 3.3 执行流程

```
用户点击 "Run Workflow"
  │
  ├─ 1. 创建 WorkflowRun（status=pending）
  ├─ 2. 为每个 Job 创建 JobRun（status=pending）
  ├─ 3. resolveDAG() → 计算执行顺序
  ├─ 4. 标记第 0 层 JobRun 为 queued
  ├─ 5. 对每个 queued JobRun：
  │    ├─ 如果有 strategy.matrix → 展开为 N 个 StepRun
  │    ├─ 对每个 StepRun：
  │    │    ├─ evaluate(if) → false 则标记 skipped
  │    │    └─ executeStep() → createTask()
  │    │         ├─ 成功 → StepRun.status=running
  │    │         └─ 失败 → StepRun.status=failure
  │    └─ 所有 Step 完成后 → 计算 Job conclusion
  │
  ├─ 6. WS 回调 onStepCompleted():
  │    ├─ 更新 StepRun 状态
  │    ├─ 检查所属 JobRun 是否全部完成
  │    │    ├─ 是 → 更新 JobRun conclusion
  │    │    │    ├─ 检查下游依赖是否满足
  │    │    │    └─ 满足则将下一层 JobRun 标记为 queued → goto 5
  │    │    └─ 否 → 等待
  │    └─ 所有 JobRun 完成 → WorkflowRun.status=success/failure
  │
  └─ 7. 保存运行历史到 localStorage
```

---

## 四、API 表面（混合模式）

### 4.1 前端 Composable

```typescript
// useWorkflowEngine.ts
function useWorkflowEngine() {
  // 定义管理
  const definitions: Ref<WorkflowDefinition[]>
  function createDefinition(def: Partial<WorkflowDefinition>): WorkflowDefinition
  function updateDefinition(id: string, patch: Partial<WorkflowDefinition>): void
  function deleteDefinition(id: string): void

  // 执行
  const runs: Ref<WorkflowRun[]>
  const currentRun: Ref<WorkflowRun | null>
  async function runWorkflow(defId: string): Promise<WorkflowRun>
  function cancelRun(runId: string): void

  // 预置模板
  const builtinTemplates: WorkflowDefinition[]  // 自动化测试 / 批量转码 / ...

  return { definitions, runs, currentRun, createDefinition, ... }
}
```

### 4.2 后端 API 预留签名（未来迁移）

```go
// internal/server/workflow.go（stub）

// POST /api/workflows          — 创建/更新定义
// GET  /api/workflows          — 列表
// GET  /api/workflows/:id      — 详情
// DELETE /api/workflows/:id    — 删除

// POST /api/workflows/:id/runs — 触发运行
// GET  /api/workflows/:id/runs — 运行历史
// GET  /api/runs/:runId        — 运行详情（含所有 Job/Step 状态）
// POST /api/runs/:runId/cancel — 取消运行
```

MVP 阶段这些接口返回 501 Not Implemented 或直接走前端 localStorage。

---

## 五、UI 组件设计

### 5.1 双视图架构

```
WorkflowDashboard.vue
├── 视图切换按钮 [Pipeline] [Tree]
│
├── PipelineView.vue（默认）
│   ├── WorkflowRunHeader.vue（DOSSIER 风格复用 TestReportHeader）
│   ├── JobPipelineCard.vue × N（每张卡片 = 一个 JobRun）
│   │   ├── 卡片头：Job 名称 + 状态印章 + matrix 变量标签
│   │   ├── 进度条：Steps 完成比例
│   │   └── StepMiniBadge × M（紧凑的状态图标 + 名字）
│   │
│   └── WorkflowSummaryBar（总览：Jobs 成功率 + 耗时）
│
└── TreeView.vue
    ├── 左侧树形导航
    │   ├── 📁 WorkflowRun（根节点）
    │   ├── 📂 JobRun × N（可折叠）
    │   │   └── 📄 StepRun × M（叶子节点，带状态图标）
    │   └── 🔍 搜索/过滤栏
    │
    └── 右侧详情面板
        └── StepDetailPanel.vue（复用 TestCaseFile.vue 的 Forensic 风格）
            ├── §1 DIAGNOSIS（错误分析）
            ├── §2 ERROR CHAIN（错误链路树）
            ├── §3 REMEDIATION（修复建议）
            └── §4 SUBMITTED PAYLOAD（提交快照）
```

### 5.2 组件复用关系

| 新组件 | 复用现有组件 |
|--------|------------|
| `WorkflowRunHeader` | `TestReportHeader`（扩展 props） |
| `StepDetailPanel` | `TestCaseFile` + `ErrorChainNode` |
| `JobPipelineCard` | 新建（Pipeline 风格） |
| `TreeView` | 新建（Ionic Virtual Scroll） |

### 5.3 预置模板

MVP 提供 3 个内置模板：

**模板 1：自动化测试套件**（从现有 useAutomationTests 迁移）
```
jobs:
  - id: setup-mock
    name: "生成 Mock 数据"
    steps:
      - action: encv_task (generate-mock)

  - id: test-encrypt
    name: "加密测试矩阵"
    needs: [setup-mock]
    strategy:
      type: matrix
      axes:
        plugin: [video-v4, audio-v4]
        cipher: [0, 1]
        compression: [none, zstd]
    steps:
      - action: encv_task (encrypt, plugin=${{plugin}}, cipher=${{cipher}}, compress=${{compression}})

  - id: test-decrypt
    name: "解密测试"
    needs: [test-encrypt]
    steps:
      - action: encv_task (decrypt, ...)
```

**模板 2：批量转码**
```
jobs:
  - id: transcode-all
    name: "批量转码"
    strategy:
      type: parallel
      max: 3
    steps:
      - action: encv_task (transcode, file=${{file}})
        # 文件列表来自文件选择器
```

**模板 3：自定义流水线**（空白模板）

---

## 六、实现计划（分批）

### Batch D1：核心数据模型 + 状态机 + 存储层
- `src/lib/workflow/types.ts` — 所有 TypeScript 接口
- `src/lib/workflow/stateMachine.ts` — 状态转换验证函数
- `src/composables/useWorkflowStore.ts` — localStorage CRUD

### Batch D2：DAG 编排引擎
- `src/composables/useWorkflowEngine.ts` — Scheduler + Executor
- `src/lib/workflow/scheduler.ts` — Kahn's algorithm + tick loop
- `src/lib/workflow/matrixExpander.ts` — matrix 展开
- `src/lib/workflow/conditionEvaluator.ts` — if 表达式求值

### Batch D3：UI — Pipeline 视图
- `WorkflowDashboard.vue` — 主页面
- `WorkflowRunHeader.vue` — 报告头部
- `JobPipelineCard.vue` — Job 卡片
- `StepMiniBadge.vue` — 步骤迷你徽章

### Batch D4：UI — Tree 视图 + 详情面板
- `TreeView.vue` — 树形导航
- `StepDetailPanel.vue` — 步骤详情（复用 TestCaseFile）
- 视图切换逻辑

### Batch D5：预置模板 + 集成
- 3 个内置模板定义
- 从现有 AutomationTestsDetail 迁移到新引擎
- 后端 API stub（501 占位）

---

## 七、向后兼容

- 现有 `useAutomationTests()` 保持不变（内部调用新引擎）
- `TestCaseResult` / `TestProgress` 作为 legacy 类型保留
- `AutomationTestsDetail.vue` 渐进式迁移为新 Dashboard 的一个「视图模式」
