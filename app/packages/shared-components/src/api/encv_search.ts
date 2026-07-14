import { proxySafeEncode } from "./encv_core";
import { apiRequest } from "./core/request";
import { ApiError, isApiStatus } from "./core/errors";

import type { FileItem } from "./encv_files";
import type { EncvTask } from "./encv_tasks";

// encv_search.ts - 拆分自 encv.ts

export async function searchFiles(path: string, keyword: string, recursive = false): Promise<FileItem[]> {
  const data = await apiRequest(
    `/api/files/search?path=${proxySafeEncode(path)}&keyword=${encodeURIComponent(keyword)}&recursive=${recursive}`
  );
  return (data as { files?: FileItem[] }).files ?? [];
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
  const data = await apiRequest("/api/search/tasks", { query: { q: query, limit } });
  const d = data as { tasks?: EncvTask[]; vector_search?: boolean; total?: number; search_mode?: SearchMode };
  return {
    results: d.tasks ?? [],
    vector_search: d.vector_search ?? false,
    total: d.total ?? 0,
    search_mode: d.search_mode ?? "none",
  };
}

/**
 * 文件向量搜索（语义搜索 + 现有搜索结果重排序）
 */

export async function searchFilesVector(path: string, query: string, recursive = true, limit = 50): Promise<VectorSearchResult<FileItem>> {
  const data = await apiRequest(
    `/api/search/files?q=${encodeURIComponent(query)}&path=${proxySafeEncode(path)}&recursive=${recursive}&limit=${limit}`
  );
  const d = data as { files?: FileItem[]; vector_search?: boolean; total?: number; search_mode?: SearchMode };
  return {
    results: d.files ?? [],
    vector_search: d.vector_search ?? false,
    total: d.total ?? 0,
    search_mode: d.search_mode ?? "none",
  };
}

/**
 * 获取搜索索引状态
 */

export async function getSearchStats(): Promise<{ available: boolean; stats?: { files: number; tasks: number } }> {
  try {
    return (await apiRequest("/api/search/stats")) as { available: boolean; stats?: { files: number; tasks: number } };
  } catch {
    return { available: false };
  }
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
  return apiRequest<IndexStats>("/api/index/stats");
}

export async function rebuildIndex(): Promise<void> {
  await apiRequest<void>("/api/index/rebuild", { method: "POST" });
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
  const params = new URLSearchParams({
    q: query,
    limit: String(limit),
  });
  if (pathPrefix) {
    params.set("path_prefix", pathPrefix);
  }
  try {
    return await apiRequest<FullTextSearchResponse>(`/api/files/search-fulltext?${params.toString()}`);
  } catch (e) {
    if (isApiStatus(e, 503)) {
      return { results: [], total: 0, query, dbEngine: "none", indexSize: 0 };
    }
    throw e;
  }
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
  try {
    return (await apiRequest("/api/files/search-fulltext/stats")) as {
      available: boolean;
      stats?: FullTextIndexStats;
      error?: string;
    };
  } catch (e) {
    const status = e instanceof ApiError ? e.status : 0;
    return { available: false, error: `HTTP ${status}` };
  }
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
  try {
    return await apiRequest<FTSRebuildResponse>("/api/files/search-fulltext/rebuild", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
    });
  } catch (e) {
    if (e instanceof ApiError && e.body && typeof e.body === "object") {
      const errData = e.body as FTSRebuildErrorResponse;
      const err = new Error(errData.error || `HTTP ${e.status}`) as Error & FTSRebuildErrorResponse;
      err.code = errData.code;
      err.taskId = errData.taskId;
      err.status = errData.status;
      throw err;
    }
    throw e;
  }
}
