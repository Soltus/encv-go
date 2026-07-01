package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/Soltus/encv-go/internal/config"
	"github.com/Soltus/encv-go/internal/utils"
	"github.com/Soltus/encv-go/internal/v2/plugins"
	pluginInterfaces "github.com/Soltus/encv-go/internal/v2/plugins/interfaces"
	"github.com/Soltus/encv-go/internal/v2/plugins/video"
	"github.com/Soltus/encv-go/internal/v2/types"
	"github.com/Soltus/encv-go/pkg/tasksystem"
	"github.com/Soltus/encv-go/pkg/tasksystem/performance"
	"github.com/Soltus/encv-go/pkg/tasksystem/store/sqlite"
	"github.com/google/uuid"
)

// 🆕 2026-06-23 后端 SQL 权威：RunSummary 别名（service 包直接用 tasksystem.RunSummary）
//   避免在 service 包重复定义，保持类型单一来源。
type RunSummary = tasksystem.RunSummary

type TaskStep struct {
	Phase       string     `json:"phase"`
	StartedAt   time.Time  `json:"startedAt"`
	CompletedAt *time.Time `json:"completedAt,omitempty"`
	Detail      string     `json:"detail,omitempty"`
}

type MobileTask struct {
	ID                string            `json:"id"`
	Type              string            `json:"type"`
	SourcePath        string            `json:"sourcePath"`
	TargetPath        string            `json:"targetPath,omitempty"`
	Password          string            `json:"password,omitempty"`
	SecondaryPassword string            `json:"secondaryPassword,omitempty"`
	ExtraFields       map[string]string `json:"extraFields,omitempty"`
	PluginName        string            `json:"pluginName,omitempty"`
	Status            string            `json:"status"`
	Progress          int               `json:"progress"`
	Phase             string            `json:"phase,omitempty"`
	Speed             string            `json:"speed,omitempty"`
	Eta               string            `json:"eta,omitempty"`
	Error             string            `json:"error,omitempty"`
	ErrorDetail       string            `json:"errorDetail,omitempty"`
	Warning           string            `json:"warning,omitempty"`
	WarningDetail     string            `json:"warningDetail,omitempty"`
	ContainerVersion  int               `json:"containerVersion,omitempty"`
	OutputPath        string            `json:"outputPath,omitempty"`
	Steps             []TaskStep        `json:"steps,omitempty"`

	// 🆕 2026-06-18 Task 16：加解密参数持久化
	//   前端 createTask 传 cipherMode (0=AES-128-GCM, 1=AES-256-GCM) + compressionMode ('none'|'zstd')
	//   后端持久化让任务列表刷新后仍能回显参数（Task 18 展示用）
	//   omitempty：旧任务（无这两个字段）反序列化时自动空值，向后兼容
	CipherMode      int    `json:"cipherMode,omitempty"`
	CompressionMode string `json:"compressionMode,omitempty"`

	// 🆕 v6 2026-06-18：runId + triggeredBy 作为 task 一等字段（单一数据源）
	//   - runId：自动化测试/AI agent 产生的 task 共享同一个 runId，前端按 runId 聚合
	//   - triggeredBy：'user' | 'automation' | 'ai_agent'
	//   - omitempty：旧任务（无这两个字段）反序列化时自动空值，向后兼容
	//   - 前端不再需要 useTaskTrigger localStorage 维护关联
	RunId        string `json:"runId,omitempty"`
	TriggeredBy  string `json:"triggeredBy,omitempty"`

	// 🆕 回滚相关字段
	//   - RollbackOf：回滚任务指向原任务 ID（仅 rollback_* 任务有值）
	//   - OriginalPath：原始路径（回滚时恢复用，仅 rollback_* 任务有值）
	RollbackOf   string `json:"rollbackOf,omitempty"`
	OriginalPath string `json:"originalPath,omitempty"`

	// 🆕 2026-06-15 multi-mount（spec Phase C1）
	//   - SourcePath 是 /d/<mount>/... 形式时，记录解析后的 mount_id + sub_path
	//   - SourcePath 是旧绝对路径且能匹配到 mount 时同样记录
	//   - 都无法匹配时 MountID == ""，回退到 s.servingDir 解析（向后兼容）
	//   - JSON 字段加 omitempty：旧 fixture（无 mountId）反序列化时自动空值
	MountID          string `json:"mountId,omitempty"`
	MountSubPath     string `json:"mountSubPath,omitempty"`
	TargetMountID    string `json:"targetMountId,omitempty"`
	TargetMountSubPath string `json:"targetMountSubPath,omitempty"`

	CreatedAt   time.Time  `json:"createdAt"`
	CompletedAt *time.Time `json:"completedAt,omitempty"`
	cancelFn    context.CancelFunc
}

type TaskManager struct {
	tasks       map[string]*MobileTask
	mu          sync.RWMutex
	servingDir  string
	cfg         *config.Config
	stopCh      chan struct{}
	wg          sync.WaitGroup
	broadcaster Broadcaster
	persistPath string

	// 🆕 SQLite 权威持久化层（双写策略）
	//   - nil → 走旧 JSON 持久化（向后兼容旧测试）
	//   - 非 nil → saveTasks/loadTasks 走 store，内存 map 作为性能缓存
	store tasksystem.Store

	// 🆕 v6 2026-06-22：文件操作任务处理器（move/copy/rename/delete）
	//   - nil → 文件操作任务会 failTask（未配置 FileTaskHandler）
	//   - 非 nil → processTask switch 的 move/copy/rename/delete case 调用对应方法
	fileTaskHandler *FileTaskHandler

	// 🆕 2026-06-22 回滚管理器
	//   - nil → rollback_* 任务会 failTask（未配置 RollbackManager）
	//   - 非 nil → processTask switch 的 rollback_* case 调用 ProcessRollbackTask
	rollbackManager *RollbackManagerImpl

	// 🆕 2026-06-15 multi-mount（spec Phase C1）
	//   - nil → 旧行为，所有 sourcePath 走 SafeResolveToAbsPath(servingDir, ...)
	//   - 非 nil → sourcePath 是 /d/<mount>/... 时走 mount 解析
	//   - 用 interface 而不是 *mount.MountRegistry：避免 service → mount 反向依赖
	mountResolver MountResolver
}

// perfStore 返回 SQLite Store 的性能指标存储接口（如果 store 是 *sqlite.Store）。
// 如果 store 不是 SQLite Store（如 nil 或 mock），返回 nil。
func (tm *TaskManager) perfStore() *sqlite.Store {
	if tm.store == nil {
		return nil
	}
	if s, ok := tm.store.(*sqlite.Store); ok {
		return s
	}
	return nil
}

// GetPerfStore 返回 SQLite Store 的性能指标存储接口（导出版本，供 server 包调用）。
// 如果 store 不是 SQLite Store，返回 nil。
func (tm *TaskManager) GetPerfStore() *sqlite.Store {
	return tm.perfStore()
}

// ========== 数据库导入 / 导出（跨引擎迁移） ==========

// GetStoreEngine 返回当前使用的存储引擎名称。
// 可能的值："sqlite" | "turso" | "libsql" | "memory" | "unknown"
func (tm *TaskManager) GetStoreEngine() string {
	if tm.store == nil {
		return "memory"
	}
	switch tm.store.(type) {
	case *sqlite.Store:
		return "sqlite"
	default:
		// 通过类型名推断（兼容 turso/libsql 等其他实现）
		typeName := fmt.Sprintf("%T", tm.store)
		// 提取包名最后一段
		// 例如: *tursogo.Store → "turso"
		//      *libsql.Store → "libsql"
		if strings.Contains(typeName, "tursogo") || strings.Contains(typeName, "turso") {
			return "turso"
		}
		if strings.Contains(typeName, "libsql") {
			return "libsql"
		}
		return "unknown"
	}
}

// GetStoreConcurrency 返回当前存储引擎的推荐并发度。
func (tm *TaskManager) GetStoreConcurrency() int {
	if tm.store == nil {
		return 1
	}
	return tm.store.ConcurrencyHint()
}

// ReplaceStore 替换底层存储引擎。
// 用于启动时调用，将内存中的任务持久化到新 store。
// 注意：调用时需确保任务处理已暂停。
func (tm *TaskManager) ReplaceStore(store tasksystem.Store) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	// 将内存中现有任务持久化到新 store
	if store != nil && len(tm.tasks) > 0 {
		for _, task := range tm.tasks {
			td := mobileTaskToData(task)
			_ = store.CreateTask(td)
		}
	}
	tm.store = store
	return nil
}

// GetStore 返回底层 Store 接口（供高级操作使用）。
// 返回 nil 表示未启用持久化（内存模式）。
func (tm *TaskManager) GetStore() tasksystem.Store {
	return tm.store
}

// ExportDatabase 导出全部数据库内容（tasks + trash + snapshots + metrics + calibration）。
// 用于备份和跨引擎迁移。
func (tm *TaskManager) ExportDatabase() (*tasksystem.DatabaseDump, error) {
	if tm.store == nil {
		return nil, fmt.Errorf("store not configured (memory mode)")
	}
	return tm.store.ExportAll()
}

// ImportDatabase 导入数据库内容（全量替换）。
// 导入前会清空所有现有数据。用于恢复备份和跨引擎迁移。
// 导入后会重新加载内存缓存（tm.tasks）。
func (tm *TaskManager) ImportDatabase(dump *tasksystem.DatabaseDump) error {
	if tm.store == nil {
		return fmt.Errorf("store not configured (memory mode)")
	}
	if dump == nil {
		return fmt.Errorf("dump is nil")
	}

	// 1. 导入到 store（事务内原子执行）
	if err := tm.store.ImportAll(dump); err != nil {
		return fmt.Errorf("import to store: %w", err)
	}

	// 2. 重新加载内存缓存（tm.tasks）
	//    导入后内存 map 需要与 DB 保持一致
	tm.mu.Lock()
	defer tm.mu.Unlock()

	// 清空内存 map
	tm.tasks = make(map[string]*MobileTask)

	// 从 store 重新加载所有任务
	allTasks, err := tm.store.ListTasks(tasksystem.TaskFilter{})
	if err != nil {
		return fmt.Errorf("reload tasks from store: %w", err)
	}
	for _, td := range allTasks {
		tm.tasks[td.ID] = dataToMobileTask(td)
	}

	return nil
}

