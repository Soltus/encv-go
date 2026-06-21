package sqlite

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/Soltus/encv-go/pkg/tasksystem"
)

func newTestStore(t *testing.T) *Store {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	store, err := New(dbPath)
	if err != nil {
		t.Fatalf("New store: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

func makeTestTask(id string) tasksystem.TaskData {
	return tasksystem.TaskData{
		ID:          id,
		Type:        tasksystem.TaskTypeEncrypt,
		Status:      tasksystem.StatusQueued,
		SourcePath:  "/test/source.mp4",
		TargetPath:  "/test/output/",
		TriggeredBy: "user",
		Progress:    0,
		CreatedAt:   time.Now().UTC(),
	}
}

func TestCreateAndGetTask(t *testing.T) {
	store := newTestStore(t)
	task := makeTestTask("task-1")
	task.ExtraFields = `{"key":"value"}`
	task.Steps = `[{"phase":"queued"}]`

	if err := store.CreateTask(task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	got, err := store.GetTask("task-1")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got.ID != task.ID {
		t.Errorf("ID = %q, want %q", got.ID, task.ID)
	}
	if got.Type != task.Type {
		t.Errorf("Type = %q, want %q", got.Type, task.Type)
	}
	if got.Status != task.Status {
		t.Errorf("Status = %q, want %q", got.Status, task.Status)
	}
	if got.SourcePath != task.SourcePath {
		t.Errorf("SourcePath = %q, want %q", got.SourcePath, task.SourcePath)
	}
	if got.TriggeredBy != task.TriggeredBy {
		t.Errorf("TriggeredBy = %q, want %q", got.TriggeredBy, task.TriggeredBy)
	}
	if got.ExtraFields != task.ExtraFields {
		t.Errorf("ExtraFields = %q, want %q", got.ExtraFields, task.ExtraFields)
	}
	if got.Steps != task.Steps {
		t.Errorf("Steps = %q, want %q", got.Steps, task.Steps)
	}
}

func TestListTasks(t *testing.T) {
	store := newTestStore(t)
	// 创建 3 个任务，createdAt 递增
	for i, id := range []string{"t1", "t2", "t3"} {
		task := makeTestTask(id)
		task.CreatedAt = time.Now().UTC().Add(time.Duration(i) * time.Second)
		if err := store.CreateTask(task); err != nil {
			t.Fatalf("CreateTask %s: %v", id, err)
		}
	}

	tasks, err := store.ListTasks(tasksystem.TaskFilter{})
	if err != nil {
		t.Fatalf("ListTasks: %v", err)
	}
	if len(tasks) != 3 {
		t.Fatalf("len(tasks) = %d, want 3", len(tasks))
	}
	// 按 createdAt 倒序，t3 应该在最前
	if tasks[0].ID != "t3" {
		t.Errorf("tasks[0].ID = %q, want t3", tasks[0].ID)
	}
	if tasks[2].ID != "t1" {
		t.Errorf("tasks[2].ID = %q, want t1", tasks[2].ID)
	}
}

func TestListTasksWithFilter(t *testing.T) {
	store := newTestStore(t)
	// 创建不同 type/status/triggeredBy 的任务
	tasks := []tasksystem.TaskData{
		{ID: "e1", Type: tasksystem.TaskTypeEncrypt, Status: tasksystem.StatusCompleted, TriggeredBy: "user", CreatedAt: time.Now().UTC()},
		{ID: "e2", Type: tasksystem.TaskTypeEncrypt, Status: tasksystem.StatusFailed, TriggeredBy: "automation", CreatedAt: time.Now().UTC()},
		{ID: "d1", Type: tasksystem.TaskTypeDecrypt, Status: tasksystem.StatusCompleted, TriggeredBy: "user", CreatedAt: time.Now().UTC()},
		{ID: "m1", Type: tasksystem.TaskTypeMove, Status: tasksystem.StatusRunning, TriggeredBy: "ai_agent", CreatedAt: time.Now().UTC()},
	}
	for _, task := range tasks {
		if err := store.CreateTask(task); err != nil {
			t.Fatalf("CreateTask %s: %v", task.ID, err)
		}
	}

	// 按 type 过滤
	got, err := store.ListTasks(tasksystem.TaskFilter{Types: []tasksystem.TaskType{tasksystem.TaskTypeEncrypt}})
	if err != nil {
		t.Fatalf("ListTasks by type: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("type=encrypt: len = %d, want 2", len(got))
	}

	// 按 status 过滤
	got, err = store.ListTasks(tasksystem.TaskFilter{Statuses: []tasksystem.TaskStatus{tasksystem.StatusCompleted}})
	if err != nil {
		t.Fatalf("ListTasks by status: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("status=completed: len = %d, want 2", len(got))
	}

	// 按 triggeredBy 过滤
	got, err = store.ListTasks(tasksystem.TaskFilter{TriggeredBy: []string{"user"}})
	if err != nil {
		t.Fatalf("ListTasks by triggeredBy: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("triggeredBy=user: len = %d, want 2", len(got))
	}

	// 组合过滤
	got, err = store.ListTasks(tasksystem.TaskFilter{
		Types:       []tasksystem.TaskType{tasksystem.TaskTypeEncrypt},
		TriggeredBy: []string{"automation"},
	})
	if err != nil {
		t.Fatalf("ListTasks combo: %v", err)
	}
	if len(got) != 1 || got[0].ID != "e2" {
		t.Errorf("combo: got %v, want [e2]", got)
	}
}

func TestUpdateTask(t *testing.T) {
	store := newTestStore(t)
	task := makeTestTask("upd-1")
	if err := store.CreateTask(task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	task.Status = tasksystem.StatusCompleted
	task.Progress = 100
	task.OutputPath = "/test/output.encv"
	now := time.Now().UTC()
	task.CompletedAt = &now

	if err := store.UpdateTask(task); err != nil {
		t.Fatalf("UpdateTask: %v", err)
	}

	got, err := store.GetTask("upd-1")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got.Status != tasksystem.StatusCompleted {
		t.Errorf("Status = %q, want completed", got.Status)
	}
	if got.Progress != 100 {
		t.Errorf("Progress = %d, want 100", got.Progress)
	}
	if got.OutputPath != "/test/output.encv" {
		t.Errorf("OutputPath = %q, want /test/output.encv", got.OutputPath)
	}
	if got.CompletedAt == nil {
		t.Errorf("CompletedAt = nil, want non-nil")
	}
}

func TestDeleteTask(t *testing.T) {
	store := newTestStore(t)
	task := makeTestTask("del-1")
	if err := store.CreateTask(task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	if err := store.DeleteTask("del-1"); err != nil {
		t.Fatalf("DeleteTask: %v", err)
	}

	if _, err := store.GetTask("del-1"); err == nil {
		t.Errorf("GetTask after delete: expected error, got nil")
	}
}

func TestSaveAndGetSnapshot(t *testing.T) {
	store := newTestStore(t)
	task := makeTestTask("snap-1")
	if err := store.CreateTask(task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	preSnap := tasksystem.Snapshot{
		ID:           "snap-pre",
		TaskID:       "snap-1",
		SnapshotType: tasksystem.SnapshotTypePreState,
		Data:         `{"path":"/test/source.mp4"}`,
		CreatedAt:    time.Now().UTC(),
	}
	if err := store.SaveSnapshot(preSnap); err != nil {
		t.Fatalf("SaveSnapshot pre: %v", err)
	}

	postSnap := tasksystem.Snapshot{
		ID:           "snap-post",
		TaskID:       "snap-1",
		SnapshotType: tasksystem.SnapshotTypePostState,
		Data:         `{"path":"/test/output.encv"}`,
		CreatedAt:    time.Now().UTC().Add(time.Second),
	}
	if err := store.SaveSnapshot(postSnap); err != nil {
		t.Fatalf("SaveSnapshot post: %v", err)
	}

	// GetSnapshot 应优先返回 pre_state
	got, err := store.GetSnapshot("snap-1")
	if err != nil {
		t.Fatalf("GetSnapshot: %v", err)
	}
	if got.SnapshotType != tasksystem.SnapshotTypePreState {
		t.Errorf("SnapshotType = %q, want pre_state", got.SnapshotType)
	}
	if got.Data != preSnap.Data {
		t.Errorf("Data = %q, want %q", got.Data, preSnap.Data)
	}
}

func TestCreateAndGetTrash(t *testing.T) {
	store := newTestStore(t)
	item := tasksystem.TrashItem{
		ID:           "trash-1",
		OriginalPath: "/test/file.mp4",
		TrashPath:    "/test/.trash/123_file.mp4",
		IsDirectory:  false,
		Size:         1024,
		DeletedAt:    time.Now().UTC(),
		TaskID:       "task-1",
	}
	if err := store.CreateTrash(item); err != nil {
		t.Fatalf("CreateTrash: %v", err)
	}

	got, err := store.GetTrash("trash-1")
	if err != nil {
		t.Fatalf("GetTrash: %v", err)
	}
	if got.OriginalPath != item.OriginalPath {
		t.Errorf("OriginalPath = %q, want %q", got.OriginalPath, item.OriginalPath)
	}
	if got.IsDirectory != item.IsDirectory {
		t.Errorf("IsDirectory = %v, want %v", got.IsDirectory, item.IsDirectory)
	}
	if got.Size != item.Size {
		t.Errorf("Size = %d, want %d", got.Size, item.Size)
	}
}

func TestGetTrashByTaskID(t *testing.T) {
	store := newTestStore(t)
	item := tasksystem.TrashItem{
		ID:           "trash-2",
		OriginalPath: "/test/file2.mp4",
		TrashPath:    "/test/.trash/456_file2.mp4",
		IsDirectory:  false,
		Size:         2048,
		DeletedAt:    time.Now().UTC(),
		TaskID:       "task-2",
	}
	if err := store.CreateTrash(item); err != nil {
		t.Fatalf("CreateTrash: %v", err)
	}

	got, err := store.GetTrashByTaskID("task-2")
	if err != nil {
		t.Fatalf("GetTrashByTaskID: %v", err)
	}
	if got.ID != "trash-2" {
		t.Errorf("ID = %q, want trash-2", got.ID)
	}
}

func TestListTrash(t *testing.T) {
	store := newTestStore(t)
	for i, id := range []string{"tr1", "tr2", "tr3"} {
		item := tasksystem.TrashItem{
			ID:           id,
			OriginalPath: "/test/" + id,
			TrashPath:    "/test/.trash/" + id,
			DeletedAt:    time.Now().UTC().Add(time.Duration(i) * time.Second),
		}
		if err := store.CreateTrash(item); err != nil {
			t.Fatalf("CreateTrash %s: %v", id, err)
		}
	}

	items, err := store.ListTrash()
	if err != nil {
		t.Fatalf("ListTrash: %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("len = %d, want 3", len(items))
	}
	// 按 deletedAt 倒序，tr3 应该在最前
	if items[0].ID != "tr3" {
		t.Errorf("items[0].ID = %q, want tr3", items[0].ID)
	}
}

func TestUpdateTrash(t *testing.T) {
	store := newTestStore(t)
	item := tasksystem.TrashItem{
		ID:           "upd-trash",
		OriginalPath: "/test/file.mp4",
		TrashPath:    "/test/.trash/file.mp4",
		DeletedAt:    time.Now().UTC(),
	}
	if err := store.CreateTrash(item); err != nil {
		t.Fatalf("CreateTrash: %v", err)
	}

	item.RestoreTaskID = "restore-task-1"
	if err := store.UpdateTrash(item); err != nil {
		t.Fatalf("UpdateTrash: %v", err)
	}

	got, err := store.GetTrash("upd-trash")
	if err != nil {
		t.Fatalf("GetTrash: %v", err)
	}
	if got.RestoreTaskID != "restore-task-1" {
		t.Errorf("RestoreTaskID = %q, want restore-task-1", got.RestoreTaskID)
	}
}

func TestDeleteTrash(t *testing.T) {
	store := newTestStore(t)
	item := tasksystem.TrashItem{
		ID:           "del-trash",
		OriginalPath: "/test/file.mp4",
		TrashPath:    "/test/.trash/file.mp4",
		DeletedAt:    time.Now().UTC(),
	}
	if err := store.CreateTrash(item); err != nil {
		t.Fatalf("CreateTrash: %v", err)
	}

	if err := store.DeleteTrash("del-trash"); err != nil {
		t.Fatalf("DeleteTrash: %v", err)
	}

	if _, err := store.GetTrash("del-trash"); err == nil {
		t.Errorf("GetTrash after delete: expected error, got nil")
	}
}
