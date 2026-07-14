import { getApiBaseUrl, proxySafeEncode } from "./encv_core";
import { apiRequest } from "./core/request";
import { formatBytes } from "../lib/format";

// encv_files.ts - 拆分自 encv.ts

export interface FileItem {
  name: string;
  display_name?: string;
  path: string;
  isDirectory: boolean;
  isEncrypted?: boolean;
  size?: number;
  modified?: string;
  /** 向量搜索相关度分数（0-1，越大越相似）。仅 /api/search/files 返回时填充。 */
  score?: number;
  /** 🆕 全文搜索命中片段（高亮 <<...>> 包裹）。仅全文搜索时填充。 */
  snippet?: string;
  /** 🆕 全文搜索命中次数。仅全文搜索时填充。 */
  hitCount?: number;
  /** 🆕 标记为全文搜索结果。 */
  isFullText?: boolean;
}

export interface FileListResponse {
  files: FileItem[];
  error?: string;
  code?: string;
}

// 错误类型已收敛到 core/errors（继承 ApiError），此处 import 供本模块 throw 使用，
// 并 re-export 以兼容既有 importer（encv_search.ts、FilePickerModal.vue 经 api/encv barrel 引用）。
import { PermissionDeniedError, NotFoundError } from "./core/errors";
export { PermissionDeniedError, NotFoundError };

export async function listFiles(path = "/"): Promise<FileItem[]> {
  // 403/404 由 apiRequest 默认映射为 PermissionDeniedError / NotFoundError
  const data = await apiRequest(`/api/files?path=${proxySafeEncode(path)}`);
  const files = (data as FileListResponse).files ?? [];
  console.info("[API] listFiles:", path, "→", files.length || 0, "files");
  return files;
}

export async function listFilesStream(
  path = "/",
  onItem: (file: FileItem) => void,
  signal?: AbortSignal
): Promise<{ files: FileItem[]; error?: string }> {
  const baseUrl = getApiBaseUrl();
  const response = await fetch(`${baseUrl}/api/files/stream?path=${proxySafeEncode(path)}`, {
    signal,
  });

  if (!response.ok) {
    if (response.status === 403) {
      throw new PermissionDeniedError("Permission denied");
    }
    if (response.status === 404) {
      throw new NotFoundError("Path not found");
    }
    throw new Error(`HTTP error! status: ${response.status}`);
  }

  const files: FileItem[] = [];
  const reader = response.body!.getReader();
  const decoder = new TextDecoder();
  let buffer = "";

  try {
    while (true) {
      const { done, value } = await reader.read();
      if (done) break;

      buffer += decoder.decode(value, { stream: true });
      const lines = buffer.split("\n");
      buffer = lines.pop() || "";

      for (const line of lines) {
        if (!line.startsWith("data: ")) continue;
        const data = line.slice(6).trim();
        if (!data) continue;

        if (data === "[DONE]") {
          return { files };
        }

        try {
          const file = JSON.parse(data) as FileItem;
          files.push(file);
          onItem(file);
        } catch {
          // skip malformed JSON
        }
      }
    }
  } finally {
    reader.releaseLock();
  }

  return { files };
}

export async function listPluginFilesStream(
  path: string,
  extensions: string[],
  onItem: (file: FileItem) => void,
  signal?: AbortSignal
): Promise<{ files: FileItem[]; error?: string }> {
  const baseUrl = getApiBaseUrl();
  const extParam = extensions.map(e => `.${e.toLowerCase()}`).join(",");
  const response = await fetch(
    `${baseUrl}/api/files/plugin-stream?path=${proxySafeEncode(path)}&extensions=${encodeURIComponent(extParam)}`,
    {
      signal,
    }
  );

  if (!response.ok) {
    if (response.status === 403) {
      throw new PermissionDeniedError("Permission denied");
    }
    throw new Error(`HTTP error! status: ${response.status}`);
  }

  const files: FileItem[] = [];
  const reader = response.body!.getReader();
  const decoder = new TextDecoder();
  let buffer = "";

  try {
    while (true) {
      const { done, value } = await reader.read();
      if (done) break;

      buffer += decoder.decode(value, { stream: true });
      const lines = buffer.split("\n");
      buffer = lines.pop() || "";

      for (const line of lines) {
        if (!line.startsWith("data: ")) continue;
        const data = line.slice(6).trim();
        if (!data) continue;

        if (data === "[DONE]") {
          return { files };
        }

        try {
          const file = JSON.parse(data) as FileItem;
          files.push(file);
          onItem(file);
        } catch {
          // skip malformed JSON
        }
      }
    }
  } finally {
    reader.releaseLock();
  }

  return { files };
}

