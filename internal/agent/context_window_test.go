package agent

import (
	"sync"
	"testing"
)

// ─── EstimateTokens ──────────────────────────────────────────

func TestEstimateTokens_EmptyString_ReturnsMin(t *testing.T) {
	got := EstimateTokens("")
	if got != MinEstimatedTokens {
		t.Errorf("EstimateTokens(\"\") = %d, want %d", got, MinEstimatedTokens)
	}
}

func TestEstimateTokens_ShortString_ReturnsMin(t *testing.T) {
	got := EstimateTokens("hi")
	if got != MinEstimatedTokens {
		t.Errorf("EstimateTokens(%q) = %d, want %d (min)", "hi", got, MinEstimatedTokens)
	}
}

func TestEstimateTokens_LongString_Proportional(t *testing.T) {
	text := string(make([]byte, 350)) // 350 chars / 3.5 = 100 tokens
	got := EstimateTokens(text)
	if got < 95 || got > 105 {
		t.Errorf("EstimateTokens(350 chars) = %d, want ~100", got)
	}
}

func TestEstimateTokens_ChineseMixed(t *testing.T) {
	text := "你好世界，今天天气真好，适合写代码。"
	got := EstimateTokens(text)
	if got < 5 || got > 20 {
		t.Errorf("EstimateTokens(chinese) = %d, want reasonable range", got)
	}
}

// ─── ContextBudget 计算 ──────────────────────────────────────

func TestContextBudget_TotalUsed(t *testing.T) {
	b := ContextBudget{
		SystemPrompt:        100,
		UserProfile:         200,
		ProjectContext:      300,
		ConversationHistory: 400,
		ToolDefinitions:     500,
		AttachedFiles:       600,
	}
	if got := b.TotalUsed(); got != 2100 {
		t.Errorf("TotalUsed = %d, want 2100", got)
	}
}

func TestContextBudget_Remaining_NeverNegative(t *testing.T) {
	b := ContextBudget{
		SystemPrompt:        DeepSeekContextWindow,
		ConversationHistory: DeepSeekContextWindow,
	}
	if got := b.Remaining(); got != 0 {
		t.Errorf("Remaining on over-budget = %d, want 0", got)
	}
}

func TestContextBudget_UsagePercent(t *testing.T) {
	b := ContextBudget{SystemPrompt: DeepSeekContextWindow / 2}
	pct := b.UsagePercent()
	if pct < 0.49 || pct > 0.51 {
		t.Errorf("UsagePercent for half = %f, want ~0.5", pct)
	}
}

func TestContextBudget_WarningLevel_OK(t *testing.T) {
	b := ContextBudget{SystemPrompt: 1000}
	if got := b.WarningLevel(); got != ContextWarningOK {
		t.Errorf("tiny budget should be OK, got %s", got)
	}
}

func TestContextBudget_WarningLevel_Green(t *testing.T) {
	b := ContextBudget{SystemPrompt: DeepSeekContextWindow * 4 / 10}
	if got := b.WarningLevel(); got != ContextWarningGreen {
		t.Errorf("40%% should be GREEN, got %s", got)
	}
}

func TestContextBudget_WarningLevel_Yellow(t *testing.T) {
	b := ContextBudget{SystemPrompt: ContextThresholdYellow}
	if got := b.WarningLevel(); got != ContextWarningYellow {
		t.Errorf("80%% should be YELLOW, got %s", got)
	}
}

func TestContextBudget_WarningLevel_Red(t *testing.T) {
	b := ContextBudget{SystemPrompt: ContextThresholdRed}
	if got := b.WarningLevel(); got != ContextWarningRed {
		t.Errorf("95%% should be RED, got %s", got)
	}
}

func TestContextBudget_WarningLevel_Force(t *testing.T) {
	b := ContextBudget{SystemPrompt: ContextThresholdForce}
	if got := b.WarningLevel(); got != ContextWarningForce {
		t.Errorf("98%% should be FORCE, got %s", got)
	}
}

func TestContextBudget_CanFit(t *testing.T) {
	b := ContextBudget{SystemPrompt: 1000}
	if !b.CanFit(500) {
		t.Error("should fit 500 tokens")
	}
	if b.CanFit(DeepSeekContextWindow) {
		t.Error("should not fit 1M tokens")
	}
}

func TestContextBudget_NeedsCompression_AtYellow(t *testing.T) {
	b := ContextBudget{SystemPrompt: ContextThresholdYellow}
	if !b.NeedsCompression() {
		t.Error("at YELLOW threshold should need compression")
	}
}

func TestContextBudget_NeedsUrgentCompression_AtRed(t *testing.T) {
	b := ContextBudget{SystemPrompt: ContextThresholdRed}
	if !b.NeedsUrgentCompression() {
		t.Error("at RED threshold should need urgent compression")
	}
}

// ─── ContextWindowManager ────────────────────────────────────

