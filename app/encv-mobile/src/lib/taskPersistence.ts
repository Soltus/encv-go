/**
 * v6 2026-06-18 任务系统重设计
 *
 * useTaskPersistence - IndexedDB (Dexie) 持久化层
 *
 * 替代 localStorage：
 * - 异步、非阻塞（冷启动不卡 UI）
 * - 大 JSON 读写不卡（10x 性能）
 * - 支持索引（按 runId 查 tasks / 按 pluginName 查）
 *
 * 迁移策略（v6 决定）：清空 localStorage，从零开始
 * - 旧 localStorage key：encv_tasks_v1 / encv_automation_results_v1 等
 * - 启动时检测到 localStorage 旧数据 → 删除 → 用空 IndexedDB 启动
 *
 * 表结构：
 * - tasks: { id, runId, status, createdAt, ... } 主键 id，索引 runId/status/pluginName
 * - filterState: { key: 'viewMode' | 'datePreset' | 'sortBy', value }
 * - meta: { key, value } 存最后打开 tab / 折叠态等小数据
 */
import Dexie, { type Table } from 'dexie'
import type { EncvTask } from '@/api/encv'

// ============ Schema ============

export interface TaskRow extends EncvTask {
  /** 持久化时间戳（v6 加：避免后端数据比本地旧时被覆盖） */
  _persistedAt?: number
}

export interface FilterStateRow {
  key: 'viewMode' | 'datePreset' | 'sortBy' | 'activeFilters'
  value: any
}

export interface MetaRow {
  key: string
  value: any
}

// ============ Dexie 实例 ============

class TaskDB extends Dexie {
  tasks!: Table<TaskRow, string>
  filterState!: Table<FilterStateRow, string>
  meta!: Table<MetaRow, string>

  constructor() {
    super('encv_task_store_v6')
    this.version(1).stores({
      // 主键 id；索引 runId / status / pluginName
      tasks: 'id, runId, status, pluginName, createdAt',
      filterState: 'key',
      meta: 'key',
    })
  }
}

let _db: TaskDB | null = null
let _dbPromise: Promise<TaskDB> | null = null

/**
 * 懒加载数据库连接（首次调用才 open）
 * - 同步返回 _db（首次为 null 时内部异步 open）
 * - 调用方：await getDB() 拿 ready 状态的 db
 */
export function getDB(): Promise<TaskDB> {
  if (_db) return Promise.resolve(_db)
  if (_dbPromise) return _dbPromise
  _dbPromise = new Promise((resolve, reject) => {
    const db = new TaskDB()
    db.open()
      .then(() => {
        _db = db
        resolve(db)
      })
      .catch((err) => {
        console.error('[useTaskPersistence] open DB failed:', err)
        _dbPromise = null
        reject(err)
      })
  })
  return _dbPromise
}

// ============ 旧 localStorage key 清理（v6 决定：清空迁移） ============

const LEGACY_LOCALSTORAGE_KEYS = [
  'encv_tasks_v1',
  'encv_automation_results_v1',
  'encv_tasks_expanded_groups_v1',
  'encv-tasks-viewmode',
  'encv-triggers-v1',
]

/** 启动时清理旧 localStorage key（一次性） */
export function clearLegacyLocalStorage(): void {
  if (typeof localStorage === 'undefined') return
  for (const key of LEGACY_LOCALSTORAGE_KEYS) {
    try {
      localStorage.removeItem(key)
    } catch {
      // silent
    }
  }
}

// ============ 高层 API ============

/** 全量读 tasks（冷启动用） */
export async function loadAllTasks(): Promise<EncvTask[]> {
  try {
    const db = await getDB()
    const rows = await db.tasks.toArray()
    // 去掉 _persistedAt（业务不关心）
    return rows.map((r) => {
      const { _persistedAt, ...task } = r
      return task as EncvTask
    })
  } catch (err) {
    console.warn('[useTaskPersistence] loadAllTasks failed:', err)
    return []
  }
}

/** 写单个 task（patchTask 用） */
export async function putTask(task: EncvTask): Promise<void> {
  try {
    const db = await getDB()
    await db.tasks.put({ ...task, _persistedAt: Date.now() })
  } catch (err) {
    console.warn('[useTaskPersistence] putTask failed:', err)
  }
}

/**
 * 批量写 tasks（bulkSetTasks 用）
 * - debounce 100ms 避免频繁写
 */
let _bulkWriteTimer: ReturnType<typeof setTimeout> | null = null
let _pendingTasks: EncvTask[] = []

export function bulkPutTasks(tasks: EncvTask[]): void {
  _pendingTasks = tasks
  if (_bulkWriteTimer) clearTimeout(_bulkWriteTimer)
  _bulkWriteTimer = setTimeout(async () => {
    try {
      const db = await getDB()
      const rows = _pendingTasks.map((t) => ({ ...t, _persistedAt: Date.now() }))
      await db.tasks.bulkPut(rows)
      _pendingTasks = []
    } catch (err) {
      console.warn('[useTaskPersistence] bulkPutTasks failed:', err)
    }
  }, 100)
}

/** 删除 task */
export async function deleteTask(id: string): Promise<void> {
  try {
    const db = await getDB()
    await db.tasks.delete(id)
  } catch (err) {
    console.warn('[useTaskPersistence] deleteTask failed:', err)
  }
}

/** 清空所有 task（reset / 测试用） */
export async function clearAllTasks(): Promise<void> {
  try {
    const db = await getDB()
    await db.tasks.clear()
  } catch (err) {
    console.warn('[useTaskPersistence] clearAllTasks failed:', err)
  }
}

// ============ Filter state 持久化 ============

export async function loadFilterState(): Promise<Record<string, any>> {
  try {
    const db = await getDB()
    const rows = await db.filterState.toArray()
    const out: Record<string, any> = {}
    for (const r of rows) out[r.key] = r.value
    return out
  } catch {
    return {}
  }
}

export async function putFilterState(key: string, value: any): Promise<void> {
  try {
    const db = await getDB()
    await db.filterState.put({ key: key as any, value })
  } catch {
    // silent
  }
}