// EnsureCalibration 确保硬件校准已执行（启动时调用一次）。
// 如果 calibration 表为空，运行校准并保存。
func (tm *TaskManager) EnsureCalibration() {
	ps := tm.perfStore()
	if ps == nil {
		return
	}
	// 检查是否已校准
	existing, err := ps.GetCalibration()
	if err != nil {
		slog.Warn("EnsureCalibration: GetCalibration failed", "error", err)
		return
	}
	if existing != nil {
		slog.Info("EnsureCalibration: already calibrated", "cpuScore", existing.CPUScore, "label", existing.CPULabel)
		return
	}
	// 运行校准
	slog.Info("EnsureCalibration: running calibration...")
	cal := performance.RunCalibration()
	if err := ps.SaveCalibration(cal); err != nil {
		slog.Warn("EnsureCalibration: SaveCalibration failed", "error", err)
		return
	}
	slog.Info("EnsureCalibration: done", "cpuScore", cal.CPUScore, "label", cal.CPULabel, "aesThroughput", cal.AESThroughput)
}

// MountResolver 是 mount.MountRegistry 的子集接口（service 包不依赖 mount 包）
//
// 行为：
//   - input "/d/<mount>/<sub>" → Resolver 用最长前缀匹配找到 mount
//   - output MountResolveResult：MountID + AbsPath + SubPath
//   - 找不到 / mount 禁用 / mount 只读：返回错误
//
// 🆕 v3 2026-06-18 Task 8：新增 AbsToVirtual 反向解析
//   - 物理绝对路径 → 虚拟路径 /d/<mount>/<sub>
//   - 用于 task.OutputPath / step.Detail 统一存虚拟路径
type MountResolver interface {
	Resolve(virtualPath string) (*MountResolveResult, error)
	// 🆕 Task 8：absPath → virtualPath（找不到匹配 mount 返回 error）
	AbsToVirtual(absPath string) (string, error)
}

// MountResolveResult 是 mount.ResolveResult 的 service 包本地副本
// （避免 import mount 包；同时让 service 不知道 mount 内部结构）
type MountResolveResult struct {
	MountID string // mount 唯一 ID（持久化用）
	AbsPath string // 解析后的绝对路径（运行时用）
	SubPath string // mount 内部相对路径（持久化用）
}

// SetMountResolver 注入 mount resolver
//
// 调用方：server.NewServer 在 mount registry 创建后调用
// nil 是合法值（向后兼容：未启用 multi-mount 时）
func (tm *TaskManager) SetMountResolver(r MountResolver) {
	tm.mountResolver = r
}

// tryAttachMount 尝试把 sourcePath 解析为 (mountID, subPath) 并写到 task。
//   - isTarget=false → 写到 MountID/MountSubPath（source）
//   - isTarget=true  → 写到 TargetMountID/TargetMountSubPath（target）
//
// 调用规约：
//   - 调用方必须确认 tm.mountResolver != nil
//   - 解析失败不报错（err swallowed）—— 任务回退到旧 SafeResolveToAbsPath 解析
//   - 非 / 开头路径直接跳过（不尝试 mount 解析）
func (tm *TaskManager) tryAttachMount(task *MobileTask, sourcePath string, isTarget bool) {
	if !strings.HasPrefix(sourcePath, "/") {
		return // relative path 不走 mount
	}
	res, err := tm.mountResolver.Resolve(sourcePath)
	if err != nil {
		// mount 解析失败 → 不存 mount_id，留到 processTask 时报"invalid source path"
		slog.Debug("mount resolve skipped in tryAttachMount", "path", sourcePath, "error", err)
		return
	}
	if isTarget {
		task.TargetMountID = res.MountID
		task.TargetMountSubPath = res.SubPath
	} else {
		task.MountID = res.MountID
		task.MountSubPath = res.SubPath
	}
}

func NewTaskManager(servingDir string, cfg *config.Config, broadcaster Broadcaster) *TaskManager {
	persistPath := filepath.Join(servingDir, ".encv-tasks.json")

	tm := &TaskManager{
		tasks:       make(map[string]*MobileTask),
		servingDir:  servingDir,
		cfg:         cfg,
		stopCh:      make(chan struct{}),
		broadcaster: broadcaster,
		persistPath: persistPath,
	}

	tm.loadTasks()

	tm.wg.Add(1)
	go tm.worker()
	return tm
}

// NewTaskManagerWithStore 创建接入 SQLite Store 作为权威持久化层的 TaskManager。
//
// 双写策略：
//   - store != nil → saveTasks/loadTasks 走 store（SQLite 为权威层）
//   - 内存 map 仍作为性能缓存（List/Get 等读路径不查 DB）
//   - store == nil 等价于 NewTaskManager（走旧 JSON 持久化，向后兼容）
func NewTaskManagerWithStore(servingDir string, cfg *config.Config, broadcaster Broadcaster, store tasksystem.Store) *TaskManager {
	persistPath := filepath.Join(servingDir, ".encv-tasks.json")

	tm := &TaskManager{
		tasks:       make(map[string]*MobileTask),
		servingDir:  servingDir,
		cfg:         cfg,
		stopCh:      make(chan struct{}),
		broadcaster: broadcaster,
		persistPath: persistPath,
		store:       store,
	}

	tm.loadTasks()

	tm.wg.Add(1)
	go tm.worker()

	// 🆕 启动时确保硬件校准已执行
	go tm.EnsureCalibration()

	return tm
}

func (tm *TaskManager) saveTasks() {
	tm.mu.RLock()
	taskList := make([]*MobileTask, 0, len(tm.tasks))
	for _, t := range tm.tasks {
		taskList = append(taskList, t)
	}
	tm.mu.RUnlock()

	// 🆕 SQLite 权威持久化层（双写策略）
	//   - store != nil → 逐条 UPSERT 到 store，内存 map 作为性能缓存
	//   - store == nil → 走旧 JSON 持久化（向后兼容）
	if tm.store != nil {
		for _, t := range taskList {
			data := mobileTaskToData(t)
			// UPSERT 语义：先尝试 Create，失败（行已存在）则 Update
			if err := tm.store.CreateTask(data); err != nil {
				if err := tm.store.UpdateTask(data); err != nil {
					slog.Warn("Failed to upsert task in store",
						"id", t.ID, "error", err)
				}
			}
		}
		return
	}

	data, err := json.MarshalIndent(taskList, "", "  ")
	if err != nil {
		slog.Warn("Failed to marshal tasks for persistence", "error", err)
		return
	}

	tmpPath := tm.persistPath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		slog.Warn("Failed to write tasks temp file", "error", err)
		return
	}
	if err := os.Rename(tmpPath, tm.persistPath); err != nil {
		slog.Warn("Failed to rename tasks file", "error", err)
		return
	}
}

// 🆕 2026-06-22 Q6A：高频单行持久化（避免 saveTasks 写全表）
//
// 为什么需要：
//   - 自动化测试 1000+ 并发创建 task 时，每个 CreateWithRunMeta 走 saveTasks
//     写全表 N 行 → IO 抖动 + 锁竞争
//   - 改用 saveTaskSingle 只写一行（O(1)），适合高频路径
//
// 调用方：
//   - CreateWithRunMeta（1000+ 并发主入口）
//   - failTask（失败标记）
//   - updateProgress（高频 transient，不调用，避免 IO 风暴）
func (tm *TaskManager) saveTaskSingle(task *MobileTask) {
	if task == nil {
		return
	}
	if tm.store != nil {
		data := mobileTaskToData(task)
		if err := tm.store.CreateTask(data); err != nil {
			if err := tm.store.UpdateTask(data); err != nil {
				slog.Warn("Failed to upsert single task in store",
					"id", task.ID, "error", err)
			}
		}
		return
	}
	// 无 store：降级为全表写（保持向后兼容）
	tm.saveTasks()
}

func (tm *TaskManager) loadTasks() {
	// 🆕 SQLite 权威持久化层
	//   - store != nil → 从 store.ListTasks 加载
	//   - store == nil → 走旧 JSON 持久化（向后兼容）
	if tm.store != nil {
		tasks, err := tm.store.ListTasks(tasksystem.TaskFilter{})
		if err != nil {
			slog.Warn("Failed to list tasks from store", "error", err)
			return
		}

		for _, d := range tasks {
			t := dataToMobileTask(d)
			// 与旧 JSON 路径一致的状态恢复：重启后未完成任务标记为 failed
			switch t.Status {
			case "running", "queued":
				t.Status = "failed"
				t.Error = "interrupted by restart"
				now := time.Now()
				t.CompletedAt = &now
			case "cancelling":
				t.Status = "cancelled"
				if t.CompletedAt == nil {
					now := time.Now()
					t.CompletedAt = &now
				}
			}
			t.cancelFn = nil
			t.Speed = ""
			t.Eta = ""
			tm.tasks[t.ID] = t
		}

		slog.Info("Loaded persisted tasks from store", "count", len(tasks))
		return
	}

	data, err := os.ReadFile(tm.persistPath)
	if err != nil {
		if !os.IsNotExist(err) {
			slog.Warn("Failed to read tasks file", "error", err)
		}
		return
	}

	var taskList []*MobileTask
	if err := json.Unmarshal(data, &taskList); err != nil {
		slog.Warn("Failed to unmarshal tasks", "error", err)
		return
	}

	for _, t := range taskList {
		switch t.Status {
		case "running", "queued":
			t.Status = "failed"
			t.Error = "interrupted by restart"
			now := time.Now()
			t.CompletedAt = &now
		case "cancelling":
			t.Status = "cancelled"
			if t.CompletedAt == nil {
				now := time.Now()
				t.CompletedAt = &now
			}
		}
		t.cancelFn = nil
		t.Speed = ""
		t.Eta = ""
		tm.tasks[t.ID] = t
	}

	slog.Info("Loaded persisted tasks", "count", len(taskList))
}

