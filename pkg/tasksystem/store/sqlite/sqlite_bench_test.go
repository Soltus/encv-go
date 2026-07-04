package sqlite

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/Soltus/encv-go/pkg/tasksystem"
)

// BenchmarkCreateTask 批量创建任务基准测试。
//
// 注意：Go benchmark 仅作函数级补充参考，性能结论以 Cypress E2E 为准。
// 详见 .trae/rules/test-master-plan.md
func BenchmarkCreateTask(b *testing.B) {
	store := newBenchStore(b)
	tasks := makeBenchTasks(b.N)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := store.CreateTask(tasks[i]); err != nil {
			b.Fatalf("CreateTask: %v", err)
		}
	}
}

// BenchmarkCreateTaskBatch 批量创建（N 个一次性写入）基准测试。
func BenchmarkCreateTaskBatch(b *testing.B) {
	sizes := []int{10, 50, 100}
	for _, size := range sizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			store := newBenchStore(b)
			tasks := makeBenchTasks(size)

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				b.StopTimer()
				// 清空后重新测
				all, _ := store.ListTasks(tasksystem.TaskFilter{})
				for _, t := range all {
					_ = store.DeleteTask(t.ID)
				}
				b.StartTimer()

				for j := 0; j < size; j++ {
					if err := store.CreateTask(tasks[j]); err != nil {
						b.Fatalf("CreateTask: %v", err)
					}
				}
			}
		})
	}
}

// BenchmarkUpdateTask 更新任务基准测试。
func BenchmarkUpdateTask(b *testing.B) {
	store := newBenchStore(b)
	task := makeBenchTask("bench-update")
	if err := store.CreateTask(task); err != nil {
		b.Fatalf("CreateTask: %v", err)
	}

	task.Status = tasksystem.StatusRunning
	task.Progress = 50

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		task.Progress = (i % 100)
		if err := store.UpdateTask(task); err != nil {
			b.Fatalf("UpdateTask: %v", err)
		}
	}
}

// BenchmarkGetTask 读取单个任务基准测试。
func BenchmarkGetTask(b *testing.B) {
	store := newBenchStore(b)
	task := makeBenchTask("bench-get")
	if err := store.CreateTask(task); err != nil {
		b.Fatalf("CreateTask: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := store.GetTask("bench-get")
		if err != nil {
			b.Fatalf("GetTask: %v", err)
		}
	}
}

// BenchmarkListTasks 列出任务基准测试。
func BenchmarkListTasks(b *testing.B) {
	sizes := []int{10, 100, 500}
	for _, size := range sizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			store := newBenchStore(b)
			for i := 0; i < size; i++ {
				task := makeBenchTask(fmt.Sprintf("bench-list-%d", i))
				if err := store.CreateTask(task); err != nil {
					b.Fatalf("CreateTask: %v", err)
				}
			}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, err := store.ListTasks(tasksystem.TaskFilter{})
				if err != nil {
					b.Fatalf("ListTasks: %v", err)
				}
			}
		})
	}
}

// BenchmarkConcurrentWrites 并发写入基准测试（模拟多 worker 场景）。
//
// 注意：SQLite 单 writer 架构下，并发写入会串行化；
// Turso/libsql 支持多 writer 并发。此 benchmark 用于对比差异。
func BenchmarkConcurrentWrites(b *testing.B) {
	concurrencies := []int{1, 4, 8}
	for _, conc := range concurrencies {
		b.Run(fmt.Sprintf("conc=%d", conc), func(b *testing.B) {
			store := newBenchStore(b)
			tasksPerGoroutine := b.N / conc
			if tasksPerGoroutine < 1 {
				tasksPerGoroutine = 1
			}

			b.ResetTimer()
			done := make(chan struct{}, conc)
			for g := 0; g < conc; g++ {
				go func(gid int) {
					defer func() { done <- struct{}{} }()
					for i := 0; i < tasksPerGoroutine; i++ {
						task := makeBenchTask(fmt.Sprintf("conc-%d-%d", gid, i))
						_ = store.CreateTask(task)
					}
				}(g)
			}
			for g := 0; g < conc; g++ {
				<-done
			}
		})
	}
}

// ==========================================================================
// 辅助函数
// ==========================================================================

func newBenchStore(b *testing.B) *Store {
	tmpDir := b.TempDir()
	dbPath := filepath.Join(tmpDir, "bench.db")
	store, err := New(dbPath)
	if err != nil {
		b.Fatalf("New store: %v", err)
	}
	b.Cleanup(func() { store.Close() })
	return store
}

func makeBenchTasks(n int) []tasksystem.TaskData {
	tasks := make([]tasksystem.TaskData, n)
	now := time.Now().UTC()
	for i := 0; i < n; i++ {
		tasks[i] = tasksystem.TaskData{
			ID:          fmt.Sprintf("bench-%06d", i),
			Type:        tasksystem.TaskTypeEncrypt,
			Status:      tasksystem.StatusQueued,
			SourcePath:  fmt.Sprintf("/test/source-%06d.mp4", i),
			TargetPath:  fmt.Sprintf("/test/output-%06d.encv", i),
			TriggeredBy: "user",
			Progress:    0,
			CreatedAt:   now,
		}
	}
	return tasks
}

func makeBenchTask(id string) tasksystem.TaskData {
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
