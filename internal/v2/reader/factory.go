package reader

import (
	"encoding/base64"
	"fmt"
	"log/slog"
	"sync"

	"github.com/Soltus/encv-go/internal/v2/container/fragment"
	"github.com/Soltus/encv-go/internal/v2/container/handle"
	"github.com/Soltus/encv-go/internal/v2/crypto"
	"github.com/Soltus/encv-go/internal/v2/types"
)

// DecryptReaderFactory 是一个通用的工厂接口，用于创建解密后的读取器
// 它负责解析容器元数据一次，并缓存结果，以高效地创建多个独立的解密流实例。
type DecryptReaderFactory interface {
	// NewDecryptReader 创建一个全新的、状态独立的解密流。
	// 每次调用返回的实例都可以安全地并发使用。
	NewDecryptReader() (DecryptReader, error)
	// 【关键新增】创建一个专用于全量解密的工具
	// 返回的实例已预先配置好所有必要信息
	NewBulkDecryptor() (*BulkDecryptor, error)
	// GetIndex 返回容器的文件索引
	GetIndex() types.Index
	// GetOriginalSize 返回原始文件大小
	GetOriginalSize() int64
	// GetContainerPath 返回工厂关联的容器路径，用于上层缓存
	GetContainerPath() string
	// IsSeekable 返回容器是否支持随机寻址。
	IsSeekable() bool
	// Close 关闭工厂持有的所有底层资源（如打开的容器文件）
	Close() error
}

// decryptReaderFactory 是 DecryptReaderFactory 的具体实现。
type decryptReaderFactory struct {
	containerPath string
	password      string

	// 缓存解析结果，避免重复读取文件
	mu                  sync.RWMutex
	cachedManifest      *types.Manifest
	cachedHeaderVersion int
	cachedIndex         types.Index
	kviProvider         types.KVIProvider
	physicalOffsets     map[string]uint64
	seekableIndex       *fragmentRangeIndex
	isSeekable          bool
}

// decryptReaderFactory 实现新方法
func (f *decryptReaderFactory) NewBulkDecryptor() (*BulkDecryptor, error) {
	// 工厂直接将持有的“秘密”注入给 BulkDecryptor
	return NewBulkDecryptor(f.containerPath, f.password), nil
}

// NewDecryptReaderFactory 是创建 DecryptReaderFactory 的唯一入口
// 它会在创建时执行所有昂贵的一次性操作，并缓存结果。
func NewDecryptReaderFactory(containerPath, password string) (DecryptReaderFactory, error) {
	f := &decryptReaderFactory{
		containerPath:   containerPath,
		password:        password,
		physicalOffsets: make(map[string]uint64),
	}

	// 在创建时就解析并缓存元数据
	if err := f.parseAndCacheMetadata(); err != nil {
		return nil, fmt.Errorf("failed to initialize factory: %w", err)
	}

	if f.cachedHeaderVersion == 4 {
		if err := f.verifyPasswordHint(); err != nil {
			return nil, err
		}
	}

	return f, nil
}

// parseAndCacheMetadata 解析容器文件并缓存关键元数据。
func (f *decryptReaderFactory) parseAndCacheMetadata() error {
	// 创建一个临时的 reader 来解析元数据
	tempReader, err := NewEncryptedContainerReaderFromFile(f.containerPath)
	if err != nil {
		return err
	}
	defer tempReader.Close()

	// 从临时的 reader 中提取所有需要缓存的数据
	f.cachedManifest = tempReader.GetManifest()
	f.kviProvider, err = types.NewKVIProviderFromManifest(f.cachedManifest)
	if err != nil {
		return err
	}
	err = fragment.ValidateGlobalStartOffsets(f.cachedManifest)
	if err != nil {
		return err
	}

	// 为了获取扫描结果，我们需要访问内部字段。
	// 因为 decryptReaderFactory 和 fileContainerReader 在同一个包内，所以这是合法的。
	fcr, ok := tempReader.(*fileContainerReader)
	if !ok {
		return fmt.Errorf("internal error: expected *fileContainerReader, got %T", tempReader)
	}
	f.physicalOffsets = fcr.physicalOffsets
	// 【关键新增】同样缓存 Header Version，无需再次探测
	f.cachedHeaderVersion = fcr.headerVersion

	seekableFragments := filterFragmentsByType(f.cachedManifest.Fragments, string(types.FragmentType_SeekableStream))
	if len(seekableFragments) > 0 {
		f.seekableIndex = newFragmentRangeIndex(seekableFragments)
		f.isSeekable = true
	}

	return nil
}

