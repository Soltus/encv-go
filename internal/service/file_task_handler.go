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
