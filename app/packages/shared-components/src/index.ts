// Barrel exports for commonly used shared modules
// 注意：只导出真正通用的模块，encv 特有模块不在这里导出

// 共享 API 基础设施基座（base URL / 认证依赖注入 / 统一请求封装）
export * from "./api/core";
// Dev Logs
export { default as DevLogsViewer } from "./components/DevLogsViewer.vue";
export { default as SettingsGroup } from "./components/settings/SettingsGroup.vue";
export { default as SettingsItem } from "./components/settings/SettingsItem.vue";
// Settings components
export { default as SettingsPage } from "./components/settings/SettingsPage.vue";
export { default as SettingsSelect } from "./components/settings/SettingsSelect.vue";
// Virtual Log List
export { default as VirtualLogList } from "./components/VirtualLogList.vue";
export { compactStatus, isActiveStatus, readTurnStatus } from "./composables/activeStatus";
export { formatRelativeTime } from "./composables/relativeTime";
export { copyToClipboard, selectAllText } from "./composables/useClipboard";
export { formatDateTime, formatDuration } from "./composables/useDateFormat";
export { useDevTools } from "./composables/useDevTools";
export {
  addFrontendLog,
  clearFrontendLogs,
  getFrontendLogsJson,
  hijackConsole,
  restoreConsole,
  useFrontendLogs,
} from "./composables/useFrontendLogs";
export { getHighRefreshInfo, initHighRefreshRate } from "./composables/useHighRefreshRate";
export type { Locale, MessageModule, MessageParams, TFieldFunction, TFunction, TSectionTitleFunction } from "./composables/useI18n";
export { registerFieldKeyMap, registerI18nModule, registerI18nModules, registerSectionTitleMap, useI18n } from "./composables/useI18n";
export { getIonicComponentNames, registerIonicComponents } from "./composables/useIonicAutoRegister";
export { usePinchZoom } from "./composables/usePinchZoom";
export { useSearchInput } from "./composables/useSearchInput";
export { useTheme } from "./composables/useTheme";
export { showToast } from "./composables/useToast";
