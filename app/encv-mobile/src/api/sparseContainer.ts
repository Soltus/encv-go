/**
 * Sparse Container API client（dev tool 域）
 *
 * 对应后端 endpoint：
 *   POST   /api/dev/sparse-container           → 写出 sparse 虚拟容器
 *   GET    /api/dev/sparse-container/probe     → 探测 1 个 fragment 的读路径
 *   DELETE /api/dev/sparse-container           → 清理产物
 *
 * 关键约束：
 *   - 100×128GB sparse 容器虚拟 12.8TB、物理 ~16KB（sparse file + ftruncate）
 *   - physicalChunkMB=0 时仅 main file；≥30 时生成 .part 物理分片
 *   - 真机降级：调用方应在 write 前先 navigator.storage.estimate() 二次确认
 *
 * 🆕 2026-06-11 v1：与 internal/v2/testutil/sparse_container.go 对齐
 */

import { getApiBaseUrl } from "@encv/shared-components/api/core";

export interface SparseContainerRequest {
  outputDir: string;
  baseName: string;
  fragmentCount: number;
  fragmentSizeGB: number;
  physicalChunkMB: number;
  cipherMode: number;
  containerType: number;
}

export interface SparseContainerResponse {
  virtualTotalBytes: number;
  physicalMainBytes: number;
  physicalUsedBytes: number;
  manifestSizeBytes: number;
  fragmentCount: number;
  fragmentSizeBytes: number;
  isSparse: boolean;
  mainFilePath: string;
  partFilePattern: string;
  durationMs: number;
}

export interface SparseContainerProbeRequest {
  mainPath: string;
  fragmentIdx: number;
  fragmentSizeGB: number;
}

export interface SparseContainerProbeResponse {
  bytesRead: number;
  heapInUseKB: number;
  physicalSize: number;
  virtualSize: number;
  durationMs: number;
  seekMs: number;
  readMs: number;
}

export interface SparseContainerCleanupRequest {
  outputDir: string;
  baseName: string;
}

export interface SparseContainerCleanupResponse {
  removedFiles: string[];
  removedBytes: number;
  durationMs: number;
}

/**
 * 解析 fetch 错误 body（透传后端 JSON {error: "..."}）
 * 与 files.ts deleteFile / useWebDavAutomationTests 的错误处理风格一致
 */
async function parseErrorBody(response: Response): Promise<string> {
  let detail = "";
  try {
    const data = await response.json();
    detail = (data as { error?: string; message?: string })?.error || (data as { error?: string; message?: string })?.message || "";
  } catch {
    try {
      detail = (await response.text()).slice(0, 200);
    } catch {
      /* ignore */
    }
  }
  return detail || `HTTP error! status: ${response.status}`;
}

export async function writeSparseContainer(req: SparseContainerRequest): Promise<SparseContainerResponse> {
  const baseUrl = getApiBaseUrl();
  const response = await fetch(`${baseUrl}/api/dev/sparse-container`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(req),
  });
  if (!response.ok) throw new Error(await parseErrorBody(response));
  return response.json() as Promise<SparseContainerResponse>;
}

export async function probeSparseContainer(req: SparseContainerProbeRequest): Promise<SparseContainerProbeResponse> {
  const baseUrl = getApiBaseUrl();
  const params = new URLSearchParams({
    mainPath: req.mainPath,
    fragmentIdx: String(req.fragmentIdx),
    fragmentSizeGB: String(req.fragmentSizeGB),
  });
  const response = await fetch(`${baseUrl}/api/dev/sparse-container/probe?${params.toString()}`);
  if (!response.ok) throw new Error(await parseErrorBody(response));
  return response.json() as Promise<SparseContainerProbeResponse>;
}

export async function cleanupSparseContainer(req: SparseContainerCleanupRequest): Promise<SparseContainerCleanupResponse> {
  const baseUrl = getApiBaseUrl();
  const params = new URLSearchParams({
    outputDir: req.outputDir,
    baseName: req.baseName,
  });
  const response = await fetch(`${baseUrl}/api/dev/sparse-container?${params.toString()}`, {
    method: "DELETE",
  });
  if (!response.ok) throw new Error(await parseErrorBody(response));
  return response.json() as Promise<SparseContainerCleanupResponse>;
}
