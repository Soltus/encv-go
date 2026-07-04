// Package tools — 统一错误类型（ToolError）。
//
// 设计动机（参考 .trae/specs/mobile-agent-polish-2026q2/spec.md §ToolError 统一异常类型）：
//   - 现有 ToolHandler 签名仍是 func(ctx, args, deps) (ToolResult, error)
//   - 错误已通过 ToolResult.IsError + Result JSON 传递到外层
//   - 但内部 handler 缺乏一个结构化的错误类型用于：
//     ① 携带错误码（ENOENT / TIMEOUT / INVALID_ARGS / ...）
//     ② 携带可恢复标志（重试决策依据）
//     ③ 携带底层错误（用于日志）
//     ④ 实现 errors.Unwrap 兼容 errors.Is / errors.As
//
// 关键约束（来自父任务 spec）：
//   - 不修改 ToolHandler 签名（保持向后兼容）
//   - ToolError 作为 *内部* 错误包装，handler 可选地返回它
//   - Dispatch 入口处统一把非 nil 错误 → ToolResult{IsError: true, Status: "failed"}
//
// 用法示例：
//
//	func myHandler(ctx context.Context, argsJSON string, deps *ToolDeps) (ToolResult, error) {
//	    if args.MountID == "" {
//	        return ToolResult{}, &ToolError{
//	            Code:    "INVALID_ARGS",
//	            Message: "mount_id is required",
//	        }
//	    }
//	    f, err := os.Open(path)
//	    if err != nil {
//	        return ToolResult{}, &ToolError{
//	            Code:        "ENOENT",
//	            Message:     fmt.Sprintf("file not found: %s", path),
//	            Underlying:  err,
//	            Recoverable: false,
//	        }
//	    }
//	    // ...
//	}
package tools

import (
	"errors"
	"fmt"
)

// 预定义错误码常量（便于上层做 switch / 国际化）。
//
// 不在此处强制所有 handler 使用，handler 可以自定 code。
const (
	// CodeInvalidArgs 参数不合法（缺字段、JSON 解析失败、类型不匹配）。
	CodeInvalidArgs = "INVALID_ARGS"
	// CodeMountNotFound mount_id 在 ResolveMount 中查不到。
	CodeMountNotFound = "MOUNT_NOT_FOUND"
	// CodePathEscape rel_path 试图越出 mount 根。
	CodePathEscape = "PATH_ESCAPE"
	// CodeENOENT 文件或目录不存在。
	CodeENOENT = "ENOENT"
	// CodeEACCES 权限不足。
	CodeEACCES = "EACCES"
	// CodePermissionDenied 显式拒绝（黑名单 / 沙箱违规）。
	CodePermissionDenied = "PERMISSION_DENIED"
	// CodeTimeout ctx 超时或 5s 上限命中。
	CodeTimeout = "TIMEOUT"
	// CodeExecFailed 底层命令执行失败（非零 exit code）。
	CodeExecFailed = "EXEC_FAILED"
	// CodeNotInWhitelist 命令名不在白名单。
	CodeNotInWhitelist = "NOT_IN_WHITELIST"
	// CodeUnsupportedPlatform 当前平台不支持此工具。
	CodeUnsupportedPlatform = "UNSUPPORTED_PLATFORM"
	// CodeCancelled 用户主动取消。
	CodeCancelled = "CANCELLED"
	// CodeUnknown 兜底未分类错误。
	CodeUnknown = "UNKNOWN"
)

// ToolError 是工具 handler 内部使用的结构化错误类型。
//
// 错误码语义：
//   - Code:    用于上层做 switch / 国际化，固定 snake_case 字符串
//   - Message: 给用户/UI 看的人类可读消息（已本地化或预留本地化）
//   - Underlying: 原始 Go error（用于日志，不应暴露给 UI）
//   - Recoverable: 是否可恢复（重试决策依据）
//
// ToolError 同时实现：
//   - error 接口（Error() 返回 Message）
//   - IsError() 辅助函数（用于显式 if 判定）
//   - errors.Unwrap（让 errors.Is / errors.As 透传到 Underlying）
type ToolError struct {
	// Code 错误代码（如 "ENOENT" / "PERMISSION_DENIED" / "TIMEOUT" / "INVALID_ARGS"）。
	Code string
	// Message 给用户看的本地化消息。
	Message string
	// Underlying 原始错误（用于日志）。可为 nil。
	Underlying error
	// Recoverable 是否可恢复（如重试可解决）。仅作为 hint。
	Recoverable bool
}

// Error 实现 error 接口。
func (e *ToolError) Error() string {
	if e == nil {
		return ""
	}
	if e.Code == "" {
		return e.Message
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Unwrap 返回底层 error，让 errors.Is / errors.As 透传。
func (e *ToolError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Underlying
}

// IsTarget 返回是否为指定错误码（对 errors.Is 友好）。
//
// 用法：
//
//	if errors.Is(err, &ToolError{Code: CodeTimeout}) { ... }
func (e *ToolError) IsTarget(target error) bool {
	if e == nil || target == nil {
		return false
	}
	t, ok := target.(*ToolError)
	if !ok {
		return false
	}
	return e.Code == t.Code
}

// IsError 是 helper：nil 接口 / nil 指针 → false。
func IsError(err error) bool {
	if err == nil {
		return false
	}
	var te *ToolError
	if errors.As(err, &te) {
		return te != nil
	}
	return true
}

// AsToolError 从 error 链中提取 *ToolError（找不到返回 nil）。
func AsToolError(err error) *ToolError {
	if err == nil {
		return nil
	}
	var te *ToolError
	if errors.As(err, &te) {
		return te
	}
	return nil
}

// NewToolError 是工厂函数：返回 *ToolError。
//
// 主要给 handler 用，避免到处写 &ToolError{...}：
//
//	return tools.NewToolError(tools.CodeENOENT, "file not found: "+path)
func NewToolError(code, message string) *ToolError {
	return &ToolError{Code: code, Message: message}
}

// Wrap 包装一个底层 error 为 *ToolError。
//
// 如果 err 已经是 *ToolError → 直接返回（不重复包装）。
// 如果 err 为 nil → 返回 nil。
//
// 用法：
//
//	f, err := os.Open(path)
//	if err != nil {
//	    return tools.Wrap(err, tools.CodeENOENT, "open file failed")
//	}
func Wrap(err error, code, message string) *ToolError {
	if err == nil {
		return nil
	}
	if te, ok := err.(*ToolError); ok {
		// 已是 ToolError → 优先用 caller 给的 code/message 覆盖（如果非空）
		if code == "" {
			code = te.Code
		}
		if message == "" {
			message = te.Message
		}
		return &ToolError{
			Code:        code,
			Message:     message,
			Underlying:  te.Underlying,
			Recoverable: te.Recoverable,
		}
	}
	return &ToolError{
		Code:       code,
		Message:    message,
		Underlying: err,
	}
}

// WrapWithRecoverable 同 Wrap，但设置 Recoverable 标志。
func WrapWithRecoverable(err error, code, message string, recoverable bool) *ToolError {
	te := Wrap(err, code, message)
	if te == nil {
		return nil
	}
	te.Recoverable = recoverable
	return te
}

// IsTimeout 便捷判断：err 是否为 timeout 类型 ToolError。
func IsTimeout(err error) bool {
	te := AsToolError(err)
	if te == nil {
		return false
	}
	return te.Code == CodeTimeout
}

// IsRecoverable 便捷判断：err 是否标记为可恢复。
func IsRecoverable(err error) bool {
	te := AsToolError(err)
	if te == nil {
		return false
	}
	return te.Recoverable
}
