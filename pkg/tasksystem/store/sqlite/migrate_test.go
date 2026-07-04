package sqlite

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Soltus/encv-go/pkg/tasksystem"
)

func TestMigrateFromJSON(t *testing.T) {
	tmpDir := t.TempDir()
	jsonPath := filepath.Join(tmpDir, ".encv-tasks.json")
	dbPath := filepath.Join(tmpDir, ".encv-tasks.db")

	// 创建 JSON 文件，含 2 个任务
	tasks := []migrateTask{
		{
			ID:        "migrate-1",
			Type:      "encrypt",
			Status:    "completed",
			SourcePath: "/test/source1.mp4",
			TriggeredBy: "user",
			CreatedAt: time.Now().UTC(),
		},
		{
			ID:        "migrate-2",
			Type:      "decrypt",
			Status:    "completed",
			SourcePath: "/test/source2.encv",
			TriggeredBy: "automation",
			CreatedAt: time.Now().UTC(),
		},
	}
	data, err := json.MarshalIndent(tasks, "", "  ")
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if err := os.WriteFile(jsonPath, data, 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// 创建空 Store
	store, err := New(dbPath)
	if err != nil {
		t.Fatalf("New store: %v", err)
	}
	defer store.Close()

	// 执行迁移
	if err := MigrateFromJSON(store, jsonPath); err != nil {
		t.Fatalf("MigrateFromJSON: %v", err)
	}

	// 验证 Store 中有 2 个任务
	got, err := store.ListTasks(tasksystem.TaskFilter{})
	if err != nil {
		t.Fatalf("ListTasks: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len(tasks) = %d, want 2", len(got))
	}

	// 验证 JSON 文件被重命名为 .migrated
	if _, err := os.Stat(jsonPath); !os.IsNotExist(err) {
		t.Errorf("JSON file still exists, expected renamed to .migrated")
	}
	migratedPath := jsonPath + ".migrated"
	if _, err := os.Stat(migratedPath); err != nil {
		t.Errorf("migrated file not found: %v", err)
	}
}

func TestMigrateFromJSON_NotExist(t *testing.T) {
	tmpDir := t.TempDir()
	jsonPath := filepath.Join(tmpDir, "nonexistent.json")
	dbPath := filepath.Join(tmpDir, "test.db")

	store, err := New(dbPath)
	if err != nil {
		t.Fatalf("New store: %v", err)
	}
	defer store.Close()

	// JSON 文件不存在，应返回 nil
	if err := MigrateFromJSON(store, jsonPath); err != nil {
		t.Errorf("MigrateFromJSON with nonexistent file: expected nil, got %v", err)
	}
}

func TestMigrateFromJSON_RunningTask(t *testing.T) {
	tmpDir := t.TempDir()
	jsonPath := filepath.Join(tmpDir, ".encv-tasks.json")
	dbPath := filepath.Join(tmpDir, ".encv-tasks.db")

	// JSON 含 running 状态任务
	tasks := []migrateTask{
		{
			ID:        "running-1",
			Type:      "encrypt",
			Status:    "running",
			SourcePath: "/test/source.mp4",
			CreatedAt: time.Now().UTC(),
		},
	}
	data, _ := json.MarshalIndent(tasks, "", "  ")
	if err := os.WriteFile(jsonPath, data, 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	store, err := New(dbPath)
	if err != nil {
		t.Fatalf("New store: %v", err)
	}
	defer store.Close()

	if err := MigrateFromJSON(store, jsonPath); err != nil {
		t.Fatalf("MigrateFromJSON: %v", err)
	}

	// 验证 running 任务被标记为 failed
	got, err := store.GetTask("running-1")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got.Status != "failed" {
		t.Errorf("Status = %q, want failed", got.Status)
	}
	if got.Error != "interrupted by restart" {
		t.Errorf("Error = %q, want 'interrupted by restart'", got.Error)
	}
	if got.CompletedAt == nil {
		t.Errorf("CompletedAt = nil, want non-nil")
	}
}

func TestMigrateFromJSON_CancellingTask(t *testing.T) {
	tmpDir := t.TempDir()
	jsonPath := filepath.Join(tmpDir, ".encv-tasks.json")
	dbPath := filepath.Join(tmpDir, ".encv-tasks.db")

	tasks := []migrateTask{
		{
			ID:        "cancelling-1",
			Type:      "encrypt",
			Status:    "cancelling",
			SourcePath: "/test/source.mp4",
			CreatedAt: time.Now().UTC(),
		},
	}
	data, _ := json.MarshalIndent(tasks, "", "  ")
	if err := os.WriteFile(jsonPath, data, 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	store, err := New(dbPath)
	if err != nil {
		t.Fatalf("New store: %v", err)
	}
	defer store.Close()

	if err := MigrateFromJSON(store, jsonPath); err != nil {
		t.Fatalf("MigrateFromJSON: %v", err)
	}

	got, err := store.GetTask("cancelling-1")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got.Status != "cancelled" {
		t.Errorf("Status = %q, want cancelled", got.Status)
	}
}
