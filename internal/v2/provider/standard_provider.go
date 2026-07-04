package provider

import (
	"fmt"
	"io"
	"os"
)

// StandardFileProvider 提供对标准文件系统文件的访问
type StandardFileProvider struct {
	file    *os.File
	size    int64
	name    string
	modTime int64 // 使用时间戳的纳秒表示，方便构建 FileInfo
}

// NewStandardFileProvider 创建一个新的 StandardFileProvider
func NewStandardFileProvider(fullPath string) (*StandardFileProvider, error) {
	file, err := os.Open(fullPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open standard file '%s': %w", fullPath, err)
	}

	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, fmt.Errorf("failed to stat standard file '%s': %w", fullPath, err)
	}

	return &StandardFileProvider{
		file:    file,
		size:    info.Size(),
		name:    info.Name(),
		modTime: info.ModTime().UnixNano(),
	}, nil
}

// --- 实现 FileContentProvider 接口 ---

func (p *StandardFileProvider) GetReader() io.ReadCloser {
	return p.file
}

func (p *StandardFileProvider) GetSeeker() (io.Seeker, bool) {
	// *os.File 本身就实现了 io.Seeker
	return p.file, true
}

func (p *StandardFileProvider) GetSeekerTo() (SeekerTo, bool) {
	// 标准文件不需要 SeekTo
	return nil, false
}

func (p *StandardFileProvider) GetSize() int64 {
	return p.size
}

func (p *StandardFileProvider) GetName() string {
	return p.name
}

func (p *StandardFileProvider) Close() error {
	return p.file.Close()
}
