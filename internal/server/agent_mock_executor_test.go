// internal/server/agent_mock_executor_test.go
//
// T3 验收：executor + branch-pick + MockEngine 集成。
package server

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// ════════════════════════════════════════════════════════════════
// MockEngine 集成
// ════════════════════════════════════════════════════════════════

func TestMockEngine_GetScenarioByID(t *testing.T) {
	e := NewMockEngine()
	sc := e.GetScenarioByID("default_friendly")
	if sc == nil {
		t.Fatal("GetScenarioByID(default_friendly) returned nil")
	}
	if sc.ID != "default_friendly" {
		t.Errorf("id = %q, want default_friendly", sc.ID)
	}
	if sc2 := e.GetScenarioByID("nonexistent"); sc2 != nil {
		t.Errorf("GetScenarioByID(nonexistent) = %+v, want nil", sc2)
	}
}

func TestMockEngine_NewMockEngineWithScenarios(t *testing.T) {
	custom := []*MockScenario{
		{ID: "custom_1", Description: "test 1"},
		{ID: "custom_2", Description: "test 2"},
	}
	e := NewMockEngineWithScenarios(custom)
	if got := e.GetScenarioByID("custom_1"); got == nil {
		t.Error("custom_1 not found")
	}
	if got := e.GetScenarioByID("custom_2"); got == nil {
		t.Error("custom_2 not found")
	}
	// 外部剧本完全覆盖 builtin（无 default_friendly）
	if got := e.GetScenarioByID("default_friendly"); got != nil {
		t.Error("default_friendly should NOT be present (custom overrides builtin)")
	}
}

// ════════════════════════════════════════════════════════════════
// argsToJSON 工具
// ════════════════════════════════════════════════════════════════

func TestArgsToJSON(t *testing.T) {
	tests := []struct {
		name string
		in   interface{}
		want string
	}{
		{"nil", nil, "{}"},
		{"empty map", map[string]interface{}{}, "{}"},
		{"simple", map[string]interface{}{"ext": ".mp4"}, `{"ext":".mp4"}`},
		{"complex", map[string]interface{}{
			"mount_id": "serving",
			"rel_path": "/01-plain-media/video",
			"min_size": 100,
		}, `{"min_size":100,"mount_id":"serving","rel_path":"/01-plain-media/video"}`},
		{"string passthrough", `{"ext":".mp4"}`, `{"ext":".mp4"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := argsToJSON(tt.in)
			if err != nil {
				t.Fatalf("argsToJSON: %v", err)
			}
			if got != tt.want {
				t.Errorf("argsToJSON(%v) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// ════════════════════════════════════════════════════════════════
// BranchPickRequest 拒绝自由文本
// ════════════════════════════════════════════════════════════════

func TestBranchPickRequest_RejectsFreeFormText(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{"user_text field", `{"scenario_id":"x","branch_id":"b","option_id":"o","user_text":"hi"}`},
		{"userText camelCase", `{"scenario_id":"x","branch_id":"b","option_id":"o","userText":"hi"}`},
		{"free_text field", `{"scenario_id":"x","branch_id":"b","option_id":"o","free_text":"hi"}`},
		{"option_text field", `{"scenario_id":"x","branch_id":"b","option_id":"o","option_text":"hi"}`},
		{"raw_text field", `{"scenario_id":"x","branch_id":"b","option_id":"o","raw_text":"hi"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var rawMap map[string]interface{}
			// 模拟 gin ShouldBindJSON
			if err := json.Unmarshal([]byte(tt.body), &rawMap); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			forbiddenKeys := []string{"user_text", "userText", "user_input", "userInput", "free_text", "freeText", "option_text", "optionText", "raw_text", "rawText"}
			hasForbidden := false
			for _, k := range forbiddenKeys {
				if _, exists := rawMap[k]; exists {
					hasForbidden = true
					break
				}
			}
			if !hasForbidden {
				t.Errorf("body %q should have been rejected (contains free-form field)", tt.body)
			}
		})
	}
}

