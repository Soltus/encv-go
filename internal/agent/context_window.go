// Stage 6 (borrow-nuclear-boy-2026q2)：ContextWindowManager 管理 6 段 token 预算分配与三级预警。
//
// 借鉴自 /tmp/nuclear-boy/api-deepseek/src/main/java/com/nuclearboy/api/deepseek/ContextWindowManager.kt。
//
// 6 段分配（来自 nuclear-boy ContextWindowManager.kt L12-19）：
//   - systemPrompt   系统提示
//   - userProfile    用户画像
//   - projectContext 项目上下文
//   - conversationHistory 对话历史
//   - toolDefinitions 工具定义
//   - attachedFiles  附加文件
//
// 4 级预警：
//   - OK         0 - 30%  深绿，正常
//   - GREEN      30-80%   浅绿，良好
//   - YELLOW     80-95%   黄色，提示
//   - RED        95-98%   红色，危险
//   - FORCE      98-100%  强制压缩
package agent

import "sync"

// ContextWarningLevel 预警等级（对应 nuclear-boy L39-44）。
type ContextWarningLevel int

const (
	// ContextWarningOK 0-30%，深绿。
	ContextWarningOK ContextWarningLevel = iota
	// ContextWarningGreen 30-80%，浅绿。
	ContextWarningGreen
	// ContextWarningYellow 80-95%，黄色。
	ContextWarningYellow
	// ContextWarningRed 95-98%，红色。
	ContextWarningRed
	// ContextWarningForce ≥98%，强制压缩。
	ContextWarningForce
)

// String 返回等级名（便于 DevLogs / SSE event 序列化）。
func (l ContextWarningLevel) String() string {
	switch l {
	case ContextWarningOK:
		return "ok"
	case ContextWarningGreen:
		return "green"
	case ContextWarningYellow:
		return "yellow"
	case ContextWarningRed:
		return "red"
	case ContextWarningForce:
		return "force"
	default:
		return "unknown"
	}
}

// ContextBudget 6 段 token 预算（对应 nuclear-boy ContextWindowManager.kt L12-19）。
type ContextBudget struct {
	SystemPrompt         int64 `json:"systemPrompt"`
	UserProfile          int64 `json:"userProfile"`
	ProjectContext       int64 `json:"projectContext"`
	ConversationHistory  int64 `json:"conversationHistory"`
	ToolDefinitions      int64 `json:"toolDefinitions"`
	AttachedFiles        int64 `json:"attachedFiles"`
}

// TotalUsed 6 段总和。
func (b ContextBudget) TotalUsed() int64 {
	return b.SystemPrompt + b.UserProfile + b.ProjectContext +
		b.ConversationHistory + b.ToolDefinitions + b.AttachedFiles
}

// Remaining 剩余 tokens。
func (b ContextBudget) Remaining() int64 {
	r := DeepSeekContextWindow - b.TotalUsed()
	if r < 0 {
		return 0
	}
	return r
}

// UsagePercent 占用比例 0.0 - 1.0+。
func (b ContextBudget) UsagePercent() float64 {
	return float64(b.TotalUsed()) / float64(DeepSeekContextWindow)
}

// WarningLevel 当前预算的预警等级。
func (b ContextBudget) WarningLevel() ContextWarningLevel {
	used := b.TotalUsed()
	switch {
	case used >= ContextThresholdForce:
		return ContextWarningForce
	case used >= ContextThresholdRed:
		return ContextWarningRed
	case used >= ContextThresholdYellow:
		return ContextWarningYellow
	case used >= DeepSeekContextWindow*3/10:
		return ContextWarningGreen
	default:
		return ContextWarningOK
	}
}

// CanFit 是否有足够空间放 additionalTokens。
func (b ContextBudget) CanFit(additionalTokens int64) bool {
	return b.Remaining() >= additionalTokens
}

// NeedsCompression 是否需要压缩（YELLOW 及以上）。
func (b ContextBudget) NeedsCompression() bool {
	return b.TotalUsed() >= ContextThresholdYellow
}

// NeedsUrgentCompression 是否需要紧急压缩（RED 及以上）。
func (b ContextBudget) NeedsUrgentCompression() bool {
	return b.TotalUsed() >= ContextThresholdRed
}

// CompressionResult 压缩结果（对应 nuclear-boy L46-50）。
type CompressionResult struct {
	TokensSaved int64         `json:"tokensSaved"`
	NewBudget   ContextBudget `json:"newBudget"`
	Message     string        `json:"message"`
}

// ContextWindowManager 线程安全的 6 段预算管理器。
//
// 借鉴 nuclear-boy ContextWindowManager.kt L52-183。
type ContextWindowManager struct {
	mu sync.RWMutex
	// budget 当前预算快照。
	budget ContextBudget
	// conversationTurnCount 累计轮数（用于压缩策略）。
	conversationTurnCount int
	// conversationSummaries 历史压缩摘要。
	conversationSummaries []string
}

// NewContextWindowManager 构造管理器（初始预算全 0）。
func NewContextWindowManager() *ContextWindowManager {
	return &ContextWindowManager{
		budget:                ContextBudget{},
		conversationSummaries: make([]string, 0, 8),
	}
}

// Budget 读快照（拷贝返回，调用方可安全修改）。
func (m *ContextWindowManager) Budget() ContextBudget {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.budget
}

// CanFit 是否有足够空间。
func (m *ContextWindowManager) CanFit(additionalTokens int64) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.budget.CanFit(additionalTokens)
}

// NeedsCompression 是否需要压缩。
func (m *ContextWindowManager) NeedsCompression() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.budget.NeedsCompression()
}

