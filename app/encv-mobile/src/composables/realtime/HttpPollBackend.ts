/**
 * HttpPollBackend — HTTP 轮询 transport 实现（沙箱 OpenPreview 浏览器 fallback）
 *
 * 适用场景：
 *   - 沙箱 OpenPreview 浏览器（trae 反代 :16000 不支持 WebSocket upgrade）
 *   - 真机浏览器不想用 WS 时
 *   - WS 失败时降级
 *
 * 算法：
 *   - 每 BASELINE_INTERVAL_MS=2s（active）/ HIDDEN_INTERVAL_MS=30s（document hidden）轮询一次
 *   - 错误时 exp backoff 重试（1s → 2s → 4s → ... → max 30s）
 *   - 维护 lastSnapshot: Map<taskId, {status, progress, phase, speed, eta, error}>
 *   - 每轮 diff：
 *     - 新 id          → emit('task:created',   {id, type, sourcePath})
 *     - status 变      → emit('task:update',    {id, type, status, progress})
 *     - progress/phase → emit('task:progress',  {id, progress, phase, speed, eta})
 *     - 终态           → emit('task:completed', {id, error})
 *     - 后端列表少 id  → emit('task:completed', {id, error: 'server-list-missing'})（罕见容错）
 *
 * 优点：
 *   - 后端零改动（用现有 GET /api/tasks）
 *   - trae 反代支持 HTTP（trae 反代 :16000 → :16666 → :2025）
 *   - 实时性可接受（2s 轮询 + 一开始立即拉首次）
 *
 * TODO 未来优化：
 *   - 后端 GET /api/tasks?since=ts 支持增量查询（避免拉全量）
 *   - Web Locks API 防多 tab 重复 poll
 */

import { getTasks, getRecentBackendLogs, type EncvTask } from '@/api/encv'
import type { Backend, EventEmitter } from './Backend'

/** snapshot 用最少的字段做 diff（其他字段变化跟 task 4 件套无关） */
interface TaskSnapshot {
  status: string
  progress: number
  phase?: string
  speed?: string
  eta?: string
  error?: string
}

function snapshotOf(t: EncvTask): TaskSnapshot {
  return {
    status: t.status,
    progress: t.progress,
    phase: t.phase,
    speed: t.speed,
    eta: t.eta,
    error: t.error,
  }
}

function isTerminal(status?: string): boolean {
  return status === 'success' || status === 'failure' || status === 'cancelled' || status === 'completed'
}

export interface HttpPollBackendOptions {
  /** 首次成功 poll 回调（给 transport 改 connectionState） */
  onConnected?: () => void
  /** 持续失败回调（默认不发 server:connection-error，由 transport 处理） */
  onError?: (e: unknown) => void
  /** 注入 fetchTasks（测试用） */
  fetchTasks?: () => Promise<EncvTask[]>
  /** 注入 fetchLogs（测试用；默认调 getRecentBackendLogs） */
  fetchLogs?: () => Promise<Awaited<ReturnType<typeof getRecentBackendLogs>>>
}

const BASELINE_INTERVAL_MS = 2000
const HIDDEN_INTERVAL_MS = 30000
const MAX_BACKOFF_MS = 30000

