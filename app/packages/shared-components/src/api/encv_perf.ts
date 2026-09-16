import { getApiBaseUrl } from "./encv_core";
import { apiRequest } from "./core/request";

// encv_perf.ts - 拆分自 encv.ts

// PerformanceSummary 已提升为共享类型层 @encv/shared-components/types/task（见 packages/shared-components/src/types/task.ts）。
// 此处 re-export 以兼容现有 `import ... from '@/api/encv_perf'`。
export type { PerformanceSummary } from "@encv/shared-components/types/task";

/** Phase 耗时 */

export interface PhaseTiming {
  phase: string;
  durationMs: number;
  bytesProcessed?: number;
  throughputMBps?: number;
}

/** 完整性能指标（GET /api/tasks/:id/performance 返回） */

export interface PerformanceMetrics {
  taskId: string;
  taskType: string;
  pluginName?: string;
  containerVersion?: number;
  cipherMode?: number;
  compressionMode?: string;
  sourceSize: number;
  outputSize: number;
  sizeRatio: number;
  avgThroughput: number;
  peakThroughput: number;
  p50Throughput: number;
  p99Throughput: number;
  phaseTimings: PhaseTiming[];
  totalDurationMs: number;
  grade: "excellent" | "good" | "warn";
  gradeScore: number;
  gradeReason?: string;
  cpuScore: number;
  cpuLabel: string;
  createdAt: string;
}

/** 硬件校准结果 */

export interface CalibrationResult {
  cpuScore: number;
  aesThroughput: number;
  cpuLabel: string;
  calibratedAt: string;
  goVersion: string;
  os: string;
  arch: string;
  numCpu: number;
}

/** 获取指定任务的完整性能指标 */

export async function getTaskPerformance(taskId: string): Promise<PerformanceMetrics> {
  const data = await apiRequest(`/api/tasks/${encodeURIComponent(taskId)}/performance`);
  return (data as { metrics: PerformanceMetrics }).metrics;
}

/** 获取当前硬件校准结果 */

export async function getCalibration(): Promise<CalibrationResult | null> {
  const data = await apiRequest("/api/performance/calibration");
  return (data as { calibration: CalibrationResult | null }).calibration;
}

/** 手动重跑硬件校准（dev-only） */

export async function recalibrateCalibration(): Promise<CalibrationResult> {
  const data = await apiRequest("/api/performance/calibration", { method: "POST" });
  return (data as { calibration: CalibrationResult }).calibration;
}

/** 获取指定 plugin + taskType 的历史性能指标 */

export async function getPerformanceHistory(plugin: string, type: string, limit: number = 10): Promise<PerformanceMetrics[]> {
  const data = await apiRequest("/api/performance/history", { query: { plugin, type, limit } });
  return (data as { history?: PerformanceMetrics[] }).history ?? [];
}

// ========== 数据库管理 API（备份/恢复/导入/导出/跨引擎迁移） ==========

export interface DatabaseInfo {
  engine: string;
  concurrency: number;
  taskCount: number;
  hasCalibration: boolean;
}

/** 获取当前数据库引擎信息 */

export async function getDatabaseInfo(): Promise<DatabaseInfo> {
  return apiRequest<DatabaseInfo>("/api/database/info");
}

/** 导出数据库为 JSON 文件并下载 */

export async function exportDatabase(): Promise<void> {
  // 需读取 Content-Disposition 响应头 → 用 responseType:"response" 返回原始 Response
  const response = await apiRequest<Response>("/api/database/export", { responseType: "response" });

  // 获取文件名
  const contentDisposition = response.headers.get("Content-Disposition");
  let filename = "encv-db-export.json";
  if (contentDisposition) {
    const match = contentDisposition.match(/filename="(.+)"/);
    if (match) filename = match[1];
  }

  // 下载文件
  const blob = await response.blob();
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = filename;
  document.body.appendChild(a);
  a.click();
  document.body.removeChild(a);
  URL.revokeObjectURL(url);
}

/** 从 JSON 文件导入数据库（全量替换，不可逆！） */

export async function importDatabase(file: File): Promise<{
  status: string;
  imported: { tasks: number; trash: number; snapshots: number; metrics: number };
}> {
  const text = await file.text();
  const data = JSON.parse(text);

  return apiRequest("/api/database/import", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(data),
  });
}

/** 备份数据库到本地文件（后端直接写入，不经过前端内存） */

export async function backupDatabase(): Promise<{
  status: string;
  path: string;
  size: number;
  name: string;
}> {
  return apiRequest("/api/database/backup", { method: "POST" });
}

/** 从本地备份文件恢复数据库（全量替换，不可逆！） */

export async function restoreDatabase(path: string): Promise<{
  status: string;
  restored: { tasks: number; trash: number; snapshots: number; metrics: number };
}> {
  return apiRequest("/api/database/restore", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ path }),
  });
}

// ========== 数据库自动化测试 API ==========

export interface DBTestProgress {
  phase: "started" | "running" | "passed" | "failed" | "completed";
  scenario: string;
  message: string;
  durationMs?: number;
  metrics?: Record<string, any>;
  error?: string;
  category?: "basic" | "capability";
  capability?: string;
  isEngineSpecific?: boolean;
}

export interface DBTestRequest {
  scenarios?: string[];
}

/**
 * 运行数据库自动化测试（SSE 流式）
 * @param onProgress 进度回调
 * @returns Promise，测试完成时 resolve
 */
export function runDatabaseTests(scenarios?: string[], onProgress?: (p: DBTestProgress) => void): Promise<DBTestProgress> {
  return new Promise((resolve, reject) => {
    const baseUrl = getApiBaseUrl();
    const url = `${baseUrl}/api/database/test/run`;

    const xhr = new XMLHttpRequest();
    xhr.open("POST", url);
    xhr.setRequestHeader("Content-Type", "application/json");
    xhr.setRequestHeader("Accept", "text/event-stream");

    let lastIndex = 0;
    let lastProgress: DBTestProgress | null = null;

    xhr.onprogress = () => {
      const text = xhr.responseText;
      while (lastIndex < text.length) {
        const newlineIdx = text.indexOf("\n\n", lastIndex);
        if (newlineIdx === -1) break;

        const chunk = text.slice(lastIndex, newlineIdx);
        lastIndex = newlineIdx + 2;

        const lines = chunk.split("\n");
        for (const line of lines) {
          if (line.startsWith("data: ")) {
            try {
              const data = JSON.parse(line.slice(6)) as DBTestProgress;
              lastProgress = data;
              onProgress?.(data);
            } catch {
              // 忽略解析错误
            }
          }
        }
      }
    };

    xhr.onload = () => {
      if (xhr.status >= 200 && xhr.status < 300) {
        if (lastProgress) {
          resolve(lastProgress);
        } else {
          reject(new Error("no progress data received"));
        }
      } else {
        reject(new Error(`HTTP error! status: ${xhr.status}`));
      }
    };

    xhr.onerror = () => {
      reject(new Error("network error"));
    };

    xhr.send(JSON.stringify({ scenarios }));
  });
}
