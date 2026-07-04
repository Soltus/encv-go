package kernel

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// ─── Tool：AI agent tool call interface（ctx-first） ────────────────────────
//
// 关键设计：
//
//  1. **ctx 一等**：每个 Tool.Invoke 都必须接受 ServiceContext。
//     AI agent 多轮对话的 trace 自动贯穿工具调用链。
//
//  2. **JSON Schema 描述**：Tool.Schema() 返回 OpenAI 兼容的 JSON Schema。
//     同一份描述既能喂给 LLM 也能用于运行时校验。
//
//  3. **可观测**：每次 Invoke 都记录 metrics + bus event ("tool.invoked")。
//     agent 决策可基于"工具调用的 P50 延迟"动态选择（fast tools first）。
//
//  4. **可拦截**：注册时通过 ToolWrapper 链（如 rate limit / audit log /
//     权限检查）。本骨架先实现 RegisterTool 直注册，wrapper 由调用方在
//     fn 内部组合。
//
//  5. **可恢复**：Invoke 内部可 Checkpoint（ctx 启用 store 时），
//     WorkManager 重启后 agent 重发 tool call，kernel 自动 Restore 续跑。
//
//  6. **可重试**：tool 内 Call 失败时由 tool 内部决定（部分工具幂等可重试，
//     部分副作用不可）。本骨架提供 RetryableError 类型供工具标识可重试错误。
type Tool interface {
	// Name 工具名（agent 调用的 key）
	Name() string

	// Description 工具描述（喂给 LLM 决定是否调用）
	Description() string

	// Schema 入参的 JSON Schema（OpenAI tools[].parameters 格式）
	Schema() json.RawMessage

	// Invoke 实际调用。args 是 LLM 输出的 json 参数。
	// 返回值是 json 序列化结果（喂回 LLM 作为 observation）。
	Invoke(ctx ServiceContext, args json.RawMessage) (json.RawMessage, error)
}

// ─── 注册 ────────────────────────────────────────────────

// RegisterTool 注册一个 tool。同名 panic。
func RegisterTool(t Tool) {
	if t == nil {
		panic("kernel: RegisterTool(nil)")
	}
	if t.Name() == "" {
		panic("kernel: RegisterTool with empty Name()")
	}
	toolMu.Lock()
	defer toolMu.Unlock()
	if _, exists := tools[t.Name()]; exists {
		panic(fmt.Sprintf("kernel: tool %q already registered", t.Name()))
	}
	tools[t.Name()] = t
}

// MustGetTool 取一个 tool
func MustGetTool(name string) Tool {
	t, ok := GetTool(name)
	if !ok {
		panic(fmt.Sprintf("kernel: tool %q not registered", name))
	}
	return t
}

// GetTool 取一个 tool
func GetTool(name string) (Tool, bool) {
	toolMu.RLock()
	defer toolMu.RUnlock()
	t, ok := tools[name]
	return t, ok
}

// ListTools 列出所有 tool 名
func ListTools() []string {
	toolMu.RLock()
	defer toolMu.RUnlock()
	out := make([]string, 0, len(tools))
	for n := range tools {
		out = append(out, n)
	}
	return out
}

// UnregisterTool 注销 tool（测试用）
func UnregisterTool(name string) {
	toolMu.Lock()
	defer toolMu.Unlock()
	delete(tools, name)
}

// ─── 调用 ────────────────────────────────────────────────

// InvokeTool 调用一个 tool。
//
// 设计要点：
//   - ctx 派生子 ctx（service 字段 = tool name）
//   - ctx.Err() 优先检查（gin 客户端断开 / agent 取消都应快速 fail）
//   - 调工具前 args 校验（如果有 schema）
//   - 调完发 bus event "tool.invoked"（异步，fire-and-forget）
//   - 错误包装：ctx deadline / tool not found / tool internal error
func InvokeTool(ctx ServiceContext, name string, args json.RawMessage) (json.RawMessage, error) {
	if ctx == nil {
		return nil, errors.New("kernel: nil context")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	t, ok := GetTool(name)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrToolNotFound, name)
	}

	// 派生子 ctx
	childCtx := &serviceCtx{
		parent:    ctx,
		service:   "tool." + name,
		requestID: ctx.RequestID(),
		traceID:   ctx.TraceID(),
		created:   time.Now(),
		store:     checkpointStoreFrom(ctx),
	}

	start := time.Now()
	resp, err := t.Invoke(childCtx, args)
	elapsed := time.Since(start)

	// 埋点
	recordToolInvoke(name, err, elapsed)

	// 异步发 bus event（不影响主流程）
	PublishAsync(ctx, "tool.invoked", map[string]any{
		"tool":    name,
		"elapsed": elapsed.Milliseconds(),
		"ok":      err == nil,
		"err":     errStr(err),
	})

	if err != nil {
		return nil, fmt.Errorf("kernel: tool %q failed after %v: %w", name, elapsed, err)
	}
	return resp, nil
}

