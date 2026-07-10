// Re-export shim: shared-components is the single source of truth for activeStatus.
// See docs/migration-task-system.md §8.1 (Module G — 通用抽象去重对齐).
// 用子路径导入（./composables/activeStatus），避免从 main barrel 导入时把 shared
// 里所有 .vue 组件一并拉进测试运行时（vitest 无 plugin-vue 会解析失败）。
export * from "@encv/shared-components/composables/activeStatus";
