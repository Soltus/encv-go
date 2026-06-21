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

	// 🆕 2026-06-15 multi-mount（spec Phase C1）
	//   - nil → 旧行为，所有 sourcePath 走 SafeResolveToAbsPath(servingDir, ...)
	//   - 非 nil → sourcePath 是 /d/<mount>/... 时走 mount 解析
	//   - 用 interface 而不是 *mount.MountRegistry：避免 service → mount 反向依赖
	mountResolver MountResolver
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

func (tm *TaskManager) saveTasks() {
	tm.mu.RLock()
	taskList := make([]*MobileTask, 0, len(tm.tasks))
	for _, t := range tm.tasks {
		taskList = append(taskList, t)
	}
	tm.mu.RUnlock()

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

func (tm *TaskManager) loadTasks() {
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
	tm.saveTasks()
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

func (tm *TaskManager) processTask(task *MobileTask) {
	slog.Info("Processing task", "id", task.ID, "type", task.Type, "source", task.SourcePath)

	tm.updateProgress(task.ID, 0, "queued", "", "")

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

	outputPath, err := plugins.EncryptFileWithPlugin(passwordCtx, plugin, absPath, tm.servingDir, outputDir)
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

	slog.Info("Task completed", "id", task.ID, "type", task.Type)
	if tm.broadcaster != nil {
		// v3 2026-06-18：task:completed 事件增加 outputPath（前端无需下拉刷新即可显示产物）
		tm.broadcaster.Broadcast("task:completed", map[string]interface{}{
			"id":         task.ID,
			"status":     "completed",
			"outputPath": task.OutputPath,
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

	outputPath, err := plugins.DecryptContainerWithPlugin(passwordCtx, plugin, absPath, outputDir)
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

	slog.Info("Task completed", "id", task.ID, "type", task.Type, "output", task.OutputPath)
	if tm.broadcaster != nil {
		// v3 2026-06-18：task:completed 事件增加 outputPath（前端无需下拉刷新即可显示产物）
		tm.broadcaster.Broadcast("task:completed", map[string]interface{}{
			"id":         task.ID,
			"status":     "completed",
			"outputPath": task.OutputPath,
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
	defer tm.mu.Unlock()

	friendlyMsg := simplifyErrorMessage(errMsg)

	if task, ok := tm.tasks[id]; ok {
		task.Status = "failed"
		task.Error = friendlyMsg
		task.ErrorDetail = errMsg
		now := time.Now()
		task.CompletedAt = &now
		slog.Error("Task failed", "id", id, "error", errMsg)
		if tm.broadcaster != nil {
			tm.broadcaster.Broadcast("task:completed", map[string]interface{}{
				"id":          id,
				"status":      "failed",
				"error":       friendlyMsg,
				"errorDetail": errMsg,
			})
			tm.broadcaster.Broadcast("log", map[string]interface{}{
				"level":   "error",
				"message": fmt.Sprintf("[Task %s] %s", id, errMsg),
			})
		}
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
