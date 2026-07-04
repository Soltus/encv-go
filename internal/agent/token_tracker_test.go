package agent

import "testing"

// ─── TokenTracker.StartRequest ───────────────────────────────

func TestTokenTracker_InitialSnapshot_Empty(t *testing.T) {
	tr := NewTokenTracker()
	s := tr.Snapshot()
	if s.RequestCount != 0 {
		t.Errorf("initial RequestCount = %d, want 0", s.RequestCount)
	}
	if s.ContextRemaining != DeepSeekContextWindow {
		t.Errorf("initial ContextRemaining = %d, want %d", s.ContextRemaining, DeepSeekContextWindow)
	}
}

func TestTokenTracker_StartRequest_InitializesPrompt(t *testing.T) {
	tr := NewTokenTracker()
	tr.StartRequest(1500)
	s := tr.Snapshot()
	if s.PromptTokensThisRequest != 1500 {
		t.Errorf("PromptTokensThisRequest = %d, want 1500", s.PromptTokensThisRequest)
	}
	if s.CompletionTokensThisRequest != 0 {
		t.Errorf("CompletionTokensThisRequest should be 0, got %d", s.CompletionTokensThisRequest)
	}
}

// ─── TokenTracker.OnRequestComplete ──────────────────────────

func TestTokenTracker_OnRequestComplete_UpdatesTotals(t *testing.T) {
	tr := NewTokenTracker()
	tr.StartRequest(1000)
	tr.OnRequestComplete(RequestUsage{
		PromptTokens:     1000,
		CompletionTokens: 200,
		CachedTokens:     300,
	}, 500)

	s := tr.Snapshot()
	if s.PromptTokensTotal != 1000 {
		t.Errorf("PromptTokensTotal = %d, want 1000", s.PromptTokensTotal)
	}
	if s.CompletionTokensTotal != 200 {
		t.Errorf("CompletionTokensTotal = %d, want 200", s.CompletionTokensTotal)
	}
	if s.CachedTokensTotal != 300 {
		t.Errorf("CachedTokensTotal = %d, want 300", s.CachedTokensTotal)
	}
	if s.RequestCount != 1 {
		t.Errorf("RequestCount = %d, want 1", s.RequestCount)
	}
}

func TestTokenTracker_OnRequestComplete_PerRequestCacheRate(t *testing.T) {
	tr := NewTokenTracker()
	tr.StartRequest(1000)
	tr.OnRequestComplete(RequestUsage{
		PromptTokens:     1000,
		CompletionTokens: 100,
		CachedTokens:     250, // 25% hit rate
	}, 400)

	s := tr.Snapshot()
	if s.CacheHitRate < 0.24 || s.CacheHitRate > 0.26 {
		t.Errorf("CacheHitRate = %f, want ~0.25 (per-request, not cumulative)", s.CacheHitRate)
	}
}

func TestTokenTracker_OnRequestComplete_IncrementalAverageLatency(t *testing.T) {
	tr := NewTokenTracker()

	// 第 1 次：latency=1000ms，avg=1000
	tr.StartRequest(500)
	tr.OnRequestComplete(RequestUsage{PromptTokens: 500, CompletionTokens: 50}, 1000)
	if avg := tr.Snapshot().AverageLatencyMs; avg != 1000 {
		t.Errorf("after 1st req, avg = %d, want 1000", avg)
	}

	// 第 2 次：latency=500ms，((1000*1)+500)/2 = 750
	tr.StartRequest(600)
	tr.OnRequestComplete(RequestUsage{PromptTokens: 600, CompletionTokens: 60}, 500)
	if avg := tr.Snapshot().AverageLatencyMs; avg != 750 {
		t.Errorf("after 2nd req, avg = %d, want 750 (incremental)", avg)
	}

	// 第 3 次：latency=250ms，((750*2)+250)/3 = 583
	tr.StartRequest(700)
	tr.OnRequestComplete(RequestUsage{PromptTokens: 700, CompletionTokens: 70}, 250)
	if avg := tr.Snapshot().AverageLatencyMs; avg != 583 {
		t.Errorf("after 3rd req, avg = %d, want 583 (incremental)", avg)
	}
}

