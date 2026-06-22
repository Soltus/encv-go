/**
 * useWorkflowTaskService — 统一工作流任务编排服务
 *
 * 职责：
 * - DAG 工作流编排（分层 + Promise.all + worker 池并发）
 * - 通过 useTaskEventBridge 订阅 WS 4 件套事件，路由到 StepRun 更新
 * - 持久化到 localStorage key `encv_workflow_tasks_v1`（UnifiedRunRecord 格式）
 * - 取消运行（cancelRun → cancelTask API）
 * - 集成 useTaskTrigger（setTaskMetadata 关联 taskId ↔ runId）
 *
 * 设计原则（spec unify-workflow-task-service Task 4）：
 * - 不直接订阅 eventBus，必须通过 useTaskEventBridge
 * - 终态保护：所有事件回调中更新 StepRun 前调用 applyTerminalGuard
 * - 状态机校验：更新 status 前调用 validateTransition，非法转换记录 warning 但不抛错
 * - 持久化三处：提交阶段 + 运行阶段 + 末尾（双保险）
 *
 * 从 useWorkflowEngine 迁移的核心逻辑：
 * - runWorkflow → submitRun
 * - executeJob（矩阵展开 + worker 池）
 * - checkJobCompletion / scheduleDependentJobs
 * - submitAction（createTask API）
 * - startListening / stopListening → useTaskEventBridge
 * - cancelCurrentRun → cancelRun
 */
import { ref, computed, type Ref, type ComputedRef } from 'vue'
import { createTask, cancelTask, type EncvTask } from '@/api/encv'
import { useTaskStore } from '@/stores/taskStore'
import { type TriggeredBy } from '@/composables/useTaskTrigger'
import { analyzeError } from '@/composables/useErrorAnalyzer'
import { useTaskEventBridge } from '@/composables/useTaskEventBridge'
import { applyTerminalGuard, validateTransition } from '@/lib/workflow/state-machine'
import {
  isTerminalStep,
  type WorkflowRun,
  type JobRun,
  type StepRun,
  type StepStatus,
  type WorkflowDefinition,
  type JobDefinition,
  type StepDefinition,
  type ActionSpec,
  type EncvTaskActionSpec,
  type MatrixBinding,
  type UnifiedRunRecord,
} from '@/lib/workflow/types'
import { computeJobConclusion, inferWorkflowStatus } from '@/lib/workflow/stateMachine'
import { resolveExecutionOrder, getNextReadyJobs } from '@/lib/workflow/scheduler'
import { expandMatrix, isMatrixStrategy } from '@/lib/workflow/matrixExpander'
import { evaluateCondition } from '@/lib/workflow/conditionEvaluator'

// ==================== 接口定义 ====================

export interface WorkflowTaskServiceOptions {
  /** 持久化 key（默认 encv_workflow_tasks_v1） */
  storageKey?: string
  /** 最大持久化记录数（默认 50） */
  maxRuns?: number
  /** worker 池并发数（默认 5） */
  concurrency?: number
}

export interface SubmitRunParams {
  workflow: WorkflowDefinition
  /** 触发器类型（默认 user） */
  triggeredBy?: TriggeredBy
  /** 是否立即持久化（默认 true） */
  persist?: boolean
}

export interface WorkflowTaskService {
  // 当前运行
  currentRun: Ref<WorkflowRun | null>
  isRunning: ComputedRef<boolean>
  totalSteps: ComputedRef<number>
  completedSteps: ComputedRef<number>
  successSteps: ComputedRef<number>
  failedSteps: ComputedRef<number>

  // 历史运行
  runs: Ref<UnifiedRunRecord[]>

  // 提交运行
  submitRun(params: SubmitRunParams): Promise<WorkflowRun>
  // 取消运行
  cancelRun(runId: string): Promise<void>
  // 获取运行
  getRun(runId: string): WorkflowRun | null
  // 列出历史运行
  listRuns(): UnifiedRunRecord[]
  // 清空历史运行
  clearRuns(): void
  // 订阅运行更新
  subscribeRun(runId: string, callback: (run: WorkflowRun) => void): () => void
}

// ==================== 辅助函数 ====================

