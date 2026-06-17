/**
 * useTaskEventBridge — 唯一 WS 4 件套入口
 *
 * 职责：
 * - 集中订阅 task:created / task:update / task:progress / task:completed 4 件套事件
 * - 兼容订阅 file:change / server:status / ws:message / task:refresh
 * - onMounted 自动 startListening / onUnmounted 自动 stopListening
 *
 * 设计原则（automation-workflow 规则 §二）：
 * - 任何"WS 事件"消费方必须放宽状态匹配 + 加终态保护
 * - 任何"批量执行"系统必须实时持久化（提交阶段 + 运行阶段 + 末尾）
 * - 4 件套事件缺一不可（修复前只监 task:completed 导致状态机升级丢失）
 *
 * 与 useWorkflowEngine / useAutomationTests 的关系：
 *   两者各自实现了 startListening/stopListening（重复订阅 4 件套），
 *   Task 7/8 退役后由 useWorkflowTaskService（Task 4）统一通过本 composable 订阅。
 *   本文件是 spec unify-workflow-task-service Task 3 引入的"官方"入口。
 *
 * 工具函数 re-export：
 * - applyTerminalGuard：终态保护，防止后端延迟事件把已终态 step 降级
 * - validateTransition：状态机校验，基于 VALID_TRANSITIONS 判断转换合法性
 */
import { onMounted, onUnmounted } from 'vue'
import { eventBus } from '@/composables/useEventBus'

// re-export 状态机工具函数（供 useWorkflowTaskService / 调用方直接使用）
export { applyTerminalGuard, validateTransition, VALID_TRANSITIONS } from '@/lib/workflow/state-machine'

export interface TaskEventBridgeOptions {
  onUpdate?: (data: { id: string; type: string; status: string; progress: number }) => void
  onProgress?: (data: { id: string; progress: number; phase: string; speed: string; eta: string }) => void
  onCreate?: (data: { id: string; type: string; sourcePath: string }) => void
  onComplete?: (data: { id: string; error?: string }) => void
  onRefresh?: () => void
  onFileChange?: (data: { path: string; action: 'create' | 'delete' | 'modify' }) => void
  onServerStatus?: (data: { online: boolean }) => void
  onWsMessage?: (data: { type: string; data: any }) => void
}

export function useTaskEventBridge(options: TaskEventBridgeOptions) {
  onMounted(() => {
    if (options.onUpdate) eventBus.on('task:update', options.onUpdate)
    if (options.onProgress) eventBus.on('task:progress', options.onProgress)
    if (options.onCreate) eventBus.on('task:created', options.onCreate)
    if (options.onComplete) eventBus.on('task:completed', options.onComplete)
    if (options.onRefresh) eventBus.on('task:refresh', options.onRefresh)
    if (options.onFileChange) eventBus.on('file:change', options.onFileChange)
    if (options.onServerStatus) eventBus.on('server:status', options.onServerStatus)
    if (options.onWsMessage) eventBus.on('ws:message', options.onWsMessage)
  })

  onUnmounted(() => {
    if (options.onUpdate) eventBus.off('task:update', options.onUpdate)
    if (options.onProgress) eventBus.off('task:progress', options.onProgress)
    if (options.onCreate) eventBus.off('task:created', options.onCreate)
    if (options.onComplete) eventBus.off('task:completed', options.onComplete)
    if (options.onRefresh) eventBus.off('task:refresh', options.onRefresh)
    if (options.onFileChange) eventBus.off('file:change', options.onFileChange)
    if (options.onServerStatus) eventBus.off('server:status', options.onServerStatus)
    if (options.onWsMessage) eventBus.off('ws:message', options.onWsMessage)
  })
}