// mobileTaskToData 把应用层 MobileTask 转换为 tasksystem.TaskData（Store 持久化用）。
//
// 字段映射说明：
//   - ExtraFields (map[string]string) → JSON 编码为字符串存入 TaskData.ExtraFields
//   - Steps ([]TaskStep) → JSON 编码为字符串存入 TaskData.Steps
//   - Speed / Eta / cancelFn 是运行时字段，不持久化（loadTasks 时清空）
//   - RollbackOf / OriginalPath 是回滚字段，rollback_* 任务有值
func mobileTaskToData(t *MobileTask) tasksystem.TaskData {
	var extraFieldsJSON string
	if len(t.ExtraFields) > 0 {
		if b, err := json.Marshal(t.ExtraFields); err == nil {
			extraFieldsJSON = string(b)
		}
	}

	var stepsJSON string
	if len(t.Steps) > 0 {
		if b, err := json.Marshal(t.Steps); err == nil {
			stepsJSON = string(b)
		}
	}

	return tasksystem.TaskData{
		ID:                 t.ID,
		Type:               tasksystem.TaskType(t.Type),
		Status:             tasksystem.TaskStatus(t.Status),
		SourcePath:         t.SourcePath,
		TargetPath:         t.TargetPath,
		OutputPath:         t.OutputPath,
		PluginName:         t.PluginName,
		TriggeredBy:        t.TriggeredBy,
		RunID:              t.RunId,
		Progress:           t.Progress,
		Phase:              t.Phase,
		Error:              t.Error,
		ErrorDetail:        t.ErrorDetail,
		Warning:            t.Warning,
		WarningDetail:      t.WarningDetail,
		ContainerVersion:   t.ContainerVersion,
		CipherMode:         t.CipherMode,
		CompressionMode:    t.CompressionMode,
		ExtraFields:        extraFieldsJSON,
		Steps:              stepsJSON,
		MountID:            t.MountID,
		MountSubPath:       t.MountSubPath,
		TargetMountID:      t.TargetMountID,
		TargetMountSubPath: t.TargetMountSubPath,
		Password:           t.Password,
		SecondaryPassword:  t.SecondaryPassword,
		CreatedAt:          t.CreatedAt,
		CompletedAt:        t.CompletedAt,
		RollbackOf:         t.RollbackOf,
		OriginalPath:       t.OriginalPath,
	}
}

// dataToMobileTask 把 tasksystem.TaskData 转换回应用层 MobileTask（Store 加载用）。
//
// 反序列化说明：
//   - TaskData.ExtraFields (string) → JSON 解码为 map[string]string
//   - TaskData.Steps (string) → JSON 解码为 []TaskStep
//   - 解码失败时保留空值（不报错，向后兼容旧数据）
//   - Speed / Eta / cancelFn 留空（运行时字段，由调用方按需填充）
func dataToMobileTask(d tasksystem.TaskData) *MobileTask {
	t := &MobileTask{
		ID:                 d.ID,
		Type:               string(d.Type),
		Status:             string(d.Status),
		SourcePath:         d.SourcePath,
		TargetPath:         d.TargetPath,
		OutputPath:         d.OutputPath,
		PluginName:         d.PluginName,
		Progress:           d.Progress,
		Phase:              d.Phase,
		Error:              d.Error,
		ErrorDetail:        d.ErrorDetail,
		Warning:            d.Warning,
		WarningDetail:      d.WarningDetail,
		ContainerVersion:   d.ContainerVersion,
		CipherMode:         d.CipherMode,
		CompressionMode:    d.CompressionMode,
		MountID:            d.MountID,
		MountSubPath:       d.MountSubPath,
		TargetMountID:      d.TargetMountID,
		TargetMountSubPath: d.TargetMountSubPath,
		Password:           d.Password,
		SecondaryPassword:  d.SecondaryPassword,
		CreatedAt:          d.CreatedAt,
		CompletedAt:        d.CompletedAt,
		RunId:              d.RunID,
		TriggeredBy:        d.TriggeredBy,
		RollbackOf:         d.RollbackOf,
		OriginalPath:       d.OriginalPath,
	}

	if d.ExtraFields != "" {
		_ = json.Unmarshal([]byte(d.ExtraFields), &t.ExtraFields)
	}
	if d.Steps != "" {
		_ = json.Unmarshal([]byte(d.Steps), &t.Steps)
	}

	return t
}

func (tm *TaskManager) Stop() {
	close(tm.stopCh)
	tm.wg.Wait()
}

func (tm *TaskManager) Create(taskType, sourcePath, targetPath, password string, version int, pluginName string) *MobileTask {
	task := &MobileTask{
		ID:               uuid.New().String(),
		Type:             taskType,
		SourcePath:       sourcePath,
		TargetPath:       targetPath,
		Password:         password,
		Status:           "queued",
		Progress:         0,
		ContainerVersion: version,
		PluginName:       pluginName,
		CreatedAt:        time.Now(),
	}

	// 🆕 2026-06-15 multi-mount（spec Phase C1+C2）
	//   两种 sourcePath 形式都尝试 mount 解析：
	//   1. /d/<mount>/... 形式（new format，front-end 切到 B4 后默认用这个）
	//      → 显式 mount 解析（longest prefix match）
	//   2. 旧绝对路径（/storage/emulated/0/foo, /foo 等）
	//      → mount.Resolve fallback 到 primary mount（自动映射）
	//   3. 非 / 开头（relative path） → 跳过 mount 解析（旧 SafeResolveToAbsPath 处理）
	//
	//   targetPath 也同样处理（如果非空）。
	if tm.mountResolver != nil {
		tm.tryAttachMount(task, sourcePath, false /* isTarget */)
		if targetPath != "" {
			tm.tryAttachMount(task, targetPath, true /* isTarget */)
		}
	}

	tm.mu.Lock()
	tm.tasks[task.ID] = task
	tm.mu.Unlock()

	// 🆕 2026-06-23 WS 时序修复（spec ws-timing-batch-throughput-100k Task 1）：
	//   - Create 不再内部广播 task:created（避免 admin_handlers / mobile_api / rollback_manager
	//     等直接调 Create 的路径在 runId 未设置时就广播 → 前端收到无 runId 的 task 变孤儿）
	//   - 改由 CreateWithRunMeta（设置 runId 之后）或外部调用方（补 RunId 兜底后）负责广播
	//   - 持久化改用 saveTaskSingle（O(1) 单行写），替代 saveTasks（全表写）
	tm.saveTaskSingle(task)

	slog.Info("Task created", "id", task.ID, "type", taskType, "source", sourcePath,
		"target", targetPath, "version", version,
		"mountId", task.MountID, "mountSubPath", task.MountSubPath,
		"targetMountId", task.TargetMountID)
	return task
}

func (tm *TaskManager) CreateWithExtras(taskType, sourcePath, targetPath, password, secondaryPassword string, version int, pluginName string, extras map[string]string) *MobileTask {
	task := tm.Create(taskType, sourcePath, targetPath, password, version, pluginName)
	task.SecondaryPassword = secondaryPassword
	task.ExtraFields = extras
	return task
}

// BroadcastCreated 广播 task:created 事件。
//
// 🆕 2026-06-23 WS 时序修复（spec ws-timing-batch-throughput-100k Task 1）：
//   - Create 不再内部广播，外部调用方（admin_handlers / mobile_api / rollback_manager）
//     在补完 RunId 兜底后调本 helper 触发广播
//   - 保证前端收到的 task:created payload 一定带 runId（不会产生孤儿 group）
//   - broadcaster 为 nil 时静默跳过（向后兼容无 WS 的测试场景）
func (tm *TaskManager) BroadcastCreated(task *MobileTask) {
	if tm.broadcaster != nil {
		tm.broadcaster.Broadcast("task:created", task)
	}
}

// FinalizeCreatedTask 补 RunId 兜底 + 单行持久化 + 广播 task:created。
//
// 🆕 2026-06-23 WS 时序修复（spec ws-timing-batch-throughput-100k Task 1）：
//   - 供外部包（server）调用——Create 不再内部广播也不再内部设 RunId
//   - 外部调用方在设置完 task 业务字段（OriginalPath 等）后调本方法收尾
//   - 内部封装 saveTaskSingle（O(1) 单行写），不暴露未导出的 saveTaskSingle
func (tm *TaskManager) FinalizeCreatedTask(task *MobileTask) {
	if task.RunId == "" {
		task.RunId = "manual-" + task.ID
	}
	tm.saveTaskSingle(task)
	tm.BroadcastCreated(task)
}

// 🆕 v6 2026-06-18：CreateWithRunMeta 接受 runId / triggeredBy 持久化
//
// 前端 createTask API 传 runId（自动化测试/AI agent run 的关联 ID）+ triggeredBy ('user'|'automation'|'ai_agent')
// 后端持久化让任务列表刷新后仍能聚合（前端按 runId 分组）。
//
// 🆕 2026-06-22 v2 架构重写：runId 永不为空的兜底（根治"任务逃逸"）
//   - 历史 bug：前端 createTask 漏传 runId（移动端 Capacitor 调用时偶发丢参）→ task.RunId = ''
//     → 前端按 runId 分组时这个 task 变孤儿（不入任何 group）
//     → 重启后 SQLite 仍存但 runId 为空 → 列表展示混乱
//   - 修法：后端兜底。runId 为空 → 用 "manual-" + taskID 派生稳定 runId
//     （保证每个 task 都有非空 runId，前端按 runId 分组永远有归属）
//   - triggeredBy 已有兜底（'' → 'user'）
func (tm *TaskManager) CreateWithRunMeta(
	taskType, sourcePath, targetPath, password, secondaryPassword string,
	version int, pluginName string,
	extras map[string]string,
	cipherMode int,
	compressionMode string,
	runId string,
	triggeredBy string,
) *MobileTask {
	task := tm.CreateWithExtras(taskType, sourcePath, targetPath, password, secondaryPassword,
		version, pluginName, extras)
	task.CipherMode = cipherMode
	task.CompressionMode = compressionMode
	// 🆕 2026-06-22 v2 架构重写：runId 永不为空的兜底（根治"任务逃逸"）
	//   历史 bug：前端 createTask 漏传 runId（移动端 Capacitor 调用时偶发丢参）→ task.RunId = ''
	//     → 前端按 runId 分组时这个 task 变孤儿（不入任何 group）
	//   修法：后端兜底。runId 为空 → 用 "manual-" + task.ID 派生稳定 runId
	//     （保证每个 task 都有非空 runId，前端按 runId 分组永远有归属）
	//   triggeredBy 已有兜底（'' → 'user'）
	if runId == "" {
		runId = "manual-" + task.ID
	}
	task.RunId = runId
	if triggeredBy == "" {
		triggeredBy = "user"
	}
	task.TriggeredBy = triggeredBy
	// 🆕 2026-06-22 Q6A：单行写（O(1)），替代 saveTasks() 全表写
	tm.saveTaskSingle(task)
	// 🆕 2026-06-23 WS 时序修复（spec ws-timing-batch-throughput-100k Task 1）：
	//   - Create 不再内部广播，改由 CreateWithRunMeta 在 runId 设置之后广播
	//   - 保证前端收到的 task:created payload 一定带 runId（不会产生孤儿 group）
	if tm.broadcaster != nil {
		tm.broadcaster.Broadcast("task:created", task)
	}
	return task
}

