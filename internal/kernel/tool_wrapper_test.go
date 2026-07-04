package kernel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ─── RegisterToolWithWrappers 基础测试 ──────────────────────────────────

func TestRegisterToolWithWrappers_NilPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for nil tool")
		}
	}()
	RegisterToolWithWrappers(nil, PermissionWrapper(nil))
}

func TestRegisterToolWithWrappers_NilWrapperPanics(t *testing.T) {
	UnregisterTool("nil-wrap-test")
	defer func() {
		recover()
		UnregisterTool("nil-wrap-test")
	}()
	RegisterToolWithWrappers(&echoTool{}, nil)
}

func TestRegisterToolWithWrappers_AppliesAndPreservesName(t *testing.T) {
	UnregisterTool("echo")
	defer UnregisterTool("echo")

	// PermissionWrapper(nil checkFn) = 透传
	RegisterToolWithWrappers(&echoTool{}, PermissionWrapper(nil))

	// echo 已被 wrap，但 Name() 仍返回 "echo"（wrapper 保留原 tool 名）
	echo, ok := GetTool("echo")
	if !ok {
		t.Fatal("echo not registered")
	}
	if echo.Name() != "echo" {
		t.Errorf("Name() = %q, want echo", echo.Name())
	}
	// 调用应正常透传
	resp, err := InvokeTool(NewContext(context.Background()), "echo", json.RawMessage(`{"msg":"hi"}`))
	if err != nil {
		t.Fatal(err)
	}
	var r struct{ Echoed string }
	json.Unmarshal(resp, &r)
	if r.Echoed != "hi" {
		t.Errorf("Echoed = %q", r.Echoed)
	}
}

// ─── PermissionWrapper 测试 ──────────────────────────────────────────────

func TestPermissionWrapper_Denies(t *testing.T) {
	UnregisterTool("echo")
	defer UnregisterTool("echo")

	RegisterToolWithWrappers(&echoTool{},
		PermissionWrapper(func(ctx ServiceContext, name string, args json.RawMessage) error {
			return errors.New("not allowed")
		}),
	)

	_, err := InvokeTool(NewContext(context.Background()), "echo", json.RawMessage(`{"msg":"x"}`))
	if !errors.Is(err, ErrPermissionDenied) {
		t.Errorf("expected ErrPermissionDenied, got %v", err)
	}
}

func TestPermissionWrapper_Allows(t *testing.T) {
	UnregisterTool("echo")
	defer UnregisterTool("echo")

	called := new(uint64)
	RegisterToolWithWrappers(&echoTool{},
		PermissionWrapper(func(ctx ServiceContext, name string, args json.RawMessage) error {
			atomic.AddUint64(called, 1)
			return nil // 允许
		}),
	)

	resp, err := InvokeTool(NewContext(context.Background()), "echo", json.RawMessage(`{"msg":"ok"}`))
	if err != nil {
		t.Fatal(err)
	}
	if atomic.LoadUint64(called) != 1 {
		t.Errorf("checkFn called = %d, want 1", atomic.LoadUint64(called))
	}
	var r struct{ Echoed string }
	json.Unmarshal(resp, &r)
	if r.Echoed != "ok" {
		t.Errorf("Echoed = %q", r.Echoed)
	}
}

func TestPermissionWrapper_NilCheckFn_Passthrough(t *testing.T) {
	UnregisterTool("echo")
	defer UnregisterTool("echo")

	RegisterToolWithWrappers(&echoTool{}, PermissionWrapper(nil))

	_, err := InvokeTool(NewContext(context.Background()), "echo", json.RawMessage(`{"msg":"y"}`))
	if err != nil {
		t.Errorf("nil checkFn should passthrough, got err: %v", err)
	}
}

// ─── AuditLogWrapper 测试 ────────────────────────────────────────────────

