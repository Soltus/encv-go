package utils

import (
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// 通过 Windows 的 assoc 和 ftype 命令查找默认应用程序的路径。
func GetDefaultAppPath(filePath string) (string, error) {
	ext := filepath.Ext(filePath)
	if ext == "" {
		return "", errors.New("file has no extension")
	}

	// 1. 获取文件类型 (例如 .mp4 -> mp4file)
	cmd1 := exec.Command("cmd", "/c", "assoc", ext)
	output1, err := cmd1.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get file association for '%s': %w", ext, err)
	}
	parts := strings.SplitN(string(output1), "=", 2)
	if len(parts) != 2 || strings.TrimSpace(parts[1]) == "" {
		return "", fmt.Errorf("no file type associated with extension '%s'", ext)
	}
	fileType := strings.TrimSpace(parts[1])

	// 2. 获取打开命令 (例如 mp4file -> "C:\...\vlc.exe" "%1")
	cmd2 := exec.Command("cmd", "/c", "ftype", fileType)
	output2, err := cmd2.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get open command for '%s': %w", fileType, err)
	}
	parts = strings.SplitN(string(output2), "=", 2)
	if len(parts) != 2 || strings.TrimSpace(parts[1]) == "" {
		return "", fmt.Errorf("no open command for file type '%s'", fileType)
	}
	openCommand := strings.TrimSpace(parts[1])

	// 3. 解析出可执行文件路径
	// openCommand 通常是这样的: "C:\Program Files\App\app.exe" "%1"
	re := regexp.MustCompile(`"([^"]+)"`)
	matches := re.FindStringSubmatch(openCommand)
	if len(matches) >= 2 {
		return matches[1], nil // 返回引号内的路径
	}

	// 如果没有引号，尝试按空格分割，取第一个部分
	fields := strings.Fields(openCommand)
	if len(fields) > 0 {
		return fields[0], nil
	}

	return "", fmt.Errorf("could not parse executable path from command: %s", openCommand)
}
