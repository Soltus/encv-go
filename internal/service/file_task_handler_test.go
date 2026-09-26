package service

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/Soltus/encv-go/pkg/tasksystem"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestFileTaskHandler() *FileTaskHandler {
	return NewFileTaskHandler(nil, nil)
}

// ─────────── 目标已存在：必须拒绝，不能覆盖 ───────────

// TestFileTaskHandler_RejectsExistingDestination 是防回归屏障：
// 历史上 copy/move/rename 都有冲突校验，改成异步任务时被误删，变成静默覆盖。
// 覆盖不可逆，且会让回滚（CopyRollbackStrategy 只删目标）变成删除用户文件。
func TestFileTaskHandler_RejectsExistingDestination(t *testing.T) {
	h := newTestFileTaskHandler()

	ops := map[string]func(task *MobileTask) error{
		"copy":   h.ProcessCopy,
		"move":   h.ProcessMove,
		"rename": h.ProcessRename,
	}

	for op, run := range ops {
		t.Run(op, func(t *testing.T) {
			dir := t.TempDir()
			src := filepath.Join(dir, "src.txt")
			dst := filepath.Join(dir, "dst.txt")
			require.NoError(t, os.WriteFile(src, []byte("src"), 0o644))
			require.NoError(t, os.WriteFile(dst, []byte("KEEP ME"), 0o644))

			err := run(&MobileTask{Type: op, SourcePath: src, TargetPath: dst})
			require.Error(t, err, "%s 到已存在目标必须失败", op)
			assert.Contains(t, err.Error(), "already exists")

			// 关键断言：既不能覆盖，也不能删除目标文件
			content, readErr := os.ReadFile(dst)
			require.NoError(t, readErr)
			assert.Equal(t, "KEEP ME", string(content), "目标文件内容必须原封不动")
			assert.FileExists(t, src, "源/原文件不应被破坏")
		})
	}
}

// TestFileTaskHandler_AllowsWhenDestinationFree 保证拒绝策略没有误伤正常路径。
func TestFileTaskHandler_AllowsWhenDestinationFree(t *testing.T) {
	h := newTestFileTaskHandler()
	dir := t.TempDir()

	src := filepath.Join(dir, "src.txt")
	copyDst := filepath.Join(dir, "copy.txt")
	moveDst := filepath.Join(dir, "move.txt")
	renameDst := filepath.Join(dir, "rename.txt")
	require.NoError(t, os.WriteFile(src, []byte("payload"), 0o644))

	require.NoError(t, h.ProcessCopy(&MobileTask{Type: "copy", SourcePath: src, TargetPath: copyDst}))
	require.NoError(t, h.ProcessMove(&MobileTask{Type: "move", SourcePath: src, TargetPath: moveDst}))
	require.NoError(t, h.ProcessRename(&MobileTask{Type: "rename", SourcePath: moveDst, TargetPath: renameDst}))

	assert.FileExists(t, copyDst)
	assert.FileExists(t, renameDst)
	assert.NoFileExists(t, src)
}

// TestFileTaskHandler_MoveRenameSamePathIsNoOp 保护「原地同名」这类调用
// 不被冲突校验误伤（否则 move/rename 到自身会莫名其妙失败）。
func TestFileTaskHandler_MoveRenameSamePathIsNoOp(t *testing.T) {
	h := newTestFileTaskHandler()
	dir := t.TempDir()
	path := filepath.Join(dir, "same.txt")
	require.NoError(t, os.WriteFile(path, []byte("keep"), 0o644))

	require.NoError(t, h.ProcessMove(&MobileTask{Type: "move", SourcePath: path, TargetPath: path}))
	require.NoError(t, h.ProcessRename(&MobileTask{Type: "rename", SourcePath: path, TargetPath: path}))

	content, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "keep", string(content))
}

// TestFileTaskHandler_CopyOntoItselfIsRejected 是数据丢失屏障：
// copyFile 用 os.Create 会先把目标截断成 0 字节，copy 到自身路径等于清空源文件。
func TestFileTaskHandler_CopyOntoItselfIsRejected(t *testing.T) {
	h := newTestFileTaskHandler()
	dir := t.TempDir()
	path := filepath.Join(dir, "self.txt")
	require.NoError(t, os.WriteFile(path, []byte("do not lose me"), 0o644))

	err := h.ProcessCopy(&MobileTask{Type: "copy", SourcePath: path, TargetPath: path})
	require.Error(t, err, "copy 到自身必须被拒绝")
	assert.Contains(t, err.Error(), "same file")

	content, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	assert.Equal(t, "do not lose me", string(content), "源文件绝不能被截断")
}

