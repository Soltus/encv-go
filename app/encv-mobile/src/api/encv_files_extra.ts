import { getApiBaseUrl, proxySafeEncode } from './encv_core'

// encv_files_extra.ts - 拆分自 encv.ts



export async function clearIndex(): Promise<void> {
  const baseUrl = getApiBaseUrl()
  const response = await fetch(`${baseUrl}/api/index/clear`, { method: 'POST' })
  if (!response.ok) {
    throw new Error(`HTTP error! status: ${response.status}`)
  }
}

export async function checkFileExists(path: string): Promise<boolean> {
  const baseUrl = getApiBaseUrl()
  const response = await fetch(`${baseUrl}/api/files/exists?path=${proxySafeEncode(path)}`)
  if (!response.ok) {
    console.debug('[API] checkFileExists failed:', response.status)
    return false
  }
  const data = await response.json()
  return !!data.exists
}

export async function checkEncryptOutputExists(sourcePath: string, targetDir?: string): Promise<{ exists: boolean; outputPath: string }> {
  const baseUrl = getApiBaseUrl()
  let url = `${baseUrl}/api/files/encrypt-output-exists?sourcePath=${proxySafeEncode(sourcePath)}`
  if (targetDir) url += `&targetDir=${proxySafeEncode(targetDir)}`
  const response = await fetch(url)
  if (!response.ok) {
    console.debug('[API] checkEncryptOutputExists failed:', response.status)
    return { exists: false, outputPath: '' }
  }
  const data = await response.json()
  return { exists: !!data.exists, outputPath: data.outputPath || '' }
}

export type DecryptErrorCode = 'wrong_password' | 'data_corrupted' | 'decrypt_failed' | 'deprecated_version'

export interface DecryptError {
  error: DecryptErrorCode
  message: string
}

export function isWrongPasswordError(error: unknown): boolean {
  if (error && typeof error === 'object' && 'error' in error) {
    return (error as DecryptError).error === 'wrong_password'
  }
  const msg = String(error).toLowerCase()
  return msg.includes('wrong password') || msg.includes('密码')
}

export async function renameFile(oldPath: string, newName: string): Promise<{ taskId: string }> {
  console.info('[API] renameFile:', oldPath, '→', newName)
  const baseUrl = getApiBaseUrl()
  const response = await fetch(`${baseUrl}/api/file/rename`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ oldPath, newName }),
  })
  if (!response.ok) {
    console.error('[API] renameFile failed:', response.status)
    throw new Error(`HTTP error! status: ${response.status}`)
  }
  return response.json()
}

export interface RenameOriginalNameResponse {
  success: boolean
  display_name: string
  error?: string
}

export async function renameOriginalName(path: string, newName: string, password?: string): Promise<RenameOriginalNameResponse> {
  console.info('[API] renameOriginalName:', path, '→', newName, 'hasPassword:', !!password)
  const baseUrl = getApiBaseUrl()
  const response = await fetch(`${baseUrl}/api/file/rename`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ path, new_name: newName, ...(password ? { password } : {}) }),
  })
  if (!response.ok) {
    const data = await response.json().catch(() => ({}))
    console.error('[API] renameOriginalName failed:', response.status, data.error)
    throw new Error(data.error || `HTTP error! status: ${response.status}`)
  }
  return response.json()
}

export async function copyFile(srcPath: string, destPath: string): Promise<{ taskId: string }> {
  console.info('[API] copyFile:', srcPath, '→', destPath)
  const baseUrl = getApiBaseUrl()
  const response = await fetch(`${baseUrl}/api/file/copy`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ srcPath, destPath }),
  })
  if (!response.ok) {
    console.error('[API] copyFile failed:', response.status)
    throw new Error(`HTTP error! status: ${response.status}`)
  }
  return response.json()
}

export async function moveFile(srcPath: string, destPath: string): Promise<{ taskId: string }> {
  console.info('[API] moveFile:', srcPath, '→', destPath)
  const baseUrl = getApiBaseUrl()
  const response = await fetch(`${baseUrl}/api/file/move`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ srcPath, destPath }),
  })
  if (!response.ok) {
    console.error('[API] moveFile failed:', response.status)
    throw new Error(`HTTP error! status: ${response.status}`)
  }
  return response.json()
}
