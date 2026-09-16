import { proxySafeEncode } from "./encv_core";
import { apiRequest } from "./core/request";

// encv_files_extra.ts - 拆分自 encv.ts

export async function clearIndex(): Promise<void> {
  await apiRequest<void>("/api/index/clear", { method: "POST" });
}

export async function checkFileExists(path: string): Promise<boolean> {
  try {
    const data = await apiRequest(`/api/files/exists?path=${proxySafeEncode(path)}`);
    return !!(data as { exists?: boolean }).exists;
  } catch {
    console.debug("[API] checkFileExists failed");
    return false;
  }
}

export async function checkEncryptOutputExists(sourcePath: string, targetDir?: string): Promise<{ exists: boolean; outputPath: string }> {
  let url = `/api/files/encrypt-output-exists?sourcePath=${proxySafeEncode(sourcePath)}`;
  if (targetDir) url += `&targetDir=${proxySafeEncode(targetDir)}`;
  try {
    const data = await apiRequest(url);
    return { exists: !!(data as { exists?: boolean }).exists, outputPath: (data as { outputPath?: string }).outputPath || "" };
  } catch {
    console.debug("[API] checkEncryptOutputExists failed");
    return { exists: false, outputPath: "" };
  }
}

export type DecryptErrorCode = "wrong_password" | "data_corrupted" | "decrypt_failed" | "deprecated_version";

export interface DecryptError {
  error: DecryptErrorCode;
  message: string;
}

export function isWrongPasswordError(error: unknown): boolean {
  if (error && typeof error === "object" && "error" in error) {
    return (error as DecryptError).error === "wrong_password";
  }
  const msg = String(error).toLowerCase();
  return msg.includes("wrong password") || msg.includes("密码");
}

export async function renameFile(oldPath: string, newName: string): Promise<{ taskId: string }> {
  console.info("[API] renameFile:", oldPath, "→", newName);
  return apiRequest<{ taskId: string }>("/api/file/rename", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ oldPath, newName }),
  });
}

export interface RenameOriginalNameResponse {
  success: boolean;
  display_name: string;
  error?: string;
}

export async function renameOriginalName(path: string, newName: string, password?: string): Promise<RenameOriginalNameResponse> {
  console.info("[API] renameOriginalName:", path, "→", newName, "hasPassword:", !!password);
  return apiRequest<RenameOriginalNameResponse>("/api/file/rename", {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ path, new_name: newName, ...(password ? { password } : {}) }),
  });
}

export async function copyFile(srcPath: string, destPath: string): Promise<{ taskId: string }> {
  console.info("[API] copyFile:", srcPath, "→", destPath);
  return apiRequest<{ taskId: string }>("/api/file/copy", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ srcPath, destPath }),
  });
}

export async function moveFile(srcPath: string, destPath: string): Promise<{ taskId: string }> {
  console.info("[API] moveFile:", srcPath, "→", destPath);
  return apiRequest<{ taskId: string }>("/api/file/move", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ srcPath, destPath }),
  });
}
