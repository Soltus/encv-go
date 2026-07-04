package agent

import (
	"strings"
	"testing"
)

// ─── BuildHistoryMessages 关键防 400 测试 ─────────────────────

func TestBuildHistoryMessages_EmptyInput(t *testing.T) {
	got := BuildHistoryMessages(nil, nil, DefaultBuildHistoryOptions())
	if got != nil {
		t.Errorf("empty input should return nil, got %v", got)
	}
}

func TestBuildHistoryMessages_SystemPreserved(t *testing.T) {
	msgs := []HistoryMessage{
		{Role: HistoryRoleSystem, Content: "you are an assistant"},
		{Role: HistoryRoleUser, Content: "hi"},
	}
	got := BuildHistoryMessages(msgs, nil, DefaultBuildHistoryOptions())
	if got[0].Role != HistoryRoleSystem {
		t.Errorf("system message should be first, got %v", got[0].Role)
	}
	if got[0].Content != "you are an assistant" {
		t.Errorf("system content should be preserved, got %q", got[0].Content)
	}
}

func TestBuildHistoryMessages_StripsOldReasoningContent(t *testing.T) {
	msgs := []HistoryMessage{
		{Role: HistoryRoleSystem, Content: "sys"},
		{Role: HistoryRoleUser, Content: "hi"},
		{Role: HistoryRoleAssistant, Content: "answer 1", ReasoningContent: "thinking 1"},
		{Role: HistoryRoleUser, Content: "next?"},
		{Role: HistoryRoleAssistant, Content: "answer 2", ReasoningContent: "thinking 2"},
	}
	got := BuildHistoryMessages(msgs, nil, DefaultBuildHistoryOptions())

	// 找到两个 assistant
	var assistants []HistoryMessage
	for _, m := range got {
		if m.Role == HistoryRoleAssistant {
			assistants = append(assistants, m)
		}
	}
	if len(assistants) != 2 {
		t.Fatalf("expected 2 assistant messages, got %d", len(assistants))
	}
	// 旧的应剥离
	if assistants[0].ReasoningContent != "" {
		t.Errorf("old assistant reasoning_content should be stripped, got %q", assistants[0].ReasoningContent)
	}
	// 最新的应保留
	if assistants[1].ReasoningContent != "thinking 2" {
		t.Errorf("latest assistant reasoning_content should be preserved, got %q", assistants[1].ReasoningContent)
	}
}

func TestBuildHistoryMessages_ToolCallCompleted_KeptInHistory(t *testing.T) {
	toolResults := map[string]string{
		"tc-1": `{"content":"file content"}`,
	}
	msgs := []HistoryMessage{
		{Role: HistoryRoleSystem, Content: "sys"},
		{Role: HistoryRoleUser, Content: "read a.txt"},
		{Role: HistoryRoleAssistant, Content: "", ToolCalls: []HistoryToolCall{
			{ID: "tc-1", Function: struct {
				Name      string `json:"name"`
				Arguments string `json:"arguments"`
			}{Name: "read_file", Arguments: `{"path":"/a.txt"}`}},
		}},
		{Role: HistoryRoleTool, ToolCallID: "tc-1", Content: `{"content":"file content"}`},
		{Role: HistoryRoleAssistant, Content: "here is the file"},
	}
	got := BuildHistoryMessages(msgs, toolResults, DefaultBuildHistoryOptions())

	// 应该包含 2 个 assistant + 1 个 tool 消息
	var assistantCount, toolCount int
	for _, m := range got {
		switch m.Role {
		case HistoryRoleAssistant:
			assistantCount++
			if m.Content == "" && len(m.ToolCalls) > 0 {
				// 第一个 assistant 必须保留 tool_calls
			}
		case HistoryRoleTool:
			toolCount++
		}
	}
	if assistantCount != 2 {
		t.Errorf("expected 2 assistants, got %d", assistantCount)
	}
	if toolCount != 1 {
		t.Errorf("expected 1 tool message, got %d", toolCount)
	}
}

func TestBuildHistoryMessages_ToolCallIncomplete_DropsToolCallsAndToolMsgs(t *testing.T) {
	// nuclear-boy L595: 如果 completedCalls 为空 → toolCalls=null
	// 关键：assistant 声明了 tool_calls 但 tool result **没**来（中断）
	msgs := []HistoryMessage{
		{Role: HistoryRoleUser, Content: "read a.txt"},
		{Role: HistoryRoleAssistant, Content: "", ToolCalls: []HistoryToolCall{
			{ID: "tc-orphan", Function: struct {
				Name      string `json:"name"`
				Arguments string `json:"arguments"`
			}{Name: "read_file", Arguments: `{"path":"/a.txt"}`}},
		}},
		// 缺少 role=tool 消息
	}
	toolResults := map[string]string{} // 空的，tc-orphan 没有 result

	got := BuildHistoryMessages(msgs, toolResults, DefaultBuildHistoryOptions())

	// 检查：assistant 的 tool_calls 应被清空（防 400）
	for _, m := range got {
		if m.Role == HistoryRoleAssistant {
			if len(m.ToolCalls) > 0 {
				t.Errorf("incomplete assistant should have toolCalls=null, got %v", m.ToolCalls)
			}
		}
	}
}

