package reader

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/Soltus/encv-go/internal/logger"
	"github.com/Soltus/encv-go/internal/utils"
	"github.com/Soltus/encv-go/internal/v2/container/block"
	"github.com/Soltus/encv-go/internal/v2/container/envelope"
	containerhandle "github.com/Soltus/encv-go/internal/v2/container/handle"
	"github.com/Soltus/encv-go/internal/v2/container/manifest"
	"github.com/Soltus/encv-go/internal/v2/crypto"
	"github.com/Soltus/encv-go/internal/v2/types"
)

// readerLogger 是 reader 包的日志记录器
var readerLogger = logger.WithComponent("reader.remote")

// URLResolver 是一个用于将物理路径解析为完整、可访问的 URL 的接口。
// 这使得 remoteEncryptedContainerReader 不再与特定的 URL 生成逻辑（如 OpenList）耦合。
type URLResolver interface {
	ResolveURL(physicalPath string) (string, error)
}

// remoteEncryptedContainerReader 实现了 EncryptedContainerReader 接口
// 它通过 HTTP Range 请求与远程服务器交互，实现按需读取
type remoteEncryptedContainerReader struct {
	containerURL string
	headers      map[string][]string
	urlResolver  URLResolver
	// 【V3/V4 适配】缓存 Header 版本和大小 (V2:16, V3:2048, V4:2048)
	headerSize int64
	version    int
	// 缓存，避免重复请求
	manifest *types.Manifest
}

// NewRemoteEncryptedContainerReader 创建一个新的远程容器读取器
func NewRemoteEncryptedContainerReader(containerURL string, headers map[string][]string, urlResolver URLResolver) (EncryptedContainerReader, error) {
	r := &remoteEncryptedContainerReader{
		containerURL: containerURL,
		headers:      headers,
		urlResolver:  urlResolver,
	}
	// 【V3 适配】初始化时探测 Header 版本并缓存大小
	if err := r.initHeaderSize(); err != nil {
		return nil, fmt.Errorf("failed to detect header size: %w", err)
	}
	return r, nil
}

// initHeaderSize 读取文件头部的前 6 字节，探测版本并缓存 Header 大小
func (r *remoteEncryptedContainerReader) initHeaderSize() error {
	// 读取前 6 字节 (4B Magic + 2B Version)
	resp, err := utils.GetRemoteStreamWithRange(r.containerURL, r.headers, 0, 5)
	if err != nil {
		return fmt.Errorf("failed to read header for version detection: %w", err)
	}
	defer resp.Body.Close()

	headerBytes := make([]byte, 6)
	if _, err := io.ReadFull(resp.Body, headerBytes); err != nil {
		return fmt.Errorf("failed to read header bytes: %w", err)
	}

	version, headerSize, err := types.DetectHeaderInfoFromBytes(headerBytes)
	if err != nil {
		return fmt.Errorf("failed to parse header: %w", err)
	}

	r.headerSize = headerSize
	r.version = version
	if r.headerSize == 0 {
		return fmt.Errorf("unknown header version detected: %d", version)
	}

	readerLogger.Info("detected container version",
		slog.Int("version", version),
		slog.Int64("header_size", r.headerSize),
	)
	return nil
}

// calculateDiskOffset 计算特定 Fragment 在文件中的物理磁盘偏移量
// 公式：HeaderSize + (FragIndex * BlockHeaderSize) + GlobalStartOffset
// 因为 GlobalStartOffset 是数据流的偏移量，但每个数据块前都有一个 BlockHeader
// 注意：此函数仅用于主文件中的逻辑分片
func (r *remoteEncryptedContainerReader) calculateDiskOffset(frag *types.Fragment) (int64, error) {
	if r.manifest == nil {
		return 0, fmt.Errorf("manifest not loaded, cannot calculate offset")
	}

	// 查找当前 Fragment 的索引
	fragIndex := -1
	for i, f := range r.manifest.Fragments {
		if f.ID == frag.ID {
			fragIndex = i
			break
		}
	}
	if fragIndex == -1 {
		return 0, fmt.Errorf("fragment %s not found in manifest", frag.ID)
	}

	blockHeaderSize := block.GetBlockHeader_v2_Size()

	// 计算公式：Envelope Header + (该 Fragment 之前的所有 Block Headers) + 该 Fragment 的数据起始偏移
	diskOffset := r.headerSize + (int64(fragIndex) * blockHeaderSize) + int64(frag.GlobalStartOffset)
	return diskOffset, nil
}

