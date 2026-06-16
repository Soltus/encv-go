const SERVER_URL_KEY = 'encv-server-url'
// 🆕 2026-06-15：跨会话持久化 backend instance_id，用于防"端口被劫持/换进程"误判
//   后端 performPingCheck (internal/register/server_start.go:103) 启动期就比对
//   instance_id 防劫持，移动端必须复用同一机制——单看 200 + JSON 不够。
//   任何"返 200 application/json" 的进程（mock / 旧 encv-go / 上游代理 / 中间人）
//   都会骗过老的 checkServerStatus。InstanceID 是 encv-go 启动时唯一生成的
//   UUID4，进程内常驻不变，重启即换。
const SERVER_INSTANCE_ID_KEY = 'encv-server-instance-id'
const SERVER_VERSION_KEY = 'encv-server-version'
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
    port === '16000'  // 🆕 trae 反代端口
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

export interface FileItem {
  name: string
  display_name?: string
  path: string
  isDirectory: boolean
  isEncrypted?: boolean
  size?: number
  modified?: string
}

export interface FileListResponse {
  files: FileItem[]
  error?: string
  code?: string
}

export class PermissionDeniedError extends Error {
  constructor(message: string) {
    super(message)
    this.name = 'PermissionDeniedError'
  }
}

export class NotFoundError extends Error {
  constructor(message: string) {
    super(message)
    this.name = 'NotFoundError'
  }
}

export async function listFiles(path = '/'): Promise<FileItem[]> {
  const baseUrl = getApiBaseUrl()
  const response = await fetch(`${baseUrl}/api/files?path=${proxySafeEncode(path)}`)
  if (!response.ok) {
    if (response.status === 403) {
      const data: FileListResponse = await response.json().catch(() => ({}))
      if (data.code === 'PERMISSION_DENIED') {
        console.debug('[API] listFiles permission denied:', path)
        throw new PermissionDeniedError(data.error || 'Permission denied')
      }
    }
    if (response.status === 404) {
      const data: FileListResponse = await response.json().catch(() => ({}))
      console.debug('[API] listFiles not found:', path)
      throw new NotFoundError(data.error || 'Path not found')
    }
    console.error('[API] listFiles failed:', response.status)
    throw new Error(`HTTP error! status: ${response.status}`)
  }
  const data: FileListResponse = await response.json()
  console.info('[API] listFiles:', path, '→', data.files?.length || 0, 'files')
  return data.files || []
}

export async function listFilesStream(
  path = '/',
  onItem: (file: FileItem) => void,
  signal?: AbortSignal
): Promise<{ files: FileItem[]; error?: string }> {
  const baseUrl = getApiBaseUrl()
  const response = await fetch(`${baseUrl}/api/files/stream?path=${proxySafeEncode(path)}`, {
    signal,
  })

  if (!response.ok) {
    if (response.status === 403) {
      throw new PermissionDeniedError('Permission denied')
    }
    if (response.status === 404) {
      throw new NotFoundError('Path not found')
    }
    throw new Error(`HTTP error! status: ${response.status}`)
  }

  const files: FileItem[] = []
  const reader = response.body!.getReader()
  const decoder = new TextDecoder()
  let buffer = ''

  try {
    while (true) {
      const { done, value } = await reader.read()
      if (done) break

      buffer += decoder.decode(value, { stream: true })
      const lines = buffer.split('\n')
      buffer = lines.pop() || ''

      for (const line of lines) {
        if (!line.startsWith('data: ')) continue
        const data = line.slice(6).trim()
        if (!data) continue

        if (data === '[DONE]') {
          return { files }
        }

        try {
          const file = JSON.parse(data) as FileItem
          files.push(file)
          onItem(file)
        } catch {
          // skip malformed JSON
        }
      }
    }
  } finally {
    reader.releaseLock()
  }

  return { files }
}

export async function listPluginFilesStream(
  path: string,
  extensions: string[],
  onItem: (file: FileItem) => void,
  signal?: AbortSignal
): Promise<{ files: FileItem[]; error?: string }> {
  const baseUrl = getApiBaseUrl()
  const extParam = extensions.map(e => `.${e.toLowerCase()}`).join(',')
  const response = await fetch(`${baseUrl}/api/files/plugin-stream?path=${proxySafeEncode(path)}&extensions=${encodeURIComponent(extParam)}`, {
    signal,
  })

  if (!response.ok) {
    if (response.status === 403) {
      throw new PermissionDeniedError('Permission denied')
    }
    throw new Error(`HTTP error! status: ${response.status}`)
  }

  const files: FileItem[] = []
  const reader = response.body!.getReader()
  const decoder = new TextDecoder()
  let buffer = ''

  try {
    while (true) {
      const { done, value } = await reader.read()
      if (done) break

      buffer += decoder.decode(value, { stream: true })
      const lines = buffer.split('\n')
      buffer = lines.pop() || ''

      for (const line of lines) {
        if (!line.startsWith('data: ')) continue
        const data = line.slice(6).trim()
        if (!data) continue

        if (data === '[DONE]') {
          return { files }
        }

        try {
          const file = JSON.parse(data) as FileItem
          files.push(file)
          onItem(file)
        } catch {
          // skip malformed JSON
        }
      }
    }
  } finally {
    reader.releaseLock()
  }

  return { files }
}

export interface BackendPermissions {
  storage: boolean
}

export async function checkBackendPermissions(): Promise<BackendPermissions> {
  const baseUrl = getApiBaseUrl()
  const response = await fetch(`${baseUrl}/api/permissions`)
  if (!response.ok) {
    console.debug('[API] checkPermissions failed:', response.status)
    return { storage: false }
  }
  const result = await response.json()
  console.info('[API] permissions:', JSON.stringify(result))
  return result
}

export function getFileStreamUrl(path: string): string {
  if (import.meta.env.DEV) {
    return `/stream?path=${proxySafeEncode(path)}`
  }
  const baseUrl = getApiBaseUrl()
  return `${baseUrl}/stream?path=${proxySafeEncode(path)}`
}

export function getFilePreviewUrl(previewPage: string, filePath: string): string {
  if (import.meta.env.DEV) {
    return `/preview/${previewPage}?file=${proxySafeEncode(filePath)}`
  }
  const baseUrl = getApiBaseUrl()
  return `${baseUrl}/preview/${previewPage}?file=${proxySafeEncode(filePath)}`
}

export function getExternalStreamUrl(path: string): string {
  if (import.meta.env.DEV) {
    return `/api/stream/external?path=${proxySafeEncode(path)}`
  }
  const baseUrl = getApiBaseUrl()
  return `${baseUrl}/api/stream/external?path=${proxySafeEncode(path)}`
}