// NeedsUrgentCompression 是否需要紧急压缩。
func (m *ContextWindowManager) NeedsUrgentCompression() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.budget.NeedsUrgentCompression()
}

// UpdateAllocation 增量更新 6 段预算（nil 表示保持原值）。
// 借鉴 nuclear-boy ContextWindowManager.kt L84-100。
func (m *ContextWindowManager) UpdateAllocation(
	systemPrompt, userProfile, projectContext, conversationHistory, toolDefinitions, attachedFiles *int64,
) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if systemPrompt != nil {
		m.budget.SystemPrompt = *systemPrompt
	}
	if userProfile != nil {
		m.budget.UserProfile = *userProfile
	}
	if projectContext != nil {
		m.budget.ProjectContext = *projectContext
	}
	if conversationHistory != nil {
		m.budget.ConversationHistory = *conversationHistory
	}
	if toolDefinitions != nil {
		m.budget.ToolDefinitions = *toolDefinitions
	}
	if attachedFiles != nil {
		m.budget.AttachedFiles = *attachedFiles
	}
}

// CompressConversation 压缩对话历史（YELLOW 时调用）。
// 借鉴 nuclear-boy ContextWindowManager.kt L106-137。
//
// 行为：把 conversationHistory 减半（最少 50K），返回节省的 token 数。
func (m *ContextWindowManager) CompressConversation(turnCount int) CompressionResult {
	m.mu.Lock()
	defer m.mu.Unlock()

	cur := m.budget
	if cur.ConversationHistory < ConversationCompressThreshold {
		return CompressionResult{
			TokensSaved: 0,
			NewBudget:   cur,
			Message:     "上下文还很充裕，不需要压缩",
		}
	}

	targetReduction := cur.ConversationHistory / 2
	newConversationTokens := cur.ConversationHistory - targetReduction
	if newConversationTokens < 50_000 {
		newConversationTokens = 50_000
	}

	summary := "（较早的对话已压缩：包含 " + itoa(turnCount/2) + " 轮对话的关键信息）"
	m.conversationSummaries = append(m.conversationSummaries, summary)

	m.budget.ConversationHistory = newConversationTokens

	return CompressionResult{
		TokensSaved: targetReduction,
		NewBudget:   m.budget,
		Message:     "我帮你整理了一下前面的对话，节省了 " + itoa64(targetReduction/1000) + "K tokens 的上下文空间 ✨",
	}
}

// EmergencyCompress 紧急压缩（RED 时调用）。
// 借鉴 nuclear-boy ContextWindowManager.kt L142-159。
//
// 行为：conversationHistory × 0.3 / attachedFiles × 0.5 / projectContext × 0.7
func (m *ContextWindowManager) EmergencyCompress() CompressionResult {
	m.mu.Lock()
	defer m.mu.Unlock()

	cur := m.budget
	target := cur.TotalUsed() - DeepSeekContextWindow/2
	if target < 0 {
		target = 0
	}

	m.budget.ConversationHistory = max64(cur.ConversationHistory*3/10, 10_000)
	m.budget.AttachedFiles = cur.AttachedFiles * 5 / 10
	m.budget.ProjectContext = cur.ProjectContext * 7 / 10

	return CompressionResult{
		TokensSaved: target,
		NewBudget:   m.budget,
		Message:     "上下文快满了，我做了一次深度压缩，释放了约 " + itoa64(target/1000) + "K tokens。早期对话和部分文件内容被精简了。",
	}
}

// Reset 重置（用于新会话）。
func (m *ContextWindowManager) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.budget = ContextBudget{}
	m.conversationTurnCount = 0
	m.conversationSummaries = m.conversationSummaries[:0]
}

// IncrementTurn 增加轮数。
func (m *ContextWindowManager) IncrementTurn() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.conversationTurnCount++
}

// ConversationSummaries 读历史压缩摘要。
func (m *ContextWindowManager) ConversationSummaries() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]string, len(m.conversationSummaries))
	copy(out, m.conversationSummaries)
	return out
}

// EstimateTokens 估算文本的 token 数（中英文混合 ~3.5 字符/token，最小 20）。
//
// 借鉴 nuclear-boy AgentEngine.kt L250-258 + ContextWindowManager.kt L164-167。
func EstimateTokens(text string) int64 {
	est := int64(float64(len(text)) / CharsPerToken)
	if est < MinEstimatedTokens {
		est = MinEstimatedTokens
	}
	return est
}

// itoa 简单 int → string（避免 strconv 引入开销）。
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	negative := false
	if n < 0 {
		negative = true
		n = -n
	}
	buf := make([]byte, 0, 12)
	for n > 0 {
		buf = append(buf, byte('0'+n%10))
		n /= 10
	}
	if negative {
		buf = append(buf, '-')
	}
	// reverse
	for i, j := 0, len(buf)-1; i < j; i, j = i+1, j-1 {
		buf[i], buf[j] = buf[j], buf[i]
	}
	return string(buf)
}

// itoa64 int64 → string。
func itoa64(n int64) string {
	if n == 0 {
		return "0"
	}
	negative := false
	if n < 0 {
		negative = true
		n = -n
	}
	buf := make([]byte, 0, 20)
	for n > 0 {
		buf = append(buf, byte('0'+n%10))
		n /= 10
	}
	if negative {
		buf = append(buf, '-')
	}
	for i, j := 0, len(buf)-1; i < j; i, j = i+1, j-1 {
		buf[i], buf[j] = buf[j], buf[i]
	}
	return string(buf)
}

// max64 int64 max。
func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
