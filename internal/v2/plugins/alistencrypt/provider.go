package alistencrypt

import (
	"io"

	"github.com/Soltus/encv-go/internal/v2/provider"
)

// AlistEncryptFileProvider 实现 provider.FileContentProvider 接口
// 把 alist-encrypt-go 兼容的解密 reader 包装为统一接口，
// 让 alist_encrypt 走与 v4 容器相同的 ContentHandler.ServeFile 路径。
//
// 范式约定：
//   - alist_encrypt 插件只负责"打开文件 + 解密 + 暴露 reader"
//   - HTTP 协议层（Range/206/Content-Range/Content-Disposition/Content-Type）
//     一律委托给 ContentHandler.ServeFile，避免在插件里重复实现。
type AlistEncryptFileProvider struct {
	reader *SeekableDecryptReader
	size   int64
	name   string
}

// NewAlistEncryptFileProvider 构造 FileContentProvider
// 参数：
//   - reader: Stream() 返回的 *SeekableDecryptReader
//   - size: 明文总大小（来自 Stream() 的 plainSize）
//   - name: 解码后的明文文件名（来自 ConvertShowName）
func NewAlistEncryptFileProvider(reader *SeekableDecryptReader, size int64, name string) *AlistEncryptFileProvider {
	return &AlistEncryptFileProvider{reader: reader, size: size, name: name}
}

func (p *AlistEncryptFileProvider) GetReader() io.ReadCloser {
	return p.reader
}

// GetSeeker 直接返回 DecryptReader 自身的 io.Seeker
// DecryptReader 已实现 Seek(offset, whence)（reader.go:80）
func (p *AlistEncryptFileProvider) GetSeeker() (io.Seeker, bool) {
	if p.reader == nil || p.reader.DecryptReader == nil {
		return nil, false
	}
	return p.reader.DecryptReader, true
}

func (p *AlistEncryptFileProvider) GetSeekerTo() (provider.SeekerTo, bool) {
	return nil, false
}

func (p *AlistEncryptFileProvider) GetSize() int64 {
	return p.size
}

// GetName 返回已解码的明文文件名（如 "CAD放样.mp4"）
// ContentHandler.ServeFile 会基于此自动设置 Content-Disposition 和 Content-Type
func (p *AlistEncryptFileProvider) GetName() string {
	return p.name
}

func (p *AlistEncryptFileProvider) Close() error {
	if p.reader == nil {
		return nil
	}
	return p.reader.Close()
}
