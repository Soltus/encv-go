/**
 * 自动化测试 composable
 *
 * 提供：
 * - loadPlugins(): 从后端拉取 plugin 列表
 * - generateTestCases(): 动态笛卡尔积生成测试用例
 * - runTests(): 顺序提交任务到后端
 *
 * 核心设计：零硬编码 cipher mode / compression / version
 * - version 从 plugin.taskOptions.supportedVersions 派生
 * - ECv4 (推荐) 才有 cipherMode (0/1) × compressionMode (none/zstd) 的笛卡尔积
 * - ECv3 (deprecated) 不带 cipher/compression
 * - 默认源文件是 01-plain-media/video/sample.mp4（跨 plugin 通用）
 *
 * 真机安全：所有 source 走 withSafetyBoundary({ forceAutomation: true })
 * 强制改写到 /storage/emulated/0/encv-automation/* 命名空间。
 *
 * 触发者标签：通过 recordTriggeredBy 登记 'automation'，Tasks.vue badge 自动显示。
 */
import { ref } from 'vue'
import {
  createTask,
  fetchPlugins,
  type PluginMeta,
  type TaskType,
  type EncvTask,
} from '@/api/encv'
import { usePathResolver } from '@/composables/usePathResolver'
import { recordTriggeredBy, setTaskMetadata, type TriggeredBy } from '@/composables/useTaskTrigger'
import { analyzeError, type ErrorAnalysis } from '@/composables/useErrorAnalyzer'
import { eventBus } from '@/composables/useEventBus'
import { isDeprecatedVersion, isRecommendedVersion, formatContainerVersion } from '@/constants/containerVersion'

export type { TriggeredBy }

export interface TestCaseSpec {
  id: string
  taskType: TaskType
  pluginName: string
  sourcePath: string
  version: number
  cipherMode?: number
  compressionMode?: 'none' | 'zstd'
  expectedBehavior: 'success' | 'might-fail'
}

export interface TestCaseResult {
  spec: TestCaseSpec
  status: 'pending' | 'running' | 'passed' | 'failed' | 'skipped'
  taskId?: string
  error?: string
  durationMs?: number
  /** 错误分析（仅 status === 'failed' 时有值） */
  errorAnalysis?: ErrorAnalysis
  /** 提交时的快照（sourcePath, version, cipher, compression） */
  submittedSourcePath?: string
  submittedAt?: string
}

export interface TestProgress {
  total: number
  completed: number
  /** 已完成且通过 */
  passed: number
  /** 提交失败或执行失败 */
  failed: number
  /** 跳过用例数（暂未启用，未来给 might-fail + 已知不支持的版本使用） */
  skipped: number
  /** 已提交、等待 WS 回调的 pending 数量 */
  pending: number
}

export interface GenerateTestCaseOptions {
  sourceFile: string
  includeDeprecated?: boolean
}

/**
 * 自动化测试默认源文件。
 *
 * 真实运行时由后端 /api/mock/generate 生成（用户主动按 UI 按钮 / 直接 curl），
 * 写到 mount registry 解析后的 /d/automation mount 根目录下。
 *
 * 2026-06-15 multi-mount 重构（spec Phase B4）：
 *   - 旧形式（硬编码绝对路径）：/storage/emulated/0/encv-automation/01-plain-media/video/sample.mp4
 *   - 新形式（mount 虚拟路径）：/d/automation/01-plain-media/video/sample.mp4
 *   - 真机：后端解析为 /data/user/<uid>/com.encvgo.app/files/encv-automation/01-plain-media/video/sample.mp4
 *   - dev 沙箱：后端解析为 $TMPDIR/encv-appdata/encv-automation/01-plain-media/video/sample.mp4
 *   - 不再依赖 withSafetyBoundary 客户端改写（mount 系统天然做命名空间隔离）
 *
 * 🆕 2026-06-15 声明式：不要再用 .split('/').slice(N) 派生 mockRoot
 *   - 正确做法：见 src/lib/mockConstants.ts 的 MOCK_GENERATE_ROOT
 *   - 此常量仅用于 source 路径默认 + 单元测试
 */