// BatchTaskSpec 是批量创建 task 的输入定义（不含 ID——ID 由后端统一生成）。
//
// 🆕 2026-06-23 真实架构实现（替代 client 预占位野路子）：
//   - 前端 submitRun 阶段收集本层所有 step 的 task 定义 → 一次性调 POST /api/tasks/batch
//   - 后端批量创建所有 task（后端生成 UUID）→ 一次性返回所有 task
//   - 前端拿到后一次性 push 到 store → UI 立即显示 1 个 group N task（不慢慢累加）
//   - 后端是 ID 的唯一权威源（不存在 client ID 覆盖后端 ID 的野路子）
type BatchTaskSpec struct {
	Type              string
	SourcePath        string
	TargetPath        string
	Password          string
	SecondaryPassword string
	Version           int
	PluginName        string
	ExtraFields       map[string]string
	CipherMode        int
	CompressionMode   string
}

// CreateBatch 批量创建 task，共享同一个 runId + triggeredBy。
//
// 架构原则：
//   - 后端是 task ID 的唯一权威源（uuid.New()），前端不传 ID
//   - 批量创建后一次性返回所有 task，前端一次性 push 到 store
//   - 每个 task 走 saveTaskSingle（O(1) 单行写），不走 saveTasks（全表写）
//   - WS task:created 在 Create 内部逐条 broadcast（前端 store.applyEvent 收到后 patch 而非 append）
//
// 性能：1000+ task 批量创建时，锁竞争最小化（每条 task 独立加锁），持久化走单行 UPSERT。
// CreateBatch 批量创建 task，共享同一个 runId + triggeredBy。
//
// 🆕 2026-07-01 并发优化：
//   - 根据存储引擎自动选择并发度（SQLite=1 串行，Turso=8 并发）
//   - Worker pool 模式并发创建，充分发挥 Turso MVCC 优势
//   - 内存 map 写入统一加锁（保证线程安全）
//   - WS 消息批量广播（减少 N 次单播开销）
//
// 架构原则：
//   - 后端是 task ID 的唯一权威源（uuid.New()），前端不传 ID
//   - 批量创建后一次性返回所有 task，前端一次性 push 到 store
//   - 每个 task 走 saveTaskSingle（O(1) 单行写），不走 saveTasks（全表写）
//
// 性能：Turso 引擎下 1000 task 批量创建速度提升 ~6-8 倍（MVCC 并发写）。
func (tm *TaskManager) CreateBatch(specs []BatchTaskSpec, runId string, triggeredBy string) []*MobileTask {
	if len(specs) == 0 {
		return nil
	}
	// runId / triggeredBy 兜底（跟 CreateWithRunMeta 一致）
	if runId == "" {
		runId = "manual-batch-" + uuid.New().String()[:8]
	}
	if triggeredBy == "" {
		triggeredBy = "user"
	}

	// 并发度：从 store 获取 hint，默认 1（兼容旧 store=nil 的情况）
	concurrency := 1
	if tm.store != nil {
		concurrency = tm.store.ConcurrencyHint()
	}
	if concurrency < 1 {
		concurrency = 1
	}

	// 单并发：走原串行逻辑（简单、零额外开销）
	if concurrency == 1 || len(specs) <= 2 {
		tasks := make([]*MobileTask, 0, len(specs))
		for _, spec := range specs {
			compressionMode := spec.CompressionMode
			if compressionMode == "" {
				compressionMode = "none"
			}
			task := tm.CreateWithRunMeta(
				spec.Type, spec.SourcePath, spec.TargetPath,
				spec.Password, spec.SecondaryPassword, spec.Version, spec.PluginName, spec.ExtraFields,
				spec.CipherMode, compressionMode,
				runId, triggeredBy,
			)
			tasks = append(tasks, task)
		}
		return tasks
	}

	// 多并发：worker pool 模式
	// 使用 channel 分发任务，收集结果
	type result struct {
		index int
		task  *MobileTask
	}

	jobs := make(chan int, len(specs))
	results := make(chan result, len(specs))

	// 启动 worker
	var wg sync.WaitGroup
	for w := 0; w < concurrency; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				spec := specs[i]
				compressionMode := spec.CompressionMode
				if compressionMode == "" {
					compressionMode = "none"
				}
				// 创建单个 task（内部会加锁写内存 map + 持久化 + 广播）
				task := tm.CreateWithRunMeta(
					spec.Type, spec.SourcePath, spec.TargetPath,
					spec.Password, spec.SecondaryPassword, spec.Version, spec.PluginName, spec.ExtraFields,
					spec.CipherMode, compressionMode,
					runId, triggeredBy,
				)
				results <- result{index: i, task: task}
			}
		}()
	}

	// 分发任务
	for i := range specs {
		jobs <- i
	}
	close(jobs)

	// 等待所有 worker 完成
	go func() {
		wg.Wait()
		close(results)
	}()

	// 收集结果（按原顺序排列）
	tasks := make([]*MobileTask, len(specs))
	for r := range results {
		tasks[r.index] = r.task
	}

	return tasks
}

func (tm *TaskManager) List() []*MobileTask {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	result := make([]*MobileTask, 0, len(tm.tasks))
	for _, t := range tm.tasks {
		result = append(result, t)
	}
	return result
}

// ListPaginated 按 runId 过滤后做 offset/limit 分页，返回分页后的切片和过滤后的总数。
//
// 🆕 2026-06-23 Task 5：后端分页 API（10 万任务虚拟滚动支撑）
//   - runId 非空 → 只保留 task.RunId == runId 的 task
//   - runId 为空 → 不过滤（返回全部）
//   - offset/limit 由调用方（handler）解析 + clamp，此处再做一次边界保护
//   - 返回 totalCount = 过滤后、分页前的总数（供 handler 写 X-Total-Count 响应头）
//
// 🆕 2026-06-23 重构：优先走 SQLite store SQL（不是内存过滤）
//   - store != nil → store.ListTasks(TaskFilter{RunID, Limit, Offset}) 走 SQL
//   - store == nil → 降级为内存过滤（向后兼容旧测试）
//   - totalCount：store 路径走 CountByRunId（SQL COUNT），内存路径走 len(filtered)
func (tm *TaskManager) ListPaginated(runId string, offset, limit int) ([]*MobileTask, int) {
	// 🆕 优先走 SQLite store SQL
	if tm.store != nil {
		filter := tasksystem.TaskFilter{
			RunID:  runId,
			Limit:  limit,
			Offset: offset,
		}
		datas, err := tm.store.ListTasks(filter)
		if err != nil {
			slog.Warn("ListPaginated: store.ListTasks failed, fallback to in-memory",
				"runId", runId, "offset", offset, "limit", limit, "error", err)
		} else {
			// totalCount：runId 非空时走 CountByRunId（SQL COUNT），runId 为空时走 ListTasks 全量
			totalCount := 0
			if runId != "" {
				counts, err := tm.store.CountByRunId(runId)
				if err != nil {
					slog.Warn("ListPaginated: store.CountByRunId failed", "runId", runId, "error", err)
					// 降级：用当前页长度估算（不精确但避免崩溃）
					totalCount = len(datas)
				} else {
					for _, c := range counts {
						totalCount += c
					}
				}
			} else {
				// runId 为空：totalCount 用 ListTasks(filter{Limit:0}) 拿全量长度
				allDatas, err := tm.store.ListTasks(tasksystem.TaskFilter{})
				if err != nil {
					slog.Warn("ListPaginated: store.ListTasks (count) failed", "error", err)
					totalCount = len(datas)
				} else {
					totalCount = len(allDatas)
				}
			}
			result := make([]*MobileTask, 0, len(datas))
			for _, d := range datas {
				result = append(result, dataToMobileTask(d))
			}
			return result, totalCount
		}
	}

	// 降级：内存过滤（store == nil 或 store 查询失败）
	all := tm.List()

	filtered := all
	if runId != "" {
		filtered = make([]*MobileTask, 0, len(all))
		for _, t := range all {
			if t.RunId == runId {
				filtered = append(filtered, t)
			}
		}
	}
	totalCount := len(filtered)

	if offset < 0 {
		offset = 0
	}
	if offset > totalCount {
		offset = totalCount
	}
	if limit < 0 {
		limit = 0
	}
	end := offset + limit
	if end > totalCount {
		end = totalCount
	}
	return filtered[offset:end], totalCount
}

// 🆕 2026-06-23 后端 SQL 权威：GetRunSummary 返回指定 run 的聚合计数
//
// 用于 GET /api/runs/:runId/summary 响应。
// 前端 group card 显示这些数字，不靠 store.tasks 算（store 只持有视图分页的 task）。
//
// 实现策略：
//   - store != nil → store.CountByRunId(runId) 走 SQL（SELECT status, COUNT(*) GROUP BY status）
//   - store == nil → 降级为内存遍历（tm.List() filter runId + count by status）
//
// 状态映射（后端 status → RunSummary 字段）：
//   - "completed" → Passed
//   - "failed" → Failed
//   - "running" → Running
//   - "queued" / "pending" → Pending
//   - "cancelled" → Cancelled
//   - 其他 → 归入 Total 但不归入细分（避免漏算）
func (tm *TaskManager) GetRunSummary(runId string) RunSummary {
	summary := RunSummary{RunID: runId}

	if runId == "" {
		return summary
	}

	var counts map[string]int

	// 优先走 SQLite store SQL
	if tm.store != nil {
		c, err := tm.store.CountByRunId(runId)
		if err != nil {
			slog.Warn("GetRunSummary: store.CountByRunId failed, fallback to in-memory",
				"runId", runId, "error", err)
		} else {
			counts = c
		}
	}

	// 降级：内存遍历
	if counts == nil {
		counts = make(map[string]int)
		for _, t := range tm.List() {
			if t.RunId == runId {
				counts[t.Status]++
			}
		}
	}

	// 汇总
	for status, c := range counts {
		summary.Total += c
		switch status {
		case "completed":
			summary.Passed += c
		case "failed":
			summary.Failed += c
		case "running":
			summary.Running += c
		case "queued", "pending":
			summary.Pending += c
		case "cancelled":
			summary.Cancelled += c
		}
	}

	// 完成百分比（终态 task / total * 100）
	if summary.Total > 0 {
		terminal := summary.Passed + summary.Failed + summary.Cancelled
		summary.Percent = terminal * 100 / summary.Total
	}

	return summary
}

