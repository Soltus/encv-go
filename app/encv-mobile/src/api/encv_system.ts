import { getApiBaseUrl } from './encv_core'

// encv_system.ts - 拆分自 encv.ts



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

// 🆕 2026-06-17：拉取 webdav manifest（多挂载点 + 虚拟文件树 + 容器映射）
// 强类型，import from types/webdav-test

export async function fetchWebDavManifest(mountName?: string): Promise<import('@/types/webdav-test').WebDavManifestResponse> {
  const baseUrl = getApiBaseUrl()
  const qs = mountName ? `?mount=${encodeURIComponent(mountName)}` : ''
  const response = await fetch(`${baseUrl}/api/webdav/manifest${qs}`)
  if (!response.ok) throw new Error(`Failed to fetch webdav manifest: ${response.status}`)
  return response.json() as Promise<import('@/types/webdav-test').WebDavManifestResponse>
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
