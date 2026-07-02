import { EncvTask } from './encv_tasks'

import { FileItem, PermissionDeniedError } from './encv_files'

import { getApiBaseUrl, proxySafeEncode } from './encv_core'

// encv_search.ts - 拆分自 encv.ts



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

// ─── 向量搜索 API（Turso 原生向量检索 + 中文 bigram 分词）───

/**
 * 搜索模式（见 debug-discipline.md §3.5）：
 *   - none：无搜索（空查询）
 *   - strict：关键词精确匹配 ≥ 20，只返回关键词结果
 *   - combined：关键词匹配 1~19，向量重排序
 *   - greedy：关键词匹配 0，纯向量 fallback（bigram 过滤放宽）
 *
 * 前端据 searchMode 对 greedy 结果加视觉标记，让用户看出是宽松匹配。
 */

export type SearchMode = 'none' | 'strict' | 'combined' | 'greedy'

export interface VectorSearchResult<T> {
  results: T[]
  vector_search: boolean
  total: number
  search_mode: SearchMode
}

/**
 * 任务向量搜索（语义搜索，支持中文模糊匹配）
 */

export async function searchTasksVector(query: string, limit = 50): Promise<VectorSearchResult<EncvTask>> {
  const baseUrl = getApiBaseUrl()
  const response = await fetch(`${baseUrl}/api/search/tasks?q=${encodeURIComponent(query)}&limit=${limit}`)
  if (!response.ok) {
    throw new Error(`HTTP error! status: ${response.status}`)
  }
  const data = await response.json()
  return {
    results: data.tasks || [],
    vector_search: data.vector_search || false,
    total: data.total || 0,
    search_mode: (data.search_mode as SearchMode) || 'none',
  }
}

/**
 * 文件向量搜索（语义搜索 + 现有搜索结果重排序）
 */

export async function searchFilesVector(path: string, query: string, recursive = true, limit = 50): Promise<VectorSearchResult<FileItem>> {
  const baseUrl = getApiBaseUrl()
  const response = await fetch(`${baseUrl}/api/search/files?q=${encodeURIComponent(query)}&path=${proxySafeEncode(path)}&recursive=${recursive}&limit=${limit}`)
  if (!response.ok) {
    throw new Error(`HTTP error! status: ${response.status}`)
  }
  const data = await response.json()
  return {
    results: data.files || [],
    vector_search: data.vector_search || false,
    total: data.total || 0,
    search_mode: (data.search_mode as SearchMode) || 'none',
  }
}

/**
 * 获取搜索索引状态
 */

export async function getSearchStats(): Promise<{ available: boolean; stats?: { files: number; tasks: number } }> {
  const baseUrl = getApiBaseUrl()
  const response = await fetch(`${baseUrl}/api/search/stats`)
  if (!response.ok) {
    return { available: false }
  }
  return await response.json()
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
