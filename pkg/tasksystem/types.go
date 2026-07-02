// Package tasksystem 提供可复用的任务系统核心抽象。
//
// 本包定义任务系统的核心接口（Task/Store/RollbackManager/TrashManager），
// 可被 encv 应用和其他 Go 应用复用。存储实现通过 Store 接口插拔，
// 当前提供 SQLite 实现（见 store/sqlite 子包）。
//
// 设计原则：
//   - 单一数据源：后端数据库为权威，前端缓存为辅助
//   - 可插拔存储：Store 接口支持 SQLite/PostgreSQL/内存等实现
//   - 可扩展回滚：RollbackStrategy 接口支持自定义操作类型的回滚策略
//   - 零应用耦合：本包不依赖任何应用层代码（internal/service 等）
package tasksystem

import (
	"time"

	"github.com/Soltus/encv-go/pkg/tasksystem/performance"
)

// TaskType 任务类型枚举。
// 包含加解密任务和文件操作任务，以及对应的回滚任务类型。
type TaskType string

const (
	TaskTypeEncrypt TaskType = "encrypt"
	TaskTypeDecrypt TaskType = "decrypt"
	TaskTypeMove    TaskType = "move"
	TaskTypeCopy    TaskType = "copy"
	TaskTypeRename  TaskType = "rename"
	TaskTypeDelete  TaskType = "delete"

	// 回滚任务类型（rollback_<原类型>）
	TaskTypeRollbackEncrypt TaskType = "rollback_encrypt"
	TaskTypeRollbackDecrypt TaskType = "rollback_decrypt"
	TaskTypeRollbackMove    TaskType = "rollback_move"
	TaskTypeRollbackCopy    TaskType = "rollback_copy"
	TaskTypeRollbackRename  TaskType = "rollback_rename"
	TaskTypeRollbackDelete  TaskType = "rollback_delete"

	// 系统维护任务类型（非文件操作，走任务系统以利用进度/耗时/取消能力）
	// 2026-07-03：FTS 索引重建任务化（spec fts-rebuild-task）
	TaskTypeRebuildFTSIndex TaskType = "rebuild_fts_index"
)

// IsRollback 判断任务类型是否为回滚任务。
// 回滚任务本身不支持二次回滚。
func (t TaskType) IsRollback() bool {
	switch t {
	case TaskTypeRollbackEncrypt, TaskTypeRollbackDecrypt,
		TaskTypeRollbackMove, TaskTypeRollbackCopy,
		TaskTypeRollbackRename, TaskTypeRollbackDelete:
		return true
	}
	return false
}

// TaskStatus 任务状态枚举。
type TaskStatus string

const (
	StatusQueued     TaskStatus = "queued"
	StatusRunning    TaskStatus = "running"
	StatusCancelling TaskStatus = "cancelling"
	StatusCompleted  TaskStatus = "completed"
	StatusFailed     TaskStatus = "failed"
	StatusCancelled  TaskStatus = "cancelled"
)

// IsTerminal 判断状态是否为终态。
// 终态任务不再接受状态变更（回滚任务除外，回滚是创建新任务）。
func (s TaskStatus) IsTerminal() bool {
	switch s {
	case StatusCompleted, StatusFailed, StatusCancelled:
		return true
	}
	return false
}

// TaskData 任务核心数据结构。
// 本包使用具体 struct 而非 interface，简化 Store 实现和序列化。
// 应用层可包装此 struct 实现 extraFields 等应用特定逻辑。
type TaskData struct {
	ID                 string     `json:"id"`
	Type               TaskType   `json:"type"`
	Status             TaskStatus `json:"status"`
	SourcePath         string     `json:"sourcePath,omitempty"`
	TargetPath         string     `json:"targetPath,omitempty"`
	OutputPath         string     `json:"outputPath,omitempty"`
	PluginName         string     `json:"pluginName,omitempty"`
	TriggeredBy        string     `json:"triggeredBy,omitempty"`
	RunID              string     `json:"runId,omitempty"`
	Progress           int        `json:"progress"`
	Phase              string     `json:"phase,omitempty"`
	Error              string     `json:"error,omitempty"`
	ErrorDetail        string     `json:"errorDetail,omitempty"`
	Warning            string     `json:"warning,omitempty"`
	WarningDetail      string     `json:"warningDetail,omitempty"`
	ContainerVersion   int        `json:"containerVersion,omitempty"`
	CipherMode         int        `json:"cipherMode,omitempty"`
	CompressionMode    string     `json:"compressionMode,omitempty"`
	ExtraFields        string     `json:"extraFields,omitempty"` // JSON 编码
	Steps              string     `json:"steps,omitempty"`       // JSON 编码
	MountID            string     `json:"mountId,omitempty"`
	MountSubPath       string     `json:"mountSubPath,omitempty"`
	TargetMountID      string     `json:"targetMountId,omitempty"`
	TargetMountSubPath string     `json:"targetMountSubPath,omitempty"`
	Password           string     `json:"password,omitempty"`
	SecondaryPassword  string     `json:"secondaryPassword,omitempty"`
	CreatedAt          time.Time  `json:"createdAt"`
	CompletedAt        *time.Time `json:"completedAt,omitempty"`

	// 回滚相关字段
	RollbackOf   string `json:"rollbackOf,omitempty"`   // 回滚任务指向原任务 ID
	OriginalPath string `json:"originalPath,omitempty"` // 原始路径（回滚用）
}

