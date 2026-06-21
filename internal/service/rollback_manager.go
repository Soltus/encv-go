package service

import (
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/Soltus/encv-go/pkg/tasksystem"
)

// RollbackManagerImpl 是 tasksystem.RollbackManager 的实现。
// 负责根据任务类型查找对应的 RollbackStrategy，并协调回滚流程。
//
// 设计要点：
//   - store：任务系统存储接口（获取原任务、快照、回收站条目）
//   - tm：TaskManager 引用（创建回滚任务、failTask、broadcaster）
//   - trash：TrashManagerImpl 引用（delete 回滚时从回收站还原）
//   - strategies：任务类型 → 回滚策略的映射
type RollbackManagerImpl struct {
	store      tasksystem.Store
	tm         *TaskManager
	trash      *TrashManagerImpl
	strategies map[tasksystem.TaskType]tasksystem.RollbackStrategy
}

// 编译期接口实现检查
var _ tasksystem.RollbackManager = (*RollbackManagerImpl)(nil)

// NewRollbackManager 创建 RollbackManagerImpl 并注册 6 种默认策略。
func NewRollbackManager(store tasksystem.Store, tm *TaskManager, trash *TrashManagerImpl) *RollbackManagerImpl {
	rm := &RollbackManagerImpl{
		store:      store,
		tm:         tm,
		trash:      trash,
		strategies: make(map[tasksystem.TaskType]tasksystem.RollbackStrategy),
	}
	rm.strategies[tasksystem.TaskTypeEncrypt] = &EncryptRollbackStrategy{}
	rm.strategies[tasksystem.TaskTypeDecrypt] = &DecryptRollbackStrategy{}
	rm.strategies[tasksystem.TaskTypeMove] = &MoveRollbackStrategy{}
	rm.strategies[tasksystem.TaskTypeCopy] = &CopyRollbackStrategy{}
	rm.strategies[tasksystem.TaskTypeRename] = &RenameRollbackStrategy{}
	rm.strategies[tasksystem.TaskTypeDelete] = &DeleteRollbackStrategy{store: store}
	return rm
}

// RegisterStrategy 注册自定义回滚策略。
// 应用层可在初始化后调用以覆盖默认策略或新增类型。
func (rm *RollbackManagerImpl) RegisterStrategy(taskType tasksystem.TaskType, strategy tasksystem.RollbackStrategy) {
	rm.strategies[taskType] = strategy
}

// CanRollback 判断任务是否可回滚。
// 返回 nil 表示可回滚，否则返回 error 说明原因。
//
// 校验项：
//  1. 任务存在（store.GetTask）
//  2. 任务状态为 completed
//  3. 任务类型不是 rollback_*（回滚任务不可二次回滚）
//  4. 任务未被回滚过（ListTasks{RollbackOf: taskID} 为空）
//  5. 对应策略存在
//  6. 策略特定校验通过（strategy.CanRollback）
func (rm *RollbackManagerImpl) CanRollback(taskID string) error {
	task, err := rm.store.GetTask(taskID)
	if err != nil {
		return fmt.Errorf("get task failed: %w", err)
	}

	if task.Status != tasksystem.StatusCompleted {
		return fmt.Errorf("task is not completed (status: %s)", task.Status)
	}

	if task.Type.IsRollback() {
		return fmt.Errorf("rollback tasks cannot be rolled back again")
	}

	// 检查任务未被回滚过
	existing, err := rm.store.ListTasks(tasksystem.TaskFilter{RollbackOf: taskID})
	if err != nil {
		return fmt.Errorf("check existing rollbacks failed: %w", err)
	}
	if len(existing) > 0 {
		return fmt.Errorf("task has already been rolled back")
	}

	// 检查策略存在
	strategy, ok := rm.strategies[task.Type]
	if !ok {
		return fmt.Errorf("no rollback strategy for task type: %s", task.Type)
	}

	// 获取快照（可能不存在，传空 Snapshot 兜底）
	snapshot, _ := rm.store.GetSnapshot(taskID)

	// 策略特定校验
	if err := strategy.CanRollback(task, snapshot); err != nil {
		return err
	}

	return nil
}

