// internal/v2/writer/container_writer_v4.go
//
// v4 容器直接写入路径（不走 SingleFileContainerWriterV4 插件加密路径）。
// 本文件实现了 v4 容器的扁平化写入流程：
//
//	[Header(2048B)][Segments...][DisasterZones...][Manifest][Footer(12B)]
//
// 每个 Segment 的内部布局（v4 升级后，34B Header + 可变 payload）：
//
//	[SegmentHeader(34B)][Nonce(16B)][Ciphertext(DataLength B)][HMAC(10B)][SeekTable(Variable)]
//
// **Task 11 集成**：
//   - CipherMode：v4 Header.CipherMode 字段（0=AES-128 / 1=AES-256），影响密钥长度
//   - HMAC-SHA1-80：Encrypt-then-MAC，紧跟 ciphertext 之后
//   - zstd seekable 压缩：可选，per-Segment 独立选择
//   - ModeFlags：bit0=Encrypted, bit1=CompressionZstd，控制每 Segment 行为
//
// **向后兼容策略**：
//   - V4WriteParams 新增字段（CipherMode/CompressionMode/EnableHMAC）默认为「旧行为」：
//     - CipherMode = 0（AES-128）
//     - CompressionMode = "none"
//     - EnableHMAC = false（旧 v4 容器不写 MAC 字节，保持原磁盘格式）
//   - 旧测试不传新字段时，行为与升级前完全一致
//   - 新测试显式 EnableHMAC=true 时，启用 v4 升级布局（写 MAC 字节）
//
// **不加密 Segment 模式**：
//   - 当 segResult.ModeFlags & ModeFlagEncrypted == 0 时（明文 Segment）：
//     - layout = [SegmentHeader(34B)][Plaintext(DataLength B)]
//     - 无 Nonce、Ciphertext、HMAC、SeekTable
//   - 当前 SegmentEncryptionResult 总是产出加密 Segment；明文模式由调用方在 ModeFlags 中置 0
package writer

import (
	"encoding/base64"
	"fmt"
	"hash"
	"hash/crc32"
	"io"
	"os"

	"github.com/Soltus/encv-go/internal/v2/crypto"
	"github.com/Soltus/encv-go/internal/v2/crypto/compression"
	"github.com/Soltus/encv-go/internal/v2/types"
)

// V4WriteParams 是 WriteV4Container 的输入参数。
//
// **Task 11 字段扩展**：
//   - CipherMode:       0=AES-128-CTR（v4 默认）/ 1=AES-256-CTR
//   - CompressionMode:  "none"（默认） / "zstd"（64KB 块大小 seekable zstd）
//   - EnableHMAC:       false（默认，**保留旧 v4 行为**） / true（v4 升级布局，写 10 字节 HMAC）
//
// 旧 v4 容器兼容性：当 EnableHMAC=false 时，writer 不写 HMAC 字节到磁盘，
// 与升级前布局完全一致（仅 [Header][Nonce][Ciphertext]）。
type V4WriteParams struct {
	OutputPath     string
	IsMain         bool
	ContainerType  uint16
	IsSeekable     bool
	IDType         types.IDType
	IDData         []byte
	Manifest       *types.Manifest_v4
	SegmentResults []*crypto.SegmentEncryptionResult
	DisasterZones  []types.DisasterZone
	PasswordHint   [16]byte

	// === Task 11 新增字段（v4 container capability upgrade）===

	// CipherMode 标识 v4 容器使用的 AES 密钥长度。
	// 0 = AES-128-CTR（v4 新默认，16 字节密钥）
	// 1 = AES-256-CTR（v4 可选，32 字节密钥）
	// 默认 0（向后兼容）。
	CipherMode uint16

	// CompressionMode 标识 v4 Segment 使用的压缩算法。
	// "none"（默认）= 不压缩
	// "zstd"        = seekable zstd 压缩（64KB 块大小）
	// 注：compressionMode 仅影响 SegmentEncryptionResult.Compressed 字段设置，
	// 实际压缩由调用方在调用 crypto.EncryptSegment 时决定（参数透传）。
	// 此字段保留用于 writer 内部一致性检查（未来可强化）。
	CompressionMode string

	// EnableHMAC 控制是否在 Segment 末尾写入 10 字节 HMAC-SHA1-80 截断值。
	// false（默认）= 旧 v4 容器布局，不写 MAC 字节
	// true         = v4 升级布局，紧跟 ciphertext 写 10 字节 MAC
	// 升级到 v4 升级布局时，**必须**同时启用 reader 端 MAC 校验（Task 12）。
	EnableHMAC bool
}

