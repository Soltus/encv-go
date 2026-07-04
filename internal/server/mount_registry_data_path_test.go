package server

import (
	"path/filepath"
	"runtime"
	"testing"
)

// TestMountRegistryDataPath_Android 验证 Android 端 ENCV_MOBILE=1 走 app 私有目录。
func TestMountRegistryDataPath_Android(t *testing.T) {
	t.Setenv("ENCV_MOBILE", "1")
	t.Setenv("ENCV_DEV", "")
	t.Setenv("ENCV_DEV_PREVIEW", "")
	t.Setenv("ENCV_APP_FILES_DIR", "/data/user/0/com.encvgo.app/files")

	got := defaultMountRegistryDataPath()
	want := "/data/user/0/com.encvgo.app/files/.encv/mounts.json"
	if got != want {
		t.Errorf("Android path mismatch:\n got %q\nwant %q", got, want)
	}
}

// TestMountRegistryDataPath_AndroidFallback 验证 Android 端 ENCV_APP_FILES_DIR 未设时 fallback。
func TestMountRegistryDataPath_AndroidFallback(t *testing.T) {
	t.Setenv("ENCV_MOBILE", "1")
	t.Setenv("ENCV_APP_FILES_DIR", "")

	got := defaultMountRegistryDataPath()
	want := "/data/user/0/com.encvgo.app/files/.encv/mounts.json"
	if got != want {
		t.Errorf("Android fallback path mismatch:\n got %q\nwant %q", got, want)
	}
}

// TestMountRegistryDataPath_AndroidCustomPackage 验证 ENCV_PACKAGE_NAME 覆盖包名（测试用）。
func TestMountRegistryDataPath_AndroidCustomPackage(t *testing.T) {
	t.Setenv("ENCV_MOBILE", "1")
	t.Setenv("ENCV_APP_FILES_DIR", "")
	t.Setenv("ENCV_PACKAGE_NAME", "com.test.encv")

	got := defaultMountRegistryDataPath()
	want := "/data/user/0/com.test.encv/files/.encv/mounts.json"
	if got != want {
		t.Errorf("Android custom pkg path mismatch:\n got %q\nwant %q", got, want)
	}
}

// TestMountRegistryDataPath_DesktopDevXDG 验证 Linux/macOS dev 走 XDG_DATA_HOME/encv-dev。
func TestMountRegistryDataPath_DesktopDevXDG(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("XDG not applicable on Windows")
	}
	t.Setenv("ENCV_MOBILE", "")
	t.Setenv("ENCV_DEV", "1")
	t.Setenv("ENCV_DEV_PREVIEW", "")
	t.Setenv("XDG_DATA_HOME", "/tmp/test-xdg")

	got := defaultMountRegistryDataPath()
	want := "/tmp/test-xdg/encv-dev/mounts.json"
	if got != want {
		t.Errorf("Linux dev XDG path mismatch:\n got %q\nwant %q", got, want)
	}
}

// TestMountRegistryDataPath_DesktopProdXDG 验证 Linux/macOS production 走 XDG_DATA_HOME/encv。
func TestMountRegistryDataPath_DesktopProdXDG(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("XDG not applicable on Windows")
	}
	t.Setenv("ENCV_MOBILE", "")
	t.Setenv("ENCV_DEV", "")
	t.Setenv("ENCV_DEV_PREVIEW", "")
	t.Setenv("XDG_DATA_HOME", "/tmp/test-xdg")

	got := defaultMountRegistryDataPath()
	want := "/tmp/test-xdg/encv/mounts.json"
	if got != want {
		t.Errorf("Linux prod XDG path mismatch:\n got %q\nwant %q", got, want)
	}
}

// TestMountRegistryDataPath_DesktopDevPreview 验证 ENCV_DEV_PREVIEW 也走 dev 路径。
func TestMountRegistryDataPath_DesktopDevPreview(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("XDG not applicable on Windows")
	}
	t.Setenv("ENCV_MOBILE", "")
	t.Setenv("ENCV_DEV", "")
	t.Setenv("ENCV_DEV_PREVIEW", "1")
	t.Setenv("XDG_DATA_HOME", "/tmp/test-xdg")

	got := defaultMountRegistryDataPath()
	want := "/tmp/test-xdg/encv-dev/mounts.json"
	if got != want {
		t.Errorf("Linux dev preview path mismatch:\n got %q\nwant %q", got, want)
	}
}

