package service

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/Soltus/encv-go/internal/config"
)

// TestTaskManagerPersistPath_NotServingDir 锁：任务持久化文件（.encv-tasks.json）
// 绝不写进 servingDir（用户媒体根），必须落 config.AppDataDir 数据目录。
// 续43 脉络：有人若把 persistPath 改回 filepath.Join(servingDir, ...) 本测试立即 FAIL。
func TestTaskManagerPersistPath_NotServingDir(t *testing.T) {
	t.Setenv("ENCV_MOBILE", "")
	t.Setenv("ENCV_DEV", "1")
	t.Setenv("XDG_DATA_HOME", "/home/x/.local/share")
	t.Setenv("HOME", "/home/x")
	servingDir := "/storage/emulated/0/DCIM" // 典型用户媒体根
	tm := NewTaskManager(servingDir, &config.Config{}, nil)
	if tm.persistPath == "" {
		t.Fatal("persistPath empty")
	}
	if strings.HasPrefix(tm.persistPath, servingDir) || strings.Contains(tm.persistPath, "servingDir") {
		t.Errorf("task persistPath must not be under servingDir %q, got %q", servingDir, tm.persistPath)
	}
	want := filepath.Join(config.AppDataDir("tasks"), ".encv-tasks.json")
	if tm.persistPath != want {
		t.Errorf("persistPath = %q, want %q", tm.persistPath, want)
	}
}

// TestTrashManagerDir_NotServingDir 锁：回收站目录绝不写进 servingDir（用户媒体根）。
func TestTrashManagerDir_NotServingDir(t *testing.T) {
	t.Setenv("ENCV_MOBILE", "")
	t.Setenv("ENCV_DEV", "1")
	t.Setenv("XDG_DATA_HOME", "/home/x/.local/share")
	t.Setenv("HOME", "/home/x")
	servingDir := "/storage/emulated/0/DCIM"
	trash := NewTrashManager(servingDir, nil, nil)
	if strings.HasPrefix(trash.trashDir, servingDir) || strings.Contains(trash.trashDir, "servingDir") {
		t.Errorf("trashDir must not be under servingDir %q, got %q", servingDir, trash.trashDir)
	}
	want := filepath.Join(config.AppDataDir("tasks"), ".trash")
	if trash.trashDir != want {
		t.Errorf("trashDir = %q, want %q", trash.trashDir, want)
	}
}
