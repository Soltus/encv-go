// encv.ts - 拆分后保留为 barrel re-export，向后兼容所有现有 `import ... from '@encv/shared-components/api/encv'`
// 实际实现见同目录 encv_*.ts 子模块

// 后端管理（权限、配置、日志、状态）
export * from "./encv_admin";
// 服务端 URL / 标识
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
// 文件操作
export * from "./encv_files";
// 扩展文件操作（重命名 / 复制 / 移动 / decrypt 错误）
export * from "./encv_files_extra";
// Openlist 站点 + alist decode
export * from "./encv_openlist";
// 性能 + 校准 + 数据库管理
export * from "./encv_perf";
// 插件 + 标签 + 挂载
export * from "./encv_plugins";
// 搜索 + 向量
export * from "./encv_search";

// 系统信息（build / container / ffmpeg / webdav local / manifest）
export * from "./encv_system";
// 任务 CRUD + run
export * from "./encv_tasks";
// 回收站
export * from "./encv_trash";
// WebDAV + Openlist
export * from "./encv_webdav";
