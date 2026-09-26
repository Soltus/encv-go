package video

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

// ENCV 自己创建的临时目录：预处理中间产物目录，以及加密后自检目录的前缀。
// Dispose 只清理这两类，其余一概不动（用户容器、字幕导出目录等）。
const (
	tempDirName     = ".encv_tmp"
	verifyDirPrefix = ".encv_verify_"
)

// Dispose 回收 video 插件的临时产物并复位任务态，实现 plugins.Disposable。
//
// 为什么需要它：预处理（MKV remux、分片合并）与加密后自检都会在输出目录下
// 留下临时目录，正常路径各自 defer 清理，但异常路径（失败、中断、插件卸载）
// 会残留。注册表现在能在卸载/停机时统一回收，插件不必各自记得清理。
//
// 契约：
//   - 幂等：可重复调用（初始化失败回滚、停机、重载配置都会走到这里）；
//   - 范围最小：只删 ENCV 自建目录，绝不碰用户产物；
//   - 单个目录删除失败不影响其余目录，错误聚合返回。
func (p *VideoPlugin) Dispose() error {
	var errs []error
	if p.outputDir != "" {
		if err := removeEncvTempDirs(p.outputDir); err != nil {
			errs = append(errs, err)
		}
	}
	p.resetTaskState()
	return errors.Join(errs...)
}

// resetTaskState 复位一次任务留下的状态，避免插件实例在下次任务里串味。
func (p *VideoPlugin) resetTaskState() {
	p.inputPath = ""
	p.inputRootDir = ""
	p.encryptedSourcePath = ""
	p.index = VideoIndex{}
	p.splitSets = nil
	p.splitPartPaths = nil
	p.isPostEncryptVerify = false
	lastVerifyWarnings = nil
}

// removeEncvTempDirs 删除 dir 下 ENCV 自建的临时目录（.encv_tmp 与 .encv_verify_*）。
func removeEncvTempDirs(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to scan output dir %s: %w", dir, err)
	}

	var errs []error
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if name != tempDirName && !strings.HasPrefix(name, verifyDirPrefix) {
			continue
		}
		target := filepath.Join(dir, name)
		if err := os.RemoveAll(target); err != nil {
			errs = append(errs, fmt.Errorf("failed to remove temp dir %s: %w", target, err))
			continue
		}
		slog.Info("Disposed video plugin temp dir", "component", "VIDEO_PLUGIN", "path", target)
	}
	return errors.Join(errs...)
}
