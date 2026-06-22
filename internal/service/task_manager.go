package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
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

	tm.saveTasks()

	slog.Info("Task created", "id", task.ID, "type", taskType, "source", sourcePath,
		"target", targetPath, "version", version,
		"mountId", task.MountID, "mountSubPath", task.MountSubPath,
		"targetMountId", task.TargetMountID)
	if tm.broadcaster != nil {
		tm.broadcaster.Broadcast("task:created", task)
	}
	return task
}

func (tm *TaskManager) CreateWithExtras(taskType, sourcePath, targetPath, password, secondaryPassword string, version int, pluginName string, extras map[string]string) *MobileTask {
	task := tm.Create(taskType, sourcePath, targetPath, password, version, pluginName)
	task.SecondaryPassword = secondaryPassword
	task.ExtraFields = extras
	return task
}

// 🆕 v6 2026-06-18：CreateWithRunMeta 接受 runId / triggeredBy 持久化
//
// 前端 createTask API 传 runId（自动化测试/AI agent run 的关联 ID）+ triggeredBy ('user'|'automation'|'ai_agent')
// 后端持久化让任务列表刷新后仍能聚合（前端按 runId 分组）。
//
// 设计取舍：
//   - 不修改 CreateWithExtras 签名（保持向后兼容）
//   - 新增独立方法，由 handleCreateTaskGin 按需调用
//   - runId="" 时视为手动创建（triggeredBy 默认 'user'）
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
	task.RunId = runId
	if triggeredBy == "" {
		triggeredBy = "user"
	}
	task.TriggeredBy = triggeredBy
	// 🆕 2026-06-22 Q6A：单行写（O(1)），替代 saveTasks() 全表写
	tm.saveTaskSingle(task)
	return task
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
	tm.mu.Unlock()

	tm.saveTasks()

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
	if task.Status != "cancelling" {
		task.Status = "completed"
		task.Progress = 100
		task.Phase = string(PhaseCompleted)
		task.Speed = ""
		task.Eta = ""
		now := time.Now()
		task.CompletedAt = &now

		// 🆕 v3 2026-06-18 Task 8：outputPath 转虚拟路径再存储
		//   - EncryptFileWithPlugin 返回物理绝对路径
		//   - 前端 Files.vue 需要虚拟路径 /d/<mount>/... 才能定位
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
	}
	tm.mu.Unlock()

	tm.saveTasks()

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
	if task.Status != "cancelling" {
		task.Status = "completed"
		task.Progress = 100
		task.Phase = string(PhaseCompleted)
		task.Speed = ""
		task.Eta = ""
		now := time.Now()
		task.CompletedAt = &now

		// 🆕 v3 2026-06-18 Task 8：outputPath 转虚拟路径再存储（与 processEncrypt 一致）
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
	}
	tm.mu.Unlock()

	tm.saveTasks()

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