// WriteV4Container 将 v4 容器扁平化写入磁盘。
//
// 写入顺序：
//  1. 写入 Header（2048 字节占位，后续回填 ManifestOffset/Length）
//  2. 遍历 SegmentResults，按 v4 升级布局写入：
//     - 加密 Segment（默认）：[Header(34B)][Nonce(16B)][Ciphertext(N B)][HMAC(10B)][SeekTable(var)]
//     - 明文 Segment（ModeFlagEncrypted=0）：[Header(34B)][Plaintext(N B)]
//  3. 写入 DisasterZones（可选，暂未深度集成）
//  4. 写入 Manifest（XOR 混淆）
//  5. 回写 Header（注入 ManifestOffset/Length/CipherMode）
//  6. 写入 Footer（12 字节）
//
// **MacSalt 注入**：当 Manifest.MACSaltBase64 为空时，自动调用
// crypto.GenerateMACSalt() 生成 16 字节随机 mac_salt 并 base64 编码注入。
//
// **CipherMode 注入**：从 params.CipherMode 同步到 header.CipherMode。
func WriteV4Container(params *V4WriteParams) error {
	header, err := types.CreateHeaderV4(params.IsMain, params.ContainerType, params.IsSeekable, params.IDType, params.IDData, params.PasswordHint)
	if err != nil {
		return fmt.Errorf("failed to create v4 header: %w", err)
	}

	// CipherMode 注入：将 params 中的 CipherMode 同步到 header。
	// WriteHeaderV4 内部会规范化非法值到 0，故此处直接赋值即可。
	header.CipherMode = params.CipherMode

	// MacSalt 注入：mac_salt 存于 Manifest（而非 Header），见 header_v4.go 注释。
	// 如果调用方未在 manifest 中预设 mac_salt，自动生成一个 16 字节随机值并注入。
	// 这样保证：
	//   1. 新写入的 v4 容器总是有独立 mac_salt（与 encrypt salt 完全解耦）
	//   2. 调用方可以选择预先设置 Manifest.MACSaltBase64（例如测试场景需要固定值）
	if params.Manifest != nil && params.Manifest.MACSaltBase64 == "" {
		macSalt, err := crypto.GenerateMACSalt()
		if err != nil {
			return fmt.Errorf("failed to generate mac salt: %w", err)
		}
		params.Manifest.MACSaltBase64 = base64.StdEncoding.EncodeToString(macSalt)
	}

	f, err := os.Create(params.OutputPath)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer f.Close()

	if err := types.WriteHeaderV4(f, header); err != nil {
		return fmt.Errorf("failed to write placeholder header: %w", err)
	}

	dataStartOffset := int64(types.EnvelopeHeaderSize_v4)
	globalHasher := crc32.NewIEEE()

	for i, segResult := range params.SegmentResults {
		if err := writeOneSegment(f, globalHasher, i, segResult, params, header); err != nil {
			return err
		}
	}

	for _, dz := range params.DisasterZones {
		srcFile, err := os.Open(params.OutputPath)
		if err != nil {
			return fmt.Errorf("failed to open source for disaster zone %s: %w", dz.Name, err)
		}

		dzData := make([]byte, dz.Size)
		if _, err := srcFile.Seek(dz.Offset, io.SeekStart); err != nil {
			srcFile.Close()
			return fmt.Errorf("failed to seek to disaster zone %s: %w", dz.Name, err)
		}
		if _, err := io.ReadFull(srcFile, dzData); err != nil {
			srcFile.Close()
			return fmt.Errorf("failed to read disaster zone %s: %w", dz.Name, err)
		}
		srcFile.Close()

		if _, err := f.Write(dzData); err != nil {
			return fmt.Errorf("failed to write disaster zone %s: %w", dz.Name, err)
		}
		globalHasher.Write(dzData)
	}

	manifestOffset, err := f.Seek(0, io.SeekCurrent)
	if err != nil {
		return fmt.Errorf("failed to get manifest offset: %w", err)
	}

	manifestJSON, err := params.Manifest.SerializeToJSON_v4()
	if err != nil {
		return fmt.Errorf("failed to serialize manifest: %w", err)
	}

	obfuscatedManifest, err := crypto.ObfuscateManifest(manifestJSON)
	if err != nil {
		return fmt.Errorf("failed to obfuscate manifest: %w", err)
	}

	if _, err := f.Write(obfuscatedManifest); err != nil {
		return fmt.Errorf("failed to write obfuscated manifest: %w", err)
	}
	globalHasher.Write(obfuscatedManifest)

	manifestLength := uint64(len(obfuscatedManifest))

	allDataBuf := make([]byte, manifestOffset-dataStartOffset)
	if _, err := f.Seek(dataStartOffset, io.SeekStart); err != nil {
		return fmt.Errorf("failed to seek to data start for crc: %w", err)
	}
	if _, err := io.ReadFull(f, allDataBuf); err != nil {
		return fmt.Errorf("failed to read data for global crc: %w", err)
	}
	globalCRC32 := crc32.ChecksumIEEE(allDataBuf)

	header.ManifestOffset = uint32(manifestOffset)
	header.ManifestLength = uint32(manifestLength)

	if params.Manifest.OriginalName != "" && params.Manifest.FilenameAlgorithm != "" {
		header.Flags |= types.FlagFilenameEncrypted
	}

	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("failed to seek to header: %w", err)
	}
	if err := types.WriteHeaderV4(f, header); err != nil {
		return fmt.Errorf("failed to rewrite header with manifest info: %w", err)
	}

	if _, err := f.Seek(0, io.SeekEnd); err != nil {
		return fmt.Errorf("failed to seek to end for footer: %w", err)
	}

	footer := &types.EnvelopeFooterV4{
		Magic:       types.MagicFooter_v2,
		GlobalCRC32: globalCRC32,
	}
	if err := types.WriteFooterV4(f, footer); err != nil {
		return fmt.Errorf("failed to write footer: %w", err)
	}

	return nil
}

