package kernel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// ─── ToolWrapper：AI agent tool 拦截链 ───────────────────────────────────
//
// 设计目标：
//
//	在不修改 Tool 实现的前提下，给 tool 调用插入横切逻辑：
//	  - 权限检查（"用户能调 delete_file 吗？"）
//	  - 限流（"search tool 每秒最多 10 次"）
//	  - 审计日志（"谁在什么时候调了什么 tool，结果如何"）
//	  - 超时（"任何 tool 调用不能超过 30s"）
//	  - 重试（"网络工具失败自动重试 3 次"）
//	  - 熔断（"某 tool 连续失败时短路"）
//
// 用法：
//
//	// 1. 应用单个 wrapper
//	kernel.RegisterToolWithWrappers(myTool,
//	    kernel.PermissionWrapper(checkPermission),
//	)
//
//	// 2. 组合多个 wrapper（顺序：从外到内）
//	// PermissionWrapper → AuditLogWrapper → RateLimitWrapper → myTool
//	// 调用时：Permission 先检查，通过后 Audit 记录开始，RateLimit 限流，最后调 myTool
//	kernel.RegisterToolWithWrappers(myTool,
//	    kernel.PermissionWrapper(checkPermission),  // 最外层
//	    kernel.AuditLogWrapper(logger),
//	    kernel.RateLimitWrapper(10),                 // 最内层
//	)
//
// 顺序约定（与 HTTP middleware 一致）：
//
//	wrappers[0] 是最外层（最先看到 args，最后看到 response/error）
//	wrappers[len-1] 是最内层（最接近真实 tool）
//
// RegisterToolWithWrappers 内部从右往左 wrap：
//
//	t = myTool
//	t = RateLimitWrapper(t)   // wrappers[2]
//	t = AuditLogWrapper(t)    // wrappers[1]
//	t = PermissionWrapper(t)  // wrappers[0]
//	RegisterTool(t)

// ToolWrapper 包装一个 Tool，返回一个新的 Tool。
// 典型实现：在 Invoke 前后插入逻辑（权限/限流/日志/超时/重试/熔断）。
type ToolWrapper func(next Tool) Tool

// RegisterToolWithWrappers 注册 tool 并应用 wrapper 链。
//
// wrappers 顺序：从外到内（wrappers[0] 是最外层）。
// 内部从右往左 wrap：先 wrap 最内层（最接近真实 tool），最后 wrap 最外层。
//
// 注意：wrapper 会改变返回的 Tool 类型（wrappedTool），但 Name() 仍返回原始名字。
// 同名 panic（与 RegisterTool 一致）。
func RegisterToolWithWrappers(t Tool, wrappers ...ToolWrapper) {
	if t == nil {
		panic("kernel: RegisterToolWithWrappers(nil)")
	}
	// 从右往左 wrap
	for i := len(wrappers) - 1; i >= 0; i-- {
		if wrappers[i] == nil {
			panic(fmt.Sprintf("kernel: RegisterToolWithWrappers: wrapper[%d] is nil", i))
		}
		t = wrappers[i](t)
	}
	RegisterTool(t)
}

// wrappedTool 通用包装器：保留原 tool 的 Name/Description/Schema，
// 只重写 Invoke。wrapper 实现者用 wrappedTool 减少样板代码。
type wrappedTool struct {
	inner Tool
	invoke func(ctx ServiceContext, args json.RawMessage, next func(ServiceContext, json.RawMessage) (json.RawMessage, error)) (json.RawMessage, error)
}

func (w *wrappedTool) Name() string        { return w.inner.Name() }
func (w *wrappedTool) Description() string { return w.inner.Description() }
func (w *wrappedTool) Schema() json.RawMessage {
	return w.inner.Schema()
}

func (w *wrappedTool) Invoke(ctx ServiceContext, args json.RawMessage) (json.RawMessage, error) {
	if w.invoke == nil {
		return w.inner.Invoke(ctx, args)
	}
	return w.invoke(ctx, args, w.inner.Invoke)
}

// ─── 内置 wrapper：PermissionWrapper ──────────────────────────────────────
//
// 用法：
//
//	kernel.PermissionWrapper(func(ctx kernel.ServiceContext, toolName string, args json.RawMessage) error {
//	    if !userCanCallTool(ctx, toolName) {
//	        return errors.New("permission denied")
//	    }
//	    return nil
//	})
//
// checkFn 返回 error 时，Invoke 立即返回该 error，不调用 next。
// checkFn 为 nil = 允许所有（等价于不包装）。

// PermissionCheckFn 权限检查函数签名
type PermissionCheckFn func(ctx ServiceContext, toolName string, args json.RawMessage) error

