package service

import (
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/Soltus/encv-go/pkg/tasksystem"
)

// trashMover is the minimal interface FileTaskHandler needs from a trash manager.
//
// Forward-declared here so this file compiles whether or not
// trash_manager.go (parallel implementation) exists yet. When
// *TrashManagerImpl is created with a MoveToTrash method matching
// tasksystem.TrashManager, it will satisfy this interface implicitly
// and can be passed to NewFileTaskHandler.
type trashMover interface {
	MoveToTrash(originalPath string, taskID string) (tasksystem.TrashItem, error)
}

// FileTaskHandler handles file-operation tasks: move/copy/rename/delete.
//
// It is invoked by TaskManager.processTask for the corresponding task types.
// File operations do not involve encryption and do not require a password.
//
// Fields:
//   - trash:       required for delete tasks; may be nil otherwise.
//   - broadcaster: used to broadcast file:change events; may be nil.
type FileTaskHandler struct {
	trash       trashMover
	broadcaster Broadcaster
}

// NewFileTaskHandler constructs a FileTaskHandler.
//
// trash may be nil if delete tasks are not used. broadcaster may be nil
// if file:change events are not needed.
func NewFileTaskHandler(trash trashMover, broadcaster Broadcaster) *FileTaskHandler {
	return &FileTaskHandler{
		trash:       trash,
		broadcaster: broadcaster,
	}
}

// ProcessMove handles a move task.
//
// task.SourcePath is the source path; task.TargetPath is the destination.
// It first tries os.Rename (atomic when source and destination share the
// same filesystem). On failure it falls back to io.Copy + os.Remove to
// support cross-filesystem moves (mirroring admin_handlers.go
// handleFileMoveGin).
//
// On success it broadcasts:
//   - file:change { path: SourcePath, action: "delete" }
//   - file:change { path: TargetPath, action: "create" }
//
// On error the returned error is surfaced by TaskManager via failTask.
func (h *FileTaskHandler) ProcessMove(task *MobileTask) error {
	src := task.SourcePath
	dst := task.TargetPath
	if src == "" || dst == "" {
		return fmt.Errorf("move requires both sourcePath and targetPath")
	}

	if err := h.prepareDestination(task, "move"); err != nil {
		return err
	}

	if err := os.Rename(src, dst); err != nil {
		slog.Warn("os.Rename failed in ProcessMove, falling back to copy+remove",
			"src", src, "dst", dst, "error", err)
		if err := h.copyAndRemove(src, dst); err != nil {
			return fmt.Errorf("move failed: %w", err)
		}
	}

	h.broadcastFileChange(src, "delete")
	h.broadcastFileChange(dst, "create")
	return nil
}

// ProcessCopy handles a copy task.
//
// task.SourcePath is the source path; task.TargetPath is the destination.
// The source file is left untouched. On success it broadcasts:
//   - file:change { path: TargetPath, action: "create" }
//
// No "delete" event is emitted because the source is unchanged.
func (h *FileTaskHandler) ProcessCopy(task *MobileTask) error {
	src := task.SourcePath
	dst := task.TargetPath
	if src == "" || dst == "" {
		return fmt.Errorf("copy requires both sourcePath and targetPath")
	}

	if err := h.prepareDestination(task, "copy"); err != nil {
		return err
	}

	if err := h.copyFile(src, dst); err != nil {
		return fmt.Errorf("copy failed: %w", err)
	}

	h.broadcastFileChange(dst, "create")
	return nil
}

// ProcessRename handles a rename task for plain files.
//
// task.SourcePath is the old path; task.TargetPath is the new path.
// task.OriginalPath (when populated by the caller) holds the old path for
// rollback purposes and equals SourcePath.
//
// Only plain files are supported here. Encrypted-container rename requires
// MobileService.RenameFile (header rewrite), which would introduce a circular
// dependency; encrypted containers continue to go through the existing
// PATCH /api/file/rename endpoint and are not routed through the task system.
//
// On success it broadcasts:
//   - file:change { path: SourcePath, action: "delete" }
//   - file:change { path: TargetPath, action: "create" }
func (h *FileTaskHandler) ProcessRename(task *MobileTask) error {
	src := task.SourcePath
	dst := task.TargetPath
	if src == "" || dst == "" {
		return fmt.Errorf("rename requires both sourcePath and targetPath")
	}

	if err := h.prepareDestination(task, "rename"); err != nil {
		return err
	}

	if err := os.Rename(src, dst); err != nil {
		return fmt.Errorf("rename failed: %w", err)
	}

	h.broadcastFileChange(src, "delete")
	h.broadcastFileChange(dst, "create")
	return nil
}

