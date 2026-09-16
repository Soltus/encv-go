/**
 * 🆕 2026-06-22 Q8B：任务类型查表（消除硬二分）
 *
 * 历史问题：
 * - `tk.type === 'encrypt' ? 'encrypt' : 'decrypt'` 硬二分
 * - 6 种类型（encrypt/decrypt/move/copy/rename/delete）都有专属 UI，但代码各处散落 switch/三元
 * - 后端 `Create("delete", ...)` 创建的 delete 任务被显示为"解密"
 *
 * 设计：
 * - 单一 typeMap（label/icon/color/handler）查表
 * - 统一 i18n 入口
 * - 6 种类型 + 6 种 rollback_* = 12 种类型全覆盖
 * - 未知类型走 'unknown' 兜底（i18n 提示 + 报告 issue）
 */
import type { TFunction } from "@encv/shared-components/composables/useI18n";

export type TaskType =
  | "encrypt"
  | "decrypt"
  | "move"
  | "copy"
  | "rename"
  | "delete"
  | "rollback_encrypt"
  | "rollback_decrypt"
  | "rollback_move"
  | "rollback_copy"
  | "rollback_rename"
  | "rollback_delete"
  | (string & {}); // 兜底字符串

export type TaskTypeMeta = {
  /** 短 label（i18n key） */
  labelKey: string;
  /** 兜底中文（i18n 缺失时显示） */
  fallbackLabel: string;
  /** ionicon */
  icon: string;
  /** ion-color */
  color: string;
  /** 是否可回滚 */
  rollbackable: boolean;
  /** 是否文件类（与目录项不同） */
  isFileOp: boolean;
  /** 类型分组（用于自动聚合展示） */
  group: "crypto" | "file" | "rollback" | "microservice";
};

export const TASK_TYPE_META: Record<string, TaskTypeMeta> = {
  encrypt: {
    labelKey: "tasks.type.encrypt",
    fallbackLabel: "加密",
    icon: "lock-closed",
    color: "primary",
    rollbackable: true,
    isFileOp: false,
    group: "crypto",
  },
  decrypt: {
    labelKey: "tasks.type.decrypt",
    fallbackLabel: "解密",
    icon: "lock-open",
    color: "success",
    rollbackable: true,
    isFileOp: false,
    group: "crypto",
  },
  move: {
    labelKey: "tasks.type.move",
    fallbackLabel: "移动",
    icon: "swap-horizontal",
    color: "warning",
    rollbackable: true,
    isFileOp: true,
    group: "file",
  },
  copy: {
    labelKey: "tasks.type.copy",
    fallbackLabel: "复制",
    icon: "copy",
    color: "tertiary",
    rollbackable: true,
    isFileOp: true,
    group: "file",
  },
  rename: {
    labelKey: "tasks.type.rename",
    fallbackLabel: "重命名",
    icon: "text",
    color: "medium",
    rollbackable: true,
    isFileOp: true,
    group: "file",
  },
  delete: {
    labelKey: "tasks.type.delete",
    fallbackLabel: "删除",
    icon: "trash",
    color: "danger",
    rollbackable: true,
    isFileOp: true,
    group: "file",
  },
  rollback_encrypt: {
    labelKey: "tasks.type.rollback_encrypt",
    fallbackLabel: "回滚加密",
    icon: "arrow-undo",
    color: "primary",
    rollbackable: false,
    isFileOp: false,
    group: "rollback",
  },
  rollback_decrypt: {
    labelKey: "tasks.type.rollback_decrypt",
    fallbackLabel: "回滚解密",
    icon: "arrow-undo",
    color: "success",
    rollbackable: false,
    isFileOp: false,
    group: "rollback",
  },
  rollback_move: {
    labelKey: "tasks.type.rollback_move",
    fallbackLabel: "回滚移动",
    icon: "arrow-undo",
    color: "warning",
    rollbackable: false,
    isFileOp: true,
    group: "rollback",
  },
  rollback_copy: {
    labelKey: "tasks.type.rollback_copy",
    fallbackLabel: "回滚复制",
    icon: "arrow-undo",
    color: "tertiary",
    rollbackable: false,
    isFileOp: true,
    group: "rollback",
  },
  rollback_rename: {
    labelKey: "tasks.type.rollback_rename",
    fallbackLabel: "回滚重命名",
    icon: "arrow-undo",
    color: "medium",
    rollbackable: false,
    isFileOp: true,
    group: "rollback",
  },
  rollback_delete: {
    labelKey: "tasks.type.rollback_delete",
    fallbackLabel: "回滚删除",
    icon: "arrow-undo",
    color: "danger",
    rollbackable: false,
    isFileOp: true,
    group: "rollback",
  },
};

