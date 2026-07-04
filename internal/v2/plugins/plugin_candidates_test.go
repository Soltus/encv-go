package plugins_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Soltus/encv-go/internal/v2/plugins"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func initPluginsForCandidates(t *testing.T) {
	t.Helper()
	initPluginsForTaskOptions(t)
}

func TestFindAllEncryptingPlugins_VideoFile_MimeMatch_P0(t *testing.T) {
	initPluginsForCandidates(t)
	tmpDir := t.TempDir()
	videoPath := filepath.Join(tmpDir, "test.mp4")
	require.NoError(t, os.WriteFile(videoPath, []byte("\x00\x00\x00\x18ftypmp42"), 0644))

	candidates := plugins.FindAllEncryptingPlugins(videoPath)
	assert.NotEmpty(t, candidates, "video file should have at least one candidate")

	videoCand := findCandidateByName(candidates, "video")
	require.NotNil(t, videoCand, "video plugin should be a candidate for .mp4")
	assert.Equal(t, "mime", videoCand.MatchType, "video should match via MIME type")
	assert.Equal(t, 0, videoCand.Priority, "video should be priority 0 (exact)")
}

func TestFindAllEncryptingPlugins_TextFile_P0(t *testing.T) {
	initPluginsForCandidates(t)
	tmpDir := t.TempDir()
	txtPath := filepath.Join(tmpDir, "test.txt")
	require.NoError(t, os.WriteFile(txtPath, []byte("hello world"), 0644))

	candidates := plugins.FindAllEncryptingPlugins(txtPath)
	assert.NotEmpty(t, candidates, "text file should have at least one candidate")

	textCand := findCandidateByName(candidates, "text")
	require.NotNil(t, textCand, "text plugin should be a candidate for .txt")
	assert.Equal(t, 0, textCand.Priority, "text should be priority 0 (exact match)")
	assert.Contains(t, []string{"mime", "extension"}, textCand.MatchType,
		"text should match via MIME or extension (both are P0)")
}

func TestFindAllEncryptingPlugins_ArbitraryFile_GeneralP1(t *testing.T) {
	initPluginsForCandidates(t)
	tmpDir := t.TempDir()
	arbitraryPath := filepath.Join(tmpDir, "data.xyz123")
	require.NoError(t, os.WriteFile(arbitraryPath, []byte("binary data"), 0644))

	candidates := plugins.FindAllEncryptingPlugins(arbitraryPath)

	alistCand := findCandidateByName(candidates, "alist_encrypt")
	require.NotNil(t, alistCand, "alist_encrypt should handle arbitrary files (ShouldProcess=true)")
	assert.Equal(t, "general", alistCand.MatchType, "alist_encrypt should be a general candidate")
	assert.Equal(t, 1, alistCand.Priority, "alist_encrypt should be priority 1 (general)")
}

func TestFindAllEncryptingPlugins_VideoFile_IncludesGeneral_NoDuplication(t *testing.T) {
	initPluginsForCandidates(t)
	tmpDir := t.TempDir()
	videoPath := filepath.Join(tmpDir, "movie.mp4")
	require.NoError(t, os.WriteFile(videoPath, []byte("\x00\x00\x00\x18ftypmp42"), 0644))

	candidates := plugins.FindAllEncryptingPlugins(videoPath)

	videoCand := findCandidateByName(candidates, "video")
	require.NotNil(t, videoCand, "video plugin should be present")
	assert.Equal(t, 0, videoCand.Priority, "video should be P0")

	alistCand := findCandidateByName(candidates, "alist_encrypt")
	require.NotNil(t, alistCand, "alist_encrypt should also be present as general fallback")
	assert.Equal(t, 1, alistCand.Priority, "alist_encrypt should be P1")

	names := make(map[string]bool)
	for _, c := range candidates {
		assert.False(t, names[c.Name], "candidate %q should not appear twice", c.Name)
		names[c.Name] = true
	}
}

func TestFindAllEncryptingPlugins_EmptyPath_OnlyTrueGeneral(t *testing.T) {
	initPluginsForCandidates(t)
	candidates := plugins.FindAllEncryptingPlugins("")

	assert.NotEmpty(t, candidates, "empty path should return general candidates")

	names := candidateNames(candidates)
	for _, c := range candidates {
		assert.Equal(t, "general", c.MatchType,
			"empty path has no MIME/extension, all candidates should be general type")
		assert.Equal(t, 1, c.Priority,
			"empty path candidates should be priority 1 (general)")
	}

	// 阶段 3 过滤后：只有无类型声明的插件（alist_encrypt）才出现
	for _, name := range names {
		if name != "alist_encrypt" {
			t.Errorf("empty path should only return true general plugins (no type declarations), got %q", name)
		}
	}
}

func TestFindAllEncryptingPlugins_NonExistentFile_NoCrash(t *testing.T) {
	initPluginsForCandidates(t)
	assert.NotPanics(t, func() {
		plugins.FindAllEncryptingPlugins("/nonexistent/path/to/file.dat")
	}, "should not crash on non-existent file")
}

// ============================================================
// 阶段 3 过滤测试：只有无 MIME/扩展名声明的插件才作为通用候选
// 核心修复：防止 audio/image/pdf/text 等声明了特定类型的插件被错误标为 general
// ============================================================