// GetFragmentReader 按需获取指定 Fragment 的加密数据流
func (r *remoteEncryptedContainerReader) GetFragmentReader(fragID string) (io.ReadCloser, error) {

	// 确保 Manifest 已加载
	if r.manifest == nil {
		r.GetManifest()
		if r.manifest == nil {
			return nil, fmt.Errorf("manifest is not available")
		}
	}

	frag, err := r.manifest.GetFragmentByID(fragID)
	if err != nil {
		return nil, fmt.Errorf("fragment '%s' not found: %w", fragID, err)
	}

	blockHeaderSize := block.GetBlockHeader_v2_Size()

	// 情况 1: 处理物理分片 (外部 .part 文件)
	// 【关键修复】正确处理物理分片，确保只读取当前 Fragment 的数据
	if frag.PhysicalPath != "" {
		readerLogger.Debug("processing physical chunk",
			slog.String("fragment_id", fragID),
			slog.String("physical_path", frag.PhysicalPath),
		)

		chunkURL, err := r.urlResolver.ResolveURL(frag.PhysicalPath)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve URL for physical chunk '%s': %w", fragID, err)
		}

		readerLogger.Debug("requesting physical chunk",
			slog.String("fragment_id", fragID),
			slog.String("url", chunkURL),
		)

		// 请求整个分片文件
		resp, err := utils.GetRemoteStreamWithRange(chunkURL, r.headers, 0, -1)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch physical chunk '%s': %w", fragID, err)
		}

		// 1. 跳过分片文件的 V3 Header
		if _, err := io.CopyN(io.Discard, resp.Body, r.headerSize); err != nil {
			resp.Body.Close()
			return nil, fmt.Errorf("failed to skip v3 header for physical chunk '%s': %w", fragID, err)
		}

		// 2. 跳过该 Fragment 的 Block Header
		if _, err := io.CopyN(io.Discard, resp.Body, blockHeaderSize); err != nil {
			resp.Body.Close()
			return nil, fmt.Errorf("failed to skip block header for physical chunk '%s': %w", fragID, err)
		}

		// 3. 【关键修复】使用 LimitReader 限制读取范围
		// 物理分片文件可能包含多个 Fragment，我们必须只读取当前 Fragment 的长度
		// 否则读取器会读到下一个 Fragment 的数据
		limitedBody := io.LimitReader(resp.Body, int64(frag.Length))
		readerLogger.Debug("fragment reader created",
			slog.String("fragment_id", fragID),
			slog.String("source", "remote_chunk"),
			slog.Int64("offset", r.headerSize+blockHeaderSize),
			slog.Int64("length", int64(frag.Length)),
		)

		return struct {
			io.Reader
			io.Closer
		}{
			Reader: limitedBody,
			Closer: resp.Body,
		}, nil
	}

	// 情况 2: 处理主文件中的逻辑分片
	switch frag.Type {
	case types.FragmentType_SeekableStream:
		// 【修复】使用 calculateDiskOffset 计算正确的物理位置
		diskStartOffset, err := r.calculateDiskOffset(frag)
		if err != nil {
			return nil, fmt.Errorf("failed to calculate disk offset for fragment '%s': %w", fragID, err)
		}

		dataEndOffset := diskStartOffset + int64(frag.Length) - 1
		readerLogger.Debug("requesting seekable fragment",
			slog.String("fragment_id", fragID),
			slog.Int64("range_start", diskStartOffset),
			slog.Int64("range_end", dataEndOffset),
		)

		resp, err := utils.GetRemoteStreamWithRange(r.containerURL, r.headers, diskStartOffset, dataEndOffset)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch seekable fragment '%s': %w", fragID, err)
		}
		readerLogger.Debug("fragment reader created",
			slog.String("fragment_id", fragID),
			slog.String("source", "remote_seekable"),
			slog.Int64("offset", diskStartOffset),
			slog.Int64("length", int64(frag.Length)),
		)

		// 直接返回 resp.Body，它是一个 io.ReadCloser
		return resp.Body, nil

	case types.FragmentType_AtomicFile:
		// 【核心修复】上游服务器对 Range 请求有 Bug。
		// 我们发起一个完整的 GET 请求，然后从流中手动提取我们需要的片段数据。
		readerLogger.Debug("upstream range requests broken, using full GET",
			slog.String("fragment_id", fragID),
		)

		// 【P0-1 修复】必须带 ctx + client.Timeout，否则上游 hang 时整测试卡死
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.containerURL, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create GET request for '%s': %w", fragID, err)
		}
		for key, values := range r.headers {
			for _, v := range values {
				req.Header.Set(key, v)
			}
		}

		client := &http.Client{Timeout: 30 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("failed to execute GET request for '%s': %w", fragID, err)
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return nil, fmt.Errorf("unexpected status code for full GET: %s", resp.Status)
		}

		// 现在，resp.Body 是整个文件的流。我们需要从中找到我们的数据。

		// 【修复】使用 calculateDiskOffset
		diskStartOffset, err := r.calculateDiskOffset(frag)
		if err != nil {
			resp.Body.Close()
			return nil, fmt.Errorf("failed to calculate disk offset for fragment '%s': %w", fragID, err)
		}

		// 1. 跳过我们不需要的数据（从文件开始到我们数据块开始的位置）
		_, err = io.CopyN(io.Discard, resp.Body, diskStartOffset)
		if err != nil {
			resp.Body.Close()
			return nil, fmt.Errorf("failed to skip %d bytes for fragment '%s': %w", diskStartOffset, fragID, err)
		}

		// 2. 创建一个 reader，它只读取我们需要的片段长度
		limitedDataReader := io.LimitReader(resp.Body, int64(frag.Length))
		readerLogger.Debug("fragment reader created",
			slog.String("fragment_id", fragID),
			slog.String("source", "remote_atomic"),
			slog.Int64("offset", diskStartOffset),
			slog.Int64("length", int64(frag.Length)),
		)

		// 3. 将它包装成一个 io.ReadCloser，确保 Close 方法能关闭底层的 HTTP 连接
		return struct {
			io.Reader
			io.Closer
		}{
			Reader: limitedDataReader,
			Closer: resp.Body,
		}, nil

	default:
		return nil, fmt.Errorf("unsupported fragment type: %s for fragment '%s'", frag.Type, fragID)
	}
}