/** 生成简易唯一 ID */
function genId(): string {
  return 'run-' + Date.now().toString(36) + '-' + Math.random().toString(36).slice(2, 8)
}

/** 从 localStorage 加载历史运行记录 */
function loadRunsFromStorage(key: string): UnifiedRunRecord[] {
  try {
    const raw = localStorage.getItem(key)
    return raw ? (JSON.parse(raw) as UnifiedRunRecord[]) : []
  } catch {
    return []
  }
}

/** 保存历史运行记录到 localStorage */
function saveRunsToStorage(key: string, data: UnifiedRunRecord[]): void {
  try {
    localStorage.setItem(key, JSON.stringify(data))
  } catch (e) {
    console.warn('[useWorkflowTaskService] persist failed:', e)
  }
}

// ==================== 🆕 v4 2026-06-18 M5：模块级单例 ====================
/**
 * 把 useWorkflowTaskService 改为单例。
 *
 * 历史问题：每个组件调用 `useWorkflowTaskService()` 都会重新创建一份 `runs` ref +
 *   重新从 localStorage 加载一遍历史记录 + 重新注册一份 WS 事件回调（虽然
 *   useTaskEventBridge 内部会去重，但服务本身的 in-memory runs ref 是各组件独立的）。
 *
 * 修复：模块级 cached instance，首次调用时创建 options 化的实例，后续复用。
 *   - 优点：Tasks.vue / PluginTestsDetail.vue / WorkflowDashboard.vue 共享同一份 runs 数据
 *   - 优点：取消订阅时不会影响其他组件
 *   - 缺点：options（maxRuns / storageKey）只在首次调用时生效（这是合理 trade-off）
 *
 * 升级指南：测试可调用 `__resetServiceForTests()` 重置单例。
 */
let _cachedInstance: WorkflowTaskService | null = null
let _cachedOptions: WorkflowTaskServiceOptions | null = null

export function __resetServiceForTests(): void {
  _cachedInstance = null
  _cachedOptions = null
}

// ==================== 主 composable ====================

export function useWorkflowTaskService(
  options: WorkflowTaskServiceOptions = {},
): WorkflowTaskService {
  // 单例模式：首次调用创建并缓存
  if (_cachedInstance && _cachedOptions) {
    return _cachedInstance
  }
  _cachedOptions = options
  _cachedInstance = createService(options)
  return _cachedInstance
}

