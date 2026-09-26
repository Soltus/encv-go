package video

import (
	"os"
	"path/filepath"
	"testing"

	pluginInterfaces "github.com/Soltus/encv-go/internal/v2/plugins/interfaces"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 编译期断言：video 插件必须实现注册表约定的 Disposable（Dispose() error）。
var _ interface{ Dispose() error } = (*VideoPlugin)(nil)

func TestVideoPlugin_Dispose_RemovesEncvTempArtifacts(t *testing.T) {
	outDir := t.TempDir()
	tmpDir := filepath.Join(outDir, ".encv_tmp")
	verifyDir := filepath.Join(outDir, ".encv_verify_ab12cd34")
	require.NoError(t, os.MkdirAll(tmpDir, 0o755))
	require.NoError(t, os.MkdirAll(verifyDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "pre.mkv"), []byte("x"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(verifyDir, "check.mp4"), []byte("x"), 0o644))

	p := &VideoPlugin{outputDir: outDir}
	require.NoError(t, p.Dispose())

	assert.NoDirExists(t, tmpDir, ".encv_tmp 应被清理")
	assert.NoDirExists(t, verifyDir, ".encv_verify_* 应被清理")
}

func TestVideoPlugin_Dispose_KeepsUserArtifacts(t *testing.T) {
	outDir := t.TempDir()
	container := filepath.Join(outDir, "movie.sccgv")
	require.NoError(t, os.WriteFile(container, []byte("x"), 0o644))
	userDir := filepath.Join(outDir, "subtitles")
	require.NoError(t, os.MkdirAll(userDir, 0o755))

	p := &VideoPlugin{outputDir: outDir}
	require.NoError(t, p.Dispose())

	assert.FileExists(t, container, "用户容器文件绝不能被 Dispose 删掉")
	assert.DirExists(t, userDir, "非 ENCV 临时目录不能被删")
}

func TestVideoPlugin_Dispose_IsIdempotent(t *testing.T) {
	outDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(outDir, ".opencv_tmp"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(outDir, ".encv_tmp"), 0o755))

	p := &VideoPlugin{outputDir: outDir}
	require.NoError(t, p.Dispose())
	require.NoError(t, p.Dispose(), "Dispose 必须可以重复调用")
	assert.NoDirExists(t, filepath.Join(outDir, ".encv_tmp"))
}

func TestVideoPlugin_Dispose_WithoutOutputDir(t *testing.T) {
	p := &VideoPlugin{}
	assert.NotPanics(t, func() { _ = p.Dispose() })
}

func TestVideoPlugin_Dispose_MissingOutputDirIsNotAnError(t *testing.T) {
	p := &VideoPlugin{outputDir: filepath.Join(t.TempDir(), "gone")}
	require.NoError(t, p.Dispose(), "outputDir 不存在不应报错")
}

func TestVideoPlugin_Dispose_ResetsTaskState(t *testing.T) {
	p := &VideoPlugin{
		outputDir:           t.TempDir(),
		inputPath:           "/in/a.mkv",
		inputRootDir:        "/in",
		encryptedSourcePath: "/in/.encv_tmp/pre.mkv",
		splitSets:           [][]string{{"p0", "p1"}},
		splitPartPaths:      map[string]bool{"p0": true},
		isPostEncryptVerify: true,
		index:               VideoIndex{OriginalFilename: "a.mkv"},
	}
	lastVerifyWarnings = []*pluginInterfaces.VerifyWarning{{CheckName: "size"}}

	require.NoError(t, p.Dispose())

	assert.Empty(t, lastVerifyWarnings, "自检警告缓存也应被复位")

	assert.Empty(t, p.inputPath)
	assert.Empty(t, p.inputRootDir)
	assert.Empty(t, p.encryptedSourcePath)
	assert.Nil(t, p.splitSets)
	assert.Nil(t, p.splitPartPaths)
	assert.False(t, p.isPostEncryptVerify)
	assert.Empty(t, p.index.OriginalFilename)
}
