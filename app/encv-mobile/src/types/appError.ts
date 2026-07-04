// AppError TS 类型对齐 Go AppError (internal/tools/app_error.go)。
//
// 设计要点：
//   - Type 是字面量联合，编译器能穷尽 switch
//   - HumanMessage 是面向用户的文案（emoji + 中英文）
//   - IsRetryable 让 UI 决定是否显示"重试"按钮

/** AppErrorType 8 个枚举值（与 Go tools.AppErrorType 一一对应）。 */
export type AppErrorType =
  | "NetworkUnavailable"
  | "NetworkTimeout"
  | "ApiKeyInvalid"
  | "InsufficientBalance"
  | "RateLimited"
  | "ServerError"
  | "UserCancelled"
  | "Unknown";

/** AppError 结构（与 Go tools.AppError 对齐）。 */
export interface AppError {
  /** 错误分类 */
  type: AppErrorType;
  /** 技术性错误消息（开发调试用） */
  message: string;
  /** 面向用户的友好文案（emoji + 中英文） */
  humanMessage: string;
  /** 是否可以重试 */
  isRetryable: boolean;
}
