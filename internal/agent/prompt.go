// Package agent 提供 ENCV AI Agent 的核心组件。
//
// Stage 1（borrow-nuclear-boy-2026q2）：SystemPromptBuilder + 5 原则校验 + PROACTIVE 主动智能段生成。
//
// 借鉴来源：/tmp/nuclear-boy/agent-core/src/main/java/com/nuclearboy/agent/SystemPromptBuilder.kt
// HANDOVER2.0.md §五 5 大原则：
//   1. 工具描述 > prompt（工具 description 字段是模型看的主要参考）
//   2. 避免否定表述（"不要用 path"会植入错误模式）
//   3. 正面示例 > 规则列表（`read_file(path="x")` 比"read_file 需要 path"有效 10 倍）
//   4. 精简至上（4000 → 800 字后成功率从 50% 飙升到 95%）
//   5. DeepSeek 默认 thinking=enabled → 必须显式传 {"thinking": {"type": "disabled"}}
//
// PROACTIVE 主动智能哲学（SystemPromptBuilder.kt L142-148）：
//   "你是主动型助理。每次回复结尾...无需用户开口主动提建议"
package agent

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// MaxPromptLength 是 system prompt 推荐上限（来自 nuclear-boy HANDOVER2.0.md §五）。
// 4000 → 800 字后成功率从 50% 飙升到 95%；这里放宽到 1500 字符（中文+英文混合）。
const MaxPromptLength = 1500

// forbiddenNegations 是禁止出现的否定词汇（5 原则之"避免否定表述"）。
// 来源：nuclear-boy HANDOVER2.0.md §五。
var forbiddenNegations = []string{
	"不要", "不能", "禁止", "不可用", "不可", "不应",
	"never", "don't", "do not", "must not", "forbidden", "unavailable",
}

// ProactiveTriggerType 描述 PROACTIVE 建议的触发条件。
//
// 借鉴自 SystemPromptBuilder.kt L142-148 "## 主动智能 (PROACTIVE)" 段。
type ProactiveTriggerType int

const (
	// ProactiveTriggerUserConsecutive 触发：用户连续发 3+ 消息
	ProactiveTriggerUserConsecutive ProactiveTriggerType = iota
	// ProactiveTriggerToolCompleted 触发：完成 1 个工具调用
	ProactiveTriggerToolCompleted
	// ProactiveTriggerToolFailed 触发：收到工具失败
	ProactiveTriggerToolFailed
	// ProactiveTriggerTaskListEmpty 触发：检测到 task 列表为空
	ProactiveTriggerTaskListEmpty
)

// ProactiveTrigger 携带触发 PROACTIVE 建议所需的所有信息。
type ProactiveTrigger struct {
	Type ProactiveTriggerType
	// 以下字段根据 Type 可选
	UserMessageCount int    // 仅 ProactiveTriggerUserConsecutive 使用
	ToolName         string // ProactiveTriggerToolCompleted / Failed 使用
	ToolError        string // ProactiveTriggerToolFailed 使用
	Context          string // 可选：当前对话上下文的简短摘要
}

// SystemPromptBuilder 是 system prompt 的动态构建器。
//
// 设计要点：
//   - 静态部分（身份 + 工具定义）由 defaultAgentSystemPrompt 提供（在 agent_api.go 中）
//   - 动态部分（PROACTIVE 建议 / 用户偏好 / 项目上下文）由本 Builder 注入到末尾
//   - 动态内容后置（nuclear-boy SystemPromptBuilder.kt L168-193）以优化缓存命中率
type SystemPromptBuilder struct {
	// basePrompt 是不含动态段落的 system prompt 原文。
	basePrompt string
	// userPreferences 是从 MemoryStore 加载的高置信度用户偏好。
	userPreferences []string
	// proactiveTriggers 累积待注入的 PROACTIVE 触发事件。
	proactiveTriggers []ProactiveTrigger
}

// NewSystemPromptBuilder 创建 Builder 实例。
func NewSystemPromptBuilder(basePrompt string) *SystemPromptBuilder {
	return &SystemPromptBuilder{
		basePrompt:        basePrompt,
		userPreferences:   nil,
		proactiveTriggers: nil,
	}
}

// AddUserPreference 添加一条用户偏好（来自 MemoryStore.LoadHighConfidence）。
// 来自 nuclear-boy SystemPromptBuilder.kt L168-193 appendUserPreferences。
func (b *SystemPromptBuilder) AddUserPreference(key, value string, confidence float64) *SystemPromptBuilder {
	if confidence < 0.5 {
		// 低于 0.5 不注入（避免噪声）
		return b
	}
	b.userPreferences = append(b.userPreferences, fmt.Sprintf("- %s: %s", key, value))
	return b
}