func TestStage3_OnlyTrueGeneralPlugins_VideoFile(t *testing.T) {
	initPluginsForCandidates(t)
	tmpDir := t.TempDir()
	videoPath := filepath.Join(tmpDir, "movie.mp4")
	require.NoError(t, os.WriteFile(videoPath, []byte("\x00\x00\x00\x18ftypmp42"), 0644))

	candidates := plugins.FindAllEncryptingPlugins(videoPath)
	names := candidateNames(candidates)

	// video 必须出现（P0 扩展名/MIME 匹配）
	assert.Contains(t, names, "video", "video plugin must match .mp4")

	// alist_encrypt 必须出现（唯一真正的通用插件）
	assert.Contains(t, names, "alist_encrypt", "alist_encrypt must appear as true general")

	// 以下插件声明了特定 MIME 前缀或扩展名，不应出现在阶段 3
	for _, excluded := range []string{"audio", "image", "pdf", "text", "wps"} {
		assert.NotContains(t, names, excluded,
			"%q declares specific types (MIME/extensions), should not be general for .mp4", excluded)
	}
}

func TestStage3_OnlyTrueGeneralPlugins_TextFile(t *testing.T) {
	initPluginsForCandidates(t)
	tmpDir := t.TempDir()
	txtPath := filepath.Join(tmpDir, "doc.txt")
	require.NoError(t, os.WriteFile(txtPath, []byte("plain text content"), 0644))

	candidates := plugins.FindAllEncryptingPlugins(txtPath)
	names := candidateNames(candidates)

	// text 必须出现（P0 扩展名匹配）
	assert.Contains(t, names, "text", "text plugin must match .txt")

	// alist_encrypt 必须出现
	assert.Contains(t, names, "alist_encrypt", "alist_encrypt must appear as true general")

	// 不相关的特定类型插件不应出现
	for _, excluded := range []string{"audio", "video", "image", "wps"} {
		assert.NotContains(t, names, excluded,
			"%q should not be a candidate for .txt", excluded)
	}
}

func TestStage3_OnlyTrueGeneralPlugins_PdfFile(t *testing.T) {
	initPluginsForCandidates(t)
	tmpDir := t.TempDir()
	pdfPath := filepath.Join(tmpDir, "report.pdf")
	require.NoError(t, os.WriteFile(pdfPath, []byte("%PDF-1.4\n"), 0644))

	candidates := plugins.FindAllEncryptingPlugins(pdfPath)
	names := candidateNames(candidates)

	assert.Contains(t, names, "pdf", "pdf plugin must match .pdf")
	assert.Contains(t, names, "alist_encrypt", "alist_encrypt must appear as true general")

	for _, excluded := range []string{"audio", "video", "image", "text", "wps"} {
		assert.NotContains(t, names, excluded,
			"%q should not be a candidate for .pdf", excluded)
	}
}

func TestStage3_OnlyTrueGeneralPlugins_ArbitraryExtension(t *testing.T) {
	initPluginsForCandidates(t)
	tmpDir := t.TempDir()
	arbitraryPath := filepath.Join(tmpDir, "data.xyz123")
	// 使用真实二进制内容，避免被 MIME 检测器识别为 text/plain 等已知类型
	require.NoError(t, os.WriteFile(arbitraryPath, []byte{0xDE, 0xAD, 0xBE, 0xEF, 0xCA, 0xFE, 0xBA, 0xBE, 0x00, 0x00}, 0644))

	candidates := plugins.FindAllEncryptingPlugins(arbitraryPath)
	names := candidateNames(candidates)

	// 无已知扩展名的文件：只有 alist_encrypt（真通用）
	assert.Contains(t, names, "alist_encrypt",
		"alist_encrypt must handle arbitrary files as the only general plugin")

	// 所有声明了类型的插件都不应出现
	typeSpecificPlugins := []string{"audio", "video", "image", "pdf", "text", "wps"}
	for _, name := range typeSpecificPlugins {
		assert.NotContains(t, names, name,
			"%q has type declarations, should not be general for unknown extension", name)
	}

	// 预期结果：只有 alist_encrypt
	assert.Equal(t, []string{"alist_encrypt"}, names,
		"unknown extension should yield exactly one candidate: alist_encrypt")
}

func TestStage3_GeneralCandidateMatchTypeIsCorrect(t *testing.T) {
	initPluginsForCandidates(t)
	tmpDir := t.TempDir()
	videoPath := filepath.Join(tmpDir, "test.mp4")
	require.NoError(t, os.WriteFile(videoPath, []byte("\x00\x00\x00\x18ftypmp42"), 0644))

	candidates := plugins.FindAllEncryptingPlugins(videoPath)

	for i := range candidates {
		c := &candidates[i]
		if c.Name == "alist_encrypt" {
			assert.Equal(t, "general", c.MatchType,
				"alist_encrypt must have matchType=general")
			assert.Equal(t, 1, c.Priority,
				"alist_encrypt must have priority=1 (general)")
		} else if c.Name == "video" {
			assert.Contains(t, []string{"mime", "extension"}, c.MatchType,
				"video must match via mime or extension, not general")
			assert.Equal(t, 0, c.Priority,
				"video must have priority=0 (exact match)")
		} else {
			t.Errorf("unexpected candidate %q with matchType=%s", c.Name, c.MatchType)
		}
	}
}

func candidateNames(candidates []plugins.PluginCandidate) []string {
	names := make([]string, len(candidates))
	for i, c := range candidates {
		names[i] = c.Name
	}
	return names
}

func findCandidateByName(candidates []plugins.PluginCandidate, name string) *plugins.PluginCandidate {
	for i := range candidates {
		if candidates[i].Name == name {
			return &candidates[i]
		}
	}
	return nil
}