// InvokeToolTyped 类型安全调用 wrapper（与 CallTyped 类似）
func InvokeToolTyped[Args, Resp any](ctx ServiceContext, name string, args Args) (Resp, error) {
	var zero Resp
	raw, err := json.Marshal(args)
	if err != nil {
		return zero, fmt.Errorf("kernel: marshal args for tool %q: %w", name, err)
	}
	respRaw, err := InvokeTool(ctx, name, raw)
	if err != nil {
		return zero, err
	}
	var resp Resp
	if err := json.Unmarshal(respRaw, &resp); err != nil {
		return zero, fmt.Errorf("kernel: unmarshal %s response: %w", name, err)
	}
	return resp, nil
}

// ─── 错误类型 ─────────────────────────────────────────────

// RetryableError 标识此错误可重试。tool 内部 / kernel / 上层 agent
// 可根据 errors.Is(err, &RetryableError{}) 决定重试。
type RetryableError struct {
	Reason string
	Err    error
}

func (e *RetryableError) Error() string {
	if e.Err == nil {
		return "retryable: " + e.Reason
	}
	return fmt.Sprintf("retryable: %s: %v", e.Reason, e.Err)
}
func (e *RetryableError) Unwrap() error { return e.Err }

// NewRetryable 构造一个可重试错误
func NewRetryable(reason string, err error) error {
	return &RetryableError{Reason: reason, Err: err}
}

// ─── 埋点 ────────────────────────────────────────────────

var (
	toolCount   sync.Map // key: tool name → *uint64
	toolLatency sync.Map // key: tool name → *uint64
	toolErr     sync.Map // key: tool name → *uint64
)

func recordToolInvoke(name string, err error, elapsed time.Duration) {
	cnt, _ := toolCount.LoadOrStore(name, new(uint64))
	atomic.AddUint64(cnt.(*uint64), 1)
	lat, _ := toolLatency.LoadOrStore(name, new(uint64))
	atomic.AddUint64(lat.(*uint64), uint64(elapsed))
	if err != nil {
		eCnt, _ := toolErr.LoadOrStore(name, new(uint64))
		atomic.AddUint64(eCnt.(*uint64), 1)
	}
}

// ToolStats 工具调用统计
type ToolStats struct {
	Name    string        `json:"name"`
	Count   uint64        `json:"count"`
	Errors  uint64        `json:"errors"`
	AvgTime time.Duration `json:"avgTime"`
}

// ToolStatsAll 列出所有工具的统计
func ToolStatsAll() []ToolStats {
	names := ListTools()
	out := make([]ToolStats, 0, len(names))
	for _, n := range names {
		cnt, lat, eCnt := loadToolStats(n)
		avg := time.Duration(0)
		if cnt > 0 {
			avg = time.Duration(lat / cnt)
		}
		out = append(out, ToolStats{
			Name:    n,
			Count:   cnt,
			Errors:  eCnt,
			AvgTime: avg,
		})
	}
	return out
}

func loadToolStats(name string) (uint64, uint64, uint64) {
	cnt, _ := toolCount.Load(name)
	lat, _ := toolLatency.Load(name)
	eCnt, _ := toolErr.Load(name)
	var c, l, e uint64
	if cnt != nil {
		c = atomic.LoadUint64(cnt.(*uint64))
	}
	if lat != nil {
		l = atomic.LoadUint64(lat.(*uint64))
	}
	if eCnt != nil {
		e = atomic.LoadUint64(eCnt.(*uint64))
	}
	return c, l, e
}

// ResetToolStats 重置某工具的统计数据（测试用）
func ResetToolStats(name string) {
	toolCount.Store(name, new(uint64))
	toolLatency.Store(name, new(uint64))
	toolErr.Store(name, new(uint64))
}

func errStr(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