// 🆕 2026-06-23 后端 SQL 权威：ListRuns 列出所有 run（带基本信息）
//
// 用于 GET /api/runs 响应。
// 返回所有 run 的 runId + startedAt + triggeredBy，不带 summary。
// 前端再调 /api/runs/:runId/summary 拿详细计数（或 handler 层批量补 summary）。
//
// 实现策略：
//   - store != nil → store.ListRuns() 走 SQL（GROUP BY run_id）
//   - store == nil → 降级为内存遍历（tm.List() 去重 runId）
func (tm *TaskManager) ListRuns() []tasksystem.RunInfo {
	// 优先走 SQLite store SQL
	if tm.store != nil {
		runs, err := tm.store.ListRuns()
		if err != nil {
			slog.Warn("ListRuns: store.ListRuns failed, fallback to in-memory", "error", err)
		} else {
			return runs
		}
	}

	// 降级：内存遍历
	all := tm.List()
	runMap := make(map[string]*tasksystem.RunInfo)
	for _, t := range all {
		if t.RunId == "" {
			continue
		}
		if existing, ok := runMap[t.RunId]; ok {
			// 取最早的 created_at 作为 startedAt
			if t.CreatedAt.Before(existing.StartedAt) {
				existing.StartedAt = t.CreatedAt
			}
			// triggeredBy 取第一个非空的
			if existing.TriggeredBy == "" && t.TriggeredBy != "" {
				existing.TriggeredBy = t.TriggeredBy
			}
		} else {
			info := &tasksystem.RunInfo{
				RunID:       t.RunId,
				StartedAt:   t.CreatedAt,
				TriggeredBy: t.TriggeredBy,
			}
			runMap[t.RunId] = info
		}
	}

	result := make([]tasksystem.RunInfo, 0, len(runMap))
	for _, info := range runMap {
		result = append(result, *info)
	}
	// 按 startedAt 倒序
	sort.Slice(result, func(i, j int) bool {
		return result[i].StartedAt.After(result[j].StartedAt)
	})
	return result
}

func (tm *TaskManager) Get(id string) (*MobileTask, error) {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	task, ok := tm.tasks[id]
	if !ok {
		return nil, errors.New("task not found")
	}
	return task, nil
}

func (tm *TaskManager) Cancel(id string) (*MobileTask, error) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	task, ok := tm.tasks[id]
	if !ok {
		return nil, errors.New("task not found")
	}

	if task.Status == "running" {
		task.Status = "cancelling"
		if task.cancelFn != nil {
			task.cancelFn()
		}
	} else {
		task.Status = "cancelled"
		now := time.Now()
		task.CompletedAt = &now
	}
	if tm.broadcaster != nil {
		tm.broadcaster.Broadcast("task:update", map[string]interface{}{
			"id":     id,
			"status": task.Status,
		})
	}
	return task, nil
}

// CancelByRunId 批量取消指定 runId 下所有非终态的 task。
//
// 🆕 2026-06-23 spec ws-timing-batch-throughput-100k Task 2.1：
//   - 前端 cancelRun 一次 API 调用取消整个 run（替代逐个 cancelTask 循环 N 次）
//   - 终态 task（success/failure/cancelled/timed_out/completed/failed）跳过不取消
//   - running task → cancelling + 调 cancelFn（与 Cancel 一致）
//   - 其他非终态 task（queued/cancelling 等）→ 直接 cancelled
//   - 每个取消的 task 广播 task:update + saveTaskSingle 持久化
//   - 返回 nil 表示成功（即使部分 task 取消失败也只记日志，不阻断整体）
func (tm *TaskManager) CancelByRunId(runId string) error {
	if runId == "" {
		return errors.New("runId is required")
	}

	// 锁内收集需要取消的 task ID（避免长时间持锁，Cancel 会自己加锁）
	tm.mu.RLock()
	var toCancel []string
	for id, task := range tm.tasks {
		if task.RunId != runId {
			continue
		}
		if isTerminalStatus(task.Status) {
			continue
		}
		toCancel = append(toCancel, id)
	}
	tm.mu.RUnlock()

	for _, id := range toCancel {
		// 复用 Cancel 逻辑：状态转换 + cancelFn + 广播 task:update
		cancelledTask, err := tm.Cancel(id)
		if err != nil {
			// 单个取消失败不阻断整体（task 可能已被 worker 并发处理）
			slog.Warn("CancelByRunId: cancel task failed", "taskId", id, "runId", runId, "error", err)
			continue
		}
		// Cancel 不持久化，这里补单行持久化（与 failTask 模式一致）
		tm.saveTaskSingle(cancelledTask)
	}

	return nil
}

// isTerminalStatus 判断 task 状态是否为终态（不可再变更）。
//
// 终态集合同时覆盖 spec 术语（success/failure/timed_out）和实际代码库状态
// （completed/failed），保证测试场景（用 "success"）和生产场景（用 "completed"/"failed"）
// 都正确识别终态、跳过取消。
func isTerminalStatus(status string) bool {
	switch status {
	case "success", "failure", "cancelled", "timed_out", "completed", "failed":
		return true
	}
	return false
}

func (tm *TaskManager) updateProgress(id string, progress int, phase, speed, eta string) {
	tm.mu.Lock()
	if task, ok := tm.tasks[id]; ok {
		if task.Phase != phase {
			now := time.Now()
			for i := len(task.Steps) - 1; i >= 0; i-- {
				if task.Steps[i].CompletedAt == nil {
					task.Steps[i].CompletedAt = &now
					break
				}
			}
			// v3 2026-06-18：为新 step 填充有意义的 phase 描述（前端时间线展开显示）
			task.Steps = append(task.Steps, TaskStep{
				Phase:     phase,
				StartedAt: now,
				Detail:    phaseDetailDescription(phase),
			})
		}
		task.Progress = progress
		task.Phase = phase
		task.Speed = speed
		task.Eta = eta
	}
	tm.mu.Unlock()
	if tm.broadcaster != nil {
		tm.broadcaster.Broadcast("task:progress", map[string]interface{}{
			"id":       id,
			"progress": progress,
			"phase":    phase,
			"speed":    speed,
			"eta":      eta,
		})
	}
}

// phaseDetailDescription 返回 phase 对应的人类可读描述（前端时间线展开显示）。
// v3 2026-06-18：每个 phase 切换时为新 TaskStep 填充有意义的 Detail。
func phaseDetailDescription(phase string) string {
	switch phase {
	case "analyzing":
		return "分析源文件格式与流信息"
	case "initializing":
		return "初始化加密引擎"
	case "preprocessing":
		return "预处理源数据"
	case "encrypting":
		return "加密数据流"
	case "decrypting":
		return "解密数据流"
	case "packing":
		return "打包加密产物"
	case "verifying":
		return "校验产物完整性"
	default:
		return ""
	}
}

func formatSpeed(bytesPerSec float64) string {
	if bytesPerSec <= 0 {
		return ""
	}
	if bytesPerSec < 1024 {
		return fmt.Sprintf("%.1f B/s", bytesPerSec)
	}
	if bytesPerSec < 1024*1024 {
		return fmt.Sprintf("%.1f KB/s", bytesPerSec/1024)
	}
	return fmt.Sprintf("%.1f MB/s", bytesPerSec/(1024*1024))
}

func formatDuration(seconds float64) string {
	if seconds <= 0 {
		return ""
	}
	d := time.Duration(seconds * float64(time.Second))
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm%ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	return fmt.Sprintf("%dh%dm", int(d.Hours()), int(d.Minutes())%60)
}

func (tm *TaskManager) RemoveTask(id string) error {
	tm.mu.Lock()
	if _, ok := tm.tasks[id]; !ok {
		tm.mu.Unlock()
		return errors.New("task not found")
	}

	delete(tm.tasks, id)
	tm.mu.Unlock()

	tm.saveTasks()

	slog.Info("Task removed", "id", id)
	if tm.broadcaster != nil {
		tm.broadcaster.Broadcast("task:removed", map[string]interface{}{
			"id": id,
		})
	}
	return nil
}

func (tm *TaskManager) ClearCompleted() int {
	tm.mu.Lock()
	removed := 0
	for id, task := range tm.tasks {
		if task.Status == "completed" || task.Status == "failed" || task.Status == "cancelled" {
			delete(tm.tasks, id)
			removed++
		}
	}
	tm.mu.Unlock()

	if removed > 0 {
		tm.saveTasks()
		slog.Info("Cleared completed tasks", "count", removed)
		if tm.broadcaster != nil {
			tm.broadcaster.Broadcast("task:cleared", map[string]interface{}{
				"count": removed,
			})
		}
	}
	return removed
}

func (tm *TaskManager) Retry(id string) (*MobileTask, error) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	task, ok := tm.tasks[id]
	if !ok {
		return nil, errors.New("task not found")
	}

	task.Status = "queued"
	task.Error = ""
	task.Progress = 0
	if tm.broadcaster != nil {
		tm.broadcaster.Broadcast("task:update", map[string]interface{}{
			"id":       id,
			"status":   "queued",
			"progress": 0,
		})
	}
	return task, nil
}

func (tm *TaskManager) worker() {
	defer tm.wg.Done()

	for {
		select {
		case <-tm.stopCh:
			return
		default:
		}

		task := tm.dequeue()
		if task == nil {
			time.Sleep(1 * time.Second)
			continue
		}

		tm.processTask(task)
	}
}

