// internal/v2/provider/remote_provider.go
package provider

import (
	"fmt"
	"io"
	"sync"

	"github.com/Soltus/encv-go/internal/v2/reader"
)

// RemoteFileProvider 提供对远程加密容器的访问
type RemoteFileProvider struct {
	mu            sync.Mutex
	factory       reader.DecryptReaderFactory
	decryptReader reader.DecryptReader
	originalSize  int64
	originalName  string
}

// 【关键重构】NewRemoteFileProvider 不再负责创建工厂，而是接收已创建好的实例
// 这使得 provider 包完全解耦，不依赖任何具体的远程实现（如 openlist）
func NewRemoteFileProvider(factory reader.DecryptReaderFactory, decryptReader reader.DecryptReader) (*RemoteFileProvider, error) {
	if factory == nil {
		return nil, fmt.Errorf("factory cannot be nil")
	}
	if decryptReader == nil {
		return nil, fmt.Errorf("decryptReader cannot be nil")
	}

	index := factory.GetIndex()
	if index == nil {
		// 不需要在这里关闭，因为调用方创建失败时应该自己负责清理
		return nil, fmt.Errorf("factory returned a nil index")
	}

	provider := &RemoteFileProvider{
		factory:       factory,
		decryptReader: decryptReader,
		originalSize:  factory.GetOriginalSize(),
		originalName:  index.GetOriginalFilename(),
	}

	return provider, nil
}

// --- 实现 FileContentProvider 接口 ---

// GetReader, GetSeeker, GetSeekerTo, GetSize, GetName 的实现与 LocalFileProvider 几乎完全相同
// 只是它们操作的是 p.factory 和 p.decryptReader

func (p *RemoteFileProvider) GetReader() io.ReadCloser {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.decryptReader
}

func (p *RemoteFileProvider) GetSeeker() (io.Seeker, bool) {
	if seeker, ok := p.decryptReader.(io.Seeker); ok {
		return seeker, true
	}
	return nil, false
}

func (p *RemoteFileProvider) GetSeekerTo() (SeekerTo, bool) {
	if seekerTo, ok := p.decryptReader.(SeekerTo); ok {
		return seekerTo, true
	}
	return nil, false
}

func (p *RemoteFileProvider) GetSize() int64 {
	return p.originalSize
}

func (p *RemoteFileProvider) GetName() string {
	return p.originalName
}

func (p *RemoteFileProvider) Close() error {
	// 对于远程提供者，必须关闭工厂以释放网络连接
	var decryptErr, factoryErr error
	if p.decryptReader != nil {
		decryptErr = p.decryptReader.Close()
	}
	if p.factory != nil {
		factoryErr = p.factory.Close()
	}
	if decryptErr != nil {
		return fmt.Errorf("failed to close decrypt reader: %w", decryptErr)
	}
	if factoryErr != nil {
		return fmt.Errorf("failed to close remote factory: %w", factoryErr)
	}
	return nil
}
