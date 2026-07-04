// internal/tools/errors_test.go
//
// ToolError 单测（参考 .trae/specs/mobile-agent-polish-2026q2/spec.md §TestToolError 5+ 用例）
//
// 覆盖：
//  1. 基本包装：&ToolError{} 字段访问 / Error() / IsError() / AsToolError()
//  2. 错误码传递：errors.Is / AsToolError 提取 Code / Message / Underlying
//  3. 错误消息本地化：Message 字段直接返回（handler 自行负责 i18n）
//  4. Recoverable 标志：WrapWithRecoverable 正确设置
//  5. Wrap 工厂：nil error → nil；已是 ToolError → 不重复包装
//  6. IsTimeout / IsRecoverable 便捷判断
//  7. Unwrap 与 errors.Is 透传
//  8. Dispatch 集成：handler 返回 *ToolError → ToolResult.IsError=true
package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
)

// ─── 1. TestToolError_BasicFields ───────────────────────────────
//
// 验证基本字段：Code / Message / Underlying / Recoverable 都能被正确读出。
func TestToolError_BasicFields(t *testing.T) {
	inner := errors.New("disk I/O error")
	te := &ToolError{
		Code:        "EACCES",
		Message:     "permission denied",
		Underlying:  inner,
		Recoverable: false,
	}
	if te.Code != "EACCES" {
		t.Errorf("Code = %q, want EACCES", te.Code)
	}
	if te.Message != "permission denied" {
		t.Errorf("Message = %q, want 'permission denied'", te.Message)
	}
	if te.Underlying != inner {
		t.Errorf("Underlying = %v, want %v", te.Underlying, inner)
	}
	if te.Recoverable {
		t.Error("Recoverable = true, want false")
	}
}

// ─── 2. TestToolError_ErrorString ───────────────────────────────
//
// 验证 Error() 输出格式："{Code}: {Message}"，空 Code 时退化为 Message。
func TestToolError_ErrorString(t *testing.T) {
	te1 := &ToolError{Code: "ENOENT", Message: "file not found"}
	if got := te1.Error(); got != "ENOENT: file not found" {
		t.Errorf("Error() = %q, want 'ENOENT: file not found'", got)
	}
	te2 := &ToolError{Message: "generic failure"}
	if got := te2.Error(); got != "generic failure" {
		t.Errorf("Error() with empty Code = %q, want 'generic failure'", got)
	}
}

// ─── 3. TestToolError_IsError ──────────────────────────────────
//
// IsError(nil) → false；IsError(*ToolError) → true；IsError(bare error) → true。
func TestToolError_IsError(t *testing.T) {
	if IsError(nil) {
		t.Error("IsError(nil) = true, want false")
	}
	if !IsError(errors.New("any")) {
		t.Error("IsError(bare error) = false, want true")
	}
	if !IsError(&ToolError{Code: "X", Message: "y"}) {
		t.Error("IsError(*ToolError) = false, want true")
	}
}

// ─── 4. TestToolError_AsToolError ──────────────────────────────
//
// errors.As / AsToolError 能从包装链中提取 *ToolError。
func TestToolError_AsToolError(t *testing.T) {
	te := &ToolError{Code: "TIMEOUT", Message: "request timed out"}
	wrapped := fmt.Errorf("layer2: %w", te)

	got := AsToolError(wrapped)
	if got == nil {
		t.Fatal("AsToolError returned nil")
	}
	if got.Code != "TIMEOUT" {
		t.Errorf("Code = %q, want TIMEOUT", got.Code)
	}
	if got.Message != "request timed out" {
		t.Errorf("Message = %q, want 'request timed out'", got.Message)
	}
	if AsToolError(nil) != nil {
		t.Error("AsToolError(nil) should be nil")
	}
	if AsToolError(errors.New("bare")) != nil {
		t.Error("AsToolError(bare error) should be nil")
	}
}

// ─── 5. TestToolError_Unwrap ────────────────────────────────────
//
// ToolError.Unwrap 返回 Underlying，errors.Is 能透传到底层错误。
func TestToolError_Unwrap(t *testing.T) {
	sentinel := errors.New("sentinel")
	te := &ToolError{Code: "X", Message: "wrapped", Underlying: sentinel}

	if te.Unwrap() != sentinel {
		t.Errorf("Unwrap() = %v, want sentinel", te.Unwrap())
	}
	if !errors.Is(te, sentinel) {
		t.Error("errors.Is(te, sentinel) = false, want true")
	}
	// 透传链
	chain := fmt.Errorf("outer: %w", te)
	if !errors.Is(chain, sentinel) {
		t.Error("errors.Is 链式透传失败")
	}
}

