/**
 * useErrorCapture.ts — 三管齐下错误捕获（2026-07-02 用户强反馈 A5）
 *
 * 用户原话："真正的渲染错误是有调试页面的，这显然是更底层的错误没有捕获，
 *           比如不支持安卓端的调用"
 *
 * 三管齐下：
 *   1. Vue 顶层 app.config.errorHandler → 捕获 Vue 组件渲染错误
 *   2. window.onerror + unhandledrejection → 捕获未处理 JS 异常
 *   3. console.error 重定向 → 收集 console 错误（很多异步错误只在这里打）
 *
 * 数据流：
 *   - 错误 → useErrorStore 收集（带去重 + LRU 上限）
 *   - 错误浮窗（<ErrorCaptureOverlay>）→ 显示在屏幕底部
 *   - 错误详情（<ErrorDetailPanel>）→ 在 Settings → DevTools 里
 *   - localStorage 持久化（encv_error_capture_v1）→ 跨会话保留
 *
 * 与 A5 三个方案对应：
 *   - "Vue 顶层 try-catch + 错误卡片"  → ① + 错误卡片 UI（在 FullTextIndexDetail 等页）
 *   - "app.config.errorHandler + window.onerror"  → ① + ②
 *   - "Console 重定向 + 浮窗显示"  → ③ + <ErrorCaptureOverlay> 浮窗
 */

import { reactive, ref, type Ref } from 'vue'

export interface CapturedError {
  id: string
  timestamp: number
  source: 'vue' | 'window' | 'promise' | 'console'
  message: string
  stack?: string
  componentName?: string
  /** 错误发生时的 URL 路径（Ionic router） */
  url?: string
}

const STORAGE_KEY = 'encv_error_capture_v1'
const MAX_ERRORS = 50

class ErrorStoreImpl {
  errors = reactive<CapturedError[]>([])
  /** 是否显示浮窗 */
  showOverlay = ref(false)
  /** 浮窗上最近一个错误（高亮） */
  latestError: Ref<CapturedError | null> = ref(null)

  private seen = new Set<string>() // 去重 fingerprint

  addError(err: Omit<CapturedError, 'id' | 'timestamp'>) {
    // 简单 fingerprint 去重（同一消息 + 同一 stack 10s 内不重复）
    const fp = `${err.source}:${err.message}:${(err.stack || '').slice(0, 200)}`
    if (this.seen.has(fp)) return
    this.seen.add(fp)
    setTimeout(() => this.seen.delete(fp), 10000)

    const entry: CapturedError = {
      id: `err_${Date.now()}_${Math.random().toString(36).slice(2, 8)}`,
      timestamp: Date.now(),
      ...err,
    }
    this.errors.unshift(entry)
    if (this.errors.length > MAX_ERRORS) {
      this.errors.length = MAX_ERRORS
    }
    this.latestError.value = entry
    this.showOverlay.value = true
    this.persist()
  }

  clear() {
    this.errors.length = 0
    this.latestError.value = null
    this.seen.clear()
    try {
      localStorage.removeItem(STORAGE_KEY)
    } catch {
      // ignore (e.g. private mode)
    }
  }

  dismissOverlay() {
    this.showOverlay.value = false
  }

  private persist() {
    try {
      // 只持久化最近 20 条（localStorage 限额）
      const subset = this.errors.slice(0, 20).map(e => ({
        ...e,
        // 不持久化完整 stack（体积大）
        stack: e.stack ? e.stack.slice(0, 500) : undefined,
      }))
      localStorage.setItem(STORAGE_KEY, JSON.stringify(subset))
    } catch {
      // localStorage 满了或不可用 → 忽略
    }
  }

  hydrate() {
    try {
      const raw = localStorage.getItem(STORAGE_KEY)
      if (!raw) return
      const arr = JSON.parse(raw) as CapturedError[]
      if (Array.isArray(arr)) {
        this.errors.push(...arr)
      }
    } catch {
      // ignore
    }
  }
}

export const errorStore = new ErrorStoreImpl()

let installed = false

/**
 * 安装三管齐下错误捕获。
 *
 * 必须在 createApp 之后调一次。多次调用安全（只生效一次）。
 */
export function installErrorCapture() {
  if (installed) return
  installed = true

  // ① Vue 组件渲染错误（用户反馈"真正的渲染错误是有调试页面的"）
  //   通过 app.config.errorHandler 安装（在 main.ts 调 installErrorCapture(app) 时挂上）
  //   ② 全局 JS 错误
  if (typeof window !== 'undefined') {
    window.addEventListener('error', (e) => {
      errorStore.addError({
        source: 'window',
        message: e.message || String(e.error || 'unknown'),
        stack: e.error?.stack,
        url: window.location.pathname,
      })
    })

    // ② 未处理的 Promise 拒绝
    window.addEventListener('unhandledrejection', (e) => {
      const reason = e.reason
      errorStore.addError({
        source: 'promise',
        message: reason instanceof Error ? reason.message : String(reason),
        stack: reason instanceof Error ? reason.stack : undefined,
        url: window.location.pathname,
      })
    })
  }

  // ③ console.error 重定向
  //   用户反馈"更底层的错误没有捕获，比如不支持安卓端的调用"
  //   很多 Capacitor / WebView 兼容性问题只在 console.error 打
  const originalConsoleError = console.error.bind(console)
  console.error = (...args: unknown[]) => {
    originalConsoleError(...args)
    // 只收集含 error/exception/fail 关键词的（避免收集 debug log）
    const msg = args.map(a => {
      if (a instanceof Error) return a.message
      if (typeof a === 'string') return a
      try {
        return JSON.stringify(a)
      } catch {
        return String(a)
      }
    }).join(' ')
    if (/error|exception|fail|undef|null|cannot/i.test(msg)) {
      errorStore.addError({
        source: 'console',
        message: msg.slice(0, 500),
        stack: args.find(a => a instanceof Error) instanceof Error
          ? (args.find(a => a instanceof Error) as Error).stack
          : undefined,
        url: typeof window !== 'undefined' ? window.location.pathname : undefined,
      })
    }
  }

  errorStore.hydrate()
}

/**
 * 包装一个 Vue app，把 app.config.errorHandler 挂上。
 *
 * 用法（main.ts）：
 *   const app = createApp(App)
 *   installErrorCapture()
 *   bindVueErrorHandler(app)
 */
export function bindVueErrorHandler(app: { config: { errorHandler?: (err: unknown, instance: unknown, info: string) => void } }) {
  app.config.errorHandler = (err, instance, info) => {
    const componentName = (instance as { $options?: { name?: string } })?.$options?.name
    errorStore.addError({
      source: 'vue',
      message: err instanceof Error ? err.message : String(err),
      stack: err instanceof Error ? err.stack : undefined,
      componentName,
      url: typeof window !== 'undefined' ? window.location.pathname : undefined,
    })
    // 仍然打 console（让 dev tools 能看到）
    console.error('[Vue error]', err, info)
  }
}
