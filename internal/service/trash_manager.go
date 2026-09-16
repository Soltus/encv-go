package service

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/Soltus/encv-go/internal/config"
	"github.com/Soltus/encv-go/pkg/tasksystem"
	"github.com/google/uuid"
)

// TrashManagerImpl 是 tasksystem.TrashManager 的实现。
// 负责将文件/目录移到回收站、还原、列出、清空等操作。
//
// 设计要点：
//   - trashDir = <servingDir>/.trash/
//   - 文件名加 UnixNano 前缀避免同名冲突
//   - 跨设备移动 fallback：os.Rename 失败时用 io.Copy + os.Remove
//   - broadcaster 可为 nil（nil 检查）
type TrashManagerImpl struct {
	store       tasksystem.Store
	trashDir    string
	broadcaster Broadcaster
	mu          sync.Mutex
}

// 编译期接口实现检查
var _ tasksystem.TrashManager = (*TrashManagerImpl)(nil)

// NewTrashManager 创建 TrashManagerImpl。
//   - servingDir: 服务根目录，回收站位于 <servingDir>/.trash/
//   - store: 任务系统存储接口（含 trash 表 CRUD）
//   - broadcaster: 文件变更事件广播器，可为 nil
func NewTrashManager(servingDir string, store tasksystem.Store, broadcaster Broadcaster) *TrashManagerImpl {
	// 续43 脉络：回收站是应用管理状态，落数据目录（config.AppDataDir），绝不进 servingDir（用户媒体根）。
	trashDir := filepath.Join(config.AppDataDir("tasks"), ".trash")
	if err := os.MkdirAll(trashDir, 0755); err != nil {
		slog.Warn("Failed to create trash directory", "path", trashDir, "error", err)
	}
	return &TrashManagerImpl{
		store:       store,
		trashDir:    trashDir,
		broadcaster: broadcaster,
	}
}

// MoveToTrash 将文件/目录移到回收站。
// 返回创建的 TrashItem（含 trashPath）。
func (tm *TrashManagerImpl) MoveToTrash(originalPath string, taskID string) (tasksystem.TrashItem, error) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	trashID := uuid.NewString()
	trashPath := filepath.Join(tm.trashDir, fmt.Sprintf("%d_%s", time.Now().UnixNano(), filepath.Base(originalPath)))

	info, err := os.Stat(originalPath)
	if err != nil {
		return tasksystem.TrashItem{}, fmt.Errorf("stat source failed: %w", err)
	}

	if err := tm.movePath(originalPath, trashPath); err != nil {
		return tasksystem.TrashItem{}, fmt.Errorf("move to trash failed: %w", err)
	}

	// 记录原属性（mtime/mode）到 metadata JSON
	metadata := map[string]interface{}{
		"mtime": info.ModTime().Format(time.RFC3339Nano),
		"mode":  info.Mode().String(),
	}
	metadataBytes, _ := json.Marshal(metadata)

	item := tasksystem.TrashItem{
		ID:           trashID,
		OriginalPath: originalPath,
		TrashPath:    trashPath,
		IsDirectory:  info.IsDir(),
		Size:         info.Size(),
		DeletedAt:    time.Now(),
		TaskID:       taskID,
		Metadata:     string(metadataBytes),
	}

	if err := tm.store.CreateTrash(item); err != nil {
		// 文件已移到回收站但 DB 写入失败 → 尝试回滚（移回原位）
		slog.Error("Failed to persist trash item, rolling back", "id", trashID, "error", err)
		if rbErr := tm.movePath(trashPath, originalPath); rbErr != nil {
			slog.Error("Rollback failed, file stranded in trash", "trashPath", trashPath, "error", rbErr)
		}
		return tasksystem.TrashItem{}, fmt.Errorf("persist trash item failed: %w", err)
	}

	if tm.broadcaster != nil {
		tm.broadcaster.Broadcast("file:change", map[string]interface{}{
			"path":   originalPath,
			"action": "delete",
		})
	}

	return item, nil
}

// Restore 从回收站还原文件到指定路径。
// destPath 为空则还原到 originalPath。
// 返回 taskID（当前实现简化为空字符串，实际还原任务由调用方创建）。
func (tm *TrashManagerImpl) Restore(trashID string, destPath string, triggeredBy string) (string, error) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	item, err := tm.store.GetTrash(trashID)
	if err != nil {
		return "", fmt.Errorf("get trash item failed: %w", err)
	}

	if destPath == "" {
		destPath = item.OriginalPath
	}

	if err := tm.movePath(item.TrashPath, destPath); err != nil {
		return "", fmt.Errorf("restore move failed: %w", err)
	}

	// 更新 trash 条目记录 restore_task_id（当前简化为空字符串）
	item.RestoreTaskID = ""
	if err := tm.store.UpdateTrash(item); err != nil {
		slog.Warn("Failed to update trash item with restore_task_id", "id", trashID, "error", err)
	}

	// 还原成功后从 trash 表删除该条目（已不在回收站）
	if err := tm.store.DeleteTrash(trashID); err != nil {
		slog.Warn("Failed to delete trash item after restore", "id", trashID, "error", err)
	}

	if tm.broadcaster != nil {
		tm.broadcaster.Broadcast("file:change", map[string]interface{}{
			"path":   destPath,
			"action": "create",
		})
	}

	return "", nil
}

