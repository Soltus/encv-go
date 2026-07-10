// 兼容垫片：任务 store 已提升为共享抽象层 @encv/shared-components/stores/taskStore
// 应用层通过依赖注入提供向量搜索 / IndexedDB 持久化（见 main.ts 的 registerSharedTaskServices）。

export type { DatePreset, SortBy, TriggeredBy, ViewMode } from "@encv/shared-components/stores/taskStore";
export { MAX_LOADED_TASKS, useTaskStore } from "@encv/shared-components/stores/taskStore";
