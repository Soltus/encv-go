// 任务系统 api 真实实现现已在 @encv/shared-components/api/encv_*（提升自 encv-mobile/src/api），
// 此处聚合为统一 barrel，维持 `@encv/shared-components/api/encv` 既有导入兼容。
// 插件领域类型（PasswordStrategy / PluginMeta / ...）由 encv_plugins 经
// @encv/shared-components/types/task 透传，无需在此重复导出。

export * from "./encv_admin";
export {
  DEFAULT_API_BASE_URL,
  DEV_SANDBOX_ENTRY,
  getApiBaseUrl,
  getPersistedBackendIdentity,
  getServerUrl,
  getWebSocketUrl,
  isOpenPreviewBrowser,
  proxySafeEncode,
  resetServerUrl,
  setApiBaseUrl,
} from "./encv_core";
export * from "./encv_files";
export * from "./encv_files_extra";
export * from "./encv_openlist";
export * from "./encv_perf";
export * from "./encv_plugins";
export * from "./encv_search";
export * from "./encv_system";
export * from "./encv_tasks";
export * from "./encv_trash";
export * from "./encv_webdav";
