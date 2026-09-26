package plugins

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ─────────── 实现可选能力的桩插件 ───────────

type outputDirStub struct {
	basePlugin
	got string
}

func (s *outputDirStub) SetOutputDir(dir string) { s.got = dir }

type verifyStub struct {
	basePlugin
	got bool
}

func (s *verifyStub) SetPostEncryptVerify(v bool) { s.got = v }

type sourcePathStub struct {
	basePlugin
	path string
}

func (s *sourcePathStub) EncryptedSourcePath() string { return s.path }

// ─────────── 测试 ───────────

func TestApplyOutputDir_CallsOptionalInterface(t *testing.T) {
	stub := &outputDirStub{}
	applyOutputDir(stub, "/out/dir")
	assert.Equal(t, "/out/dir", stub.got)
}

func TestApplyOutputDir_SkipsPluginWithoutCapability(t *testing.T) {
	// 未实现 OutputDirSetter 的插件：应被静默跳过，而不是 panic 或报错。
	assert.NotPanics(t, func() { applyOutputDir(&basePlugin{name: "plain"}, "/out") })
}

func TestApplyPostEncryptVerify_CallsOptionalInterface(t *testing.T) {
	stub := &verifyStub{}
	applyPostEncryptVerify(stub)
	assert.True(t, stub.got)

	assert.NotPanics(t, func() { applyPostEncryptVerify(&basePlugin{name: "plain"}) })
}

func TestCleanupPreprocessedSource_RemovesEncryptedTemp(t *testing.T) {
	tmpRoot := t.TempDir()
	tmpDir := filepath.Join(tmpRoot, "job123.encv_tmp")
	require.NoError(t, os.MkdirAll(tmpDir, 0o755))
	artifact := filepath.Join(tmpDir, "pre.m3u8")
	require.NoError(t, os.WriteFile(artifact, []byte("x"), 0o644))

	stub := &sourcePathStub{path: artifact}
	cleanupPreprocessedSource(stub, filepath.Join(tmpRoot, "source.mp4"))

	_, err := os.Stat(artifact)
	assert.True(t, os.IsNotExist(err), "ENCV 临时目录里的中间产物应被清理")
}

func TestCleanupPreprocessedSource_NeverRemovesUserFile(t *testing.T) {
	tmpRoot := t.TempDir()
	userFile := filepath.Join(tmpRoot, "important.mp4")
	require.NoError(t, os.WriteFile(userFile, []byte("x"), 0o644))

	cases := map[string]string{
		"输入文件本身":      userFile,
		"非 encv 临时目录": filepath.Join(tmpRoot, "elsewhere", "pre.m3u8"),
	}
	for name, path := range cases {
		t.Run(name, func(t *testing.T) {
			cleanupPreprocessedSource(&sourcePathStub{path: path}, userFile)
			if path == userFile {
				_, err := os.Stat(userFile)
				assert.NoError(t, err, "绝不能删除输入文件本身")
			}
		})
	}
}

func TestCleanupPreprocessedSource_SkipsPluginWithoutCapability(t *testing.T) {
	assert.NotPanics(t, func() { cleanupPreprocessedSource(&basePlugin{name: "plain"}, "/x") })
}

// TestVideoPlugin_ImplementsExpectedCapabilities 锁住「video 插件仍然具备这些能力」：
// 调度层不再 import video 包，能力如果哪天被误删，这里会红。
func TestVideoPlugin_ImplementsExpectedCapabilities(t *testing.T) {
	var vp Plugin
	for _, p := range Plugins {
		if p.Name() == "video" {
			vp = p
			break
		}
	}
	require.NotNil(t, vp, "应能在注册表里找到 video 插件")

	_, isOutputDir := vp.(interface{ SetOutputDir(string) })
	_, isVerify := vp.(interface{ SetPostEncryptVerify(bool) })
	_, isSource := vp.(interface{ EncryptedSourcePath() string })

	assert.True(t, isOutputDir, "video 插件应实现 SetOutputDir")
	assert.True(t, isVerify, "video 插件应实现 SetPostEncryptVerify")
	assert.True(t, isSource, "video 插件应实现 EncryptedSourcePath")

	// 卸载能力：video 是唯一会产生临时目录的插件，它必须参与统一回收，
	// 否则 DisposePlugins 就成了空转。
	_, isDisposable := vp.(Disposable)
	assert.True(t, isDisposable, "video 插件应实现 Disposable（Dispose() error）")
}

// TestRegistry_DoesNotSpecialCaseConcretePlugins 是「无特权核心」的守卫：
// registry.go 一旦再次对某个具体插件类型做类型断言/分支，这条测试立刻红。
//
// 判据刻意只针对「类型断言与分支」而不禁止类型名本身：
// Plugins 装配列表里出现 &video.VideoPlugin{} 是合法的（装配 != 特判），
// 真正要消灭的是调度逻辑里 `if vp, ok := plugin.(*video.VideoPlugin)` 这种写法。
func TestRegistry_DoesNotSpecialCaseConcretePlugins(t *testing.T) {
	src, err := os.ReadFile("registry.go")
	require.NoError(t, err)

	pluginPkgs := []string{"video", "audio", "image", "text", "pdf", "wps", "alistencrypt"}
	for _, pkg := range pluginPkgs {
		for _, pattern := range []string{".(*" + pkg + ".", "case *" + pkg + "."} {
			assert.NotContains(t, string(src), pattern,
				"registry.go 不应再对具体插件类型做断言/分支（%s）；请改用 interfaces 里的可选能力接口", pattern)
		}
	}
}