// AddProactiveTrigger 记录一次 PROACTIVE 触发事件。
// 调用方负责在合适的时机（例如每 5 轮对话或工具完成时）调用。
func (b *SystemPromptBuilder) AddProactiveTrigger(t ProactiveTrigger) *SystemPromptBuilder {
	b.proactiveTriggers = append(b.proactiveTriggers, t)
	return b
}

// Build 拼接最终 system prompt。
// 顺序：base → 用户偏好（高置信度，最多 20 条）→ PROACTIVE 段
//
// 借鉴 nuclear-boy SystemPromptBuilder.kt L168-193：动态内容放在最末（缓存友好）。
func (b *SystemPromptBuilder) Build() string {
	var sb strings.Builder
	sb.WriteString(b.basePrompt)

	// 注入用户偏好（最多 20 条，nuclear-boy 限制）
	if len(b.userPreferences) > 0 {
		sb.WriteString("\n\n## 用户偏好（从历史对话自动学习）\n")
		limit := len(b.userPreferences)
		if limit > 20 {
			limit = 20
		}
		for _, p := range b.userPreferences[:limit] {
			sb.WriteString(p)
			sb.WriteString("\n")
		}
	}

	// 注入 PROACTIVE 主动智能段
	if section := b.renderProactiveSection(); section != "" {
		sb.WriteString("\n\n")
		sb.WriteString(section)
	}

	return sb.String()
}

// renderProactiveSection 生成 "## 主动智能 (PROACTIVE)" 段文本。
// 模板来自 SystemPromptBuilder.kt L142-148。
func (b *SystemPromptBuilder) renderProactiveSection() string {
	if len(b.proactiveTriggers) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("## 主动智能 (PROACTIVE)\n")
	sb.WriteString("你是主动型助理。每次回复结尾，无需用户开口，主动给 2-3 条 `建议:`。\n")
	sb.WriteString("- 触发条件: 用户创建了新项目/搜索了资料/写了代码/完成了复杂任务/看起来不知道做什么\n")
	sb.WriteString("- 行为: 无需用户开口，主动给 2-3 条建议\n")
	sb.WriteString("- 边界: 不要问问题引导用户，要直接给方案\n\n")
	sb.WriteString("### 本轮触发\n")

	for _, t := range b.proactiveTriggers {
		switch t.Type {
		case ProactiveTriggerUserConsecutive:
			sb.WriteString(fmt.Sprintf("- 用户连续发了 %d 条消息（>3）。", t.UserMessageCount))
		case ProactiveTriggerToolCompleted:
			sb.WriteString(fmt.Sprintf("- 工具 %s 已完成。", t.ToolName))
		case ProactiveTriggerToolFailed:
			sb.WriteString(fmt.Sprintf("- 工具 %s 失败：%s。", t.ToolName, t.ToolError))
		case ProactiveTriggerTaskListEmpty:
			sb.WriteString("- 当前任务列表为空。")
		}
		if t.Context != "" {
			sb.WriteString(" 上下文: " + t.Context)
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// ValidatePrompt 校验 prompt 是否符合 5 大原则。
// 用于测试 + 调试：lint 阶段调用。
//
// 返回 error 表示违反原则；error.Message 描述具体问题。
//
// 来源：nuclear-boy HANDOVER2.0.md §五。
func ValidatePrompt(prompt string) error {
	// 原则 4：精简至上（≤ MaxPromptLength）
	length := utf8.RuneCountInString(prompt)
	if length > MaxPromptLength {
		return fmt.Errorf("prompt 长度 %d 超过上限 %d（原则 4：精简至上）", length, MaxPromptLength)
	}

	// 原则 2：避免否定表述（大小写不敏感）
	lower := strings.ToLower(prompt)
	for _, neg := range forbiddenNegations {
		if strings.Contains(lower, neg) {
			return fmt.Errorf("prompt 包含否定词 %q（原则 2：避免否定表述，会植入错误模式）", neg)
		}
	}

	return nil
}

// DisableThinkingRequestBody 返回 DeepSeek ChatRequest 中强制禁用 thinking 的 body 片段。
// 借鉴 DeepSeekApiClient.kt L327："Must be explicit: DeepSeek defaults to enabled!"。
//
// 调用方：拼装 ChatRequest 时把本函数返回的 map 合并到 body 中。
//   body["thinking"] = agent.DisableThinkingRequestBody()
func DisableThinkingRequestBody() map[string]interface{} {
	return map[string]interface{}{
		"type": "disabled",
	}
}

// EstimatePromptTokens 估算 prompt 的 token 数。
// 算法：len(text) / 3.5，nuclear-boy AgentEngine.kt L250-258。
// 借鉴到 Stage 6 的 EstimateTokens 通用函数；本文件先用局部版本。
func EstimatePromptTokens(text string) int64 {
	if len(text) == 0 {
		return 0
	}
	tokens := float64(len(text)) / 3.5
	if tokens < 20 {
		return 20
	}
	return int64(tokens)
}