// ─────────── 显式 overwrite：先入回收站，可恢复 ───────────

// fakeTrash 是最小 trashMover 实现：把目标挪到 <dir>/.trash/<name>，不做持久化。
type fakeTrash struct {
	dir   string
	moved []string
}

func (f *fakeTrash) MoveToTrash(originalPath string, taskID string) (tasksystem.TrashItem, error) {
	if err := os.MkdirAll(filepath.Join(f.dir, ".trash"), 0o755); err != nil {
		return tasksystem.TrashItem{}, err
	}
	trashPath := filepath.Join(f.dir, ".trash", filepath.Base(originalPath))
	if err := os.Rename(originalPath, trashPath); err != nil {
		return tasksystem.TrashItem{}, err
	}
	f.moved = append(f.moved, trashPath)
	return tasksystem.TrashItem{
		ID:           "trash-" + filepath.Base(originalPath),
		OriginalPath: originalPath,
		TrashPath:    trashPath,
		TaskID:       taskID,
	}, nil
}

func TestFileTaskHandler_OverwriteMovesExistingToTrash(t *testing.T) {
	dir := t.TempDir()
	trash := &fakeTrash{dir: dir}
	h := NewFileTaskHandler(trash, nil)

	src := filepath.Join(dir, "src.txt")
	dst := filepath.Join(dir, "dst.txt")
	require.NoError(t, os.WriteFile(src, []byte("new"), 0o644))
	require.NoError(t, os.WriteFile(dst, []byte("old"), 0o644))

	err := h.ProcessCopy(&MobileTask{Type: "copy", SourcePath: src, TargetPath: dst, Overwrite: true})
	require.NoError(t, err)

	// 1. 覆盖成功：目标现在是新内容
	content, err := os.ReadFile(dst)
	require.NoError(t, err)
	assert.Equal(t, "new", string(content))

	// 2. 关键：被覆盖的旧内容没消失，而是在回收站里
	assert.Len(t, trash.moved, 1, "被覆盖的文件必须先入回收站")
	saved, err := os.ReadFile(trash.moved[0])
	require.NoError(t, err)
	assert.Equal(t, "old", string(saved), "旧内容必须可恢复")
}

func TestFileTaskHandler_OverwriteWithoutTrashManagerIsRefused(t *testing.T) {
	// 没有回收站 = 覆盖不可恢复 → 必须拒绝，而不是硬覆盖
	h := NewFileTaskHandler(nil, nil)
	dir := t.TempDir()
	src := filepath.Join(dir, "src.txt")
	dst := filepath.Join(dir, "dst.txt")
	require.NoError(t, os.WriteFile(src, []byte("new"), 0o644))
	require.NoError(t, os.WriteFile(dst, []byte("old"), 0o644))

	err := h.ProcessCopy(&MobileTask{Type: "copy", SourcePath: src, TargetPath: dst, Overwrite: true})
	require.Error(t, err, "无 TrashManager 时必须拒绝覆盖")
	assert.Contains(t, err.Error(), "TrashManager")

	content, readErr := os.ReadFile(dst)
	require.NoError(t, readErr)
	assert.Equal(t, "old", string(content), "目标不得被破坏")
}

func TestFileTaskHandler_OverwriteFreeDestinationStillWorks(t *testing.T) {
	// overwrite=true 但目标不存在 → 正常执行，不应报错也不应产生回收站条目
	dir := t.TempDir()
	trash := &fakeTrash{dir: dir}
	h := NewFileTaskHandler(trash, nil)

	src := filepath.Join(dir, "src.txt")
	dst := filepath.Join(dir, "fresh.txt")
	require.NoError(t, os.WriteFile(src, []byte("payload"), 0o644))

	require.NoError(t, h.ProcessCopy(&MobileTask{Type: "copy", SourcePath: src, TargetPath: dst, Overwrite: true}))
	assert.FileExists(t, dst)
	assert.Empty(t, trash.moved, "目标不存在时不应产生回收站条目")
}