export function createHttpPollBackend(
  emit: EventEmitter,
  options: HttpPollBackendOptions = {},
): Backend {
  // 🆕 2026-06-10 修复：pollTimer 用 any 避免 setTimeout 在 Node (number) / Browser (Timeout) 类型冲突
  let pollTimer: any = null
  let backoffMs = 1000
  let running = false
  let firstTickResolved = false

  const lastSnapshot = new Map<string, TaskSnapshot>()

  const _fetchTasks = options.fetchTasks ?? getTasks
  const _fetchLogs = options.fetchLogs ?? getRecentBackendLogs
  let lastLogTimestamp = ''  // 增量拉日志游标（HH:MM:SS 字符串）

  function intervalMs(): number {
    if (typeof document !== 'undefined' && document.visibilityState === 'hidden') {
      return HIDDEN_INTERVAL_MS
    }
    return BASELINE_INTERVAL_MS
  }

  function diffAndEmit(tasks: EncvTask[]): void {
    const seen = new Set<string>()
    for (const t of tasks) {
      seen.add(t.id)
      const prev = lastSnapshot.get(t.id)
      if (!prev) {
        // 🆕 新 task
        lastSnapshot.set(t.id, snapshotOf(t))
        // 🆕 2026-06-10 修复：emit 完整 task（不是只 3 字段），applyTaskCreated 需要 pluginName/version/targetPath
        // 历史 bug：{id, type, sourcePath} → pluginName 丢失 → Tasks.vue 任务组按 pluginName 分桶全部落到
        //   '(unknown plugin)' → 「插件没正确识别，任务依旧全部平铺」
        // 跟 WsBackend 行为对齐（WsBackend 直接 emit msg.data，透传全字段）
        emit('task:created', t)
        if (t.status && t.status !== 'queued') {
          emit('task:update', { id: t.id, type: t.type, status: t.status, progress: t.progress })
        }
        if (isTerminal(t.status)) {
          emit('task:completed', { id: t.id, error: t.error })
        }
        continue
      }
      // status 变化
      if (prev.status !== t.status) {
        emit('task:update', { id: t.id, type: t.type, status: t.status, progress: t.progress })
        if (isTerminal(t.status)) {
          emit('task:completed', { id: t.id, error: t.error })
        }
      }
      // progress/phase 变化
      if (
        prev.progress !== t.progress ||
        prev.phase !== t.phase ||
        prev.speed !== t.speed ||
        prev.eta !== t.eta
      ) {
        emit('task:progress', { id: t.id, progress: t.progress, phase: t.phase, speed: t.speed, eta: t.eta })
      }
      lastSnapshot.set(t.id, snapshotOf(t))
    }

    // 🆕 防御：snapshot 有但 server 列表没了（罕见：后端重启）→ 标 completed
    for (const id of lastSnapshot.keys()) {
      if (!seen.has(id)) {
        const prev = lastSnapshot.get(id)!
        if (!isTerminal(prev.status)) {
          emit('task:completed', { id, error: 'server-list-missing' })
        }
        lastSnapshot.delete(id)
      }
    }
  }

  async function fetchAndEmitLogs(): Promise<void> {
    // 🆕 2026-06-16：拉后端 ring buffer 日志（http-poll 模式下用户期望 devlogs 看到后端日志）
    // 增量拉：since=lastLogTimestamp（HH:MM:SS 字符串），server 端按字符串 > 过滤
    try {
      const resp = await _fetchLogs(lastLogTimestamp || undefined)
      for (const e of resp.logs || []) {
        emit('log', {
          type: 'log',
          data: {
            level: e.level,
            message: e.message,
            timestamp: e.timestamp,
          },
        })
        if (e.timestamp > lastLogTimestamp) lastLogTimestamp = e.timestamp
      }
    } catch (e) {
      console.warn('[HttpPollBackend] fetchLogs failed:', e instanceof Error ? e.message : String(e))
    }
  }

  async function tick(): Promise<void> {
    if (!running) return
    try {
      const tasks = await _fetchTasks()
      diffAndEmit(tasks)
      // 🆕 拉后端日志（独立 try/catch — 不让日志拉失败影响 task 拉取）
      await fetchAndEmitLogs()
      backoffMs = 1000  // 成功重置
      if (!firstTickResolved) {
        firstTickResolved = true
        options.onConnected?.()
        emit('server:status', { online: true })
      }
    } catch (e) {
      // 失败：exp backoff（不弹 connection-error 给用户，避免 noise）
      console.warn('[HttpPollBackend] tick failed:', e instanceof Error ? e.message : String(e))
      options.onError?.(e)
      if (typeof window !== 'undefined') {
        pollTimer = window.setTimeout(tick, backoffMs)
      } else {
        pollTimer = setTimeout(tick, backoffMs)
      }
      backoffMs = Math.min(backoffMs * 2, MAX_BACKOFF_MS)
      return
    }
    if (typeof window !== 'undefined') {
      pollTimer = window.setTimeout(tick, intervalMs())
    } else {
      pollTimer = setTimeout(tick, intervalMs())
    }
  }

  return {
    start() {
      if (running) return
      running = true
      firstTickResolved = false
      backoffMs = 1000
      lastLogTimestamp = ''  // 重置增量游标（start 重新拉全量）
      tick()
    },
    stop() {
      running = false
      if (pollTimer) {
        clearTimeout(pollTimer)
        pollTimer = null
      }
    },
    reset() {
      this.stop()
      lastSnapshot.clear()
      backoffMs = 1000
      firstTickResolved = false
      lastLogTimestamp = ''
    },
  }
}
