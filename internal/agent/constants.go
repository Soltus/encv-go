// Package agent 常量集中管理。
//
// 借鉴自 nuclear-boy common/src/main/java/com/nuclearboy/common/AppConstants.kt。
//
// 唯一来源原则：所有 token 阈值都在这里定义，调用方通过常量名引用，禁止硬编码数字。
package agent

// DeepSeek 上下文窗口与输出上限（来自 nuclear-boy AppConstants.kt L12-18）。
const (
	// DeepSeekContextWindow DeepSeek V4 上下文窗口大小（tokens）。
	DeepSeekContextWindow int64 = 1_000_000

	// DeepSeekMaxOutput DeepSeek V4 单次最大输出 tokens。
	DeepSeekMaxOutput int64 = 384_000
)

// 6 段预算分配（来自 nuclear-boy AppConstants.kt L21-26）。
const (
	// BudgetSystemPrompt 系统提示 token 预算。
	BudgetSystemPrompt int64 = 6_000
	// BudgetUserProfile 用户画像 token 预算。
	BudgetUserProfile int64 = 2_000
	// BudgetProjectContext 项目上下文 token 预算。
	BudgetProjectContext int64 = 50_000
	// BudgetConversationHistory 对话历史 token 预算。
	BudgetConversationHistory int64 = 100_000
	// BudgetToolDefinitions 工具定义 token 预算。
	BudgetToolDefinitions int64 = 5_000
	// BudgetAttachedFiles 附加文件 token 预算。
	BudgetAttachedFiles int64 = 200_000
)

// 上下文预警阈值（来自 nuclear-boy AppConstants.kt L28-31）。
//
// 命名空间隔离：这些是 int64 阈值常量，与 ContextWarningLevel 枚举
// 区分（后者是 ContextWindowManager 的返回类型）。
const (
	// ContextThresholdYellow 黄色预警阈值（80%，即 800K tokens）。
	ContextThresholdYellow int64 = 800_000
	// ContextThresholdRed 红色预警阈值（95%，即 950K tokens）。
	ContextThresholdRed int64 = 950_000
	// ContextThresholdForce 强制压缩阈值（98%，即 980K tokens）。
	ContextThresholdForce int64 = 980_000
)

// 压缩 / 截断阈值（来自 nuclear-boy AppConstants.kt L33-37）。
const (
	// ConversationCompressThreshold 对话历史达到此值时触发压缩。
	ConversationCompressThreshold int64 = 200_000
	// FileContentTruncateThreshold 文件内容截断阈值。
	FileContentTruncateThreshold int64 = 300_000
	// CriticalRemainingTokens 剩余 token 危急阈值。
	CriticalRemainingTokens int64 = 100_000
	// EmergencyRemainingTokens 剩余 token 紧急阈值。
	EmergencyRemainingTokens int64 = 50_000
)

// 字符/token 比例（EstimateTokens 算法用）。
//
// 来自 nuclear-boy AgentEngine.kt L250-258：text.length / 3.5，中英文混合近似。
const CharsPerToken = 3.5

// MinEstimatedTokens EstimateTokens 最小值（避免空文本返回 0）。
const MinEstimatedTokens int64 = 20
