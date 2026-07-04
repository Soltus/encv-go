// internal/server/mock_scenario_schema_test.go
//
// T1 验收：schema 解析 + 校验全过。
//
// 覆盖：
//   - 基础字段解析
//   - 5 种 event type（stream_start / text_delta / tool_call / mock_branch_choice / stream_end）
//   - 缺 id 拒绝
//   - 空 steps 拒绝
//   - 模板语法 `{{` 拒绝
//   - mock_branch_choice options < 2 拒绝
//   - 硬编码路径/大小/计数/错误 拒绝
//   - 自由文本字段 拒绝
//   - tool_result 事件 拒绝
package server

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSchema_ParseYAML_BasicFields(t *testing.T) {
	yaml := `
id: search_recursive_mp4
description: 搜索 100MB+ 的 mp4
keywords:
  - search_recursive_mp4
  - 找视频
steps:
  - id: step1
    events:
      - type: stream_start
        data: { scenario: search_recursive_mp4 }
      - type: stream_end
        data: { finishReason: stop }
`
	var s LoadedScenario
	if err := unmarshalYAML([]byte(yaml), &s); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if s.ID != "search_recursive_mp4" {
		t.Errorf("id = %q, want %q", s.ID, "search_recursive_mp4")
	}
	if len(s.Keywords) != 2 {
		t.Errorf("keywords len = %d, want 2", len(s.Keywords))
	}
	if err := s.Validate(); err != nil {
		t.Errorf("validate failed: %v", err)
	}
}

func TestSchema_ParseYAML_AllEventTypes(t *testing.T) {
	yaml := `
id: all_event_types
steps:
  - id: s1
    events:
      - type: stream_start
        data: { scenario: all_event_types }
      - type: text_delta
        data: { text: "正在搜索..." }
      - type: tool_call
        data:
          id: call_1
          name: search_files
          args: { ext: ".mp4" }
      - type: mock_branch_choice
        data:
          branch_id: post_search
          options:
            - id: relax
              label: 放宽条件
              icon: "🎚️"
            - id: cancel
              label: 取消
              icon: "❌"
      - type: stream_end
        data: { finishReason: stop }
`
	var s LoadedScenario
	if err := unmarshalYAML([]byte(yaml), &s); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if err := s.Validate(); err != nil {
		t.Errorf("validate failed: %v", err)
	}
	if len(s.Steps[0].Events) != 5 {
		t.Errorf("events len = %d, want 5", len(s.Steps[0].Events))
	}
}

func TestSchema_Validate_RejectsMissingID(t *testing.T) {
	s := &LoadedScenario{
		Steps: []YAMLStep{
			{Events: []YAMLEvent{{Type: "stream_end", Data: map[string]interface{}{"finishReason": "stop"}}}},
		},
	}
	if err := s.Validate(); err == nil {
		t.Fatal("expected error for missing id, got nil")
	} else if !strings.Contains(err.Error(), "missing id") {
		t.Errorf("error %q should contain 'missing id'", err)
	}
}

func TestSchema_Validate_RejectsEmptySteps(t *testing.T) {
	s := &LoadedScenario{ID: "empty"}
	if err := s.Validate(); err == nil {
		t.Fatal("expected error for empty steps, got nil")
	} else if !strings.Contains(err.Error(), "steps is empty") {
		t.Errorf("error %q should contain 'steps is empty'", err)
	}
}

func TestSchema_Validate_RejectsTemplateSyntax(t *testing.T) {
	tests := []struct {
		name string
		text string
	}{
		{"go template", "Hello {{ .UserText }}"},
		{"jinja", "Found {{ files | length }} files"},
		{"legacy id", "Result: {% call_files1:files %}"},
		{"partial", "Some text with {{ incomplete"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &LoadedScenario{
				ID: "tmpl",
				Steps: []YAMLStep{
					{Events: []YAMLEvent{
						{Type: "text_delta", Data: map[string]interface{}{"text": tt.text}},
					}},
				},
			}
			if err := s.Validate(); err == nil {
				t.Errorf("expected error for template syntax %q, got nil", tt.text)
			} else if !strings.Contains(err.Error(), "template syntax") {
				t.Errorf("error %q should contain 'template syntax'", err)
			}
		})
	}
}

