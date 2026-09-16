package server

import (
	"strings"
	"testing"

	"github.com/Soltus/encv-go/internal/config"
)

// TestTasksDataPath_NotServingDir 锁定用户要求：任务系统（DB/持久化/回收站）应用数据
// 必须落【数据目录】，绝不混进 servingDir（静态 web 根/用户媒体）。
// 续43 脉络：与 themeDataPath 同原则，servingDir 只装用户内容。
func TestTasksDataPath_NotServingDir(t *testing.T) {
	t.Setenv("ENCV_MOBILE", "")
	t.Setenv("ENCV_DEV", "1")
	t.Setenv("XDG_DATA_HOME", "/home/x/.local/share")
	t.Setenv("HOME", "/home/x")
	got := tasksDataPath()
	want := "/home/x/.local/share/encv-dev/tasks"
	if got != want {
		t.Errorf("linux dev tasksDataPath mismatch:\n got %q\nwant %q", got, want)
	}
	if strings.Contains(got, "servingDir") {
		t.Errorf("tasksDataPath must not reference servingDir: %q", got)
	}

	t.Setenv("ENCV_MOBILE", "1")
	t.Setenv("ENCV_APP_FILES_DIR", "/data/user/0/com.encvgo.app/files")
	gotA := tasksDataPath()
	wantA := "/data/user/0/com.encvgo.app/files/.encv/tasks"
	if gotA != wantA {
		t.Errorf("android tasksDataPath mismatch:\n got %q\nwant %q", gotA, wantA)
	}
	if !strings.Contains(gotA, "/data/user/0/") {
		t.Errorf("android tasksDataPath must be app-private path: %q", gotA)
	}
}

// TestFTSDataPath_NotServingDir 锁定：FTS5 全文索引应用数据绝不进 servingDir。
func TestFTSDataPath_NotServingDir(t *testing.T) {
	t.Setenv("ENCV_MOBILE", "")
	t.Setenv("ENCV_DEV", "1")
	t.Setenv("XDG_DATA_HOME", "/home/x/.local/share")
	t.Setenv("HOME", "/home/x")
	got := ftsDataPath()
	want := "/home/x/.local/share/encv-dev/fts"
	if got != want {
		t.Errorf("linux dev ftsDataPath mismatch:\n got %q\nwant %q", got, want)
	}
	if strings.Contains(got, "servingDir") {
		t.Errorf("ftsDataPath must not reference servingDir: %q", got)
	}
}

// TestAppDataDir_NotServingDir 反向锁：config.AppDataDir 派生不得包含字面量 servingDir，
// 且 ENCV_<SUB>_DIR 覆盖优先于派生默认值。
func TestAppDataDir_NotServingDir(t *testing.T) {
	t.Setenv("ENCV_MOBILE", "")
	t.Setenv("ENCV_DEV", "1")
	t.Setenv("XDG_DATA_HOME", "/home/x/.local/share")
	t.Setenv("HOME", "/home/x")
	got := config.AppDataDir("tasks")
	if strings.Contains(got, "servingDir") {
		t.Errorf("AppDataDir must not reference servingDir: %q", got)
	}
	// 覆盖优先
	t.Setenv("ENCV_TASKS_DIR", "/custom/tasks")
	if got2 := config.AppDataDir("tasks"); got2 != "/custom/tasks" {
		t.Errorf("ENCV_TASKS_DIR override failed: got %q", got2)
	}
}
