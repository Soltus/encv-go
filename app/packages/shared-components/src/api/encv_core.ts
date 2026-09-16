// 迁移垫片：服务端 URL / 标识 / 持久化常量真源已提升为共享基座
// @encv/shared-components/api/core（见 packages/shared-components/src/api/core）。
// 此处 re-export 以维持 `@encv/shared-components/api/encv_core` 既有导入兼容。
export * from "./core";