export function getFileStreamUrl(path: string): string {
  if (import.meta.env.DEV) {
    return `/stream?path=${proxySafeEncode(path)}`;
  }
  const baseUrl = getApiBaseUrl();
  return `${baseUrl}/stream?path=${proxySafeEncode(path)}`;
}

export function getFilePreviewUrl(previewPage: string, filePath: string): string {
  if (import.meta.env.DEV) {
    return `/preview/${previewPage}?file=${proxySafeEncode(filePath)}`;
  }
  const baseUrl = getApiBaseUrl();
  return `${baseUrl}/preview/${previewPage}?file=${proxySafeEncode(filePath)}`;
}

export function getExternalStreamUrl(path: string): string {
  if (import.meta.env.DEV) {
    return `/api/stream/external?path=${proxySafeEncode(path)}`;
  }
  const baseUrl = getApiBaseUrl();
  return `${baseUrl}/api/stream/external?path=${proxySafeEncode(path)}`;
}

export async function deleteFile(path: string): Promise<{ taskId: string }> {
  // 🆕 2026-06-10 修复 #1：deleteFile 500 错误需要可读 message
  // apiRequest 在 4xx/5xx 会把 body.error 作为 ApiError.message 抛出（含状态码与 body）
  console.debug("[API] deleteFile:", path);
  return apiRequest<{ taskId: string }>(`/api/files?path=${proxySafeEncode(path)}`, { method: "DELETE" });
}

export async function createDirectory(parentPath: string, name: string): Promise<void> {
  console.info("[API] createDirectory:", parentPath, name);
  await apiRequest<void>("/api/files/mkdir", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ parent_path: parentPath, name }),
  });
}

export async function uploadFile(targetPath: string, file: File): Promise<FileItem> {
  console.info("[API] uploadFile:", targetPath, file.name, "size:", file.size);
  const formData = new FormData();
  formData.append("file", file);

  const result = await apiRequest<FileItem>(`/api/files/upload?path=${proxySafeEncode(targetPath)}`, {
    method: "POST",
    body: formData,
  });
  console.info("[API] uploadFile success:", result.path, "size:", result.size);
  return result;
}

/**
 * 🆕 v6 2026-06-22：单文件 metadata 查询（file:change 增量更新用）
 *   - 调 /api/file/info?path=... 拿单个文件的 FileItem（含 size/modified/isDirectory/isEncrypted）
 *   - 404 → 抛 NotFoundError（调用方据此从 files.value 移除）
 *   - 用于 file:change action=create|modify 的增量更新，避免全量 listFiles
 */

export async function getFileInfo(path: string): Promise<FileItem> {
  // 404 由 apiRequest 默认映射为 NotFoundError
  const data = await apiRequest(`/api/file/info?path=${proxySafeEncode(path)}`);
  const d = data as {
    name: string;
    display_name?: string;
    path: string;
    is_directory: boolean;
    is_encrypted?: boolean;
    size?: number;
    modified?: string;
  };
  return {
    name: d.name,
    display_name: d.display_name,
    path: d.path,
    isDirectory: d.is_directory,
    isEncrypted: d.is_encrypted,
    size: d.size,
    modified: d.modified,
  };
}

export interface FileContentResponse {
  name: string;
  path: string;
  size: number;
  content: string;
  encoding: string;
}

export async function readFileContent(path: string): Promise<FileContentResponse> {
  const data = await apiRequest<FileContentResponse>(`/api/file?path=${proxySafeEncode(path)}`);
  console.info("[API] readFileContent:", path, "size:", data.size);
  return data;
}

export function formatFileSize(bytes?: number): string {
  return formatBytes(bytes);
}

export function getFileExtension(name: string): string {
  const lastDot = name.lastIndexOf(".");
  if (lastDot === -1) return "";
  return name.substring(lastDot + 1).toLowerCase();
}

export type FileCategory = "video" | "audio" | "image" | "document" | "other";

export function getFileCategory(name: string): FileCategory {
  const ext = getFileExtension(name);
  const videoExts = ["mp4", "mkv", "avi", "mov", "wmv", "flv", "webm", "m4v"];
  const audioExts = ["mp3", "flac", "wav", "aac", "ogg", "wma", "m4a"];
  const imageExts = ["jpg", "jpeg", "png", "gif", "webp", "bmp", "svg"];
  const docExts = ["pdf", "doc", "docx", "txt", "xls", "xlsx", "ppt", "pptx"];

  if (videoExts.includes(ext)) return "video";
  if (audioExts.includes(ext)) return "audio";
  if (imageExts.includes(ext)) return "image";
  if (docExts.includes(ext)) return "document";
  return "other";
}
