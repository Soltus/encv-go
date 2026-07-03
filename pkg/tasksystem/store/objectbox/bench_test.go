//go:build objectbox

package objectbox

import (
	"fmt"
	"testing"
	"time"

	"github.com/Soltus/encv-go/pkg/tasksystem"
)

func BenchmarkObjectBoxWrite(b *testing.B) {
	tmpDir := b.TempDir()
	store, err := New(tmpDir + "/obx")
	if err != nil {
		b.Fatal(err)
	}
	defer store.Close()

	tasks := make([]tasksystem.TaskData, 1000)
	for i := 0; i < 1000; i++ {
		tasks[i] = tasksystem.TaskData{
			ID:          fmt.Sprintf("bench-%d", i),
			Type:        tasksystem.TaskTypeEncrypt,
			Status:      tasksystem.StatusQueued,
			SourcePath:  fmt.Sprintf("/src/%d.mp4", i),
			TargetPath:  fmt.Sprintf("/dst/%d.encv", i),
			PluginName:  "test-plugin",
			TriggeredBy: "benchmark",
			Progress:    0,
			CreatedAt:   time.Now(),
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, t := range tasks {
			_ = store.CreateTask(t)
		}
	}
}

func BenchmarkObjectBoxWriteBatch(b *testing.B) {
	tmpDir := b.TempDir()
	store, err := New(tmpDir + "/obx")
	if err != nil {
		b.Fatal(err)
	}
	defer store.Close()

	entities := make([]*TaskEntity, 1000)
	for i := 0; i < 1000; i++ {
		entities[i] = &TaskEntity{
			TaskID:      fmt.Sprintf("bench-%d", i),
			Type:        "encrypt",
			Status:      "queued",
			SourcePath:  fmt.Sprintf("/src/%d.mp4", i),
			TargetPath:  fmt.Sprintf("/dst/%d.encv", i),
			PluginName:  "test-plugin",
			TriggeredBy: "benchmark",
			Progress:    0,
			CreatedAt:   time.Now().UnixMilli(),
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = store.tasks.PutMany(entities)
	}
}

func BenchmarkObjectBoxWriteTx(b *testing.B) {
	tmpDir := b.TempDir()
	store, err := New(tmpDir + "/obx")
	if err != nil {
		b.Fatal(err)
	}
	defer store.Close()

	tasks := make([]tasksystem.TaskData, 1000)
	for i := 0; i < 1000; i++ {
		tasks[i] = tasksystem.TaskData{
			ID:          fmt.Sprintf("bench-%d", i),
			Type:        tasksystem.TaskTypeEncrypt,
			Status:      tasksystem.StatusQueued,
			SourcePath:  fmt.Sprintf("/src/%d.mp4", i),
			TargetPath:  fmt.Sprintf("/dst/%d.encv", i),
			PluginName:  "test-plugin",
			TriggeredBy: "benchmark",
			Progress:    0,
			CreatedAt:   time.Now(),
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = store.ob.RunInWriteTx(func() error {
			for _, t := range tasks {
				_ = store.CreateTask(t)
			}
			return nil
		})
	}
}

func BenchmarkObjectBoxRead(b *testing.B) {
	tmpDir := b.TempDir()
	store, err := New(tmpDir + "/obx")
	if err != nil {
		b.Fatal(err)
	}
	defer store.Close()

	for i := 0; i < 1000; i++ {
		_ = store.CreateTask(tasksystem.TaskData{
			ID:          fmt.Sprintf("bench-%d", i),
			Type:        tasksystem.TaskTypeEncrypt,
			Status:      []tasksystem.TaskStatus{tasksystem.StatusCompleted, tasksystem.StatusRunning, tasksystem.StatusQueued}[i%3],
			SourcePath:  fmt.Sprintf("/src/%d.mp4", i),
			PluginName:  "test-plugin",
			TriggeredBy: []string{"user", "system", "automation"}[i%3],
			Progress:    i % 100,
			CreatedAt:   time.Now(),
		})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = store.ListTasks(tasksystem.TaskFilter{
			Statuses: []tasksystem.TaskStatus{tasksystem.StatusCompleted},
			Limit:    100,
		})
	}
}
