package service

import (
	"fmt"
	"log/slog"
	"sync"

	"github.com/Soltus/encv-go/internal/v2/container/detector"
	"github.com/Soltus/encv-go/internal/v2/namer"
	"github.com/Soltus/encv-go/internal/v2/physical"
)

// CachedContainer 缓存一个已重建的容器信息
type CachedContainer struct {
	UnifiedPath string
	CleanupFunc func()
}

// ContainerManager 负责管理容器的重建和缓存
// 【架构核心】它的首要职责是“智能地选择”一个可用的容器路径，而不是盲目地重建。
type ContainerManager struct {
	mu    sync.RWMutex
	cache map[string]*CachedContainer
}

// NewContainerManager 创建一个新的管理器实例
func NewContainerManager() *ContainerManager {
	return &ContainerManager{
		cache: make(map[string]*CachedContainer),
	}
}

// GetReadablePath 返回一个可直接用于创建 DecryptReaderFactory 的文件路径。
// 【新的工作流】
// 1. 优先使用原始路径（零开销）。
// 2. 只有在原始文件损坏时，才使用缓存的或新创建的重建文件。
func (cm *ContainerManager) GetReadablePath(originalPath string, chunkNamer namer.ChunkNamer) (string, error) {
	// 1. 【关键修复】调用权威的 detector.DetectContainer
	_, err := detector.DetectContainer(originalPath)
	if err == nil {
		// 如果 err 为 nil，说明文件是有效的 ENCV 容器
		slog.Debug("Container is valid, using original path", "path", originalPath)
		return originalPath, nil
	}

	// 2. 【降级路径】文件无效（例如，下载损坏、Footer 丢失），需要重建。检查缓存。
	cm.mu.RLock()
	if cached, found := cm.cache[originalPath]; found {
		cm.mu.RUnlock()
		slog.Info("Using cached rebuilt file", "path", originalPath)
		return cached.UnifiedPath, nil
	}
	cm.mu.RUnlock()

	// 3. 缓存未命中，执行重建（加锁防止并发重建）
	cm.mu.Lock()
	defer cm.mu.Unlock()

	// 双重检查锁定，防止并发竞争
	if cached, found := cm.cache[originalPath]; found {
		slog.Info("Using cached rebuilt file (after double-check)", "path", originalPath)
		return cached.UnifiedPath, nil
	}

	slog.Warn("Container is invalid and not cached, rebuilding as last resort", "path", originalPath)

	unpacker := physical.NewFileChunkerPhysicalUnpacker(chunkNamer)
	unifiedPath, cleanup, err := unpacker.Unpack(originalPath)
	if err != nil {
		return "", fmt.Errorf("container manager failed to rebuild '%s': %w", originalPath, err)
	}

	// 缓存结果
	cm.cache[originalPath] = &CachedContainer{
		UnifiedPath: unifiedPath,
		CleanupFunc: cleanup,
	}
	slog.Info("Container rebuild successful", "path", originalPath, "cached_at", unifiedPath)

	return unifiedPath, nil
}

// Cleanup 清理所有缓存的临时文件
func (cm *ContainerManager) Cleanup() {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	slog.Info("ContainerManager cleaning up cached temporary files", "count", len(cm.cache))
	for path, cached := range cm.cache {
		if cached.CleanupFunc != nil {
			cached.CleanupFunc()
		}
		delete(cm.cache, path)
	}
	slog.Info("ContainerManager cleanup complete")
}
