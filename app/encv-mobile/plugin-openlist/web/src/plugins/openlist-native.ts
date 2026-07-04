/**
 * OpenList Native 桥接（plugin-openlist web 端）
 *
 * 调 window.OpenListNative（由 plugin-openlist/OpenListPluginJSInterface 注册），
 * 与 OpenListBridge / OpenListConfig / OpenListService 通信。
 *
 * 开发模式（无 WebView）：window.OpenListNative 不存在 → 返回安全默认值
 * 生产模式（嵌入 WebView）：window.OpenListNative 真实存在
 */
import type { OpenListRuntime, OpenListLog } from '@/components-shared'

declare global {
  interface Window {
    OpenListNative?: {
      startOpenList(): string
      stopOpenList(): boolean
      getRuntimeStatus(): string
      setAdminPassword(password: string): boolean
      readConfig(): string
      writeConfig(content: string): boolean
      getVersion(): string
      getDataDir(): string
      getPort(): number
      getIsRunning(): boolean
    }
  }
}

function safe<T>(fallback: T, fn: () => T): T {
  try {
    return fn()
  } catch {
    return fallback
  }
}

export const OpenListNative = {
  /** 启动 OpenList 后端，返回端口号 */
  startOpenList(): number {
    return safe(0, () => parseInt(window.OpenListNative?.startOpenList() ?? '0', 10))
  },

  /** 停止 OpenList 后端 */
  stopOpenList(): boolean {
    return safe(false, () => window.OpenListNative?.stopOpenList() ?? false)
  },

  /** 获取运行时状态 */
  getStatus(): OpenListRuntime {
    const empty: OpenListRuntime = {
      running: false,
      port: 0,
      pid: 0,
      dataSizeBytes: 0,
      lastError: '',
      lastUpdateTs: 0,
      dataDir: '',
      isInstalled: true,
    }
    return safe(empty, () => {
      const json = window.OpenListNative?.getRuntimeStatus() ?? '{}'
      return JSON.parse(json) as OpenListRuntime
    })
  },

  /** 设置管理员密码 */
  setPassword(password: string): boolean {
    return safe(false, () => window.OpenListNative?.setAdminPassword(password) ?? false)
  },

  /** 读取 config.json */
  readConfig(): string {
    return safe('{}', () => window.OpenListNative?.readConfig() ?? '{}')
  },

  /** 写入 config.json（自动备份） */
  writeConfig(content: string): boolean {
    return safe(false, () => window.OpenListNative?.writeConfig(content) ?? false)
  },

  /** 获取 OpenList 版本 */
  getVersion(): string {
    return safe('unknown', () => window.OpenListNative?.getVersion() ?? 'unknown')
  },

  /** 获取 data 目录路径 */
  getDataDir(): string {
    return safe('', () => window.OpenListNative?.getDataDir() ?? '')
  },

  /** 获取当前配置的端口 */
  getPort(): number {
    return safe(0, () => window.OpenListNative?.getPort() ?? 0)
  },

  /** 检查 OpenList 后端是否运行中 */
  getIsRunning(): boolean {
    return safe(false, () => window.OpenListNative?.getIsRunning() ?? false)
  },
}

/**
 * 简单日志缓冲器（web 端独立维护）
 * 注：真实日志流需要 OpenListBridge 主动推送（如未来通过 Capacitor Events / SSE 暴露）
 */
export class LogBuffer {
  private logs: OpenListLog[] = []
  private listeners: Array<(logs: OpenListLog[]) => void> = []
  private maxLength = 500

  add(level: OpenListLog['level'], message: string) {
    this.logs.push({ level, message, timestamp: Date.now() })
    if (this.logs.length > this.maxLength) {
      this.logs = this.logs.slice(-this.maxLength)
    }
    this.notify()
  }

  info(message: string) { this.add('info', message) }
  warn(message: string) { this.add('warn', message) }
  error(message: string) { this.add('error', message) }
  debug(message: string) { this.add('debug', message) }

  getAll(): OpenListLog[] {
    return this.logs
  }

  clear() {
    this.logs = []
    this.notify()
  }

  subscribe(listener: (logs: OpenListLog[]) => void): () => void {
    this.listeners.push(listener)
    listener(this.logs)
    return () => {
      this.listeners = this.listeners.filter((l) => l !== listener)
    }
  }

  private notify() {
    for (const l of this.listeners) l(this.logs)
  }
}

export const logBuffer = new LogBuffer()
