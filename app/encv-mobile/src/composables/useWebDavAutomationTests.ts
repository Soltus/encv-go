/**
 * useWebDavAutomationTests — 7 module 协调器 + 持久化
 *
 * 🆕 2026-06-17：声明式重构（multi-mount-storage-refactor spec 续）
 *
 * 设计：
 *  - 7 module 各自独立的 run state（ref<RunState>）
 *  - 持久化到 localStorage（key 升级到 v2 兼容破坏性 schema 变化）
 *  - 旧 v1 数据清空（用户已确认）
 *  - 翻译注入：调用 useI18n，传入 runner 的 translateName 闭包
 *
 * 与 useWebDavManifest 关系：
 *  - 测试运行前必须 refresh manifest（拿到真实 mount + virtual file + container map）
 *  - 同一 manifest 在多次 run 之间复用（30s TTL 由 useWebDavManifest 内部管理）
 *
 * 与 useWebDavTestRunner 关系：
 *  - 每个 module 调用 runner.runCase() 跑每个 case
 *  - abort signal 由协调器持有，用户点击 Cancel 时调用 abort()
 */

import { ref, type Ref, type ComputedRef, computed } from 'vue'
import { useI18n } from '@/composables/useI18n'
import { useWebDavManifest } from '@/composables/useWebDavManifest'
import { useWebDavTestRunner } from '@/composables/useWebDavTestRunner'
import { WEBDAV_TEST_MODULES, getModuleById } from '@/composables/useWebDavTestModules'
import type {
  TestCaseResult,
  TestRun,
  WebDavTestContext,
} from '@/types/webdav-test'

const STORAGE_KEY = 'encv_webdav_automation_results_v2'
const MAX_RUNS = 50
const LEGACY_KEYS = ['encv_webdav_automation_results_v1', 'encv_webdav_automation_results']

export type ModuleRunStatus = 'idle' | 'running' | 'done' | 'cancelling' | 'cancelled' | 'error'

export interface ModuleRunState {
  status: ModuleRunStatus
  startedAt?: string
  completedAt?: string
  results: TestCaseResult[]
  error?: string
}

export interface UseWebDavAutomationTestsReturn {
  modules: typeof WEBDAV_TEST_MODULES
  moduleStates: Record<string, Ref<ModuleRunState>>
  historyRuns: Ref<TestRun[]>
  isAnyRunning: ComputedRef<boolean>
  manifestComposable: ReturnType<typeof useWebDavManifest>
  /** 跑单个 module（所有 case 串行） */
  runModule: (moduleId: string) => Promise<void>
  /** 跑所有 module（按顺序） */
  runAll: () => Promise<void>
  /** 取消正在运行的 module */
  cancelModule: (moduleId: string) => void
  /** 清空历史 */
  clearHistory: () => void
  /** 清空单个 module 的当前结果 */
  resetModule: (moduleId: string) => void
}

