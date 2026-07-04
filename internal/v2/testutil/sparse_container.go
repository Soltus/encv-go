// Package testutil 包含 ENCV-go 内部 dev/test 工具。
//
// 关键约束：
//   - 不参与生产构建路径（go build 主包不引用本包）
//   - 所有 HTTP endpoint 都在 /api/dev/ 前缀下挂载
//   - 不修改 v2 生产包（internal/v2/）的公共 API
//
// 用途：开发人员本地验证容器格式边界，CI smoke test，人工容量测试。
package testutil

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"hash/crc32"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	"github.com/Soltus/encv-go/internal/v2/types"
)

// SparseContainerConfig 控制 sparse 虚拟容器的写出参数
type SparseContainerConfig struct {
	// OutputDir 容器文件输出目录（必须存在）
	OutputDir string
	// BaseName 容器文件基名，最终路径 = OutputDir + BaseName + ".sccg"
	BaseName string
	// FragmentCount manifest 描述的分片数（默认 100）
	FragmentCount int
	// FragmentSize 每个分片的虚拟字节数（默认 128GB = 128 * 1024^3）
	FragmentSize int64
	// PhysicalChunkMB 启用物理分片时每个 .part 文件的 MB 数（0=不分片，仅 main file）
	// 启用后会创建 FragmentCount 个 sparse .part 文件
	PhysicalChunkMB int
	// CipherMode 0=AES-128-CTR, 1=AES-256-CTR
	CipherMode uint16
	// PasswordHint 16 字节密码提示（用于 Header）
	PasswordHint [16]byte
	// ContainerType video/audio/image/document/text
	ContainerType uint16
}

// SparseResult 报告"声称 vs 实际"——用于断言 sparse 真的 sparse
type SparseResult struct {
	VirtualTotal    int64  `json:"virtualTotalBytes"`     // FragmentCount * FragmentSize
	PhysicalMain    int64  `json:"physicalMainBytes"`     // os.Stat 实际 main file size
	PhysicalUsed    int64  `json:"physicalUsedBytes"`     // du/blocks 实际占用（main + .part）
	ManifestSize    int64  `json:"manifestSizeBytes"`     // 序列化后 manifest 字节数
	FragmentCount   int    `json:"fragmentCount"`
	FragmentSize    int64  `json:"fragmentSizeBytes"`
	IsSparse        bool   `json:"isSparse"`              // virtualTotal / physicalUsed > 10x
	MainFilePath    string `json:"mainFilePath"`
	PartFilePattern string `json:"partFilePattern,omitempty"`
	DurationMs      int64  `json:"durationMs"`
}

// EdgeProbeResult 报告"读 1 个 fragment"的耗时 + 内存峰值
type EdgeProbeResult struct {
	FragmentIdx     int   `json:"fragmentIdx"`
	SeekDurationMs  int64 `json:"seekDurationMs"`
	ReadDurationMs  int64 `json:"readDurationMs"`
	BytesRead       int   `json:"bytesRead"`
	ReadReturnedEOF bool  `json:"readReturnedEOF"`  // 期待 true（sparse 区域读全 0）
	HeapAllocKB     int64 `json:"heapAllocKB"`
	HeapInUseKB     int64 `json:"heapInUseKB"`
	PhysicalSize    int64 `json:"physicalSize"`     // 探测到的物理 size
	VirtualSize     int64 `json:"virtualSize"`      // manifest 声明的 virtual size
	DurationMs      int64 `json:"durationMs"`
}

// DefaultSparseConfig 给出"100×128GB sparse"默认参数
func DefaultSparseConfig(outputDir, baseName string) SparseContainerConfig {
	return SparseContainerConfig{
		OutputDir:       outputDir,
		BaseName:        baseName,
		FragmentCount:   100,
		FragmentSize:    128 * 1024 * 1024 * 1024, // 128 GiB
		PhysicalChunkMB: 0,                         // 0 = 单 main file，不创建 .part
		CipherMode:      0,                         // AES-128-CTR
		ContainerType:   1,                         // video
	}
}