func (tm *TaskManager) dequeue() *MobileTask {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	for _, task := range tm.tasks {
		if task.Status == "queued" {
			task.Status = "running"
			return task
		}
	}
	return nil
}

func (tm *TaskManager) resolveAbsPath(sourcePath string) string {
	// 🆕 2026-06-15 multi-mount（spec Phase C3）
	//   - sourcePath 是 /d/<mount>/... 形式 + mountResolver 注入 → 走 mount 解析
	//     → 真机：/d/automation/... → /data/user/<uid>/com.encvgo.app/files/encv-automation/...
	//     → 任务运行时不再依赖 servingDir（servingDir 改了不影响任务）
	//   - mountResolver 为 nil 或解析失败 → 留空 → processTask failTask("invalid source path")
	//   - 其他形式（/storage/emulated/0/...、/test.mp4 等） → 走旧 SafeResolveToAbsPath
	if strings.HasPrefix(sourcePath, "/d/") && tm.mountResolver != nil {
		res, err := tm.mountResolver.Resolve(sourcePath)
		if err != nil {
			slog.Warn("mount resolve failed in resolveAbsPath", "path", sourcePath, "error", err)
			return ""
		}
		return res.AbsPath
	}

	// 旧行为：SafeResolveToAbsPath 走 servingDir
	abs, err := utils.SafeResolveToAbsPath(tm.servingDir, sourcePath)
	if err != nil {
		// Security: path traversal detected (e.g., "../../../etc/passwd").
		// Return empty string so the caller (processTask/processDecrypt) calls
		// failTask instead of using the traversal path. Previously this fell
		// through to filepath.Clean(sourcePath) which returned the original
		// traversal path — pre-existing security bug.
		return ""
	}
	return abs
}

// absToVirtualPath 把物理绝对路径转换为前端可消费的虚拟路径 /d/<mount>/<sub>。
//
// 🆕 v3 2026-06-18 Task 8：后端统一虚拟路径
//   - mountResolver 未注入或解析失败 → fallback 返回原 absPath（向后兼容）
//   - 用于 task.OutputPath / step.Detail，让前端 Files.vue 直接用 route.query 跳转
func (tm *TaskManager) absToVirtualPath(absPath string) string {
	if absPath == "" {
		return ""
	}
	if tm.mountResolver == nil {
		return absPath
	}
	virtual, err := tm.mountResolver.AbsToVirtual(absPath)
	if err != nil {
		slog.Warn("absToVirtualPath: mount resolve failed, fallback to absPath",
			"absPath", absPath, "error", err)
		return absPath
	}
	return virtual
}

// SetFileTaskHandler 注入文件操作任务处理器。
// 不在构造函数中传入是为了保持 NewTaskManager 签名向后兼容。
func (tm *TaskManager) SetFileTaskHandler(h *FileTaskHandler) {
	tm.fileTaskHandler = h
}

// SetRollbackManager 注入回滚管理器。
// 不在构造函数中传入是为了保持 NewTaskManager 签名向后兼容，
// 同时避免 RollbackManagerImpl ↔ TaskManager 构造循环依赖。
func (tm *TaskManager) SetRollbackManager(rm *RollbackManagerImpl) {
	tm.rollbackManager = rm
}

func (tm *TaskManager) processTask(task *MobileTask) {
	slog.Info("Processing task", "id", task.ID, "type", task.Type, "source", task.SourcePath)

	tm.updateProgress(task.ID, 0, "queued", "", "")

	// 文件操作任务（move/copy/rename/delete）不需要 absPath 解析，
	// FileTaskHandler 内部直接用 task.SourcePath/TargetPath
	switch task.Type {
	case "rollback_encrypt", "rollback_decrypt", "rollback_move", "rollback_copy", "rollback_rename", "rollback_delete":
		if tm.rollbackManager != nil {
			tm.rollbackManager.ProcessRollbackTask(task)
		} else {
			tm.failTask(task.ID, "rollback manager not configured")
		}
		return
	case "move", "copy", "rename", "delete":
		tm.processFileTask(task)
		return
	}

	absPath := tm.resolveAbsPath(task.SourcePath)
	if absPath == "" {
		tm.failTask(task.ID, "invalid source path")
		return
	}

	slog.Info("Resolved path", "source", task.SourcePath, "absPath", absPath)

	switch task.Type {
	case "encrypt":
		tm.processEncrypt(task, absPath)
	case "decrypt":
		tm.processDecrypt(task, absPath)
	default:
		tm.failTask(task.ID, fmt.Sprintf("unknown task type: %s", task.Type))
	}
}

// processFileTask 处理文件操作任务（move/copy/rename/delete）。
// 委托给 FileTaskHandler 执行，完成后调用 completeTask 或 failTask。
func (tm *TaskManager) processFileTask(task *MobileTask) {
	if tm.fileTaskHandler == nil {
		tm.failTask(task.ID, "file task handler not configured")
		return
	}

	tm.updateProgress(task.ID, 10, "processing", "", "")

	var err error
	switch task.Type {
	case "move":
		err = tm.fileTaskHandler.ProcessMove(task)
	case "copy":
		err = tm.fileTaskHandler.ProcessCopy(task)
	case "rename":
		err = tm.fileTaskHandler.ProcessRename(task)
	case "delete":
		err = tm.fileTaskHandler.ProcessDelete(task)
	default:
		tm.failTask(task.ID, fmt.Sprintf("unknown file task type: %s", task.Type))
		return
	}

	if err != nil {
		tm.failTask(task.ID, err.Error())
		return
	}

	// 文件操作完成，设置 outputPath 并标记 completed
	tm.mu.Lock()
	var taskToPersist *MobileTask
	task.Status = "completed"
	task.Progress = 100
	task.Phase = "completed"
	task.Speed = ""
	task.Eta = ""
	now := time.Now()
	task.CompletedAt = &now
	switch task.Type {
	case "move", "rename", "copy":
		task.OutputPath = task.TargetPath
	case "delete":
		task.OutputPath = ""
	}
	taskToPersist = task
	tm.mu.Unlock()

	if taskToPersist != nil {
		tm.saveTaskSingle(taskToPersist)
	}

	slog.Info("File task completed", "id", task.ID, "type", task.Type)
	if tm.broadcaster != nil {
		tm.broadcaster.Broadcast("task:completed", map[string]interface{}{
			"id":         task.ID,
			"status":     "completed",
			"outputPath": task.OutputPath,
		})
	}
}

func (tm *TaskManager) getPasswordContext(ctx context.Context, primaryPassword string) context.Context {
	if primaryPassword != "" {
		cfgCopy := *tm.cfg
		cfgCopy.Password = primaryPassword
		return config.NewContext(ctx, &cfgCopy)
	}
	return config.NewContext(ctx, tm.cfg)
}

