package utils

import (
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
)

func DecodePathParam(raw string) string {
	s, err := url.QueryUnescape(raw)
	if err != nil {
		return raw
	}
	s2, err := url.QueryUnescape(s)
	if err != nil {
		return s
	}
	return s2
}

// DecodeGinQueryParam 解码 Gin query 参数中的路径。
//
// 编码契约（前端 proxySafeEncode = 双重 encodeURIComponent）：
//  1. 前端: encodeURIComponent(encodeURIComponent(raw_path))
//  2. Gin c.Query() 自动反向解码一层（%252B → %2B）
//  3. 本函数用 url.PathUnescape 反向解码第二层（%2B → +）
//  4. 结果 = 原始路径 ✅
//
// 注意：必须使用 PathUnescape 而非 QueryUnescape，
// 因为 QueryUnescape 会将 '+' 解码为空格（HTML form 规范），
// 但文件名中的 '+' 是合法字符（如 hyYGPCwJPQ3+xrdAvfnn2.bin）。
func DecodeGinQueryParam(raw string) string {
	s, err := url.PathUnescape(raw)
	if err != nil {
		return raw
	}
	return s
}

// SafeResolveToAbsPath 将用户提供的本地路径安全地解析为位于基础目录内的绝对路径。
// 这个函数专门处理文件系统路径，不处理URL编码，适用于内部调用。
//
// 参数:
//   - baseDir: 安全的根目录（已确保是绝对路径）
//   - userPath: 用户提供的路径（可以是相对路径如 "/foo/bar.bin"，
//     也可以是绝对路径如 "/storage/emulated/0/foo/bar.bin"——只要它在 baseDir 下）
//
// 返回值:
//   - string: 解析后的安全绝对路径
//   - error: 如果路径不安全或解析失败，则返回错误
func SafeResolveToAbsPath(baseDir, userPath string) (string, error) {
	// 确保baseDir是绝对路径且已清理
	absBaseDir := filepath.Clean(baseDir)
	if !filepath.IsAbs(absBaseDir) {
		var err error
		absBaseDir, err = filepath.Abs(absBaseDir)
		if err != nil {
			return "", fmt.Errorf("invalid base directory: %w", err)
		}
	}

	// ★ 关键修复 ★: 防止路径双重拼接
	//
	// 当用户传入的路径以 / 开头时，Go 的 filepath.Join 会把它当作绝对路径
	// 直接使用，导致 servingDir + userPath = 双前缀：
	//   servingDir=/storage/emulated/0
	//   userPath=/storage/emulated/0/hyYGPCwJPQ3+xrdAvfnn2.bin
	//   → filepath.Join → /storage/emulated/0/storage/emulated/0/...  ❌
	//
	// 修复策略（在 Clean 之前检查，避免丢失前缀信息）：
	//   - 如果 userPath == absBaseDir：原样返回
	//   - 如果 userPath 以 absBaseDir + separator 开头：剥掉前缀再 join（消除双前缀）
	//   - 其他情况：当作相对路径处理
	//
	// 安全说明: filepath.Clean 之后的最终安全检查（rel 必须不以 .. 开头）
	// 仍然保留，能阻止路径穿越。
	if userPath == absBaseDir {
		return absBaseDir, nil
	}
	baseWithSep := absBaseDir + string(os.PathSeparator)
	if strings.HasPrefix(userPath, baseWithSep) {
		// ★ 在 baseDir 下的绝对路径：剥掉前缀规范化
		userPath = strings.TrimPrefix(userPath, baseWithSep)
	}

	// 清理用户路径（处理多余的./等）
	cleanUserPath := filepath.Clean(userPath)

	// 去除前导/防止 filepath.Join 将其视为绝对路径
	cleanUserPath = strings.TrimPrefix(cleanUserPath, string(os.PathSeparator))
	if cleanUserPath == "" {
		cleanUserPath = "."
	}

	// 构建完整路径
	fullPath := filepath.Join(absBaseDir, cleanUserPath)
	finalPath := filepath.Clean(fullPath)

	// 使用filepath.Rel进行最终安全检查
	rel, err := filepath.Rel(absBaseDir, finalPath)
	if err != nil {
		return "", fmt.Errorf("security check failed: %w", err)
	}

	// 防止路径遍历
	if strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("forbidden: path traversal detected")
	}

	return finalPath, nil
}

