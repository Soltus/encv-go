package agent

import (
	"strings"
	"testing"
)

// ─── 5 原则校验测试 ────────────────────────────────────────────

func TestValidatePrompt_AcceptsGoodPrompt(t *testing.T) {
	good := "你是 ENCV AI 助手。调用 read_file(path=\"x\") 读取文件。调用 list_mounts() 列出挂载点。"
	if err := ValidatePrompt(good); err != nil {
		t.Errorf("good prompt should pass, got: %v", err)
	}
}

func TestValidatePrompt_RejectsForbiddenNegation_Chinese(t *testing.T) {
	bad := "不要用 path 参数。"
	if err := ValidatePrompt(bad); err == nil {
		t.Errorf("prompt with '不要' should be rejected (nuclear-boy HANDOVER2.0.md §五 原则 2)")
	}
}

func TestValidatePrompt_RejectsForbiddenNegation_English(t *testing.T) {
	bad := "Never use the path parameter."
	if err := ValidatePrompt(bad); err == nil {
		t.Errorf("prompt with 'Never' should be rejected")
	}
}

func TestValidatePrompt_RejectsOverLength(t *testing.T) {
	// 构造 1600 字符 prompt（超过 MaxPromptLength=1500）
	bad := strings.Repeat("a", 1600)
	if err := ValidatePrompt(bad); err == nil {
		t.Errorf("prompt of 1600 chars should be rejected (原则 4 精简至上)")
	}
}

func TestValidatePrompt_AcceptsExactlyMaxLength(t *testing.T) {
	// 构造 1500 字符 prompt（正好等于上限）
	good := strings.Repeat("a", MaxPromptLength)
	if err := ValidatePrompt(good); err != nil {
		t.Errorf("prompt of exactly %d chars should pass, got: %v", MaxPromptLength, err)
	}
}

// ─── PROACTIVE 段生成测试 ─────────────────────────────────────

func TestBuilder_NoTriggers_NoProactiveSection(t *testing.T) {
	b := NewSystemPromptBuilder("base prompt")
	got := b.Build()
	if strings.Contains(got, "## 主动智能") {
		t.Errorf("builder without triggers should not include PROACTIVE section")
	}
}

func TestBuilder_UserConsecutiveTriggers_IncludesProactive(t *testing.T) {
	b := NewSystemPromptBuilder("base")
	b.AddProactiveTrigger(ProactiveTrigger{
		Type:             ProactiveTriggerUserConsecutive,
		UserMessageCount: 5,
	})
	got := b.Build()
	if !strings.Contains(got, "## 主动智能") {
		t.Error("PROACTIVE section should be present")
	}
	if !strings.Contains(got, "5 条消息") {
		t.Error("PROACTIVE section should mention user message count")
	}
}

func TestBuilder_ToolCompletedTrigger_IncludesToolName(t *testing.T) {
	b := NewSystemPromptBuilder("base")
	b.AddProactiveTrigger(ProactiveTrigger{
		Type:     ProactiveTriggerToolCompleted,
		ToolName: "read_file",
	})
	got := b.Build()
	if !strings.Contains(got, "read_file") {
		t.Error("PROACTIVE should mention tool name")
	}
}

func TestBuilder_ToolFailedTrigger_IncludesError(t *testing.T) {
	b := NewSystemPromptBuilder("base")
	b.AddProactiveTrigger(ProactiveTrigger{
		Type:      ProactiveTriggerToolFailed,
		ToolName:  "list_files",
		ToolError: "permission denied",
	})
	got := b.Build()
	if !strings.Contains(got, "list_files") {
		t.Error("PROACTIVE should mention tool name on failure")
	}
	if !strings.Contains(got, "permission denied") {
		t.Error("PROACTIVE should include error message on failure")
	}
}

