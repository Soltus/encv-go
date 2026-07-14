/**
 * appCapabilities — 应用层 → 共享层的「能力注入」DI 注册点
 *
 * 背景：shared 作为共享层，不得反向依赖应用层（@/...）内部实现。
 * 但部分通用模块（agent base URL、alist-encrypt 特征等）运行时需要
 * 应用层特有的能力：原生环境判断、打开新建任务弹窗、弹出密码对话框等。
 *
 * 约定：
 *   - shared 内部一律通过 getAppCapabilities() 取用这些能力，绝不 import @/。
 *   - app 在启动期（main.ts）调用 setAppCapabilities(...) 注入具体实现。
 *   - 未注入时使用安全默认：isNative→false（web），其余抛清晰错误，
 *     便于在 standalone / 测试环境尽早暴露「忘记注入」的问题。
 */

import type { Ref } from "vue";
import type { FileItem } from "@encv/shared-components/api/encv";

/** DEV-only 测试后门所需的本地上下文（由 useFilesView 提供，app 注入实现）。 */
export interface TestBackdoorContext {
  files: Ref<FileItem[]>;
  onLongPress: (file: FileItem) => Promise<void>;
  onClick: (file: FileItem) => Promise<void>;
  navigateTo: (path: string) => void;
  __debugOnFileChange: (payload: { path: string; action: "create" | "delete" | "modify" }) => void;
  __debugGetPendingChanges: () => number;
  __debugIsStreamLoading: () => boolean;
}

/** 原生播放器打开结果（签名对齐 @/plugins/GoProcess.PlayResult）。 */
export interface PlayResult {
  success: boolean;
  error?: string;
  errorDetail?: string;
}

/** 权限请求结果（签名对齐 @/plugins/GoProcess.PermissionResult）。 */
export interface PermissionResult {
  granted: boolean;
}

export interface AppCapabilities {
  /** 是否运行在原生（Capacitor/APK）环境。默认 false（web SPA）。 */
  isNative: () => boolean;
  /** 打开新建任务弹窗（encrypt/decrypt）。未注入时抛错提示。 */
  openNewTask: (initialSourcePath?: string, initialTaskType?: "encrypt" | "decrypt") => void;
  /** 弹出密码输入对话框，返回用户输入的密码或 null（取消）。未注入时抛错提示。 */
  alertPassword: (fileDisplayName: string) => Promise<string | null>;
  /** 打开原生播放器。未注入时抛错提示。 */
  openPlayer: (filePath: string, name: string, mimeType: string, mode?: string) => Promise<PlayResult>;
  /** 用外部应用打开 URL（如视频流）。未注入时抛错提示。 */
  openExternal: (url: string, mimeType: string) => Promise<void>;
  /** 获取文件的本地路径（原生环境下载到本地后）。未注入时抛错提示。 */
  getLocalFilePath: (path: string) => Promise<string>;
  /** 请求存储权限。未注入时抛错提示。 */
  requestStoragePermission: () => Promise<PermissionResult>;
  /** DEV-only：注册测试后门（app 提供实现，web/生产未注入则静默跳过）。 */
  registerTestBackdoor?: (ctx: TestBackdoorContext) => void;
}

const defaults: AppCapabilities = {
  isNative: () => false,
  openNewTask: () => {
    throw new Error("[appCapabilities] openNewTask 未注入（需在 app 启动期调用 setAppCapabilities）");
  },
  alertPassword: () => {
    throw new Error("[appCapabilities] alertPassword 未注入（需在 app 启动期调用 setAppCapabilities）");
  },
  openPlayer: () => {
    throw new Error("[appCapabilities] openPlayer 未注入（需在 app 启动期调用 setAppCapabilities）");
  },
  openExternal: () => {
    throw new Error("[appCapabilities] openExternal 未注入（需在 app 启动期调用 setAppCapabilities）");
  },
  getLocalFilePath: () => {
    throw new Error("[appCapabilities] getLocalFilePath 未注入（需在 app 启动期调用 setAppCapabilities）");
  },
  requestStoragePermission: () => {
    throw new Error("[appCapabilities] requestStoragePermission 未注入（需在 app 启动期调用 setAppCapabilities）");
  },
};

let capabilities: AppCapabilities = { ...defaults };

/** app 启动期调用：注入 / 覆盖应用层能力。可多次部分覆盖。 */
export function setAppCapabilities(partial: Partial<AppCapabilities>): void {
  capabilities = { ...capabilities, ...partial };
}

/** shared 内部取用应用层能力。 */
export function getAppCapabilities(): AppCapabilities {
  return capabilities;
}
