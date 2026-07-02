package service

// task_manager_worker.go — 拆分自 task_manager.go

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Soltus/encv-go/internal/config"
	"github.com/Soltus/encv-go/internal/utils"
	"github.com/Soltus/encv-go/internal/v2/plugins"
	pluginInterfaces "github.com/Soltus/encv-go/internal/v2/plugins/interfaces"
	"github.com/Soltus/encv-go/internal/v2/plugins/video"
	"github.com/Soltus/encv-go/internal/v2/types"
	"github.com/Soltus/encv-go/pkg/tasksystem/performance"
)

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

func (tm *TaskManager) SetFileTaskHandler(h *FileTaskHandler) {
	tm.fileTaskHandler = h
}

func (tm *TaskManager) SetRollbackManager(rm *RollbackManagerImpl) {
	tm.rollbackManager = rm
}

// SetFTSRebuilder 注入 FTS 索引重建器。
//
// 2026-07-03 新增（spec fts-rebuild-task）
//   - 调用方：server 层 NewServer 后注入实现
//   - nil 时 rebuild_fts_index 任务会 failTask("fts rebuilder not configured")
//   - 非 nil 时 processTask switch 的 rebuild_fts_index case 调用 RebuildWithProgress
func (tm *TaskManager) SetFTSRebuilder(r FTSRebuilder) {
	tm.ftsRebuilder = r
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
	case "rebuild_fts_index":
		// 🆕 2026-07-03：FTS 索引重建任务
		//   - 无 sourcePath（不是文件操作），必须放第一层 switch 避免 resolveAbsPath 拦截
		//   - 通过 FTSRebuilder interface 注入（避免 service → server 反向依赖）
		tm.processRebuildFTSIndex(task)
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
			"id":                 task.ID,
			"status":             "completed",
			"outputPath":         task.OutputPath,
			"performanceSummary": perfSummary,
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
			"id":                 task.ID,
			"status":             "completed",
			"outputPath":         task.OutputPath,
			"performanceSummary": perfSummary,
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
