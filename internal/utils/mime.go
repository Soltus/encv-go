package utils

import (
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/Soltus/encv-go/internal/config"
)

// 根据 URL 文件扩展名获取 Content-Type
func GetContentTypeFromExtension(fileURL string) string {
	ext := strings.ToLower(filepath.Ext(fileURL))
	if len(ext) > 0 {
		ext = ext[1:]
	}
	if ct, ok := config.ContentTypes[ext]; ok {
		return ct
	}
	return "application/octet-stream"
}

// DetectFileMIMEType 检测文件的 MIME 类型
// 优先使用文件头嗅探，如果失败则回退到扩展名检测
func DetectFileMIMEType(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to open file for MIME detection: %w", err)
	}
	defer file.Close()

	// 1. 优先使用文件头嗅探
	buffer := make([]byte, 512)
	bytesRead, err := file.Read(buffer)
	if err != nil && err != io.EOF {
		return "", fmt.Errorf("failed to read file header for MIME detection: %w", err)
	}

	mimeType := http.DetectContentType(buffer[:bytesRead])

	// 2. 【关键修复】如果嗅探结果是通用类型，则回退到扩展名检测
	if mimeType == "application/octet-stream" {
		ext := strings.ToLower(filepath.Ext(filePath))
		if len(ext) > 0 {
			// 尝试从 mime 标准库中查找
			mimeType = mime.TypeByExtension(ext)
			// 如果标准库没有，再从我们的配置中查找
			if mimeType == "" {
				if ct, ok := config.ContentTypes[ext[1:]]; ok {
					mimeType = ct
				}
			}
		}
	}

	return mimeType, nil
}
