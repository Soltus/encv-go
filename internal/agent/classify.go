// Stage 5 (borrow-nuclear-boy-2026q2)：ClassifyException 把 Go 异常分类到 AppErrorType。
//
// 借鉴自 /tmp/nuclear-boy/agent-core/src/main/java/com/nuclearboy/agent/AgentEngine.kt L803-821：
//   fun classifyException(e: Throwable): AppError = when (e) {
//     is DeepSeekHttpException -> AppError.fromHttpCode(e.code) ?: AppError.ServerError
//     is SSLException -> AppError(NetworkUnavailable, ...)
//     is SocketTimeoutException -> AppError(NetworkTimeout, ...)
//     is IOException -> AppError(NetworkUnavailable, ...)
//     is CancellationException -> AppError(UserCancelled, ...)
//     else -> AppError(Unknown, ...)
//   }
package agent

import (
	"context"
	"errors"
	"net"
	"net/url"
	"strings"

	"github.com/Soltus/encv-go/internal/tools"
)

// ClassifyException 把 Go error 分类为 *AppError。
// 借鉴 nuclear-boy AgentEngine.kt L803-821。
//
// 映射规则：
//   - context.Canceled → UserCancelled
//   - context.DeadlineExceeded → NetworkTimeout
//   - net.Error{Timeout: true} → NetworkTimeout
//   - *url.Error{Timeout: true} → NetworkTimeout
//   - *url.Error 含 HTTP 状态码 → FromHTTPStatusCode
//   - syscall.ECONNREFUSED / ENETUNREACH → NetworkUnavailable
//   - 其他 → Unknown
func ClassifyException(err error) *tools.AppError {
	if err == nil {
		return nil
	}

	// context.Canceled → UserCancelled
	if errors.Is(err, context.Canceled) {
		return tools.NewAppError(tools.AppErrUserCancelled, err.Error())
	}

	// context.DeadlineExceeded → NetworkTimeout
	if errors.Is(err, context.DeadlineExceeded) {
		return tools.NewAppError(tools.AppErrNetworkTimeout, err.Error())
	}

	// *url.Error → 可能含 HTTP 状态码 / timeout
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		if urlErr.Timeout() {
			return tools.NewAppError(tools.AppErrNetworkTimeout, urlErr.Error())
		}
		// 尝试从字符串提取 HTTP 状态码（nuclear-boy 模式）
		if code := extractHTTPStatusFromMessage(urlErr.Error()); code != 0 {
			if ae := tools.FromHTTPStatusCode(code); ae != nil {
				return ae
			}
		}
	}

	// net.Error{Timeout: true} → NetworkTimeout
	var netErr net.Error
	if errors.As(err, &netErr) {
		if netErr.Timeout() {
			return tools.NewAppError(tools.AppErrNetworkTimeout, netErr.Error())
		}
	}

	// 字符串启发式（最后兜底）
	msg := err.Error()
	lower := strings.ToLower(msg)
	switch {
	case strings.Contains(lower, "timeout"):
		return tools.NewAppError(tools.AppErrNetworkTimeout, msg)
	case strings.Contains(lower, "connection refused"):
		return tools.NewAppError(tools.AppErrNetworkUnavailable, msg)
	case strings.Contains(lower, "no such host"):
		return tools.NewAppError(tools.AppErrNetworkUnavailable, msg)
	case strings.Contains(lower, "ssl"):
		return tools.NewAppError(tools.AppErrNetworkUnavailable, msg)
	case strings.Contains(lower, "401"):
		return tools.FromHTTPStatusCode(401)
	case strings.Contains(lower, "402"):
		return tools.FromHTTPStatusCode(402)
	case strings.Contains(lower, "429"):
		return tools.FromHTTPStatusCode(429)
	case strings.HasPrefix(msg, "HTTP 5") || strings.Contains(lower, "internal server error"):
		return tools.FromHTTPStatusCode(500)
	}

	// 兜底
	return tools.NewAppError(tools.AppErrUnknown, msg)
}

// extractHTTPStatusFromMessage 从错误消息字符串里提取 HTTP 状态码。
// 例 "API returned HTTP 429 Too Many Requests" → 429
func extractHTTPStatusFromMessage(msg string) int {
	// 查找 "HTTP NNN" 模式
	idx := strings.Index(msg, "HTTP ")
	if idx < 0 {
		return 0
	}
	rest := msg[idx+5:]
	// 截取 3 位数字
	if len(rest) < 3 {
		return 0
	}
	code := 0
	for i := 0; i < 3; i++ {
		c := rest[i]
		if c < '0' || c > '9' {
			return 0
		}
		code = code*10 + int(c-'0')
	}
	return code
}

// ClassifiedErrorPayload 是 emitClassifiedError 发到前端的 JSON 结构。
// 与 internal/tools/AppError 字段保持一致。
//
// 用途：让前端拿到结构化错误（type / humanMessage / isRetryable），
// 而不只是 string。Message 字段保留技术细节供 DevLogs 展示。
type ClassifiedErrorPayload struct {
	Code         string `json:"code"`         // 错误代号，如 "openai_error"
	Type         string `json:"type"`         // AppErrorType
	Message      string `json:"message"`      // 原始技术消息
	HumanMessage string `json:"humanMessage"` // 面向用户的友好文案
	IsRetryable  bool   `json:"isRetryable"`  // 是否可重试
}

// ClassifyAndBuildPayload 把 err 转 ClassifiedErrorPayload。
// 供 agent 包内部 emit error 时使用，避免在多处重复组装。
//
// 借鉴 nuclear-boy AgentEngine.kt L803-821 的 classifyException 模式。
func ClassifyAndBuildPayload(code string, err error) ClassifiedErrorPayload {
	if err == nil {
		return ClassifiedErrorPayload{Code: code, Type: string(tools.AppErrUnknown), HumanMessage: ""}
	}
	ae := ClassifyException(err)
	if ae == nil {
		// 不应发生（ClassifyException 永远返回非 nil）
		return ClassifiedErrorPayload{Code: code, Type: string(tools.AppErrUnknown), Message: err.Error()}
	}
	return ClassifiedErrorPayload{
		Code:         code,
		Type:         string(ae.Type),
		Message:      ae.Message,
		HumanMessage: ae.HumanMessage,
		IsRetryable:  ae.IsRetryable,
	}
}