export function useWebDavAutomationTests(): UseWebDavAutomationTestsReturn {
  const { t } = useI18n()
  const manifestComposable = useWebDavManifest()
  const { runCase } = useWebDavTestRunner()

  // 7 module 独立状态
  const moduleStates: Record<string, Ref<ModuleRunState>> = {}
  for (const m of WEBDAV_TEST_MODULES) {
    moduleStates[m.id] = ref<ModuleRunState>({
      status: 'idle',
      results: [],
    })
  }

  // 历史（持久化）
  const historyRuns = ref<TestRun[]>(loadHistory())

  const isAnyRunning: ComputedRef<boolean> = computed(() => {
    return Object.values(moduleStates).some(
      (s) => s.value.status === 'running' || s.value.status === 'cancelling'
    )
  })

  // 当前 run 的 abort controller（per module）
  const abortControllers: Record<string, AbortController | null> = {}

  async function runModule(moduleId: string): Promise<void> {
    const module = getModuleById(moduleId)
    if (!module) {
      console.error(`[useWebDavAutomationTests] unknown module: ${moduleId}`)
      return
    }
    const state = moduleStates[moduleId]
    if (state.value.status === 'running' || state.value.status === 'cancelling') return

    // 1. 确保 manifest 已就绪
    if (!manifestComposable.isReady.value) {
      await manifestComposable.refresh()
    }
    if (!manifestComposable.isReady.value || !manifestComposable.activeMount.value) {
      state.value = {
        status: 'error',
        results: [],
        error: 'manifest not ready: backend webdav may not be enabled',
      }
      return
    }

    const ctx = buildContext(manifestComposable)
    const controller = new AbortController()
    abortControllers[moduleId] = controller
    ctx.abortSignal = controller.signal

    state.value = {
      status: 'running',
      startedAt: new Date().toISOString(),
      results: [],
    }

    const translateName = (id: string) => t(`devtools.webdav.cases.${id}.name`)
    const results: TestCaseResult[] = []

    try {
      for (const desc of module.cases) {
        if (controller.signal.aborted) {
          // 用户取消：剩余 case 标记 skipped
          for (const skipped of module.cases.filter((c) => !results.find((r) => r.id === c.id))) {
            results.push({
              id: skipped.id,
              name: translateName(skipped.id),
              module: skipped.module,
              status: 'skipped',
              durationMs: 0,
            })
          }
          break
        }
        const result = await runCase(desc, ctx, {
          abortSignal: controller.signal,
          translateName,
        })
        results.push(result)
        state.value = { ...state.value, results: [...results] }
      }

      state.value = {
        ...state.value,
        status: controller.signal.aborted ? 'cancelled' : 'done',
        completedAt: new Date().toISOString(),
        results,
      }
    } catch (e) {
      state.value = {
        ...state.value,
        status: 'error',
        completedAt: new Date().toISOString(),
        error: e instanceof Error ? e.message : String(e),
        results,
      }
    } finally {
      abortControllers[moduleId] = null
      persistRun(moduleId, results, state.value)
    }
  }

  async function runAll(): Promise<void> {
    for (const m of WEBDAV_TEST_MODULES) {
      await runModule(m.id)
    }
  }

  function cancelModule(moduleId: string): void {
    const ctrl = abortControllers[moduleId]
    if (ctrl) {
      ctrl.abort()
      const state = moduleStates[moduleId]
      if (state.value.status === 'running') {
        state.value = { ...state.value, status: 'cancelling' }
      }
    }
  }

  function clearHistory(): void {
    historyRuns.value = []
    persistHistory()
  }

  function resetModule(moduleId: string): void {
    moduleStates[moduleId].value = { status: 'idle', results: [] }
  }

  return {
    modules: WEBDAV_TEST_MODULES,
    moduleStates,
    historyRuns,
    isAnyRunning,
    manifestComposable,
    runModule,
    runAll,
    cancelModule,
    clearHistory,
    resetModule,
  }
}

// ============= 内部辅助 =============

function buildContext(
  manifestComposable: ReturnType<typeof useWebDavManifest>
): WebDavTestContext {
  if (!manifestComposable.manifest.value || !manifestComposable.activeMount.value) {
    throw new Error('manifest not ready')
  }
  return {
    manifest: manifestComposable.manifest.value,
    serverBaseUrl: manifestComposable.serverBaseUrl.value,
    webdavPath: manifestComposable.webdavPath.value || manifestComposable.activeMount.value.webdav_path,
    auth: manifestComposable.auth.value,
    activeMount: manifestComposable.activeMount.value,
    shared: {},
  }
}

function persistRun(moduleId: string, results: TestCaseResult[], state: ModuleRunState): void {
  if (results.length === 0) return
  const passed = results.filter((r) => r.status === 'success').length
  const failed = results.filter((r) => r.status === 'failure' || r.status === 'timed_out').length
  const skipped = results.filter((r) => r.status === 'skipped').length
  const run: TestRun = {
    id: `run_${Date.now()}_${moduleId}`,
    startedAt: state.startedAt ?? new Date().toISOString(),
    completedAt: state.completedAt,
    module: moduleId,
    totalCases: results.length,
    passed,
    failed,
    skipped,
    results,
  }
  const all = loadHistory()
  all.unshift(run)
  // 裁剪：按 startedAt 倒序，最多 MAX_RUNS
  all.sort((a, b) => b.startedAt.localeCompare(a.startedAt))
  const trimmed = all.slice(0, MAX_RUNS)
  localStorage.setItem(STORAGE_KEY, JSON.stringify(trimmed))
}

function loadHistory(): TestRun[] {
  // 🆕 2026-06-17：清空 v1 历史（用户已确认）
  for (const k of LEGACY_KEYS) {
    try { localStorage.removeItem(k) } catch { /* ignore */ }
  }
  try {
    const raw = localStorage.getItem(STORAGE_KEY)
    if (!raw) return []
    const parsed = JSON.parse(raw) as TestRun[]
    return Array.isArray(parsed) ? parsed : []
  } catch {
    return []
  }
}

function persistHistory(runs?: TestRun[]): void {
  if (runs !== undefined) {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(runs))
  }
}
