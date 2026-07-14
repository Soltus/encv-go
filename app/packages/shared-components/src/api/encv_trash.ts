import { apiRequest } from "./core/request";

// encv_trash.ts - 拆分自 encv.ts

export interface TrashItem {
  id: string;
  originalPath: string;
  trashPath: string;
  isDirectory: boolean;
  size: number;
  deletedAt: string;
  taskId?: string;
  restoreTaskId?: string;
}

/** 列出回收站所有项 */

export async function listTrash(): Promise<TrashItem[]> {
  const data = await apiRequest("/api/trash");
  return (data as { items?: TrashItem[] }).items ?? [];
}

/** 从回收站恢复（可选指定目标路径） */

export async function restoreTrash(trashId: string, destPath?: string): Promise<{ taskId: string }> {
  return apiRequest<{ taskId: string }>("/api/trash/restore", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ trashId, destPath }),
  });
}

/** 永久删除回收站中的指定项 */

export async function purgeTrash(trashId: string): Promise<void> {
  await apiRequest<void>(`/api/trash/${encodeURIComponent(trashId)}`, { method: "DELETE" });
}

/** 清空整个回收站 */

export async function emptyTrash(): Promise<void> {
  await apiRequest<void>("/api/trash", { method: "DELETE" });
}
