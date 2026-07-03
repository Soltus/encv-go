package kernel

import (
	"encoding/json"
	"time"
)

// ─── 任务类型枚举 ────────────────────────────────────────────────────────
//
// 微内核中所有服务调用都被记录为 TaskRecord，TaskType 用于区分不同业务类型。
// 约定：{service}.{method} 格式，便于按服务/方法过滤。
//
// 常见类型示例：
//   - encrypt.encrypt       加密任务
//   - encrypt.decrypt       解密任务
//   - fts.rebuild           全文索引重建
//   - fts.search            全文搜索
//   - vector.build_index    向量索引构建
//   - vector.search         向量搜索
//   - cache.clean           缓存清理
//   - db.backup             数据库备份
//   - db.restore            数据库恢复
//   - plugin.install        插件安装
//   - plugin.uninstall      插件卸载
//   - tool.invoke           AI Agent 工具调用
//   - system.health         健康检查
//
// 设计原则：不做封闭枚举（服务可以动态注册），用字符串类型 + 常量约定。

// TaskType 任务类型字符串（{service}.{method} 约定）
type TaskType string

// 常用任务类型常量（非封闭，服务可自定义扩展）
const (
	TaskTypeEncryptEncrypt TaskType = "encrypt.encrypt"
	TaskTypeEncryptDecrypt TaskType = "encrypt.decrypt"
	TaskTypeFTSRebuild     TaskType = "fts.rebuild"
	TaskTypeFTSSearch      TaskType = "fts.search"
	TaskTypeVectorBuild    TaskType = "vector.build_index"
	TaskTypeVectorSearch   TaskType = "vector.search"
	TaskTypeCacheClean     TaskType = "cache.clean"
	TaskTypeDBBackup       TaskType = "db.backup"
	TaskTypeDBRestore      TaskType = "db.restore"
	TaskTypePluginInstall  TaskType = "plugin.install"
	TaskTypePluginUninstall TaskType = "plugin.uninstall"
	TaskTypeToolInvoke     TaskType = "tool.invoke"
	TaskTypeSystemHealth   TaskType = "system.health"
)

// TaskStatus 任务状态
type TaskStatus string

const (
	TaskStatusPending   TaskStatus = "pending"   // 已创建，等待执行
	TaskStatusRunning   TaskStatus = "running"   // 执行中
	TaskStatusSuccess   TaskStatus = "success"   // 成功完成
	TaskStatusFailed    TaskStatus = "failed"    // 失败
	TaskStatusCancelled TaskStatus = "cancelled" // 被取消
)

// IsTerminal 是否终态
func (s TaskStatus) IsTerminal() bool {
	return s == TaskStatusSuccess || s == TaskStatusFailed || s == TaskStatusCancelled
}

// TaskPriority 任务优先级
type TaskPriority int

const (
	TaskPriorityLow    TaskPriority = 0
	TaskPriorityNormal TaskPriority = 50
	TaskPriorityHigh   TaskPriority = 100
)

// ─── TaskRecord：统一任务记录 ────────────────────────────────────────────
//
// 微内核中所有服务调用都会生成一条 TaskRecord，记录完整的执行上下文。
// 用于：
//   - 任务历史查询
//   - 失败重试分析
//   - 性能监控（耗时分布）
//   - 审计追踪（谁在什么时候调用了什么）
//   - 多租户用量统计

