// Package storetest 提供 tasksystem.Store 接口的通用测试套件。
//
// 各引擎实现（sqlite / libsql / objectbox / turso）可以调用 RunStoreTests
// 来跑完整的接口一致性测试，确保所有引擎行为一致。
//
// 用法示例（sqlite 包内）：
//
//	func TestStoreSuite(t *testing.T) {
//	    storetest.RunStoreTests(t, func(t *testing.T) tasksystem.Store {
//	        tmpDir := t.TempDir()
//	        store, err := New(filepath.Join(tmpDir, "test.db"))
//	        if err != nil { t.Fatal(err) }
//	        t.Cleanup(func() { store.Close() })
//	        return store
//	    })
//	}
package storetest

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/Soltus/encv-go/pkg/tasksystem"
	"github.com/Soltus/encv-go/pkg/tasksystem/performance"
)

// StoreFactory 创建一个新的测试用 Store，t.Cleanup 中已注册 Close。
type StoreFactory func(t *testing.T) tasksystem.Store

// RunStoreTests 运行完整的 Store 接口测试套件。
func RunStoreTests(t *testing.T, factory StoreFactory) {
	t.Run("CRUD", func(t *testing.T) {
		testCRUD(t, factory)
	})
	t.Run("ListFilter", func(t *testing.T) {
		testListFilter(t, factory)
	})
	t.Run("ListPagination", func(t *testing.T) {
		testListPagination(t, factory)
	})
	t.Run("Update", func(t *testing.T) {
		testUpdate(t, factory)
	})
	t.Run("Delete", func(t *testing.T) {
		testDelete(t, factory)
	})
	t.Run("Snapshot", func(t *testing.T) {
		testSnapshot(t, factory)
	})
	t.Run("Trash", func(t *testing.T) {
		testTrash(t, factory)
	})
	t.Run("Concurrency", func(t *testing.T) {
		testConcurrency(t, factory)
	})
	t.Run("ExportImport", func(t *testing.T) {
		testExportImport(t, factory)
	})
	t.Run("Calibration", func(t *testing.T) {
		testCalibration(t, factory)
	})
	t.Run("ListRuns", func(t *testing.T) {
		testListRuns(t, factory)
	})
	t.Run("CountByRunId", func(t *testing.T) {
		testCountByRunId(t, factory)
	})
}

func makeTask(id string, i int) tasksystem.TaskData {
	types := []tasksystem.TaskType{"encrypt", "decrypt"}
	statuses := []tasksystem.TaskStatus{"pending", "running", "success", "failed"}
	triggeredBys := []string{"user", "system", "automation", "ai_agent"}
	return tasksystem.TaskData{
		ID:          id,
		Type:        types[i%len(types)],
		Status:      statuses[i%len(statuses)],
		TriggeredBy: triggeredBys[i%len(triggeredBys)],
		SourcePath:  fmt.Sprintf("/test/source/file_%d.mp4", i),
		TargetPath:  fmt.Sprintf("/test/target/file_%d.encv", i),
		Progress:    i % 100,
		ServiceName: "test_service",
		MethodName:  "test_method",
		RunID:       fmt.Sprintf("run-%d", i%3),
		CreatedAt:   time.Now().UTC().Add(-time.Duration(i) * time.Second),
	}
}

