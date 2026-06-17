package utils

import (
	"path/filepath"
	"strings"
	"testing"

	// 强制激活 test-guard：拦截裸 go test 调用
	_ "github.com/Soltus/encv-go/internal/testguard"
)

func TestSafeResolveToAbsPath(t *testing.T) {
	baseDir := "/storage/emulated/0"

	tests := []struct {
		name     string
		baseDir  string
		userPath string
		want     string
		wantErr  bool
	}{
		{
			name:     "相对路径 - 正常情况",
			baseDir:  baseDir,
			userPath: "/foo/bar.bin",
			want:     filepath.Join(baseDir, "foo/bar.bin"),
			wantErr:  false,
		},
		{
			name:     "相对路径 - 无前导斜杠",
			baseDir:  baseDir,
			userPath: "foo/bar.bin",
			want:     filepath.Join(baseDir, "foo/bar.bin"),
			wantErr:  false,
		},
		{
			name:     "★绝对路径 - 在 baseDir 下（关键修复）★",
			baseDir:  baseDir,
			userPath: "/storage/emulated/0/hyYGPCwJPQ3+xrdAvfnn2.bin",
			want:     "/storage/emulated/0/hyYGPCwJPQ3+xrdAvfnn2.bin",
			wantErr:  false,
		},
		{
			name:     "★绝对路径 - 在 baseDir 子目录下（关键修复）★",
			baseDir:  baseDir,
			userPath: "/storage/emulated/0/01-plain-media/video/sample.mp4",
			want:     "/storage/emulated/0/01-plain-media/video/sample.mp4",
			wantErr:  false,
		},
		{
			name:     "★绝对路径 - 等于 baseDir 本身（关键修复）★",
			baseDir:  baseDir,
			userPath: "/storage/emulated/0",
			want:     "/storage/emulated/0",
			wantErr:  false,
		},
		{
			name:     "绝对路径 - 在 baseDir 外（降级为相对路径）",
			baseDir:  baseDir,
			userPath: "/etc/passwd",
			want:     "/storage/emulated/0/etc/passwd",
			wantErr:  false,
		},
		{
			name:     "绝对路径 - 路径穿越（被安全检查拒绝）",
			baseDir:  baseDir,
			userPath: "/storage/emulated/0/../emulated/0/foo.bin",
			want:     "",
			wantErr:  true,
		},
		{
			name:     "空路径",
			baseDir:  baseDir,
			userPath: "",
			want:     baseDir,
			wantErr:  false,
		},
		{
			name:     "相对路径穿越（应被拒绝）",
			baseDir:  baseDir,
			userPath: "../../../etc/passwd",
			want:     "",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := SafeResolveToAbsPath(tt.baseDir, tt.userPath)
			if (err != nil) != tt.wantErr {
				t.Errorf("SafeResolveToAbsPath() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("SafeResolveToAbsPath() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSafeResolveToAbsPath_NoDoublePrefix(t *testing.T) {
	// ★ 回归测试 ★: 确保不会发生路径双重拼接
	baseDir := "/storage/emulated/0"
	cases := []string{
		"/storage/emulated/0/foo.bin",
		"/storage/emulated/0/folder/foo.bin",
	}

	for _, userPath := range cases {
		got, err := SafeResolveToAbsPath(baseDir, userPath)
		if err != nil {
			t.Fatalf("SafeResolveToAbsPath(%q) failed: %v", userPath, err)
		}

		// 关键断言: 结果不应该包含双重 baseDir 前缀
		doublePrefix := baseDir + strings.TrimPrefix(baseDir, "/")
		if got == doublePrefix || strings.HasPrefix(got, doublePrefix+"/") {
			t.Errorf("DOUBLE PREFIX DETECTED: SafeResolveToAbsPath(%q) = %q (should not start with %q)", userPath, got, doublePrefix)
		}
	}
}