// ─── 6. TestToolError_Wrap ──────────────────────────────────────
//
// Wrap 工厂：nil → nil；已是 *ToolError → 不重复包装 + 允许覆盖 code/message。
func TestToolError_Wrap(t *testing.T) {
	// nil → nil
	if got := Wrap(nil, "X", "y"); got != nil {
		t.Errorf("Wrap(nil) = %v, want nil", got)
	}
	// 裸 error → ToolError
	te := Wrap(errors.New("disk error"), "EIO", "I/O failure")
	if te == nil {
		t.Fatal("Wrap(bare) returned nil")
	}
	if te.Code != "EIO" || te.Message != "I/O failure" {
		t.Errorf("Wrap 字段不对: %+v", te)
	}
	if te.Underlying == nil || te.Underlying.Error() != "disk error" {
		t.Errorf("Underlying 未透传: %v", te.Underlying)
	}
	// 已是 ToolError → 允许 code/message 覆盖
	inner := &ToolError{Code: "A", Message: "m1", Underlying: errors.New("x")}
	wrapped := Wrap(inner, "B", "m2")
	if wrapped.Code != "B" || wrapped.Message != "m2" {
		t.Errorf("Wrap 覆盖失败: %+v", wrapped)
	}
	// 空 code/message 时保留原值
	wrapped2 := Wrap(inner, "", "")
	if wrapped2.Code != "A" || wrapped2.Message != "m1" {
		t.Errorf("Wrap 空覆盖应保留原值: %+v", wrapped2)
	}
}

// ─── 7. TestToolError_RecoverableFlag ──────────────────────────
//
// WrapWithRecoverable 正确设置 Recoverable，且 IsRecoverable 能识别。
func TestToolError_RecoverableFlag(t *testing.T) {
	te := WrapWithRecoverable(errors.New("net timeout"), "TIMEOUT", "网络超时", true)
	if !te.Recoverable {
		t.Error("Recoverable 未设置")
	}
	if !IsRecoverable(te) {
		t.Error("IsRecoverable(te) = false, want true")
	}
	if IsRecoverable(errors.New("not a ToolError")) {
		t.Error("IsRecoverable(bare) 应返回 false（兜底）")
	}
	te2 := WrapWithRecoverable(nil, "X", "y", true)
	if te2 != nil {
		t.Error("WrapWithRecoverable(nil) 应返回 nil")
	}
}

// ─── 8. TestToolError_IsTimeout ────────────────────────────────
//
// IsTimeout 便捷判断。
func TestToolError_IsTimeout(t *testing.T) {
	if !IsTimeout(&ToolError{Code: CodeTimeout}) {
		t.Error("IsTimeout(CodeTimeout) = false, want true")
	}
	if IsTimeout(&ToolError{Code: "ENOENT"}) {
		t.Error("IsTimeout(ENOENT) = true, want false")
	}
	if IsTimeout(nil) {
		t.Error("IsTimeout(nil) = true, want false")
	}
}

// ─── 9. TestToolError_DispatchIntegration ──────────────────────
//
// 验证 Dispatch 入口：handler 返回 *ToolError → ToolResult.IsError=true + Status=failed。
// 这是 Task 1.4 的核心集成测试。
//
// 注意：不调用 ResetGlobal() —— TestMain 已经注册了 high-level 工具。
// 这里只覆盖一个测试 tool，不影响其它。
func TestToolError_DispatchIntegration(t *testing.T) {
	_ = GlobalRegistry.Register(&ToolDef{
		Name:        "test_error_handler",
		Description: "x",
		ArgsSchema:  "{}",
		Handler: func(ctx context.Context, argsJSON string, deps *ToolDeps) (ToolResult, error) {
			return ToolResult{}, &ToolError{
				Code:        CodeInvalidArgs,
				Message:     "bad input",
				Recoverable: false,
			}
		},
	})

	res, err := GlobalRegistry.Dispatch(context.Background(), "test_error_handler", `{}`, nil)
	if err == nil {
		t.Fatal("Dispatch should return non-nil error")
	}
	if !res.IsError {
		t.Error("ToolResult.IsError = false, want true")
	}
	if res.Status != "failed" {
		t.Errorf("Status = %q, want failed", res.Status)
	}
	// 验证 Result JSON 含 code / message
	var payload map[string]string
	if e := json.Unmarshal([]byte(res.Result), &payload); e != nil {
		t.Fatalf("Result 不是 JSON: %v (raw=%s)", e, res.Result)
	}
	if payload["code"] != CodeInvalidArgs {
		t.Errorf("Result.code = %q, want %q", payload["code"], CodeInvalidArgs)
	}
	if payload["message"] != "bad input" {
		t.Errorf("Result.message = %q, want 'bad input'", payload["message"])
	}
}

