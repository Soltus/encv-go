package mount

import (
	"path/filepath"
	"testing"

	// 强制激活 test-guard：拦截裸 go test 调用
	_ "github.com/Soltus/encv-go/internal/testguard"
)

// makeRegistryWithLocalDriver 构造一个带 local driver factory 的 MountRegistry（add 需要 instantiate driver）。
// 复用 bootstrap_test.go 的 fakeLocalDriver（同包内）。
func makeRegistryWithLocalDriver(t *testing.T) *MountRegistry {
	t.Helper()
	r := NewRegistry(nil, "")
	r.RegisterDriverFactory(DriverLocal, func() Driver { return &fakeLocalDriver{} })
	return r
}

// 🆕 v3 2026-06-18 Task 8：AbsToVirtual 反向解析单测
//   - 验证 absPath → virtualPath 转换正确性
//   - 验证 longest-prefix 匹配（多个 mount 时选最具体的）
//   - 验证 disabled mount 仍可匹配（产物路径回显不应因 mount 被禁用而丢失）
//   - 验证路径逃逸 / 不匹配 / 空输入的错误路径
func TestMountRegistry_AbsToVirtual(t *testing.T) {
	t.Parallel()

	// 构造测试 registry：2 个 mount
	//   /d/primary → /data/serving
	//   /d/automation → /data/appdata/automation
	reg := makeRegistryWithLocalDriver(t)
	primary := &Mount{
		ID:        "primary-1",
		Name:      "primary",
		MountPath: "/d/primary",
		Driver:    "local",
		RootPath:  "/data/serving",
		Enabled:   true,
	}
	automation := &Mount{
		ID:        "automation-1",
		Name:      "automation",
		MountPath: "/d/automation",
		Driver:    "local",
		RootPath:  "/data/appdata/automation",
		Enabled:   true,
	}
	if err := reg.add(primary); err != nil {
		t.Fatalf("add primary failed: %v", err)
	}
	if err := reg.add(automation); err != nil {
		t.Fatalf("add automation failed: %v", err)
	}

	tests := []struct {
		name    string
		absPath string
		want    string
		wantErr bool
	}{
		{
			name:    "primary root 本身",
			absPath: "/data/serving",
			want:    "/d/primary",
		},
		{
			name:    "primary 子路径",
			absPath: "/data/serving/video/sample.mp4",
			want:    "/d/primary/video/sample.mp4",
		},
		{
			name:    "automation root 本身",
			absPath: "/data/appdata/automation",
			want:    "/d/automation",
		},
		{
			name:    "automation 子路径",
			absPath: "/data/appdata/automation/01-plain-media/audio/sample.mp3",
			want:    "/d/automation/01-plain-media/audio/sample.mp3",
		},
		{
			name:    "不匹配任何 mount",
			absPath: "/etc/passwd",
			wantErr: true,
		},
		{
			name:    "空路径",
			absPath: "",
			wantErr: true,
		},
		{
			name:    "相对路径（非绝对路径）",
			absPath: "relative/path/file.txt",
			wantErr: true,
		},
		{
			name:    "前缀相似但非子路径（/data/serving-other 不应匹配 /data/serving）",
			absPath: "/data/serving-other/file.txt",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := reg.AbsToVirtual(tt.absPath)
			if tt.wantErr {
				if err == nil {
					t.Errorf("AbsToVirtual(%q) expected error, got %q", tt.absPath, got)
				}
				return
			}
			if err != nil {
				t.Errorf("AbsToVirtual(%q) unexpected error: %v", tt.absPath, err)
				return
			}
			// filepath.Clean 处理可能的 trailing slash 差异
			wantClean := filepath.Clean(tt.want)
			if got != wantClean {
				t.Errorf("AbsToVirtual(%q) = %q, want %q", tt.absPath, got, wantClean)
			}
		})
	}
}