func TestAuditLogWrapper_LogsInvoke(t *testing.T) {
	UnregisterTool("echo")
	defer UnregisterTool("echo")

	// 用 testHandler 捕获日志
	handler := &testSlogHandler{mu: sync.Mutex{}, records: []slog.Record{}}
	logger := slog.New(handler)

	RegisterToolWithWrappers(&echoTool{}, AuditLogWrapper(logger))

	_, err := InvokeTool(NewContext(context.Background()), "echo", json.RawMessage(`{"msg":"log"}`))
	if err != nil {
		t.Fatal(err)
	}

	handler.mu.Lock()
	defer handler.mu.Unlock()
	if len(handler.records) < 2 {
		t.Fatalf("expected ≥2 log records (start+done), got %d", len(handler.records))
	}
	// 第一条应是 start
	startMsg := handler.records[0].Message
	if !strings.Contains(startMsg, "start") {
		t.Errorf("first record message = %q, want contains 'start'", startMsg)
	}
	// 验证 tool 字段
	var hasToolField bool
	handler.records[0].Attrs(func(a slog.Attr) bool {
		if a.Key == "tool" && a.Value.String() == "echo" {
			hasToolField = true
		}
		return true
	})
	if !hasToolField {
		t.Error("start record missing tool=echo field")
	}
}

func TestAuditLogWrapper_NilLogger_UsesDefault(t *testing.T) {
	UnregisterTool("echo")
	defer UnregisterTool("echo")

	// nil logger 不应 panic，用 slog.Default()
	RegisterToolWithWrappers(&echoTool{}, AuditLogWrapper(nil))

	_, err := InvokeTool(NewContext(context.Background()), "echo", json.RawMessage(`{"msg":"x"}`))
	if err != nil {
		t.Errorf("nil logger should not cause error: %v", err)
	}
}

// testSlogHandler 测试用 slog.Handler，捕获所有日志记录
type testSlogHandler struct {
	mu      sync.Mutex
	records []slog.Record
}

func (h *testSlogHandler) Enabled(ctx context.Context, level slog.Level) bool { return true }
func (h *testSlogHandler) WithAttrs(attrs []slog.Attr) slog.Handler           { return h }
func (h *testSlogHandler) WithGroup(name string) slog.Handler                 { return h }
func (h *testSlogHandler) Handle(ctx context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.records = append(h.records, r)
	return nil
}

// ─── RateLimitWrapper 测试 ───────────────────────────────────────────────

func TestRateLimitWrapper_AllowsBurstThenBlocks(t *testing.T) {
	UnregisterTool("echo")
	defer UnregisterTool("echo")

	// maxPerSec=2：允许瞬间 2 次，第 3 次被限流
	RegisterToolWithWrappers(&echoTool{}, RateLimitWrapper(2))

	ctx := NewContext(context.Background())
	r1, err1 := InvokeTool(ctx, "echo", json.RawMessage(`{"msg":"1"}`))
	r2, err2 := InvokeTool(ctx, "echo", json.RawMessage(`{"msg":"2"}`))
	_, err3 := InvokeTool(ctx, "echo", json.RawMessage(`{"msg":"3"}`))

	if err1 != nil || err2 != nil {
		t.Fatalf("first two should pass: err1=%v err2=%v", err1, err2)
	}
	_ = r1
	_ = r2
	if !errors.Is(err3, ErrRateLimited) {
		t.Errorf("third should be rate limited, got %v", err3)
	}
}

func TestRateLimitWrapper_RefillsOverTime(t *testing.T) {
	UnregisterTool("echo")
	defer UnregisterTool("echo")

	// maxPerSec=100：burst 100，但 100ms 后补充 10 个 token
	RegisterToolWithWrappers(&echoTool{}, RateLimitWrapper(100))

	ctx := NewContext(context.Background())
	emptyArgs := json.RawMessage(`{}`)
	// 用光 burst
	for i := 0; i < 100; i++ {
		_, err := InvokeTool(ctx, "echo", emptyArgs)
		if err != nil {
			t.Fatalf("burst[%d] failed: %v", i, err)
		}
	}
	// 第 101 次被限流
	_, err := InvokeTool(ctx, "echo", emptyArgs)
	if !errors.Is(err, ErrRateLimited) {
		t.Errorf("expected rate limit after burst, got %v", err)
	}

	// 等 50ms，补充 5 个 token
	time.Sleep(50 * time.Millisecond)
	for i := 0; i < 5; i++ {
		_, err := InvokeTool(ctx, "echo", emptyArgs)
		if err != nil {
			t.Errorf("after refill[%d]: %v", i, err)
		}
	}
}