// SafeURLPathToRelative 将URL路径安全地转换为文件系统相对路径。
// 此函数会正确处理URL编码，将路径转换为文件系统可用的格式。
//
// 参数:
//   - urlPath: URL原始路径（以/开头），例如 "/folder/%E4%B8%AD%E6%96%87.txt"
//
// 返回值:
//   - string: 解码并清理后的相对路径，例如 "folder/中文.txt"
//   - error: 如果URL无效或包含危险内容，则返回错误
func SafeURLPathToRelative(urlPath string) (string, error) {
	decodedPath := urlPath
	if decodedPath == "" {
		decodedPath = "/"
	}

	decodedPath = DecodePathParam(decodedPath)

	if decodedPath == "" {
		decodedPath = "/"
	}

	// 使用path包处理URL路径（注意：path包使用正斜杠）
	cleanPath := path.Clean(decodedPath)

	// 确保路径以斜杠开头（规范化）
	if !strings.HasPrefix(cleanPath, "/") {
		cleanPath = "/" + cleanPath
	}

	// 安全检查：防止空字节注入
	if strings.Contains(cleanPath, "\x00") {
		return "", fmt.Errorf("invalid path: null byte detected")
	}

	// 移除开头的斜杠得到相对路径
	relativePath := strings.TrimPrefix(cleanPath, "/")

	return relativePath, nil
}

// SafeURLToAbsPath 安全地将URL路径转换为位于基础目录内的绝对文件系统路径。
// 这是SafeURLPathToRelative和SafeResolveToAbsPath的组合，是处理HTTP请求路径的推荐方法。
func SafeURLToAbsPath(baseDir, urlPath string) (string, error) {
	// 1. URL路径 -> 相对路径 (会进行URL解码)
	relativePath, err := SafeURLPathToRelative(urlPath)
	if err != nil {
		return "", fmt.Errorf("URL validation failed: %w", err)
	}

	// 2. 相对路径 -> 绝对路径 (不进行URL解码)
	absPath, err := SafeResolveToAbsPath(baseDir, relativePath)
	if err != nil {
		return "", fmt.Errorf("path resolution failed: %w", err)
	}

	return absPath, nil
}

// BuildURLPath 构建安全的 URL 路径。
// 注意：此函数不执行URL编码，调用者有责任在最终输出到HTTP响应前进行编码。
//
// 参数:
//   - forwardedPrefix: 代理前缀，例如 "/my-app"
//   - basePath: 基础路径，例如 "/files/docs"
//   - fileName: 文件或目录名，例如 "中文图片.jpg"
//   - isDir: 指示fileName是否为目录
//
// 返回值:
//   - string: 构建好的URL路径，例如 "/my-app/files/docs/中文图片.jpg/"
func BuildURLPath(forwardedPrefix, basePath, fileName string, isDir bool) string {
	// 清理所有输入
	forwardedPrefix = strings.TrimSuffix(forwardedPrefix, "/")
	basePath = path.Clean(basePath)

	// 如果 basePath 是根目录
	if basePath == "/" || basePath == "." {
		basePath = "/"
	} else if !strings.HasSuffix(basePath, "/") {
		basePath += "/"
	}

	// 构建完整路径
	var fullPath string
	if basePath == "/" {
		fullPath = "/" + fileName
	} else {
		fullPath = basePath + fileName
	}

	// 如果是目录，添加斜杠
	if isDir && !strings.HasSuffix(fullPath, "/") {
		fullPath += "/"
	}

	// 添加代理前缀
	if forwardedPrefix != "" {
		// 确保 forwardedPrefix 不以斜杠结尾
		forwardedPrefix = strings.TrimSuffix(forwardedPrefix, "/")
		fullPath = forwardedPrefix + fullPath
	}

	return fullPath
}