// Rollback 执行回滚。
// 创建一个新的回滚任务并异步执行，返回新任务 ID。
//
// 流程：
//  1. 调 CanRollback 校验
//  2. 从 store 获取原任务和快照
//  3. 调 strategy.PrepareRollback 准备回滚任务数据
//  4. 通过 tm.Create 创建回滚任务（tm 会自动 processTask）
//  5. 设置 RollbackOf / OriginalPath / TriggeredBy
func (rm *RollbackManagerImpl) Rollback(taskID string, triggeredBy string) (string, error) {
	if err := rm.CanRollback(taskID); err != nil {
		return "", err
	}

	if triggeredBy == "" {
		triggeredBy = "user"
	}

	original, err := rm.store.GetTask(taskID)
	if err != nil {
		return "", fmt.Errorf("get original task failed: %w", err)
	}

	snapshot, _ := rm.store.GetSnapshot(taskID)

	strategy, ok := rm.strategies[original.Type]
	if !ok {
		return "", fmt.Errorf("no rollback strategy for task type: %s", original.Type)
	}

	rollbackData, err := strategy.PrepareRollback(original, snapshot)
	if err != nil {
		return "", fmt.Errorf("prepare rollback failed: %w", err)
	}

	// 通过 tm.Create 创建回滚任务（tm.Create 会把任务加入队列，worker 自动 processTask）
	task := rm.tm.Create(
		string(rollbackData.Type),
		rollbackData.SourcePath,
		rollbackData.TargetPath,
		"",
		0,
		"",
	)

	// 设置回滚相关字段（加锁避免与 worker 的 dequeue 竞争）
	rm.tm.mu.Lock()
	task.RollbackOf = rollbackData.RollbackOf
	task.OriginalPath = rollbackData.OriginalPath
	task.TriggeredBy = triggeredBy
	rm.tm.mu.Unlock()
	rm.tm.saveTasks()

	slog.Info("Rollback task created",
		"rollbackTaskId", task.ID,
		"originalTaskId", taskID,
		"type", rollbackData.Type,
		"triggeredBy", triggeredBy,
	)

	return task.ID, nil
}

// ProcessRollbackTask 处理回滚任务的执行。
// 由 TaskManager.processTask 的 rollback_* case 调用。
//
// 流程：
//  1. 从 store 获取原任务（task.RollbackOf）
//  2. 从 store 获取快照
//  3. 调用对应 strategy.ExecuteRollback(task, snapshot)
//  4. 成功则标记 completed，失败则 failTask
func (rm *RollbackManagerImpl) ProcessRollbackTask(task *MobileTask) {
	rm.tm.updateProgress(task.ID, 10, "rolling back", "", "")

	// 从 store 获取原任务
	original, err := rm.store.GetTask(task.RollbackOf)
	if err != nil {
		rm.tm.failTask(task.ID, fmt.Sprintf("get original task failed: %v", err))
		return
	}

	// 获取快照（可能不存在，传空 Snapshot 兜底）
	snapshot, _ := rm.store.GetSnapshot(task.RollbackOf)

	// 获取策略（按原任务类型查找）
	strategy, ok := rm.strategies[original.Type]
	if !ok {
		rm.tm.failTask(task.ID, fmt.Sprintf("no strategy for original task type: %s", original.Type))
		return
	}

	// 转换为 TaskData 供策略使用
	taskData := mobileTaskToData(task)

	// 执行回滚
	if err := strategy.ExecuteRollback(taskData, snapshot); err != nil {
		rm.tm.failTask(task.ID, err.Error())
		return
	}

	// 标记完成
	rm.tm.mu.Lock()
	task.Status = "completed"
	task.Progress = 100
	task.Phase = "completed"
	task.Speed = ""
	task.Eta = ""
	now := time.Now()
	task.CompletedAt = &now
	rm.tm.mu.Unlock()
	rm.tm.saveTasks()

	slog.Info("Rollback task completed", "id", task.ID, "type", task.Type,
		"originalTaskId", task.RollbackOf)
	if rm.tm.broadcaster != nil {
		rm.tm.broadcaster.Broadcast("task:completed", map[string]interface{}{
			"id":     task.ID,
			"status": "completed",
		})
	}
}

// ============================================================
// Strategies
// ============================================================

// EncryptRollbackStrategy 加密任务回滚策略。
// 回滚操作：删除加密产物（original.OutputPath）。
type EncryptRollbackStrategy struct{}

var _ tasksystem.RollbackStrategy = (*EncryptRollbackStrategy)(nil)

func (s *EncryptRollbackStrategy) CanRollback(task tasksystem.TaskData, snapshot tasksystem.Snapshot) error {
	if task.OutputPath == "" {
		return fmt.Errorf("encrypt task has no output path")
	}
	return nil
}

func (s *EncryptRollbackStrategy) PrepareRollback(original tasksystem.TaskData, snapshot tasksystem.Snapshot) (tasksystem.TaskData, error) {
	return tasksystem.TaskData{
		Type:         tasksystem.TaskTypeRollbackEncrypt,
		SourcePath:   original.OutputPath,
		RollbackOf:   original.ID,
		OriginalPath: original.SourcePath,
	}, nil
}

func (s *EncryptRollbackStrategy) ExecuteRollback(task tasksystem.TaskData, snapshot tasksystem.Snapshot) error {
	if err := os.Remove(task.SourcePath); err != nil {
		if os.IsNotExist(err) {
			// 文件不存在视为已回滚（不报错）
			return nil
		}
		return fmt.Errorf("remove encrypted file failed: %w", err)
	}
	return nil
}