func TestTokenTracker_OnRequestComplete_ContextRemaining(t *testing.T) {
	tr := NewTokenTracker()
	tr.StartRequest(100_000)
	tr.OnRequestComplete(RequestUsage{PromptTokens: 100_000, CompletionTokens: 500}, 1000)

	s := tr.Snapshot()
	if s.ContextUsed != 100_000 {
		t.Errorf("ContextUsed = %d, want 100000", s.ContextUsed)
	}
	if s.ContextRemaining != DeepSeekContextWindow-100_000 {
		t.Errorf("ContextRemaining = %d, want %d", s.ContextRemaining, DeepSeekContextWindow-100_000)
	}
}

func TestTokenTracker_OnRequestComplete_ContextRemainingNeverNegative(t *testing.T) {
	tr := NewTokenTracker()
	tr.StartRequest(DeepSeekContextWindow + 1000) // overflow
	tr.OnRequestComplete(RequestUsage{PromptTokens: DeepSeekContextWindow + 1000, CompletionTokens: 1}, 100)

	if rem := tr.Snapshot().ContextRemaining; rem != 0 {
		t.Errorf("over-budget ContextRemaining = %d, want 0", rem)
	}
}

// ─── TokenTracker.OnStreamToken ──────────────────────────────

func TestTokenTracker_OnStreamToken_IgnoresOutsideRequest(t *testing.T) {
	tr := NewTokenTracker()
	// 不 StartRequest 直接 OnStreamToken
	tr.OnStreamToken(false)
	if got := tr.Snapshot().CompletionTokensThisRequest; got != 0 {
		t.Errorf("OnStreamToken outside request should be ignored, got %d", got)
	}
}

func TestTokenTracker_OnStreamToken_CountsCompletion(t *testing.T) {
	tr := NewTokenTracker()
	tr.StartRequest(100)
	for i := 0; i < 5; i++ {
		tr.OnStreamToken(false)
	}
	if got := tr.Snapshot().CompletionTokensThisRequest; got != 5 {
		t.Errorf("CompletionTokensThisRequest = %d, want 5", got)
	}
}

func TestTokenTracker_OnStreamToken_ReasoningNotInTps(t *testing.T) {
	tr := NewTokenTracker()
	tr.StartRequest(100)
	tr.OnStreamToken(true) // 思考 token
	tr.OnStreamToken(true)
	// 思考 token 也增加 CompletionTokensThisRequest
	// 但 tps 字段会被 isReasoning 路径覆盖（参考 nuclear-boy L102-108）
	s := tr.Snapshot()
	if s.CompletionTokensThisRequest != 2 {
		t.Errorf("CompletionTokensThisRequest = %d, want 2", s.CompletionTokensThisRequest)
	}
	if s.ReasoningTokensPerSecond <= 0 {
		t.Error("ReasoningTokensPerSecond should be > 0 after reasoning tokens")
	}
}

// ─── TokenTracker.Reset ──────────────────────────────────────

func TestTokenTracker_Reset(t *testing.T) {
	tr := NewTokenTracker()
	tr.StartRequest(1000)
	tr.OnRequestComplete(RequestUsage{PromptTokens: 1000, CompletionTokens: 200}, 500)

	tr.Reset()
	s := tr.Snapshot()
	if s.RequestCount != 0 {
		t.Errorf("after reset, RequestCount = %d, want 0", s.RequestCount)
	}
	if s.PromptTokensTotal != 0 {
		t.Errorf("after reset, PromptTokensTotal = %d, want 0", s.PromptTokensTotal)
	}
	if s.ContextRemaining != DeepSeekContextWindow {
		t.Errorf("after reset, ContextRemaining = %d, want %d", s.ContextRemaining, DeepSeekContextWindow)
	}
}
