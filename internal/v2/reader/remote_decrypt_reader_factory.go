package reader

import (
	"fmt"
	"sync"

	"github.com/Soltus/encv-go/internal/v2/types"
)

// RemoteDecryptReaderFactory 是 DecryptReaderFactory 的远程适配器实现
type RemoteDecryptReaderFactory struct {
	mu sync.RWMutex
	// 【关键修复】不再持有共享的 containerReader，而是持有创建它所需的参数
	containerURL string
	password     string
	headers      map[string][]string
	urlResolver  URLResolver

	// 缓存解析结果，避免重复请求
	cachedIndex  types.Index
	isSeekable   bool
	originalSize int64
}

// NewRemoteDecryptReaderFactory 创建一个新的远程工厂
func NewRemoteDecryptReaderFactory(containerURL string, password string, headers map[string][]string, urlResolver URLResolver) (DecryptReaderFactory, error) {
	f := &RemoteDecryptReaderFactory{
		containerURL: containerURL,
		password:     password,
		headers:      headers,
		urlResolver:  urlResolver,
	}

	// 在创建时就解析并缓存元数据
	if err := f.parseAndCacheMetadata(); err != nil {
		return nil, fmt.Errorf("failed to initialize remote factory: %w", err)
	}

	return f, nil
}

// parseAndCacheMetadata 创建一个临时的 reader 来获取元数据，然后立即关闭它
func (f *RemoteDecryptReaderFactory) parseAndCacheMetadata() error {
	// 【关键修复】创建一个临时的、一次性的 reader 来获取元数据
	tempCr, err := NewRemoteEncryptedContainerReader(f.containerURL, f.headers, f.urlResolver)
	if err != nil {
		return err
	}
	defer tempCr.Close() // 立即关闭，不持有连接

	kviProvider, err := tempCr.GetKVIProvider()
	if err != nil {
		return err
	}
	f.cachedIndex = kviProvider.GetIndex()
	f.originalSize = f.cachedIndex.GetOriginalFileSize()

	// 判断是否可寻址
	f.isSeekable = false
	for _, frag := range tempCr.GetFragments() {
		if frag.Type == types.FragmentType_SeekableStream {
			f.isSeekable = true
			break
		}
	}
	return nil
}

// NewDecryptReader 使用缓存的数据高效地创建解密器
func (f *RemoteDecryptReaderFactory) NewDecryptReader() (DecryptReader, error) {
	// 【关键修复】每次调用都创建一个全新的、独立的 containerReader
	containerReader, err := NewRemoteEncryptedContainerReader(f.containerURL, f.headers, f.urlResolver)
	if err != nil {
		return nil, err
	}

	var decryptReader DecryptReader
	if f.isSeekable {
		decryptReader, err = NewVirtualSeekableDecryptReader(containerReader, f.password)
	} else {
		decryptReader, err = NewSequentialDecryptReader(containerReader, f.password)
	}

	if err != nil {
		containerReader.Close()
		return nil, err
	}

	// 【关键修复】返回一个包装器，确保关闭时能关闭底层的 containerReader
	return &managedRemoteDecryptReader{
		DecryptReader:   decryptReader,
		containerReader: containerReader,
	}, nil
}

// managedRemoteDecryptReader 确保资源被正确关闭
type managedRemoteDecryptReader struct {
	DecryptReader
	containerReader EncryptedContainerReader
}

func (m *managedRemoteDecryptReader) Close() error {
	err1 := m.DecryptReader.Close()
	err2 := m.containerReader.Close()
	if err1 != nil {
		return fmt.Errorf("decrypt reader close failed: %w", err1)
	}
	if err2 != nil {
		return fmt.Errorf("container reader close failed: %w", err2)
	}
	return nil
}

// ... (GetIndex, GetOriginalSize, GetContainerPath, IsSeekable, Close 方法保持不变) ...
func (f *RemoteDecryptReaderFactory) NewBulkDecryptor() (*BulkDecryptor, error) {
	return nil, fmt.Errorf("BulkDecryptor is not supported for remote containers")
}

func (f *RemoteDecryptReaderFactory) GetIndex() types.Index {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.cachedIndex
}

func (f *RemoteDecryptReaderFactory) GetOriginalSize() int64 {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.originalSize
}

func (f *RemoteDecryptReaderFactory) GetContainerPath() string {
	return f.containerURL
}

func (f *RemoteDecryptReaderFactory) IsSeekable() bool {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.isSeekable
}

func (f *RemoteDecryptReaderFactory) Close() error {
	// 远程工厂本身不持有持久连接，无需关闭
	return nil
}
