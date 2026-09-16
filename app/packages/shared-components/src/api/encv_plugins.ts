import { proxySafeEncode } from "./encv_core";
import { apiRequest } from "./core/request";

import type { FileItem, FileListResponse } from "./encv_files";
import type { TaskType } from "./encv_tasks";

// encv_plugins.ts - 拆分自 encv.ts

export interface PluginMeta {
  name: string;
  supportedExtensions: string[];
  supportedMimePrefixes: string[];
  containerExtension: string;
  taskOptions: TaskOptions;
}

export type PasswordStrategy = "global" | "independent" | "none";

export interface TaskField {
  key: string;
  label: string;
  type: "string" | "password" | "select" | "bool";
  required: boolean;
  defaultValue: string;
  help: string;
  options?: string[];
  optionLabels?: Record<string, string>;
  condition?: "" | "encrypt" | "decrypt";
}

export interface TaskOptions {
  passwordStrategy: PasswordStrategy;
  supportVersionSelect: boolean;
  supportedVersions: number[] | null;
  defaultVersion: number;
  extraFields: TaskField[];
}

export interface PluginCandidate {
  name: string;
  matchType: "mime" | "extension" | "general" | "container";
  priority: number;
  taskOptions: TaskOptions | null;
}

export interface PredictPluginResponse {
  candidates: PluginCandidate[];
  pluginName: string | null;
  error?: string;
  taskOptions: TaskOptions | null;
}

export async function fetchPlugins(): Promise<PluginMeta[]> {
  const data = await apiRequest("/api/plugins");
  const plugins = (data as { plugins?: PluginMeta[] }).plugins ?? [];
  console.info("[API] fetchPlugins:", plugins.length || 0, "plugins");
  return plugins;
}

export interface TagInfo {
  name: string;
  count: number;
}

export async function fetchTags(): Promise<TagInfo[]> {
  const data = await apiRequest("/api/files/tags");
  const tags = (data as { tags?: TagInfo[] }).tags ?? [];
  console.info("[API] fetchTags:", tags.length || 0, "tags");
  return tags;
}

export async function addTag(path: string, tag: string): Promise<void> {
  console.info("[API] addTag:", path, tag);
  await apiRequest<void>("/api/files/tags", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ path, tag, action: "add" }),
  });
}

export async function removeTag(path: string, tag: string): Promise<void> {
  console.info("[API] removeTag:", path, tag);
  await apiRequest<void>("/api/files/tags", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ path, tag, action: "remove" }),
  });
}

export async function listFilesByTag(tag: string, path?: string): Promise<FileItem[]> {
  const data = await apiRequest(`/api/files?path=${proxySafeEncode(path || "/")}&tag=${encodeURIComponent(tag)}`);
  const files = (data as FileListResponse).files ?? [];
  console.info("[API] listFilesByTag:", tag, "→", files.length || 0, "files");
  return files;
}

export async function predictPlugin(sourcePath: string, type: TaskType): Promise<PredictPluginResponse> {
  return apiRequest<PredictPluginResponse>("/api/tasks/predict-plugin", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ sourcePath, type }),
  });
}

// 🆕 2026-06-15 multi-mount (spec Phase E)：挂载点管理 API
//
// 后端路由：internal/server/mount_api.go
// 数据模型：internal/mount/mount.go (Mount)
//
// 端点：
//   GET    /api/mounts               → listMounts / ListMountsResponse
//   GET    /api/mounts/:id           → getMount
//   POST   /api/mounts               → createMount
//   PUT    /api/mounts/:id           → updateMount
//   DELETE /api/mounts/:id           → deleteMount
//   POST   /api/mounts/:id/resolve   → resolveMountPath
//   GET    /api/mounts/:id/usage     → fetchMountUsage

/** Mount 挂载点数据模型。与后端 mount.Mount 字段一一对应（snake_case）。 */

export interface Mount {
  id: string;
  name: string;
  mount_path: string;
  driver: string;
  root_path: string;
  enabled: boolean;
  read_only: boolean;
  driver_config?: Record<string, unknown>;
  created_at?: string;
  updated_at?: string;
}

/** 预置 driver 名常量（与后端 mount.DriverLocal/AppData/Sandbox 一致）。 */

export const MOUNT_DRIVERS = ["local", "appdata", "sandbox"] as const;

export type MountDriver = (typeof MOUNT_DRIVERS)[number];

/** 预置 mount name 提示常量（仅用于 UI 提示，非后端强制）。 */

export const MOUNT_PRESET_NAMES = ["primary", "automation", "sandbox"] as const;

export interface ListMountsResponse {
  mounts: Mount[];
  drivers: string[];
  /**
   * 🆕 2026-06-16：mount 启动期错误（不再静默）
   * - 后端 server.go 在 MigrateFromServingDir 失败时 append 到 s.mountBootstrapErrors
   * - /api/mounts 响应里暴露
   * - MountsDetail.vue 顶部 banner 展示，每条对应一个 mount 启动失败原因
   * - 典型场景：mounts.json 损坏 / bootstrap 写盘失败 / driver 工厂 panic
   */
  bootstrap_errors: string[];
}

export interface ResolveMountResponse {
  virtual_path: string;
  abs_path: string;
  rel_path: string;
  mount_name: string;
}

export interface MountUsageResponse {
  mount_id: string;
  root_path: string;
  entry_count: number;
}

export async function listMounts(): Promise<ListMountsResponse> {
  return apiRequest<ListMountsResponse>("/api/mounts");
}

export async function getMount(id: string): Promise<Mount> {
  return apiRequest<Mount>(`/api/mounts/${encodeURIComponent(id)}`);
}

/** Create / Update 通用 body 字段（不含 id / created_at / updated_at）。 */

export interface MountInput {
  name: string;
  mount_path: string;
  driver: string;
  enabled: boolean;
  read_only: boolean;
  driver_config?: Record<string, unknown>;
}

export async function createMount(input: MountInput): Promise<Mount> {
  return apiRequest<Mount>("/api/mounts", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
}

export async function updateMount(id: string, input: MountInput): Promise<Mount> {
  return apiRequest<Mount>(`/api/mounts/${encodeURIComponent(id)}`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
}

export async function deleteMount(id: string): Promise<void> {
  // 204 No Content 视为成功（apiRequest 空响应体返 undefined，等价于成功）
  await apiRequest<void>(`/api/mounts/${encodeURIComponent(id)}`, { method: "DELETE" });
}

export async function resolveMountPath(id: string, subPath: string): Promise<ResolveMountResponse> {
  return apiRequest<ResolveMountResponse>(`/api/mounts/${encodeURIComponent(id)}/resolve`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ sub_path: subPath }),
  });
}

export async function fetchMountUsage(id: string): Promise<MountUsageResponse> {
  return apiRequest<MountUsageResponse>(`/api/mounts/${encodeURIComponent(id)}/usage`);
}

// ========== 🆕 性能指标 ==========

/** 性能指标摘要（task:completed 事件推送） */
