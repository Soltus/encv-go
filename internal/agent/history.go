// Stage 3 (borrow-nuclear-boy-2026q2)：buildHistoryMessages 防 400 + reasoningContent 剥离。
//
// 借鉴自 /tmp/nuclear-boy/agent-core/src/main/java/com/nuclearboy/agent/AgentEngine.kt L543-630。
//
// 关键设计：
//   1. 按 toolCallId 去重（nuclear-boy 实战：AgentEngine 发射两次 ToolExecution: RUNNING+COMPLETED）
//   2. completedCalls 过滤（output != null && toolCallId != null）
//   3. completedCalls 为空 → toolCalls=null（防 API 400 insufficient tool messages）
//   4. 旧消息的 reasoning_content **不**入 history（防 token 浪费 + 400）
//   5. 最新一条 assistant 的 reasoning_content 可保留（前端折叠展示）
package agent

// HistoryMessageRole 描述 LLM API 接受的消息角色。
type HistoryMessageRole string

const (
	HistoryRoleSystem    HistoryMessageRole = "system"
	HistoryRoleUser      HistoryMessageRole = "user"
	HistoryRoleAssistant HistoryMessageRole = "assistant"
	HistoryRoleTool      HistoryMessageRole = "tool"
)

// HistoryToolCall 描述 LLM API 接受的工具调用。
type HistoryToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`     // 通常 "function"
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"` // JSON 字符串
	} `json:"function"`
}

// HistoryToolResult 描述 LLM API 接受的工具结果（role=tool 消息）。
type HistoryToolResult struct {
	Role       HistoryMessageRole `json:"role"`        // "tool"
	ToolCallID string             `json:"tool_call_id"`
	Content    string             `json:"content"`     // tool 输出（截断到 4K）
	Name       string             `json:"name,omitempty"`
}

// HistoryMessage 描述 LLM API 接受的一条消息。
type HistoryMessage struct {
	Role             HistoryMessageRole `json:"role"`
	Content          string             `json:"content,omitempty"`
	ReasoningContent string             `json:"reasoning_content,omitempty"`
	ToolCalls        []HistoryToolCall  `json:"tool_calls,omitempty"`
	// 仅当这是 tool 消息时使用
	ToolCallID string `json:"tool_call_id,omitempty"`
	Name       string `json:"name,omitempty"`
}

// AssistantTurn 描述一轮 assistant 消息 + 后续 tool 消息。
// 内部状态：可能 assistant 有 tool_calls 但还没收到结果（pending），
// 或 tool_calls 都已完成（completed）。
type AssistantTurn struct {
	// Assistant 消息
	AssistantMessage HistoryMessage
	// 该 assistant 调用的 tool 消息（按 toolCallId 去重）
	ToolMessages []HistoryToolResult
}

// PendingAssistantTurn 描述一轮"等待完成"的 assistant 调用。
// 当 tool_calls 还在累积（args 未完整）或 tool_result 还没回来时使用。
type PendingAssistantTurn struct {
	AssistantMessage HistoryMessage
	PendingToolCalls []HistoryToolCall // 还未完成（无 result）的 tool_calls
}

// BuildHistoryOptions 配置 BuildHistoryMessages 的行为。
type BuildHistoryOptions struct {
	// StripReasoningContent 控制是否剥离历史 assistant 消息的 reasoning_content。
	// 默认 true（nuclear-boy DeepSeekApiClient.kt L342-345 "we keep it intact" 是指
	//  累积时保留，但发送给 API 时剥离旧消息的）。
	StripReasoningContent bool
	// KeepLatestReasoning 控制最新一条 assistant 消息是否保留 reasoning_content。
	// 默认 true（前端 ReasoningSection 折叠展示）。
	KeepLatestReasoning bool
	// MaxToolResultContentLen 限制单条 tool result 的最大字符数。
	// 默认 4096。超过会被截断（防 token 爆炸）。
	MaxToolResultContentLen int
}

// DefaultBuildHistoryOptions 返回默认配置。
func DefaultBuildHistoryOptions() BuildHistoryOptions {
	return BuildHistoryOptions{
		StripReasoningContent:   true,
		KeepLatestReasoning:     true,
		MaxToolResultContentLen: 4096,
	}
}

