/**
 * 主 app 共享组件（就地共享，不通过 monorepo package）
 *
 * 历史：原通过 `@encvgo/components` workspace package 共享，但 pnpm + Vite 8
 *   在 vite root 内的 workspace package URL 解析有 bug（变成 `/packages/...` 落到
 *   SPA fallback）。2026-06-05 改为在主 app 和 plugin 各放一份本地 components-shared，
 *   通过 `@/components-shared` alias 引用，零胶水、零配置。
 *
 * 组件通过 JS-Native 桥接（window.OpenListNative）与 plugin-openlist 通信，
 * 不依赖主 app 的 IPC 桥接。
 */

export { default as OpenListLogList } from "./OpenListLogList.vue";
export { default as OpenListStatusCard } from "./OpenListStatusCard.vue";

export interface OpenListRuntime {
  running: boolean;
  port: number;
  pid: number;
  dataSizeBytes: number;
  lastError: string;
  lastUpdateTs: number;
  dataDir: string;
  isInstalled: boolean;
}

export interface OpenListLog {
  level: "info" | "warn" | "error" | "debug";
  message: string;
  timestamp: number;
}