// TaskFilter 任务查询过滤。
// Store.ListTasks 接收此结构进行过滤、排序、分页。
type TaskFilter struct {
	Types       []TaskType
	Statuses    []TaskStatus
	TriggeredBy []string
	RunID       string
	RollbackOf  string // 查询某任务的回滚任务
	Limit       int
	Offset      int
}

// Snapshot 任务执行前后的状态快照。
// 用于回滚时恢复任务执行前的文件系统状态。
type Snapshot struct {
	ID           string    `json:"id"`
	TaskID       string    `json:"taskId"`
	SnapshotType string    `json:"snapshotType"` // "pre_state" | "post_state"
	Data         string    `json:"data"`         // JSON 编码的状态数据
	CreatedAt    time.Time `json:"createdAt"`
}

// SnapshotType 快照类型枚举。
const (
	SnapshotTypePreState  = "pre_state"
	SnapshotTypePostState = "post_state"
)

// TrashItem 回收站条目。
type TrashItem struct {
	ID             string    `json:"id"`
	OriginalPath   string    `json:"originalPath"`
	TrashPath      string    `json:"trashPath"`
	IsDirectory    bool      `json:"isDirectory"`
	Size           int64     `json:"size"`
	DeletedAt      time.Time `json:"deletedAt"`
	TaskID         string    `json:"taskId,omitempty"`
	RestoreTaskID  string    `json:"restoreTaskId,omitempty"`
	Metadata       string    `json:"metadata,omitempty"` // JSON 编码（mtime/mode 等）
}

// Store 任务存储接口。
// 可插拔，支持 SQLite/Turso/LibSQL 等实现。
type Store interface {
	// CreateTask 创建任务。
	CreateTask(task TaskData) error

	// GetTask 按 ID 查询任务。
	GetTask(id string) (TaskData, error)

	// ListTasks 按过滤条件查询任务列表。
	// 按 CreatedAt 倒序排列。
	ListTasks(filter TaskFilter) ([]TaskData, error)

	// UpdateTask 更新任务（全字段更新）。
	UpdateTask(task TaskData) error

	// DeleteTask 按 ID 删除任务（硬删除）。
	DeleteTask(id string) error

	// SaveSnapshot 保存任务快照。
	SaveSnapshot(snapshot Snapshot) error

	// GetSnapshot 查询任务的快照。
	// 返回最新的快照（pre_state 优先）。
	GetSnapshot(taskID string) (Snapshot, error)

	// CreateTrash 创建回收站条目。
	CreateTrash(item TrashItem) error

	// GetTrash 按 ID 查询回收站条目。
	GetTrash(id string) (TrashItem, error)

	// GetTrashByTaskID 按任务 ID 查询回收站条目。
	GetTrashByTaskID(taskID string) (TrashItem, error)

	// ListTrash 查询回收站列表，按 DeletedAt 倒序。
	ListTrash() ([]TrashItem, error)

	// UpdateTrash 更新回收站条目（如记录 restore_task_id）。
	UpdateTrash(item TrashItem) error

	// DeleteTrash 按 ID 删除回收站条目（硬删除，不删文件）。
	DeleteTrash(id string) error

	// CountByRunId 按 runId 统计各状态的任务数。
	// 返回 map[status]count，例如 {"completed": 1040, "failed": 15, "running": 5}
	// runId 为空时返回空 map（不统计 runId 为空的 task）。
	CountByRunId(runId string) (map[string]int, error)

	// ListRuns 列出所有 run（去重 runId），带最早创建时间和 triggeredBy。
	// 按 startedAt 倒序排列（最早的 task 的 created_at 代表 run 的启动时间）。
	ListRuns() ([]RunInfo, error)

	// ConcurrencyHint 返回推荐的并发写入协程数。
	//   - SQLite 返回 1（单连接串行写，避免 database is locked）
	//   - Turso 返回 8（MVCC 支持多 writer 并发写）
	ConcurrencyHint() int

	// SaveMetrics 保存性能指标。
	SaveMetrics(m performance.PerformanceMetrics) error

	// GetMetrics 按 task_id 查询性能指标。
	GetMetrics(taskID string) (performance.PerformanceMetrics, error)

	// ListMetricsByPlugin 按 plugin + taskType 查询历史性能指标。
	// 按 created_at 倒序，最多返回 limit 条。
	ListMetricsByPlugin(pluginName string, taskType string, limit int) ([]performance.PerformanceMetrics, error)

	// GetLatestMetrics 获取同 plugin + taskType 的上一次运行指标。
	GetLatestMetrics(pluginName string, taskType string) (*performance.PerformanceMetrics, error)

	// SaveCalibration 保存硬件校准结果。
	SaveCalibration(cal performance.CalibrationResult) error

	// GetCalibration 获取硬件校准结果。返回 nil 表示尚未校准。
	GetCalibration() (*performance.CalibrationResult, error)

	// ExportAll 导出全部数据（tasks + trash + snapshots + metrics + calibration）。
	// 返回通用 JSON 格式，用于跨引擎迁移和备份。
	ExportAll() (*DatabaseDump, error)

	// ImportAll 导入全部数据（全量替换）。
	// 导入前会清空现有数据。用于跨引擎迁移和恢复备份。
	ImportAll(dump *DatabaseDump) error

	// Close 关闭存储连接。
	Close() error
}