func TestSchema_Validate_RejectsBranchWithLessThan2Options(t *testing.T) {
	tests := []struct {
		name    string
		options []interface{}
	}{
		{"zero options", []interface{}{}},
		{"one option", []interface{}{map[string]interface{}{"id": "x", "label": "y"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &LoadedScenario{
				ID: "branch",
				Steps: []YAMLStep{
					{Events: []YAMLEvent{
						{Type: "mock_branch_choice", Data: map[string]interface{}{"options": tt.options}},
					}},
				},
			}
			if err := s.Validate(); err == nil {
				t.Errorf("expected error for options < 2, got nil")
			} else if !strings.Contains(err.Error(), "at least 2 entries") {
				t.Errorf("error %q should contain 'at least 2 entries'", err)
			}
		})
	}
}

// ════════════════════════════════════════════════════════════════
// 硬编码数据红线测试
// ════════════════════════════════════════════════════════════════

func TestSchema_Validate_RejectsHardcodedPath(t *testing.T) {
	tests := []struct {
		name string
		text string
	}{
		{"mp4 path", "发现文件 Movies/2024/big_buck_bunny.mp4"},
		{"two-level path", "Path: 01-plain-media/video/sample.mp4"},
		{"json path", "Config: app/data/config.json"},
		{"log path", "Log: var/log/server.log"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &LoadedScenario{
				ID: "hardpath",
				Steps: []YAMLStep{
					{Events: []YAMLEvent{
						{Type: "text_delta", Data: map[string]interface{}{"text": tt.text}},
					}},
				},
			}
			if err := s.Validate(); err == nil {
				t.Errorf("expected error for hardcoded path %q, got nil", tt.text)
			} else if !strings.Contains(err.Error(), "hardcoded path") {
				t.Errorf("error %q should contain 'hardcoded path'", err)
			}
		})
	}
}

func TestSchema_Validate_RejectsHardcodedSize(t *testing.T) {
	tests := []struct {
		name string
		text string
	}{
		{"MB", "文件大小: 524MB"},
		{"KB with decimal", "Size: 11.4 KB"},
		{"GB", "约 1.2 GB"},
		{"bytes", "Total: 268000000 bytes"},
		{"B", "Tiny file: 512 B"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &LoadedScenario{
				ID: "hardsize",
				Steps: []YAMLStep{
					{Events: []YAMLEvent{
						{Type: "text_delta", Data: map[string]interface{}{"text": tt.text}},
					}},
				},
			}
			if err := s.Validate(); err == nil {
				t.Errorf("expected error for hardcoded size %q, got nil", tt.text)
			} else if !strings.Contains(err.Error(), "hardcoded size") {
				t.Errorf("error %q should contain 'hardcoded size'", err)
			}
		})
	}
}

func TestSchema_Validate_RejectsHardcodedCount(t *testing.T) {
	tests := []struct {
		name string
		text string
	}{
		{"零匹配", "搜索结果: 0 个匹配"},
		{"三文件", "发现 3 个文件"},
		{"五结果", "返回 5 个结果"},
		{"二条目", "共 2 个条目"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &LoadedScenario{
				ID: "hardcount",
				Steps: []YAMLStep{
					{Events: []YAMLEvent{
						{Type: "text_delta", Data: map[string]interface{}{"text": tt.text}},
					}},
				},
			}
			if err := s.Validate(); err == nil {
				t.Errorf("expected error for hardcoded count %q, got nil", tt.text)
			} else if !strings.Contains(err.Error(), "hardcoded count") {
				t.Errorf("error %q should contain 'hardcoded count'", err)
			}
		})
	}
}

func TestSchema_Validate_RejectsHardcodedError(t *testing.T) {
	tests := []struct {
		name string
		text string
	}{
		{"English", "ERROR: timeout after 30s"},
		{"中文冒号", "失败: 文件不存在"},
		{"全角冒号", "错误：权限不足"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &LoadedScenario{
				ID: "harderr",
				Steps: []YAMLStep{
					{Events: []YAMLEvent{
						{Type: "text_delta", Data: map[string]interface{}{"text": tt.text}},
					}},
				},
			}
			if err := s.Validate(); err == nil {
				t.Errorf("expected error for hardcoded error %q, got nil", tt.text)
			} else if !strings.Contains(err.Error(), "hardcoded error") {
				t.Errorf("error %q should contain 'hardcoded error'", err)
			}
		})
	}
}

func TestSchema_Validate_RejectsFreeFormInputField(t *testing.T) {
	// user_text / userText / user_input / free_text 等自由文本字段
	tests := []struct {
		name string
		data map[string]interface{}
	}{
		{"user_text", map[string]interface{}{"user_text": "hello world"}},
		{"userText", map[string]interface{}{"userText": "hi"}},
		{"free_text", map[string]interface{}{"free_text": "anything"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &LoadedScenario{
				ID: "freeform",
				Steps: []YAMLStep{
					{Events: []YAMLEvent{
						{Type: "text_delta", Data: tt.data},
					}},
				},
			}
			if err := s.Validate(); err == nil {
				t.Errorf("expected error for free-form input field, got nil")
			} else if !strings.Contains(err.Error(), "free-form input") &&
				!strings.Contains(err.Error(), "user_text") {
				t.Errorf("error %q should mention 'free-form input' or 'user_text'", err)
			}
		})
	}
}