// DecryptRollbackStrategy 解密任务回滚策略。
// 回滚操作：删除解密产物（original.OutputPath）。
type DecryptRollbackStrategy struct{}

var _ tasksystem.RollbackStrategy = (*DecryptRollbackStrategy)(nil)

func (s *DecryptRollbackStrategy) CanRollback(task tasksystem.TaskData, snapshot tasksystem.Snapshot) error {
	if task.OutputPath == "" {
		return fmt.Errorf("decrypt task has no output path")
	}
	return nil
}

func (s *DecryptRollbackStrategy) PrepareRollback(original tasksystem.TaskData, snapshot tasksystem.Snapshot) (tasksystem.TaskData, error) {
	return tasksystem.TaskData{
		Type:         tasksystem.TaskTypeRollbackDecrypt,
		SourcePath:   original.OutputPath,
		RollbackOf:   original.ID,
		OriginalPath: original.SourcePath,
	}, nil
}

func (s *DecryptRollbackStrategy) ExecuteRollback(task tasksystem.TaskData, snapshot tasksystem.Snapshot) error {
	if err := os.Remove(task.SourcePath); err != nil {
		if os.IsNotExist(err) {
			// 文件不存在视为已回滚（不报错）
			return nil
		}
		return fmt.Errorf("remove decrypted file failed: %w", err)
	}
	return nil
}

// MoveRollbackStrategy 移动任务回滚策略。
// 回滚操作：将文件从 TargetPath 移回 OriginalPath/SourcePath。
type MoveRollbackStrategy struct{}

var _ tasksystem.RollbackStrategy = (*MoveRollbackStrategy)(nil)

func (s *MoveRollbackStrategy) CanRollback(task tasksystem.TaskData, snapshot tasksystem.Snapshot) error {
	if task.TargetPath == "" {
		return fmt.Errorf("move task has no target path")
	}
	return nil
}

func (s *MoveRollbackStrategy) PrepareRollback(original tasksystem.TaskData, snapshot tasksystem.Snapshot) (tasksystem.TaskData, error) {
	origPath := original.OriginalPath
	if origPath == "" {
		origPath = original.SourcePath
	}
	return tasksystem.TaskData{
		Type:       tasksystem.TaskTypeRollbackMove,
		SourcePath: original.TargetPath,
		TargetPath: origPath,
		RollbackOf: original.ID,
	}, nil
}

func (s *MoveRollbackStrategy) ExecuteRollback(task tasksystem.TaskData, snapshot tasksystem.Snapshot) error {
	// 边界：SourcePath 不存在 → 文件不在目标位置
	if _, err := os.Stat(task.SourcePath); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("file not found at target path: %s", task.SourcePath)
		}
		return fmt.Errorf("stat source failed: %w", err)
	}
	// 边界：TargetPath 已存在 → 原位置被占用
	if _, err := os.Stat(task.TargetPath); err == nil {
		return fmt.Errorf("original path already occupied: %s", task.TargetPath)
	}
	// 移回原位（含跨设备 fallback）
	if err := moveWithFallback(task.SourcePath, task.TargetPath); err != nil {
		return fmt.Errorf("move back failed: %w", err)
	}
	return nil
}

// CopyRollbackStrategy 复制任务回滚策略。
// 回滚操作：删除副本（original.TargetPath）。
type CopyRollbackStrategy struct{}

var _ tasksystem.RollbackStrategy = (*CopyRollbackStrategy)(nil)

func (s *CopyRollbackStrategy) CanRollback(task tasksystem.TaskData, snapshot tasksystem.Snapshot) error {
	if task.TargetPath == "" {
		return fmt.Errorf("copy task has no target path")
	}
	return nil
}

func (s *CopyRollbackStrategy) PrepareRollback(original tasksystem.TaskData, snapshot tasksystem.Snapshot) (tasksystem.TaskData, error) {
	return tasksystem.TaskData{
		Type:       tasksystem.TaskTypeRollbackCopy,
		SourcePath: original.TargetPath,
		RollbackOf: original.ID,
	}, nil
}

func (s *CopyRollbackStrategy) ExecuteRollback(task tasksystem.TaskData, snapshot tasksystem.Snapshot) error {
	if err := os.Remove(task.SourcePath); err != nil {
		if os.IsNotExist(err) {
			// 文件不存在视为已回滚（不报错）
			return nil
		}
		return fmt.Errorf("remove copy failed: %w", err)
	}
	return nil
}

// RenameRollbackStrategy 重命名任务回滚策略。
// 回滚操作：将文件从新名改回原名。
type RenameRollbackStrategy struct{}

var _ tasksystem.RollbackStrategy = (*RenameRollbackStrategy)(nil)

