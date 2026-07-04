package agent

import (
	"errors"
	"fmt"
)

// MaxToolIterations 是 ReAct 循环的显式上限。
// 借鉴自 /tmp/nuclear-boy/agent-core/src/main/java/com/nuclearboy/agent/AgentEngine.kt L166
// `private val maxToolIterations = 20`。
//
// 作用：防止 LLM 死循环（web_search 失败重试 100 次 / 工具结果注入后 LLM 再次调同工具）。
// 在 encv-go 中，`confirm` handler 触发 `executeAndRecurse` 构造 tool 消息后再 chat()，
// 没有显式上限 → 加 RecursionGuard 在 callOpenAIChatOnce 之前判断。
const MaxToolIterations = 20

// ErrMaxToolIterationsExceeded 当 ReAct 循环超过 maxToolIterations 时返回。
// 调用方应当把它翻译成 user-facing 错误（"已达到 20 轮工具调用上限"）+ 终止递归。
var ErrMaxToolIterationsExceeded = errors.New("已达到工具调用上限")

// RecursionGuard 跟踪单次对话的 ReAct 迭代次数。
//
// 用法（在 confirm handler 中）：
//
//	guard := agent.NewRecursionGuard(sess)
//	if err := guard.Increment(); err != nil {
//	    // 推 stream_error + stream_end，不再递归
//	    return
//	}
//	// ... 继续调 LLM
//
// 并发安全：sess.mu 已经在外部持锁；本类型自身**不**加锁（轻量）。
type RecursionGuard struct {
	// current 是当前迭代计数（从 0 开始）
	current int
	// max 是上限（默认 20）
	max int
}

// NewRecursionGuard 创建 RecursionGuard。
// max 传 0 或负数时用默认值 MaxToolIterations。
func NewRecursionGuard(current, max int) *RecursionGuard {
	if max <= 0 {
		max = MaxToolIterations
	}
	if current < 0 {
		current = 0
	}
	return &RecursionGuard{
		current: current,
		max:     max,
	}
}

// Increment 把迭代计数 +1 并检查是否超限。
// 返回 nil 表示未超限，可继续；返回 ErrMaxToolIterationsExceeded 表示已达上限。
func (g *RecursionGuard) Increment() error {
	g.current++
	if g.current > g.max {
		return fmt.Errorf("%w (max=%d, current=%d)", ErrMaxToolIterationsExceeded, g.max, g.current)
	}
	return nil
}

// Current 返回当前迭代计数（不含本轮自增）。
func (g *RecursionGuard) Current() int {
	return g.current
}

// Max 返回上限值。
func (g *RecursionGuard) Max() int {
	return g.max
}

// Reset 把计数重置为 0（用于新一轮对话）。
func (g *RecursionGuard) Reset() {
	g.current = 0
}

// Remaining 返回还剩多少轮可用（用于 UI 显示"还剩 5 轮"）。
func (g *RecursionGuard) Remaining() int {
	r := g.max - g.current
	if r < 0 {
		return 0
	}
	return r
}