func testCRUD(t *testing.T, factory StoreFactory) {
	store := factory(t)

	task := makeTask("crud-test-1", 0)
	task.ExtraFields = `{"key":"value"}`
	task.Steps = `[{"phase":"queued"}]`

	if err := store.CreateTask(task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	got, err := store.GetTask("crud-test-1")
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
	if got.ExtraFields != task.ExtraFields {
		t.Errorf("ExtraFields = %q, want %q", got.ExtraFields, task.ExtraFields)
	}
	if got.Steps != task.Steps {
		t.Errorf("Steps = %q, want %q", got.Steps, task.Steps)
	}

	// Get 不存在的任务应返回 ErrNotFound
	_, err = store.GetTask("nonexistent")
	if err != tasksystem.ErrNotFound {
		t.Errorf("GetTask(nonexistent) error = %v, want ErrNotFound", err)
	}
}

func testListFilter(t *testing.T, factory StoreFactory) {
	store := factory(t)

	const n = 20
	for i := 0; i < n; i++ {
		task := makeTask(fmt.Sprintf("filter-test-%d", i), i)
		if err := store.CreateTask(task); err != nil {
			t.Fatalf("CreateTask %d: %v", i, err)
		}
	}

	// 按 type 过滤
	encryptTasks, err := store.ListTasks(tasksystem.TaskFilter{
		Types: []tasksystem.TaskType{"encrypt"},
		Limit: n,
	})
	if err != nil {
		t.Fatalf("ListTasks(type=encrypt): %v", err)
	}
	if len(encryptTasks) == 0 {
		t.Error("no encrypt tasks found")
	}
	for _, task := range encryptTasks {
		if task.Type != "encrypt" {
			t.Errorf("task %s has type %s, want encrypt", task.ID, task.Type)
		}
	}

	// 按 status 过滤
	successTasks, err := store.ListTasks(tasksystem.TaskFilter{
		Statuses: []tasksystem.TaskStatus{"success"},
		Limit: n,
	})
	if err != nil {
		t.Fatalf("ListTasks(status=success): %v", err)
	}
	for _, task := range successTasks {
		if task.Status != "success" {
			t.Errorf("task %s has status %s, want success", task.ID, task.Status)
		}
	}

	// 按 triggeredBy 过滤
	userTasks, err := store.ListTasks(tasksystem.TaskFilter{
		TriggeredBy: []string{"user"},
		Limit: n,
	})
	if err != nil {
		t.Fatalf("ListTasks(triggeredBy=user): %v", err)
	}
	for _, task := range userTasks {
		if task.TriggeredBy != "user" {
			t.Errorf("task %s has triggeredBy %s, want user", task.ID, task.TriggeredBy)
		}
	}

	// 组合过滤
	combined, err := store.ListTasks(tasksystem.TaskFilter{
		Types:       []tasksystem.TaskType{"encrypt"},
		Statuses:    []tasksystem.TaskStatus{"running"},
		TriggeredBy: []string{"user"},
		Limit:       n,
	})
	if err != nil {
		t.Fatalf("ListTasks(combined): %v", err)
	}
	for _, task := range combined {
		if task.Type != "encrypt" || task.Status != "running" || task.TriggeredBy != "user" {
			t.Errorf("task %s doesn't match combined filter", task.ID)
		}
	}

	// 按 runId 过滤
	runTasks, err := store.ListTasks(tasksystem.TaskFilter{
		RunID: "run-0",
		Limit: n,
	})
	if err != nil {
		t.Fatalf("ListTasks(runId=run-0): %v", err)
	}
	for _, task := range runTasks {
		if task.RunID != "run-0" {
			t.Errorf("task %s has runId %s, want run-0", task.ID, task.RunID)
		}
	}
}

func testListPagination(t *testing.T, factory StoreFactory) {
	store := factory(t)

	const n = 30
	for i := 0; i < n; i++ {
		task := makeTask(fmt.Sprintf("page-test-%d", i), i)
		if err := store.CreateTask(task); err != nil {
			t.Fatalf("CreateTask %d: %v", i, err)
		}
	}

	// 第一页
	page1, err := store.ListTasks(tasksystem.TaskFilter{Limit: 10, Offset: 0})
	if err != nil {
		t.Fatalf("ListTasks(page1): %v", err)
	}
	if len(page1) != 10 {
		t.Errorf("len(page1) = %d, want 10", len(page1))
	}

	// 第二页
	page2, err := store.ListTasks(tasksystem.TaskFilter{Limit: 10, Offset: 10})
	if err != nil {
		t.Fatalf("ListTasks(page2): %v", err)
	}
	if len(page2) != 10 {
		t.Errorf("len(page2) = %d, want 10", len(page2))
	}

	// 两页不应有重叠
	ids1 := map[string]bool{}
	for _, task := range page1 {
		ids1[task.ID] = true
	}
	for _, task := range page2 {
		if ids1[task.ID] {
			t.Errorf("task %s appears in both pages", task.ID)
		}
	}
}

func testUpdate(t *testing.T, factory StoreFactory) {
	store := factory(t)

	task := makeTask("update-test-1", 0)
	if err := store.CreateTask(task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	task.Status = "success"
	task.Progress = 100
	task.Error = "test error"
	if err := store.UpdateTask(task); err != nil {
		t.Fatalf("UpdateTask: %v", err)
	}

	got, err := store.GetTask("update-test-1")
	if err != nil {
		t.Fatalf("GetTask after update: %v", err)
	}
	if got.Status != "success" {
		t.Errorf("Status = %q, want success", got.Status)
	}
	if got.Progress != 100 {
		t.Errorf("Progress = %d, want 100", got.Progress)
	}
	if got.Error != "test error" {
		t.Errorf("Error = %q, want 'test error'", got.Error)
	}

	// 更新不存在的任务 — 不同引擎行为可能不同（有的返回 ErrNotFound，有的静默成功）
	// 这里不做严格断言，只要不 panic 即可
	nonexistent := makeTask("nonexistent", 0)
	_ = store.UpdateTask(nonexistent)
}

func testDelete(t *testing.T, factory StoreFactory) {
	store := factory(t)

	task := makeTask("delete-test-1", 0)
	if err := store.CreateTask(task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	if err := store.DeleteTask("delete-test-1"); err != nil {
		t.Fatalf("DeleteTask: %v", err)
	}

	_, err := store.GetTask("delete-test-1")
	if err != tasksystem.ErrNotFound {
		t.Errorf("GetTask after delete error = %v, want ErrNotFound", err)
	}

	// 删除不存在的任务不应报错（或返回 NotFound，取决于实现）
	err = store.DeleteTask("nonexistent")
	// 两种行为都可以接受，不做断言
	_ = err
}

func testSnapshot(t *testing.T, factory StoreFactory) {
	store := factory(t)

	task := makeTask("snap-test-1", 0)
	if err := store.CreateTask(task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	snap := tasksystem.Snapshot{
		TaskID:       "snap-test-1",
		SnapshotType: tasksystem.SnapshotTypePreState,
		Data:         "test snapshot data",
		CreatedAt:    time.Now().UTC(),
	}
	if err := store.SaveSnapshot(snap); err != nil {
		t.Fatalf("SaveSnapshot: %v", err)
	}

	got, err := store.GetSnapshot("snap-test-1")
	if err != nil {
		t.Fatalf("GetSnapshot: %v", err)
	}
	if got.TaskID != "snap-test-1" {
		t.Errorf("TaskID = %q, want snap-test-1", got.TaskID)
	}
	if got.Data != "test snapshot data" {
		t.Errorf("Data = %q, want 'test snapshot data'", got.Data)
	}
}

func testTrash(t *testing.T, factory StoreFactory) {
	store := factory(t)

	item := tasksystem.TrashItem{
		ID:           "trash-test-1",
		OriginalPath: "/test/original.txt",
		TrashPath:    "/.trash/original.txt",
		IsDirectory:  false,
		Size:         1024,
		DeletedAt:    time.Now().UTC(),
		TaskID:       "task-trash-1",
	}
	if err := store.CreateTrash(item); err != nil {
		t.Fatalf("CreateTrash: %v", err)
	}

	got, err := store.GetTrash("trash-test-1")
	if err != nil {
		t.Fatalf("GetTrash: %v", err)
	}
	if got.ID != item.ID {
		t.Errorf("ID = %q, want %q", got.ID, item.ID)
	}
	if got.OriginalPath != item.OriginalPath {
		t.Errorf("OriginalPath = %q, want %q", got.OriginalPath, item.OriginalPath)
	}

	// 按 taskID 查
	gotByTask, err := store.GetTrashByTaskID("task-trash-1")
	if err != nil {
		t.Fatalf("GetTrashByTaskID: %v", err)
	}
	if gotByTask.ID != item.ID {
		t.Errorf("GetTrashByTaskID ID = %q, want %q", gotByTask.ID, item.ID)
	}

	// 列出回收站
	list, err := store.ListTrash()
	if err != nil {
		t.Fatalf("ListTrash: %v", err)
	}
	if len(list) < 1 {
		t.Error("ListTrash returned empty")
	}

	// 更新
	item.RestoreTaskID = "restore-task-1"
	if err := store.UpdateTrash(item); err != nil {
		t.Fatalf("UpdateTrash: %v", err)
	}
	got2, err := store.GetTrash("trash-test-1")
	if err != nil {
		t.Fatalf("GetTrash after update: %v", err)
	}
	if got2.RestoreTaskID != "restore-task-1" {
		t.Errorf("RestoreTaskID = %q, want restore-task-1", got2.RestoreTaskID)
	}

	// 删除
	if err := store.DeleteTrash("trash-test-1"); err != nil {
		t.Fatalf("DeleteTrash: %v", err)
	}
	_, err = store.GetTrash("trash-test-1")
	if err != tasksystem.ErrNotFound {
		t.Errorf("GetTrash after delete error = %v, want ErrNotFound", err)
	}
}

func testConcurrency(t *testing.T, factory StoreFactory) {
	store := factory(t)

	const goroutines = 5
	const perGoroutine = 20
	const total = goroutines * perGoroutine

	var wg sync.WaitGroup
	errCh := make(chan error, goroutines)

	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(gid int) {
			defer wg.Done()
			for i := 0; i < perGoroutine; i++ {
				task := makeTask(fmt.Sprintf("conc-%d-%d", gid, i), gid*perGoroutine+i)
				if err := store.CreateTask(task); err != nil {
					errCh <- fmt.Errorf("g%d i%d: %w", gid, i, err)
					return
				}
			}
		}(g)
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Fatalf("concurrent create error: %v", err)
	}

	// 验证总数
	all, err := store.ListTasks(tasksystem.TaskFilter{Limit: total * 2})
	if err != nil {
		t.Fatalf("ListTasks: %v", err)
	}
	if len(all) < total {
		t.Errorf("total tasks = %d, want at least %d", len(all), total)
	}
}

func testExportImport(t *testing.T, factory StoreFactory) {
	store := factory(t)

	const n = 10
	ids := make([]string, n)
	for i := 0; i < n; i++ {
		ids[i] = fmt.Sprintf("export-test-%d", i)
		task := makeTask(ids[i], i)
		if err := store.CreateTask(task); err != nil {
			t.Fatalf("CreateTask %d: %v", i, err)
		}
	}

	// 导出
	dump, err := store.ExportAll()
	if err != nil {
		t.Fatalf("ExportAll: %v", err)
	}
	if dump.Version == 0 {
		t.Error("dump.Version is 0")
	}
	if len(dump.Tasks) < n {
		t.Errorf("dump.Tasks len = %d, want >= %d", len(dump.Tasks), n)
	}

	// 清理
	for _, id := range ids {
		if err := store.DeleteTask(id); err != nil {
			t.Fatalf("DeleteTask %s: %v", id, err)
		}
	}

	// 导入
	if err := store.ImportAll(dump); err != nil {
		t.Fatalf("ImportAll: %v", err)
	}

	// 验证导入
	got, err := store.GetTask(ids[0])
	if err != nil {
		t.Fatalf("GetTask after import: %v", err)
	}
	if got.ID != ids[0] {
		t.Errorf("ID = %q, want %q", got.ID, ids[0])
	}
}

func testCalibration(t *testing.T, factory StoreFactory) {
	store := factory(t)

	cal := &performance.CalibrationResult{
		CPUScore:      1234.5,
		AESThroughput: 678.9,
		CPULabel:      "test-cpu",
		CalibratedAt:  time.Now().UTC(),
		GoVersion:     "go1.22.0",
		OS:            "linux",
		Arch:          "amd64",
		NumCPU:        8,
	}

	if err := store.SaveCalibration(*cal); err != nil {
		t.Fatalf("SaveCalibration: %v", err)
	}

	got, err := store.GetCalibration()
	if err != nil {
		t.Fatalf("GetCalibration: %v", err)
	}
	if got == nil {
		t.Fatal("GetCalibration returned nil")
	}
	if got.CPUScore != cal.CPUScore {
		t.Errorf("CPUScore = %f, want %f", got.CPUScore, cal.CPUScore)
	}
	if got.CPULabel != cal.CPULabel {
		t.Errorf("CPULabel = %q, want %q", got.CPULabel, cal.CPULabel)
	}
	if got.NumCPU != cal.NumCPU {
		t.Errorf("NumCPU = %d, want %d", got.NumCPU, cal.NumCPU)
	}
}

func testListRuns(t *testing.T, factory StoreFactory) {
	store := factory(t)

	const n = 15
	for i := 0; i < n; i++ {
		task := makeTask(fmt.Sprintf("runs-test-%d", i), i)
		if err := store.CreateTask(task); err != nil {
			t.Fatalf("CreateTask %d: %v", i, err)
		}
	}

	runs, err := store.ListRuns()
	if err != nil {
		t.Fatalf("ListRuns: %v", err)
	}
	if len(runs) < 3 {
		t.Errorf("runs count = %d, want >= 3", len(runs))
	}

	// 检查每个 run 都有非空 ID
	for _, r := range runs {
		if r.RunID == "" {
			t.Error("run has empty RunID")
		}
	}
}

func testCountByRunId(t *testing.T, factory StoreFactory) {
	store := factory(t)

	const n = 12
	for i := 0; i < n; i++ {
		task := makeTask(fmt.Sprintf("count-test-%d", i), i)
		if err := store.CreateTask(task); err != nil {
			t.Fatalf("CreateTask %d: %v", i, err)
		}
	}

	counts, err := store.CountByRunId("run-0")
	if err != nil {
		t.Fatalf("CountByRunId: %v", err)
	}
	if len(counts) == 0 {
		t.Error("CountByRunId returned empty map")
	}

	total := 0
	for _, c := range counts {
		total += c
	}
	if total == 0 {
		t.Error("CountByRunId total is 0")
	}
}