export async function checkServerStatus(): Promise<{
  online: boolean
  error?: string
  instanceId?: string
  version?: string
  /** 🆕 2026-06-15：backend instance_id 跨会话变化（后端真崩重启场景，不是劫持）。
   *  仍然 online=true（ping 200 + JSON + 合法 instance_id）；
   *  上层 useServerStatus emit 'backend:instance-changed' 给 UI banner 提示。 */
  instanceChanged?: { previous: string; current: string }
}> {
  try {
    // 🆕 2026-06-15：复用桌面后端 performPingCheck 的 InstanceID 防劫持机制。
    //
    // 历史：老代码只校验 Content-Type: application/json，但任何返 JSON 200 的进程
    //   都能骗过——mock / 旧 encv-go / 上游代理 / 编译时拼出来的"伪 backend" 都会
    //   被误判为 online。然后 token / 文件路径 / 任务数据就泄露给错误进程。
    //
    // 新逻辑（对齐 internal/register/server_start.go:89 performPingCheck）：
    //   1. 调 /ping（无副作用的探测端点，后端返 PingResponse{status, version, instance_id, server_dir, webdav_dir}）
    //   2. status code == 200 + content-type: application/json
    //   3. **decode** 响应体成 PingResponse（不能只看 status == "ok"）
    //   4. **instance_id 必填**：为空或缺字段 → 不是 encv-go → 报 hijacked
    //   5. **跨会话比对**：localStorage 缓存的 instance_id 不一致 → "进程被替换" → 报 hijacked
    //   6. 通过 → 持久化新的 instance_id + version 覆盖旧值（upgrade 场景：version 变了清缓存）
    //
    // 为什么不用 /api/config：
    //   - /api/config 返空 JSON 也能 200，老逻辑下无法判断 backend 是不是 encv-go
    //   - /ping 必有 instance_id，是 encv-go 唯一契约
    const baseUrl = getApiBaseUrl()
    const response = await fetch(`${baseUrl}/ping`, {
      // 强制不要缓存——instance_id 是 freshness 敏感的，HTTP 缓存可能让上一进程的 instance_id
      // 错认给当前进程
      cache: 'no-store',
      headers: { 'Accept': 'application/json' },
    })
    if (!response.ok) {
      return { online: false, error: `HTTP ${response.status}` }
    }
    const contentType = response.headers.get('content-type') || ''
    if (!contentType.includes('application/json')) {
      // Vite SPA fallback / 任何返回 text/html 的"假 backend"——老逻辑的 case 仍要拦截
      console.warn('[API] server probe returned non-JSON, treating as offline')
      return { online: false, error: `ping returned ${contentType || 'unknown'} (likely vite SPA fallback or wrong port)` }
    }
    const ping = await parsePingResponse(response)
    if (!ping) {
      return { online: false, error: 'ping response missing required fields (instance_id / status / version)' }
    }
    if (ping.status !== 'ok') {
      return { online: false, error: `ping status=${ping.status}` }
    }
    // 🆕 2026-06-15 修复 #1（死锁根因）：
    //
    // 历史 bug：旧逻辑 "比对失败就 return online:false 且不 persist"——后端真的崩重启后，
    //   新 instance_id 跟 localStorage 老 ID 不一致 → 报 online:false + error: 'instance changed'
    //   → 永远不 persist 新 ID → 下次探测还是不一致 → 死循环 offline
    //
    // 修复（顺序关键）：
    //   1. **先 persist 新 instance_id**（不管一不一样都 persist——重启用）
    //   2. 再比对——不一致时**仍然 return online:true**（ping 实质是 200 + JSON + 合法 instance_id）
    //   3. hijack 警告通过专用 `instanceChanged: {previous, current}` 字段返回，不靠 error
    //   4. 上层 useServerStatus 收到 instanceChanged 字段 → emit instanceChanged 事件
    //      → UI 顶部 banner 提示（不阻塞状态机 / 不进 lastError）
    const previousId = readPersistedInstanceId()
    persistBackendIdentity(ping.instance_id, ping.version) // ① 永远先 persist
    let instanceChanged: { previous: string; current: string } | undefined
    if (previousId && previousId !== ping.instance_id) {
      // ② 不一致 → 是 backend 真的崩重启（不是劫持；劫持场景下 ping 不会 200+JSON+合法 instance_id）
      console.warn('[API] backend instance_id changed (likely backend restart, not hijack)', {
        previous: previousId, current: ping.instance_id,
      })
      instanceChanged = { previous: previousId, current: ping.instance_id }
      // ③ 仍然 return online=true + 仅在 instanceChanged 字段带警告
      return {
        online: true,
        instanceId: ping.instance_id,
        version: ping.version,
        instanceChanged, // 上层用此发事件，UI 顶部 banner 提示
      }
    }
    console.info('[API] server online (ping OK)', { instanceId: shortHash(ping.instance_id), version: ping.version })
    return { online: true, instanceId: ping.instance_id, version: ping.version }
  } catch (e) {
    const msg = e instanceof Error ? e.message : String(e)
    console.debug('[API] server offline:', msg)
    return { online: false, error: msg }
  }
}

/** PingResponse 子集（snake_case 与后端 internal/v2/types/types.go PingResponse 一致） */
interface PingResponse {
  status: string
  version: string
  instance_id: string
  server_dir?: string
  webdav_dir?: string
}

/**
 * 安全 decode PingResponse。返回 null 表示"非 encv-go 响应"（字段缺失/类型错）。
 *
 * 不 throw：让调用方走 {online:false, error:...} 路径而不是 exception 路径。
 */
async function parsePingResponse(response: Response): Promise<PingResponse | null> {
  try {
    const obj = await response.json()
    if (!obj || typeof obj !== 'object') return null
    const status = typeof obj.status === 'string' ? obj.status : ''
    const version = typeof obj.version === 'string' ? obj.version : ''
    const instanceId = typeof obj.instance_id === 'string' ? obj.instance_id : ''
    if (!status || !instanceId) {
      // 关键字段缺失 → 不是 encv-go 响应
      return null
    }
    return {
      status,
      version,
      instance_id: instanceId,
      server_dir: typeof obj.server_dir === 'string' ? obj.server_dir : undefined,
      webdav_dir: typeof obj.webdav_dir === 'string' ? obj.webdav_dir : undefined,
    }
  } catch {
    return null
  }
}

function readPersistedInstanceId(): string | null {
  if (typeof localStorage === 'undefined') return null
  return localStorage.getItem(SERVER_INSTANCE_ID_KEY)
}