func TestBranchPickRequest_AcceptsCleanPayload(t *testing.T) {
	body := `{"session_id":"sess-1","scenario_id":"search_recursive_mp4","branch_id":"post_search","option_id":"relax"}`
	var rawMap map[string]interface{}
	if err := json.Unmarshal([]byte(body), &rawMap); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	forbiddenKeys := []string{"user_text", "userText", "user_input", "userInput", "free_text", "freeText", "option_text", "optionText", "raw_text", "rawText"}
	for _, k := range forbiddenKeys {
		if _, exists := rawMap[k]; exists {
			t.Errorf("clean body should not have %q", k)
		}
	}
}

// ════════════════════════════════════════════════════════════════
// findBranchAndOption 测试
// ════════════════════════════════════════════════════════════════

func TestFindBranchAndOption_NotFound(t *testing.T) {
	sc := &MockScenario{
		ID: "test",
		Branches: []Branch{
			{ID: "b1", Label: "Branch 1"},
		},
	}
	branch, option := findBranchAndOption(sc, "nonexistent", "o1")
	if branch != nil {
		t.Errorf("expected nil branch, got %+v", branch)
	}
	if option != nil {
		t.Errorf("expected nil option, got %+v", option)
	}
}

func TestFindBranchAndOption_FoundInOnMatch(t *testing.T) {
	sc := &MockScenario{
		ID: "test",
		Branches: []Branch{
			{
				ID:    "post_search",
				Label: "Post Search",
				OnMatch: &MockScenario{
					ID: "post_search_child",
					Steps: []MockStep{
						{Events: []MockEvent{
							{Type: "mock_branch_choice", Data: map[string]interface{}{
								"options": []interface{}{
									map[string]interface{}{"id": "relax", "label": "放宽条件", "icon": "🎚️"},
									map[string]interface{}{"id": "cancel", "label": "取消", "icon": "❌"},
								},
							}},
						}},
					},
				},
			},
		},
	}
	branch, option := findBranchAndOption(sc, "post_search", "relax")
	if branch == nil {
		t.Fatal("expected branch, got nil")
	}
	if option == nil {
		t.Fatal("expected option, got nil")
	}
	if option.ID != "relax" {
		t.Errorf("option.id = %q, want relax", option.ID)
	}
	if option.Label != "放宽条件" {
		t.Errorf("option.label = %q, want 放宽条件", option.Label)
	}
}

func TestFindBranchAndOption_OptionNotInList(t *testing.T) {
	sc := &MockScenario{
		ID: "test",
		Branches: []Branch{
			{
				ID: "b1",
				OnMatch: &MockScenario{
					Steps: []MockStep{
						{Events: []MockEvent{
							{Type: "mock_branch_choice", Data: map[string]interface{}{
								"options": []interface{}{
									map[string]interface{}{"id": "a", "label": "A"},
									map[string]interface{}{"id": "b", "label": "B"},
								},
							}},
						}},
					},
				},
			},
		},
	}
	branch, option := findBranchAndOption(sc, "b1", "nonexistent")
	if branch == nil {
		t.Fatal("branch should be found")
	}
	if option != nil {
		t.Errorf("option for invalid id should be nil, got %+v", option)
	}
}

// ════════════════════════════════════════════════════════════════
// ctx cancellation 测试（executor 必须尊重 ctx）
// ════════════════════════════════════════════════════════════════

func TestExecutor_RespectsContextCancellation(t *testing.T) {
	// 简单的 ctx 取消测试：构造一个 cancelled ctx
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立即取消

	exec := newMockExecutor(ctx, nil, nil, nil, nil)
	step := MockStep{
		Events: []MockEvent{
			{Type: "text_delta", Data: map[string]interface{}{"text": "should not be reached"}},
		},
	}
	sc := &MockScenario{ID: "test"}
	called := 0
	emit := func(ev MockEvent, stepIdx, evIdx int) { called++ }

	err := exec.executeStep(step, sc, 0, emit)
	if err == nil {
		t.Error("expected error from cancelled ctx, got nil")
	}
	if !strings.Contains(err.Error(), "context canceled") {
		t.Errorf("error should mention context canceled, got: %v", err)
	}
	if called != 0 {
		t.Errorf("emit should not be called after ctx cancel, called=%d", called)
	}
}