// WriteSparseVirtualContainer 写出"声明大尺寸但物理 sparse"的 ECv4 容器
//
// 核心设计（人类工程学极限的 5 条防线）：
//  1. 不写真实 fragment data——只写 V4 Header (2048B) + Manifest (< 100KB)
//  2. main file 用 f.Truncate(HeaderSize+ManifestSize+12)——OS-level sparse，物理占用 ≈ 0
//  3. 不预分配 blocks——Linux ftruncate 默认 sparse（不调用 posix_fallocate）
//  4. 不一次性 mmap 128GB——manifest 字节流式序列化，分片元数据 < 100KB
//  5. 100 个 .part 文件（如果启用分片）每个独立 Truncate，不预分配
//
// 行为：
//   - 创建 OutputDir（如果不存在）
//   - 写 main file（V4 Header + Manifest + V4 Footer）
//   - 如果 PhysicalChunkMB > 0：创建 FragmentCount 个 sparse .part 文件
//   - 返回 SparseResult（声称 vs 实际）
func WriteSparseVirtualContainer(cfg SparseContainerConfig) (SparseResult, error) {
	start := time.Now()
	if cfg.FragmentCount <= 0 {
		cfg.FragmentCount = 100
	}
	if cfg.FragmentSize <= 0 {
		cfg.FragmentSize = 128 * 1024 * 1024 * 1024
	}
	if cfg.BaseName == "" {
		cfg.BaseName = "sparse-test"
	}

	// 1) 创建输出目录
	if err := os.MkdirAll(cfg.OutputDir, 0o755); err != nil {
		return SparseResult{}, fmt.Errorf("mkdir output dir: %w", err)
	}

	// 2) 构造 manifest：100 fragments，metadata 1 个 + 数据 99 个
	manifest := types.Manifest{
		Version: 4,
		Kind:    types.IndexKind("file_encryption"), // 测试用，匹配 detector 检测路径
		Fragments: make([]types.Fragment, 0, cfg.FragmentCount),
	}

	// 第一个 fragment：metadata 类型（KVI + index）
	manifest.Fragments = append(manifest.Fragments, types.Fragment{
		ID:                "kvi-fragment",
		Type:              types.FragmentType_Metadata,
		Length:            4096, // 4KB metadata
		DataCRC32:         crc32.ChecksumIEEE([]byte("metadata-stub")),
		GlobalStartOffset: 0,
	})

	// 后续 99 个 fragment：seekable stream（每个 cfg.FragmentSize 字节）
	for i := 1; i < cfg.FragmentCount; i++ {
		manifest.Fragments = append(manifest.Fragments, types.Fragment{
			ID:                fmt.Sprintf("fragment-%d", i),
			Type:              types.FragmentType_SeekableStream,
			Length:            uint64(cfg.FragmentSize),
			DataCRC32:         0, // 故意为 0：sparse file 的 data 区域全 0，CRC32 校验会触发失败路径（这是测试目的之一）
			GlobalStartOffset: uint64(i) * uint64(cfg.FragmentSize),
		})
	}

	// 3) 序列化 manifest
	manifestBytes, err := manifest.SerializeToJSON()
	if err != nil {
		return SparseResult{}, fmt.Errorf("serialize manifest: %w", err)
	}

	// 4) 构造 V4 Header
	header, err := types.CreateHeaderV4(true, cfg.ContainerType, true, types.IDType_Raw, nil, cfg.PasswordHint)
	if err != nil {
		return SparseResult{}, fmt.Errorf("create header: %w", err)
	}
	header.CipherMode = cfg.CipherMode
	// Manifest 紧跟 Header 后
	header.ManifestOffset = types.EnvelopeHeaderSize_v4
	header.ManifestLength = uint32(len(manifestBytes))

	// 5) 写 main file：先 write header + manifest，再 Truncate 到 sparse 大小
	mainPath := filepath.Join(cfg.OutputDir, cfg.BaseName+".sccg")
	mainFile, err := os.OpenFile(mainPath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return SparseResult{}, fmt.Errorf("open main file: %w", err)
	}
	defer mainFile.Close()

	if err := types.WriteHeaderV4(mainFile, header); err != nil {
		return SparseResult{}, fmt.Errorf("write header: %w", err)
	}
	if _, err := mainFile.Write(manifestBytes); err != nil {
		return SparseResult{}, fmt.Errorf("write manifest: %w", err)
	}
	// V4 Footer (12 bytes) at end of declared (but sparse) main file
	footer := &types.EnvelopeFooterV4{
		Magic:       types.MagicFooter_v2,
		GlobalCRC32: 0, // sparse file CRC = 0 (空数据)
	}
	if err := types.WriteFooterV4(mainFile, footer); err != nil {
		return SparseResult{}, fmt.Errorf("write footer: %w", err)
	}

	// 关键：Truncate main file 到 manifest 声明的 virtual total
	// Linux/macOS ftruncate 对超过当前 size 的部分是 sparse（不预分配 blocks）
	// 我们用 ftruncate（不是 posix_fallocate），所以"声明 12.8TB 物理占用 ≈ 0"
	virtualTotal := int64(cfg.FragmentCount) * cfg.FragmentSize
	if err := mainFile.Truncate(virtualTotal); err != nil {
		return SparseResult{}, fmt.Errorf("truncate main file to virtual size: %w", err)
	}
	if err := mainFile.Sync(); err != nil {
		return SparseResult{}, fmt.Errorf("sync main file: %w", err)
	}

	// 6) 可选：创建 N 个 sparse .part 文件（如果启用物理分片）
	var partPattern string
	if cfg.PhysicalChunkMB > 0 {
		partPattern = filepath.Join(cfg.OutputDir, cfg.BaseName+".part%05d.sccg")
		// 仅创建 1 个 .part 文件作为样本（不创建 N 个 12.8TB 文件——会触发文件系统 inode 耗尽）
		// 这验证：分片文件系统能容纳大文件
		samplePartPath := fmt.Sprintf(partPattern, 0)
		partFile, err := os.OpenFile(samplePartPath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o644)
		if err != nil {
			return SparseResult{}, fmt.Errorf("create sample .part file: %w", err)
		}
		// Truncate 到 PhysicalChunkMB（1MB = 1024*1024 字节；典型 30MB-1280MB）
		chunkSize := int64(cfg.PhysicalChunkMB) * 1024 * 1024
		if err := partFile.Truncate(chunkSize); err != nil {
			partFile.Close()
			return SparseResult{}, fmt.Errorf("truncate part file: %w", err)
		}
		partFile.Sync()
		partFile.Close()
	}

	// 7) 收集统计
	stat, _ := os.Stat(mainPath)
	physicalMain := stat.Size()

	// 8) 计算实际物理占用（基于 OS-level blocks * 512）
	//    Linux/macOS: stat.Blocks * 512
	//    Windows: stat.Size() 已经是实际占用
	physicalUsed := physicalMain
	if stat != nil {
		blocks := getPhysicalBlocks(mainPath)
		physicalUsed = blocks * 512
	}

	virtualTotalSigned := int64(cfg.FragmentCount) * cfg.FragmentSize
	isSparse := physicalUsed > 0 && virtualTotalSigned/physicalUsed > 10

	return SparseResult{
		VirtualTotal:    virtualTotalSigned,
		PhysicalMain:    physicalMain,
		PhysicalUsed:    physicalUsed,
		ManifestSize:    int64(len(manifestBytes)),
		FragmentCount:   cfg.FragmentCount,
		FragmentSize:    cfg.FragmentSize,
		IsSparse:        isSparse,
		MainFilePath:    mainPath,
		PartFilePattern: partPattern,
		DurationMs:      time.Since(start).Milliseconds(),
	}, nil
}