// BuildHistoryMessages 把 SessionHistory 转成 LLM API 可用的 history。
//
// 输入：所有消息按时间顺序
// 输出：history 消息列表（system 在前，按对偶规则排 assistant+tool）
//
// 防 400 关键规则（nuclear-boy L543-630）：
//   - assistant 消息如果有 tool_calls，**必须**所有 tool_calls 都有对应 tool result
//   - 如果有 tool_calls 但没有任何 tool result（中断/超时），**整轮丢弃**
//   - 同一 toolCallId 出现多次（running + completed），只保留 1 条
//   - 旧消息的 reasoning_content 剥离
func BuildHistoryMessages(
	allMessages []HistoryMessage,
	allToolResults map[string]string, // toolCallId -> result content
	opts BuildHistoryOptions,
) []HistoryMessage {
	if len(allMessages) == 0 {
		return nil
	}

	if opts.MaxToolResultContentLen <= 0 {
		opts.MaxToolResultContentLen = 4096
	}

	// 第一步：按时间分组 (system, user, [assistant + tool messages]*)
	var out []HistoryMessage
	var i int

	// system 在最前（且只 1 条）
	if allMessages[0].Role == HistoryRoleSystem {
		out = append(out, allMessages[0])
		i = 1
	}

	// 跟踪当前正在处理的 assistant 消息 + 它的 tool 消息
	for i < len(allMessages) {
		msg := allMessages[i]
		if msg.Role == HistoryRoleUser {
			out = append(out, msg)
			i++
			continue
		}

		if msg.Role == HistoryRoleAssistant {
			// 收集这一轮的所有 tool 消息（紧跟 assistant 之后）
			turn := collectAssistantTurn(msg, allMessages[i+1:], allToolResults, opts.MaxToolResultContentLen)

			// 关键检查：turn 是否有未完成的 tool_call？
			hasIncomplete := false
			for _, tc := range turn.AssistantMessage.ToolCalls {
				if _, ok := allToolResults[tc.ID]; !ok {
					hasIncomplete = true
					break
				}
			}

			if hasIncomplete || len(turn.AssistantMessage.ToolCalls) == 0 {
				// 无 tool_calls → 纯文本 assistant 消息，剥离 reasoning_content（除最后一条）
				processed := msg
				processed.ToolCalls = nil // 防御性
				out = append(out, processed)
				i++
				// 跳过紧跟的 tool 消息（如果有但 assistant 没声明）
				i = skipOrphanToolMessages(allMessages, i)
				continue
			}

			// 完整轮次：assistant + 它的 tool 消息
			out = append(out, turn.AssistantMessage)
			out = append(out, convertToolResultsToMessages(turn.ToolMessages)...)
			i += 1 + len(turn.ToolMessages) // 跳过 assistant + 已消费的 tool 消息
			continue
		}

		if msg.Role == HistoryRoleTool {
			// 孤儿 tool 消息（没有前置 assistant）→ 跳过
			i++
			continue
		}

		// system / 未知 → 跳过
		i++
	}

	// 后处理：剥离旧消息的 reasoning_content（保留最后一条）
	if opts.StripReasoningContent {
		out = stripOldReasoningContent(out, opts.KeepLatestReasoning)
	}

	return out
}

