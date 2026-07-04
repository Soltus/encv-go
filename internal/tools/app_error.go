// Stage 5 (borrow-nuclear-boy-2026q2)：HumanMessage 本地化 + AppErrorType 枚举 + FromHTTPStatusCode。
//
// 借鉴自 /tmp/nuclear-boy/common/src/main/java/com/nuclearboy/common/AppError.kt：
//   - humanMessage 本地化（带 emoji）
//   - isRetryable 字段
//   - fromHttpCode 401/402/429/5xx → AppError
//
// 关键设计决策：
//   - **不**修改 ToolError 结构（避免破坏 mobile-agent-polish-2026q2 已有行为）
//   - 提供 AppErrorType 枚举 + HumanMessage 映射，独立模块
//   - 提供 FromHTTPStatusCode 静态方法
//   - 调用方：handler 返回 *ToolError 后，dispatch 入口处 wrap 成 *AppError
package tools

import "fmt"

// AppErrorType 描述错误的大类（用于 i18n + 决策）。
//
// 借鉴 nuclear-boy AppError.kt ErrorType 枚举。
type AppErrorType string

const (
	AppErrNetworkUnavailable    AppErrorType = "network_unavailable"
	AppErrNetworkTimeout        AppErrorType = "network_timeout"
	AppErrUserCancelled         AppErrorType = "user_cancelled"
	AppErrApiKeyInvalid         AppErrorType = "api_key_invalid"
	AppErrInsufficientBalance   AppErrorType = "insufficient_balance"
	AppErrRateLimited           AppErrorType = "rate_limited"
	AppErrServerError           AppErrorType = "server_error"
	AppErrUnknown               AppErrorType = "unknown"
)

// AppError 携带 type + 调试消息 + 本地化消息 + 可重试标志。
// 借鉴 nuclear-boy AppError.kt 完整字段。
type AppError struct {
	// Type 错误大类（用于 i18n + 决策树）。
	Type AppErrorType
	// Message 调试用英文 message。
	Message string
	// HumanMessage 给用户看的本地化消息（带 emoji）。
	HumanMessage string
	// IsRetryable 是否可重试。
	IsRetryable bool
	// Underlying 原始 error。
	Underlying error
}

// Error 实现 error 接口（返回 HumanMessage 优先）。
func (e *AppError) Error() string {
	if e == nil {
		return ""
	}
	if e.HumanMessage != "" {
		return e.HumanMessage
	}
	return e.Message
}

// Unwrap 返回底层 error。
func (e *AppError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Underlying
}

// humanMessageTable 是 AppErrorType → HumanMessage 的映射。
// 借鉴 nuclear-boy AppError.kt L60-68 中文文案。
var humanMessageTable = map[AppErrorType]string{
	AppErrNetworkUnavailable:  "网络好像断开了…等会儿再试？🌐",
	AppErrNetworkTimeout:      "网络有点慢，再试一次？⏱️",
	AppErrUserCancelled:       "已停止 ✋",
	AppErrApiKeyInvalid:       "API Key 不对了，去设置里检查一下？🔑",
	AppErrInsufficientBalance: "DeepSeek 余额不足 💸",
	AppErrRateLimited:         "调用太频繁了，休息一下再试 🐢",
	AppErrServerError:         "DeepSeek 服务器开了小差 😢",
	AppErrUnknown:             "出了点小问题 😅，要不要重试？",
}

// retryableTable 是 AppErrorType → IsRetryable 的映射。
// 借鉴 nuclear-boy AppError.kt isRetryable 字段。
var retryableTable = map[AppErrorType]bool{
	AppErrNetworkUnavailable:  true,
	AppErrNetworkTimeout:      true,
	AppErrUserCancelled:       false,
	AppErrApiKeyInvalid:       false,
	AppErrInsufficientBalance: false,
	AppErrRateLimited:         true,
	AppErrServerError:         true,
	AppErrUnknown:             true,
}

// NewAppError 是工厂函数，自动填充 HumanMessage + IsRetryable。
func NewAppError(t AppErrorType, message string) *AppError {
	hm, ok := humanMessageTable[t]
	if !ok {
		hm = "出了点小问题 😅"
	}
	r, ok := retryableTable[t]
	if !ok {
		r = false
	}
	return &AppError{
		Type:         t,
		Message:      message,
		HumanMessage: hm,
		IsRetryable:  r,
	}
}

// FromHTTPStatusCode 把 HTTP 状态码映射到 AppError。
// 借鉴 nuclear-boy AppError.kt fromHttpCode 静态方法。
//
// 状态码 → AppErrorType 映射：
//   - 401 → ApiKeyInvalid
//   - 402 → InsufficientBalance
//   - 429 → RateLimited
//   - 500-599 → ServerError
//   - 其他 → nil（不映射）
func FromHTTPStatusCode(code int) *AppError {
	var t AppErrorType
	switch {
	case code == 401:
		t = AppErrApiKeyInvalid
	case code == 402:
		t = AppErrInsufficientBalance
	case code == 429:
		t = AppErrRateLimited
	case code >= 500 && code < 600:
		t = AppErrServerError
	default:
		return nil
	}
	return NewAppError(t, fmt.Sprintf("HTTP %d", code))
}

// FromToolError 把 *ToolError 升级为 *AppError（如果可能）。
// 借鉴 nuclear-boy AppError.fromHttpCode + classifyException 模式。
func FromToolError(te *ToolError) *AppError {
	if te == nil {
		return nil
	}
	// 已知 Code 映射到 AppErrorType
	var t AppErrorType
	switch te.Code {
	case CodeTimeout:
		t = AppErrNetworkTimeout
	case CodeCancelled:
		t = AppErrUserCancelled
	case CodeEACCES, CodePermissionDenied:
		t = AppErrUnknown
	case CodeMountNotFound, CodeENOENT, CodePathEscape:
		t = AppErrUnknown
	case CodeInvalidArgs:
		t = AppErrUnknown
	default:
		t = AppErrUnknown
	}

	ae := NewAppError(t, te.Message)
	ae.Underlying = te.Underlying
	return ae
}

// HumanMessage 工具函数（独立可调用）。
func HumanMessage(t AppErrorType) string {
	if hm, ok := humanMessageTable[t]; ok {
		return hm
	}
	return ""
}

// IsRetryable 工具函数（独立可调用）。
func IsAppErrorRetryable(t AppErrorType) bool {
	if r, ok := retryableTable[t]; ok {
		return r
	}
	return false
}