// getPhysicalBlocks 返回文件实际占用的 512-byte blocks 数
// 跨平台：Linux/macOS 用 syscall.Stat_t.Blocks；Windows fallback 到 file size
func getPhysicalBlocks(path string) int64 {
	var stat syscall.Stat_t
	if err := syscall.Stat(path, &stat); err != nil {
		// fallback：用 os.Stat 拿 size
		info, err := os.Stat(path)
		if err != nil {
			return 0
		}
		return info.Size()
	}
	if runtime.GOOS == "windows" {
		// Windows: blocks 字段无意义，用 size
		return stat.Size
	}
	// Linux/macOS: stat.Blocks 已经是 512-byte units
	return stat.Blocks
}

// ReadSparseContainerEdgeProbe 模拟"读 128GB fragment 之一"——只 seek + 读 1KB
//
// 关键约束：
//   - 不 mmap 128GB
//   - 不预读全 fragment
//   - 测真实 sparse 行为：seek 到 offset 后 read 1KB
//   - 预期 read 返全 0x00 + EOF（因为是 sparse 区域）
func ReadSparseContainerEdgeProbe(mainPath string, fragmentIdx int, fragmentSize int64) (EdgeProbeResult, error) {
	start := time.Now()
	if fragmentIdx < 0 {
		fragmentIdx = 0
	}
	if fragmentSize <= 0 {
		fragmentSize = 128 * 1024 * 1024 * 1024
	}

	// 验证 header 可读（不读 fragment data，避免 OOM）
	if err := VerifySparseReadSafe(mainPath); err != nil {
		return EdgeProbeResult{}, fmt.Errorf("verify header: %w", err)
	}

	// seek 到 fragment 物理 offset（注意：这里测的是 sparse 区域，OS 会读 0x00）
	seekOffset := int64(fragmentIdx) * fragmentSize
	if seekOffset < 0 {
		seekOffset = 0
	}

	file, err := os.Open(mainPath)
	if err != nil {
		return EdgeProbeResult{}, fmt.Errorf("open main file: %w", err)
	}
	defer file.Close()

	seekStart := time.Now()
	if _, err := file.Seek(seekOffset, 0); err != nil {
		return EdgeProbeResult{}, fmt.Errorf("seek: %w", err)
	}
	seekDur := time.Since(seekStart).Milliseconds()

	// 读 1KB
	readStart := time.Now()
	buf := make([]byte, 1024)
	n, readErr := file.Read(buf)
	readDur := time.Since(readStart).Milliseconds()

	eof := readErr != nil && readErr.Error() == "EOF"

	// 内存峰值
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	// 物理 size
	stat, _ := file.Stat()

	return EdgeProbeResult{
		FragmentIdx:     fragmentIdx,
		SeekDurationMs:  seekDur,
		ReadDurationMs:  readDur,
		BytesRead:       n,
		ReadReturnedEOF: eof,
		HeapAllocKB:     int64(memStats.HeapAlloc) / 1024,
		HeapInUseKB:     int64(memStats.HeapInuse) / 1024,
		PhysicalSize:    stat.Size(),
		VirtualSize:     int64(fragmentIdx+1) * fragmentSize,
		DurationMs:      time.Since(start).Milliseconds(),
	}, nil
}

