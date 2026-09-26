// internal/v2/plugins/capabilities.go
//
// 调度层与插件之间的「可选能力」适配。
//
// 目的：registry 不再需要 import 具体插件包、不再对 `(*video.VideoPlugin)` 做类型特判。
// 调度层只问「你有没有这个能力」，有就调用，没有就跳过——插件之间完全对等，
// 新增一个需要同样能力的插件时，调度层零改动。

package plugins

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	pluginInterfaces "github.com/Soltus/encv-go/internal/v2/plugins/interfaces"
)

// encvTempDirMarker 是 ENCV 自己创建的临时目录标记。
// 只有落在这种目录里的中间产物才会被自动清理，避免误删用户文件。
const encvTempDirMarker = ".encv_tmp"

// applyOutputDir 若插件声明了 OutputDirSetter，则把本次输出目录注入给它。
func applyOutputDir(p Plugin, outputDir string) {
	if setter, ok := p.(pluginInterfaces.OutputDirSetter); ok {
		setter.SetOutputDir(outputDir)
	}
}

// applyPostEncryptVerify 若插件声明了 PostEncryptVerifySetter，则开启加密后自检。
func applyPostEncryptVerify(p Plugin) {
	if setter, ok := p.(pluginInterfaces.PostEncryptVerifySetter); ok {
		setter.SetPostEncryptVerify(true)
	}
}

// cleanupPreprocessedSource 清理插件预处理产生的中间产物。
//
// 安全边界（三重判断，缺一不可）：
//  1. 插件必须实现 EncryptedSourceProvider 并给出非空路径；
//  2. 该路径不能就是输入文件本身（否则等于删了用户的源文件）；
//  3. 该路径必须位于 ENCV 自己的临时目录（目录名含 .encv_tmp）。
func cleanupPreprocessedSource(p Plugin, inputPath string) {
	provider, ok := p.(pluginInterfaces.EncryptedSourceProvider)
	if !ok {
		return
	}
	sourcePath := provider.EncryptedSourcePath()
	if sourcePath == "" || sourcePath == inputPath {
		return
	}
	if !strings.Contains(filepath.Base(filepath.Dir(sourcePath)), encvTempDirMarker) {
		return
	}
	if err := os.Remove(sourcePath); err != nil {
		slog.Warn("Failed to clean up preprocessed temp file", "path", sourcePath, "error", err)
		return
	}
	slog.Info("Cleaning up preprocessed temp file", "path", sourcePath)
}