export const DEFAULT_AUTOMATION_SOURCE = '/d/automation/01-plain-media/video/sample.mp4'

export function useAutomationTests() {
  const { withSafetyBoundary } = usePathResolver()
  const plugins = ref<PluginMeta[]>([])
  const isLoadingPlugins = ref(false)
  const isRunning = ref(false)
  const progress = ref<TestProgress>({ total: 0, completed: 0, passed: 0, failed: 0, skipped: 0, pending: 0 })
  const results = ref<TestCaseResult[]>([])
  const lastError = ref<string | null>(null)
  const testCases = ref<TestCaseSpec[]>([])

  // WS 回调：task:completed — 将 pending 结果更新为实际状态
  // 🆕 2026-06-10 修复 #4：实时持久化 — 每收到一个 task:completed 就写一次 localStorage
  //   历史 bug：persistCurrentRun 只在 runTests 末尾调一次 → 200 个 case 跑 5 分钟期间
  //     用户刷新 / 关 App 全部丢失
  //   修复：每收到 task:completed 就 persistCurrentRun()（防 200 case 期间崩溃）
  function onTaskCompleted(data: { id: string; error?: string }) {
    const idx = results.value.findIndex((r) => r.taskId === data.id && r.status === 'pending')
    if (idx === -1) return

    const result = results.value[idx]
    if (data.error) {
      result.status = 'failed'
      result.error = data.error
      result.errorAnalysis = analyzeError(data.error, { phase: 'backend' })
      result.durationMs = (result.submittedAt ? Date.now() - new Date(result.submittedAt).getTime() : 0)
      progress.value.failed++
    } else {
      result.status = 'passed'
      result.durationMs = (result.submittedAt ? Date.now() - new Date(result.submittedAt).getTime() : 0)
      progress.value.passed++
    }
    progress.value.pending--

    // 🆕 实时持久化（debounce 由 localStorage 写入抖动处理）
    persistCurrentRun()
  }

  // WS 回调：task:update — 更新进度信息（可选）
  function onTaskUpdate(_data: { id: string; type: string; status: string; progress: number }) {
    // 暂不处理，未来可用来显示实时进度百分比
  }

  /** 开始监听 WS 事件（调用方在 onMounted 中调用） */
  function startListening() {
    eventBus.on('task:completed', onTaskCompleted)
    eventBus.on('task:update', onTaskUpdate)
  }

  /** 停止监听（调用方在 onUnmounted 中调用） */
  function stopListening() {
    eventBus.off('task:completed', onTaskCompleted)
    eventBus.off('task:update', onTaskUpdate)
  }

  // ==================== 本地持久化（修 #2：测试结果保存到本地方便分析） ====================
  // 🆕 2026-06-10 修复：测试结果原只存内存，刷新页面 / 关 App 丢失
  // 修复：localStorage 持久化最近 50 次 run + 当前 run，AI agent 也能用

  const RESULTS_STORAGE_KEY = 'encv_automation_results_v1'
  const MAX_PERSISTED_RUNS = 50

  interface PersistedRun {
    id: string
    startedAt: string
    completedAt?: string
    totalCases: number
    passed: number
    failed: number
    skipped: number
    results: TestCaseResult[]
  }

  function loadPersistedRuns(): PersistedRun[] {
    try {
      const raw = localStorage.getItem(RESULTS_STORAGE_KEY)
      if (!raw) return []
      const parsed = JSON.parse(raw) as PersistedRun[]
      return Array.isArray(parsed) ? parsed : []
    } catch {
      return []
    }
  }

  function savePersistedRuns(runs: PersistedRun[]): void {
    try {
      // 按 startedAt 倒序裁剪到 MAX_PERSISTED_RUNS
      const trimmed = [...runs]
        .sort((a, b) => b.startedAt.localeCompare(a.startedAt))
        .slice(0, MAX_PERSISTED_RUNS)
      localStorage.setItem(RESULTS_STORAGE_KEY, JSON.stringify(trimmed))
    } catch (e) {
      console.debug('[useAutomationTests] localStorage save failed:', e)
    }
  }

  /** 把当前 results 快照成 PersistedRun 并写入 localStorage */
  function persistCurrentRun(): void {
    if (results.value.length === 0) return
    const startedAt = results.value
      .map((r) => r.submittedAt ?? '')
      .filter(Boolean)
      .sort()[0] ?? new Date().toISOString()
    const completedAt = new Date().toISOString()
    const passed = results.value.filter((r) => r.status === 'passed').length
    const failed = results.value.filter((r) => r.status === 'failed').length
    const skipped = results.value.filter((r) => r.status === 'skipped').length
    const run: PersistedRun = {
      id: `ar-${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 8)}`,
      startedAt,
      completedAt,
      totalCases: results.value.length,
      passed,
      failed,
      skipped,
      results: results.value,
    }
    const all = loadPersistedRuns()
    all.unshift(run)
    savePersistedRuns(all)
  }

  /** 暴露给 UI：读取历史 run（用户可以打开看历史报告） */
  function getPersistedRuns(): PersistedRun[] {
    return loadPersistedRuns()
  }

  /** 暴露给 UI：清空历史 */
  function clearPersistedRuns(): void {
    try {
      localStorage.removeItem(RESULTS_STORAGE_KEY)
    } catch {
      // noop
    }
  }

  async function loadPlugins(): Promise<void> {
    isLoadingPlugins.value = true
    lastError.value = null
    try {
      plugins.value = await fetchPlugins()
    } catch (e) {
      lastError.value = e instanceof Error ? e.message : String(e)
    } finally {
      isLoadingPlugins.value = false
    }
  }

  /**
   * 动态笛卡尔积生成测试用例
   *
   * 遍历每个 plugin × taskType × versions × (v4 cipher × compression)
   * 零硬编码 cipher mode / compression / version——从 PluginMeta.taskOptions 派生。
   */
  function generateTestCases(opts: GenerateTestCaseOptions): TestCaseSpec[] {
    const cases: TestCaseSpec[] = []
    for (const plugin of plugins.value) {
      const pluginOpts = plugin.taskOptions
      if (!pluginOpts) continue

      const versions: number[] =
        pluginOpts.supportVersionSelect && pluginOpts.supportedVersions
          ? pluginOpts.supportedVersions
          : [pluginOpts.defaultVersion]

      for (const taskType of ['encrypt', 'decrypt'] as const) {
        for (const version of versions) {
          // includeDeprecated 默认 true：包含 ECv2/ECv3（用于回归）
          // 用户关闭后跳过这些版本
          if (opts.includeDeprecated === false && isDeprecatedVersion(version)) continue

          const isV4 = isRecommendedVersion(version)
          // ECv4 (推荐) encrypt 才有 cipher + compression 笛卡尔积
          // decrypt 不需要（解密时由文件头决定）
          const cipherModes: Array<number | undefined> =
            isV4 && taskType === 'encrypt' ? [0, 1] : [undefined]
          const compressionModes: Array<'none' | 'zstd' | undefined> =
            isV4 && taskType === 'encrypt' ? ['none', 'zstd'] : [undefined]

          for (const cipherMode of cipherModes) {
            for (const compressionMode of compressionModes) {
              const idParts = [
                plugin.name,
                taskType,
                formatContainerVersion(version),
                cipherMode !== undefined ? `c${cipherMode}` : '',
                compressionMode !== undefined ? compressionMode : '',
              ].filter(Boolean)
              cases.push({
                id: idParts.join('-'),
                taskType,
                pluginName: plugin.name,
                sourcePath: opts.sourceFile,
                version,
                cipherMode,
                compressionMode,
                expectedBehavior: isDeprecatedVersion(version) ? 'might-fail' : 'success',
              })
            }
          }
        }
      }
    }
    testCases.value = cases
    return cases
  }

  /**
   * 顺序执行所有测试用例，逐个提交任务。
   * 每个用例独立错误隔离：一个失败不影响其他。
   *
   * 🆕 2026-06-10 修复 #4：每个 case 提交后立即 persistCurrentRun
   *   历史 bug：仅在 runTests 末尾 persistCurrentRun → 200 case 跑 5 分钟期间
   *     用户刷新 / 关 App 全部丢失
   *   修复：每提交一个 case 立即写一次（保证"提交阶段"数据不丢）
   *   配合 onTaskCompleted 中的实时持久化（运行结果阶段数据不丢）→ 全流程实时
   */
  async function runTests(specs: TestCaseSpec[]): Promise<void> {
    isRunning.value = true
    progress.value = { total: specs.length, completed: 0, passed: 0, failed: 0, skipped: 0, pending: 0 }
    results.value = []

    // 🆕 2026-06-10 修复：1 次 runTests = 1 个共享 runId，所有 task 归到同一 group
    // 历史 bug：循环内 Date.now() → 每个 task 独立 runId → Tasks.vue 永远看不到 group 聚合
    // 跟 useWorkflowEngine.runWorkflow 保持一致（共享 run.id）
    const sharedRunId = `at-${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 6)}`

    for (const spec of specs) {
      const result: TestCaseResult = {
        spec,
        status: 'running',
        submittedAt: new Date().toISOString(),
      }
      results.value = [...results.value, result]
      const start = Date.now()

      try {
        // 真机安全：强制改写到 encv-automation 命名空间
        const safeSource = withSafetyBoundary(spec.sourcePath, { forceAutomation: true })
        result.submittedSourcePath = safeSource

        // 前置检查：source 路径是否看起来在 encv-automation 命名空间内
        // 如果不在，说明 withSafetyBoundary 可能被 dev 模式跳过了（forceAutomation 应该覆盖）
        // 这里不做阻塞式检查（因为前端无法 stat 文件），仅记录路径供报告展示
        const task: EncvTask = await createTask(
          spec.taskType,
          safeSource,
          undefined, // targetPath 让后端决定
          'automation-test-pwd', // 全局 password
          spec.version,
          spec.pluginName,
          {},
          undefined, // secondaryPassword
          spec.cipherMode,
          spec.compressionMode,
        )
        recordTriggeredBy(task.id, 'automation', sharedRunId)
        setTaskMetadata(task.id, 'automation', sharedRunId)  // 🆕 v4：merge 到 task 对象
        result.taskId = task.id
        result.status = 'pending' // 任务已提交，等 WS 回调（task:completed）决定最终状态
        result.durationMs = Date.now() - start
        progress.value.pending++ // ← 只计为 pending，不计为 passed
      } catch (e) {
        result.status = 'failed'
        const errMsg = e instanceof Error ? e.message : String(e)
        result.error = errMsg
        // 调用错误分析器生成结构化错误链路 + 修复建议
        result.errorAnalysis = analyzeError(errMsg, { phase: 'submission' })
        result.durationMs = Date.now() - start
        progress.value.failed++
      }

      progress.value.completed++

      // 🆕 实时持久化：每个 case 提交完就写一次（让"提交阶段"的数据不丢）
      persistCurrentRun()
    }

    // 末尾再写一次（确保收尾完整）
    persistCurrentRun()

    isRunning.value = false
  }

  function reset(): void {
    isRunning.value = false
    progress.value = { total: 0, completed: 0, passed: 0, failed: 0, skipped: 0, pending: 0 }
    results.value = []
    testCases.value = []
    lastError.value = null
  }

  return {
    plugins,
    isLoadingPlugins,
    isRunning,
    progress,
    results,
    lastError,
    testCases,
    loadPlugins,
    generateTestCases,
    runTests,
    reset,
    startListening,
    stopListening,
    // 🆕 2026-06-10：本地持久化
    getPersistedRuns,
    clearPersistedRuns,
  }
}