func TestRateLimitWrapper_ZeroOrNegative_Passthrough(t *testing.T) {
	UnregisterTool("echo")
	defer UnregisterTool("echo")

	RegisterToolWithWrappers(&echoTool{}, RateLimitWrapper(0))

	ctx := NewContext(context.Background())
	emptyArgs := json.RawMessage(`{}`)
	for i := 0; i < 1000; i++ {
		_, err := InvokeTool(ctx, "echo", emptyArgs)
		if err != nil {
			t.Fatalf("passthrough[%d]: %v", i, err)
		}
	}
}

// ─── TimeoutWrapper 测试 ─────────────────────────────────────────────────

func TestTimeoutWrapper_TimesOut(t *testing.T) {
	UnregisterTool("slow-tool")
	defer UnregisterTool("slow-tool")

	// 100ms timeout，tool 内部 sleep 10s
	RegisterToolWithWrappers(&slowTool{}, TimeoutWrapper(100*time.Millisecond))

	_, err := InvokeTool(NewContext(context.Background()), "slow-tool", nil)
	if !errors.Is(err, ErrTimeout) {
		t.Errorf("expected ErrTimeout, got %v", err)
	}
}

func TestTimeoutWrapper_PassesThroughIfFast(t *testing.T) {
	UnregisterTool("echo")
	defer UnregisterTool("echo")

	RegisterToolWithWrappers(&echoTool{}, TimeoutWrapper(5*time.Second))

	_, err := InvokeTool(NewContext(context.Background()), "echo", json.RawMessage(`{"msg":"fast"}`))
	if err != nil {
		t.Errorf("fast tool should pass: %v", err)
	}
}

func TestTimeoutWrapper_NegativeDuration_Passthrough(t *testing.T) {
	UnregisterTool("slow-tool")
	defer UnregisterTool("slow-tool")

	RegisterToolWithWrappers(&slowTool{}, TimeoutWrapper(-1))

	// -1 = 透传，slowTool 会 sleep 10s，所以用 ctx 提前 cancel
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()
	_, err := InvokeTool(NewContext(ctx), "slow-tool", nil)
	if err == nil {
		t.Error("expected error from slow-tool after ctx cancel")
	}
}

func TestTimeoutWrapper_PreservesCtxFields(t *testing.T) {
	UnregisterTool("ctx-check-tool")
	defer UnregisterTool("ctx-check-tool")

	// 自定义 tool：检查 ctx 的 RequestID/TraceID/Service
	RegisterToolWithWrappers(&ctxCheckTool{}, TimeoutWrapper(5*time.Second))

	rootCtx := NewContext(context.Background(),
		WithTraceID("trace-timeout-ctx"),
		WithServiceName("client.test"),
	)
	_, err := InvokeTool(rootCtx, "ctx-check-tool", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("ctx-check-tool failed: %v", err)
	}
}

type ctxCheckTool struct{}

func (c *ctxCheckTool) Name() string            { return "ctx-check-tool" }
func (c *ctxCheckTool) Description() string     { return "checks ctx fields" }
func (c *ctxCheckTool) Schema() json.RawMessage { return json.RawMessage(`{}`) }
func (c *ctxCheckTool) Invoke(ctx ServiceContext, args json.RawMessage) (json.RawMessage, error) {
	// 关键：TimeoutWrapper 派生的 ctx 必须保留 RequestID/TraceID
	// Service 应该是 "tool.ctx-check-tool"（InvokeTool 包装）
	if ctx.Service() != "tool.ctx-check-tool" {
		return nil, fmt.Errorf("Service = %q, want tool.ctx-check-tool", ctx.Service())
	}
	if ctx.TraceID() != "trace-timeout-ctx" {
		return nil, fmt.Errorf("TraceID = %q, want trace-timeout-ctx", ctx.TraceID())
	}
	if ctx.RequestID() == "" {
		return nil, errors.New("RequestID is empty")
	}
	return json.Marshal(map[string]bool{"ok": true})
}