// responseBodyWrapper 确保底层的 http.Response.Body 被正确关闭
type responseBodyWrapper struct {
	io.ReadCloser
	resp *http.Response
}

func (w *responseBodyWrapper) Close() error {
	// 首先关闭我们创建的 ReadCloser
	if err := w.ReadCloser.Close(); err != nil {
		return err
	}
	// 然后关闭底层的 HTTP 响应体
	return w.resp.Body.Close()
}

// GetManifest 按需获取并缓存 Manifest
func (r *remoteEncryptedContainerReader) GetManifest() *types.Manifest {
	if r.manifest != nil {
		return r.manifest
	}

	if r.version == 4 {
		return r.getManifestV4()
	}

	return r.getManifestV23()
}

func (r *remoteEncryptedContainerReader) getManifestV23() *types.Manifest {
	// 1. 【V2/V3】下载 Footer (最后 32 字节)
	footerResp, err := utils.GetRemoteStreamWithRange(r.containerURL, r.headers, -32, -1)
	if err != nil {
		readerLogger.Error("failed to fetch footer", slog.Any("error", err))
		return nil
	}
	defer footerResp.Body.Close()

	footerData, err := io.ReadAll(footerResp.Body)
	if err != nil {
		readerLogger.Error("failed to read footer data", slog.Any("error", err))
		return nil
	}

	// 2. 使用 envelope 包的 Bytes 解析函数
	footer, err := envelope.ParseEnvelopeFooterFromBytes(footerData)
	if err != nil {
		readerLogger.Error("failed to parse footer", slog.Any("error", err))
		return nil
	}

	// 3. 下载整个 Manifest 块 (Header + EncryptedData)
	manifestStart := int64(footer.ManifestOffset)
	manifestEnd := manifestStart + int64(footer.ManifestLength) - 1

	readerLogger.Debug("fetching manifest block",
		slog.Int64("start", manifestStart),
		slog.Int64("end", manifestEnd),
	)

	manifestResp, err := utils.GetRemoteStreamWithRange(r.containerURL, r.headers, manifestStart, manifestEnd)
	if err != nil {
		readerLogger.Error("failed to fetch manifest block", slog.Any("error", err))
		return nil
	}
	defer manifestResp.Body.Close()

	manifestBlockData, err := io.ReadAll(manifestResp.Body)
	if err != nil {
		readerLogger.Error("failed to read manifest block data", slog.Any("error", err))
		return nil
	}

	// 4. 使用 manifest 包的辅助函数解密并解析
	r.manifest, err = manifest.ReadEncryptedManifestBlock(manifestBlockData)
	if err != nil {
		readerLogger.Error("failed to decrypt/parse manifest", slog.Any("error", err))
		return nil
	}

	readerLogger.Info("manifest loaded successfully",
		slog.Int("fragment_count", len(r.manifest.Fragments)),
		slog.String("kind", string(r.manifest.Kind)),
	)

	return r.manifest
}

