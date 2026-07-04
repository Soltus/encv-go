package server

import (
	"os"
	"testing"

	"github.com/Soltus/encv-go/internal/mount"
)

// TestAutomationDriver_DefaultIsAppData 验证真机默认 driver 是 appdata。
//
// 关键不变量（用户 2026-06-15 怒批"必然失败"）：
//   - 真机 release 默认 = DriverAppData（app-private 路径，有权限）
//   - 改成 DriverLocal 是致命错（shared storage /storage/emulated/0/ EACCES）
//
// 任何 PR 把默认值改成 local 都会被这个测试拦截。
func TestAutomationDriver_DefaultIsAppData(t *testing.T) {
	// 防御：测试前置清理 + 还原（万一其他测试设过 env）
	prev := os.Getenv("ENCV_AUTOMATION_DRIVER")
	defer os.Setenv("ENCV_AUTOMATION_DRIVER", prev)
	os.Unsetenv("ENCV_AUTOMATION_DRIVER")

	p := &configMountProvider{cfg: nil}
	got := p.AutomationDriver()
	if got != mount.DriverAppData {
		t.Errorf("AutomationDriver() 默认值错了: got %q, want %q (app-private 路径)", got, mount.DriverAppData)
	}
	if got == mount.DriverLocal {
		t.Errorf("AutomationDriver() 默认 = local 是致命错！真机写 /storage/emulated/0/ 必 EACCES")
	}
}

// TestAutomationDriver_EnvOverride 验证 opt-in 切换到 local（沙箱用）能工作。
func TestAutomationDriver_EnvOverride(t *testing.T) {
	prev := os.Getenv("ENCV_AUTOMATION_DRIVER")
	defer os.Setenv("ENCV_AUTOMATION_DRIVER", prev)

	os.Setenv("ENCV_AUTOMATION_DRIVER", mount.DriverLocal)
	p := &configMountProvider{cfg: nil}
	if got := p.AutomationDriver(); got != mount.DriverLocal {
		t.Errorf("env 覆盖失败: got %q, want %q", got, mount.DriverLocal)
	}
}

// TestDataDir_AppPrivateOnMobile 验证 DataDir 在真机模式下返回 app-private 路径。
//
// 不变量：
//   - 真机模式：/data/user/0/<pkg>/files/（或 ENCV_APP_FILES_DIR 注入）
//   - 不能是 /storage/emulated/0/
//   - 不能是 /sdcard/
func TestDataDir_AppPrivateOnMobile(t *testing.T) {
	prevFiles := os.Getenv("ENCV_APP_FILES_DIR")
	prevMobile := os.Getenv("ENCV_MOBILE")
	prevPkg := os.Getenv("ENCV_PACKAGE_NAME")
	defer func() {
		os.Setenv("ENCV_APP_FILES_DIR", prevFiles)
		os.Setenv("ENCV_MOBILE", prevMobile)
		os.Setenv("ENCV_PACKAGE_NAME", prevPkg)
	}()

	os.Setenv("ENCV_MOBILE", "1")
	os.Setenv("ENCV_PACKAGE_NAME", "com.encvgo.app")
	os.Unsetenv("ENCV_APP_FILES_DIR")

	p := &configMountProvider{cfg: nil}
	got := p.DataDir()
	if got == "" {
		t.Fatal("DataDir() 返回空")
	}
	wantPrefix := "/data/user/0/com.encvgo.app/files"
	if len(got) < len(wantPrefix) || got[:len(wantPrefix)] != wantPrefix {
		t.Errorf("DataDir() 不是 app-private 路径: got %q, want prefix %q", got, wantPrefix)
	}
	// 红线词检查
	for _, bad := range []string{"emulated", "sdcard", "/storage/"} {
		if containsSubstr(got, bad) {
			t.Errorf("DataDir() 命中 shared-storage 红线 %q: %q", bad, got)
		}
	}
	t.Logf("✅ 真机 DataDir = %s", got)
}

func containsSubstr(s, sub string) bool {
	if sub == "" {
		return true
	}
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
