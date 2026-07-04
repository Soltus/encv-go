package agent

import (
	"context"
	"errors"
	"net"
	"net/url"
	"strings"
	"testing"

	"github.com/Soltus/encv-go/internal/tools"
)

// ─── AppError / NewAppError 测试 ─────────────────────────────

func TestNewAppError_FillsHumanMessage(t *testing.T) {
	ae := tools.NewAppError(tools.AppErrNetworkUnavailable, "connection lost")
	if ae.HumanMessage == "" {
		t.Error("HumanMessage should be auto-filled from table")
	}
	if !strings.Contains(ae.HumanMessage, "🌐") {
		t.Errorf("NetworkUnavailable should contain 🌐 emoji, got: %q", ae.HumanMessage)
	}
}

func TestNewAppError_FillsIsRetryable(t *testing.T) {
	if !tools.NewAppError(tools.AppErrNetworkTimeout, "x").IsRetryable {
		t.Error("NetworkTimeout should be retryable")
	}
	if tools.NewAppError(tools.AppErrApiKeyInvalid, "x").IsRetryable {
		t.Error("ApiKeyInvalid should NOT be retryable")
	}
}

func TestNewAppError_UnknownType_FallsBackToDefault(t *testing.T) {
	ae := tools.NewAppError(tools.AppErrorType("totally_made_up"), "x")
	if ae.HumanMessage == "" {
		t.Error("unknown type should still get a fallback HumanMessage")
	}
}

// ─── FromHTTPStatusCode 测试 ────────────────────────────────

func TestFromHTTPStatusCode_401(t *testing.T) {
	ae := tools.FromHTTPStatusCode(401)
	if ae == nil {
		t.Fatal("401 should map to ApiKeyInvalid")
	}
	if ae.Type != tools.AppErrApiKeyInvalid {
		t.Errorf("401 should be ApiKeyInvalid, got %s", ae.Type)
	}
	if ae.IsRetryable {
		t.Error("ApiKeyInvalid should NOT be retryable")
	}
}

func TestFromHTTPStatusCode_402(t *testing.T) {
	ae := tools.FromHTTPStatusCode(402)
	if ae == nil || ae.Type != tools.AppErrInsufficientBalance {
		t.Error("402 should map to InsufficientBalance")
	}
}

func TestFromHTTPStatusCode_429(t *testing.T) {
	ae := tools.FromHTTPStatusCode(429)
	if ae == nil || ae.Type != tools.AppErrRateLimited {
		t.Error("429 should map to RateLimited")
	}
	if !ae.IsRetryable {
		t.Error("RateLimited should be retryable")
	}
}

func TestFromHTTPStatusCode_5xx(t *testing.T) {
	for _, code := range []int{500, 502, 503, 599} {
		ae := tools.FromHTTPStatusCode(code)
		if ae == nil || ae.Type != tools.AppErrServerError {
			t.Errorf("%d should map to ServerError", code)
		}
	}
}

func TestFromHTTPStatusCode_Other_ReturnsNil(t *testing.T) {
	for _, code := range []int{200, 301, 400, 404, 418} {
		ae := tools.FromHTTPStatusCode(code)
		if ae != nil {
			t.Errorf("code %d should NOT map to AppError, got %v", code, ae)
		}
	}
}

// ─── FromToolError 测试 ─────────────────────────────────────

func TestFromToolError_NilSafe(t *testing.T) {
	if ae := tools.FromToolError(nil); ae != nil {
		t.Error("nil ToolError should give nil AppError")
	}
}

func TestFromToolError_Timeout(t *testing.T) {
	te := &tools.ToolError{Code: tools.CodeTimeout, Message: "context deadline"}
	ae := tools.FromToolError(te)
	if ae == nil || ae.Type != tools.AppErrNetworkTimeout {
		t.Error("CodeTimeout should map to NetworkTimeout")
	}
}

func TestFromToolError_Cancelled(t *testing.T) {
	te := &tools.ToolError{Code: tools.CodeCancelled, Message: "user clicked cancel"}
	ae := tools.FromToolError(te)
	if ae == nil || ae.Type != tools.AppErrUserCancelled {
		t.Error("CodeCancelled should map to UserCancelled")
	}
}

func TestFromToolError_PreservesUnderlying(t *testing.T) {
	underlying := errors.New("os: file not found")
	te := &tools.ToolError{Code: tools.CodeENOENT, Message: "x", Underlying: underlying}
	ae := tools.FromToolError(te)
	if ae.Underlying != underlying {
		t.Error("Underlying should be preserved")
	}
}

// ─── ClassifyException 测试 ─────────────────────────────────