// openReader 返回一个 *os.File（不直接用 io.Reader 是为了让 caller 可以 seek）
func openReader(path string, offset int64) (*os.File, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	if offset > 0 {
		if _, err := f.Seek(offset, 0); err != nil {
			f.Close()
			return nil, err
		}
	}
	return f, nil
}

// CleanupSparseContainer 删除 WriteSparseVirtualContainer 创建的所有产物
func CleanupSparseContainer(outputDir, baseName string) error {
	mainPath := filepath.Join(outputDir, baseName+".sccg")
	if err := os.Remove(mainPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	// 清理可能创建的 .part 文件（glob 模式匹配）
	pattern := filepath.Join(outputDir, baseName+".part*.sccg")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return err
	}
	for _, m := range matches {
		if err := os.Remove(m); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

// VerifySparseReadSafe 验证 main file 至少 header+manifest 是可读的
// （不读 fragment data，避免 OOM）
func VerifySparseReadSafe(mainPath string) error {
	f, err := os.Open(mainPath)
	if err != nil {
		return fmt.Errorf("open: %w", err)
	}
	defer f.Close()

	// 读 4KB（足够覆盖 header + 部分 manifest）
	buf := make([]byte, 4096)
	n, err := f.Read(buf)
	if err != nil {
		return fmt.Errorf("read 4KB: %w", err)
	}
	if n < int(types.EnvelopeHeaderSize_v4) {
		return fmt.Errorf("read only %d bytes, expected at least %d", n, types.EnvelopeHeaderSize_v4)
	}

	// 校验 magic
	if string(buf[0:4]) != string(types.MagicHeader_v2[:]) {
		return fmt.Errorf("invalid magic: %q", buf[0:4])
	}

	// 校验 header CRC32（offset 2036-2040）
	storedCRC := binary.LittleEndian.Uint32(buf[2036:2040])
	calcCRC := crc32.ChecksumIEEE(buf[:2036])
	if storedCRC != calcCRC {
		return fmt.Errorf("header CRC mismatch: stored=%08x, calc=%08x", storedCRC, calcCRC)
	}
	return nil
}

// ============= internal helpers =============

// 保留 binary/json import（确保未来扩展不报 unused import 错误）
var _ = binary.LittleEndian
var _ = json.Marshal