function persistBackendIdentity(instanceId: string, version: string) {
  if (typeof localStorage === 'undefined') return
  localStorage.setItem(SERVER_INSTANCE_ID_KEY, instanceId)
  if (version) localStorage.setItem(SERVER_VERSION_KEY, version)
}

/** 短 hash 显示（前 8 字符）——给 UI 展示用，避免完整 UUID 占太多视觉空间 */
function shortHash(id: string): string {
  if (!id) return '(empty)'
  return id.length > 8 ? id.slice(0, 8) : id
}

/** UI 读取持久化的 backend instance_id（用于"上次连接的是哪个进程"展示） */
export function getPersistedBackendIdentity(): { instanceId: string; version: string } | null {
  if (typeof localStorage === 'undefined') return null
  const id = localStorage.getItem(SERVER_INSTANCE_ID_KEY)
  if (!id) return null
  return { instanceId: id, version: localStorage.getItem(SERVER_VERSION_KEY) || '' }
}

export async function deleteFile(path: string): Promise<void> {
  // 🆕 2026-06-10 修复 #1：deleteFile 500 错误没有可读 message
  // 历史 bug：只 throw "HTTP error! status: 500" → 用户看到红色 toast 不知道为啥
  // 修复：把 response body 的 error 字段也读出来，throw 带详细 message
  console.debug('[API] deleteFile:', path)
  const baseUrl = getApiBaseUrl()
  const response = await fetch(`${baseUrl}/api/files?path=${proxySafeEncode(path)}`, {
    method: 'DELETE',
  })
  if (!response.ok) {
    // 4xx/5xx 都尝试读 JSON body（后端 writeServiceErrorGin 总是返回 {error: ...}）
    let detail = ''
    try {
      const data = await response.json()
      detail = data?.error || data?.message || ''
    } catch {
      // body 不是 JSON — 读 raw text
      try { detail = (await response.text()).slice(0, 200) } catch { /* noop */ }
    }
    const fullMsg = detail
      ? `[API] deleteFile failed: ${response.status} ${response.statusText} — ${detail}`
      : `[API] deleteFile failed: ${response.status} ${response.statusText}`
    console.error(fullMsg, { path, status: response.status })
    throw new Error(detail || `HTTP error! status: ${response.status}`)
  }
}

export async function createDirectory(parentPath: string, name: string): Promise<void> {
  console.info('[API] createDirectory:', parentPath, name)
  const response = await fetch(`${getApiBaseUrl()}/api/files/mkdir`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ parent_path: parentPath, name }),
  })
  if (!response.ok) {
    const data = await response.json().catch(() => ({}))
    throw new Error(data.error || `Failed to create directory (${response.status})`)
  }
}

export async function uploadFile(targetPath: string, file: File): Promise<FileItem> {
  console.info('[API] uploadFile:', targetPath, file.name, 'size:', file.size)
  const baseUrl = getApiBaseUrl()
  const formData = new FormData()
  formData.append('file', file)

  const response = await fetch(`${baseUrl}/api/files/upload?path=${proxySafeEncode(targetPath)}`, {
    method: 'POST',
    body: formData,
  })
  if (!response.ok) {
    let detail = `HTTP ${response.status}`
    try {
      const body = await response.text()
      if (body) detail += `: ${body}`
    } catch {}
    throw new Error(detail)
  }
  const result: FileItem = await response.json()
  console.info('[API] uploadFile success:', result.path, 'size:', result.size)
  return result
}

export interface ServiceGuardResult {
  ready: boolean
  servingDir: string
  expected: string
  envDevPreview?: boolean
  envMobile?: boolean
  detail?: string
  remediation?: Array<{ scenario: string; command?: string; steps?: string[]; explain?: string }>
  error?: string
}

export async function checkServiceGuard(): Promise<ServiceGuardResult> {
  console.info('[API] checkServiceGuard')
  const baseUrl = getApiBaseUrl()
  const response = await fetch(`${baseUrl}/api/service-guard`)
  const data: ServiceGuardResult = await response.json()

  if (!data.ready) {
    const err = new Error(`ServiceGuard: ${data.detail}`) as Error & { code: string; payload: ServiceGuardResult }
    err.code = 'SERVICE_GUARD_BLOCKED'
    err.payload = data
    console.error('[API] checkServiceGuard BLOCKED —', data.detail)
    throw err
  }

  console.info('[API] checkServiceGuard OK — servingDir:', data.servingDir)
  return data
}

export interface FileContentResponse {
  name: string
  path: string
  size: number
  content: string
  encoding: string
}

export async function readFileContent(path: string): Promise<FileContentResponse> {
  const baseUrl = getApiBaseUrl()
  const response = await fetch(`${baseUrl}/api/file?path=${proxySafeEncode(path)}`)
  if (!response.ok) {
    console.error('[API] readFileContent failed:', response.status)
    throw new Error(`HTTP error! status: ${response.status}`)
  }
  const data = await response.json()
  console.info('[API] readFileContent:', path, 'size:', data.size)
  return data
}

export type TaskType = 'encrypt' | 'decrypt'
export type TaskStatus = 'queued' | 'running' | 'completed' | 'failed' | 'cancelled' | 'cancelling'

export interface TaskStep {
  phase: string
  startedAt: string
  completedAt?: string
  detail?: string
}

export interface EncvTask {
  id: string
  type: TaskType
  sourcePath: string
  targetPath?: string
  pluginName?: string
  status: TaskStatus
  progress: number
  phase?: string
  speed?: string
  eta?: string
  error?: string
  errorDetail?: string
  warning?: string
  warningDetail?: string
  containerVersion?: number
  outputPath?: string
  steps?: TaskStep[]
  createdAt: string
  completedAt?: string
  // 🆕 2026-06-10 修复 v4：triggeredBy + runId 直接放 task 对象上
  // 历史：分组依赖 localStorage.useTaskTrigger，跨 session / localStorage 清空后全失效
  //   → 「任务组只在一开始的时候正确显示」+「插件没正确识别，任务依旧全部平铺」
  // 修复：这两个字段在 submitAction 返回时就写到 task 对象上，不再只存 localStorage
  //   - 当前 session：直接读 t.triggeredBy / t.runId（O(1) 内存访问）
  //   - 跨 session：localStorage 作 fallback（旧 task 没有这 2 字段，try useTaskTrigger）
  triggeredBy?: 'user' | 'automation' | 'ai_agent'
  runId?: string
}