func TestClassifyException_Nil(t *testing.T) {
	if ae := ClassifyException(nil); ae != nil {
		t.Error("nil err should give nil AppError")
	}
}

func TestClassifyException_ContextCanceled(t *testing.T) {
	ae := ClassifyException(context.Canceled)
	if ae == nil || ae.Type != tools.AppErrUserCancelled {
		t.Error("context.Canceled should map to UserCancelled")
	}
}

func TestClassifyException_ContextDeadline(t *testing.T) {
	ae := ClassifyException(context.DeadlineExceeded)
	if ae == nil || ae.Type != tools.AppErrNetworkTimeout {
		t.Error("context.DeadlineExceeded should map to NetworkTimeout")
	}
}

func TestClassifyException_URLErrorTimeout(t *testing.T) {
	urlErr := &url.Error{Op: "Get", URL: "https://x", Err: &timeoutError{}}
	ae := ClassifyException(urlErr)
	if ae == nil || ae.Type != tools.AppErrNetworkTimeout {
		t.Error("url.Error with timeout should map to NetworkTimeout")
	}
}

func TestClassifyException_URLErrorWithStatus(t *testing.T) {
	urlErr := &url.Error{Op: "Post", URL: "https://api.deepseek", Err: errors.New("HTTP 429 Too Many Requests")}
	ae := ClassifyException(urlErr)
	if ae == nil || ae.Type != tools.AppErrRateLimited {
		t.Errorf("url.Error with HTTP 429 should map to RateLimited, got %v", ae)
	}
}

func TestClassifyException_NetOpErrorTimeout(t *testing.T) {
	netErr := &timeoutError{}
	ae := ClassifyException(netErr)
	if ae == nil || ae.Type != tools.AppErrNetworkTimeout {
		t.Error("net.Error with Timeout()=true should map to NetworkTimeout")
	}
}

func TestClassifyException_HeuristicTimeout(t *testing.T) {
	ae := ClassifyException(errors.New("read timeout after 30s"))
	if ae == nil || ae.Type != tools.AppErrNetworkTimeout {
		t.Error("message containing 'timeout' should map to NetworkTimeout")
	}
}

func TestClassifyException_HeuristicConnectionRefused(t *testing.T) {
	ae := ClassifyException(errors.New("dial tcp: connection refused"))
	if ae == nil || ae.Type != tools.AppErrNetworkUnavailable {
		t.Error("connection refused should map to NetworkUnavailable")
	}
}

func TestClassifyException_Heuristic401(t *testing.T) {
	ae := ClassifyException(errors.New("server returned status 401"))
	if ae == nil || ae.Type != tools.AppErrApiKeyInvalid {
		t.Error("message containing '401' should map to ApiKeyInvalid")
	}
}

func TestClassifyException_Heuristic5xx(t *testing.T) {
	ae := ClassifyException(errors.New("HTTP 502 Bad Gateway"))
	if ae == nil || ae.Type != tools.AppErrServerError {
		t.Error("HTTP 5xx should map to ServerError")
	}
}

func TestClassifyException_FallbackUnknown(t *testing.T) {
	ae := ClassifyException(errors.New("something weird happened"))
	if ae == nil || ae.Type != tools.AppErrUnknown {
		t.Error("unknown error should fall back to Unknown type")
	}
}

// 辅助：实现 net.Error 接口
type timeoutError struct{}

func (e *timeoutError) Error() string   { return "i/o timeout" }
func (e *timeoutError) Timeout() bool   { return true }
func (e *timeoutError) Temporary() bool { return true }

var _ net.Error = (*timeoutError)(nil)

// ─── ClassifyAndBuildPayload 测试 ───────────────────────────

func TestClassifyAndBuildPayload_NilError(t *testing.T) {
	p := ClassifyAndBuildPayload("test_code", nil)
	if p.Code != "test_code" {
		t.Errorf("Code = %q, want %q", p.Code, "test_code")
	}
}

func TestClassifyAndBuildPayload_Timeout(t *testing.T) {
	p := ClassifyAndBuildPayload("openai_error", context.DeadlineExceeded)
	if p.Type != string(tools.AppErrNetworkTimeout) {
		t.Errorf("Type = %q, want NetworkTimeout", p.Type)
	}
	if !p.IsRetryable {
		t.Error("NetworkTimeout should be retryable")
	}
	if p.HumanMessage == "" {
		t.Error("HumanMessage should be filled")
	}
}

func TestClassifyAndBuildPayload_Cancelled(t *testing.T) {
	p := ClassifyAndBuildPayload("user_cancelled", context.Canceled)
	if p.Type != string(tools.AppErrUserCancelled) {
		t.Errorf("Type = %q, want UserCancelled", p.Type)
	}
}
