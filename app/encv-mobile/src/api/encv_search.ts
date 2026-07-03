import { getApiBaseUrl, proxySafeEncode } from "./encv_core";

import { type FileItem, PermissionDeniedError } from "./encv_files";
import type { EncvTask } from "./encv_tasks";

// encv_search.ts - 拆分自 encv.ts

export async function searchFiles(path: string, keyword: string, recursive = false): Promise<FileItem[]> {
  const baseUrl = getApiBaseUrl();
  const response = await fetch(
    `${baseUrl}/api/files/search?path=${proxySafeEncode(path)}&keyword=${encodeURIComponent(keyword)}&recursive=${recursive}`
  );
  if (!response.ok) {
    if (response.status === 403) {
      const data = await response.json().catch(() => ({}));
      if (data.code === "PERMISSION_DENIED") {
        throw new PermissionDeniedError(data.error || "Permission denied");
      }
    }
    throw new Error(`HTTP error! status: ${response.status}`);
  }
  const data = await response.json();
  return data.files || [];
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

export type SearchMode = "none" | "strict" | "combined" | "greedy";

export interface VectorSearchResult<T> {
  results: T[];
  vector_search: boolean;
  total: number;
  search_mode: SearchMode;
}

/**
 * 任务向量搜索（语义搜索，支持中文模糊匹配）
 */

export async function searchTasksVector(query: string, limit = 50): Promise<VectorSearchResult<EncvTask>> {
  const baseUrl = getApiBaseUrl();
  const response = await fetch(`${baseUrl}/api/search/tasks?q=${encodeURIComponent(query)}&limit=${limit}`);
  if (!response.ok) {
    throw new Error(`HTTP error! status: ${response.status}`);
  }
  const data = await response.json();
  return {
    results: data.tasks || [],
    vector_search: data.vector_search || false,
    total: data.total || 0,
    search_mode: (data.search_mode as SearchMode) || "none",
  };
}

/**
 * 文件向量搜索（语义搜索 + 现有搜索结果重排序）
 */

export async function searchFilesVector(path: string, query: string, recursive = true, limit = 50): Promise<VectorSearchResult<FileItem>> {
  const baseUrl = getApiBaseUrl();
  const response = await fetch(
    `${baseUrl}/api/search/files?q=${encodeURIComponent(query)}&path=${proxySafeEncode(path)}&recursive=${recursive}&limit=${limit}`
  );
  if (!response.ok) {
    throw new Error(`HTTP error! status: ${response.status}`);
  }
  const data = await response.json();
  return {
    results: data.files || [],
    vector_search: data.vector_search || false,
    total: data.total || 0,
    search_mode: (data.search_mode as SearchMode) || "none",
  };
}

/**
 * 获取搜索索引状态
 */

export async function getSearchStats(): Promise<{ available: boolean; stats?: { files: number; tasks: number } }> {
  const baseUrl = getApiBaseUrl();
  const response = await fetch(`${baseUrl}/api/search/stats`);
  if (!response.ok) {
    return { available: false };
  }
  return await response.json();
}

export interface IndexStats {
  totalFiles: number;
  totalDirs: number;
  totalSize: number;
  indexedAt: string;
  isIndexing: boolean;
  lastBuildMs: number;
  source?: string;
  containers?: number;
}

export async function getIndexStats(): Promise<IndexStats> {
  const baseUrl = getApiBaseUrl();
  const response = await fetch(`${baseUrl}/api/index/stats`);
  if (!response.ok) {
    throw new Error(`HTTP error! status: ${response.status}`);
  }
  return await response.json();
}

export async function rebuildIndex(): Promise<void> {
  const baseUrl = getApiBaseUrl();
  const response = await fetch(`${baseUrl}/api/index/rebuild`, { method: "POST" });
  if (!response.ok) {
    throw new Error(`HTTP error! status: ${response.status}`);
  }
}

// ─── 全文搜索 API（FTS5，2026-07-02 新增）───

/**
 * 全文搜索结果（Go 端 FTS5 返回）。
 * - snippet: 命中片段，用 `<<...>>` 包裹命中词
 * - hitCount: 命中次数
 * - score: bm25 相关度（负数，越小越相关）
 */
export interface FullTextSearchResult extends FileItem {
  snippet: string;
  hitCount: number;
}

export interface FullTextSearchResponse {
  results: FullTextSearchResult[];
  total: number;
  query: string;
  dbEngine: "sqlite" | "libsql" | "none";
  indexSize: number;
}

/**
 * 全文搜索（FTS5）。
 *
 * 支持查询语法（详见 Go 端 internal/fts/query.go）：
 *   - 空格分隔（隐式 AND）
 *   - AND / OR / NOT（必须大写）
 *   - "exact phrase"（双引号短语）
 *   - regex:^pattern  或  regex:/^pattern/（正则）
 *   - \ 转义下一个字符
 *
 * @param query 用户查询字符串
 * @param limit 最大返回数（默认 200）
 * @param pathPrefix 路径前缀过滤（可选）
 */
export async function searchFilesFullText(query: string, limit = 200, pathPrefix?: string): Promise<FullTextSearchResponse> {
  const baseUrl = getApiBaseUrl();
  const params = new URLSearchParams({
    q: query,
    limit: String(limit),
  });
  if (pathPrefix) {
    params.set("path_prefix", pathPrefix);
  }
  const response = await fetch(`${baseUrl}/api/files/search-fulltext?${params.toString()}`);
  if (!response.ok) {
    if (response.status === 503) {
      return { results: [], total: 0, query, dbEngine: "none", indexSize: 0 };
    }
    throw new Error(`HTTP error! status: ${response.status}`);
  }
  return await response.json();
}

/**
 * 获取全文索引统计信息。
 */
export interface FullTextIndexStats {
  totalFiles: number;
  totalDirs: number;
  totalSize: number;
  indexedAt: string;
  isIndexing: boolean;
  lastBuildMs: number;
  dbPath: string;
  fts5Enabled: boolean;
  tokenizer: string;
  indexVersion: number;
}

export async function getFullTextIndexStats(): Promise<{
  available: boolean;
  stats?: FullTextIndexStats;
  error?: string;
}> {
  const baseUrl = getApiBaseUrl();
  const response = await fetch(`${baseUrl}/api/files/search-fulltext/stats`);
  if (!response.ok) {
    return { available: false, error: `HTTP ${response.status}` };
  }
  return await response.json();
}

/**
 * 触发 FTS 索引重建任务。
 *
 * 2026-07-03 新增（spec fts-rebuild-task）
 *
 * 返回 taskId，前端通过 WS 事件 task:progress / task:completed 跟踪进度。
 * 任务走任务系统，自带进度百分比、phase、speed、eta、取消能力。
 *
 * 返回：
 *   - 200: { taskId, status: "queued", runId }
 *   - 409: { error, code: "REBUILD_IN_PROGRESS", taskId, status }  — 已有重建任务在跑
 *   - 503: { error, code: "FULLTEXT_UNAVAILABLE" }  — FTS 索引未初始化
 */
export interface FTSRebuildResponse {
  taskId: string;
  status: string;
  runId?: string;
}

export interface FTSRebuildErrorResponse {
  error: string;
  code: string;
  taskId?: string;
  status?: string;
}

export async function rebuildFullTextIndex(): Promise<FTSRebuildResponse> {
  const baseUrl = getApiBaseUrl();
  const response = await fetch(`${baseUrl}/api/files/search-fulltext/rebuild`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
  });
  if (!response.ok) {
    const errData = await response.json().catch(() => ({}) as FTSRebuildErrorResponse);
    const err = new Error(errData.error || `HTTP ${response.status}`) as Error & FTSRebuildErrorResponse;
    err.code = errData.code;
    err.taskId = errData.taskId;
    err.status = errData.status;
    throw err;
  }
  return await response.json();
}
