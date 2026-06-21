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
		CreatedAt:   time.Now().UTC().Truncate(time.Second),
	}
}

func TestCreateAndGetTask(t *testing.T) {
	store := newTestStore(t)
	task := makeTestTask("task-1")
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
	if got.TargetPath != task.TargetPath {
		t.Errorf("TargetPath = %q, want %q", got.TargetPath, task.TargetPath)
	}
	if got.TriggeredBy != task.TriggeredBy {
		t.Errorf("TriggeredBy = %q, want %q", got.TriggeredBy, task.TriggeredBy)
	}
	if got.Progress != task.Progress {
		t.Errorf("Progress = %d, want %d", got.Progress, task.Progress)
	}
	if !got.CreatedAt.Equal(task.CreatedAt) {
		t.Errorf("CreatedAt = %v, want %v", got.CreatedAt, task.CreatedAt)
	}
}

func TestListTasks(t *testing.T) {
	store := newTestStore(t)
	base := time.Now().UTC().Truncate(time.Second)
	tasks := []tasksystem.TaskData{
		{ID: "old", Type: tasksystem.TaskTypeEncrypt, Status: tasksystem.StatusCompleted, TriggeredBy: "user", CreatedAt: base.Add(-2 * time.Hour)},
		{ID: "mid", Type: tasksystem.TaskTypeEncrypt, Status: tasksystem.StatusCompleted, TriggeredBy: "user", CreatedAt: base.Add(-1 * time.Hour)},
		{ID: "new", Type: tasksystem.TaskTypeEncrypt, Status: tasksystem.StatusCompleted, TriggeredBy: "user", CreatedAt: base},
	}
	for _, task := range tasks {
		if err := store.CreateTask(task); err != nil {
			t.Fatalf("CreateTask %s: %v", task.ID, err)
		}
	}
	got, err := store.ListTasks(tasksystem.TaskFilter{})
	if err != nil {
		t.Fatalf("ListTasks: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}
	if got[0].ID != "new" || got[1].ID != "mid" || got[2].ID != "old" {
		t.Errorf("order = %s, %s, %s; want new, mid, old", got[0].ID, got[1].ID, got[2].ID)
	}
}

func TestListTasksWithFilter(t *testing.T) {
	store := newTestStore(t)
	base := time.Now().UTC().Truncate(time.Second)
	tasks := []tasksystem.TaskData{
		{ID: "t1", Type: tasksystem.TaskTypeEncrypt, Status: tasksystem.StatusCompleted, TriggeredBy: "user", CreatedAt: base},
		{ID: "t2", Type: tasksystem.TaskTypeDecrypt, Status: tasksystem.StatusFailed, TriggeredBy: "automation", CreatedAt: base.Add(-1 * time.Hour)},
		{ID: "t3", Type: tasksystem.TaskTypeEncrypt, Status: tasksystem.StatusFailed, TriggeredBy: "user", CreatedAt: base.Add(-2 * time.Hour)},
		{ID: "t4", Type: tasksystem.TaskTypeDecrypt, Status: tasksystem.StatusCompleted, TriggeredBy: "automation", CreatedAt: base.Add(-3 * time.Hour)},
	}
	for _, task := range tasks {
		if err := store.CreateTask(task); err != nil {
			t.Fatalf("CreateTask %s: %v", task.ID, err)
		}
	}

	// Filter by Type
	got, err := store.ListTasks(tasksystem.TaskFilter{Types: []tasksystem.TaskType{tasksystem.TaskTypeEncrypt}})
	if err != nil {
		t.Fatalf("ListTasks by type: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("filter by type: len = %d, want 2", len(got))
	}
	for _, task := range got {
		if task.Type != tasksystem.TaskTypeEncrypt {
			t.Errorf("filter by type: got %s", task.Type)
		}
	}

	// Filter by Status
	got, err = store.ListTasks(tasksystem.TaskFilter{Statuses: []tasksystem.TaskStatus{tasksystem.StatusFailed}})
	if err != nil {
		t.Fatalf("ListTasks by status: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("filter by status: len = %d, want 2", len(got))
	}
	for _, task := range got {
		if task.Status != tasksystem.StatusFailed {
			t.Errorf("filter by status: got %s", task.Status)
		}
	}

	// Filter by TriggeredBy
	got, err = store.ListTasks(tasksystem.TaskFilter{TriggeredBy: []string{"automation"}})
	if err != nil {
		t.Fatalf("ListTasks by triggeredBy: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("filter by triggeredBy: len = %d, want 2", len(got))
	}
	for _, task := range got {
		if task.TriggeredBy != "automation" {
			t.Errorf("filter by triggeredBy: got %s", task.TriggeredBy)
		}
	}

	// Combined filter
	got, err = store.ListTasks(tasksystem.TaskFilter{
		Types:       []tasksystem.TaskType{tasksystem.TaskTypeEncrypt},
		Statuses:    []tasksystem.TaskStatus{tasksystem.StatusCompleted},
		TriggeredBy: []string{"user"},
	})
	if err != nil {
		t.Fatalf("ListTasks combined: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("combined filter: len = %d, want 1", len(got))
	}
	if got[0].ID != "t1" {
		t.Errorf("combined filter: ID = %s, want t1", got[0].ID)
	}
}

func TestUpdateTask(t *testing.T) {
	store := newTestStore(t)
	task := makeTestTask("task-update")
	if err := store.CreateTask(task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	task.Status = tasksystem.StatusRunning
	task.Progress = 50
	task.Phase = "processing"
	if err := store.UpdateTask(task); err != nil {
		t.Fatalf("UpdateTask: %v", err)
	}
	got, err := store.GetTask("task-update")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got.Status != tasksystem.StatusRunning {
		t.Errorf("Status = %q, want %q", got.Status, tasksystem.StatusRunning)
	}
	if got.Progress != 50 {
		t.Errorf("Progress = %d, want 50", got.Progress)
	}
	if got.Phase != "processing" {
		t.Errorf("Phase = %q, want %q", got.Phase, "processing")
	}
}

func TestDeleteTask(t *testing.T) {
	store := newTestStore(t)
	task := makeTestTask("task-delete")
	if err := store.CreateTask(task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if err := store.DeleteTask("task-delete"); err != nil {
		t.Fatalf("DeleteTask: %v", err)
	}
	if _, err := store.GetTask("task-delete"); err == nil {
		t.Error("GetTask after delete: expected error, got nil")
	}
}

func TestSaveAndGetSnapshot(t *testing.T) {
	store := newTestStore(t)
	now := time.Now().UTC().Truncate(time.Second)

	// Save post_state first (newer)
	postSnap := tasksystem.Snapshot{
		ID:           "snap-post",
		TaskID:       "task-snap",
		SnapshotType: tasksystem.SnapshotTypePostState,
		Data:         `{"post":true}`,
		CreatedAt:    now,
	}
	if err := store.SaveSnapshot(postSnap); err != nil {
		t.Fatalf("SaveSnapshot post: %v", err)
	}

	// Save pre_state (older, but should be preferred by GetSnapshot)
	preSnap := tasksystem.Snapshot{
		ID:           "snap-pre",
		TaskID:       "task-snap",
		SnapshotType: tasksystem.SnapshotTypePreState,
		Data:         `{"pre":true}`,
		CreatedAt:    now.Add(-1 * time.Second),
	}
	if err := store.SaveSnapshot(preSnap); err != nil {
		t.Fatalf("SaveSnapshot pre: %v", err)
	}

	got, err := store.GetSnapshot("task-snap")
	if err != nil {
		t.Fatalf("GetSnapshot: %v", err)
	}
	if got.SnapshotType != tasksystem.SnapshotTypePreState {
		t.Errorf("SnapshotType = %q, want %q (pre_state should take priority)", got.SnapshotType, tasksystem.SnapshotTypePreState)
	}
	if got.ID != "snap-pre" {
		t.Errorf("ID = %q, want %q", got.ID, "snap-pre")
	}
	if got.Data != `{"pre":true}` {
		t.Errorf("Data = %q, want %q", got.Data, `{"pre":true}`)
	}
	if got.TaskID != "task-snap" {
		t.Errorf("TaskID = %q, want %q", got.TaskID, "task-snap")
	}
}

func TestCreateAndGetTrash(t *testing.T) {
	store := newTestStore(t)
	now := time.Now().UTC().Truncate(time.Second)
	item := tasksystem.TrashItem{
		ID:           "trash-1",
		OriginalPath: "/test/original.txt",
		TrashPath:    "/trash/trash-1.txt",
		IsDirectory:  false,
		Size:         1024,
		DeletedAt:    now,
		TaskID:       "task-1",
		Metadata:     `{"mtime":"2026-01-01"}`,
	}
	if err := store.CreateTrash(item); err != nil {
		t.Fatalf("CreateTrash: %v", err)
	}
	got, err := store.GetTrash("trash-1")
	if err != nil {
		t.Fatalf("GetTrash: %v", err)
	}
	if got.ID != item.ID {
		t.Errorf("ID = %q, want %q", got.ID, item.ID)
	}
	if got.OriginalPath != item.OriginalPath {
		t.Errorf("OriginalPath = %q, want %q", got.OriginalPath, item.OriginalPath)
	}
	if got.TrashPath != item.TrashPath {
		t.Errorf("TrashPath = %q, want %q", got.TrashPath, item.TrashPath)
	}
	if got.IsDirectory != item.IsDirectory {
		t.Errorf("IsDirectory = %v, want %v", got.IsDirectory, item.IsDirectory)
	}
	if got.Size != item.Size {
		t.Errorf("Size = %d, want %d", got.Size, item.Size)
	}
	if got.TaskID != item.TaskID {
		t.Errorf("TaskID = %q, want %q", got.TaskID, item.TaskID)
	}
	if got.Metadata != item.Metadata {
		t.Errorf("Metadata = %q, want %q", got.Metadata, item.Metadata)
	}
	if !got.DeletedAt.Equal(item.DeletedAt) {
		t.Errorf("DeletedAt = %v, want %v", got.DeletedAt, item.DeletedAt)
	}
}

func TestGetTrashByTaskID(t *testing.T) {
	store := newTestStore(t)
	now := time.Now().UTC().Truncate(time.Second)
	item := tasksystem.TrashItem{
		ID:           "trash-2",
		OriginalPath: "/test/original2.txt",
		TrashPath:    "/trash/trash-2.txt",
		IsDirectory:  true,
		Size:         4096,
		DeletedAt:    now,
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
		t.Errorf("ID = %q, want %q", got.ID, "trash-2")
	}
	if got.TaskID != "task-2" {
		t.Errorf("TaskID = %q, want %q", got.TaskID, "task-2")
	}
	if !got.IsDirectory {
		t.Errorf("IsDirectory = false, want true")
	}
}

func TestListTrash(t *testing.T) {
	store := newTestStore(t)
	base := time.Now().UTC().Truncate(time.Second)
	items := []tasksystem.TrashItem{
		{ID: "old", OriginalPath: "/o", TrashPath: "/t/o", DeletedAt: base.Add(-2 * time.Hour)},
		{ID: "mid", OriginalPath: "/m", TrashPath: "/t/m", DeletedAt: base.Add(-1 * time.Hour)},
		{ID: "new", OriginalPath: "/n", TrashPath: "/t/n", DeletedAt: base},
	}
	for _, item := range items {
		if err := store.CreateTrash(item); err != nil {
			t.Fatalf("CreateTrash %s: %v", item.ID, err)
		}
	}
	got, err := store.ListTrash()
	if err != nil {
		t.Fatalf("ListTrash: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}
	if got[0].ID != "new" || got[1].ID != "mid" || got[2].ID != "old" {
		t.Errorf("order = %s, %s, %s; want new, mid, old", got[0].ID, got[1].ID, got[2].ID)
	}
}

func TestUpdateTrash(t *testing.T) {
	store := newTestStore(t)
	now := time.Now().UTC().Truncate(time.Second)
	item := tasksystem.TrashItem{
		ID:           "trash-upd",
		OriginalPath: "/test/upd.txt",
		TrashPath:    "/trash/upd.txt",
		DeletedAt:    now,
		TaskID:       "task-upd",
	}
	if err := store.CreateTrash(item); err != nil {
		t.Fatalf("CreateTrash: %v", err)
	}
	item.RestoreTaskID = "restore-task-1"
	if err := store.UpdateTrash(item); err != nil {
		t.Fatalf("UpdateTrash: %v", err)
	}
	got, err := store.GetTrash("trash-upd")
	if err != nil {
		t.Fatalf("GetTrash: %v", err)
	}
	if got.RestoreTaskID != "restore-task-1" {
		t.Errorf("RestoreTaskID = %q, want %q", got.RestoreTaskID, "restore-task-1")
	}
}

func TestDeleteTrash(t *testing.T) {
	store := newTestStore(t)
	now := time.Now().UTC().Truncate(time.Second)
	item := tasksystem.TrashItem{
		ID:           "trash-del",
		OriginalPath: "/test/del.txt",
		TrashPath:    "/trash/del.txt",
		DeletedAt:    now,
	}
	if err := store.CreateTrash(item); err != nil {
		t.Fatalf("CreateTrash: %v", err)
	}
	if err := store.DeleteTrash("trash-del"); err != nil {
		t.Fatalf("DeleteTrash: %v", err)
	}
	if _, err := store.GetTrash("trash-del"); err == nil {
		t.Error("GetTrash after delete: expected error, got nil")
	}
}
