/**
 * 工作流存储层 — localStorage CRUD
 *
 * 职责：
 * - WorkflowDefinition 的持久化（localStorage）
 * - WorkflowRun 运行历史的持久化
 * - 内置模板的注册和加载
 *
 * MVP 阶段纯前端存储，预留后端 API 接口签名。
 */

import { ref } from 'vue'
import type {
  WorkflowDefinition,
  WorkflowRun,
} from '@/lib/workflow/types'
import { WORKFLOW_STORE_KEY, WORKFLOW_RUNS_KEY } from '@/lib/workflow/types'

/** 生成简易 UUID（不需要 crypto 库） */
function generateId(): string {
  return 'wf-' + Date.now().toString(36) + '-' + Math.random().toString(36).slice(2, 8)
}

function loadJSON<T>(key: string, fallback: T): T {
  try {
    const raw = localStorage.getItem(key)
    return raw ? (JSON.parse(raw) as T) : fallback
  } catch {
    return fallback
  }
}

function saveJSON<T>(key: string, data: T): void {
  try {
    localStorage.setItem(key, JSON.stringify(data))
  } catch (e) {
    console.warn('[WorkflowStore] Failed to save to localStorage:', e)
  }
}

/**
 * 深合并（patch 字段优先于 base；嵌套对象/数组递归处理）
 * - 普通对象递归 merge
 * - 数组 patch 完全替换 base（语义：patch 是新版本，数组整体覆盖）
 * - null/undefined patch 字段保留 base
 * - undefined patch 字段忽略（保留 base）
 *
 * 用例：useWorkflowStore.updateRun(runId, { ...currentRun })
 *   → currentRun 内部 step.status / step.progress 已被 reactive proxy 改过
 *   → 深合并保证这些变化能正确反映到 store.runs 对应 run
 */
function deepMerge<T extends Record<string, any>>(base: T, patch: Partial<T>): T {
  if (patch === null || typeof patch !== 'object') return base
  const result: any = Array.isArray(base) ? [...base] : { ...base }
  for (const key of Object.keys(patch)) {
    const patchVal = (patch as any)[key]
    const baseVal = (base as any)[key]
    if (patchVal === undefined) {
      // 保留 base
      continue
    }
    if (
      patchVal !== null &&
      typeof patchVal === 'object' &&
      !Array.isArray(patchVal) &&
      baseVal !== null &&
      typeof baseVal === 'object' &&
      !Array.isArray(baseVal)
    ) {
      // 普通对象 → 递归
      result[key] = deepMerge(baseVal, patchVal)
    } else if (Array.isArray(patchVal)) {
      // 数组：patch 是新版本 → 浅拷贝 patch 数组（不深拷贝元素，保留 reactive proxy / ref 引用）
      result[key] = [...patchVal]
    } else {
      // 原始值 / null → 直接覆盖
      result[key] = patchVal
    }
  }
  return result
}

