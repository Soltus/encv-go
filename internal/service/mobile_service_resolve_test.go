package service

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// 🆕 2026-06-15 multi-mount（spec Phase D3）：mount-aware resolveUserPath
//
// 行为：
//   - mountResolver == nil → 旧行为，所有路径走 SafeResolveToAbsPath(servingDir, ...)
//   - mountResolver != nil + 路径以 /d/ 开头 → 走 mount.Resolve
//   - mountResolver != nil + 路径不以 /d/ 开头 → 仍走 SafeResolveToAbsPath
//   - mountResolver != nil + 解析失败（未知 /d/xxx）→ 返回 error
//
// 这是 mobile_service.go 13 处 SafeResolveToAbsPath 替换的统一入口。
// helper 内部行为与 task_manager.resolveAbsPath 几乎对称，
// 但 mobile_service 这边返回 error（让 HTTP handler wrap 成 status code），
// 而 task_manager 那边返回空字符串（让 worker 走 failTask）。
func TestMobileService_ResolveUserPath_Mount(t *testing.T) {
	// 1. mountResolver nil → 旧行为
	t.Run("mountResolver=nil 时所有路径走旧 SafeResolveToAbsPath", func(t *testing.T) {
		s := &MobileService{servingDir: "/data/serving"}

		t.Run("相对路径 /foo.mp4", func(t *testing.T) {
			abs, err := s.resolveUserPath("/foo.mp4")
			assert.NoError(t, err)
			assert.Equal(t, "/data/serving/foo.mp4", abs)
		})

		t.Run("/d/automation 路径也会走旧（当作相对）", func(t *testing.T) {
			// 没有 mountResolver 时 /d/automation 应当被 SafeResolveToAbsPath 当作相对路径
			//   /d/automation/foo.mp4 → /data/serving/d/automation/foo.mp4
			abs, err := s.resolveUserPath("/d/automation/foo.mp4")
			assert.NoError(t, err)
			assert.Equal(t, "/data/serving/d/automation/foo.mp4", abs)
		})
	})

	// 2. mountResolver 注入
	s := &MobileService{
		servingDir: "/data/serving",
		mountResolver: &fakeMountResolver{
			mounts: map[string]*fakeMount{
				"/d/automation": {mountID: "m-auto", absPath: "/data/app/encv-automation"},
				"/d/primary":    {mountID: "m-prim", absPath: "/data/serving"},
			},
		},
	}

	t.Run("/d/automation/... 走 mount 解析", func(t *testing.T) {
		abs, err := s.resolveUserPath("/d/automation/01-plain-media/video/sample.mp4")
		assert.NoError(t, err)
		assert.Equal(t, "/data/app/encv-automation/01-plain-media/video/sample.mp4", abs)
	})

	t.Run("/d/primary/Download/foo.txt 走 mount 解析", func(t *testing.T) {
		abs, err := s.resolveUserPath("/d/primary/Download/foo.txt")
		assert.NoError(t, err)
		assert.Equal(t, "/data/serving/Download/foo.txt", abs)
	})

	t.Run("非 /d 路径（即使有 resolver）走旧 SafeResolveToAbsPath", func(t *testing.T) {
		abs, err := s.resolveUserPath("/video.mp4")
		assert.NoError(t, err)
		assert.Equal(t, "/data/serving/video.mp4", abs)
	})

	t.Run("未知 /d/xxx 解析失败返回 error", func(t *testing.T) {
		abs, err := s.resolveUserPath("/d/nonexistent/foo.mp4")
		assert.Error(t, err)
		assert.Empty(t, abs)
		// err 信息应包含原路径（便于定位）
		assert.True(t, strings.Contains(err.Error(), "/d/nonexistent"))
	})

	// 3. 移除 mountResolver 后 /d/ 路径也回退到旧行为
	s2 := &MobileService{servingDir: "/data/serving"}
	t.Run("mountResolver 移除后 /d/automation 也走旧行为", func(t *testing.T) {
		abs, err := s2.resolveUserPath("/d/automation/foo.mp4")
		assert.NoError(t, err)
		assert.Equal(t, "/data/serving/d/automation/foo.mp4", abs)
	})
}

// TestMobileService_ResolveUserPath_PathTraversal 验证 helper 仍阻止 path traversal
//
// 不管走 mount 还是旧 SafeResolveToAbsPath，都不应让路径逃出基础根。
// 注意 SafeResolveToAbsPath 内部的 filepath.Clean 会先简化 ../ ，
// 所以用能逃出 baseDir 的 ../../ 形式触发。
func TestMobileService_ResolveUserPath_PathTraversal(t *testing.T) {
	t.Run("mountResolver=nil 时 ../../etc/passwd 仍被 SafeResolveToAbsPath 阻止", func(t *testing.T) {
		s := &MobileService{servingDir: "/data/serving"}
		_, err := s.resolveUserPath("../../etc/passwd")
		assert.Error(t, err)
	})

	t.Run("mountResolver 注入时 ../../etc/passwd 仍被阻止（不走 mount）", func(t *testing.T) {
		s := &MobileService{
			servingDir: "/data/serving",
			mountResolver: &fakeMountResolver{
				mounts: map[string]*fakeMount{
					"/d/automation": {mountID: "m-auto", absPath: "/data/app/encv-automation"},
				},
			},
		}
		// 非 /d/ 开头 → 走 SafeResolveToAbsPath → ../../ 逃出 servingDir → 报错
		_, err := s.resolveUserPath("../../etc/passwd")
		assert.Error(t, err)
	})
}
