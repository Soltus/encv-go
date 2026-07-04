// Stage 7 (borrow-nuclear-boy-2026q2)：safeUnzip 防止 ZIP-slip 越界。
//
// 借鉴自 /tmp/nuclear-boy/skills/src/main/java/com/nuclearboy/skills/SkillManager.kt L867-873。
//
// 攻击场景：恶意 zip 含 ../../etc/passwd 之类路径，解压到任意位置。
// 防护：canonical 化目标目录 + 校验每个 entry 的目标路径必须在 dest 之下。
package skills

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// SafeUnzip 安全解压 zip 文件到 destDir，防止 ZIP-slip。
//
// 错误返回：解压失败 / 越界 / 符号链接。
func SafeUnzip(zipPath, destDir string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("open zip: %w", err)
	}
	defer r.Close()

	// canonical 化目标目录（绝对路径 + Clean + 末尾分隔符）
	destCanonical, err := filepath.Abs(destDir)
	if err != nil {
		return fmt.Errorf("abs dest: %w", err)
	}
	destCanonical = filepath.Clean(destCanonical) + string(os.PathSeparator)

	// 确保 dest 存在
	if err := os.MkdirAll(destCanonical, 0o755); err != nil {
		return fmt.Errorf("mkdir dest: %w", err)
	}

	for _, f := range r.File {
		// 拒绝符号链接（防符号链接攻击）
		if f.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("ZIP entry %s is a symlink, refusing for security", f.Name)
		}

		// 关键检查：目标路径必须在 dest 之下
		target := filepath.Join(destDir, f.Name)
		targetCanonical := filepath.Clean(target) + string(os.PathSeparator)
		if !strings.HasPrefix(targetCanonical, destCanonical) && targetCanonical != strings.TrimSuffix(destCanonical, string(os.PathSeparator)) {
			return fmt.Errorf("ZIP-slip blocked: entry %q would extract outside dest to %q", f.Name, target)
		}

		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return fmt.Errorf("mkdir %s: %w", target, err)
			}
			continue
		}

		// 确保父目录存在
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return fmt.Errorf("mkdir parent of %s: %w", target, err)
		}

		if err := extractFile(f, target); err != nil {
			return err
		}
	}
	return nil
}

// extractFile 复制单个 zip entry 到目标路径。
func extractFile(f *zip.File, target string) error {
	in, err := f.Open()
	if err != nil {
		return fmt.Errorf("open zip entry %s: %w", f.Name, err)
	}
	defer in.Close()

	out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode()&0o777)
	if err != nil {
		return fmt.Errorf("create %s: %w", target, err)
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return fmt.Errorf("copy to %s: %w", target, err)
	}
	return nil
}
