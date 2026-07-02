import { getApiBaseUrl } from './encv_core'

// encv_trash.ts - 拆分自 encv.ts



export interface TrashItem {
  id: string
  originalPath: string
  trashPath: string
  isDirectory: boolean
  size: number
  deletedAt: string
  taskId?: string
  restoreTaskId?: string
}

/** 列出回收站所有项 */

export async function listTrash(): Promise<TrashItem[]> {
  const baseUrl = getApiBaseUrl()
  const response = await fetch(`${baseUrl}/api/trash`)
  if (!response.ok) throw new Error(`HTTP error! status: ${response.status}`)
  const data = await response.json()
  return data.items || []
}

/** 从回收站恢复（可选指定目标路径） */

export async function restoreTrash(trashId: string, destPath?: string): Promise<{ taskId: string }> {
  const baseUrl = getApiBaseUrl()
  const response = await fetch(`${baseUrl}/api/trash/restore`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ trashId, destPath }),
  })
  if (!response.ok) throw new Error(`HTTP error! status: ${response.status}`)
  return response.json()
}

/** 永久删除回收站中的指定项 */

export async function purgeTrash(trashId: string): Promise<void> {
  const baseUrl = getApiBaseUrl()
  const response = await fetch(`${baseUrl}/api/trash/${encodeURIComponent(trashId)}`, {
    method: 'DELETE',
  })
  if (!response.ok) throw new Error(`HTTP error! status: ${response.status}`)
}

/** 清空整个回收站 */

export async function emptyTrash(): Promise<void> {
  const baseUrl = getApiBaseUrl()
  const response = await fetch(`${baseUrl}/api/trash`, { method: 'DELETE' })
  if (!response.ok) throw new Error(`HTTP error! status: ${response.status}`)
}