// writeOneSegment 写入单个 v4 Segment（加密或明文）。
//
// 加密 Segment（ModeFlagEncrypted=1）布局：
//
//	[SegmentHeader(34B)][Nonce(16B)][Ciphertext(DataLength B)][HMAC(10B)][SeekTable(var)]
//
// 明文 Segment（ModeFlagEncrypted=0）布局：
//
//	[SegmentHeader(34B)][Plaintext(DataLength B)]
//
// 根据 params.EnableHMAC 决定是否在密文后追加 10 字节 HMAC。
// params.EnableHMAC=false 时与升级前 v4 布局完全一致（无 HMAC 字节）。
func writeOneSegment(f *os.File, globalHasher hash.Hash32, i int, segResult *crypto.SegmentEncryptionResult, params *V4WriteParams, header *types.EnvelopeHeaderV4) error {
	// 计算布局大小（用于 Manifest 记录）
	encrypted := segResult.ModeFlags&types.ModeFlagEncrypted != 0

	// 1. 构造 SegmentHeader（34 字节扩展布局）
	segHeader := buildSegmentHeader(segResult, params)

	segHeaderBytes, err := segHeader.MarshalBinary()
	if err != nil {
		return fmt.Errorf("failed to marshal segment header %d: %w", i, err)
	}

	if _, err := f.Write(segHeaderBytes); err != nil {
		return fmt.Errorf("failed to write segment header %d: %w", i, err)
	}
	globalHasher.Write(segHeaderBytes)

	// 2. 写入 Segment payload
	if encrypted {
		// 加密 Segment：[Nonce][Ciphertext][HMAC?][SeekTable?]
		if _, err := f.Write(segResult.Nonce); err != nil {
			return fmt.Errorf("failed to write nonce for segment %d: %w", i, err)
		}
		globalHasher.Write(segResult.Nonce)

		if _, err := f.Write(segResult.EncryptedData); err != nil {
			return fmt.Errorf("failed to write encrypted data for segment %d: %w", i, err)
		}
		globalHasher.Write(segResult.EncryptedData)

		// HMAC 写入（v4 升级布局）：仅在 EnableHMAC=true 时追加 10 字节
		if params.EnableHMAC {
			if _, err := f.Write(segResult.HMAC[:]); err != nil {
				return fmt.Errorf("failed to write HMAC for segment %d: %w", i, err)
			}
			globalHasher.Write(segResult.HMAC[:])
		}

		// SeekTable 写入：仅在 zstd 压缩时有内容
		if len(segResult.SeekTable) > 0 {
			if _, err := f.Write(segResult.SeekTable); err != nil {
				return fmt.Errorf("failed to write seek table for segment %d: %w", i, err)
			}
			globalHasher.Write(segResult.SeekTable)
		}
	} else {
		// 明文 Segment：直接写 EncryptedData 字段（语义上即明文）
		// 调用方应在 ModeFlags 中清掉 Encrypted 位
		if _, err := f.Write(segResult.EncryptedData); err != nil {
			return fmt.Errorf("failed to write plaintext segment %d: %w", i, err)
		}
		globalHasher.Write(segResult.EncryptedData)
	}

	// 3. 更新 Manifest.Segments[i] 的 offset/size/nonce
	segOffset, err := f.Seek(0, io.SeekCurrent)
	if err != nil {
		return fmt.Errorf("failed to get current offset after segment %d: %w", i, err)
	}

	segTotalSize := computeSegmentTotalSize(segResult, encrypted, params.EnableHMAC)

	if i < len(params.Manifest.Segments) {
		params.Manifest.Segments[i].Offset = uint64(segOffset) - segTotalSize
		params.Manifest.Segments[i].Size = segTotalSize
		if encrypted {
			params.Manifest.Segments[i].Nonce = base64.StdEncoding.EncodeToString(segResult.Nonce)
		} else {
			params.Manifest.Segments[i].Nonce = ""
		}
	}

	_ = header // reserved for future use
	return nil
}

