import { getApiBaseUrl } from './encv_core'

// encv_webdav.ts - 拆分自 encv.ts



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
