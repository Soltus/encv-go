// 兼容垫片：运行态 task store 已提升为共享抽象层 @encv/shared-components/stores/runTasksStore
// getTasks 经依赖注入由应用层提供（见 main.ts 的 registerSharedTaskServices）。

export type { UseRunTasksStore } from "@encv/shared-components/stores/runTasksStore";
export { __resetRunTasksStoreForTests, useRunTasksStore, useRunTasksStoreSingleton } from "@encv/shared-components/stores/runTasksStore";
