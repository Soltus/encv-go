type Handler<T = any> = (data: T) => void

// 🆕 2026-06-10 修复：task:* 事件 payload 是完整的 Task 快照（不是只 3 个字段）
// 历史：applyTaskCreated 只解构 {id, type, sourcePath} → 丢失 pluginName/version/targetPath
//       → Tasks.vue 任务组按 pluginName 分桶时全部落到 '(unknown plugin)'
//       → 插件子段头永远不显示，用户报「插件没正确识别，任务依旧全部平铺」
// 修复：类型用 EncvTask 全字段，apply 函数 spread 整个 payload
//
// 后端 Broadcast('task:created', task) 发的就是 *MobileTask 全字段（internal/service/task_manager.go:178-180）
//   WsBackend.ts:88 → emit(msg.type, msg.data) 透传 → 完整结构
//   HttpPollBackend.ts:100 → emit('task:created', {id, type, sourcePath}) ✗ 截断了！需要修
//   ⚠️ 上面那条要同步改 backend/poll → emit 完整 task（见 HttpPollBackend.ts）
import type { EncvTask } from '@/api/encv'
export interface EncvEvents {
  'task:update': Partial<EncvTask> & { id: string; type: string; status: string; progress: number }
  'task:progress': { id: string; progress: number; phase: string; speed: string; eta: string }
  'task:created': Partial<EncvTask> & { id: string; type: string; sourcePath: string; pluginName?: string; version?: number; targetPath?: string; createdAt?: string }
  'task:completed': { id: string; error?: string }
  'task:refresh': Record<string, never>
  'file:change': { path: string; action: 'create' | 'delete' | 'modify' }
  'server:status': { online: boolean }
  'server:connection-error': { error: string }
  'log:message': { level: string; message: string }
  'ws:message': { type: string; data: any }
  'openlist:status': {
    running: boolean
    port: number
    pid: number
    dataSizeBytes: number
    isInstalled: boolean
    lastError: string
    lastUpdateTs: number
  }
  'openlist:log': { level: number; message: string; timestamp: number }
  'openlist:error': { type: string; message: string; code?: number }
  /**
   * api-base:connected — useApiBaseProbe 探测成功 + WS 已重建后 emit。
   * 其它 composable / view 可监听此事件做后续动作（如刷新 agent list、re-mount 工具等）。
   */
  'api-base:connected': { baseUrl: string; source: 'cached' | 'current-origin' | 'loopback' | 'lan-candidate' }
  /**
   * api-base:disconnected — 探测失败（all-candidates-failed）后 emit。
   * UI 监听后显示错误 banner。
   */
  'api-base:disconnected': { error: string }
  /**
   * 🆕 2026-06-15：backend instance_id 跨会话变化（后端真崩重启场景）
   * payload: { previous: string; current: string }
   * UI 监听后顶部 banner 提示 4s，**不**进 lastError / 不阻塞状态机。
   * 历史 bug：之前进 lastError → 永远显示"backend instance changed" → offline 死锁
   */
  'backend:instance-changed': { previous: string; current: string }
}

type EventKey = keyof EncvEvents

const listeners = new Map<string, Set<Handler>>()

function on<K extends EventKey>(event: K, handler: Handler<EncvEvents[K]>) {
  if (!listeners.has(event)) {
    listeners.set(event, new Set())
  }
  listeners.get(event)!.add(handler)
}

function off<K extends EventKey>(event: K, handler: Handler<EncvEvents[K]>) {
  listeners.get(event)?.delete(handler)
}

function emit<K extends EventKey>(event: K, data: EncvEvents[K]) {
  listeners.get(event)?.forEach(handler => {
    try {
      handler(data)
    } catch (e) {
      console.error(`Event bus error on "${event}":`, e)
    }
  })
}

function clear() {
  listeners.clear()
}

export const eventBus = {
  on,
  off,
  emit,
  clear,
}
