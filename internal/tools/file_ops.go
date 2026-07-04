// Stage 10 (borrow-nuclear-boy-2026q2)：FileOperations 路径安全 + 搜索跳过隐藏目录。
//
// 借鉴自 /tmp/nuclear-boy/file/.../FileOperations.kt L386-405。
//
// 关键模式：
//   - ResolvePath 防路径穿越（canonical + 前缀检查）
//   - SkipDirs 跳过 .git / node_modules / __pycache__ / build / .gradle
package tools

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// DefaultSkipDirs 搜索时跳过的隐藏/构建目录（对应 nuclear-boy L280-296）。
var DefaultSkipDirs = []string{
	".git",
	"node_modules",
	"__pycache__",
	".gradle",
	"build",
	"dist",
	".next",
	".nuxt",
	"target",
	".venv",
	"venv",
	"env",
	".idea",
	".vscode",
	"vendor",
	".terraform",
}

// ResolvePath 把相对路径解析为绝对路径，校验必须在 rootDir 之内。
//
// 防路径穿越：所有 ../ 必须在 canonical 化后仍以 rootDir 为前缀。
//
// 借鉴 nuclear-boy FileOperations.kt L386-405 resolvePath。
func ResolvePath(rootDir, userPath string) (string, error) {
	// 1. clean 路径（消除 ../ 和 ./）
	cleaned := filepath.Clean(userPath)

	// 2. 拒绝绝对路径（除非 rootDir 也用绝对）
	if filepath.IsAbs(cleaned) && !strings.HasPrefix(cleaned, rootDir) {
		return "", fmt.Errorf("absolute path outside rootDir: %s", cleaned)
	}

	// 3. 拼接 rootDir + userPath
	full := filepath.Join(rootDir, cleaned)

	// 4. canonical 化
	absFull, err := filepath.Abs(full)
	if err != nil {
		return "", fmt.Errorf("abs path: %w", err)
	}

	// 5. 必须以 canonical rootDir 为前缀
	cleanRoot, err := filepath.Abs(rootDir)
	if err != nil {
		return "", fmt.Errorf("abs root: %w", err)
	}
	// 在比较时确保末尾分隔符
	cleanRootWithSep := cleanRoot + string(os.PathSeparator)
	absFullWithSep := absFull + string(os.PathSeparator)
	if !strings.HasPrefix(absFullWithSep, cleanRootWithSep) && absFull != cleanRoot {
		return "", fmt.Errorf("path traversal blocked: %s resolves to %s, outside %s", userPath, absFull, cleanRoot)
	}

	return absFull, nil
}

// ShouldSkipDir 是否在 skip 列表中（用于 searchFiles 遍历）。
func ShouldSkipDir(name string, skipDirs []string) bool {
	for _, s := range skipDirs {
		if name == s {
			return true
		}
	}
	// 隐藏目录（点开头）默认也跳过
	if strings.HasPrefix(name, ".") && name != "." && name != ".." {
		return true
	}
	return false
}

// SearchFiles 在 rootDir 下递归搜索文件名匹配 pattern 的文件，跳过 skipDirs。
//
// 借鉴 nuclear-boy FileOperations.kt L280-296。
func SearchFiles(rootDir, pattern string, skipDirs []string) ([]string, error) {
	if skipDirs == nil {
		skipDirs = DefaultSkipDirs
	}
	var out []string
	pattern = strings.ToLower(pattern)
	err := filepath.WalkDir(rootDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if ShouldSkipDir(d.Name(), skipDirs) {
				return filepath.SkipDir
			}
			return nil
		}
		if pattern == "" || strings.Contains(strings.ToLower(d.Name()), pattern) {
			rel, _ := filepath.Rel(rootDir, path)
			out = append(out, rel)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk dir: %w", err)
	}
	return out, nil
}
