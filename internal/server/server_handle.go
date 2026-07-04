package server

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/Soltus/encv-go/internal/utils"
	"github.com/Soltus/encv-go/internal/v2/container/detector"
	"github.com/Soltus/encv-go/internal/v2/namer"
	"github.com/Soltus/encv-go/internal/v2/provider"
	"github.com/Soltus/encv-go/internal/v2/types"
)

// compositeChunkNamer 实现 ChunkNamer 接口
// 它包含一个“激活”的 Namer（用于生成），但检查所有 Namers（用于判断）
type compositeChunkNamer struct {
	namers      []namer.ChunkNamer
	activeNamer namer.ChunkNamer // 当前选中的用于生成规则的 Namer
}

// newCompositeChunkNamer 创建适配器，默认使用第一个作为激活的 Namer
func newCompositeChunkNamer(namers []namer.ChunkNamer) *compositeChunkNamer {
	var active namer.ChunkNamer
	if len(namers) > 0 {
		active = namers[0]
	}
	return &compositeChunkNamer{
		namers:      namers,
		activeNamer: active,
	}
}

// IsDataChunk 检查是否为数据分片（遍历所有规则）
func (c *compositeChunkNamer) IsDataChunk(filename string) bool {
	for _, n := range c.namers {
		if n.IsDataChunk(filename) {
			return true
		}
	}
	return false
}

// GenerateMainChunkName 生成主容器文件名（使用激活的 Namer）
func (c *compositeChunkNamer) GenerateMainChunkName(baseName string) string {
	if c.activeNamer != nil {
		return c.activeNamer.GenerateMainChunkName(baseName)
	}
	return baseName
}

// ParseFirstChunkName 解析主容器路径
// 【关键】它会尝试所有 Namer，如果成功，则将对应的 Namer 设为激活状态
func (c *compositeChunkNamer) ParseFirstChunkName(firstChunkPath string) (string, error) {
	for _, n := range c.namers {
		base, err := n.ParseFirstChunkName(firstChunkPath)
		if err == nil {
			c.activeNamer = n // 【自动切换】找到匹配的规则，锁定它
			return base, nil
		}
	}
	return "", fmt.Errorf("no suitable namer found for path: %s", firstChunkPath)
}

// GenerateDataChunkName 生成数据分片文件名（使用激活的 Namer）
func (c *compositeChunkNamer) GenerateDataChunkName(baseName string, index int) string {
	if c.activeNamer != nil {
		return c.activeNamer.GenerateDataChunkName(baseName, index)
	}
	return fmt.Sprintf("%s.%d", baseName, index) // Fallback
}

// GetFirstDataChunkIndex 获取第一个数据分片索引（使用激活的 Namer）
func (c *compositeChunkNamer) GetFirstDataChunkIndex() int {
	if c.activeNamer != nil {
		return c.activeNamer.GetFirstDataChunkIndex()
	}
	return 1 // Fallback
}

// handleStreamRequest 处理 /stream?file=... 格式的请求
func (s *Server) handleStreamRequest(w http.ResponseWriter, r *http.Request) {
	// 1. 从查询参数中获取文件的绝对路径
	rawPath := r.URL.Query().Get("path")
	if rawPath == "" {
		rawPath = r.URL.Query().Get("file")
	}
	if rawPath == "" {
		http.Error(w, "Bad Request: 'path' or 'file' query parameter is missing", http.StatusBadRequest)
		return
	}

	filePath := utils.DecodeGinQueryParam(rawPath)

	cleanedFilePath, err := s.resolveUserPath(filePath)
	if err != nil {
		// 根据错误类型返回不同的 HTTP 状态码
		if strings.Contains(err.Error(), "forbidden") {
			http.Error(w, err.Error(), http.StatusForbidden)
		} else {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}
	_, detectErr := detector.DetectContainer(cleanedFilePath)
	if detectErr != nil {
		slog.Info("File is not an ENCV container, serving raw file", "path", cleanedFilePath)
		http.ServeFile(w, r, cleanedFilePath)
		return
	}
	s.serveEncryptedFile(w, r, cleanedFilePath)
}

func (s *Server) serveEncryptedFile(w http.ResponseWriter, r *http.Request, fullPath string) {
	ctx := r.Context()

	// 1. 创建组合适配器
	adapterNamer := &compositeChunkNamer{namers: s.chunkNamers}

	// 2. 【关键修改】接收 factory
	// 现在 GetDecryptReader 返回 (factory, decryptReader, index, size, err)
	factory, decryptReader, _, _, err := s.readerService.GetDecryptReader(
		*s.cfg,
		fullPath,
		s.cfg.Password,
		adapterNamer,
	)
	if err != nil {
		slog.Error("GetDecryptReader failed", "path", fullPath, "error", err)
		if errors.Is(err, types.ErrWrongPassword) {
			http.Error(w, `{"error":"wrong_password","message":"密码可能错误，请检查后重试"}`, http.StatusForbidden)
			return
		}
		if errors.Is(err, types.ErrDataCorrupted) {
			http.Error(w, `{"error":"data_corrupted","message":"文件数据已损坏"}`, http.StatusUnprocessableEntity)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	// 注意：只关闭 decryptReader，不要关闭 factory，因为 factory 是 Service 缓存的
	defer decryptReader.Close()

	// 3. 【关键修复】传入 factory
	// NewLocalFileProvider 需要 factory 来判断 IsSeekable，从而决定缓存策略
	prov, err := provider.NewLocalFileProvider(ctx, factory, decryptReader)
	if err != nil {
		slog.Error("NewLocalFileProvider failed", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer prov.Close()

	// 4. 处理内容
	s.contentHandler.ServeFile(w, r, prov)
}
