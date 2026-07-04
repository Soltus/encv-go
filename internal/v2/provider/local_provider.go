// internal/v2/provider/local_provider.go
package provider

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"runtime"
	"sync"

	"github.com/Soltus/encv-go/internal/v2/reader"
	"github.com/Soltus/encv-go/internal/v2/types"
)

// LocalFileProvider 提供对本地加密文件的访问
type LocalFileProvider struct {
	mu            sync.Mutex
	decryptReader reader.DecryptReader
	index         types.Index
	originalSize  int64
	originalName  string
	// 持有 factory 的引用，用于查询元信息
	factory reader.DecryptReaderFactory
	// 按需加载的字节切片
	cachedData []byte
	once       sync.Once
	loadErr    error
}

// 【关键修复】定义一个自定义的 ReadCloser，它同时支持 Seek
type cachedReadCloser struct {
	*bytes.Reader
}

func (c *cachedReadCloser) Close() error {
	// bytes.Reader 不需要关闭，所以 Close 是空操作
	return nil
}

// NewLocalFileProvider 创建一个新的 LocalFileProvider
func NewLocalFileProvider(ctx context.Context, factory reader.DecryptReaderFactory, decryptReader reader.DecryptReader) (*LocalFileProvider, error) {
	if factory == nil || decryptReader == nil {
		return nil, fmt.Errorf("factory and decryptReader cannot be nil")
	}

	index := factory.GetIndex()
	if index == nil {
		return nil, fmt.Errorf("factory returned a nil index")
	}

	provider := &LocalFileProvider{
		decryptReader: decryptReader,
		index:         index,
		originalSize:  factory.GetOriginalSize(),
		originalName:  index.GetOriginalFilename(),
		factory:       factory,
	}

	// 【关键重构】使用新的、基于 IsSeekable 的判断逻辑
	shouldCache := provider.shouldCacheInMemory()
	if !shouldCache {
		slog.Debug("File is large or container is seekable, will be streamed", "size", provider.originalSize)
		runtime.SetFinalizer(provider, func(p *LocalFileProvider) {
			p.decryptReader.Close()
		})
		return provider, nil
	}

	runtime.SetFinalizer(provider, func(p *LocalFileProvider) {
		if p.cachedData == nil && p.decryptReader != nil {
			p.decryptReader.Close()
		}
	})

	return provider, nil
}

// --- 实现 FileContentProvider 接口 ---

func (p *LocalFileProvider) GetReader() io.ReadCloser {
	p.mu.Lock()
	defer p.mu.Unlock()

	// 如果已经加载到内存
	if p.cachedData != nil {
		return &cachedReadCloser{Reader: bytes.NewReader(p.cachedData)}
	}

	// 如果加载失败了
	if p.loadErr != nil {
		return nil // 或者返回一个错误包装器
	}

	// 如果需要缓存但还没加载，则触发加载
	if p.shouldCacheInMemory() {
		p.loadIntoMemory()
		if p.loadErr != nil {
			return nil
		}
		return &cachedReadCloser{Reader: bytes.NewReader(p.cachedData)}
	}

	// 对于大文件，返回原始的解密器
	return p.decryptReader
}

func (p *LocalFileProvider) GetSeeker() (io.Seeker, bool) {
	// 如果已经加载到内存
	if p.cachedData != nil {
		return bytes.NewReader(p.cachedData), true
	}

	// 如果加载失败了
	if p.loadErr != nil {
		return nil, false
	}

	// 如果需要缓存但还没加载，触发加载
	if p.shouldCacheInMemory() {
		p.loadIntoMemory()
		if p.loadErr != nil {
			return nil, false
		}
		return bytes.NewReader(p.cachedData), true
	}

	// 对于可寻址文件，检查原始解密器
	if seeker, ok := p.decryptReader.(io.Seeker); ok {
		return seeker, true
	}
	return nil, false
}

func (p *LocalFileProvider) GetSeekerTo() (SeekerTo, bool) {
	// 内存缓存不需要 SeekTo
	if p.cachedData != nil || p.loadErr != nil {
		return nil, false
	}

	// 如果需要缓存但还没加载，触发加载
	if p.shouldCacheInMemory() {
		p.loadIntoMemory()
		return nil, false
	}
	// 对于可寻址文件，检查原始解密器
	if seekerTo, ok := p.decryptReader.(SeekerTo); ok {
		return seekerTo, true
	}
	return nil, false
}

func (p *LocalFileProvider) GetSize() int64 {
	return p.originalSize
}

func (p *LocalFileProvider) GetName() string {
	return p.originalName
}

func (p *LocalFileProvider) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	// 如果使用了内存缓存，原始的 decryptReader 已经在 loadIntoMemory 中关闭
	if p.cachedData != nil {
		p.cachedData = nil // 【关键】释放内存，防止泄露
		return nil
	}

	// 否则，关闭解密器
	if err := p.decryptReader.Close(); err != nil {
		return fmt.Errorf("failed to close decrypt reader: %w", err)
	}
	return nil
}

// shouldCacheInMemory 是一个新增的智能判断方法
func (p *LocalFileProvider) shouldCacheInMemory() bool {
	if p.originalSize <= 0 {
		return false
	}

	// 1. 检查容器是否原生支持 Seek (例如 VirtualSeekableDecryptReader)
	if p.factory.IsSeekable() {
		// 【策略 A】如果容器可寻址：
		// 通常情况下，我们希望直接流式传输，而不是将整个文件读入内存。
		// 例外：对于极小的文件（例如 < 1MB），缓存到内存可能比文件 IO 更快。
		const smallFileThreshold = 3 * 1024 * 1024 // MB
		if p.originalSize <= smallFileThreshold {
			slog.Debug("Seekable but tiny, caching for speed", "size", p.originalSize)
			return true
		}

		// 对于大的可寻址文件（如视频），直接 Streaming。
		// VirtualSeekableDecryptReader 已经优化了 Fragment 的读取。
		slog.Debug("Seekable container detected, streaming to save RAM", "size", p.originalSize)
		return false
	}

	// 2. 检查容器是否不可寻址
	// 【策略 B】如果容器不可寻址（纯流式）：
	// 为了支持 HTTP Range 请求 或 WebDAV 随机读，我们必须将其缓存到内存。
	// 但我们设置一个上限，防止 OOM。
	const unseekableCacheLimit = 150 * 1024 * 1024 // MB
	if p.originalSize <= unseekableCacheLimit {
		slog.Debug("Non-seekable container, caching in memory to enable Seek", "size", p.originalSize)
		return true
	}

	// 文件太大且不可寻址，无法安全地支持 Seek。
	slog.Warn("Non-seekable file is too large, Range requests may fail", "size", p.originalSize)
	return false
}

// 【关键新增】loadIntoMemory 按需将文件读入内存
func (p *LocalFileProvider) loadIntoMemory() {
	p.once.Do(func() {
		slog.Debug("Loading file into memory on first access")
		allData, err := io.ReadAll(p.decryptReader)
		if err != nil {
			p.loadErr = fmt.Errorf("failed to read file into memory: %w", err)
			return
		}

		// 数据读取成功，关闭原始流和工厂
		p.decryptReader.Close()

		p.cachedData = allData
		slog.Debug("File loaded into memory", "size", len(p.cachedData))
	})
}