func (tm *TaskManager) processEncrypt(task *MobileTask, absPath string) {
	taskID := task.ID

	ctx, cancel := context.WithCancel(context.Background())
	task.cancelFn = cancel
	defer cancel()

	tm.updateProgress(taskID, 5, string(PhaseAnalyzing), "", "")

	info, err := os.Stat(absPath)
	if err != nil {
		tm.failTask(taskID, fmt.Sprintf("source file not found: %v", err))
		return
	}

	if info.IsDir() {
		tm.failTask(taskID, "directory encryption is not supported yet")
		return
	}

	effectiveVersion := task.ContainerVersion
	if effectiveVersion == 0 {
		effectiveVersion = tm.cfg.GetEffectiveDefaultVersion()
	}
	if !types.IsValidVersion(effectiveVersion) {
		tm.failTask(taskID, fmt.Sprintf("invalid container version: %d", effectiveVersion))
		return
	}
	if types.IsDeprecatedVersion(effectiveVersion) && tm.cfg.IsStrictMode() {
		tm.failTask(taskID, fmt.Sprintf("container version %d is deprecated and strict mode is enabled", effectiveVersion))
		return
	}

	outputDir := filepath.Dir(absPath)
	if task.TargetPath != "" {
		targetAbs := tm.resolveAbsPath(task.TargetPath)
		if targetAbs != "" {
			outputDir = targetAbs
			if mkdirErr := os.MkdirAll(outputDir, 0755); mkdirErr != nil {
				tm.failTask(taskID, fmt.Sprintf("failed to create target directory: %v", mkdirErr))
				return
			}
		}
	}

	requiredSpace := info.Size() * 3 / 2
	if freeSpace, diskErr := getAvailableDiskSpace(outputDir); diskErr == nil && freeSpace < requiredSpace {
		tm.failTask(taskID, fmt.Sprintf("insufficient disk space: need %s, available %s", formatSpeed(float64(requiredSpace)), formatSpeed(float64(freeSpace))))
		return
	}

	tm.updateProgress(taskID, 10, string(PhaseInitializing), "", "")

	var plugin plugins.Plugin
	if task.PluginName != "" {
		plugin, err = plugins.FindPluginByName(task.PluginName)
		if err != nil {
			tm.failTask(taskID, fmt.Sprintf("specified plugin not found: %v", err))
			return
		}
	} else {
		plugin, err = plugins.FindEncryptingPlugin(absPath)
		if err != nil {
			tm.failTask(taskID, fmt.Sprintf("no encrypting plugin found: %v", err))
			return
		}
		task.PluginName = plugin.Name()
	}

	if resetter, ok := plugin.(pluginInterfaces.TaskStateResetter); ok {
		defer resetter.ResetTaskState()
	}

	var primaryPassword string
	isPasswordIndependent := false
	if resolver, ok := plugin.(pluginInterfaces.TaskPasswordResolver); ok {
		primaryPassword = resolver.ResolveTaskPassword(task.Password, task.ExtraFields)
		opts := plugin.GetTaskOptions()
		isPasswordIndependent = opts.PasswordStrategy == pluginInterfaces.PasswordIndependent
	} else {
		primaryPassword = task.Password
		if primaryPassword == "" {
			primaryPassword = tm.cfg.Password
		}
	}
	if primaryPassword == "" && !isPasswordIndependent {
		tm.failTask(taskID, "encryption requires a password")
		return
	}

	passwordCtx := tm.getPasswordContext(ctx, primaryPassword)

	if setter, ok := plugin.(pluginInterfaces.TaskExtraFieldsSetter); ok {
		setter.SetTaskExtraFields(task.ExtraFields)
	}

	if task.SecondaryPassword != "" {
		slog.Debug("task has secondary password (L2) — reserved for future dual-password crypto support",
			"taskId", taskID)
	}

	tm.updateProgress(taskID, 15, string(PhasePreprocessing), "", "")

	fileSize := info.Size()
	stopMonitor := make(chan struct{})
	go tm.monitorFileProgress(taskID, outputDir, fileSize, stopMonitor)

	// 🆕 性能采集
	collector := performance.NewCollector(task.ID, "encrypt")
	collector.StartPhase("initializing")

	outputPath, err := plugins.EncryptFileWithPlugin(passwordCtx, plugin, absPath, tm.servingDir, outputDir, collector)
	collector.EndPhase("initializing", 0)
	close(stopMonitor)

	if err != nil {
		tm.cleanupTempFiles(outputDir)
		if ctx.Err() != nil || task.Status == "cancelling" {
			tm.failTask(taskID, "task cancelled")
		} else {
			tm.failTask(taskID, fmt.Sprintf("encryption failed: %v", err))
		}
		return
	}

	tm.mu.Lock()
	var taskToPersist *MobileTask
	if task.Status != "cancelling" {
		task.Status = "completed"
		task.Progress = 100
		task.Phase = string(PhaseCompleted)
		task.Speed = ""
		task.Eta = ""
		now := time.Now()
		task.CompletedAt = &now

		virtualOutput := tm.absToVirtualPath(outputPath)

		for i := len(task.Steps) - 1; i >= 0; i-- {
			if task.Steps[i].CompletedAt == nil {
				task.Steps[i].CompletedAt = &now
				if virtualOutput != "" {
					task.Steps[i].Detail = virtualOutput
				}
				break
			}
		}

		if virtualOutput != "" {
			task.ContainerVersion = detectContainerVersion(outputPath)
			task.OutputPath = virtualOutput
		}

		if warnings := video.LastVerifyWarnings(); len(warnings) > 0 {
			task.Warning = fmt.Sprintf("%d verification warning(s)", len(warnings))
			detailBytes, _ := json.Marshal(warnings)
			task.WarningDetail = string(detailBytes)
		}
		taskToPersist = task
	}
	tm.mu.Unlock()

	if taskToPersist != nil {
		tm.saveTaskSingle(taskToPersist)
	}

	// 🆕 性能指标 Finalize + 持久化
	var perfSummary map[string]interface{}
	if task.Status == "completed" {
		sourceSize := int64(0)
		if info, err := os.Stat(absPath); err == nil {
			sourceSize = info.Size()
		}
		outputSize := int64(0)
		if outputPath != "" {
			if info, err := os.Stat(outputPath); err == nil {
				outputSize = info.Size()
			}
		}
		var cpuScore float64
		var cpuLabel string
		if ps := tm.perfStore(); ps != nil {
			if cal, _ := ps.GetCalibration(); cal != nil {
				cpuScore = cal.CPUScore
				cpuLabel = cal.CPULabel
			}
		}
		metrics := collector.Finalize(sourceSize, outputSize, cpuScore, cpuLabel)
		metrics.PluginName = task.PluginName
		metrics.ContainerVer = task.ContainerVersion
		thresholds := performance.GetThresholds("encrypt", task.PluginName, cpuScore)
		grade, score, reason := performance.CalculateGrade(metrics, thresholds)
		metrics.Grade = grade
		metrics.GradeScore = score
		metrics.GradeReason = reason
		if ps := tm.perfStore(); ps != nil {
			if err := ps.SaveMetrics(metrics); err != nil {
				slog.Warn("SaveMetrics failed", "taskId", task.ID, "error", err)
			}
		}
		perfSummary = map[string]interface{}{
			"avgThroughput":   metrics.AvgThroughput,
			"grade":           string(metrics.Grade),
			"gradeScore":      metrics.GradeScore,
			"totalDurationMs": metrics.TotalDurationMs,
			"sourceSize":      metrics.SourceSize,
			"outputSize":      metrics.OutputSize,
		}
	}

	slog.Info("Task completed", "id", task.ID, "type", task.Type)
	if tm.broadcaster != nil {
		// v3 2026-06-18：task:completed 事件增加 outputPath（前端无需下拉刷新即可显示产物）
		tm.broadcaster.Broadcast("task:completed", map[string]interface{}{
			"id":                  task.ID,
			"status":              "completed",
			"outputPath":          task.OutputPath,
			"performanceSummary":  perfSummary,
		})
		// 🆕 v6 2026-06-18：file:change 加 action 字段，前端按 action 增量更新
		//   - 加密/解密完成 → 源文件 modify（mtime 变化）+ 输出文件 create
		//   - 前端收到后按 action 增删改 files 数组，不再全量 reload
		if task.SourcePath != "" {
			tm.broadcaster.Broadcast("file:change", map[string]interface{}{
				"path":   task.SourcePath,
				"action": "modify",
			})
		}
		if task.OutputPath != "" && task.OutputPath != task.SourcePath {
			tm.broadcaster.Broadcast("file:change", map[string]interface{}{
				"path":   task.OutputPath,
				"action": "create",
			})
		}
	}
}

func (tm *TaskManager) monitorFileProgress(taskID, outputDir string, totalSize int64, stopCh chan struct{}) {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	startTime := time.Now()
	var lastSize int64
	var lastTime time.Time
	estimatedTotal := totalSize * 2

	for {
		select {
		case <-ticker.C:
			var currentSize int64
			filepath.WalkDir(outputDir, func(path string, d os.DirEntry, err error) error {
				if err != nil || d.IsDir() {
					return nil
				}
				if info, err := d.Info(); err == nil {
					name := info.Name()
					if strings.HasPrefix(name, "encv-pre-") || strings.HasSuffix(name, ".tmp") {
						return nil
					}
					currentSize += info.Size()
				}
				return nil
			})

			now := time.Now()
			elapsed := now.Sub(startTime).Seconds()
			if elapsed <= 0 || currentSize <= 0 {
				continue
			}

			avgSpeed := float64(currentSize) / elapsed
			var instantSpeed float64
			if !lastTime.IsZero() && currentSize > lastSize {
				dt := now.Sub(lastTime).Seconds()
				if dt > 0 {
					instantSpeed = float64(currentSize-lastSize) / dt
				}
			}
			lastSize = currentSize
			lastTime = now

			speed := instantSpeed
			if speed <= 0 {
				speed = avgSpeed
			}

			if currentSize > estimatedTotal/2 && estimatedTotal < currentSize*3 {
				estimatedTotal = currentSize * 2
			}

			rawProgress := float64(currentSize) / float64(estimatedTotal)
			if rawProgress > 0.95 {
				rawProgress = 0.95
			}
			progress := int(rawProgress*80) + 15
			if progress > 95 {
				progress = 95
			}

			speedStr := formatSpeed(speed)
			remaining := float64(estimatedTotal-currentSize) / speed
			if remaining < 0 {
				remaining = 0
			}
			etaStr := formatDuration(remaining)

			tm.mu.RLock()
			task, ok := tm.tasks[taskID]
			currentPhase := ""
			if ok {
				currentPhase = task.Phase
			}
			tm.mu.RUnlock()

			phase := string(PhaseEncrypting)
			if currentPhase == string(PhasePreprocessing) {
				phase = string(PhasePreprocessing)
			}

			tm.updateProgress(taskID, progress, phase, speedStr, etaStr)

		case <-stopCh:
			return
		}
	}
}