function createService(
  options: WorkflowTaskServiceOptions,
): WorkflowTaskService {
  const storageKey = options.storageKey ?? 'encv_workflow_tasks_v1'
  const maxRuns = options.maxRuns ?? 50
  const concurrency = options.concurrency ?? 5

  // ==================== 响应式状态 ====================

  const currentRun = ref<WorkflowRun | null>(null)
  const currentDef = ref<WorkflowDefinition | null>(null)
  const runs = ref<UnifiedRunRecord[]>(loadRunsFromStorage(storageKey))

  const isRunning = computed(() => currentRun.value?.status === 'running')

  const totalSteps = computed(() => {
    if (!currentRun.value) return 0
    return currentRun.value.jobs.reduce((sum, j) => sum + j.steps.length, 0)
  })

  const completedSteps = computed(() => {
    if (!currentRun.value) return 0
    return currentRun.value.jobs.reduce(
      (sum, j) => sum + j.steps.filter((s) => isTerminalStep(s.status)).length,
      0,
    )
  })

  const successSteps = computed(() => {
    if (!currentRun.value) return 0
    return currentRun.value.jobs.reduce(
      (sum, j) => sum + j.steps.filter((s) => s.status === 'success').length,
      0,
    )
  })

  const failedSteps = computed(() => {
    if (!currentRun.value) return 0
    return currentRun.value.jobs.reduce(
      (sum, j) => sum + j.steps.filter((s) => s.status === 'failure' || s.status === 'timed_out').length,
      0,
    )
  })

  // ==================== 订阅者管理 ====================

  const subscribers = new Map<string, Set<(run: WorkflowRun) => void>>()

  function subscribeRun(runId: string, callback: (run: WorkflowRun) => void): () => void {
    if (!subscribers.has(runId)) subscribers.set(runId, new Set())
    subscribers.get(runId)!.add(callback)
    return () => {
      subscribers.get(runId)?.delete(callback)
    }
  }

  function notifySubscribers(): void {
    if (!currentRun.value) return
    const subs = subscribers.get(currentRun.value.id)
    if (!subs) return
    for (const cb of subs) {
      try {
        cb(currentRun.value)
      } catch (e) {
        console.error('[useWorkflowTaskService] subscriber callback error:', e)
      }
    }
  }

  // ==================== 持久化 ====================

  /** 将 UnifiedRunRecord[] 排序+裁剪后写入 localStorage */
  function persistRuns(): void {
    const sorted = [...runs.value]
      .sort((a, b) => b.startedAt.localeCompare(a.startedAt))
      .slice(0, maxRuns)
    runs.value = sorted
    saveRunsToStorage(storageKey, sorted)
  }

  /** 从 WorkflowRun 构造 UnifiedRunRecord（含 workflowRun 深拷贝快照） */
  function createRecord(run: WorkflowRun): UnifiedRunRecord {
    const steps = run.jobs.flatMap((j) => j.steps)
    return {
      id: run.id,
      startedAt: run.startedAt ?? run.createdAt,
      completedAt: run.completedAt,
      totalCases: steps.length,
      passed: steps.filter((s) => s.status === 'success').length,
      failed: steps.filter((s) => s.status === 'failure' || s.status === 'timed_out').length,
      skipped: steps.filter((s) => s.status === 'skipped').length,
      results: steps.map((s) => ({
        caseId: s.id,
        status: s.status === 'success' ? 'success' as const
          : s.status === 'skipped' ? 'skipped' as const
          : 'failure' as const,
        error: s.error,
        duration: s.durationMs != null ? `${s.durationMs}ms` : undefined,
      })),
      workflowRun: JSON.parse(JSON.stringify(run)) as WorkflowRun,
    }
  }

  /** 持久化当前运行（提交阶段 + 运行阶段 + 末尾三处调用） */
  function persistCurrentRun(): void {
    if (!currentRun.value) return
    const record = createRecord(currentRun.value)
    const idx = runs.value.findIndex((r) => r.id === currentRun.value!.id)
    if (idx >= 0) {
      runs.value = runs.value.map((r) => (r.id === currentRun.value!.id ? record : r))
    } else {
      runs.value = [record, ...runs.value]
    }
    persistRuns()
  }

  // ==================== WS 事件回调（4 件套） ====================

  /** 按 taskId 查找当前运行中的 StepRun */
  function findStepByTaskId(taskId: string): { step: StepRun; job: JobRun } | null {
    if (!currentRun.value) return null
    for (const job of currentRun.value.jobs) {
      for (const step of job.steps) {
        if (step.taskId === taskId) return { step, job }
      }
    }
    return null
  }

  /** task:created — 后端已创建任务，step 从 submitted/pending 升级到 queued */
  function onTaskCreated(data: { id: string; type: string; sourcePath: string }): void {
    if (!currentRun.value) return
    const found = findStepByTaskId(data.id)
    if (!found) return
    const { step } = found
    if (isTerminalStep(step.status)) return
    // 状态机校验：submitted/pending → queued
    if (validateTransition(step.status, 'queued')) {
      step.status = 'queued'
    }
    notifySubscribers()
    persistCurrentRun()
  }

  /** task:update — 状态机升级（pending → queued → running → cancelling → cancelled） */
  function onTaskUpdate(data: { id: string; type: string; status: string; progress: number }): void {
    if (!currentRun.value) return
    const found = findStepByTaskId(data.id)
    if (!found) return
    const { step } = found

    // 终态保护：已终态的 step 仅刷新 progress，不改 status
    const guarded = applyTerminalGuard(step, { progress: data.progress })
    if (typeof guarded.progress === 'number') step.progress = guarded.progress

    if (isTerminalStep(step.status)) {
      notifySubscribers()
      persistCurrentRun()
      return
    }

    // 状态升级映射
    let targetStatus: StepStatus | null = null
    if (data.status === 'running' && (step.status === 'pending' || step.status === 'queued' || step.status === 'submitted')) {
      targetStatus = 'running'
    } else if (data.status === 'cancelling' && step.status === 'running') {
      targetStatus = 'cancelling'
    } else if (data.status === 'cancelled' && !isTerminalStep(step.status)) {
      targetStatus = 'cancelled'
    }

    if (targetStatus) {
      if (validateTransition(step.status, targetStatus)) {
        step.status = targetStatus
        if (targetStatus === 'running' && !step.startedAt) {
          step.startedAt = new Date().toISOString()
        }
        if (targetStatus === 'cancelled') {
          step.completedAt = new Date().toISOString()
          if (step.startedAt) {
            step.durationMs = Date.now() - new Date(step.startedAt).getTime()
          }
        }
      } else {
        // 非法转换：记录 warning 但不抛错（兼容后端可能跳发事件）
        console.warn(
          `[useWorkflowTaskService] invalid transition: ${step.status} → ${targetStatus}`,
        )
      }
    }

    notifySubscribers()
    persistCurrentRun()
  }

  /** task:progress — 更新 progress / phase / speed / eta 元数据 */
  function onTaskProgress(data: {
    id: string
    progress: number
    phase: string
    speed: string
    eta: string
  }): void {
    if (!currentRun.value) return
    const found = findStepByTaskId(data.id)
    if (!found) return
    const { step } = found
    // 终态保护：只更新元数据，不改 status
    const guarded = applyTerminalGuard(step, {
      progress: data.progress,
      phase: data.phase,
      speed: data.speed,
      eta: data.eta,
    })
    if (typeof guarded.progress === 'number') step.progress = guarded.progress
    if (guarded.phase) step.phase = guarded.phase
    if (guarded.speed) step.speed = guarded.speed
    if (guarded.eta) step.eta = guarded.eta
    notifySubscribers()
    persistCurrentRun()
  }

  /** task:completed — 标记终态 + 触发 scheduleDependentJobs */
  function onTaskCompleted(data: { id: string; error?: string }): void {
    if (!currentRun.value) return
    let found = false
    for (const job of currentRun.value.jobs) {
      for (const step of job.steps) {
        if (step.taskId !== data.id) continue
        // 已终态的 step 跳过（防重复）
        if (isTerminalStep(step.status)) {
          found = true
          break
        }
        found = true
        const newStatus: StepStatus = data.error ? 'failure' : 'success'
        if (validateTransition(step.status, newStatus)) {
          step.status = newStatus
        } else {
          // 兼容后端跳发事件：强制更新（但记录 warning）
          console.warn(
            `[useWorkflowTaskService] invalid transition on complete: ${step.status} → ${newStatus}`,
          )
          step.status = newStatus
        }
        step.completedAt = new Date().toISOString()
        step.durationMs = step.startedAt
          ? Date.now() - new Date(step.startedAt).getTime()
          : 0
        if (data.error) {
          step.error = data.error
          step.errorAnalysis = analyzeError(data.error, { phase: 'backend' })
        }
        checkJobCompletion(job)
        break
      }
      if (found) break
    }
    if (found) {
      checkWorkflowCompletion()
      notifySubscribers()
      persistCurrentRun()
    }
  }

  // 注册 4 件套事件回调（通过 useTaskEventBridge，不直接订阅 eventBus）
  useTaskEventBridge({
    onCreate: onTaskCreated,
    onUpdate: onTaskUpdate,
    onProgress: onTaskProgress,
    onComplete: onTaskCompleted,
  })

  // ==================== 执行核心 ====================

  /**
   * 提交一次工作流运行
   *
   * 流程：
   * 1. 创建 WorkflowRun → 设置 currentRun
   * 2. 持久化（提交阶段）
   * 3. DAG 分层 → 逐层 Promise.all → 每层内 worker 池并发提交
   * 4. 持久化（末尾）
   */
  async function submitRun(params: SubmitRunParams): Promise<WorkflowRun> {
    const { workflow, triggeredBy = 'user', persist = true } = params
    if (isRunning.value) throw new Error('A workflow is already running')

    const now = new Date().toISOString()
    const run: WorkflowRun = {
      id: genId(),
      workflowDefId: workflow.id,
      status: 'running',
      triggeredBy,
      createdAt: now,
      startedAt: now,
      jobs: [],
    }
    currentRun.value = run
    currentDef.value = workflow

    // 提交阶段持久化
    if (persist) persistCurrentRun()

    try {
      // DAG 分层
      const layers = resolveExecutionOrder(workflow.jobs)
      if (layers.length === 0) {
        run.status = 'success'
        run.completedAt = new Date().toISOString()
        run.durationMs = Date.now() - new Date(run.startedAt!).getTime()
        if (persist) persistCurrentRun()
        notifySubscribers()
        return run
      }

      // 逐层执行（layer 内 Promise.all 并行提交）
      for (const layerJobIds of layers) {
        const jobPromises = layerJobIds.map((jobId) => {
          const jobDef = workflow.jobs.find((j) => j.id === jobId)!
          // jobRun 创建后立即 push 到 run.jobs（UI 立刻可见）
          const jobRun: JobRun = {
            id: genId(),
            jobDefId: jobDef.id,
            status: 'running',
            startedAt: new Date().toISOString(),
            steps: [],
          }
          run.jobs.push(jobRun)
          return executeJob(jobDef, workflow.env ?? {}, jobRun, run.id).then(() => jobRun)
        })
        await Promise.all(jobPromises)
      }

      // 空 workflow（有 job 但无 step）→ 直接标记完成
      if (run.jobs.every((j) => j.steps.length === 0)) {
        run.status = 'success'
        run.completedAt = new Date().toISOString()
        run.durationMs = Date.now() - new Date(run.startedAt!).getTime()
      }

      // 末尾持久化
      if (persist) persistCurrentRun()
      notifySubscribers()
      return run
    } catch (e) {
      run.status = 'failure'
      run.completedAt = new Date().toISOString()
      if (persist) persistCurrentRun()
      notifySubscribers()
      throw e
    }
  }

  /**
   * 执行单个 Job：展开 Steps 并按 strategy 限流提交
   *
   * - matrix 策略：笛卡尔积展开
   * - parallel 策略：按 strategy.max 并发
   * - sequential 策略：逐个执行
   * - 默认：按 concurrency 选项并发
   */
  async function executeJob(
    jobDef: JobDefinition,
    env: Record<string, string>,
    jobRun: JobRun,
    runId: string,
  ): Promise<JobRun> {
    // 构建 continueOnError 映射
    const continueOnErrorMap = new Map<string, boolean>()
    for (const step of jobDef.steps) {
      continueOnErrorMap.set(step.id, step.continueOnError ?? false)
    }

    // 展开 matrix 或直接使用步骤列表
    let stepExecutions: Array<{ stepDef: StepDefinition; binding?: MatrixBinding }> = []
    if (isMatrixStrategy(jobDef.strategy)) {
      const bindings = expandMatrix(jobDef.strategy)
      for (const binding of bindings) {
        for (const step of jobDef.steps) {
          stepExecutions.push({ stepDef: step, binding })
        }
      }
    } else {
      for (const step of jobDef.steps) {
        stepExecutions.push({ stepDef: step })
      }
    }

    // 评估条件 + 构造 stepRun（同步 push 到 jobRun.steps，UI 立刻可见）
    type ExecutableStep = { stepDef: StepDefinition; binding?: MatrixBinding; stepRun: StepRun; clientTaskId: string }
    const executableSteps: ExecutableStep[] = []
    let prevStatus: StepStatus | undefined

    // 🆕 2026-06-22：直接用顶层 import 的 useTaskStore 来预占位 placeholder（同步 push 1000+ task 到 store）
    //   场景：自动化测试一次提交 1000+ step → 用户期望 UI 立即显示 1 个 group 1000+ task
    //   修法：submitRun 阶段同步 push 1000+ placeholder EncvTask 到 store（client 生成 ID，runId 已绑）
    //   → 调 createTask API 时把 client ID 传给后端 → 后端用 client ID 覆盖默认 UUID
    //   → WS 推 task:created 时 task.id == placeholder.id → 找到 placeholder patch 更新（不是 append）
    //   → submitRun 同步阶段 store 里就有 1000+ placeholder，UI 立即显示 1 个 group 1000+ task
    //   → 没有"1000+ task 慢慢累加"的视觉延迟
    // 注意：必须用顶层 import 的 useTaskStore（不能用 await import，那是 microtask 异步）
    const taskStore = useTaskStore()
    const placeholderIdByStepIdx = new Map<number, string>()

    for (let stepIdx = 0; stepIdx < stepExecutions.length; stepIdx++) {
      const exec = stepExecutions[stepIdx]!
      const { stepDef, binding } = exec
      // 评估 if 条件
      if (stepDef.if) {
        const shouldExecute = evaluateCondition(stepDef.if, {
          previousStepStatus: prevStatus,
          vars: binding ? { ...env, ...binding } : env,
        })
        if (!shouldExecute) {
          jobRun.steps.push({
            id: genId(),
            stepDefId: stepDef.id,
            status: 'skipped',
            matrixVars: binding,
          })
          prevStatus = 'skipped'
          continue
        }
      }
      // 创建 StepRun
      const stepRun: StepRun = {
        id: genId(),
        stepDefId: stepDef.id,
        status: 'pending',
        startedAt: new Date().toISOString(),
        matrixVars: binding,
      }
      jobRun.steps.push(stepRun)
      // 🆕 2026-06-22：同步预占位 placeholder EncvTask 到 store
      //   用 client 生成的 UUID 作为 taskId，确保后续 createTask 返回的 task.id 跟 placeholder.id 一致
      const clientTaskId = `client-${(crypto as any)?.randomUUID?.() ?? `${Date.now()}-${Math.random().toString(36).slice(2)}`}`
      placeholderIdByStepIdx.set(executableSteps.length, clientTaskId)
      // 推 placeholder 到 store（status='queued'，runId 已绑，pluginName/type/sourcePath 从 action 推）
      // 注意：必须从 stepDef.action 推 pluginName（matrix binding 还没 apply）
      const actionForPlaceholder = stepDef.action
      const placeholderTask: EncvTask = {
        id: clientTaskId,
        type: actionForPlaceholder.type === 'encv_task'
          ? (actionForPlaceholder as EncvTaskActionSpec).taskType
          : 'encrypt',
        sourcePath: actionForPlaceholder.type === 'encv_task'
          ? ((actionForPlaceholder as EncvTaskActionSpec).params.sourcePath ?? '')
          : '',
        status: 'queued',
        progress: 0,
        runId: runId,
        triggeredBy: currentRun.value?.triggeredBy ?? 'user',
        pluginName: actionForPlaceholder.type === 'encv_task'
          ? (actionForPlaceholder as EncvTaskActionSpec).pluginName
          : undefined,
        createdAt: new Date().toISOString(),
      }
      taskStore.appendTask(placeholderTask)
      executableSteps.push({ stepDef, binding, stepRun, clientTaskId })
    }

    // 并发限流：jobDef.strategy.max 优先，否则用 concurrency 选项
    const max =
      jobDef.strategy && 'max' in jobDef.strategy ? jobDef.strategy.max : concurrency

    // 执行单个 step（提交 + 状态更新）
    const taskTriggeredBy: TriggeredBy = currentRun.value?.triggeredBy ?? 'user'
    const runOneStep = async (ex: ExecutableStep): Promise<void> => {
      const { stepDef, binding, stepRun, clientTaskId } = ex
      try {
        const action = applyEnvToAction(stepDef.action, env, binding)
        // 🆕 2026-06-22：传 clientTaskId 给 createTask API → 后端用 client ID 覆盖默认 UUID
        //   → WS 推 task:created 时 task.id == placeholder.id → 找到 placeholder patch 更新（不 append）
        const task = await submitAction(action, clientTaskId, runId, taskTriggeredBy)
        stepRun.taskId = task.id
        stepRun.status = 'running' // 已提交，等 WS 回调
      } catch (e) {
        // 提交失败 → 直接标记 failure
        // 注意：placeholder 还在 store 里（status='queued'），WS 不会来更新它
        // 用户应该能 retry → placeholder 会被 patchTaskById 找到更新
        stepRun.status = 'failure'
        stepRun.error = e instanceof Error ? e.message : String(e)
        stepRun.errorAnalysis = analyzeError(stepRun.error, { phase: 'submission' })
        stepRun.completedAt = new Date().toISOString()
        stepRun.durationMs = Date.now() - new Date(stepRun.startedAt!).getTime()
      }
    }

    // worker 池：N 个 worker 共享 cursor 轮转拉取
    let cursor = 0
    const worker = async (): Promise<void> => {
      while (cursor < executableSteps.length) {
        const idx = cursor++
        const ex = executableSteps[idx]
        if (ex) await runOneStep(ex)
      }
    }
    const workerCount = Math.min(max, executableSteps.length)
    await Promise.all(Array.from({ length: workerCount }, () => worker()))

    // 检查 Job 是否已全部完成
    const allDone = jobRun.steps.every((s) => isTerminalStep(s.status))
    if (allDone) {
      jobRun.conclusion = computeJobConclusion(jobRun.steps, continueOnErrorMap)
      jobRun.status = jobRun.conclusion === 'success' ? 'success' : 'failure'
      jobRun.completedAt = new Date().toISOString()
      jobRun.durationMs = jobRun.startedAt
        ? Date.now() - new Date(jobRun.startedAt).getTime()
        : 0
    }

    return jobRun
  }

  /** 将 ActionSpec 提交为 EncvTask（调用 createTask API） */
  async function submitAction(
    action: ActionSpec,
    clientTaskId?: string, // 🆕 2026-06-22：客户端预占位 ID
    runId?: string,
    triggeredBy?: TriggeredBy,
  ): Promise<EncvTask> {
    if (action.type !== 'encv_task') {
      throw new Error(`Unsupported action type: ${action.type}`)
    }
    const spec = action as EncvTaskActionSpec
    return createTask(
      spec.taskType,
      spec.params.sourcePath ?? '',
      spec.params.targetPath,
      spec.params.password ?? '',
      spec.params.version,
      spec.pluginName,
      spec.params.extraFields ?? {},
      spec.params.secondaryPassword,
      spec.params.cipherMode,
      spec.params.compressionMode,
      runId,
      triggeredBy,
      clientTaskId, // 🆕 2026-06-22
    )
  }

  /** 将 env 变量和 matrix 绑定注入到 ActionSpec 中（${{ varName }} 模板替换） */
  function applyEnvToAction(
    action: ActionSpec,
    env: Record<string, string>,
    binding?: Record<string, string>,
  ): ActionSpec {
    if (action.type !== 'encv_task') return action
    const mergedVars = { ...env, ...binding }
    const params = { ...action.params }
    for (const [key, val] of Object.entries(params)) {
      if (typeof val === 'string' && val.includes('${{')) {
        ;(params as Record<string, unknown>)[key] = val.replace(
          /\$\{\{\s*(\w+)\s*\}\}/g,
          (_m, v: string) => mergedVars[v] ?? val,
        )
      }
    }
    return { ...action, params }
  }

  // ==================== Job 管理 ====================

  /** 检查 Job 是否全部完成，更新 conclusion + 派发下游 Jobs */
  function checkJobCompletion(job: JobRun): void {
    const allTerminal = job.steps.every((s) => isTerminalStep(s.status))
    if (!allTerminal) return

    // 从 currentDef 获取 continueOnError 映射
    const coMap = new Map<string, boolean>()
    if (currentDef.value) {
      const jobDef = currentDef.value.jobs.find((j) => j.id === job.jobDefId)
      if (jobDef) {
        for (const step of jobDef.steps) {
          coMap.set(step.id, step.continueOnError ?? false)
        }
      }
    }

    job.conclusion = computeJobConclusion(job.steps, coMap)
    job.status = job.conclusion === 'success' ? 'success' : 'failure'
    job.completedAt = new Date().toISOString()
    job.durationMs = job.startedAt
      ? Date.now() - new Date(job.startedAt).getTime()
      : 0

    // 派发下一批依赖此 job 的 Jobs
    scheduleDependentJobs(job.jobDefId)
  }

  /** 当一个 Job 完成后，检查并启动依赖它的下游 Jobs */
  function scheduleDependentJobs(_completedJobDefId: string): void {
    if (!currentRun.value || currentRun.value.status !== 'running') return
    if (!currentDef.value) return

    const completedJobIds = new Set(
      currentRun.value.jobs
        .filter((j) => isTerminalStep(j.status))
        .map((j) => j.jobDefId),
    )

    const readyIds = getNextReadyJobs(currentDef.value.jobs, completedJobIds)
    for (const readyId of readyIds) {
      // 已有 Run 的 Job 跳过
      const existing = currentRun.value.jobs.find((j) => j.jobDefId === readyId)
      if (existing) continue

      const jobDef = currentDef.value.jobs.find((j) => j.id === readyId)!
      const jobRun: JobRun = {
        id: genId(),
        jobDefId: jobDef.id,
        status: 'running',
        startedAt: new Date().toISOString(),
        steps: [],
      }
      currentRun.value.jobs.push(jobRun)
      executeJob(jobDef, currentDef.value.env ?? {}, jobRun, currentRun.value.id).then(() => {
        persistCurrentRun()
      })
    }
  }

  /** 检查整个 Workflow 是否全部完成 */
  function checkWorkflowCompletion(): void {
    if (!currentRun.value) return
    const status = inferWorkflowStatus(currentRun.value.jobs)
    if (status !== 'running') {
      currentRun.value.status = status
      currentRun.value.completedAt = new Date().toISOString()
      currentRun.value.durationMs = currentRun.value.startedAt
        ? Date.now() - new Date(currentRun.value.startedAt!).getTime()
        : 0
    }
  }

  // ==================== 取消 ====================

  /**
   * 取消运行
   *
   * 流程：
   * 1. 找到所有非终态且有 taskId 的 step → 标记 cancelling
   * 2. 调用 cancelTask API
   * 3. 标记所有非终态 step 为 cancelled
   * 4. 更新 run.status = 'cancelled'
   */
  async function cancelRun(runId: string): Promise<void> {
    if (!currentRun.value || currentRun.value.id !== runId) return
    if (currentRun.value.status !== 'running') return

    // 标记 cancelling + 调用 cancel API
    const cancelPromises: Promise<void>[] = []
    for (const job of currentRun.value.jobs) {
      for (const step of job.steps) {
        if (!isTerminalStep(step.status) && step.taskId) {
          if (validateTransition(step.status, 'cancelling')) {
            step.status = 'cancelling'
          }
          cancelPromises.push(
            cancelTask(step.taskId).catch((e) => {
              console.warn(
                `[useWorkflowTaskService] cancelTask failed for ${step.taskId}:`,
                e,
              )
            }),
          )
        }
      }
    }
    await Promise.all(cancelPromises)

    // 标记所有非终态 step 为 cancelled
    for (const job of currentRun.value.jobs) {
      for (const step of job.steps) {
        if (!isTerminalStep(step.status)) {
          step.status = 'cancelled'
          step.completedAt = new Date().toISOString()
        }
      }
      if (job.status === 'running' || job.status === 'queued' || job.status === 'pending') {
        job.conclusion = 'cancelled'
        job.status = 'cancelled'
        job.completedAt = new Date().toISOString()
      }
    }

    currentRun.value.status = 'cancelled'
    currentRun.value.completedAt = new Date().toISOString()

    notifySubscribers()
    persistCurrentRun()
  }

  // ==================== 查询 ====================

  /** 获取运行（优先 currentRun，其次从历史记录的 workflowRun 快照中查找） */
  function getRun(runId: string): WorkflowRun | null {
    if (currentRun.value?.id === runId) return currentRun.value
    const record = runs.value.find((r) => r.id === runId)
    return record?.workflowRun ?? null
  }

  /** 列出历史运行（返回副本） */
  function listRuns(): UnifiedRunRecord[] {
    return [...runs.value]
  }

  /** 清空历史运行 */
  function clearRuns(): void {
    runs.value = []
    saveRunsToStorage(storageKey, [])
  }

  // ==================== 返回 ====================

  return {
    currentRun,
    isRunning,
    totalSteps,
    completedSteps,
    successSteps,
    failedSteps,
    runs,
    submitRun,
    cancelRun,
    getRun,
    listRuns,
    clearRuns,
    subscribeRun,
  }
}