func TestBuildHistoryMessages_DedupesByToolCallId(t *testing.T) {
	// 借鉴 nuclear-boy HANDOVER2.0.md §三.3: 同一 toolCallId 推 2 次（running + completed）→ 只 1 条
	toolResults := map[string]string{
		"tc-1": "result",
	}
	msgs := []HistoryMessage{
		{Role: HistoryRoleUser, Content: "x"},
		{Role: HistoryRoleAssistant, Content: "", ToolCalls: []HistoryToolCall{
			{ID: "tc-1", Function: struct {
				Name      string `json:"name"`
				Arguments string `json:"arguments"`
			}{Name: "read_file", Arguments: `{}`}},
			{ID: "tc-1", Function: struct { // 重复
				Name      string `json:"name"`
				Arguments string `json:"arguments"`
			}{Name: "read_file", Arguments: `{}`}},
		}},
		{Role: HistoryRoleTool, ToolCallID: "tc-1", Content: "result"},
		{Role: HistoryRoleTool, ToolCallID: "tc-1", Content: "result"}, // 重复
	}
	got := BuildHistoryMessages(msgs, toolResults, DefaultBuildHistoryOptions())

	toolCount := 0
	for _, m := range got {
		if m.Role == HistoryRoleTool {
			toolCount++
		}
	}
	if toolCount != 1 {
		t.Errorf("deduped tool messages should be 1, got %d", toolCount)
	}
}

func TestBuildHistoryMessages_TruncatesLongToolResult(t *testing.T) {
	longContent := strings.Repeat("a", 5000)
	toolResults := map[string]string{
		"tc-1": longContent,
	}
	msgs := []HistoryMessage{
		{Role: HistoryRoleUser, Content: "x"},
		{Role: HistoryRoleAssistant, Content: "", ToolCalls: []HistoryToolCall{
			{ID: "tc-1", Function: struct {
				Name      string `json:"name"`
				Arguments string `json:"arguments"`
			}{Name: "read_file", Arguments: `{}`}},
		}},
	}
	got := BuildHistoryMessages(msgs, toolResults, DefaultBuildHistoryOptions())

	for _, m := range got {
		if m.Role == HistoryRoleTool {
			if len(m.Content) > 4096+10 { // 4096 + 截断标识
				t.Errorf("tool result should be truncated to ~4096, got %d chars", len(m.Content))
			}
			if !strings.HasSuffix(m.Content, "…(truncated)") {
				t.Errorf("truncated tool result should have marker, got suffix: %q",
					m.Content[max(0, len(m.Content)-20):])
			}
		}
	}
}

// ─── HasIncompleteToolCalls 测试 ──────────────────────────────

func TestHasIncompleteToolCalls_NoAssistant(t *testing.T) {
	msgs := []HistoryMessage{
		{Role: HistoryRoleUser, Content: "x"},
	}
	if HasIncompleteToolCalls(msgs, nil) {
		t.Error("no assistant with tool_calls → should be false")
	}
}

func TestHasIncompleteToolCalls_AllComplete(t *testing.T) {
	msgs := []HistoryMessage{
		{Role: HistoryRoleAssistant, ToolCalls: []HistoryToolCall{
			{ID: "tc-1", Function: struct {
				Name      string `json:"name"`
				Arguments string `json:"arguments"`
			}{Name: "f", Arguments: `{}`}},
		}},
	}
	results := map[string]string{"tc-1": "ok"}
	if HasIncompleteToolCalls(msgs, results) {
		t.Error("all complete → should be false")
	}
}

func TestHasIncompleteToolCalls_Missing(t *testing.T) {
	msgs := []HistoryMessage{
		{Role: HistoryRoleAssistant, ToolCalls: []HistoryToolCall{
			{ID: "tc-1", Function: struct {
				Name      string `json:"name"`
				Arguments string `json:"arguments"`
			}{Name: "f", Arguments: `{}`}},
		}},
	}
	results := map[string]string{} // tc-1 缺失
	if !HasIncompleteToolCalls(msgs, results) {
		t.Error("missing result → should be true")
	}
}

// 辅助函数
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