func (f *decryptReaderFactory) verifyPasswordHint() error {
	src, err := handle.NewFileSource(f.containerPath)
	if err != nil {
		return fmt.Errorf("failed to open container for password hint verification: %w", err)
	}
	defer src.Close()

	h, err := handle.Open(src)
	if err != nil {
		return fmt.Errorf("failed to open container handle: %w", err)
	}
	defer h.Close()

	hdr := h.HeaderV4()
	if hdr.PasswordHint == [16]byte{} {
		return nil
	}

	saltBase64 := f.kviProvider.GetEncryptionInfo().SaltBase64
	salt, err := base64.StdEncoding.DecodeString(saltBase64)
	if err != nil {
		return fmt.Errorf("failed to decode salt for password hint verification: %w", err)
	}

	if !crypto.VerifyPasswordHint(hdr.PasswordHint, f.password, salt) {
		return fmt.Errorf("%w: password hint verification failed", types.ErrWrongPassword)
	}

	return nil
}

// NewDecryptReader 使用缓存的数据高效地创建解密器
func (f *decryptReaderFactory) NewDecryptReader() (DecryptReader, error) {
	// 【关键】使用新的轻量级构造函数，直接使用缓存好的数据，避免重复扫描
	containerReader, err := NewFileContainerReaderFromMetadata(f.containerPath, f.cachedManifest, f.cachedHeaderVersion, f.physicalOffsets)
	if err != nil {
		return nil, err
	}

	var decryptReader DecryptReader
	// 【关键修复】对于解密操作（顺序读取），使用更简单的 SequentialSeekableDecryptReader
	// 避免 VirtualSeekableDecryptReader 中复杂的 seek 逻辑导致的错误
	if f.isSeekable {
		decryptReader, err = newSequentialSeekableDecryptReader(containerReader, f.password, f.seekableIndex)
	} else {
		decryptReader, err = NewSequentialDecryptReader(containerReader, f.password)
	}

	if err != nil {
		containerReader.Close()
		return nil, err
	}

	return decryptReader, nil
}

// GetIndex, GetOriginalSize, IsSeekable 方法直接返回缓存的数据
func (f *decryptReaderFactory) GetIndex() types.Index {
	if f.kviProvider == nil {
		// 记录严重错误，但返回一个安全的、无操作的实现，避免上层 panic
		slog.Error("kviProvider is nil in GetIndex", "container", f.containerPath)
		return &types.NoOpIndex{} // 假设你有一个 NoOpIndex 实现
	}
	return f.kviProvider.GetIndex()
}

// GetOriginalSize 返回原始文件大小
func (f *decryptReaderFactory) GetOriginalSize() int64 {
	if f.kviProvider == nil {
		slog.Error("kviProvider is nil in GetOriginalSize", "container", f.containerPath)
		return 0 // 返回 0 是一个安全的默认值
	}
	return f.kviProvider.GetIndex().GetOriginalFileSize()
}

func (f *decryptReaderFactory) GetContainerPath() string {
	return f.containerPath
}

func (f *decryptReaderFactory) IsSeekable() bool {
	return f.isSeekable
}

func (f *decryptReaderFactory) Close() error {
	// 工厂本身不持有文件句柄，只需清理缓存
	f.mu.Lock()
	f.cachedManifest = nil
	f.cachedIndex = nil
	f.mu.Unlock()
	return nil
}
