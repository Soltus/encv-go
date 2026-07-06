/**
 * twoLevelGrouping - 两级折叠行为共享常量
 *
 * 参照 codex-web-repo/apps/web/src/app/components/MessageBlocks.tsx:70
 *   const FILE_CHANGE_INITIAL_ROW_COUNT = 3
 *
 * 设计：
 * - 外层 OperationGroupSummary：单一摘要卡片（"已运行 N 条命令"/"已编辑 N 个文件"）
 * - 内层 OperationItemDetail 列表：默认折叠显示前 INITIAL_COUNT 条，展开后显示全部
 *
 * 边界处理：
 * - 总数 <= INITIAL_COUNT：直接展开（不显示"显示更多"按钮）
 * - 总数 >  INITIAL_COUNT：默认只显示前 INITIAL_COUNT 条，附"显示更多 (N)"按钮
 */
export const OPERATION_COLLAPSE_INITIAL_COUNT = 3;
