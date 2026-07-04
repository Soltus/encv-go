package utils

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// 统一ETag生成格式
func GenETag(modTime time.Time, size int64) string {
	return `"` + modTime.Format(time.RFC3339Nano) + "-" + fmt.Sprintf("%d", size) + `"`
}

// 获取文件大小
func GetFileSize(path string) int64 {
	info, _ := os.Stat(path)
	if info == nil {
		return 0
	}
	return info.Size()
}

// 复制文件
func CopyFile(src, dst string) error {
	source, err := os.Open(src)
	if err != nil {
		return err
	}
	defer source.Close()

	destination, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destination.Close()

	_, err = io.Copy(destination, source)
	return err
}

// 按长度降序排序扩展名
func SortExtensionsByLength(exts []string) []string {
	sorted := make([]string, len(exts))
	copy(sorted, exts)
	sort.Slice(sorted, func(i, j int) bool {
		return len(sorted[i]) > len(sorted[j])
	})
	return sorted
}

// 剥离给定的扩展名，用于视频字幕等场景
func StripKnownExtensions(filename string, exts []string) string {
	name := filename
	for _, ext := range exts {
		if strings.HasSuffix(name, ext) {
			name = strings.TrimSuffix(name, ext)
			break
		}
	}
	return name
}

// GetBaseNameWithoutExt 获取文件名，并移除所有已知的扩展名。
// 例如: "video.mp4" -> "video", "archive.tar.gz" -> "archive"
func GetBaseNameWithoutExt(path string) string {
	filename := filepath.Base(path)
	// 循环移除所有扩展名，直到没有为止
	for {
		ext := filepath.Ext(filename)
		if ext == "" {
			break
		}
		filename = strings.TrimSuffix(filename, ext)
	}
	return filename
}

// GenerateReversedExt 将文件扩展名倒序，例如 "jpg" -> "gpj"
// 用于避免不同扩展名的文件重名
func GenerateReversedExt(ext string) string {
	// 移除可能存在的前导点，例如 ".jpg" -> "jpg"
	if len(ext) > 0 && ext[0] == '.' {
		ext = ext[1:]
	}
	runes := []rune(ext)
	for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
		runes[i], runes[j] = runes[j], runes[i]
	}
	return string(runes)
}

// GenerateEncryptedFilename 根据原始文件名和容器扩展名，生成加密后的文件名
// 例如: originalFilename="321.mp4", containerExt=".sccgv" -> "321.4pm.sccgv"
// 例如: originalFilename="my_archive", containerExt=".sccgv" -> "my_archive.sccgv"
func GenerateEncryptedFilename(originalFilename, containerExt string) string {
	// 1. 获取基础名和原始扩展名
	base := filepath.Base(originalFilename)
	ext := filepath.Ext(base)
	cleanBaseName := strings.TrimSuffix(base, ext)

	// 2. 如果没有原始扩展名，直接附加容器扩展名
	if ext == "" {
		return cleanBaseName + containerExt
	}

	// 3. 倒转原始扩展名
	reversedExt := GenerateReversedExt(ext)

	// 4. 构建最终的文件名: 基础名.倒转原始扩展名.容器扩展名
	return fmt.Sprintf("%s.%s%s", cleanBaseName, reversedExt, containerExt)
}

// IsEncryptedContainer 检查文件是否为有效的 ENCV 主容器文件。
// 它会正确地忽略子分片。
// func IsEncryptedContainer(ctx context.Context, filePath string) (bool, error) {
// 	detectedType, err := container.DetectMainOrSubContainerType(ctx, filePath)
// 	if err != nil {
// 		// 如果不是我们的文件，返回 false 和 nil，而不是错误，以便调用者可以静默跳过
// 		if strings.Contains(err.Error(), "not a recognized") {
// 			return false, nil
// 		}
// 		// 其他读取错误则返回
// 		return false, err
// 	}

// 	// 只有当检测到的是主容器类型（而不是 "sub_chunk"）时，才返回 true
// 	return detectedType != container.SubChunkType, nil
// }

// CreateFileForOutput 安全地创建一个用于输出的文件。
// 如果 force 为 true，它将覆盖已存在的文件。
// 如果 force 为 false，它将创建一个新文件，如果文件名冲突，则自动重命名为 (1), (2)...
// 它返回打开的文件句柄和实际使用的文件路径。
func CreateFileForOutput(path string, force bool) (*os.File, string, error) {
	// 如果强制覆盖，直接尝试创建（这会覆盖同名文件）
	if force {
		file, err := os.Create(path)
		if err != nil {
			return nil, "", fmt.Errorf("failed to force create file '%s': %w", path, err)
		}
		return file, path, nil
	}

	// --- 非强制模式 ---

	// 1. 尝试以独占模式创建文件（如果文件已存在，则会失败）
	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0666)
	if err == nil {
		// 创建成功，文件不存在
		return file, path, nil
	}

	// 2. 如果失败是因为文件已存在，则开始重命名逻辑
	if !os.IsExist(err) {
		// 是其他错误（如权限不足）
		return nil, "", fmt.Errorf("failed to create file '%s': %w", path, err)
	}

	// 3. 分解路径，准备生成新名称
	dir := filepath.Dir(path)
	ext := filepath.Ext(path)
	base := filepath.Base(path)
	name := strings.TrimSuffix(base, ext)

	// 4. 循环查找可用的文件名
	for i := 1; ; i++ {
		newName := fmt.Sprintf("%s(%d)%s", name, i, ext)
		newPath := filepath.Join(dir, newName)

		file, err := os.OpenFile(newPath, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0666)
		if err == nil {
			// 找到一个可用的名字，创建成功
			return file, newPath, nil
		}

		if !os.IsExist(err) {
			// 遇到其他错误，停止尝试
			return nil, "", fmt.Errorf("failed to create file '%s': %w", newPath, err)
		}
		// 如果是文件已存在，继续循环 i++
	}
}
