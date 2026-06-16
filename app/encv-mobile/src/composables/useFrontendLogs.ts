import { ref } from 'vue'

export interface LogEntry {
  id: number
  timestamp: string
  level: string
  message: string
  /** 🆕 2026-06-16：来源标签（哪个 console.* 通道 / 哪个 WS 事件 / 哪个 transport）
   *  · 前端：'console.error' / 'console.warn' / 'console.info' / 'console.debug' / 'console.log'
   *  · 后端：http-poll 拉过来的写 'backend_http_poll'；WS 推过来的由 onWsMessage 写 'ws_log_handler' */
  source?: string
  /** 🆕 2026-06-16：错误级别原始堆栈（仅 Error 对象 + 显式含 stack 的字符串）
   *  · 前端：safeStringify 提取 v.stack 写到这字段
   *  · 后端：slog 当前没把 stack 推到 ring buffer（按需后续扩展） */
  stack?: string
}

let nextId = 0
const logs = ref<LogEntry[]>([])

let origConsole: {
  debug: Console['debug']
  info: Console['info']
  warn: Console['warn']
  error: Console['error']
  log: Console['log']
} | null = null

/**
 * 安全序列化任意值为可读字符串。
 * 解决核心痛点：JSON.stringify(new Error('test')) → '{}' （Error 属性不可枚举）
 *
 * 🆕 2026-06-16 增强：返回 { text, stack } 结构 — text 是单行摘要，stack 是 Error.stack
 *   · 弹窗里 message = text（单行） + 单独显示 stack（多行 monospace）
 *   · 列表里仍只显示 text（不撑高）
 */
function safeStringify(v: any): { text: string; stack?: string } {
  if (v == null) return { text: '(null)' }
  if (typeof v === 'string') {
    // 检查字符串里是否含 stack 痕迹（slog 有时把 stack 拼到 message 末尾）
    const stackIdx = v.indexOf('\n    at ')
    if (stackIdx > 0) {
      return { text: v.slice(0, stackIdx).trim(), stack: v.slice(stackIdx).trim() }
    }
    return { text: v }
  }
  if (typeof v === 'number' || typeof v === 'boolean') return { text: String(v) }
  // Error / TypeError / DOMException 等
  if (v instanceof Error) {
    const parts: string[] = [v.name || 'Error', v.message || '(no message)']
    if ((v as any).cause) parts.push(`cause=${safeStringify((v as any).cause).text}`)
    const text = parts.filter(Boolean).join(': ')
    return { text, stack: typeof v.stack === 'string' ? v.stack : undefined }
  }
  // Event 对象
  if (v instanceof Event) {
    const code = (v as any).code
    return { text: code ? v.type + ' (code=' + code + ')' : v.type }
  }
  // Response 对象
  if (v instanceof Response) {
    return { text: `HTTP ${v.status} ${v.statusText}` }
  }
  // 普通对象 / 数组
  try {
    const s = JSON.stringify(v)
    return { text: s !== '{}' ? s : Object.prototype.toString.call(v) }
  } catch {
    return { text: Object.prototype.toString.call(v) }
  }
}

function addLog(level: string, args: any[], source?: string) {
  // 多参数：第一个参数是 message，后面的拼到同一行（兼容原行为）
  // 同时把 Error.stack 从任一参数里挑出来合并到最终 stack
  const textParts: string[] = []
  const stackParts: string[] = []
  for (const a of args) {
    const r = safeStringify(a)
    textParts.push(r.text)
    if (r.stack) stackParts.push(r.stack)
  }
  const entry: LogEntry = {
    id: ++nextId,
    timestamp: new Date().toLocaleTimeString('zh-CN', { hour12: false }),
    level,
    message: textParts.join(' '),
  }
  if (source) entry.source = source
  if (stackParts.length > 0) entry.stack = stackParts.join('\n\n')
  logs.value.push(entry)
  if (logs.value.length > 2000) {
    logs.value = logs.value.slice(-1500)
  }
}

export function hijackConsole() {
  if (origConsole) return
  const saved = {
    debug: console.debug,
    info: console.info,
    warn: console.warn,
    error: console.error,
    log: console.log,
  }
  origConsole = saved
  // 启动日志（确保 DevLogs 首次打开前端面板时不为空）
  addLog('info', ['Frontend logger ready'], 'console.info')
  // 🆕 2026-06-16：立即打一个 Error 让用户能在 DevLogs 看到 stack 渲染效果
  //   点开这条 error 日志应该看到「调用堆栈」section 包含 Error.stack
  //   用于验证 safeStringify 的 stack 提取 + 弹窗 stack section 渲染
  console.error(new Error('Frontend stack test (DevLogs 详情弹窗应显示堆栈)'))
  // 沙箱预览（16000 → 2025 → 5173）链路下 agent-tool-host 不支持 WebSocket 升级，
  // vite HMR client 会持续报 `failed to connect to websocket`。
  // 这是预期内的环境噪声，不影响应用运行（vites 仍能 deliver 模块，
  // 只是没有热重载）。在 DevLogs 里把它降级为 debug 级别，避免淹没真正的错误。
  const isHmrWsNoise = (args: any[]): boolean => {
    if (args.length === 0) return false
    // 拼接所有参数为字符串后统一匹配（覆盖单参数、多参数、Error 对象嵌套等场景）
    const combined = args.map((a): string => {
      if (typeof a === 'string') return a
      if (a instanceof Error) return a.message || ''
      if (a?.message) return String(a.message)
      return String(a ?? '')
    }).join(' ')
    return (
      combined.includes('failed to connect to websocket') ||
      combined.includes('WebSocket closed without opened') ||
      combined.includes('HMR')
    )
  }
  console.debug = (...args: any[]) => { saved.debug(...args); addLog('debug', args, 'console.debug') }
  console.info = (...args: any[]) => { saved.info(...args); addLog('info', args, 'console.info') }
  console.warn = (...args: any[]) => { saved.warn(...args); addLog('warn', args, 'console.warn') }
  console.error = (...args: any[]) => {
    saved.error(...args)
    if (isHmrWsNoise(args)) {
      addLog('debug', ['[HMR WS sandbox noise] ' + args[0]], 'console.error')
      return
    }
    addLog('error', args, 'console.error')
  }
  console.log = (...args: any[]) => { saved.log(...args); addLog('info', args, 'console.log') }
}

export function restoreConsole() {
  if (!origConsole) return
  console.debug = origConsole.debug
  console.info = origConsole.info
  console.warn = origConsole.warn
  console.error = origConsole.error
  console.log = origConsole.log
  origConsole = null
}

export function clearFrontendLogs() {
  logs.value = []
}

export function useFrontendLogs() {
  return {
    logs,
    clearLogs: clearFrontendLogs,
  }
}

export function getFrontendLogsJson(): string {
  return JSON.stringify(logs.value, null, 2)
}