func TestSchema_Validate_RejectsToolResultEvent(t *testing.T) {
	s := &LoadedScenario{
		ID: "tresult",
		Steps: []YAMLStep{
			{Events: []YAMLEvent{
				{Type: "tool_result", Data: map[string]interface{}{
					"id":     "call_1",
					"result": `{"hardcoded":true}`,
				}},
			}},
		},
	}
	if err := s.Validate(); err == nil {
		t.Fatal("expected error for tool_result event, got nil")
	} else if !strings.Contains(err.Error(), "FORBIDDEN") || !strings.Contains(err.Error(), "tool_result") {
		t.Errorf("error %q should contain 'FORBIDDEN' + 'tool_result'", err)
	}
}

// ════════════════════════════════════════════════════════════════
// ConvertToMockScenario 测试
// ════════════════════════════════════════════════════════════════

func TestSchema_ConvertToMockScenario_AutoInjectsToolResult(t *testing.T) {
	yaml := `
id: convert_test
steps:
  - id: s1
    events:
      - type: tool_call
        data:
          id: call_1
          name: search_files
          args: { ext: ".mp4" }
      - type: stream_end
        data: { finishReason: stop }
`
	var s LoadedScenario
	if err := unmarshalYAML([]byte(yaml), &s); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if err := s.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
	sc := s.ConvertToMockScenario()
	if sc == nil {
		t.Fatal("ConvertToMockScenario returned nil")
	}
	// 转换后该 step 应有 3 个 event：tool_call + 自动 tool_result + stream_end
	if got := len(sc.Steps[0].Events); got != 3 {
		t.Fatalf("step events = %d, want 3 (tool_call + auto tool_result + stream_end)", got)
	}
	if sc.Steps[0].Events[0].Type != "tool_call" {
		t.Errorf("event[0].Type = %q, want tool_call", sc.Steps[0].Events[0].Type)
	}
	if sc.Steps[0].Events[1].Type != "tool_result" {
		t.Errorf("event[1].Type = %q, want tool_result (auto-injected)", sc.Steps[0].Events[1].Type)
	}
	if marked, _ := sc.Steps[0].Events[1].Data["__yaml_auto_generated"].(bool); !marked {
		t.Error("auto-injected tool_result should be marked __yaml_auto_generated=true")
	}
	if id, _ := sc.Steps[0].Events[1].Data["id"].(string); id != "call_1" {
		t.Errorf("auto tool_result id = %q, want call_1", id)
	}
	if sc.Steps[0].Events[2].Type != "stream_end" {
		t.Errorf("event[2].Type = %q, want stream_end", sc.Steps[0].Events[2].Type)
	}
}

// ════════════════════════════════════════════════════════════════
// CI 红线测试：扫所有内置 YAML（mock_scenarios/builtin/，v2 已合并入 builtin）
// ════════════════════════════════════════════════════════════════

func TestSchema_NoHardcodedData_AllBuiltinScenarios(t *testing.T) {
	// 注意：此测试在 T4 迁移完成前是 no-op（目录不存在或为空），
	// 迁移完成后 MUST PASS，否则阻断 CI。
	scenarios := loadBuiltinScenariosFromYAML(t)
	if len(scenarios) == 0 {
		t.Skip("no YAML scenarios yet — T4 migration pending")
	}
	for _, s := range scenarios {
		if err := s.Validate(); err != nil {
			t.Errorf("scenario %q failed validation: %v", s.ID, err)
		}
	}
}

// loadBuiltinScenariosFromYAML 从 mock_scenarios/builtin/ 加载所有 YAML。
func loadBuiltinScenariosFromYAML(t *testing.T) []LoadedScenario {
	t.Helper()
	dirs := []string{
		"mock_scenarios/builtin",
		"internal/server/mock_scenarios/builtin",
	}
	var all []LoadedScenario
	for _, d := range dirs {
		// 相对于测试工作目录
		absDir, _ := filepath.Abs(d)
		info, err := os.Stat(absDir)
		if err != nil || !info.IsDir() {
			continue
		}
		loader := NewScenarioLoader(absDir)
		loaded := loader.GetLoadedYAMLScenarios()
		all = append(all, loaded...)
	}
	return all
}
