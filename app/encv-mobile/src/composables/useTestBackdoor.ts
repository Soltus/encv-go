import type { Ref } from "vue";
import type { FileItem } from "@encv/shared-components/api/encv";
import { eventBus } from "@encv/shared-components/composables/useEventBus";

export interface TestBackdoorAPI {
  simulateLongPress: (fileName: string) => Promise<void>;
  simulateFileClick: (fileName: string) => Promise<void>;
  navigateToPath: (path: string) => void;
  getCurrentFiles: () => FileItem[];
  triggerActionSheet: (fileName: string) => Promise<void>;
  openNewTaskModal: (sourcePath?: string, taskType?: "encrypt" | "decrypt") => Promise<void>;
  /** @internal 调试用：直接设置文件列表，验证响应式更新 */
  __debugSetFiles?: (files: FileItem[]) => void;
  /** @internal 调试用：检查 eventBus 实例是否与 spec 里的一致 */
  __debugGetEventBusMarker?: () => string | null;
  /** @internal 调试用：直接触发 file:change 处理函数 */
  __debugTriggerFileChange?: (payload: { path: string; action: "create" | "delete" | "modify" }) => void;
  /** @internal 调试用：获取待处理的 file change 数量 */
  __debugGetPendingChanges?: () => number;
  /** @internal 调试用：是否正在 stream loading */
  __debugIsStreamLoading?: () => boolean;
}

declare global {
  interface Window {
    __ENCV_TEST?: TestBackdoorAPI;
  }
}

export function useTestBackdoor(
  files: Ref<FileItem[]>,
  options: {
    onLongPress: (file: FileItem) => Promise<void>;
    onClick: (file: FileItem) => Promise<void>;
    navigateTo: (path: string) => void;
    openNewTask?: (sourcePath?: string, taskType?: "encrypt" | "decrypt") => Promise<void>;
    /** @internal 调试用：file change 处理函数 */
    __debugOnFileChange?: (payload: { path: string; action: "create" | "delete" | "modify" }) => void;
    /** @internal 调试用：获取待处理的 file change 数量 */
    __debugGetPendingChanges?: () => number;
    /** @internal 调试用：是否正在 stream loading */
    __debugIsStreamLoading?: () => boolean;
  }
): TestBackdoorAPI | null {
  if (!import.meta.env.DEV) return null;

  const api: TestBackdoorAPI = {
    simulateLongPress: async (fileName: string) => {
      const file = files.value.find(f => f.name === fileName);
      if (!file) throw new Error(`[TEST-BACKDOOR] File not found: ${fileName}`);
      console.warn(`[TEST-BACKDOOR] simulateLongPress(${fileName})`);
      await options.onLongPress(file);
    },

    simulateFileClick: async (fileName: string) => {
      const file = files.value.find(f => f.name === fileName);
      if (!file) throw new Error(`[TEST-BACKDOOR] File not found: ${fileName}`);
      console.warn(`[TEST-BACKDOOR] simulateFileClick(${fileName})`);
      await options.onClick(file);
    },

    navigateToPath: (path: string) => {
      console.warn(`[TEST-BACKDOOR] navigateToPath(${path})`);
      options.navigateTo(path);
    },

    getCurrentFiles: () => {
      return [...files.value];
    },

    triggerActionSheet: async (fileName: string) => {
      return api.simulateLongPress(fileName);
    },

    openNewTaskModal: async (sourcePath?: string, taskType?: "encrypt" | "decrypt") => {
      if (!options.openNewTask) {
        throw new Error("[TEST-BACKDOOR] openNewTask not provided in options");
      }
      console.warn(`[TEST-BACKDOOR] openNewTaskModal(${sourcePath}, ${taskType})`);
      await options.openNewTask(sourcePath, taskType);
    },

    __debugSetFiles: (newFiles: FileItem[]) => {
      console.warn("[TEST-BACKDOOR] __debugSetFiles called, newFiles.length:", newFiles.length);
      files.value = newFiles;
    },

    /** @internal 调试用：检查 eventBus 实例是否与 spec 里的一致 */
    __debugGetEventBusMarker: () => {
      return (eventBus as any).__testMarker ?? null;
    },

    /** @internal 调试用：直接触发 file:change 处理函数 */
    __debugTriggerFileChange: (payload: { path: string; action: "create" | "delete" | "modify" }) => {
      console.warn("[TEST-BACKDOOR] __debugTriggerFileChange:", payload);
      if (options.__debugOnFileChange) {
        options.__debugOnFileChange(payload);
      }
    },

    /** @internal 调试用：获取待处理的 file change 数量 */
    __debugGetPendingChanges: () => {
      return options.__debugGetPendingChanges ? options.__debugGetPendingChanges() : -1;
    },

    /** @internal 调试用：是否正在 stream loading */
    __debugIsStreamLoading: () => {
      return options.__debugIsStreamLoading ? options.__debugIsStreamLoading() : false;
    },
  };

  window.__ENCV_TEST = api;
  console.warn("[TEST-BACKDOOR] API registered on window.__ENCV_TEST");
  console.warn("[TEST-BACKDOOR] Available methods:", Object.keys(api).join(", "));

  return api;
}
