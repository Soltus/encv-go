package reader

import (
	"fmt"
	"io"
	"os"
)

// TempFileReadCloser 包装了 *os.File，在 Close 时仅关闭文件句柄。
// 底层文件的生命周期由调用方管理，支持显式清理。
type TempFileReadCloser struct {
	file *os.File
	path string
}

// NewTempFileReadCloser 是一个构造函数，它打开指定路径的文件并包装成 TempFileReadCloser。
// 如果文件打开失败，它会返回错误。
func NewTempFileReadCloser(filePath string) (io.ReadCloser, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open temp file '%s': %w", filePath, err)
	}
	return &TempFileReadCloser{file: file, path: filePath}, nil
}

// Read 实现了 io.Reader 接口
func (t *TempFileReadCloser) Read(p []byte) (n int, err error) {
	return t.file.Read(p)
}

func (t *TempFileReadCloser) Close() error {
	return t.file.Close()
}

// Name 返回底层文件的路径
func (t *TempFileReadCloser) Name() string {
	return t.path
}