// collectAssistantTurn 收集一轮 assistant 消息 + 后续 tool 消息。
// 关键：按 toolCallId 去重（nuclear-boy 实战：running + completed 推 2 次）。
func collectAssistantTurn(
	assistant HistoryMessage,
	following []HistoryMessage,
	toolResults map[string]string,
	maxContentLen int,
) AssistantTurn {
	turn := AssistantTurn{
		AssistantMessage: assistant,
	}

	// 工具消息按 toolCallId 去重
	seen := make(map[string]bool)

	// 1. 从 assistant 声明的 tool_calls 中获取 ordered list
	turn.AssistantMessage.ToolCalls = dedupeToolCalls(assistant.ToolCalls)

	// 2. 收集紧跟 assistant 的 tool 消息（按 id 关联）
	for _, m := range following {
		if m.Role != HistoryRoleTool {
			break
		}
		if m.ToolCallID == "" {
			continue
		}
		if seen[m.ToolCallID] {
			continue // 去重
		}
		seen[m.ToolCallID] = true

		content := m.Content
		if toolResult, ok := toolResults[m.ToolCallID]; ok && toolResult != "" {
			content = toolResult
		}
		if len(content) > maxContentLen {
			content = content[:maxContentLen] + "…(truncated)"
		}
		turn.ToolMessages = append(turn.ToolMessages, HistoryToolResult{
			Role:       HistoryRoleTool,
			ToolCallID: m.ToolCallID,
			Content:    content,
			Name:       m.Name,
		})

		// 防止无限循环：超过 assistant 声明的 tool_calls 数就停
		if len(turn.ToolMessages) >= len(turn.AssistantMessage.ToolCalls) {
			break
		}
	}

	// 3. 关键防 400 规则：检查 completedCalls
	//    如果 assistant 声明了 tool_calls 但一个都没有 tool result → toolCalls=null
	//    （nuclear-boy L595 "如果 completedCalls 为空 → toolCalls=null"）
	if len(turn.AssistantMessage.ToolCalls) > 0 && len(turn.ToolMessages) == 0 {
		turn.AssistantMessage.ToolCalls = nil
	}

	return turn
}

// dedupeToolCalls 按 toolCallId 去重。
// 借鉴 nuclear-boy HANDOVER2.0.md §三.3。
func dedupeToolCalls(calls []HistoryToolCall) []HistoryToolCall {
	seen := make(map[string]bool)
	out := make([]HistoryToolCall, 0, len(calls))
	for _, c := range calls {
		if c.ID == "" || seen[c.ID] {
			continue
		}
		seen[c.ID] = true
		out = append(out, c)
	}
	return out
}

// convertToolResultsToMessages 把 []HistoryToolResult 转成 []HistoryMessage。
func convertToolResultsToMessages(results []HistoryToolResult) []HistoryMessage {
	out := make([]HistoryMessage, 0, len(results))
	for _, r := range results {
		out = append(out, HistoryMessage{
			Role:       HistoryRoleTool,
			Content:    r.Content,
			ToolCallID: r.ToolCallID,
			Name:       r.Name,
		})
	}
	return out
}

// skipOrphanToolMessages 跳过孤儿的 tool 消息（assistant 没声明 tool_calls）。
// 返回更新后的 i。
func skipOrphanToolMessages(messages []HistoryMessage, i int) int {
	for i < len(messages) && messages[i].Role == HistoryRoleTool {
		i++
	}
	return i
}

// stripOldReasoningContent 剥离旧消息的 reasoning_content。
// 保留最后一条（前端折叠展示）。
// 借鉴 nuclear-boy DeepSeekApiClient.kt L342-345。
func stripOldReasoningContent(messages []HistoryMessage, keepLatest bool) []HistoryMessage {
	out := make([]HistoryMessage, 0, len(messages))
	for idx, m := range messages {
		if m.Role != HistoryRoleAssistant {
			out = append(out, m)
			continue
		}
		if keepLatest && idx == len(messages)-1 {
			out = append(out, m) // 最后一条保留
			continue
		}
		// 旧消息剥离
		m.ReasoningContent = ""
		out = append(out, m)
	}
	return out
}

// HasIncompleteToolCalls 检查 messages 中是否有 assistant 声明了 tool_calls
// 但缺少对应 tool result 的情况（中断 / 超时 / 30s timeout）。
//
// 借鉴 nuclear-boy AgentEngine.kt L483-518：取消的 tool_call 必须在 history 中移除。
func HasIncompleteToolCalls(messages []HistoryMessage, toolResults map[string]string) bool {
	for _, m := range messages {
		if m.Role != HistoryRoleAssistant {
			continue
		}
		for _, tc := range m.ToolCalls {
			if _, ok := toolResults[tc.ID]; !ok {
				return true
			}
		}
	}
	return false
}