func (s *RenameRollbackStrategy) CanRollback(task tasksystem.TaskData, snapshot tasksystem.Snapshot) error {
	if task.TargetPath == "" {
		return fmt.Errorf("rename task has no target path")
	}
	return nil
}

func (s *RenameRollbackStrategy) PrepareRollback(original tasksystem.TaskData, snapshot tasksystem.Snapshot) (tasksystem.TaskData, error) {
	origPath := original.OriginalPath
	if origPath == "" {
		origPath = original.SourcePath
	}
	return tasksystem.TaskData{
		Type:       tasksystem.TaskTypeRollbackRename,
		SourcePath: original.TargetPath,
		TargetPath: origPath,
		RollbackOf: original.ID,
	}, nil
}

func (s *RenameRollbackStrategy) ExecuteRollback(task tasksystem.TaskData, snapshot tasksystem.Snapshot) error {
	// 边界：SourcePath 不存在 → 文件不在新名位置
	if _, err := os.Stat(task.SourcePath); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("file not found at target path: %s", task.SourcePath)
		}
		return fmt.Errorf("stat source failed: %w", err)
	}
	// 边界：TargetPath 已存在 → 原名被占用
	if _, err := os.Stat(task.TargetPath); err == nil {
		return fmt.Errorf("original path already occupied: %s", task.TargetPath)
	}
	// 改回原名（含跨设备 fallback）
	if err := moveWithFallback(task.SourcePath, task.TargetPath); err != nil {
		return fmt.Errorf("rename back failed: %w", err)
	}
	return nil
}

// DeleteRollbackStrategy 删除任务回滚策略。
// 回滚操作：从回收站还原文件到原位置。
// 需要持有 store 引用以查询和清理回收站条目。
type DeleteRollbackStrategy struct {
	store tasksystem.Store
}

var _ tasksystem.RollbackStrategy = (*DeleteRollbackStrategy)(nil)

func (s *DeleteRollbackStrategy) CanRollback(task tasksystem.TaskData, snapshot tasksystem.Snapshot) error {
	if s.store == nil {
		return fmt.Errorf("delete rollback requires store")
	}
	if _, err := s.store.GetTrashByTaskID(task.ID); err != nil {
		return fmt.Errorf("trash item not found: %w", err)
	}
	return nil
}

func (s *DeleteRollbackStrategy) PrepareRollback(original tasksystem.TaskData, snapshot tasksystem.Snapshot) (tasksystem.TaskData, error) {
	trashItem, err := s.store.GetTrashByTaskID(original.ID)
	if err != nil {
		return tasksystem.TaskData{}, fmt.Errorf("get trash item failed: %w", err)
	}
	return tasksystem.TaskData{
		Type:       tasksystem.TaskTypeRollbackDelete,
		SourcePath: trashItem.TrashPath,
		TargetPath: trashItem.OriginalPath,
		RollbackOf: original.ID,
	}, nil
}

func (s *DeleteRollbackStrategy) ExecuteRollback(task tasksystem.TaskData, snapshot tasksystem.Snapshot) error {
	// 边界：TrashPath 不存在 → 回收站条目已被清除
	if _, err := os.Stat(task.SourcePath); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("trash item no longer exists: %s", task.SourcePath)
		}
		return fmt.Errorf("stat trash path failed: %w", err)
	}
	// 边界：OriginalPath 已被占用
	if _, err := os.Stat(task.TargetPath); err == nil {
		return fmt.Errorf("original path already occupied: %s", task.TargetPath)
	}
	// 从回收站还原
	if err := moveWithFallback(task.SourcePath, task.TargetPath); err != nil {
		return fmt.Errorf("restore from trash failed: %w", err)
	}
	// 还原成功后删除回收站条目
	trashItem, err := s.store.GetTrashByTaskID(task.RollbackOf)
	if err != nil {
		slog.Warn("Failed to get trash item for cleanup after restore",
			"rollbackOf", task.RollbackOf, "error", err)
		return nil
	}
	if err := s.store.DeleteTrash(trashItem.ID); err != nil {
		slog.Warn("Failed to delete trash item after restore",
			"id", trashItem.ID, "error", err)
	}
	return nil
}

// moveWithFallback 移动文件或目录，支持跨设备 fallback。
// os.Rename 失败时（如跨设备），用 io.Copy + os.Remove 兜底。
// 复用 trash_manager.go 中的 copyDirRecursive / copyFileAndRemove。
func moveWithFallback(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	info, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("stat source failed: %w", err)
	}
	if info.IsDir() {
		if err := copyDirRecursive(src, dst, info.Mode()); err != nil {
			return fmt.Errorf("copy directory failed: %w", err)
		}
		if err := os.RemoveAll(src); err != nil {
			return fmt.Errorf("remove source directory after copy failed: %w", err)
		}
		return nil
	}
	return copyFileAndRemove(src, dst, info.Mode())
}