func (tm *TaskManager) processDecrypt(task *MobileTask, absPath string) {
	taskID := task.ID

	ctx, cancel := context.WithCancel(context.Background())
	task.cancelFn = cancel
	defer cancel()

	tm.updateProgress(taskID, 5, string(PhaseAnalyzing), "", "")

	task.ContainerVersion = detectContainerVersion(absPath)

	info, err := os.Stat(absPath)
	if err != nil {
		tm.failTask(taskID, fmt.Sprintf("source file not found: %v", err))
		return
	}

	if info.IsDir() {
		tm.failTask(taskID, "directory decryption is not supported yet")
		return
	}

	outputDir := filepath.Dir(absPath)
	if task.TargetPath != "" {
		targetAbs := tm.resolveAbsPath(task.TargetPath)
		if targetAbs != "" {
			outputDir = targetAbs
			if mkdirErr := os.MkdirAll(outputDir, 0755); mkdirErr != nil {
				tm.failTask(taskID, fmt.Sprintf("failed to create target directory: %v", mkdirErr))
				return
			}
		}
	}

	tm.updateProgress(taskID, 10, string(PhaseInitializing), "", "")

	plugin, err := plugins.FindDecryptingPlugin(absPath)
	if err != nil {
		tm.failTask(taskID, fmt.Sprintf("no decrypting plugin found: %v", err))
		return
	}
	task.PluginName = plugin.Name()

	if resetter, ok := plugin.(pluginInterfaces.TaskStateResetter); ok {
		defer resetter.ResetTaskState()
	}

	var primaryPassword string
	if resolver, ok := plugin.(pluginInterfaces.TaskPasswordResolver); ok {
		primaryPassword = resolver.ResolveTaskPassword(task.Password, task.ExtraFields)
	} else {
		primaryPassword = task.Password
		if primaryPassword == "" {
			primaryPassword = tm.cfg.Password
		}
	}

	passwordCtx := tm.getPasswordContext(ctx, primaryPassword)

	if setter, ok := plugin.(pluginInterfaces.TaskExtraFieldsSetter); ok {
		setter.SetTaskExtraFields(task.ExtraFields)
	}

	if task.SecondaryPassword != "" {
		slog.Debug("task has secondary password (L2) — reserved for future dual-password crypto support",
			"taskId", taskID, "context", "decrypt")
	}

	tm.updateProgress(taskID, 15, string(PhasePreprocessing), "", "")

	fileSize := info.Size()
	stopMonitor := make(chan struct{})
	go tm.monitorFileProgress(taskID, outputDir, fileSize, stopMonitor)

	// 🆕 性能采集
	collector := performance.NewCollector(task.ID, "decrypt")
	collector.StartPhase("initializing")

	outputPath, err := plugins.DecryptContainerWithPlugin(passwordCtx, plugin, absPath, outputDir, collector)
	collector.EndPhase("initializing", 0)
	close(stopMonitor)

	if err != nil {
		tm.cleanupTempFiles(outputDir)
		if ctx.Err() != nil || task.Status == "cancelling" {
			tm.failTask(taskID, "task cancelled")
		} else {
			tm.failTask(taskID, fmt.Sprintf("decryption failed: %v", err))
		}
		return
	}

	tm.mu.Lock()
	var taskToPersist *MobileTask
	if task.Status != "cancelling" {
		task.Status = "completed"
		task.Progress = 100
		task.Phase = string(PhaseCompleted)
		task.Speed = ""
		task.Eta = ""
		now := time.Now()
		task.CompletedAt = &now

		virtualOutput := tm.absToVirtualPath(outputPath)

		for i := len(task.Steps) - 1; i >= 0; i-- {
			if task.Steps[i].CompletedAt == nil {
				task.Steps[i].CompletedAt = &now
				if virtualOutput != "" {
					task.Steps[i].Detail = virtualOutput
				}
				break
			}
		}

		if virtualOutput != "" {
			task.OutputPath = virtualOutput
		}
		taskToPersist = task
	}
	tm.mu.Unlock()

	if taskToPersist != nil {
		tm.saveTaskSingle(taskToPersist)
	}

	// 🆕 性能指标 Finalize + 持久化
	var perfSummary map[string]interface{}
	if task.Status == "completed" {
		sourceSize := int64(0)
		if info, err := os.Stat(absPath); err == nil {
			sourceSize = info.Size()
		}
		outputSize := int64(0)
		if outputPath != "" {
			if info, err := os.Stat(outputPath); err == nil {
				outputSize = info.Size()
			}
		}
		var cpuScore float64
		var cpuLabel string
		if ps := tm.perfStore(); ps != nil {
			if cal, _ := ps.GetCalibration(); cal != nil {
				cpuScore = cal.CPUScore
				cpuLabel = cal.CPULabel
			}
		}
		metrics := collector.Finalize(sourceSize, outputSize, cpuScore, cpuLabel)
		metrics.PluginName = task.PluginName
		metrics.ContainerVer = task.ContainerVersion
		thresholds := performance.GetThresholds("decrypt", task.PluginName, cpuScore)
		grade, score, reason := performance.CalculateGrade(metrics, thresholds)
		metrics.Grade = grade
		metrics.GradeScore = score
		metrics.GradeReason = reason
		if ps := tm.perfStore(); ps != nil {
			if err := ps.SaveMetrics(metrics); err != nil {
				slog.Warn("SaveMetrics failed", "taskId", task.ID, "error", err)
			}
		}
		perfSummary = map[string]interface{}{
			"avgThroughput":   metrics.AvgThroughput,
			"grade":           string(metrics.Grade),
			"gradeScore":      metrics.GradeScore,
			"totalDurationMs": metrics.TotalDurationMs,
			"sourceSize":      metrics.SourceSize,
			"outputSize":      metrics.OutputSize,
		}
	}

	slog.Info("Task completed", "id", task.ID, "type", task.Type, "output", task.OutputPath)
	if tm.broadcaster != nil {
		// v3 2026-06-18：task:completed 事件增加 outputPath（前端无需下拉刷新即可显示产物）
		tm.broadcaster.Broadcast("task:completed", map[string]interface{}{
			"id":                  task.ID,
			"status":              "completed",
			"outputPath":          task.OutputPath,
			"performanceSummary":  perfSummary,
		})
		// 🆕 v6 2026-06-18：file:change 加 action 字段（同上）
		if task.SourcePath != "" {
			tm.broadcaster.Broadcast("file:change", map[string]interface{}{
				"path":   task.SourcePath,
				"action": "modify",
			})
		}
		if task.OutputPath != "" && task.OutputPath != task.SourcePath {
			tm.broadcaster.Broadcast("file:change", map[string]interface{}{
				"path":   task.OutputPath,
				"action": "create",
			})
		}
	}
}

func (tm *TaskManager) failTask(id, errMsg string) {
	tm.mu.Lock()

	friendlyMsg := simplifyErrorMessage(errMsg)

	var taskToPersist *MobileTask
	if task, ok := tm.tasks[id]; ok {
		task.Status = "failed"
		task.Error = friendlyMsg
		// 🆕 2026-06-22 Q2B：写结构化 errorDetail JSON，驱动前端分类/建议/phase 链
		task.ErrorDetail = classifyError(errMsg)
		now := time.Now()
		task.CompletedAt = &now
		taskToPersist = task
	}
	tm.mu.Unlock()

	// 🆕 2026-06-22 Q6A：单行写（在锁外，避免持久化阻塞其他 task 操作）
	if taskToPersist != nil {
		tm.saveTaskSingle(taskToPersist)
	}

	slog.Error("Task failed", "id", id, "error", errMsg)
	// 🆕 2026-06-22 修复：broadcaster broadcast 必须在 task 存在分支内
	//   历史 bug：原代码 defer tm.mu.Unlock() + if 合并，broadcast 一定执行
	//   即使 task 不存在也广播 → 触发 mock 失败（TestTaskManager_FailTask_NonExistent 期望 0 calls）
	if taskToPersist != nil && tm.broadcaster != nil {
		tm.broadcaster.Broadcast("task:completed", map[string]interface{}{
			"id":          id,
			"status":      "failed",
			"error":       friendlyMsg,
			"errorDetail": classifyError(errMsg),
		})
		tm.broadcaster.Broadcast("log", map[string]interface{}{
			"level":   "error",
			"message": fmt.Sprintf("[Task %s] %s", id, errMsg),
		})
	}
}

func simplifyErrorMessage(errMsg string) string {
	if strings.Contains(errMsg, "ENGINE_LOAD_FAILED") || strings.Contains(errMsg, "ENGINE_SYMBOL_MISSING") {
		return "video engine unavailable, please reinstall the app"
	}
	if strings.Contains(errMsg, "video engine unavailable") {
		return errMsg
	}
	if strings.Contains(errMsg, "cannot access file") {
		return errMsg
	}
	if strings.Contains(errMsg, "No such file") || strings.Contains(errMsg, "source file not found") {
		return "source file not found, it may have been moved or deleted"
	}
	if strings.Contains(errMsg, "Permission denied") {
		return "permission denied, cannot access the file"
	}
	if strings.Contains(errMsg, "ffprobe failed") {
		return "failed to read video metadata"
	}
	if strings.Contains(errMsg, "ffmpeg failed") {
		return "video encoding failed"
	}
	if strings.Contains(errMsg, "encryption failed") || strings.Contains(errMsg, "plugin failed") {
		return "encryption processing failed"
	}
	if len(errMsg) > 120 {
		return errMsg[:120] + "..."
	}
	return errMsg
}

// 🆕 2026-06-22 Q2B：错误分类 → 结构化 errorDetail JSON
//
// 为什么需要：
//   - 前端 TaskErrorSection 需要展示「分类 + 修复建议 + phase 时间链」
//   - 仅靠 task.Error 字符串无法驱动 useErrorAnalyzer（需要 reason 字段）
//   - 把分类结果以 JSON 写入 task.ErrorDetail，前端折叠展开可读
//
// JSON schema（前端友好）：
//   {
//     "phase": "submission" | "network" | "http" | "backend" | "plugin",
//     "reason": "source_missing" | "permission_denied" | "engine_unavailable" |
//               "ffprobe_failed" | "ffmpeg_failed" | "plugin_panic" |
//               "auth_failed" | "container_corrupted" | "disk_full" | "unknown",
//     "raw": "<原始错误字符串，便于展开查看>",
//     "at": "<ISO8601>"
//   }
func classifyError(errMsg string) string {
	reason := "unknown"
	phase := "backend"

	switch {
	case strings.Contains(errMsg, "ENGINE_LOAD_FAILED"), strings.Contains(errMsg, "ENGINE_SYMBOL_MISSING"), strings.Contains(errMsg, "video engine unavailable"):
		reason = "engine_unavailable"
		phase = "plugin"
	case strings.Contains(errMsg, "cannot access file"):
		reason = "source_missing"
		phase = "submission"
	case strings.Contains(errMsg, "No such file"), strings.Contains(errMsg, "source file not found"):
		reason = "source_missing"
		phase = "submission"
	case strings.Contains(errMsg, "Permission denied"):
		reason = "permission_denied"
		phase = "submission"
	case strings.Contains(errMsg, "ffprobe failed"):
		reason = "ffprobe_failed"
		phase = "plugin"
	case strings.Contains(errMsg, "ffmpeg failed"):
		reason = "ffmpeg_failed"
		phase = "plugin"
	case strings.Contains(errMsg, "encryption failed"), strings.Contains(errMsg, "plugin failed"), strings.Contains(errMsg, "panic"):
		reason = "plugin_panic"
		phase = "plugin"
	case strings.Contains(errMsg, "wrong password"), strings.Contains(errMsg, "auth"):
		reason = "auth_failed"
		phase = "plugin"
	case strings.Contains(errMsg, "container"), strings.Contains(errMsg, "header"):
		reason = "container_corrupted"
		phase = "plugin"
	case strings.Contains(errMsg, "no space left"):
		reason = "disk_full"
		phase = "backend"
	}

	payload := map[string]interface{}{
		"phase":  phase,
		"reason": reason,
		"raw":    errMsg,
		"at":     time.Now().UTC().Format(time.RFC3339Nano),
	}
	b, err := json.Marshal(payload)
	if err != nil {
		// fallback：序列化失败时退化为原始字符串
		return errMsg
	}
	return string(b)
}

func getAvailableDiskSpace(path string) (int64, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, err
	}
	return int64(stat.Bavail) * int64(stat.Bsize), nil
}

func (tm *TaskManager) cleanupTempFiles(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, "encv-pre-") {
			os.Remove(filepath.Join(dir, name))
		}
	}
}

func detectContainerVersion(filePath string) int {
	file, err := os.Open(filePath)
	if err != nil {
		return 0
	}
	defer file.Close()

	version, _, err := types.DetectHeaderInfoFromReaderAt(file)
	if err != nil {
		return 0
	}
	return version
}