// TaskRecord 任务记录（完整生命周期）
type TaskRecord struct {
	// ── 基础标识 ──
	ID        string    `json:"id"`        // 任务唯一 ID（ULID / UUID）
	TraceID   string    `json:"traceID"`   // 链路追踪 ID（ServiceContext.TraceID）
	RequestID string    `json:"requestID"` // 请求 ID（ServiceContext.RequestID）
	ParentID  string    `json:"parentID,omitempty"` // 父任务 ID（如有）

	// ── 业务类型 ──
	Type    TaskType `json:"type"`    // 任务类型（{service}.{method}）
	Service string   `json:"service"` // 服务名（冗余，便于按服务过滤）
	Method  string   `json:"method"`  // 方法名（冗余）

	// ── 生命周期时间点 ──
	CreatedAt   time.Time  `json:"createdAt"`            // 创建时间
	StartedAt   *time.Time `json:"startedAt,omitempty"`  // 开始执行时间
	CompletedAt *time.Time `json:"completedAt,omitempty"`// 完成时间（终态）
	UpdatedAt   time.Time  `json:"updatedAt"`            // 最后更新时间

	// ── 状态与进度 ──
	Status    TaskStatus `json:"status"`    // 当前状态
	Progress  int        `json:"progress"`  // 0-100 进度百分比
	Phase     string     `json:"phase,omitempty"`     // 当前阶段描述
	Speed     string     `json:"speed,omitempty"`     // 执行速度（"12.5 MB/s"）
	Eta       string     `json:"eta,omitempty"`       // 预计剩余时间
	Priority  TaskPriority `json:"priority"`          // 优先级

	// ── 输入输出（可配置是否存储 payload，避免敏感数据泄露） ──
	InputJSON  json.RawMessage `json:"inputJSON,omitempty"`  // 输入参数（JSON 快照）
	OutputJSON json.RawMessage `json:"outputJSON,omitempty"` // 输出结果（JSON 快照）

	// ── 错误信息 ──
	Error       string `json:"error,omitempty"`       // 错误摘要
	ErrorDetail string `json:"errorDetail,omitempty"` // 错误详情（堆栈/详细信息）

	// ── 资源用量 ──
	DurationMs    int64 `json:"durationMs,omitempty"`    // 总耗时（毫秒）
	PeakMemBytes  int64 `json:"peakMemBytes,omitempty"`  // 峰值内存（字节，可选）
	InputBytes    int64 `json:"inputBytes,omitempty"`    // 输入数据量
	OutputBytes   int64 `json:"outputBytes,omitempty"`   // 输出数据量

	// ── 重试 ──
	Attempts    int      `json:"attempts"`    // 当前重试次数
	MaxAttempts int      `json:"maxAttempts"` // 最大重试次数
	RetryReasons []string `json:"retryReasons,omitempty"` // 每次重试的原因

	// ── 多租户 ──
	TenantID string `json:"tenantID,omitempty"` // 租户 ID

	// ── 触发来源 ──
	TriggeredBy string `json:"triggeredBy,omitempty"` // 触发者：'user' | 'automation' | 'ai_agent' | 'system'
	RunID       string `json:"runId,omitempty"`       // 关联的 workflow run ID

	// ── 标签（灵活扩展） ──
	Tags map[string]string `json:"tags,omitempty"` // 自定义标签（如 sourcePath、pluginName 等）
}

// TaskRecordFilter 任务查询过滤器
type TaskRecordFilter struct {
	Types       []TaskType   `json:"types,omitempty"`       // 按类型过滤
	Services    []string     `json:"services,omitempty"`    // 按服务过滤
	Statuses    []TaskStatus `json:"statuses,omitempty"`    // 按状态过滤
	TenantID    string       `json:"tenantID,omitempty"`    // 按租户过滤
	TriggeredBy string       `json:"triggeredBy,omitempty"` // 按触发者过滤
	RunID       string       `json:"runId,omitempty"`       // 按 run ID 过滤

	CreatedAfter  *time.Time `json:"createdAfter,omitempty"`  // 创建时间范围（起）
	CreatedBefore *time.Time `json:"createdBefore,omitempty"` // 创建时间范围（止）

	Limit  int `json:"limit"`  // 分页：每页数量（默认 50）
	Offset int `json:"offset"` // 分页：偏移量

	SortBy    string `json:"sortBy"`    // 排序字段：createdAt | updatedAt | durationMs | priority
	SortOrder string `json:"sortOrder"` // asc | desc（默认 desc）
}

// TaskStats 任务统计
type TaskStats struct {
	Total     int64 `json:"total"`     // 总任务数
	Pending   int64 `json:"pending"`   // 等待中
	Running   int64 `json:"running"`   // 执行中
	Success   int64 `json:"success"`   // 成功
	Failed    int64 `json:"failed"`    // 失败
	Cancelled int64 `json:"cancelled"` // 已取消

	AvgDurationMs int64 `json:"avgDurationMs"` // 平均耗时
	MaxDurationMs int64 `json:"maxDurationMs"` // 最大耗时
	P95DurationMs int64 `json:"p95DurationMs"` // P95 耗时

	ByType   map[TaskType]int64 `json:"byType,omitempty"`   // 按类型统计
	ByService map[string]int64  `json:"byService,omitempty"`// 按服务统计
	ByTenant  map[string]int64  `json:"byTenant,omitempty"` // 按租户统计
}
