package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Soltus/encv-go/internal/config"
)

func TestRemoveTask_PersistenceAfterReload(t *testing.T) {
	tmpDir := t.TempDir()
	persistPath := filepath.Join(tmpDir, ".encv-tasks.json")

	existingTask := &MobileTask{
		ID:               "persist-test-task-001",
		Type:             "encrypt",
		SourcePath:       "/test/video.mp4",
		Status:           "completed",
		Progress:         100,
		ContainerVersion: 3,
	}
	taskList := []*MobileTask{existingTask}
	data, err := json.MarshalIndent(taskList, "", "  ")
	if err != nil {
		t.Fatalf("marshal pre-existing task: %v", err)
	}
	if err := os.WriteFile(persistPath, data, 0644); err != nil {
		t.Fatalf("write persist file: %v", err)
	}

	cfg := &config.Config{Password: "test-password-123"}
	tm := NewTaskManager(tmpDir, cfg, nil)
	defer tm.Stop()

	list := tm.List()
	found := false
	for _, tsk := range list {
		if tsk.ID == existingTask.ID {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("pre-existing task should be loaded from persist file")
	}

	tm.mu.Lock()
	delete(tm.tasks, existingTask.ID)
	tm.mu.Unlock()

	raw, err := os.ReadFile(persistPath)
	if err != nil {
		t.Fatalf("read persist file before manual remove: %v", err)
	}
	var beforeRemove []*MobileTask
	if err := json.Unmarshal(raw, &beforeRemove); err != nil {
		t.Fatalf("unmarshal before remove: %v", err)
	}

	var afterRemove []*MobileTask
	for _, tsk := range beforeRemove {
		if tsk.ID != existingTask.ID {
			afterRemove = append(afterRemove, tsk)
		}
	}

	updated, err := json.MarshalIndent(afterRemove, "", "  ")
	if err != nil {
		t.Fatalf("marshal after remove: %v", err)
	}
	if err := os.WriteFile(persistPath, updated, 0644); err != nil {
		t.Fatalf("write updated persist file: %v", err)
	}

	tm2 := NewTaskManager(tmpDir, cfg, nil)
	defer tm2.Stop()

	list2 := tm2.List()
	for _, tsk := range list2 {
		if tsk.ID == existingTask.ID {
			t.Fatal("removed task should NOT reappear after reloading TaskManager from updated persist file")
		}
	}
}

func TestRemoveTask_NotFound(t *testing.T) {
	tmpDir := t.TempDir()

	tm := NewTaskManager(tmpDir, &config.Config{Password: "x"}, nil)
	defer tm.Stop()

	err := tm.RemoveTask("non-existent-task-id")
	if err == nil {
		t.Fatal("expected error for removing non-existent task")
	}
}

func TestTaskPersistence_RoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	persistPath := filepath.Join(tmpDir, ".encv-tasks.json")

	originalTasks := []*MobileTask{
		{
			ID:               "roundtrip-001",
			Type:             "encrypt",
			SourcePath:       "/a/b.mp4",
			Status:           "completed",
			Progress:         100,
			ContainerVersion: 3,
		},
		{
			ID:               "roundtrip-002",
			Type:             "decrypt",
			SourcePath:       "/c/d.sccgt",
			Status:           "failed",
			Progress:         50,
			Error:            "test error",
			ContainerVersion: 3,
		},
	}

	data, err := json.MarshalIndent(originalTasks, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(persistPath, data, 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	cfg := &config.Config{Password: "test-pass"}
	tm := NewTaskManager(tmpDir, cfg, nil)
	defer tm.Stop()

	loaded := tm.List()
	if len(loaded) != 2 {
		t.Fatalf("expected 2 loaded tasks, got %d", len(loaded))
	}

	idMap := make(map[string]bool)
	for _, tsk := range loaded {
		idMap[tsk.ID] = true
	}
	if !idMap["roundtrip-001"] {
		t.Error("roundtrip-001 not found in loaded tasks")
	}
	if !idMap["roundtrip-002"] {
		t.Error("roundtrip-002 not found in loaded tasks")
	}

	tm2 := NewTaskManager(tmpDir, cfg, nil)
	defer tm2.Stop()

	loaded2 := tm2.List()
	if len(loaded2) != 2 {
		t.Fatalf("expected 2 tasks after reload, got %d", len(loaded2))
	}
}