export async function getTasks(): Promise<EncvTask[]> {
  const baseUrl = getApiBaseUrl()
  const response = await fetch(`${baseUrl}/api/tasks`)
  if (!response.ok) {
    throw new Error(`HTTP error! status: ${response.status}`)
  }
  const data = await response.json()
  return data.tasks || []
}

/**
 * 🆕 2026-06-16：拉取后端 ring buffer 最近 N 条日志
 *   - http-poll 模式：每次 tick 拉一次
 *   - WS 模式：onMounted 冷启动时拉一次历史（WS 推的实时日志不补历史）
 *   - since 参数：增量拉取（时间戳字符串，HH:MM:SS 格式）
 */
export interface BackendLogEntry {
  level: 'debug' | 'info' | 'warn' | 'error'
  message: string
  timestamp: string  // HH:MM:SS
}
export interface RecentLogsResponse {
  logs: BackendLogEntry[]
  count: number
  capacity: number
}

export async function getRecentBackendLogs(since?: string): Promise<RecentLogsResponse> {
  const baseUrl = getApiBaseUrl()
  const url = since
    ? `${baseUrl}/api/logs/recent?since=${encodeURIComponent(since)}`
    : `${baseUrl}/api/logs/recent`
  const response = await fetch(url)
  if (!response.ok) {
    throw new Error(`HTTP error! status: ${response.status}`)
  }
  return response.json()
}

export async function createTask(
  type: TaskType,
  sourcePath: string,
  targetPath?: string,
  password?: string,
  version?: number,
  pluginName?: string,
  extraFields?: Record<string, string>,
  secondaryPassword?: string,
  cipherMode?: number,
  compressionMode?: 'none' | 'zstd',
): Promise<EncvTask> {
  console.info('[API] createTask:', type, sourcePath, targetPath || '',
    'hasPassword:', !!password, 'version:', version ?? 'default',
    'pluginName:', pluginName ?? 'auto',
    'hasExtraFields:', extraFields && Object.keys(extraFields).length > 0,
    'hasSecondaryPassword:', !!secondaryPassword,
    'cipherMode:', cipherMode ?? 0,
    'compressionMode:', compressionMode ?? 'none')
  const baseUrl = getApiBaseUrl()
  const body: Record<string, unknown> = { type, sourcePath }
  if (targetPath) body.targetPath = targetPath
  if (password) body.password = password
  if (version) body.version = version
  if (pluginName) body.pluginName = pluginName
  if (extraFields && Object.keys(extraFields).length > 0) body.extraFields = extraFields
  if (secondaryPassword) body.secondaryPassword = secondaryPassword
  if (cipherMode !== undefined) body.cipherMode = cipherMode
  if (compressionMode !== undefined) body.compressionMode = compressionMode
  const response = await fetch(`${baseUrl}/api/tasks`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })
  if (!response.ok) {
    throw new Error(`HTTP error! status: ${response.status}`)
  }
  return await response.json()
}

export async function cancelTask(id: string): Promise<void> {
  const baseUrl = getApiBaseUrl()
  const response = await fetch(`${baseUrl}/api/tasks/${id}/cancel`, {
    method: 'POST',
  })
  if (!response.ok) {
    throw new Error(`HTTP error! status: ${response.status}`)
  }
}

export async function retryTask(id: string): Promise<void> {
  const baseUrl = getApiBaseUrl()
  const response = await fetch(`${baseUrl}/api/tasks/${id}/retry`, {
    method: 'POST',
  })
  if (!response.ok) {
    throw new Error(`HTTP error! status: ${response.status}`)
  }
}

export async function removeTask(id: string): Promise<void> {
  console.info('[API] removeTask:', id)
  const baseUrl = getApiBaseUrl()
  const response = await fetch(`${baseUrl}/api/tasks/${id}`, {
    method: 'DELETE',
  })
  if (!response.ok) {
    throw new Error(`HTTP error! status: ${response.status}`)
  }
}

export async function clearCompletedTasks(): Promise<{ removed: number }> {
  console.info('[API] clearCompletedTasks')
  const baseUrl = getApiBaseUrl()
  const response = await fetch(`${baseUrl}/api/tasks`, {
    method: 'DELETE',
  })
  if (!response.ok) {
    throw new Error(`HTTP error! status: ${response.status}`)
  }
  const data = await response.json()
  return { removed: data.removed ?? 0 }
}

export interface WebDAVConfig {
  id: string
  name: string
  url: string
  username: string
  password: string
  mountPath: string
  isBuiltIn?: boolean
}

export interface RemoteWebDAVInfo {
  enabled: boolean
  url: string
  username: string
  root: string
}

export interface OpenlistSiteInfo {
  host: string
  description: string
  proxyUrl: string
  isBuiltIn?: boolean
}

export interface RemoteInfo {
  webdav: RemoteWebDAVInfo
  openlistSites: Record<string, OpenlistSiteInfo>
}

export type LocalOpenListState = 'not_installed' | 'port_conflict' | 'running' | 'stopped'

export interface LocalOpenListStatus {
  state: LocalOpenListState
  running: boolean
  pid: number
  port: number
  dataDirSize: number
  lastHeartbeat: number
  error?: string
}

export async function fetchLocalOpenListStatus(): Promise<LocalOpenListStatus> {
  console.debug('[API] fetchLocalOpenListStatus')
  const baseUrl = getApiBaseUrl()
  const response = await fetch(`${baseUrl}/openlist/local/status`)
  if (!response.ok) {
    console.error('[API] fetchLocalOpenListStatus failed:', response.status)
    throw new Error(`HTTP error! status: ${response.status}`)
  }
  return await response.json()
}

const WEBDAV_CONFIGS_KEY = 'encv-webdav-configs'

export function getWebDAVConfigs(): WebDAVConfig[] {
  const stored = localStorage.getItem(WEBDAV_CONFIGS_KEY)
  return stored ? JSON.parse(stored) : []
}

export function saveWebDAVConfigs(configs: WebDAVConfig[]) {
  localStorage.setItem(WEBDAV_CONFIGS_KEY, JSON.stringify(configs))
}

export interface LocalWebDAVTestResult {
  available: boolean
  url?: string
  authRequired?: boolean
  details?: {
    propfindRoot: string
    authWorks: string
    dirReadable: string
  }
  error?: string
}

export async function testLocalWebDAV(): Promise<LocalWebDAVTestResult> {
  const baseUrl = getApiBaseUrl()
  const response = await fetch(`${baseUrl}/api/webdav/test-local`)
  if (!response.ok) {
    throw new Error(`HTTP error! status: ${response.status}`)
  }
  return await response.json()
}