func TestBuilder_MultipleTriggers_AllIncluded(t *testing.T) {
	b := NewSystemPromptBuilder("base")
	b.AddProactiveTrigger(ProactiveTrigger{Type: ProactiveTriggerUserConsecutive, UserMessageCount: 4})
	b.AddProactiveTrigger(ProactiveTrigger{Type: ProactiveTriggerToolCompleted, ToolName: "stat_file"})
	b.AddProactiveTrigger(ProactiveTrigger{Type: ProactiveTriggerTaskListEmpty})
	got := b.Build()
	if !strings.Contains(got, "4 条消息") {
		t.Error("should include user consecutive trigger")
	}
	if !strings.Contains(got, "stat_file") {
		t.Error("should include tool completed trigger")
	}
	if !strings.Contains(got, "任务列表为空") {
		t.Error("should include task list empty trigger")
	}
}

// ─── 用户偏好注入测试 ────────────────────────────────────────

func TestBuilder_LowConfidencePreference_NotInjected(t *testing.T) {
	b := NewSystemPromptBuilder("base")
	b.AddUserPreference("preferred_language", "TypeScript", 0.3) // < 0.5 不注入
	got := b.Build()
	if strings.Contains(got, "preferred_language") {
		t.Error("low confidence preference should not be injected")
	}
}

func TestBuilder_HighConfidencePreference_Injected(t *testing.T) {
	b := NewSystemPromptBuilder("base")
	b.AddUserPreference("preferred_language", "TypeScript", 0.7)
	got := b.Build()
	if !strings.Contains(got, "preferred_language: TypeScript") {
		t.Error("high confidence preference should be injected")
	}
}

func TestBuilder_MaxTwentyPreferences_LimitEnforced(t *testing.T) {
	b := NewSystemPromptBuilder("base")
	for i := 0; i < 30; i++ {
		b.AddUserPreference("k", "v", 0.9)
	}
	got := b.Build()
	count := strings.Count(got, "- k:")
	if count != 20 {
		t.Errorf("expected exactly 20 preferences injected, got %d", count)
	}
}

// ─── 综合测试 ────────────────────────────────────────────────

func TestBuilder_FullAssembly(t *testing.T) {
	b := NewSystemPromptBuilder("你是 ENCV AI 助手。")
	b.AddUserPreference("language", "Chinese", 0.9)
	b.AddProactiveTrigger(ProactiveTrigger{
		Type:             ProactiveTriggerUserConsecutive,
		UserMessageCount: 3,
	})
	got := b.Build()

	// 顺序：base → 用户偏好 → PROACTIVE
	baseIdx := strings.Index(got, "ENCV AI 助手")
	prefIdx := strings.Index(got, "## 用户偏好")
	proactiveIdx := strings.Index(got, "## 主动智能")

	if baseIdx == -1 || prefIdx == -1 || proactiveIdx == -1 {
		t.Fatalf("all three sections should exist: base=%d pref=%d proactive=%d",
			baseIdx, prefIdx, proactiveIdx)
	}
	if !(baseIdx < prefIdx && prefIdx < proactiveIdx) {
		t.Errorf("section order should be: base < preferences < proactive")
	}
}

// ─── DisableThinkingRequestBody 测试 ──────────────────────────

func TestDisableThinkingRequestBody(t *testing.T) {
	body := DisableThinkingRequestBody()
	if body["type"] != "disabled" {
		t.Errorf("DisableThinkingRequestBody should set type=disabled, got: %v", body["type"])
	}
}

// ─── EstimatePromptTokens 算法测试 ───────────────────────────

func TestEstimatePromptTokens_EmptyString(t *testing.T) {
	if got := EstimatePromptTokens(""); got != 0 {
		t.Errorf("empty string should return 0, got %d", got)
	}
}

func TestEstimatePromptTokens_ShortStringMin(t *testing.T) {
	// "hi" = 2 chars / 3.5 = 0.57 < 20, 应回退到 20（nuclear-boy 算法）
	if got := EstimatePromptTokens("hi"); got != 20 {
		t.Errorf("short string should floor to 20, got %d", got)
	}
}

func TestEstimatePromptTokens_LongString(t *testing.T) {
	// 350 chars / 3.5 = 100
	text := strings.Repeat("a", 350)
	if got := EstimatePromptTokens(text); got != 100 {
		t.Errorf("350 chars should give 100 tokens, got %d", got)
	}
}