// ─────────── 回滚：覆盖后要能还原被覆盖的文件 ───────────

// fakeStore 只实现回滚兜底需要的两个方法，其余通过嵌入接口占位。
type fakeStore struct {
	tasksystem.Store
	byTaskID map[string]tasksystem.TrashItem
	deleted  []string
}

func (f *fakeStore) GetTrashByTaskID(taskID string) (tasksystem.TrashItem, error) {
	item, ok := f.byTaskID[taskID]
	if !ok {
		return tasksystem.TrashItem{}, fmt.Errorf("no trash item for task %s", taskID)
	}
	return item, nil
}

func (f *fakeStore) DeleteTrash(id string) error {
	f.deleted = append(f.deleted, id)
	return nil
}

func TestRestoreOverwrittenFromTrash_RestoresAndCleansUp(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "dst.txt")
	trashPath := filepath.Join(dir, "dst.txt.in-trash")
	require.NoError(t, os.WriteFile(trashPath, []byte("original content"), 0o644))

	store := &fakeStore{byTaskID: map[string]tasksystem.TrashItem{
		"task-1": {ID: "trash-1", OriginalPath: dst, TrashPath: trashPath, TaskID: "task-1"},
	}}

	require.NoError(t, restoreOverwrittenFromTrash(store, "task-1"))

	content, err := os.ReadFile(dst)
	require.NoError(t, err)
	assert.Equal(t, "original content", string(content), "回滚必须还原被覆盖的文件")
	assert.NoFileExists(t, trashPath)
	assert.Equal(t, []string{"trash-1"}, store.deleted, "还原后应清理回收站条目")
}

func TestRestoreOverwrittenFromTrash_NoOverwriteIsNoOp(t *testing.T) {
	// 大多数任务没有覆盖过 → 查不到回收站条目，必须静默跳过（不能让回滚失败）
	store := &fakeStore{byTaskID: map[string]tasksystem.TrashItem{}}
	require.NoError(t, restoreOverwrittenFromTrash(store, "task-without-overwrite"))
	assert.Empty(t, store.deleted)

	// store 为 nil 时同样安全
	require.NoError(t, restoreOverwrittenFromTrash(nil, "any"))
}

func TestRestoreOverwrittenFromTrash_RefusesWhenPathOccupied(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "dst.txt")
	trashPath := filepath.Join(dir, "dst.txt.in-trash")
	require.NoError(t, os.WriteFile(dst, []byte("something else"), 0o644))
	require.NoError(t, os.WriteFile(trashPath, []byte("original"), 0o644))

	store := &fakeStore{byTaskID: map[string]tasksystem.TrashItem{
		"task-1": {ID: "trash-1", OriginalPath: dst, TrashPath: trashPath},
	}}

	err := restoreOverwrittenFromTrash(store, "task-1")
	require.Error(t, err, "原路径被占用时不能强行还原")
	assert.Contains(t, err.Error(), "already occupied")

	content, readErr := os.ReadFile(dst)
	require.NoError(t, readErr)
	assert.Equal(t, "something else", string(content), "不得破坏现有文件")
}

// TestFileTaskHandler_CopyRollbackCannotDeletePreExistingFile 锁住二阶风险：
// 回滚策略只删目标，所以「覆盖」一旦发生，回滚就等于删掉用户原有文件。
// 有了 ensureDestFree 前置校验，这条路径根本不会发生。
func TestFileTaskHandler_CopyRollbackCannotDeletePreExistingFile(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "dst.txt")
	require.NoError(t, os.WriteFile(dst, []byte("user data"), 0o644))

	strategy := &CopyRollbackStrategy{}
	err := strategy.ExecuteRollback(
		tasksystem.TaskData{Type: tasksystem.TaskTypeRollbackCopy, SourcePath: dst},
		tasksystem.Snapshot{},
	)
	// 说明：回滚确实就是「删除副本」。正因为如此，前置的冲突校验不能省。
	require.NoError(t, err, "回滚语义本身是删除副本（这正是必须先禁覆盖的原因）")
	assert.NoFileExists(t, dst)
}