// TestMountRegistryDataPath_OverrideEnv 验证 ENCV_MOUNTS_FILE 最高优先级。
func TestMountRegistryDataPath_OverrideEnv(t *testing.T) {
	t.Setenv("ENCV_MOUNTS_FILE", "/custom/explicit/path.json")
	t.Setenv("ENCV_MOBILE", "1")
	t.Setenv("ENCV_APP_FILES_DIR", "/data/user/0/com.encvgo.app/files")

	got := mountRegistryDataPath(nil)
	want := "/custom/explicit/path.json"
	if got != want {
		t.Errorf("override env mismatch:\n got %q\nwant %q", got, want)
	}
}

// TestConfigMountProvider_DataDir_Android 验证 ConfigProvider.DataDir() Android 模式。
//
// 模拟 cfg.Mobile != nil 的状态：通过 IsMobile() 路径走到 Android 分支。
func TestConfigMountProvider_DataDir_Android(t *testing.T) {
	// 模拟 Android 真机
	t.Setenv("ENCV_MOBILE", "1")
	t.Setenv("ENCV_APP_FILES_DIR", "/data/user/0/com.encvgo.app/files")

	// 构造 configMountProvider，绕过 cfg（IsMobile 只看 IsMobile 方法）
	p := &configMountProvider{}
	got := p.DataDir()
	want := "/data/user/0/com.encvgo.app/files"
	if got != want {
		t.Errorf("DataDir Android mismatch:\n got %q\nwant %q", got, want)
	}
}

// TestConfigMountProvider_DataDir_AndroidFallback 验证 Android ENCV_APP_FILES_DIR fallback。
func TestConfigMountProvider_DataDir_AndroidFallback(t *testing.T) {
	t.Setenv("ENCV_MOBILE", "1")
	t.Setenv("ENCV_APP_FILES_DIR", "")

	p := &configMountProvider{}
	got := p.DataDir()
	want := filepath.Join("/data/user/0", "com.encvgo.app", "files")
	if got != want {
		t.Errorf("DataDir Android fallback mismatch:\n got %q\nwant %q", got, want)
	}
}

// TestConfigMountProvider_DataDir_DesktopDev 验证桌面 dev 走 XDG/encv-dev。
func TestConfigMountProvider_DataDir_DesktopDev(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("XDG not applicable on Windows")
	}
	t.Setenv("ENCV_MOBILE", "")
	t.Setenv("ENCV_DEV", "1")
	t.Setenv("XDG_DATA_HOME", "/tmp/test-xdg")

	p := &configMountProvider{}
	got := p.DataDir()
	want := "/tmp/test-xdg/encv-dev"
	if got != want {
		t.Errorf("DataDir desktop dev mismatch:\n got %q\nwant %q", got, want)
	}
}

// TestConfigMountProvider_DataDir_DesktopProd 验证桌面 production 走 XDG/encv。
func TestConfigMountProvider_DataDir_DesktopProd(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("XDG not applicable on Windows")
	}
	t.Setenv("ENCV_MOBILE", "")
	t.Setenv("ENCV_DEV", "")
	t.Setenv("XDG_DATA_HOME", "/tmp/test-xdg")

	p := &configMountProvider{}
	got := p.DataDir()
	want := "/tmp/test-xdg/encv"
	if got != want {
		t.Errorf("DataDir desktop prod mismatch:\n got %q\nwant %q", got, want)
	}
}

// TestConfigMountProvider_DataDir_NotServingDir 验证 DataDir 绝不返回 cfg.Server.Dir。
//
// 2026-06-15 用户反馈根因：原版 DataDir() 返回 cfg.Server.Dir，
// 导致 /storage/emulated/0/.encv/mounts.json 出现在用户视图里。
// 新版必须独立于 cfg.Server.Dir。
func TestConfigMountProvider_DataDir_NotServingDir(t *testing.T) {
	// Android 模式下 cfg.Server.Dir 通常是 /storage/emulated/0（mobile overlay）
	// DataDir 必须返回 /data/user/0/<pkg>/files，**绝不能**是 /storage/emulated/0
	t.Setenv("ENCV_MOBILE", "1")
	t.Setenv("ENCV_APP_FILES_DIR", "/data/user/0/com.encvgo.app/files")

	p := &configMountProvider{}
	got := p.DataDir()
	for _, bad := range []string{"/storage/emulated/0", "/workspace", "./"} {
		if got == bad || filepath.HasPrefix(got, bad) {
			t.Errorf("DataDir must NOT be serving dir %q, got %q", bad, got)
		}
	}
}
