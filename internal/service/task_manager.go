package service

// task_manager.go — 拆分后保留

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/Soltus/encv-go/internal/config"
	"github.com/Soltus/encv-go/internal/v2/types"
	"github.com/Soltus/encv-go/pkg/tasksystem"
	"github.com/Soltus/encv-go/pkg/tasksystem/performance"
	"github.com/Soltus/encv-go/pkg/tasksystem/store/sqlite"
)

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
	RunId       string `json:"runId,omitempty"`
	TriggeredBy string `json:"triggeredBy,omitempty"`

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
	MountID            string `json:"mountId,omitempty"`
	MountSubPath       string `json:"mountSubPath,omitempty"`
	TargetMountID      string `json:"targetMountId,omitempty"`
	TargetMountSubPath string `json:"targetMountSubPath,omitempty"`

	CreatedAt   time.Time  `json:"createdAt"`
	CompletedAt *time.Time `json:"completedAt,omitempty"`

	// 🆕 2026-07-02：向量搜索相关度分数（0-1，越大越相似）。
	// 仅在向量搜索（/api/search/tasks）返回时填充，普通列表查询不填。
	// 前端可据此显示相关度徽章（与文件搜索复用 RelevanceBadge 组件）。
	SearchScore float64 `json:"searchScore,omitempty"`

	cancelFn context.CancelFunc
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

	// 🆕 进度节流持久化
	//   - dirtyTasks: 进度有变化需要持久化的任务 ID 集合
	//   - persistTicker: 定期批量持久化进度的 ticker
	dirtyTasks  map[string]struct{}
	dirtyMu     sync.Mutex
	persistDone chan struct{}

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

func (tm *TaskManager) perfStore() *sqlite.Store {
	if tm.store == nil {
		return nil
	}
	if s, ok := tm.store.(*sqlite.Store); ok {
		return s
	}
	return nil
}

func (tm *TaskManager) GetPerfStore() *sqlite.Store {
	return tm.perfStore()
}

func (tm *TaskManager) GetStoreEngine() string {
	if tm.store == nil {
		return "unknown"
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

func (tm *TaskManager) GetStoreConcurrency() int {
	if tm.store == nil {
		return 1
	}
	return tm.store.ConcurrencyHint()
}

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

func (tm *TaskManager) GetStore() tasksystem.Store {
	return tm.store
}

func (tm *TaskManager) ExportDatabase() (*tasksystem.DatabaseDump, error) {
	if tm.store == nil {
		return nil, fmt.Errorf("store not configured (memory mode)")
	}
	return tm.store.ExportAll()
}

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

type MountResolver interface {
	Resolve(virtualPath string) (*MountResolveResult, error)
	// 🆕 Task 8：absPath → virtualPath（找不到匹配 mount 返回 error）
	AbsToVirtual(absPath string) (string, error)
}

type MountResolveResult struct {
	MountID string // mount 唯一 ID（持久化用）
	AbsPath string // 解析后的绝对路径（运行时用）
	SubPath string // mount 内部相对路径（持久化用）
}

func (tm *TaskManager) SetMountResolver(r MountResolver) {
	tm.mountResolver = r
}

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
		dirtyTasks:  make(map[string]struct{}),
	}

	tm.loadTasks()

	workerCount := 1
	tm.wg.Add(workerCount)
	for i := 0; i < workerCount; i++ {
		go tm.worker()
	}
	return tm
}

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
		dirtyTasks:  make(map[string]struct{}),
		persistDone: make(chan struct{}),
	}

	tm.loadTasks()

	workerCount := 1
	if store != nil {
		workerCount = store.ConcurrencyHint()
	}
	if workerCount < 1 {
		workerCount = 1
	}
	tm.wg.Add(workerCount)
	for i := 0; i < workerCount; i++ {
		go tm.worker()
	}
	slog.Info("TaskManager workers started", "count", workerCount)

	if store != nil {
		tm.wg.Add(1)
		go tm.progressPersister()
		slog.Info("Progress persister started")
	}

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

func (tm *TaskManager) saveTaskSingle(task *MobileTask) {
	if task == nil {
		return
	}
	if tm.store != nil {
		tm.persistTaskWithRetry(task)
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
			switch t.Status {
			case "running", "cancelling":
				t.Status = "failed"
				t.Error = "interrupted by restart"
				now := time.Now()
				t.CompletedAt = &now
			case "queued":
				t.Status = "paused"
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
		case "running", "cancelling":
			t.Status = "failed"
			t.Error = "interrupted by restart"
			now := time.Now()
			t.CompletedAt = &now
		case "queued":
			t.Status = "paused"
		}
		t.cancelFn = nil
		t.Speed = ""
		t.Eta = ""
		tm.tasks[t.ID] = t
	}

	slog.Info("Loaded persisted tasks", "count", len(taskList))
}

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

func isTerminalStatus(status string) bool {
	switch status {
	case "success", "failure", "cancelled", "timed_out", "completed", "failed":
		return true
	}
	return false
}

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