// ─── RetryWrapper 测试 ───────────────────────────────────────────────────

func TestRetryWrapper_RetriesRetryableError(t *testing.T) {
	UnregisterTool("retry-tool")
	defer UnregisterTool("retry-tool")

	ft := &flakyTool{failTimes: 2}
	RegisterToolWithWrappers(ft, RetryWrapper(3, 1*time.Millisecond))

	_, err := InvokeTool(NewContext(context.Background()), "retry-tool", nil)
	if err != nil {
		t.Errorf("should succeed after retries, got %v", err)
	}
	if ft.attempts != 3 {
		t.Errorf("attempts = %d, want 3 (2 fail + 1 success)", ft.attempts)
	}
}

func TestRetryWrapper_DoesNotRetryNonRetryable(t *testing.T) {
	UnregisterTool("retry-tool")
	defer UnregisterTool("retry-tool")

	ft := &flakyTool{failTimes: 5, nonRetryable: true}
	RegisterToolWithWrappers(ft, RetryWrapper(3, 1*time.Millisecond))

	_, err := InvokeTool(NewContext(context.Background()), "retry-tool", nil)
	if err == nil {
		t.Error("expected error (non-retryable should not retry)")
	}
	if ft.attempts != 1 {
		t.Errorf("attempts = %d, want 1 (no retry)", ft.attempts)
	}
}

func TestRetryWrapper_RespectsContextCancel(t *testing.T) {
	UnregisterTool("retry-tool")
	defer UnregisterTool("retry-tool")

	ft := &flakyTool{failTimes: 100}
	RegisterToolWithWrappers(ft, RetryWrapper(10, 100*time.Millisecond))

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()
	_, err := InvokeTool(NewContext(ctx), "retry-tool", nil)
	if err == nil {
		t.Error("expected error after ctx cancel")
	}
}

type flakyTool struct {
	failTimes     int32
	attempts      int32
	nonRetryable  bool
}

func (f *flakyTool) Name() string            { return "retry-tool" }
func (f *flakyTool) Description() string     { return "flaky" }
func (f *flakyTool) Schema() json.RawMessage { return json.RawMessage(`{}`) }
func (f *flakyTool) Invoke(ctx ServiceContext, args json.RawMessage) (json.RawMessage, error) {
	n := atomic.AddInt32(&f.attempts, 1)
	if int32(n) <= f.failTimes {
		if f.nonRetryable {
			return nil, errors.New("hard fail")
		}
		return nil, NewRetryable("flaky", errors.New("transient"))
	}
	return json.Marshal("ok")
}

// ─── ChainWrapper 测试 ───────────────────────────────────────────────────

func TestChainWrapper_ComposesMultiple(t *testing.T) {
	UnregisterTool("echo")
	defer UnregisterTool("echo")

	// 用 ChainWrapper 把 3 个 wrapper 预组合
	chain := ChainWrapper(
		PermissionWrapper(func(ctx ServiceContext, name string, args json.RawMessage) error {
			return nil // 允许
		}),
		AuditLogWrapper(nil), // nil = slog.Default()
		TimeoutWrapper(5 * time.Second),
	)

	RegisterToolWithWrappers(&echoTool{}, chain)

	_, err := InvokeTool(NewContext(context.Background()), "echo", json.RawMessage(`{"msg":"chain"}`))
	if err != nil {
		t.Errorf("chain should pass: %v", err)
	}
}