func (r *remoteEncryptedContainerReader) getManifestV4() *types.Manifest {
	// 1. 读取 V4 Footer（最后 12 字节）
	footerResp, err := utils.GetRemoteStreamWithRange(r.containerURL, r.headers, -int64(types.EnvelopeFooterSize_v4), -1)
	if err != nil {
		readerLogger.Error("failed to fetch v4 footer", slog.Any("error", err))
		return nil
	}
	defer footerResp.Body.Close()

	footerData, err := io.ReadAll(footerResp.Body)
	if err != nil {
		readerLogger.Error("failed to read v4 footer data", slog.Any("error", err))
		return nil
	}

	v4Footer := &types.EnvelopeFooterV4{}
	if err := binary.Read(bytes.NewReader(footerData), binary.LittleEndian, v4Footer); err != nil {
		readerLogger.Error("failed to parse v4 footer", slog.Any("error", err))
		return nil
	}

	// 2. 读取 V4 Header（前 2048 字节）获取 ManifestOffset/ManifestLength
	headerResp, err := utils.GetRemoteStreamWithRange(r.containerURL, r.headers, 0, int64(types.EnvelopeHeaderSize_v4)-1)
	if err != nil {
		readerLogger.Error("failed to fetch v4 header", slog.Any("error", err))
		return nil
	}
	defer headerResp.Body.Close()

	headerData, err := io.ReadAll(headerResp.Body)
	if err != nil {
		readerLogger.Error("failed to read v4 header data", slog.Any("error", err))
		return nil
	}

	v4Header, err := types.ReadHeaderV4(bytes.NewReader(headerData))
	if err != nil {
		readerLogger.Error("failed to parse v4 header", slog.Any("error", err))
		return nil
	}

	if v4Header.ManifestLength == 0 || v4Header.ManifestOffset == 0 {
		readerLogger.Error("v4 header has invalid manifest offset/length")
		return nil
	}

	// 3. 读取 Obfuscated Manifest 数据
	manifestStart := int64(v4Header.ManifestOffset)
	manifestEnd := manifestStart + int64(v4Header.ManifestLength) - 1

	manifestResp, err := utils.GetRemoteStreamWithRange(r.containerURL, r.headers, manifestStart, manifestEnd)
	if err != nil {
		readerLogger.Error("failed to fetch v4 manifest", slog.Any("error", err))
		return nil
	}
	defer manifestResp.Body.Close()

	obfuscatedData, err := io.ReadAll(manifestResp.Body)
	if err != nil {
		readerLogger.Error("failed to read v4 manifest data", slog.Any("error", err))
		return nil
	}

	// 4. Deobfuscate + Deserialize
	plainData, err := crypto.DeobfuscateManifest(obfuscatedData)
	if err != nil {
		readerLogger.Error("failed to deobfuscate v4 manifest", slog.Any("error", err))
		return nil
	}

	v4Manifest, err := types.DeserializeManifest_v4(plainData)
	if err != nil {
		readerLogger.Error("failed to deserialize v4 manifest", slog.Any("error", err))
		return nil
	}

	// 5. 适配为 Manifest
	r.manifest = containerhandle.AdaptV4ToV2(v4Manifest, v4Header)

	readerLogger.Info("v4 manifest loaded successfully",
		slog.Int("segment_count", len(v4Manifest.Segments)),
		slog.String("container_type", v4Manifest.ContainerType),
	)

	return r.manifest
}

// GetKVIProvider 从缓存的 Manifest 中获取 KVI
func (r *remoteEncryptedContainerReader) GetKVIProvider() (types.KVIProvider, error) {
	manifest := r.GetManifest()
	if manifest == nil {
		return nil, fmt.Errorf("could not get manifest to retrieve KVI")
	}
	return types.NewKVIProviderFromManifest(manifest)
}

// GetFragments 返回 Manifest 中的所有片段定义
func (r *remoteEncryptedContainerReader) GetFragments() []types.Fragment {
	manifest := r.GetManifest()
	if manifest == nil {
		return nil
	}
	return manifest.Fragments
}

// Close 对于远程读取器是空操作
func (r *remoteEncryptedContainerReader) Close() error {
	return nil
}
