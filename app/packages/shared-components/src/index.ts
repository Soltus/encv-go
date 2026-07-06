// Barrel exports for commonly used shared modules
// 注意：只导出真正通用的模块，encv 特有模块不在这里导出

export { useTheme } from "./composables/useTheme";
export { useI18n, registerI18nModule, registerI18nModules, registerFieldKeyMap, registerSectionTitleMap } from "./composables/useI18n";
export type { Locale, MessageParams, TFunction, TFieldFunction, TSectionTitleFunction, MessageModule } from "./composables/useI18n";
export { useToast } from "./composables/useToast";
export { useClipboard } from "./composables/useClipboard";
export { useDateFormat } from "./composables/useDateFormat";
export { relativeTime } from "./composables/relativeTime";
export { useFrontendLogs, addFrontendLog, frontendLogs } from "./composables/useFrontendLogs";
export { useEventBus, eventBus } from "./composables/useEventBus";
export { useDevTools } from "./composables/useDevTools";
export { useHighRefreshRate, getHighRefreshInfo } from "./composables/useHighRefreshRate";
export { registerIonicComponents, getIonicComponentNames } from "./composables/useIonicAutoRegister";
export { usePinchZoom } from "./composables/usePinchZoom";
export { useSearchInput } from "./composables/useSearchInput";
export { useActiveStatus } from "./composables/activeStatus";