export interface WebDAVTestResult {
  success: boolean
  reachable: boolean
  is_webdav: boolean
  auth_ok: boolean
  dir_readable: boolean
  status_code: number
  dav_header?: string
  error?: string
}

export async function testWebDAVConnection(config: Omit<WebDAVConfig, 'id'>): Promise<WebDAVTestResult> {
  console.info('[API] testWebDAV')
  const baseUrl = getApiBaseUrl()
  const response = await fetch(`${baseUrl}/api/webdav/test`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(config),
  })
  if (!response.ok) {
    let detail = `HTTP ${response.status}`
    try {
      const body = await response.text()
      if (body) detail += `: ${body}`
    } catch {}
    throw new Error(detail)
  }
  const data = await response.json()
  if (data.success === false) {
    const result = data as WebDAVTestResult
    return result
  }
  return data as WebDAVTestResult
}

export function formatFileSize(bytes?: number): string {
  if (bytes === undefined || bytes === null) return ''
  if (bytes === 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  const k = 1024
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return `${parseFloat((bytes / Math.pow(k, i)).toFixed(1))} ${units[i]}`
}

export interface TextPreviewExts {
  extensions: string[]
  custom_extensions: string[]
}

let cachedTextExts: Set<string> | null = null

export async function fetchTextPreviewExts(): Promise<Set<string>> {
  if (cachedTextExts) return cachedTextExts
  const baseUrl = getApiBaseUrl()
  const controller = new AbortController()
  const timer = setTimeout(() => controller.abort(), 5000)
  try {
    const response = await fetch(`${baseUrl}/api/file/text-preview-exts`, { signal: controller.signal })
    if (!response.ok) {
      console.error('[API] fetchTextPreviewExts failed:', response.status)
      return new Set()
    }
    const data = await response.json() as TextPreviewExts
    const all = new Set([...(data.extensions || []), ...(data.custom_extensions || [])])
    cachedTextExts = all
    return all
  } catch (err: any) {
    if (err?.name === 'AbortError') {
      console.debug('[API] fetchTextPreviewExts timed out after 5s')
    } else {
      console.error('[API] fetchTextPreviewExts error:', err)
    }
    return new Set()
  } finally {
    clearTimeout(timer)
  }
}

export function isTextPreviewable(name: string): boolean {
  if (!cachedTextExts) return false
  const ext = getFileExtension(name)
  return cachedTextExts.has(ext)
}

export function invalidateTextExtsCache(): void {
  cachedTextExts = null
}

export function getFileExtension(name: string): string {
  const lastDot = name.lastIndexOf('.')
  if (lastDot === -1) return ''
  return name.substring(lastDot + 1).toLowerCase()
}

export type FileCategory = 'video' | 'audio' | 'image' | 'document' | 'other'

export function getFileCategory(name: string): FileCategory {
  const ext = getFileExtension(name)
  const videoExts = ['mp4', 'mkv', 'avi', 'mov', 'wmv', 'flv', 'webm', 'm4v']
  const audioExts = ['mp3', 'flac', 'wav', 'aac', 'ogg', 'wma', 'm4a']
  const imageExts = ['jpg', 'jpeg', 'png', 'gif', 'webp', 'bmp', 'svg']
  const docExts = ['pdf', 'doc', 'docx', 'txt', 'xls', 'xlsx', 'ppt', 'pptx']

  if (videoExts.includes(ext)) return 'video'
  if (audioExts.includes(ext)) return 'audio'
  if (imageExts.includes(ext)) return 'image'
  if (docExts.includes(ext)) return 'document'
  return 'other'
}

export async function fetchConfig(): Promise<Record<string, unknown>> {
  const baseUrl = getApiBaseUrl()
  const response = await fetch(`${baseUrl}/api/config`)
  if (!response.ok) {
    let detail = `HTTP ${response.status}`
    try {
      const body = await response.text()
      if (body) detail += `: ${body.slice(0, 200)}`
    } catch {}
    throw new Error(detail)
  }
  // 关键健壮性：vite dev SPA fallback 对未匹配的路径返回 index.html
  // （即 <!DOCTYPE html>...），如果响应是 HTML 而不是 JSON，
  // 说明 baseUrl 配错或请求被错误地路由到了 vite dev server。
  // 抛出明确错误，避免上游 JSON.parse 报 "Unexpected token '<'"。
  const contentType = response.headers.get('content-type') || ''
  if (!contentType.includes('application/json')) {
    const snippet = (await response.text()).slice(0, 200)
    throw new Error(
      `fetchConfig: response is not JSON (content-type: "${contentType}"). ` +
      `This usually means /api is being routed to vite dev SPA fallback instead of the Go backend. ` +
      `Use the preview-gateway :16666 entry, not vite :8100 directly. ` +
      `Body: ${snippet}`,
    )
  }
  return await response.json()
}

export async function updateConfig(config: Record<string, unknown>): Promise<{ message: string; needsRestart?: boolean }> {
  const baseUrl = getApiBaseUrl()
  const response = await fetch(`${baseUrl}/api/config`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(config),
  })
  if (!response.ok) {
    let detail = `HTTP ${response.status}`
    try {
      const body = await response.text()
      if (body) detail += `: ${body}`
    } catch {}
    throw new Error(detail)
  }
  try {
    return await response.json()
  } catch {
    return { message: 'config updated' }
  }
}

export async function fetchConfigSchema(): Promise<Record<string, unknown>> {
  const baseUrl = getApiBaseUrl()
  const response = await fetch(`${baseUrl}/api/config/schema`)
  if (!response.ok) {
    throw new Error(`HTTP error! status: ${response.status}`)
  }
  return await response.json()
}

export async function searchFiles(path: string, keyword: string, recursive = false): Promise<FileItem[]> {
  const baseUrl = getApiBaseUrl()
  const response = await fetch(`${baseUrl}/api/files/search?path=${proxySafeEncode(path)}&keyword=${encodeURIComponent(keyword)}&recursive=${recursive}`)
  if (!response.ok) {
    if (response.status === 403) {
      const data = await response.json().catch(() => ({}))
      if (data.code === 'PERMISSION_DENIED') {
        throw new PermissionDeniedError(data.error || 'Permission denied')
      }
    }
    throw new Error(`HTTP error! status: ${response.status}`)
  }
  const data = await response.json()
  return data.files || []
}

export interface IndexStats {
  totalFiles: number
  totalDirs: number
  totalSize: number
  indexedAt: string
  isIndexing: boolean
  lastBuildMs: number
  source?: string
  containers?: number
}