func TestChainWrapper_Empty_Noop(t *testing.T) {
	UnregisterTool("echo")
	defer UnregisterTool("echo")

	RegisterToolWithWrappers(&echoTool{}, ChainWrapper())

	_, err := InvokeTool(NewContext(context.Background()), "echo", json.RawMessage(`{"msg":"noop"}`))
	if err != nil {
		t.Errorf("empty chain should be noop: %v", err)
	}
}

// ─── 多 wrapper 顺序测试 ──────────────────────────────────────────────────
//
// 验证 wrapper 执行顺序：从外到内
// outer.before → middle.before → inner.before → tool.Invoke → inner.after → middle.after → outer.after
func TestWrapper_Order_FromOutsideToInside(t *testing.T) {
	UnregisterTool("order-tool")
	defer UnregisterTool("order-tool")

	var trace []string
	var mu sync.Mutex

	recordWrapper := func(name string) ToolWrapper {
		return func(next Tool) Tool {
			return &wrappedTool{
				inner: next,
				invoke: func(ctx ServiceContext, args json.RawMessage, _ func(ServiceContext, json.RawMessage) (json.RawMessage, error)) (json.RawMessage, error) {
					mu.Lock()
					trace = append(trace, name+".before")
					mu.Unlock()
					resp, err := next.Invoke(ctx, args)
					mu.Lock()
					trace = append(trace, name+".after")
					mu.Unlock()
					return resp, err
				},
			}
		}
	}

	RegisterToolWithWrappers(&orderTraceTool{trace: &trace, mu: &mu},
		recordWrapper("outer"), // 最外层
		recordWrapper("middle"),
		recordWrapper("inner"), // 最内层
	)

	_, err := InvokeTool(NewContext(context.Background()), "order-tool", nil)
	if err != nil {
		t.Fatal(err)
	}

	mu.Lock()
	defer mu.Unlock()
	expected := []string{
		"outer.before",
		"middle.before",
		"inner.before",
		"tool.invoke",
		"inner.after",
		"middle.after",
		"outer.after",
	}
	if len(trace) != len(expected) {
		t.Fatalf("trace length = %d, want %d. trace=%v", len(trace), len(expected), trace)
	}
	for i, want := range expected {
		if trace[i] != want {
			t.Errorf("trace[%d] = %q, want %q. full=%v", i, trace[i], want, trace)
		}
	}
}

type orderTraceTool struct {
	trace *[]string
	mu    *sync.Mutex
}

func (o *orderTraceTool) Name() string            { return "order-tool" }
func (o *orderTraceTool) Description() string     { return "trace order" }
func (o *orderTraceTool) Schema() json.RawMessage { return json.RawMessage(`{}`) }
func (o *orderTraceTool) Invoke(ctx ServiceContext, args json.RawMessage) (json.RawMessage, error) {
	o.mu.Lock()
	*o.trace = append(*o.trace, "tool.invoke")
	o.mu.Unlock()
	return json.Marshal("ok")
}

// ─── 完整链集成测试 ───────────────────────────────────────────────────────

func TestWrapperChain_FullIntegration(t *testing.T) {
	UnregisterTool("echo")
	defer UnregisterTool("echo")

	// 完整链：权限 → 审计 → 限流 → 超时 → 重试 → tool
	RegisterToolWithWrappers(&echoTool{},
		PermissionWrapper(func(ctx ServiceContext, name string, args json.RawMessage) error {
			return nil
		}),
		AuditLogWrapper(nil),
		RateLimitWrapper(1000),
		TimeoutWrapper(5*time.Second),
		RetryWrapper(2, 1*time.Millisecond),
	)

	resp, err := InvokeTool(NewContext(context.Background()), "echo", json.RawMessage(`{"msg":"full"}`))
	if err != nil {
		t.Fatalf("full chain failed: %v", err)
	}
	var r struct{ Echoed string }
	json.Unmarshal(resp, &r)
	if r.Echoed != "full" {
		t.Errorf("Echoed = %q, want full", r.Echoed)
	}
}

// ─── 编译期断言：确保 wrappedTool 实现 Tool 接口 ──────────────────────────
var _ Tool = (*wrappedTool)(nil)
