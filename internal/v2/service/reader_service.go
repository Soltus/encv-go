package service

import (
	"fmt"
	"log/slog"
	"sync"

	"github.com/Soltus/encv-go/internal/config"
	"github.com/Soltus/encv-go/internal/v2/namer"
	"github.com/Soltus/encv-go/internal/v2/reader"
	"github.com/Soltus/encv-go/internal/v2/types"
)

// ReaderService 是一个高级别的服务，它封装了容器重建和解密的所有复杂性。
// 它是 v2 架构中推荐的、唯一的对外服务入口。
type ReaderService struct {
	mu      sync.RWMutex
	manager *ContainerManager
	// 缓存 DecryptReaderFactory 实例，而不是原始元数据
	factories map[string]reader.DecryptReaderFactory
}

// NewReaderService 创建一个新的 ReaderService 实例。
// 它需要接收一个 ContainerManager 实例，以确保缓存共享。
func NewReaderService(manager *ContainerManager) *ReaderService {
	return &ReaderService{
		manager:   manager, // 【关键修复】使用传入的 manager
		factories: make(map[string]reader.DecryptReaderFactory),
	}
}

// 它返回一个通用的解密流、文件索引和原始大小。
// 解密器可能是可寻址的（实现了 io.Seeker），也可能是顺序的。
// 调用者有责任检查解密器的能力并采取相应的策略。
// 注意：调用者负责关闭解密器。
func (s *ReaderService) GetDecryptReader(cfg config.Config, originalPath, password string, chunkNamer namer.ChunkNamer) (reader.DecryptReaderFactory, reader.DecryptReader, types.Index, int64, error) {
	// 1. 获取可读路径（现在这个调用非常快）
	readablePath, err := s.manager.GetReadablePath(originalPath, chunkNamer)
	if err != nil {
		return nil, nil, nil, 0, fmt.Errorf("failed to get a readable container path for '%s': %w", originalPath, err)
	}

	// 2. 检查工厂缓存
	s.mu.Lock()
	factory, exists := s.factories[readablePath]
	if !exists {
		// 创建工厂
		factory, err = reader.NewDecryptReaderFactory(readablePath, password)
		if err != nil {
			s.mu.Unlock()
			return nil, nil, nil, 0, fmt.Errorf("failed to create decrypt reader factory for '%s': %w", readablePath, err)
		}
		s.factories[readablePath] = factory
	}
	s.mu.Unlock()

	if factory == nil {
		return nil, nil, nil, 0, fmt.Errorf("internal error: factory for path '%s' is nil in cache", readablePath)
	}

	// 3. 创建解密器
	decryptReader, err := factory.NewDecryptReader()
	if err != nil {
		return nil, nil, nil, 0, fmt.Errorf("failed to create decrypt reader: %w", err)
	}

	index := factory.GetIndex()
	if index == nil {
		decryptReader.Close()
		return nil, nil, nil, 0, fmt.Errorf("internal error: factory returned a nil index for path '%s'", readablePath)
	}

	return factory, decryptReader, index, factory.GetOriginalSize(), nil
}

// GetBulkDecryptor 创建一个专用于全量解密的工具。
func (s *ReaderService) GetBulkDecryptor(cfg config.Config, originalPath, password string, chunkNamer namer.ChunkNamer) (*reader.BulkDecryptor, error) {
	readablePath, err := s.manager.GetReadablePath(originalPath, chunkNamer)
	if err != nil {
		return nil, fmt.Errorf("failed to get a readable container path for '%s': %w", originalPath, err)
	}

	// BulkDecryptor 需要路径和密码，我们直接从服务中获取
	// 注意：这里我们不缓存 BulkDecryptor，因为它是一次性工具
	return reader.NewBulkDecryptor(readablePath, password), nil
}

// managedDecryptReader 需要更新，以关闭 containerReader
type managedDecryptReader struct {
	reader.DecryptReader
	containerReader reader.EncryptedContainerReader // 关闭这个
	service         *ReaderService
	metadataKey     string
}

func (m *managedDecryptReader) Close() error {
	err1 := m.DecryptReader.Close()
	err2 := m.containerReader.Close()
	// 元数据缓存由 service.Cleanup() 统一管理
	if err1 != nil {
		return fmt.Errorf("reader close failed: %w", err1)
	}
	if err2 != nil {
		return fmt.Errorf("container reader close failed: %w", err2)
	}
	return nil
}

// Cleanup 清理所有由服务管理的资源
func (s *ReaderService) Cleanup() {
	slog.Info("ReaderService starting cleanup")
	s.mu.Lock()
	defer s.mu.Unlock()

	for path, factory := range s.factories {
		if err := factory.Close(); err != nil {
			slog.Error("Failed to close factory", "path", path, "error", err)
		}
		delete(s.factories, path)
	}
	slog.Info("ReaderService factories cache cleared")
	s.manager.Cleanup()
	slog.Info("ReaderService cleanup complete")
}