// ─── 10. TestToolError_DispatchBareError ────────────────────────
//
// 验证 Dispatch 入口：handler 返回裸 error（不是 *ToolError）也能被规范化为 IsError=true。
func TestToolError_DispatchBareError(t *testing.T) {
	_ = GlobalRegistry.Register(&ToolDef{
		Name:        "test_bare_error",
		Description: "x",
		ArgsSchema:  "{}",
		Handler: func(ctx context.Context, argsJSON string, deps *ToolDeps) (ToolResult, error) {
			return ToolResult{}, errors.New("some unexpected error")
		},
	})

	res, err := GlobalRegistry.Dispatch(context.Background(), "test_bare_error", `{}`, nil)
	if err == nil {
		t.Fatal("Dispatch should return non-nil error")
	}
	if !res.IsError {
		t.Error("ToolResult.IsError = false, want true")
	}
	// 裸 error → code 应该是 UNKNOWN
	var payload map[string]string
	_ = json.Unmarshal([]byte(res.Result), &payload)
	if payload["code"] != CodeUnknown {
		t.Errorf("Result.code = %q, want %q (兜底)", payload["code"], CodeUnknown)
	}
}

// ─── 11. TestToolError_DispatchUnknownTool ──────────────────────
//
// 验证 Dispatch 入口：未知 tool 名 → IsError=true + 错误信息含 "unknown"。
func TestToolError_DispatchUnknownTool(t *testing.T) {
	res, err := GlobalRegistry.Dispatch(context.Background(), "nonexistent_tool", `{}`, nil)
	if err == nil {
		t.Fatal("Dispatch(unknown) should return error")
	}
	if !res.IsError {
		t.Error("ToolResult.IsError = false, want true")
	}
	if !strings.Contains(err.Error(), "unknown") {
		t.Errorf("error = %q, should contain 'unknown'", err.Error())
	}
}

// ─── 12. TestToolError_NewToolError ────────────────────────────
//
// NewToolError 工厂。
func TestToolError_NewToolError(t *testing.T) {
	te := NewToolError(CodeENOENT, "file not found")
	if te == nil || te.Code != CodeENOENT || te.Message != "file not found" {
		t.Errorf("NewToolError 字段不对: %+v", te)
	}
}

// ─── 13. TestToolError_DispatchPreservesResultData ──────────────
//
// 验证 Dispatch：当 handler 部分填充了 res.Result 时，error 包装会把
// 已有 data 保留在 "data" 字段中。
func TestToolError_DispatchPreservesResultData(t *testing.T) {
	_ = GlobalRegistry.Register(&ToolDef{
		Name:        "test_partial_result",
		Description: "x",
		ArgsSchema:  "{}",
		Handler: func(ctx context.Context, argsJSON string, deps *ToolDeps) (ToolResult, error) {
			// handler 部分填充 result，然后返回 error
			return ToolResult{Result: `{"partial_field":"hello"}`}, &ToolError{
				Code:    CodeExecFailed,
				Message: "exec failed midway",
			}
		},
	})

	res, err := GlobalRegistry.Dispatch(context.Background(), "test_partial_result", `{}`, nil)
	if err == nil {
		t.Fatal("Dispatch should return error")
	}
	if !res.IsError {
		t.Error("IsError should be true")
	}
	var payload map[string]interface{}
	if e := json.Unmarshal([]byte(res.Result), &payload); e != nil {
		t.Fatalf("Result 不是 JSON: %v", e)
	}
	if payload["code"] != CodeExecFailed {
		t.Errorf("code = %v, want %q", payload["code"], CodeExecFailed)
	}
	if _, ok := payload["data"]; !ok {
		t.Error("Result 应保留原 data 字段")
	}
}