// ProcessDelete handles a delete task.
//
// task.SourcePath is the path to delete. The file is moved to the trash via
// TrashManager, which broadcasts file:change { action: "delete" } internally.
//
// If trash is nil, an error is returned: delete tasks require a TrashManager.
func (h *FileTaskHandler) ProcessDelete(task *MobileTask) error {
	if h.trash == nil {
		return fmt.Errorf("delete task requires TrashManager (trash is nil)")
	}
	src := task.SourcePath
	if src == "" {
		return fmt.Errorf("delete requires sourcePath")
	}
	if _, err := h.trash.MoveToTrash(src, task.ID); err != nil {
		return fmt.Errorf("move to trash failed: %w", err)
	}
	// TrashManager is responsible for broadcasting file:change { action: "delete" }.
	return nil
}

// prepareDestination 是 copy / move / rename 的统一目标处理：
//
//   - 目标不存在 → 直接放行；
//   - 目标存在且 task.Overwrite == false → 拒绝（默认语义，绝不静默覆盖）；
//   - 目标存在且 task.Overwrite == true  → **先把目标移入回收站**再放行。
//     这是「可恢复覆盖」的关键：被覆盖的文件在 .trash 里有条目，
//     且该条目的 task_id 指向本任务，回滚时可据此还原（见 rollback_manager.go）。
//
// 没有 TrashManager 时拒绝覆盖——宁可失败，也不能做不可恢复的覆盖。
func (h *FileTaskHandler) prepareDestination(task *MobileTask, op string) error {
	src, dst := task.SourcePath, task.TargetPath

	// src == dst 的处理按 op 区分：
	//   - move / rename 到自身路径是 no-op，放行；
	//   - copy 到自身必须拒绝：copyFile 用 os.Create 会把目标截断成 0 字节（自我销毁）。
	if src == dst {
		if op == "copy" {
			return fmt.Errorf("copy failed: source and destination are the same file: %s", src)
		}
		return nil
	}

	_, statErr := os.Stat(dst)
	switch {
	case statErr == nil:
		// 目标已存在
		if !task.Overwrite {
			slog.Warn("file task rejected: destination already exists",
				"op", op, "src", src, "dst", dst)
			return fmt.Errorf("%s failed: destination already exists: %s", op, dst)
		}
		if h.trash == nil {
			return fmt.Errorf("%s failed: overwrite requires TrashManager (refusing to destroy %s)", op, dst)
		}
		item, err := h.trash.MoveToTrash(dst, task.ID)
		if err != nil {
			return fmt.Errorf("%s failed: failed to move existing destination to trash: %w", op, err)
		}
		slog.Info("overwrite: existing destination moved to trash",
			"op", op, "dst", dst, "trashId", item.ID)
		return nil
	case os.IsNotExist(statErr):
		return nil
	default:
		return fmt.Errorf("%s failed: stat destination failed: %w", op, statErr)
	}
}

// 设计备注（为什么默认拒绝、覆盖必须先入回收站）：
//  1. 历史上 handler 里有 os.Stat(dest) → 409 的冲突校验，改成异步任务时被一并删掉，
//     而 copyFile 用 os.Create（截断）、os.Rename 在 POSIX 上会静默替换已存在目标
//     → 变成「静默覆盖」的回归；
//  2. 回滚策略假定目标原本不存在：CopyRollbackStrategy 只做 os.Remove(target)，
//     一旦覆盖过，回滚就会把用户原有的文件删掉（比覆盖更糟）；
//  3. 校验放在任务真正执行前是唯一可靠的防线——HTTP 层的 stat 与任务执行之间
//     存在 TOCTOU 窗口，且无法覆盖非 HTTP 入口。

// copyFile copies src to dst using io.Copy (mirroring admin_handlers.go
// handleFileCopyGin). On copy failure the partial destination is removed.
func (h *FileTaskHandler) copyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		os.Remove(dst)
		return err
	}
	return nil
}

// copyAndRemove copies src to dst then removes src. Used as the cross-device
// fallback for ProcessMove (mirroring admin_handlers.go handleFileMoveGin).
func (h *FileTaskHandler) copyAndRemove(src, dst string) error {
	if err := h.copyFile(src, dst); err != nil {
		return err
	}
	if err := os.Remove(src); err != nil {
		slog.Error("failed to remove source file after copy in move fallback",
			"src", src, "error", err)
		return err
	}
	return nil
}

// broadcastFileChange emits a file:change event when a broadcaster is configured.
// No-op when broadcaster is nil.
func (h *FileTaskHandler) broadcastFileChange(path, action string) {
	if h.broadcaster == nil {
		return
	}
	h.broadcaster.Broadcast("file:change", map[string]string{
		"path":   path,
		"action": action,
	})
}
