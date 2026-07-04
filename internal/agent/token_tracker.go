// Stage 6 (borrow-nuclear-boy-2026q2)：TokenTracker 实时追踪 token 用量、缓存命中率、平均延迟。
//
// 借鉴自 /tmp/nuclear-boy/api-deepseek/src/main/java/com/nuclearboy/api/deepseek/TokenTracker.kt。
//
// 关键模式：
//   - cache hit rate 是 per-request 而不是累计（更符合用户预期，nuclear-boy L147-150）
//   - 平均延迟用增量公式 `((cur.avg * count) + latency) / (count + 1)`，避免 sum/count 丢精度
//   - 线程安全（snapshot 暴露原子读）
package agent

import (
	"sync"
	"time"
)

// TokenSnapshot 实时 token 用量快照（前端 HUD 显示）。
// 对应 nuclear-boy TokenTracker.kt L16-35。
type TokenSnapshot struct {
	// TokensPerSecond 当前流式速率。
	TokensPerSecond float64 `json:"tokensPerSecond"`
	// ReasoningTokensPerSecond 思考 token 速率。
	ReasoningTokensPerSecond float64 `json:"reasoningTokensPerSecond"`
	// PromptTokensTotal 累计 prompt tokens（含历史所有请求）。
	PromptTokensTotal int64 `json:"promptTokensTotal"`
	// CompletionTokensTotal 累计 completion tokens。
	CompletionTokensTotal int64 `json:"completionTokensTotal"`
	// CachedTokensTotal 累计缓存命中 tokens。
	CachedTokensTotal int64 `json:"cachedTokensTotal"`
	// ReasoningTokensTotal 累计思考 tokens。
	ReasoningTokensTotal int64 `json:"reasoningTokensTotal"`
	// PromptTokensThisRequest 当前请求 prompt tokens。
	PromptTokensThisRequest int64 `json:"promptTokensThisRequest"`
	// CompletionTokensThisRequest 当前请求 completion tokens。
	CompletionTokensThisRequest int64 `json:"completionTokensThisRequest"`
	// CachedTokensThisRequest 当前请求缓存命中 tokens。
	CachedTokensThisRequest int64 `json:"cachedTokensThisRequest"`
	// CacheHitRate 0.0 - 1.0，per-request 命中率。
	CacheHitRate float64 `json:"cacheHitRate"`
	// ContextUsed 当前请求占用的 context tokens（通常 = PromptTokensThisRequest）。
	ContextUsed int64 `json:"contextUsed"`
	// ContextRemaining 剩余 context tokens。
	ContextRemaining int64 `json:"contextRemaining"`
	// ContextUsagePercent 0.0 - 1.0+。
	ContextUsagePercent float64 `json:"contextUsagePercent"`
	// EstimatedCostUsd 累计费用（USD）。
	EstimatedCostUsd float64 `json:"estimatedCostUsd"`
	// RequestCount 累计请求数。
	RequestCount int `json:"requestCount"`
	// AverageLatencyMs 平均延迟（ms，增量平均）。
	AverageLatencyMs int64 `json:"averageLatencyMs"`
}

// RequestUsage 单次请求的 usage 报告（来自 OpenAI API 响应）。
type RequestUsage struct {
	PromptTokens     int64
	CompletionTokens int64
	CachedTokens     int64 // 来自 prompt_tokens_details.cached_tokens
	ReasoningTokens  int64 // 来自 completion_tokens_details.reasoning_tokens
}

// TokenTracker 实时 token 追踪器。
//
// 借鉴 nuclear-boy TokenTracker.kt L50-223。
type TokenTracker struct {
	mu              sync.RWMutex
	snapshot        TokenSnapshot
	streamStartMs   int64
	lastStreamToken int64
	currentRequest  bool
}

// NewTokenTracker 构造追踪器。
func NewTokenTracker() *TokenTracker {
	return &TokenTracker{
		snapshot: TokenSnapshot{
			ContextRemaining: DeepSeekContextWindow,
		},
	}
}

// Snapshot 读当前快照（拷贝返回，调用方安全）。
func (t *TokenTracker) Snapshot() TokenSnapshot {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.snapshot
}

