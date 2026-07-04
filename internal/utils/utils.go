package utils

import (
	"strings"

	"github.com/Soltus/encv-go/internal/config"
)

// 根据后缀名 优先从全局配置中查找 MIME 类型，找不到则返回默认值
// 支持带点和不带点的格式，如 ".mp4" 和 "mp4" 都会处理为 "mp4"
func GetContentType(format string) string {
	// 标准化格式：去掉开头的点，统一转为小写
	normalized := strings.TrimPrefix(strings.ToLower(format), ".")

	if ct, ok := config.ContentTypes[normalized]; ok {
		return ct
	}

	// 如果在全局映射中找不到，返回通用的二进制流类型
	return "application/octet-stream"
}
