import { TaskType } from './encv_tasks'

import { FileItem, FileListResponse } from './encv_files'

import { getApiBaseUrl, proxySafeEncode } from './encv_core'

// encv_plugins.ts - 拆分自 encv.ts



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
  /**
   * 🆕 2026-06-16：mount 启动期错误（不再静默）
   * - 后端 server.go 在 MigrateFromServingDir 失败时 append 到 s.mountBootstrapErrors
   * - /api/mounts 响应里暴露
   * - MountsDetail.vue 顶部 banner 展示，每条对应一个 mount 启动失败原因
   * - 典型场景：mounts.json 损坏 / bootstrap 写盘失败 / driver 工厂 panic
   */
  bootstrap_errors: string[]
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

// ========== 🆕 性能指标 ==========

/** 性能指标摘要（task:completed 事件推送） */