export async function getIndexStats(): Promise<IndexStats> {
  const baseUrl = getApiBaseUrl()
  const response = await fetch(`${baseUrl}/api/index/stats`)
  if (!response.ok) {
    throw new Error(`HTTP error! status: ${response.status}`)
  }
  return await response.json()
}

export async function rebuildIndex(): Promise<void> {
  const baseUrl = getApiBaseUrl()
  const response = await fetch(`${baseUrl}/api/index/rebuild`, { method: 'POST' })
  if (!response.ok) {
    throw new Error(`HTTP error! status: ${response.status}`)
  }
}

export async function clearIndex(): Promise<void> {
  const baseUrl = getApiBaseUrl()
  const response = await fetch(`${baseUrl}/api/index/clear`, { method: 'POST' })
  if (!response.ok) {
    throw new Error(`HTTP error! status: ${response.status}`)
  }
}

export async function fetchRemoteInfo(): Promise<RemoteInfo> {
  const baseUrl = getApiBaseUrl()
  const response = await fetch(`${baseUrl}/api/remote/info`)
  if (!response.ok) {
    throw new Error(`HTTP error! status: ${response.status}`)
  }
  return await response.json()
}

export async function addOpenlistSite(siteId: string, host: string, description: string): Promise<void> {
  const baseUrl = getApiBaseUrl()
  const response = await fetch(`${baseUrl}/api/remote/openlist`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ siteId, host, description }),
  })
  if (!response.ok) {
    const data = await response.json().catch(() => ({}))
    throw new Error(data.error || `HTTP ${response.status}`)
  }
}

export async function updateOpenlistSite(siteId: string, host: string, description: string): Promise<void> {
  const baseUrl = getApiBaseUrl()
  const response = await fetch(`${baseUrl}/api/remote/openlist/${siteId}`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ host, description }),
  })
  if (!response.ok) {
    const data = await response.json().catch(() => ({}))
    throw new Error(data.error || `HTTP ${response.status}`)
  }
}

export async function deleteOpenlistSite(siteId: string): Promise<void> {
  const baseUrl = getApiBaseUrl()
  const response = await fetch(`${baseUrl}/api/remote/openlist/${siteId}`, {
    method: 'DELETE',
  })
  if (!response.ok) {
    const data = await response.json().catch(() => ({}))
    throw new Error(data.error || `HTTP ${response.status}`)
  }
}

export async function checkFileExists(path: string): Promise<boolean> {
  const baseUrl = getApiBaseUrl()
  const response = await fetch(`${baseUrl}/api/files/exists?path=${proxySafeEncode(path)}`)
  if (!response.ok) {
    console.debug('[API] checkFileExists failed:', response.status)
    return false
  }
  const data = await response.json()
  return !!data.exists
}

export async function checkEncryptOutputExists(sourcePath: string, targetDir?: string): Promise<{ exists: boolean; outputPath: string }> {
  const baseUrl = getApiBaseUrl()
  let url = `${baseUrl}/api/files/encrypt-output-exists?sourcePath=${proxySafeEncode(sourcePath)}`
  if (targetDir) url += `&targetDir=${proxySafeEncode(targetDir)}`
  const response = await fetch(url)
  if (!response.ok) {
    console.debug('[API] checkEncryptOutputExists failed:', response.status)
    return { exists: false, outputPath: '' }
  }
  const data = await response.json()
  return { exists: !!data.exists, outputPath: data.outputPath || '' }
}

export interface FFmpegStatus {
  ffmpeg_available: boolean
  ffprobe_available: boolean
  error?: string
  ffmpeg_detail?: string
  ffprobe_detail?: string
}

export async function fetchFFmpegStatus(): Promise<FFmpegStatus> {
  const baseUrl = getApiBaseUrl()
  const response = await fetch(`${baseUrl}/api/ffmpeg-status`)
  if (!response.ok) {
    throw new Error(`HTTP error! status: ${response.status}`)
  }
  return await response.json()
}

export interface BuildInfo {
  ffmpeg_version: string
  ffmpeg_codename: string
  ndk_version: string
  api_level: number
  abi: string
  build_date: string
  enabled_decoders: string[]
  enabled_encoders: string[]
  enabled_muxers: string[]
  enabled_demuxers: string[]
  enabled_parsers: string[]
  enabled_protocols: string[]
  enabled_filters: string[]
  static_libs: string[]
  linking: string
  cflags: string
  ffmpeg_license: string
  app_version?: string
}

export async function fetchBuildInfo(): Promise<BuildInfo> {
  const baseUrl = getApiBaseUrl()
  const response = await fetch(`${baseUrl}/api/build-info`)
  if (!response.ok) {
    throw new Error(`HTTP error! status: ${response.status}`)
  }
  return await response.json()
}

export interface ContainerVersionInfo {
  version: number
  status: 'deprecated' | 'stable' | 'recommended'
  label: string
}

export interface ContainerVersionsResponse {
  versions: ContainerVersionInfo[]
  default: number
}

export async function fetchContainerVersions(): Promise<ContainerVersionsResponse> {
  const baseUrl = getApiBaseUrl()
  const response = await fetch(`${baseUrl}/api/container/versions`)
  if (!response.ok) throw new Error('Failed to fetch container versions')
  return response.json()
}

export interface WebDavLocalInfo {
  enabled: boolean
  authRequired: boolean
  username: string
  password: string
  webdavPath: string
  serverBaseUrl: string
}

/**
 * 拉取后端本地 webdav endpoint 元信息（账号/密码/是否启用）
 * 用于构造 Basic Auth header，避免触发浏览器 401 弹窗
 */
export async function fetchWebDavLocalInfo(): Promise<WebDavLocalInfo> {
  const baseUrl = getApiBaseUrl()
  const response = await fetch(`${baseUrl}/api/webdav/local-info`)
  if (!response.ok) throw new Error(`Failed to fetch webdav local info: ${response.status}`)
  return response.json() as Promise<WebDavLocalInfo>
}

export type DecryptErrorCode = 'wrong_password' | 'data_corrupted' | 'decrypt_failed' | 'deprecated_version'

export interface DecryptError {
  error: DecryptErrorCode
  message: string
}

export function isWrongPasswordError(error: unknown): boolean {
  if (error && typeof error === 'object' && 'error' in error) {
    return (error as DecryptError).error === 'wrong_password'
  }
  const msg = String(error).toLowerCase()
  return msg.includes('wrong password') || msg.includes('密码')
}

export async function renameFile(oldPath: string, newName: string): Promise<void> {
  console.info('[API] renameFile:', oldPath, '→', newName)
  const baseUrl = getApiBaseUrl()
  const response = await fetch(`${baseUrl}/api/file/rename`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ oldPath, newName }),
  })
  if (!response.ok) {
    console.error('[API] renameFile failed:', response.status)
    throw new Error(`HTTP error! status: ${response.status}`)
  }
}