// buildSegmentHeader 根据 segResult 构造 34 字节扩展 SegmentHeader。
//
// 字段填充规则：
//   - SegmentID:           来自 segResult.SegmentID
//   - DataLength:          len(segResult.EncryptedData)（密文长度，不含 MAC/SeekTable）
//   - NonceSize:           len(segResult.Nonce)（加密时 16，明文时 0）
//   - ModeFlags:           来自 segResult.ModeFlags
//   - MACSize:             HMACSize_v4（10）若 EnableHMAC else 0
//   - DataCRC32:           来自 segResult.DataCRC32
//   - CompressedBlockSize: 压缩时 = DefaultZstdBlockSize else 0
//   - Reserved:            0
//   - SeekTableOffset:     0（写入位置由 writer 隐式确定，reader 可重新计算）
//   - SeekTableLength:     len(segResult.SeekTable)
func buildSegmentHeader(segResult *crypto.SegmentEncryptionResult, params *V4WriteParams) *types.SegmentHeader {
	encrypted := segResult.ModeFlags&types.ModeFlagEncrypted != 0
	compressed := segResult.ModeFlags&types.ModeFlagCompressionZstd != 0

	macSize := uint16(0)
	if encrypted && params.EnableHMAC {
		macSize = crypto.HMACSize_v4
	}

	nonceSize := uint16(0)
	if encrypted {
		nonceSize = uint16(len(segResult.Nonce))
	}

	compressedBlockSize := uint16(0)
	if compressed {
		// CompressedBlockSize 字段是 uint16（最大 65535），而 DefaultZstdBlockSize = 64*1024 = 65536 超出 1。
		// 为避免溢出，存储为 KB 单位（64 KB → 64），reader 端再 * 1024 还原。
		// 上界 65535 KB ≈ 64 MB，覆盖所有合理配置。
		compressedBlockSize = uint16(compression.DefaultZstdBlockSize / 1024)
	}

	return &types.SegmentHeader{
		SegmentID:           segResult.SegmentID,
		DataLength:          uint64(len(segResult.EncryptedData)),
		NonceSize:           nonceSize,
		ModeFlags:           segResult.ModeFlags,
		MACSize:             macSize,
		DataCRC32:           segResult.DataCRC32,
		CompressedBlockSize: compressedBlockSize,
		Reserved:            0,
		SeekTableOffset:     0, // 写入位置隐式确定
		SeekTableLength:     uint32(len(segResult.SeekTable)),
	}
}

// computeSegmentTotalSize 计算一个 Segment 在磁盘上的总字节数（含 Header）。
//
// 加密 Segment 总大小 = SegmentHeaderSize + len(Nonce) + len(EncryptedData) [+ HMACSize] [+ len(SeekTable)]
// 明文 Segment 总大小 = SegmentHeaderSize + len(EncryptedData)（即"明文"长度）
func computeSegmentTotalSize(segResult *crypto.SegmentEncryptionResult, encrypted bool, enableHMAC bool) uint64 {
	size := uint64(types.SegmentHeaderSize)
	if encrypted {
		size += uint64(len(segResult.Nonce))
		size += uint64(len(segResult.EncryptedData))
		if enableHMAC {
			size += crypto.HMACSize_v4
		}
		size += uint64(len(segResult.SeekTable))
	} else {
		size += uint64(len(segResult.EncryptedData))
	}
	return size
}

// WriteV4EmptyContainer 是空容器的便捷写入函数（无 Segment 无 DisasterZone）。
// 内部直接转发到 WriteV4Container。
func WriteV4EmptyContainer(params *V4WriteParams) error {
	params.SegmentResults = nil
	params.DisasterZones = nil
	return WriteV4Container(params)
}