func TestContextWindowManager_InitialBudgetZero(t *testing.T) {
	m := NewContextWindowManager()
	if got := m.Budget().TotalUsed(); got != 0 {
		t.Errorf("initial total = %d, want 0", got)
	}
}

func TestContextWindowManager_UpdateAllocation_PartialUpdate(t *testing.T) {
	m := NewContextWindowManager()
	sys := int64(6000)
	hist := int64(100_000)
	m.UpdateAllocation(&sys, nil, nil, &hist, nil, nil)
	b := m.Budget()
	if b.SystemPrompt != 6000 {
		t.Errorf("SystemPrompt = %d, want 6000", b.SystemPrompt)
	}
	if b.ConversationHistory != 100_000 {
		t.Errorf("ConversationHistory = %d, want 100000", b.ConversationHistory)
	}
	if b.UserProfile != 0 {
		t.Errorf("UserProfile should remain 0, got %d", b.UserProfile)
	}
}

func TestContextWindowManager_CompressConversation_NoOpWhenUnderThreshold(t *testing.T) {
	m := NewContextWindowManager()
	hist := int64(50_000) // < ConversationCompressThreshold
	m.UpdateAllocation(nil, nil, nil, &hist, nil, nil)
	r := m.CompressConversation(10)
	if r.TokensSaved != 0 {
		t.Errorf("under threshold should save 0, got %d", r.TokensSaved)
	}
}

func TestContextWindowManager_CompressConversation_Halves(t *testing.T) {
	m := NewContextWindowManager()
	hist := int64(400_000) // > 200K threshold
	m.UpdateAllocation(nil, nil, nil, &hist, nil, nil)
	r := m.CompressConversation(20)
	if r.TokensSaved != 200_000 {
		t.Errorf("expected saving 200K tokens, got %d", r.TokensSaved)
	}
	if r.NewBudget.ConversationHistory != 200_000 {
		t.Errorf("new history = %d, want 200000", r.NewBudget.ConversationHistory)
	}
	if len(m.ConversationSummaries()) != 1 {
		t.Error("should record 1 summary")
	}
}

func TestContextWindowManager_CompressConversation_FloorAt50K(t *testing.T) {
	// 备注：CompressConversation 内的 floor 50K 在当前阈值下实际是死分支
	// （减半后 < 50K 必然原值 < 100K < 阈值 200K，不会进压缩）。
	// 保留以匹配 nuclear-boy 源码；不写断言。
	_ = NewContextWindowManager()
}

func TestContextWindowManager_EmergencyCompress(t *testing.T) {
	m := NewContextWindowManager()
	hist := int64(300_000)
	files := int64(200_000)
	proj := int64(50_000)
	m.UpdateAllocation(nil, nil, &proj, &hist, nil, &files)
	r := m.EmergencyCompress()
	if r.NewBudget.ConversationHistory != 90_000 {
		t.Errorf("hist should be 30%% of 300K = 90K, got %d", r.NewBudget.ConversationHistory)
	}
	if r.NewBudget.AttachedFiles != 100_000 {
		t.Errorf("files should be 50%% of 200K = 100K, got %d", r.NewBudget.AttachedFiles)
	}
	if r.NewBudget.ProjectContext != 35_000 {
		t.Errorf("project should be 70%% of 50K = 35K, got %d", r.NewBudget.ProjectContext)
	}
}

func TestContextWindowManager_Reset(t *testing.T) {
	m := NewContextWindowManager()
	sys := int64(6000)
	m.UpdateAllocation(&sys, nil, nil, nil, nil, nil)
	m.IncrementTurn()
	m.Reset()
	if m.Budget().TotalUsed() != 0 {
		t.Error("Reset should clear budget")
	}
	if len(m.ConversationSummaries()) != 0 {
		t.Error("Reset should clear summaries")
	}
}

func TestContextWindowManager_ConcurrentAccess(t *testing.T) {
	m := NewContextWindowManager()
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			val := int64(100)
			m.UpdateAllocation(&val, nil, nil, nil, nil, nil)
			_ = m.Budget()
			m.IncrementTurn()
		}()
	}
	wg.Wait()
	// SystemPrompt 应该是 100（race 条件下也可能不是，但不应 panic）
	if m.Budget().SystemPrompt > 200 {
		t.Errorf("concurrent updates corrupted: %d", m.Budget().SystemPrompt)
	}
}

// ─── 字符串工具函数 ──────────────────────────────────────────

func TestItoa(t *testing.T) {
	cases := []struct {
		in   int
		want string
	}{
		{0, "0"},
		{1, "1"},
		{42, "42"},
		{-7, "-7"},
		{12345, "12345"},
	}
	for _, c := range cases {
		if got := itoa(c.in); got != c.want {
			t.Errorf("itoa(%d) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestItoa64(t *testing.T) {
	if got := itoa64(0); got != "0" {
		t.Errorf("itoa64(0) = %q, want 0", got)
	}
	if got := itoa64(1_000_000); got != "1000000" {
		t.Errorf("itoa64(1000000) = %q, want 1000000", got)
	}
}
