/**
 * messageStatus.ts — Stage 3 (borrow-nuclear-boy-2026q2)
 *
 * MessageStatus 8 状态机 + ToolCallStatus 5 状态机。
 * 借鉴自 /tmp/nuclear-boy/common/src/main/java/com/nuclearboy/common/Models.kt L57-66 + L80-82。
 *
 * 现状（encv-mobile 4 状态字符串）：'pending' | 'streaming' | 'complete' | 'error'
 * 新增（8 状态）：SENDING / SENT / THINKING / STREAMING / EXECUTING / COMPLETE / ERROR / CANCELLED
 *
 * 升级原因：
 *   - 缺 THINKING → 没法在前端区分"模型在思考" vs "模型在打字"
 *   - 缺 SENDING → 没法显示"发送中" spinner
 *   - 缺 CANCELLED → 用户取消时不知道该显示什么
 *   - 缺 EXECUTING → 没法精细展示"工具调用执行中"
 *
 * 设计：8 状态作为标准；保留 4 状态的字面量别名（向后兼容旧代码）。
 */

/** 标准 8 状态 */
export type MessageStatus =
  | "sending" // 准备发送（HTTP 请求未发出）
  | "sent" // 已发出，未收到响应
  | "thinking" // LLM 思考中（reasoning_content 流式）
  | "streaming" // 文本流式输出
  | "executing" // 工具调用执行中
  | "complete" // 完成
  | "error" // 失败
  | "cancelled"; // 用户取消

/** 5 状态工具调用 */
export type ToolCallStatus =
  | "pending" // 已收到 TOOL_CALL_START
  | "running" // args 累积中或 tool 执行中
  | "completed" // 成功完成
  | "failed" // 执行失败
  | "cancelled"; // 用户取消

/**
 * 旧 4 状态 → 新 8 状态映射（向后兼容）。
 * 来自 nuclear-boy 状态机升级经验：宁可保留所有旧字面量做兼容，
 * 也别在升级时破坏旧组件。
 */
export type LegacyMessageStatus = "pending" | "streaming" | "complete" | "error";

/**
 * 旧状态到新状态的转换函数。
 * 用法（升级 useAgent.ts 时）：
 *   const newStatus = migrateMessageStatus(oldStatus)
 */
export function migrateMessageStatus(old: LegacyMessageStatus | MessageStatus | string | undefined): MessageStatus {
  switch (old) {
    case "pending":
      return "sending";
    case "sent":
    case "thinking":
    case "streaming":
    case "executing":
    case "complete":
    case "error":
    case "cancelled":
      return old;
    case undefined:
    case "":
      return "sending";
    default:
      // 未知值降级为 sending
      return "sending";
  }
}

/**
 * 判断消息是否处于"进行中"状态。
 * 借鉴 nuclear-boy Models.kt L65 "isActive" 计算属性。
 */
export function isMessageActive(status: MessageStatus): boolean {
  return status === "sending" || status === "sent" || status === "thinking" || status === "streaming" || status === "executing";
}

/**
 * 判断工具调用是否处于"进行中"状态。
 */
export function isToolCallActive(status: ToolCallStatus): boolean {
  return status === "pending" || status === "running";
}

/**
 * UI 颜色映射（借鉴 nuclear-boy MessageBubble.kt 5 状态颜色编码）。
 * 复用 mobile-agent-polish-2026q2 已有的 ionic 颜色系统。
 */
export const MessageStatusColor: Record<MessageStatus, string> = {
  sending: "medium",
  sent: "medium",
  thinking: "tertiary",
  streaming: "primary",
  executing: "secondary",
  complete: "success",
  error: "danger",
  cancelled: "medium",
};

export const ToolCallStatusColor: Record<ToolCallStatus, string> = {
  pending: "medium",
  running: "primary",
  completed: "success",
  failed: "danger",
  cancelled: "medium",
};

/**
 * UI 显示文本（i18n 友好，借鉴 nuclear-boy 的人类可读命名）。
 */
export const MessageStatusText: Record<MessageStatus, string> = {
  sending: "发送中",
  sent: "已发送",
  thinking: "思考中",
  streaming: "生成中",
  executing: "执行中",
  complete: "完成",
  error: "失败",
  cancelled: "已取消",
};

export const ToolCallStatusText: Record<ToolCallStatus, string> = {
  pending: "等待",
  running: "执行中",
  completed: "完成",
  failed: "失败",
  cancelled: "已取消",
};