// List 列出回收站所有条目。
func (tm *TrashManagerImpl) List() ([]tasksystem.TrashItem, error) {
	return tm.store.ListTrash()
}

// Purge 永久删除回收站指定条目（os.Remove trashPath）。
// 不广播 file:change（回收站内部操作）。
func (tm *TrashManagerImpl) Purge(trashID string) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	item, err := tm.store.GetTrash(trashID)
	if err != nil {
		return fmt.Errorf("get trash item failed: %w", err)
	}

	if item.IsDirectory {
		if err := os.RemoveAll(item.TrashPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove trash directory failed: %w", err)
		}
	} else {
		if err := os.Remove(item.TrashPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove trash file failed: %w", err)
		}
	}

	if err := tm.store.DeleteTrash(trashID); err != nil {
		return fmt.Errorf("delete trash item failed: %w", err)
	}

	return nil
}

// Empty 清空回收站（删除所有条目及其文件）。
// 不广播 file:change。
func (tm *TrashManagerImpl) Empty() error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	items, err := tm.store.ListTrash()
	if err != nil {
		return fmt.Errorf("list trash failed: %w", err)
	}

	var firstErr error
	errCount := 0
	for _, item := range items {
		if item.IsDirectory {
			if err := os.RemoveAll(item.TrashPath); err != nil && !os.IsNotExist(err) {
				slog.Warn("Failed to remove trash directory during empty", "path", item.TrashPath, "error", err)
				if firstErr == nil {
					firstErr = err
				}
				errCount++
				continue
			}
		} else {
			if err := os.Remove(item.TrashPath); err != nil && !os.IsNotExist(err) {
				slog.Warn("Failed to remove trash file during empty", "path", item.TrashPath, "error", err)
				if firstErr == nil {
					firstErr = err
				}
				errCount++
				continue
			}
		}

		if err := tm.store.DeleteTrash(item.ID); err != nil {
			slog.Warn("Failed to delete trash item during empty", "id", item.ID, "error", err)
			if firstErr == nil {
				firstErr = err
			}
			errCount++
		}
	}

	if errCount > 0 {
		return fmt.Errorf("empty trash completed with %d error(s), first: %w", errCount, firstErr)
	}
	return nil
}

// movePath 移动文件或目录，支持跨设备 fallback。
// os.Rename 失败时（如跨设备），用 io.Copy + os.Remove 兜底。
// 目录则递归复制后删除源目录。
func (tm *TrashManagerImpl) movePath(src, dst string) error {
	// 先尝试 os.Rename（同设备原子操作）
	if err := os.Rename(src, dst); err == nil {
		return nil
	}

	// fallback：判断源是文件还是目录
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

// copyFileAndRemove 复制单个文件并删除源文件（跨设备移动 fallback）。
func copyFileAndRemove(src, dst string, mode os.FileMode) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open source failed: %w", err)
	}
	defer srcFile.Close()

	dstFile, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return fmt.Errorf("create destination failed: %w", err)
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		os.Remove(dst)
		return fmt.Errorf("copy data failed: %w", err)
	}

	if err := os.Remove(src); err != nil {
		return fmt.Errorf("remove source after copy failed: %w", err)
	}
	return nil
}

// copyDirRecursive 递归复制目录（不删除源），源目录由调用方删除。
func copyDirRecursive(src, dst string, mode os.FileMode) error {
	if err := os.MkdirAll(dst, mode); err != nil {
		return fmt.Errorf("create destination directory failed: %w", err)
	}

	entries, err := os.ReadDir(src)
	if err != nil {
		return fmt.Errorf("read source directory failed: %w", err)
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())
		entryInfo, err := entry.Info()
		if err != nil {
			return fmt.Errorf("get entry info failed for %s: %w", entry.Name(), err)
		}
		if entry.IsDir() {
			if err := copyDirRecursive(srcPath, dstPath, entryInfo.Mode()); err != nil {
				return err
			}
		} else {
			if err := copyFileKeep(srcPath, dstPath, entryInfo.Mode()); err != nil {
				return err
			}
		}
	}
	return nil
}

// copyFileKeep 复制单个文件（不删除源），用于目录递归复制。
func copyFileKeep(src, dst string, mode os.FileMode) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open source failed: %w", err)
	}
	defer srcFile.Close()

	dstFile, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return fmt.Errorf("create destination failed: %w", err)
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		os.Remove(dst)
		return fmt.Errorf("copy data failed: %w", err)
	}
	return nil
}