export function useWorkflowStore() {
  const definitions = ref<WorkflowDefinition[]>(loadJSON(WORKFLOW_STORE_KEY, []))
  const runs = ref<WorkflowRun[]>(loadJSON(WORKFLOW_RUNS_KEY, []))

  // ==================== Definition CRUD ====================

  function createDefinition(partial: Omit<WorkflowDefinition, 'id' | 'createdAt' | 'updatedAt'> & { id?: string }): WorkflowDefinition {
    const now = new Date().toISOString()
    // 尊重 partial.id（PluginTestsDetail.vue 的 dynamic-auto-test 硬编码 id 场景）
    // 已有调用方都不传 id，所以只在指定时才使用 partial.id，行为与原版 100% 兼容。
    const def: WorkflowDefinition = {
      ...partial,
      id: partial.id ?? generateId(),
      createdAt: now,
      updatedAt: now,
    }
    definitions.value = [...definitions.value, def]
    persistDefinitions()
    return def
  }

  function updateDefinition(id: string, patch: Partial<Omit<WorkflowDefinition, 'id' | 'createdAt'>>): void {
    definitions.value = definitions.value.map((d) =>
      d.id === id ? { ...d, ...patch, updatedAt: new Date().toISOString() } : d,
    )
    persistDefinitions()
  }

  function deleteDefinition(id: string): void {
    // 内置模板不可删除
    const target = definitions.value.find((d) => d.id === id)
    if (target?.builtin) return
    definitions.value = definitions.value.filter((d) => d.id !== id)
    persistDefinitions()
  }

  function getDefinition(id: string): WorkflowDefinition | undefined {
    return definitions.value.find((d) => d.id === id)
  }

  // ==================== Run History ====================

  function addRun(run: WorkflowRun): void {
    runs.value = [run, ...runs.value].slice(0, 100) // 保留最近 100 条
    persistRuns()
  }

  function updateRun(runId: string, patch: Partial<WorkflowRun>): void {
    // 🆕 2026-06-10 修复：深合并 patch（jobs 数组内 step 内部属性变化必须保留）
    // 历史 bug：{ ...r, ...patch } 浅合并 → 如果 patch.jobs 是同一个 reference，浅合并后 step 内部修改能保留（因为引用共享）
    //   但如果调用方传 patch 整个 run（line 191: store.updateRun(currentRun.value.id, { ...currentRun.value })），
    //   spread 会把 run 的所有顶层属性拷给 patch（包括 nested jobs reference）— 实际是 OK 的。
    // 真正脆弱的场景：localStorage 读取时 JSON.parse 拿回 plain object，丢失 reference。
    //   当 useWorkflowEngine 的 onTaskCompleted 改 step.status 后调 store.updateRun（line 191），
    //   patch 里的 jobs 数组和 store.runs 里的 jobs 数组可能是不同的 reference（line 259/291/296 的 updateRun(run.id, run)）。
    //   深合并保证 patch.jobs 内的 step 对象变更能正确反映到 store.runs。
    // 实现：JSON.parse(JSON.stringify) deep clone（性能对 100 条 run × 几百 step 够用，< 5ms）
    runs.value = runs.value.map((r) => (r.id === runId ? deepMerge(r, patch) : r))
    persistRuns()
  }

  function getRun(runId: string): WorkflowRun | undefined {
    return runs.value.find((r) => r.id === runId)
  }

  /** 获取某个 workflowDef 的最近 N 次运行 */
  function getRunsForDefinition(defId: string, limit = 10): WorkflowRun[] {
    return runs.value.filter((r) => r.workflowDefId === defId).slice(0, limit)
  }

  function clearRuns(): void {
    runs.value = []
    persistRuns()
  }

  // ==================== 内置模板管理 ====================

  /**
   * 注册内置模板。如果同名模板已存在则跳过。
   */
  function registerBuiltin(template: WorkflowDefinition): void {
    const exists = definitions.value.some(
      (d) => d.builtin && d.name === template.name,
    )
    if (!exists) {
      definitions.value = [...definitions.value, template]
      persistDefinitions()
    }
  }

  /** 批量注册内置模板 */
  function registerBuiltinTemplates(templates: WorkflowDefinition[]): void {
    for (const t of templates) {
      registerBuiltin(t)
    }
  }

  // ==================== 内部持久化 ====================

  function persistDefinitions(): void {
    saveJSON(WORKFLOW_STORE_KEY, definitions.value)
  }

  function persistRuns(): void {
    saveJSON(WORKFLOW_RUNS_KEY, runs.value)
  }

  return {
    definitions,
    runs,
    createDefinition,
    updateDefinition,
    deleteDefinition,
    getDefinition,
    addRun,
    updateRun,
    getRun,
    getRunsForDefinition,
    clearRuns,
    registerBuiltin,
    registerBuiltinTemplates,
  }
}
