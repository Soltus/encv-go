import { getApiBaseUrl, proxySafeEncode } from './encv_core'

// encv_files.ts - 拆分自 encv.ts



export interface FileItem {
  name: string
  display_name?: string
  path: string
  isDirectory: boolean
  isEncrypted?: boolean
  size?: number
  modified?: string
  /** 向量搜索相关度分数（0-1，越大越相似）。仅 /api/search/files 返回时填充。 */
  score?: number
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

export async function deleteFile(path: string): Promise<{ taskId: string }> {
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
  return response.json()
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

/**
 * 🆕 v6 2026-06-22：单文件 metadata 查询（file:change 增量更新用）
 *   - 调 /api/file/info?path=... 拿单个文件的 FileItem（含 size/modified/isDirectory/isEncrypted）
 *   - 404 → 抛 NotFoundError（调用方据此从 files.value 移除）
 *   - 用于 file:change action=create|modify 的增量更新，避免全量 listFiles
 */

export async function getFileInfo(path: string): Promise<FileItem> {
  const baseUrl = getApiBaseUrl()
  const response = await fetch(`${baseUrl}/api/file/info?path=${proxySafeEncode(path)}`)
  if (!response.ok) {
    if (response.status === 404) {
      throw new NotFoundError('File not found')
    }
    throw new Error(`HTTP error! status: ${response.status}`)
  }
  const data = await response.json()
  return {
    name: data.name,
    display_name: data.display_name,
    path: data.path,
    isDirectory: data.is_directory,
    isEncrypted: data.is_encrypted,
    size: data.size,
    modified: data.modified,
  }
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

export function formatFileSize(bytes?: number): string {
  if (bytes === undefined || bytes === null) return ''
  if (bytes === 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  const k = 1024
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return `${parseFloat((bytes / Math.pow(k, i)).toFixed(1))} ${units[i]}`
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
