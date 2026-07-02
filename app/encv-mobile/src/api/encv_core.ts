// encv_core.ts - 服务端 URL / 标识 / 持久化常量
// 拆分自 encv.ts

export const SERVER_URL_KEY = 'encv-server-url'
// 🆕 2026-06-15：跨会话持久化 backend instance_id，用于防"端口被劫持/换进程"误判
//   后端 performPingCheck (internal/register/server_start.go:103) 启动期就比对
//   instance_id 防劫持，移动端必须复用同一机制——单看 200 + JSON 不够。
//   任何"返 200 application/json" 的进程（mock / 旧 encv-go / 上游代理 / 中间人）
//   都会骗过老的 checkServerStatus。InstanceID 是 encv-go 启动时唯一生成的
//   UUID4，进程内常驻不变，重启即换。
export const SERVER_INSTANCE_ID_KEY = 'encv-server-instance-id'
export const SERVER_VERSION_KEY = 'encv-server-version'
export const DEFAULT_API_BASE_URL = 'http://127.0.0.1:2025'
// 🆕 2026-06-10 沙箱 OpenPreview 浏览器专用：必须用**同源** fetch。
//   - OpenPreview 浏览器在 agent-tool-host 上，访问 127.0.0.1/localhost
//     解析到 agent-tool-host 自己的端口（不存在 :16666）→ 永远 connect refused
//   - trae 反代已经把 trae.cn/api/* 完整代理到 :16000 → :16666 → :2025
//     （curl -s http://127.0.0.1:16000/api/config 直接 200，proxy 链路 OK）
//   - 所以 sandbox 浏览器下 baseUrl 必须是**同源**（window.location.origin），
//     fetch 走同源相对 URL，让 trae 反代处理；或者直接返回 '' 让浏览器补 origin
//   - 沙箱本地（非 OpenPreview）的 loopback 浏览器走原 127.0.0.1:16666 路径
//   - APK 真机（capacitor://）直连 127.0.0.1:2025
export const DEV_SANDBOX_ENTRY = 'http://127.0.0.1:16666'

/** 判断当前是否在 OpenPreview 浏览器（agent-tool-host 提供的 trae 域名 mock 浏览器）
 *
 * 🆕 2026-06-10 修复：把 trae 反代端口 16000 也算上
 *   背景：trae 反代 16000 不支持 WebSocket upgrade，OpenPreview 工具激活时
 *   location 可能是 `http://127.0.0.1:16000`（trae 把页面代理到 16000），
 *   这种情况下 origin.hostname === '127.0.0.1'，原 trae 域名正则匹配不到。
 *   必须靠端口 16000 嗅探。
 */
export function isOpenPreviewBrowser(): boolean {
  if (typeof window === 'undefined' || !window.location) return false
  const origin = window.location.origin
  const port = window.location.port
  return (
    /trae\.cn$/i.test(origin) ||
    /agent-sandbox/i.test(origin) ||
    /^run-agent-/i.test(origin) ||
    port === '16000' // 🆕 trae 反代端口
  )
}

export function getApiBaseUrl(): string {
  if (import.meta.env.DEV) {
    // OpenPreview 浏览器（trae 域名）→ 必须同源，让 trae 反代处理
    if (isOpenPreviewBrowser()) {
      return typeof window !== 'undefined' ? window.location.origin : ''
    }
    const stored = localStorage.getItem(SERVER_URL_KEY)
    if (stored) return stored
    return DEV_SANDBOX_ENTRY
  }
  return localStorage.getItem(SERVER_URL_KEY) || DEFAULT_API_BASE_URL
}

export function setApiBaseUrl(url: string) {
  localStorage.setItem(SERVER_URL_KEY, url)
}

export function getServerUrl(): string {
  return getApiBaseUrl()
}

export function resetServerUrl() {
  localStorage.removeItem(SERVER_URL_KEY)
}

export function getWebSocketUrl(): string {
  if (import.meta.env.DEV) {
    const wsProtocol = location.protocol === 'https:' ? 'wss:' : 'ws:'
    return `${wsProtocol}//${location.host}/ws`
  }
  const baseUrl = getApiBaseUrl()
  const wsUrl = baseUrl
    .replace(/^https:\/\//, 'wss://')
    .replace(/^http:\/\//, 'ws://')
  return `${wsUrl}/ws`
}

export function proxySafeEncode(value: string): string {
  return encodeURIComponent(encodeURIComponent(value))
}

export function getPersistedBackendIdentity(): { instanceId: string; version: string } | null {
  if (typeof localStorage === 'undefined') return null
  const id = localStorage.getItem(SERVER_INSTANCE_ID_KEY)
  if (!id) return null
  return { instanceId: id, version: localStorage.getItem(SERVER_VERSION_KEY) || '' }
}