const UNKNOWN_META: TaskTypeMeta = {
  labelKey: "tasks.type.unknown",
  fallbackLabel: "未知类型",
  icon: "help-circle",
  color: "medium",
  rollbackable: false,
  isFileOp: false,
  group: "crypto",
};

/**
 * 🆕 2026-07-03：微服务任务类型元数据
 *
 * 微服务任务类型采用 {service}.{method} 命名规范：
 *   - fts.rebuild / fts.search
 *   - vector.build_index / vector.search
 *   - cache.clean
 *   - db.backup / db.restore / db.vacuum
 *   - plugin.install / plugin.uninstall
 *   - tool.invoke
 *   - system.health
 *
 * 按服务名分组获取图标和颜色，无需为每种组合单独注册。
 */

// 微服务名 → 元数据映射
const MICROSERVICE_META: Record<string, { label: string; icon: string; color: string }> = {
  fts: { label: "全文索引", icon: "search", color: "primary" },
  vector: { label: "向量搜索", icon: "git-network", color: "tertiary" },
  cache: { label: "缓存", icon: "cloud", color: "warning" },
  db: { label: "数据库", icon: "server", color: "success" },
  plugin: { label: "插件", icon: "puzzle", color: "secondary" },
  tool: { label: "工具", icon: "hammer", color: "medium" },
  system: { label: "系统", icon: "settings", color: "dark" },
};

// 微服务方法名 → 中文 label 映射
const MICROSERVICE_METHOD_LABELS: Record<string, string> = {
  rebuild: "重建索引",
  search: "搜索",
  build_index: "构建索引",
  clean: "清理",
  backup: "备份",
  restore: "恢复",
  vacuum: "VACUUM",
  install: "安装",
  uninstall: "卸载",
  invoke: "调用",
  health: "健康检查",
};

/**
 * 判断是否为微服务任务类型（{service}.{method} 格式）。
 */
export function isMicroserviceType(type: string): boolean {
  return type.includes(".") && !type.startsWith("rollback_");
}

/**
 * 解析微服务任务类型，返回 serviceName 和 methodName。
 */
export function parseMicroserviceType(type: string): { serviceName: string; methodName: string } {
  const dotIndex = type.indexOf(".");
  if (dotIndex < 0) {
    return { serviceName: type, methodName: "" };
  }
  return {
    serviceName: type.slice(0, dotIndex),
    methodName: type.slice(dotIndex + 1),
  };
}

/**
 * 获取微服务任务的元数据（动态构造，无需提前注册所有组合）。
 */
export function getMicroserviceMeta(type: string): TaskTypeMeta {
  const { serviceName, methodName } = parseMicroserviceType(type);
  const svc = MICROSERVICE_META[serviceName] ?? {
    label: serviceName,
    icon: "settings-outline",
    color: "medium",
  };
  const methodLabel = MICROSERVICE_METHOD_LABELS[methodName] ?? methodName;
  return {
    labelKey: `tasks.type.${type}`,
    fallbackLabel: `${svc.label} · ${methodLabel}`,
    icon: svc.icon,
    color: svc.color,
    rollbackable: false,
    isFileOp: false,
    group: "microservice",
  };
}

/**
 * 查表获取类型元数据
 * 🆕 2026-07-03：支持微服务任务类型动态解析
 */
export function getTaskTypeMeta(type: string): TaskTypeMeta {
  if (isMicroserviceType(type)) {
    return getMicroserviceMeta(type);
  }
  return TASK_TYPE_META[type] ?? UNKNOWN_META;
}

/**
 * 查表获取显示 label（i18n 友好，缺失时 fallback）
 */
export function getTaskTypeLabel(type: string, t: TFunction): string {
  const meta = getTaskTypeMeta(type);
  const i18n = t(meta.labelKey);
  return i18n && i18n !== meta.labelKey ? i18n : meta.fallbackLabel;
}

/**
 * 查表获取 icon
 */
export function getTaskTypeIcon(type: string): string {
  return getTaskTypeMeta(type).icon;
}

/**
 * 查表获取 color
 */
export function getTaskTypeColor(type: string): string {
  return getTaskTypeMeta(type).color;
}

/**
 * 是否可回滚
 */
export function isRollbackable(type: string): boolean {
  return getTaskTypeMeta(type).rollbackable;
}

/**
 * 提取主类型（去 rollback_ 前缀）
 */
export function getBaseType(type: string): string {
  return type.startsWith("rollback_") ? type.slice(9) : type;
}

/**
 * 是否 rollback_* 类型
 */
export function isRollbackType(type: string): boolean {
  return type.startsWith("rollback_");
}