// TestMountRegistry_AbsToVirtual_LongestPrefix 验证 longest-prefix 匹配
//   - 嵌套 mount：/d/outer → /data/outer, /d/outer/inner → /data/outer/inner
//   - absPath 在 inner 下应匹配 inner（更具体的 mount）
func TestMountRegistry_AbsToVirtual_LongestPrefix(t *testing.T) {
	t.Parallel()

	reg := makeRegistryWithLocalDriver(t)
	outer := &Mount{
		ID:        "outer-1",
		Name:      "outer",
		MountPath: "/d/outer",
		Driver:    "local",
		RootPath:  "/data/outer",
		Enabled:   true,
	}
	inner := &Mount{
		ID:        "inner-1",
		Name:      "inner",
		MountPath: "/d/outer/inner",
		Driver:    "local",
		RootPath:  "/data/outer/inner",
		Enabled:   true,
	}
	if err := reg.add(outer); err != nil {
		t.Fatalf("add outer failed: %v", err)
	}
	if err := reg.add(inner); err != nil {
		t.Fatalf("add inner failed: %v", err)
	}

	// 在 inner root 下的文件应匹配 inner mount
	got, err := reg.AbsToVirtual("/data/outer/inner/file.txt")
	if err != nil {
		t.Fatalf("AbsToVirtual failed: %v", err)
	}
	want := "/d/outer/inner/file.txt"
	if got != want {
		t.Errorf("AbsToVirtual = %q, want %q (should match inner mount, not outer)", got, want)
	}

	// 在 outer 但不在 inner 下的文件应匹配 outer mount
	got, err = reg.AbsToVirtual("/data/outer/other.txt")
	if err != nil {
		t.Fatalf("AbsToVirtual failed: %v", err)
	}
	want = "/d/outer/other.txt"
	if got != want {
		t.Errorf("AbsToVirtual = %q, want %q (should match outer mount)", got, want)
	}
}

// TestMountRegistry_AbsToVirtual_DisabledMount 验证 disabled mount 仍可匹配
//   - 产物路径回显不应因 mount 被禁用而丢失
func TestMountRegistry_AbsToVirtual_DisabledMount(t *testing.T) {
	t.Parallel()

	reg := makeRegistryWithLocalDriver(t)
	disabled := &Mount{
		ID:        "disabled-1",
		Name:      "disabled",
		MountPath: "/d/disabled",
		Driver:    "local",
		RootPath:  "/data/disabled",
		Enabled:   false, // 禁用
	}
	if err := reg.add(disabled); err != nil {
		t.Fatalf("add disabled failed: %v", err)
	}

	// disabled mount 仍应能匹配（AbsToVirtual 不检查 Enabled）
	got, err := reg.AbsToVirtual("/data/disabled/file.txt")
	if err != nil {
		t.Fatalf("AbsToVirtual failed for disabled mount: %v", err)
	}
	want := "/d/disabled/file.txt"
	if got != want {
		t.Errorf("AbsToVirtual = %q, want %q (disabled mount should still match)", got, want)
	}
}

// TestMountRegistry_AbsToVirtual_RoundTrip 验证 Resolve → AbsToVirtual 往返一致
//   - virtualPath → Resolve → absPath → AbsToVirtual → virtualPath（应等于原始 virtualPath）
func TestMountRegistry_AbsToVirtual_RoundTrip(t *testing.T) {
	t.Parallel()

	reg := makeRegistryWithLocalDriver(t)
	primary := &Mount{
		ID:        "primary-1",
		Name:      "primary",
		MountPath: "/d/primary",
		Driver:    "local",
		RootPath:  "/data/serving",
		Enabled:   true,
	}
	if err := reg.add(primary); err != nil {
		t.Fatalf("add primary failed: %v", err)
	}

	cases := []string{
		"/d/primary",
		"/d/primary/video/sample.mp4",
		"/d/primary/audio/deep/nested/file.mp3",
	}
	for _, virtual := range cases {
		// virtual → abs
		res, err := reg.Resolve(virtual)
		if err != nil {
			t.Fatalf("Resolve(%q) failed: %v", virtual, err)
		}
		// abs → virtual
		got, err := reg.AbsToVirtual(res.AbsPath)
		if err != nil {
			t.Fatalf("AbsToVirtual(%q) failed: %v", res.AbsPath, err)
		}
		want := filepath.Clean(virtual)
		if got != want {
			t.Errorf("Round-trip failed: %q → %q → %q, want %q", virtual, res.AbsPath, got, want)
		}
	}
}