// DatabaseDump 数据库全量导出格式（跨引擎迁移的通用格式）。
//
// 所有引擎都导出为这个统一的 JSON 结构，导入时再转换回各自的存储格式。
// 支持 SQLite ↔ Turso ↔ LibSQL 之间的无缝迁移。
type DatabaseDump struct {
	Version     int                          `json:"version"`     // 格式版本，当前为 1
	Engine      string                       `json:"engine"`      // 导出时的引擎名（sqlite/turso/libsql）
	ExportedAt  time.Time                    `json:"exportedAt"`  // 导出时间
	Tasks       []TaskData                   `json:"tasks"`       // 任务列表
	Trash       []TrashItem                  `json:"trash"`       // 回收站
	Snapshots   []Snapshot                   `json:"snapshots"`   // 回滚快照
	Metrics     []performance.PerformanceMetrics `json:"metrics"`  // 性能指标
	Calibration *performance.CalibrationResult  `json:"calibration,omitempty"` // 硬件校准
}

// RunSummary run 的聚合计数（SQL COUNT + GROUP BY status 出）
//
// 用于 /api/runs/:runId/summary 响应。
// 前端 group card 显示这些数字，不靠 store.tasks 算（store 只持有视图分页的 task）。
type RunSummary struct {
	RunID     string `json:"runId"`
	Total     int    `json:"total"`
	Passed    int    `json:"passed"`
	Failed    int    `json:"failed"`
	Running   int    `json:"running"`
	Pending   int    `json:"pending"`
	Cancelled int    `json:"cancelled"`
	Percent   int    `json:"percent"` // 完成百分比（终态 task / total * 100）
}

// RunInfo run 列表项（带 summary，避免 N+1 查询）
//
// 用于 /api/runs 响应。
// ListRuns 返回所有 run 的基本信息，前端再决定是否调 /summary 拿详细计数。
type RunInfo struct {
	RunID       string    `json:"runId"`
	StartedAt   time.Time `json:"startedAt"`
	TriggeredBy string    `json:"triggeredBy"`
}

// RollbackStrategy 回滚策略接口。
// 每种 TaskType 对应一个实现，定义如何准备和执行回滚。
type RollbackStrategy interface {
	// CanRollback 判断任务是否可回滚。
	// 检查任务状态、快照存在性、所需资源可访问性。
	CanRollback(task TaskData, snapshot Snapshot) error

	// PrepareRollback 准备回滚任务数据。
	// 返回一个新的 TaskData，Type 为 rollback_*，RollbackOf 指向原任务。
	PrepareRollback(original TaskData, snapshot Snapshot) (TaskData, error)

	// ExecuteRollback 执行回滚操作。
	// 在回滚任务执行时调用，实际撤销原任务的副作用。
	ExecuteRollback(task TaskData, snapshot Snapshot) error
}

// RollbackManager 回滚管理器。
// 负责根据任务类型查找对应的 RollbackStrategy，并协调回滚流程。
type RollbackManager interface {
	// CanRollback 判断任务是否可回滚。
	CanRollback(taskID string) error

	// Rollback 执行回滚。
	// 创建一个新的回滚任务并异步执行，返回新任务 ID。
	Rollback(taskID string, triggeredBy string) (newTaskID string, err error)

	// RegisterStrategy 注册回滚策略。
	// 应用层在初始化时调用，为每种 TaskType 注册策略。
	RegisterStrategy(taskType TaskType, strategy RollbackStrategy)
}

// TrashManager 回收站管理器。
// 负责文件移到回收站、还原、列出、清空等操作。
type TrashManager interface {
	// MoveToTrash 将文件/目录移到回收站。
	// 返回创建的 TrashItem（含 trashPath）。
	MoveToTrash(originalPath string, taskID string) (TrashItem, error)

	// Restore 从回收站还原文件到指定路径。
	// destPath 为空则还原到 originalPath。
	// 创建还原任务并返回 taskID。
	Restore(trashID string, destPath string, triggeredBy string) (taskID string, err error)

	// List 列出回收站所有条目。
	List() ([]TrashItem, error)

	// Purge 永久删除回收站指定条目（os.Remove trashPath）。
	Purge(trashID string) error

	// Empty 清空回收站（删除所有条目及其文件）。
	Empty() error
}
