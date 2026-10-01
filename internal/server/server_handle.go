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
	"github.com/Soltus/encv-go/internal/v2/reader"
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
	// 3. 【关键修复】传入 factory
	// NewLocalFileProvider 需要 factory 来判断 IsSeekable，从而决定缓存策略
	prov, err := provider.NewLocalFileProvider(ctx, factory, decryptReader)
	if err != nil {
		// provider 没接管 decryptReader，这里自己关（避免句柄泄漏）
		decryptReader.Close()
		slog.Error("NewLocalFileProvider failed", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	// ⚠️ 所有权已交给 provider：**只**用 prov.Close() 关，不要再加
	// `defer decryptReader.Close()` —— 那会关两次。
	// 重复 Close 会把全局共享文件句柄的引用计数多减一次，导致并发的
	// 其它请求读到 "file already closed"（2026-10-02 模拟器实测）。
	// （factory 是 Service 缓存的，不要关。）
	defer prov.Close()

	// 3.5 【完整性预校验】在写任何响应体之前先确认文件没坏。
	//
	// 背景：分块 CRC 只能在**读到损坏块**时中止 —— 那时响应头（200 + Content-Length）
	// 已经发出去了，客户端拿到的是"截断的流"而不是明确的错误。真机上表现为
	// "播到一半卡住"，用户和前端都不知道是文件坏了。
	// 预校验让我们能在吐第一个字节前就返回 422 data_corrupted。
	//
	// 代价 = 一次顺序读（不解密）。因此严格限制触发条件，避免拖慢大文件首字节：
	//   · 仅本地容器（远程/alist 流没有 NewRawContainerReader，自动跳过）
	//   · 仅不带 Range 的请求（带 Range 的拖动/续传由分块 CRC 兜底）
	//   · 仅大小 ≤ preVerifyMaxSize
	//   · 容器必须真的带 CRC 元数据（老容器跳过 ⇒ 行为完全不变）
	if shouldPreVerify(r, prov.GetSize()) {
		if err := verifyContainerIntegrity(factory, prov.GetSize()); err != nil {
			slog.Error("pre-serve integrity check failed", "path", fullPath, "error", err)
			if errors.Is(err, types.ErrDataCorrupted) {
				http.Error(w, `{"error":"data_corrupted","message":"文件数据已损坏（完整性校验失败）"}`, http.StatusUnprocessableEntity)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	// 4. 处理内容
	s.contentHandler.ServeFile(w, r, prov)
}

// preVerifyMaxSize 预校验的大小上限（与 reader.MaxPreVerifySize 对齐）
const preVerifyMaxSize = reader.MaxPreVerifySize

// shouldPreVerify 是否值得在吐字节前先整片校验一遍
func shouldPreVerify(r *http.Request, size int64) bool {
	if size <= 0 || size > preVerifyMaxSize {
		return false
	}
	// 带 Range 的请求（拖动/续传/并发分段）不预校验：
	// 为了 1KB 的 Range 去读整片不划算，且分块 CRC 已经能在损坏处中止。
	return r.Header.Get("Range") == ""
}

// verifyContainerIntegrity 见 reader.VerifyContainerIntegrity（下沉到 reader 包，
// 好让 server 与 service 两条路径共用同一份实现）。
func verifyContainerIntegrity(factory reader.DecryptReaderFactory, size int64) error {
	return reader.VerifyContainerIntegrity(factory, size)
}