export interface RenameOriginalNameResponse {
  success: boolean
  display_name: string
  error?: string
}

export async function renameOriginalName(path: string, newName: string, password?: string): Promise<RenameOriginalNameResponse> {
  console.info('[API] renameOriginalName:', path, '→', newName, 'hasPassword:', !!password)
  const baseUrl = getApiBaseUrl()
  const response = await fetch(`${baseUrl}/api/file/rename`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ path, new_name: newName, ...(password ? { password } : {}) }),
  })
  if (!response.ok) {
    const data = await response.json().catch(() => ({}))
    console.error('[API] renameOriginalName failed:', response.status, data.error)
    throw new Error(data.error || `HTTP error! status: ${response.status}`)
  }
  return response.json()
}

export async function copyFile(srcPath: string, destPath: string): Promise<void> {
  console.info('[API] copyFile:', srcPath, '→', destPath)
  const baseUrl = getApiBaseUrl()
  const response = await fetch(`${baseUrl}/api/file/copy`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ srcPath, destPath }),
  })
  if (!response.ok) {
    console.error('[API] copyFile failed:', response.status)
    throw new Error(`HTTP error! status: ${response.status}`)
  }
}

export async function moveFile(srcPath: string, destPath: string): Promise<void> {
  console.info('[API] moveFile:', srcPath, '→', destPath)
  const baseUrl = getApiBaseUrl()
  const response = await fetch(`${baseUrl}/api/file/move`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ srcPath, destPath }),
  })
  if (!response.ok) {
    console.error('[API] moveFile failed:', response.status)
    throw new Error(`HTTP error! status: ${response.status}`)
  }
}

export interface PluginMeta {
  name: string
  supportedExtensions: string[]
  supportedMimePrefixes: string[]
  containerExtension: string
  taskOptions: TaskOptions
}

export type PasswordStrategy = 'global' | 'independent' | 'none'

export interface TaskField {
  key: string
  label: string
  type: 'string' | 'password' | 'select' | 'bool'
  required: boolean
  defaultValue: string
  help: string
  options?: string[]
  optionLabels?: Record<string, string>
  condition?: '' | 'encrypt' | 'decrypt'
}

export interface TaskOptions {
  passwordStrategy: PasswordStrategy
  supportVersionSelect: boolean
  supportedVersions: number[] | null
  defaultVersion: number
  extraFields: TaskField[]
}

export interface PluginCandidate {
  name: string
  matchType: 'mime' | 'extension' | 'general' | 'container'
  priority: number
  taskOptions: TaskOptions | null
}

export interface PredictPluginResponse {
  candidates: PluginCandidate[]
  pluginName: string | null
  error?: string
  taskOptions: TaskOptions | null
}

export async function fetchPlugins(): Promise<PluginMeta[]> {
  const baseUrl = getApiBaseUrl()
  const response = await fetch(`${baseUrl}/api/plugins`)
  if (!response.ok) {
    console.error('[API] fetchPlugins failed:', response.status)
    throw new Error(`HTTP error! status: ${response.status}`)
  }
  const data = await response.json()
  console.info('[API] fetchPlugins:', data.plugins?.length || 0, 'plugins')
  return data.plugins || []
}

export interface TagInfo {
  name: string
  count: number
}

export async function fetchTags(): Promise<TagInfo[]> {
  const baseUrl = getApiBaseUrl()
  const response = await fetch(`${baseUrl}/api/files/tags`)
  if (!response.ok) {
    console.error('[API] fetchTags failed:', response.status)
    throw new Error(`HTTP error! status: ${response.status}`)
  }
  const data = await response.json()
  console.info('[API] fetchTags:', data.tags?.length || 0, 'tags')
  return data.tags || []
}

export async function addTag(path: string, tag: string): Promise<void> {
  console.info('[API] addTag:', path, tag)
  const baseUrl = getApiBaseUrl()
  const response = await fetch(`${baseUrl}/api/files/tags`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ path, tag, action: 'add' }),
  })
  if (!response.ok) {
    console.error('[API] addTag failed:', response.status)
    throw new Error(`HTTP error! status: ${response.status}`)
  }
}

export async function removeTag(path: string, tag: string): Promise<void> {
  console.info('[API] removeTag:', path, tag)
  const baseUrl = getApiBaseUrl()
  const response = await fetch(`${baseUrl}/api/files/tags`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ path, tag, action: 'remove' }),
  })
  if (!response.ok) {
    console.error('[API] removeTag failed:', response.status)
    throw new Error(`HTTP error! status: ${response.status}`)
  }
}

export async function listFilesByTag(tag: string, path?: string): Promise<FileItem[]> {
  const baseUrl = getApiBaseUrl()
  const response = await fetch(`${baseUrl}/api/files?path=${proxySafeEncode(path || '/')}&tag=${encodeURIComponent(tag)}`)
  if (!response.ok) {
    console.error('[API] listFilesByTag failed:', response.status)
    throw new Error(`HTTP error! status: ${response.status}`)
  }
  const data: FileListResponse = await response.json()
  console.info('[API] listFilesByTag:', tag, '→', data.files?.length || 0, 'files')
  return data.files || []
}

export function getAlistEncryptStreamUrl(params: { path: string; password: string }): string {
  // 注意：path 用单次 encodeURIComponent（不是 proxySafeEncode）。
  // 双重编码（proxySafeEncode）是为经过 WAF / 代理的场景，而 alist-encrypt
  // stream 端点会自行解码一次，单编码才是正确的客户端编码层次。
  if (import.meta.env.DEV) {
    return `/api/alist-encrypt/stream?path=${encodeURIComponent(params.path)}&password=${encodeURIComponent(params.password)}`
  }
  const baseUrl = getApiBaseUrl()
  return `${baseUrl}/api/alist-encrypt/stream?path=${encodeURIComponent(params.path)}&password=${encodeURIComponent(params.password)}`
}

export interface AlistDecodeResult {
  plain_name: string
  success: boolean
}

export async function decodeAlistFilename(params: { encodedName: string; password: string; encType?: string }): Promise<AlistDecodeResult> {
  const baseUrl = getApiBaseUrl()
  const urlParams = new URLSearchParams({
    encoded: params.encodedName,
    password: params.password,
  })
  if (params.encType) urlParams.set('enc_type', params.encType)
  const response = await fetch(`${baseUrl}/api/alist-encrypt/decode-filename?${urlParams}`)
  if (!response.ok) {
    console.error('[API] decodeAlistFilename failed:', response.status)
    throw new Error(`HTTP error! status: ${response.status}`)
  }
  return response.json()
}