// PermissionWrapper 权限检查 wrapper。
// checkFn 返回非 nil error → Invoke 立即返回 ErrPermissionDenied 包装的 error。
// checkFn 为 nil = 透传（不检查）。
func PermissionWrapper(checkFn PermissionCheckFn) ToolWrapper {
	return func(next Tool) Tool {
		return &wrappedTool{
			inner: next,
			invoke: func(ctx ServiceContext, args json.RawMessage, _ func(ServiceContext, json.RawMessage) (json.RawMessage, error)) (json.RawMessage, error) {
				if checkFn != nil {
					if err := checkFn(ctx, next.Name(), args); err != nil {
						return nil, fmt.Errorf("%w: tool %s: %v", ErrPermissionDenied, next.Name(), err)
					}
				}
				return next.Invoke(ctx, args)
			},
		}
	}
}

// ErrPermissionDenied 权限拒绝
var ErrPermissionDenied = errors.New("kernel: permission denied")

// ─── 内置 wrapper：AuditLogWrapper ────────────────────────────────────────
//
// 用法：
//
//	kernel.AuditLogWrapper(slog.Default())
//
// 每次 Invoke 前后记录日志：
//   - 调用前：tool 名 / args 摘要 / ctx.RequestID / ctx.TraceID
//   - 调用后：elapsed / 成功 or 失败 / error 摘要
//
// logger 为 nil = 使用 slog.Default()。
// 审计日志不改变 Invoke 行为（即使日志失败也继续）。

// AuditLogWrapper 审计日志 wrapper
func AuditLogWrapper(logger *slog.Logger) ToolWrapper {
	if logger == nil {
		logger = slog.Default()
	}
	return func(next Tool) Tool {
		return &wrappedTool{
			inner: next,
			invoke: func(ctx ServiceContext, args json.RawMessage, _ func(ServiceContext, json.RawMessage) (json.RawMessage, error)) (json.RawMessage, error) {
				start := time.Now()
				attrs := []any{
					"tool", next.Name(),
					"requestID", ctx.RequestID(),
					"traceID", ctx.TraceID(),
					"argsLen", len(args),
				}
				logger.InfoContext(ctx, "tool.invoke.start", attrs...)

				resp, err := next.Invoke(ctx, args)
				elapsed := time.Since(start)

				doneAttrs := []any{
					"tool", next.Name(),
					"requestID", ctx.RequestID(),
					"elapsed", elapsed.Milliseconds(),
					"ok", err == nil,
				}
				if err != nil {
					doneAttrs = append(doneAttrs, "err", err.Error())
					logger.WarnContext(ctx, "tool.invoke.done", doneAttrs...)
				} else {
					logger.InfoContext(ctx, "tool.invoke.done", doneAttrs...)
				}
				return resp, err
			},
		}
	}
}

// ─── 内置 wrapper：RateLimitWrapper ───────────────────────────────────────
//
// 用法：
//
//	kernel.RateLimitWrapper(10)  // 每秒最多 10 次
//
// 实现简单的 token bucket：
//   - 容量 = maxPerSec（允许瞬间 burst）
//   - 补充速率 = maxPerSec / sec
//   - 拿不到 token 时 Invoke 立即返回 ErrRateLimited（不阻塞）
//
// maxPerSec <= 0 = 不限流（透传）。
// 注意：每个 tool 实例独立的 bucket（不共享）。

// ErrRateLimited 限流
var ErrRateLimited = errors.New("kernel: rate limited")

// RateLimitWrapper 限流 wrapper（token bucket，每 tool 独立 bucket）
func RateLimitWrapper(maxPerSec int) ToolWrapper {
	if maxPerSec <= 0 {
		// 透传 wrapper（无限流）
		return func(next Tool) Tool { return next }
	}
	// bucket 状态：用闭包捕获，每个 wrapped tool 实例独立
	var mu sync.Mutex
	tokens := float64(maxPerSec)
	last := time.Now()
	rate := float64(maxPerSec) / float64(time.Second)
	capacity := float64(maxPerSec)

	return func(next Tool) Tool {
		return &wrappedTool{
			inner: next,
			invoke: func(ctx ServiceContext, args json.RawMessage, _ func(ServiceContext, json.RawMessage) (json.RawMessage, error)) (json.RawMessage, error) {
				mu.Lock()
				now := time.Now()
				elapsed := now.Sub(last)
				last = now
				// 补充 token
				tokens += float64(elapsed) * rate
				if tokens > capacity {
					tokens = capacity
				}
				if tokens < 1.0 {
					mu.Unlock()
					return nil, fmt.Errorf("%w: tool %s", ErrRateLimited, next.Name())
				}
				tokens -= 1.0
				mu.Unlock()
				return next.Invoke(ctx, args)
			},
		}
	}
}

// ─── 内置 wrapper：TimeoutWrapper ─────────────────────────────────────────
//
// 用法：
//
//	kernel.TimeoutWrapper(30 * time.Second)
//
// 给每次 Invoke 加 deadline。超时返回 ErrTimeout。
// 如果 ctx 已有更短的 deadline，以 ctx 为准（不延长）。
// d <= 0 = 不超时（透传）。

