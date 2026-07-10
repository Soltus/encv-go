// Re-export shim: shared-components is the single source of truth for showToast.
// See docs/migration-task-system.md §8.1 (Module G — 通用抽象去重对齐).
// 应用层其余 35+ 处 `import { showToast } from "@/composables/useToast"` 可保持不变。
// 用子路径导入（./composables/useToast），避免从 main barrel 导入时把 shared 里
// 所有 .vue 组件一并拉进测试运行时（vitest 无 plugin-vue 会解析失败）。
export { showToast } from "@encv/shared-components/composables/useToast";