export interface ContainerExtensionConflict {
  extension: string
  pluginNames: string[]
}

export interface ContainerExtensionsResponse {
  extensions: Record<string, string>
  conflicts: ContainerExtensionConflict[]
}

export async function fetchContainerExtensions(): Promise<ContainerExtensionsResponse> {
  const baseUrl = getApiBaseUrl()
  const response = await fetch(`${baseUrl}/api/plugins/container-extensions`)
  if (!response.ok) {
    console.error('[API] fetchContainerExtensions failed:', response.status)
    throw new Error(`HTTP error! status: ${response.status}`)
  }
  return response.json()
}

export async function predictPlugin(
  sourcePath: string,
  type: TaskType
): Promise<PredictPluginResponse> {
  const baseUrl = getApiBaseUrl()
  const response = await fetch(`${baseUrl}/api/tasks/predict-plugin`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ sourcePath, type }),
  })
  if (!response.ok) {
    console.error('[API] predictPlugin failed:', response.status)
    throw new Error(`HTTP error! status: ${response.status}`)
  }
  return response.json()
}

// 🆕 2026-06-15 multi-mount (spec Phase E)：挂载点管理 API
//
// 后端路由：internal/server/mount_api.go
// 数据模型：internal/mount/mount.go (Mount)
//
// 端点：
//   GET    /api/mounts               → listMounts / ListMountsResponse
//   GET    /api/mounts/:id           → getMount
//   POST   /api/mounts               → createMount
//   PUT    /api/mounts/:id           → updateMount
//   DELETE /api/mounts/:id           → deleteMount
//   POST   /api/mounts/:id/resolve   → resolveMountPath
//   GET    /api/mounts/:id/usage     → fetchMountUsage

/** Mount 挂载点数据模型。与后端 mount.Mount 字段一一对应（snake_case）。 */
export interface Mount {
  id: string
  name: string
  mount_path: string
  driver: string
  root_path: string
  enabled: boolean
  read_only: boolean
  driver_config?: Record<string, unknown>
  created_at?: string
  updated_at?: string
}

/** 预置 driver 名常量（与后端 mount.DriverLocal/AppData/Sandbox 一致）。 */
export const MOUNT_DRIVERS = ['local', 'appdata', 'sandbox'] as const
export type MountDriver = (typeof MOUNT_DRIVERS)[number]

/** 预置 mount name 提示常量（仅用于 UI 提示，非后端强制）。 */
export const MOUNT_PRESET_NAMES = ['primary', 'automation', 'sandbox'] as const

export interface ListMountsResponse {
  mounts: Mount[]
  drivers: string[]
}

export interface ResolveMountResponse {
  virtual_path: string
  abs_path: string
  rel_path: string
  mount_name: string
}

export interface MountUsageResponse {
  mount_id: string
  root_path: string
  entry_count: number
}

/** 通用 mount 错误格式化。 */
async function readMountError(response: Response, op: string): Promise<string> {
  let detail = `HTTP ${response.status}`
  try {
    const body = await response.text()
    if (body) {
      // 尝试解析 {"error": "..."}
      try {
        const parsed = JSON.parse(body)
        if (parsed?.error) detail = parsed.error
        else detail = body.slice(0, 200)
      } catch {
        detail = body.slice(0, 200)
      }
    }
  } catch {}
  return `${op} failed: ${detail}`
}

export async function listMounts(): Promise<ListMountsResponse> {
  const baseUrl = getApiBaseUrl()
  const response = await fetch(`${baseUrl}/api/mounts`)
  if (!response.ok) {
    const msg = await readMountError(response, 'listMounts')
    console.error('[API]', msg)
    throw new Error(msg)
  }
  return response.json()
}

export async function getMount(id: string): Promise<Mount> {
  const baseUrl = getApiBaseUrl()
  const response = await fetch(`${baseUrl}/api/mounts/${encodeURIComponent(id)}`)
  if (!response.ok) {
    const msg = await readMountError(response, 'getMount')
    console.error('[API]', msg)
    throw new Error(msg)
  }
  return response.json()
}

/** Create / Update 通用 body 字段（不含 id / created_at / updated_at）。 */
export interface MountInput {
  name: string
  mount_path: string
  driver: string
  enabled: boolean
  read_only: boolean
  driver_config?: Record<string, unknown>
}

export async function createMount(input: MountInput): Promise<Mount> {
  const baseUrl = getApiBaseUrl()
  const response = await fetch(`${baseUrl}/api/mounts`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(input),
  })
  if (!response.ok) {
    const msg = await readMountError(response, 'createMount')
    console.error('[API]', msg)
    throw new Error(msg)
  }
  return response.json()
}

export async function updateMount(id: string, input: MountInput): Promise<Mount> {
  const baseUrl = getApiBaseUrl()
  const response = await fetch(`${baseUrl}/api/mounts/${encodeURIComponent(id)}`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(input),
  })
  if (!response.ok) {
    const msg = await readMountError(response, 'updateMount')
    console.error('[API]', msg)
    throw new Error(msg)
  }
  return response.json()
}

export async function deleteMount(id: string): Promise<void> {
  const baseUrl = getApiBaseUrl()
  const response = await fetch(`${baseUrl}/api/mounts/${encodeURIComponent(id)}`, {
    method: 'DELETE',
  })
  // 204 No Content 视为成功；其他都抛错
  if (response.status === 204) return
  if (!response.ok) {
    const msg = await readMountError(response, 'deleteMount')
    console.error('[API]', msg)
    throw new Error(msg)
  }
}

export async function resolveMountPath(id: string, subPath: string): Promise<ResolveMountResponse> {
  const baseUrl = getApiBaseUrl()
  const response = await fetch(`${baseUrl}/api/mounts/${encodeURIComponent(id)}/resolve`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ sub_path: subPath }),
  })
  if (!response.ok) {
    const msg = await readMountError(response, 'resolveMountPath')
    console.error('[API]', msg)
    throw new Error(msg)
  }
  return response.json()
}

export async function fetchMountUsage(id: string): Promise<MountUsageResponse> {
  const baseUrl = getApiBaseUrl()
  const response = await fetch(`${baseUrl}/api/mounts/${encodeURIComponent(id)}/usage`)
  if (!response.ok) {
    const msg = await readMountError(response, 'fetchMountUsage')
    console.error('[API]', msg)
    throw new Error(msg)
  }
  return response.json()
}