// ErrTimeout 工具调用超时
var ErrTimeout = errors.New("kernel: tool timeout")

// TimeoutWrapper 超时 wrapper
func TimeoutWrapper(d time.Duration) ToolWrapper {
	if d <= 0 {
		return func(next Tool) Tool { return next }
	}
	return func(next Tool) Tool {
		return &wrappedTool{
			inner: next,
			invoke: func(ctx ServiceContext, args json.RawMessage, _ func(ServiceContext, json.RawMessage) (json.RawMessage, error)) (json.RawMessage, error) {
				// 派生带 deadline 的 ctx
				timeoutCtx, cancel := context.WithTimeout(ctx, d)
				defer cancel()

				// 用 channel 等 Invoke 完成或 timeout
				type result struct {
					resp json.RawMessage
					err  error
				}
				done := make(chan result, 1)
				go func() {
					// 关键：把 timeoutCtx 包装回 ServiceContext，保留原 ctx 的字段
					// 但 deadline 用 timeoutCtx 的
					// created 优先用原 ctx 的，类型断言失败则用 now
					created := time.Now()
					if sc, ok := ctx.(*serviceCtx); ok {
						created = sc.created
					}
					wrappedCtx := &serviceCtx{
						parent:    timeoutCtx,
						service:   ctx.Service(),
						requestID: ctx.RequestID(),
						traceID:   ctx.TraceID(),
						created:   created,
						store:     checkpointStoreFrom(ctx),
					}
					resp, err := next.Invoke(wrappedCtx, args)
					done <- result{resp, err}
				}()

				select {
				case r := <-done:
					return r.resp, r.err
				case <-timeoutCtx.Done():
					if timeoutCtx.Err() == context.DeadlineExceeded {
						return nil, fmt.Errorf("%w: tool %s after %v", ErrTimeout, next.Name(), d)
					}
					// ctx 被其他原因 cancel（如父 ctx cancel）
					return nil, timeoutCtx.Err()
				}
			},
		}
	}
}

// ─── 内置 wrapper：RetryWrapper ───────────────────────────────────────────
//
// 用法：
//
//	kernel.RetryWrapper(3, 100*time.Millisecond)
//
// 工具返回 RetryableError 时自动重试，最多 maxRetries 次。
// 非可重试错误立即返回。ctx 取消立即返回。
// maxRetries <= 0 = 不重试（透传）。

// RetryWrapper 重试 wrapper（仅对 RetryableError 重试）
func RetryWrapper(maxRetries int, backoff time.Duration) ToolWrapper {
	if maxRetries <= 0 || backoff < 0 {
		return func(next Tool) Tool { return next }
	}
	return func(next Tool) Tool {
		return &wrappedTool{
			inner: next,
			invoke: func(ctx ServiceContext, args json.RawMessage, _ func(ServiceContext, json.RawMessage) (json.RawMessage, error)) (json.RawMessage, error) {
				var lastErr error
				for attempt := 0; attempt <= maxRetries; attempt++ {
					if err := ctx.Err(); err != nil {
						return nil, err
					}
					resp, err := next.Invoke(ctx, args)
					if err == nil {
						return resp, nil
					}
					lastErr = err
					// 只重试 RetryableError
					var re *RetryableError
					if !errors.As(err, &re) {
						return nil, err
					}
					// 等待 backoff 或 ctx cancel
					if attempt < maxRetries {
						select {
						case <-time.After(backoff):
						case <-ctx.Done():
							return nil, ctx.Err()
						}
					}
				}
				return nil, fmt.Errorf("kernel: tool %s failed after %d retries: %w", next.Name(), maxRetries, lastErr)
			},
		}
	}
}

// ─── 辅助：ChainWrapper ──────────────────────────────────────────────────
//
// 把多个 wrapper 预组合成一个 wrapper（便于复用）。
//
// 用法：
//
//	// 标准 agent tool 安全链
//	agentToolChain := kernel.ChainWrapper(
//	    kernel.PermissionWrapper(checkPermission),
//	    kernel.AuditLogWrapper(logger),
//	    kernel.TimeoutWrapper(30*time.Second),
//	)
//
//	// 注册多个 tool 时复用同一个 chain
//	kernel.RegisterToolWithWrappers(searchTool, agentToolChain)
//	kernel.RegisterToolWithWrappers(deleteTool, agentToolChain)

// ChainWrapper 把多个 wrapper 组合成一个 wrapper
func ChainWrapper(wrappers ...ToolWrapper) ToolWrapper {
	if len(wrappers) == 0 {
		return func(next Tool) Tool { return next } // noop
	}
	return func(next Tool) Tool {
		t := next
		for i := len(wrappers) - 1; i >= 0; i-- {
			if wrappers[i] == nil {
				continue
			}
			t = wrappers[i](t)
		}
		return t
	}
}
