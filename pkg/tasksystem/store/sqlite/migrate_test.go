package sqlite

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Soltus/encv-go/pkg/tasksystem"
)

// migrateTaskJSON mirrors the JSON format expected by MigrateFromJSON
// (字段 tag 与 migrate.go 中的 migrateTask 对齐)。
type migrateTaskJSON struct {
	ID          string     `json:"id"`
	Type        string     `json:"type"`
	SourcePath  string     `json:"sourcePath"`
	TargetPath  string     `json:"targetPath,omitempty"`
	Status      string     `json:"status"`
	Progress    int        `json:"progress"`
	TriggeredBy string     `json:"triggeredBy,omitempty"`
	CreatedAt   time.Time  `json:"createdAt"`
	CompletedAt *time.Time `json:"completedAt,omitempty"`
	Error       string     `json:"error,omitempty"`
}

func TestMigrateFromJSON(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	jsonPath := filepath.Join(tmpDir, ".encv-tasks.json")

	now := time.Now().UTC().Truncate(time.Second)
	tasks := []migrateTaskJSON{
		{ID: "m1", Type: "encrypt", SourcePath: "/src1.mp4", Status: "completed", TriggeredBy: "user", CreatedAt: now},
		{ID: "m2", Type: "decrypt", SourcePath: "/src2.mp4", Status: "failed", TriggeredBy: "user", CreatedAt: now.Add(-1 * time.Hour)},
	}
	data, err := json.Marshal(tasks)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(jsonPath, data, 0644); err != nil {
		t.Fatalf("write json: %v", err)
	}

	store, err := New(dbPath)
	if err != nil {
		t.Fatalf("New store: %v", err)
	}
	defer store.Close()

	if err := MigrateFromJSON(store, jsonPath); err != nil {
		t.Fatalf("MigrateFromJSON: %v", err)
	}

	got, err := store.ListTasks(tasksystem.TaskFilter{})
	if err != nil {
		t.Fatalf("ListTasks: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}

	// 验证 JSON 文件被重命名为 .migrated
	if _, err := os.Stat(jsonPath); !os.IsNotExist(err) {
		t.Errorf("original JSON file should not exist: %v", err)
	}
	migratedPath := jsonPath + ".migrated"
	if _, err := os.Stat(migratedPath); err != nil {
		t.Errorf("migrated JSON file should exist: %v", err)
	}
}

func TestMigrateFromJSON_NotExist(t *testing.T) {
	store := newTestStore(t)
	jsonPath := filepath.Join(t.TempDir(), "nonexistent.json")
	if err := MigrateFromJSON(store, jsonPath); err != nil {
		t.Errorf("MigrateFromJSON with nonexistent file: expected nil, got %v", err)
	}
}

func TestMigrateFromJSON_RunningTask(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	jsonPath := filepath.Join(tmpDir, ".encv-tasks.json")

	now := time.Now().UTC().Truncate(time.Second)
	tasks := []migrateTaskJSON{
		{ID: "r1", Type: "encrypt", SourcePath: "/src.mp4", Status: "running", TriggeredBy: "user", CreatedAt: now},
	}
	data, err := json.Marshal(tasks)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(jsonPath, data, 0644); err != nil {
		t.Fatalf("write json: %v", err)
	}

	store, err := New(dbPath)
	if err != nil {
		t.Fatalf("New store: %v", err)
	}
	defer store.Close()

	if err := MigrateFromJSON(store, jsonPath); err != nil {
		t.Fatalf("MigrateFromJSON: %v", err)
	}

	got, err := store.GetTask("r1")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got.Status != tasksystem.StatusFailed {
		t.Errorf("Status = %q, want %q", got.Status, tasksystem.StatusFailed)
	}
	if got.Error != "interrupted by restart" {
		t.Errorf("Error = %q, want %q", got.Error, "interrupted by restart")
	}
}