// StartRequest 标记新请求开始。
// promptTokens 是 LLM 上报前的预估值（来自前端），用于初始化 HUD。
func (t *TokenTracker) StartRequest(promptTokens int64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.streamStartMs = nowMs()
	t.lastStreamToken = t.streamStartMs
	t.currentRequest = true
	t.snapshot.PromptTokensThisRequest = promptTokens
	t.snapshot.CompletionTokensThisRequest = 0
	t.snapshot.CachedTokensThisRequest = 0
	t.snapshot.TokensPerSecond = 0
	t.snapshot.ReasoningTokensPerSecond = 0
}

// OnStreamToken 记录流式 token。
// isReasoning=true 表示思考 token（不计入 tps）。
func (t *TokenTracker) OnStreamToken(isReasoning bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.currentRequest {
		return
	}
	now := nowMs()
	elapsed := now - t.streamStartMs
	if elapsed <= 0 {
		elapsed = 1
	}
	t.lastStreamToken = now
	t.snapshot.CompletionTokensThisRequest++
	if !isReasoning {
		t.snapshot.TokensPerSecond = float64(t.snapshot.CompletionTokensThisRequest) * 1000.0 / float64(elapsed)
	} else {
		t.snapshot.ReasoningTokensPerSecond = float64(t.snapshot.CompletionTokensThisRequest) * 1000.0 / float64(elapsed)
	}
}

// OnRequestComplete 处理单次请求结束（OpenAI 报回 usage）。
//
// cache hit rate 是 per-request（不是累计）— 借鉴 nuclear-boy L147-150。
// average latency 用增量平均 — 借鉴 nuclear-boy L168-170。
func (t *TokenTracker) OnRequestComplete(usage RequestUsage, latencyMs int64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.currentRequest = false

	// per-request cache hit rate
	var hitRate float64
	if usage.PromptTokens > 0 {
		hitRate = float64(usage.CachedTokens) / float64(usage.PromptTokens)
	}

	cur := t.snapshot
	// 增量平均延迟：((cur.avg * count) + latency) / (count + 1)
	newAvgLatency := (cur.AverageLatencyMs*int64(cur.RequestCount) + latencyMs) / int64(cur.RequestCount+1)
	if newAvgLatency < 0 {
		newAvgLatency = 0
	}

	// tokens per second（本次请求速率）
	var speed float64
	if latencyMs > 0 {
		speed = float64(usage.CompletionTokens) * 1000.0 / float64(latencyMs)
	}

	t.snapshot = TokenSnapshot{
		TokensPerSecond:            speed,
		ReasoningTokensPerSecond:   cur.ReasoningTokensPerSecond, // 保留（无新思考数据）
		PromptTokensTotal:          cur.PromptTokensTotal + usage.PromptTokens,
		CompletionTokensTotal:      cur.CompletionTokensTotal + usage.CompletionTokens,
		CachedTokensTotal:          cur.CachedTokensTotal + usage.CachedTokens,
		ReasoningTokensTotal:       cur.ReasoningTokensTotal + usage.ReasoningTokens,
		PromptTokensThisRequest:    usage.PromptTokens,
		CompletionTokensThisRequest: usage.CompletionTokens,
		CachedTokensThisRequest:    usage.CachedTokens,
		CacheHitRate:               hitRate,
		ContextUsed:                usage.PromptTokens,
		ContextRemaining:           max64(0, DeepSeekContextWindow-usage.PromptTokens),
		ContextUsagePercent:        float64(usage.PromptTokens) / float64(DeepSeekContextWindow),
		EstimatedCostUsd:           cur.EstimatedCostUsd, // TODO: 接入 pricing
		RequestCount:               cur.RequestCount + 1,
		AverageLatencyMs:           newAvgLatency,
	}
}

// nowMs 当前毫秒时间戳（独立函数便于测试时 mock）。
func nowMs() int64 {
	return time.Now().UnixMilli()
}
func (t *TokenTracker) Reset() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.snapshot = TokenSnapshot{
		ContextRemaining: DeepSeekContextWindow,
	}
	t.currentRequest = false
}
