// internal/v2/crypto/segment_crypto.go
// v4 Segment 加解密模块（Encrypt-then-MAC 模式 + 可选 zstd 压缩）
//
// 本文件实现 ENCV v4 容器的 Segment 级别加解密，遵循 WinZip AE-2 规范的
// "Encrypt-then-MAC" 顺序：先 zstd 压缩（可选）→ AES-CTR 加密 → 对
// nonce||ciphertext 计算 HMAC-SHA1-80（10 字节截断）作为完整性保护。
//
// 安全保证（防 CTR 模式比特翻转攻击 / 防 zstd 解压炸弹）：
//   1. 解密前 **必须** 校验 MAC，失败立即返回 ErrMACMismatch，不解 CTR 也不解压
//   2. nonce 也参与 HMAC 计算，防 nonce 翻转
//   3. MAC 校验使用 crypto/subtle.ConstantTimeCompare 防侧信道
//   4. HMAC 校验失败的 Segment **绝不解压**（防 zstd 解压炸弹攻击 DoS）
//
// 磁盘格式（v4 Segment 内层布局，由本模块输出/消费）：
//
//	[Nonce(16B)][Ciphertext(N B)][HMAC-SHA1-80(10B)]
//
// 字段说明：
//   - Nonce:        AES-CTR 初始向量（16 字节 = aes.BlockSize）
//   - Ciphertext:   AES-CTR 输出（与"加密前的数据"等长——若启用 zstd，则为 zstd 压缩数据）
//   - HMAC:         HMAC-SHA1-80(macKey, nonce || ciphertext) 的前 10 字节
//
// 数据流方向（完整 pipeline）：
//
//	Encrypt:  plaintext → [zstd compress 可选] → AES-CTR → HMAC
//	Decrypt:  HMAC verify → AES-CTR → [zstd decompress 可选] → plaintext
//
// 设计参考：
//   - spec.md "Requirement: HMAC-SHA1-80 完整性保护（Encrypt-then-MAC）"
//   - spec.md "Requirement: MAC 计算顺序（Encrypt-then-MAC）"
//   - spec.md "Requirement: MAC 校验前置（解密前）"
//   - spec.md "Requirement: Segment 集成 zstd 压缩" (Task 9)
package crypto

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"log"

	"github.com/Soltus/encv-go/internal/v2/crypto/compression"
	"github.com/Soltus/encv-go/internal/v2/types"
)

// CompressionMode 标识 v4 Segment 数据通道使用的压缩算法。
//
// 字符串枚举（与 v4 配置文件 v4_compression_mode 直接对应）：
//   - CompressionModeNone  = "none"：不压缩，明文 / 密文 1:1
//   - CompressionModeZstd  = "zstd"：使用 seekable zstd 压缩（64KB 块大小）
//
// 为避免与 types.ModeFlagCompressionZstd 数值常量混淆，这里使用字符串枚举：
// 配置文件 schema、调用方 API、Segment 写入路径都按字符串传递。
const (
	// CompressionModeNone 显式不压缩。
	CompressionModeNone = "none"
	// CompressionModeZstd 使用 seekable zstd 压缩。
	CompressionModeZstd = "zstd"
)

// MinimumCompressionSize 是启用 zstd 压缩的最小明文大小（字节）。
//
// 设计依据：
//   - 低于 1KB 的数据走 zstd 的成本（CPU + 帧头 overhead）可能高于节省的空间
//   - zstd-seekable 至少要产 1 帧 + 1 seek-table 帧，约 30+ 字节固定开销
//   - < 1KB 压缩常常是负收益（压缩后比原数据更大）
//   - 1KB 阈值与行业惯例（zstd CLI 默认值、7z 字典阈值）一致
//
// 此阈值由 spec 显式定义，调用方无需关心。`EncryptSegment` 内部自动判断。
const MinimumCompressionSize = 1024

// ErrCompressionFailed 是 zstd 压缩失败的哨兵错误。
//
// 重要：`EncryptSegment` **不会** 直接返回此错误。压缩失败时它会**降级**
// 为明文加密（不压缩），同时 log 一条 warn 消息。此错误仅在调用方需要
// 显式捕获压缩失败时使用（例如测试中）。
var ErrCompressionFailed = errors.New("zstd compression failed")

// ErrDecompressionFailed 是 zstd 解压失败的错误。
//
// 与压缩失败不同，解压失败**直接返回 error**（不静默）——因为：
//   1. 解压失败意味着磁盘数据被破坏或 seek table 不匹配，是不可恢复的错误
//   2. 静默返回 "未解压数据" 会让调用方把 zstd 压缩字节误当明文用
//   3. 配合 MAC 校验，可以保证解压路径只在"数据完整"前提下执行
var ErrDecompressionFailed = errors.New("zstd decompression failed")

// compressZstdSeekableFunc 是 compression.CompressZstdSeekable 的可替换引用，
// 默认为 compression.CompressZstdSeekable。测试中可临时替换以模拟压缩失败
// （用于 TestEncryptSegment_CompressionFailure_GracefulDegrade）。
//
// 这是 Go 测试中的"依赖注入"惯用法——避免在压缩包内引入 mock 框架。
var compressZstdSeekableFunc = compression.CompressZstdSeekable

// decompressZstdSeekableFunc 同理，用于测试中模拟解压失败。
var decompressZstdSeekableFunc = compression.DecompressZstdSeekable

// ErrSegmentTooShort 在解密数据长度 < HMACSize_v4 时返回（缺少 trailing MAC）。
var ErrSegmentTooShort = errors.New("segment ciphertext too short: missing trailing HMAC")

// SegmentEncryptionResult 封装一个 v4 Segment 加密结果。
//
// 字段语义：
//   - SegmentID:     Segment 唯一标识（用于 playlist / 重排定位）
//   - Nonce:         AES-CTR IV（16 字节）
//   - EncryptedData: 纯密文（不含 nonce / MAC），长度 = len(输入压缩后数据)
//   - DataCRC32:     密文 CRC32（可选二次校验，不替代 MAC）
//   - HMAC:          HMAC-SHA1-80(macKey, nonce || EncryptedData)[:10]
//   - Compressed:    是否经过 zstd 压缩（true=压缩过，false=不压缩）
//   - SeekTable:     zstd seek table 字节切片（压缩时非空，用于 reader 重建索引）
//   - ModeFlags:     对应 SegmentHeader.ModeFlags 位字段
//                    (ModeFlagEncrypted | ModeFlagCompressionZstd if Compressed)
//
// 磁盘布局：`Nonce(16B) || EncryptedData(N B) || HMAC(10B)`。
// Compressed / SeekTable / ModeFlags 不进磁盘格式，仅供 writer 写入
// SegmentHeader（见 internal/v2/types/segment_v4.go）。
type SegmentEncryptionResult struct {
	SegmentID     uint32
	Nonce         []byte
	EncryptedData []byte
	DataCRC32     uint32
	HMAC          [HMACSize_v4]byte

	// Compressed 表示该 Segment 是否经过 zstd 压缩。
	// true → EncryptedData 是 zstd 压缩数据的密文；解压需配合 SeekTable
	// false → EncryptedData 是明文的密文；解密即得 plaintext
	Compressed bool

	// SeekTable 是 zstd-seekable 的 "skippable frame"，由 CompressZstdSeekable
	// 返回。仅在 Compressed=true 时非空。reader 用它重建索引后随机访问。
	SeekTable []byte

	// ModeFlags 对应 SegmentHeader.ModeFlags 位字段（types.ModeFlag*）。
	// 当前实现固定为 ModeFlagEncrypted（| ModeFlagCompressionZstd if Compressed）。
	// 写入方应直接拷贝到 segHeader.ModeFlags。
	ModeFlags uint16
}

// EncryptSegment 使用 AES-CTR 加密 plainData，并按 Encrypt-then-MAC 顺序追加
// HMAC-SHA1-80 截断值到结果。可选地先对 plainData 做 zstd 压缩（取决于
// compressionMode 与数据大小）。
//
// 参数：
//   - plainData:       明文（任意长度，CTR 模式无需填充）
//   - key:             AES 密钥（16/24/32 字节，与 CipherMode 配套）
//   - macKey:          HMAC-SHA1-80 用的独立 MAC 密钥（推荐 32 字节）
//   - segmentID:       Segment 唯一标识，仅用于错误信息
//   - compressionMode: 压缩模式（"none" / "zstd"）。其他值按 "none" 处理
//
// 返回：
//   - *SegmentEncryptionResult：包含 nonce / 密文 / CRC32 / HMAC / 压缩元数据
//   - error：仅在 cipher.NewCipher 失败（key 长度非法）时返回
//
// 计算顺序（Encrypt-then-MAC，可选 zstd 前置）：
//  1. 决策是否压缩：compressionMode == "zstd" 且 len(plainData) >= 1KB
//  2. 若压缩：调 compression.CompressZstdSeekable → (compressed, seekTable)
//     - 失败时**降级**为明文加密（不返回 error，warn log）
//  3. AES-CTR 加密 [压缩后] 数据 → ciphertext
//  4. 计算 MAC = HMACSHA1_80(macKey, nonce || ciphertext)[:10]
//  5. 装填到 SegmentEncryptionResult（含 ModeFlags / Compressed / SeekTable）
//
// 重要：macKey 为 nil 时 HMACSHA1_80 仍会计算（hmac.New 接受 nil key），
// 但语义上是"无 MAC 派生"，调用方必须确保 macKey 来自独立 PBKDF2 派生通道。
func EncryptSegment(plainData []byte, key []byte, macKey []byte, segmentID uint32, compressionMode string) (*SegmentEncryptionResult, error) {
	// 步骤 1：决策是否压缩
	shouldCompress := compressionMode == CompressionModeZstd && len(plainData) >= MinimumCompressionSize

	// dataToEncrypt 是进入 AES-CTR 的字节切片（明文 或 zstd 压缩后字节）
	dataToEncrypt := plainData
	compressed := false
	var seekTable []byte

	if shouldCompress {
		c, st, compErr := compressZstdSeekableFunc(bytes.NewReader(plainData))
		if compErr != nil {
			// 降级：warn log 后用明文加密，不返回 error
			log.Printf("[EncryptSegment] segment %d: zstd compression failed (%v), degrade to plaintext encryption", segmentID, compErr)
		} else {
			dataToEncrypt = c
			seekTable = st
			compressed = true
		}
	}

	// 步骤 2：生成 nonce + AES-CTR 加密
	nonce, err := GenerateIV_v2(IVSize_v2)
	if err != nil {
		return nil, fmt.Errorf("failed to generate nonce for segment %d: %w", segmentID, err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher block for segment %d: %w", segmentID, err)
	}

	stream := cipher.NewCTR(block, nonce)
	encryptedData := make([]byte, len(dataToEncrypt))
	stream.XORKeyStream(encryptedData, dataToEncrypt)

	dataCRC32 := crc32.ChecksumIEEE(encryptedData)

	// 步骤 3：Encrypt-then-MAC：MAC = HMACSHA1_80(macKey, nonce || ciphertext)
	// nonce 也必须进 HMAC，防 attacker 整体替换 nonce 后重放旧密文
	macInput := make([]byte, 0, len(nonce)+len(encryptedData))
	macInput = append(macInput, nonce...)
	macInput = append(macInput, encryptedData...)
	macSum := HMACSHA1_80(macKey, macInput)

	// 步骤 4：组装 ModeFlags
	modeFlags := uint16(types.ModeFlagEncrypted)
	if compressed {
		modeFlags |= types.ModeFlagCompressionZstd
	}

	return &SegmentEncryptionResult{
		SegmentID:     segmentID,
		Nonce:         nonce,
		EncryptedData: encryptedData,
		DataCRC32:     dataCRC32,
		HMAC:          macSum,
		Compressed:    compressed,
		SeekTable:     seekTable,
		ModeFlags:     modeFlags,
	}, nil
}

// DecryptSegment 校验 HMAC 后解密 AES-CTR 密文，可选执行 zstd 解压。
//
// **严格顺序（先 MAC → 后解密 → 最后解压）**：
//  1. 校验 nonce 长度（必须是 aes.BlockSize = 16）
//  2. 校验 mac 长度（必须严格 == HMACSize_v4 = 10）
//  3. 重新计算 expected_mac = HMACSHA1_80(macKey, nonce || encryptedData)
//  4. ConstantTimeCompare(expected_mac, mac)
//  5. 失败 → 立即返回 ErrMACMismatch（**不解 CTR，不解压**）
//  6. 成功 → AES-CTR 解密 → 若 compressionMode=="zstd" && len(seekTable)>0 则解压
//
// 参数：
//   - encryptedData:   纯密文（**不含** trailing MAC），由 reader 按 SegmentHeader.DataLength 切出
//   - nonce:           AES-CTR IV（16 字节）
//   - key:             AES 密钥（16/24/32 字节）
//   - macKey:          HMAC-SHA1-80 用的独立 MAC 密钥
//   - mac:             存储的 trailing 10 字节 HMAC（由 reader 从 MACSize 字段定位切出）
//   - compressionMode: 压缩模式（"none" / "zstd"）。"zstd" 时执行解压
//   - seekTable:       zstd seek table 字节切片（压缩时必传，否则解压失败）
//
// 返回：
//   - plaintext:     解密（且可选解压）后的明文
//   - error:         ErrInvalidIVLength_v2 / ErrSegmentTooShort / ErrMACMismatch / ErrDecompressionFailed
//
// **安全约束**：MAC 校验失败时**绝不解密**也**绝不解压**。即使解压失败也会
// 返回明确 error（不静默），让调用方立即发现数据损坏。
func DecryptSegment(encryptedData []byte, nonce []byte, key []byte, macKey []byte, mac []byte, compressionMode string, seekTable []byte) ([]byte, error) {
	if len(nonce) != aes.BlockSize {
		return nil, ErrInvalidIVLength_v2
	}

	if len(mac) != HMACSize_v4 {
		return nil, ErrSegmentTooShort
	}

	// 步骤 1：先计算 expected MAC（**不解密、不解压**）
	macInput := make([]byte, 0, len(nonce)+len(encryptedData))
	macInput = append(macInput, nonce...)
	macInput = append(macInput, encryptedData...)

	// 步骤 2：常量时间校验 MAC，失败立即返回（不解 CTR 不解压）
	if !VerifyHMACSHA1_80(mac, macInput, macKey) {
		return nil, ErrMACMismatch
	}

	// 步骤 3：MAC 校验通过后才执行 AES-CTR 解密
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher block: %w", err)
	}

	stream := cipher.NewCTR(block, nonce)
	plaintextEncrypted := make([]byte, len(encryptedData))
	stream.XORKeyStream(plaintextEncrypted, encryptedData)

	// 步骤 4：可选 zstd 解压
	if compressionMode == CompressionModeZstd && len(seekTable) > 0 {
		dec, decErr := decompressZstdSeekableFunc(plaintextEncrypted, seekTable)
		if decErr != nil {
			return nil, fmt.Errorf("%w: %v", ErrDecompressionFailed, decErr)
		}
		return dec, nil
	}

	return plaintextEncrypted, nil
}

// EncryptStreamToSegments 将 src 切分为固定大小的 Segment，逐个调用
// EncryptSegment 加密，全部用同一对 (key, macKey) 派生加密通道与 MAC 通道。
//
// 参数：
//   - src:             输入流
//   - key:             AES 密钥（16/24/32 字节）
//   - macKey:          HMAC-SHA1-80 用的 MAC 密钥
//   - segmentSize:     单个 Segment 的明文最大字节数
//   - compressionMode: 压缩模式（"none" / "zstd"）传递给每个 EncryptSegment
//
// 返回：每个 Segment 独立 nonce + 密文 + HMAC + 压缩元数据，可直接由 writer 拼成
//
//	[SegmentHeader][Nonce(16B)][Ciphertext][HMAC(10B)]
func EncryptStreamToSegments(src io.Reader, key []byte, macKey []byte, segmentSize int64, compressionMode string) ([]*SegmentEncryptionResult, error) {
	if segmentSize <= 0 {
		return nil, fmt.Errorf("invalid segmentSize: %d", segmentSize)
	}

	var results []*SegmentEncryptionResult
	buf := make([]byte, segmentSize)
	segmentID := uint32(0)

	for {
		n, err := io.ReadFull(src, buf)
		if err != nil {
			if err == io.EOF {
				break
			}
			if err == io.ErrUnexpectedEOF {
				if n == 0 {
					break
				}
				result, encErr := EncryptSegment(buf[:n], key, macKey, segmentID, compressionMode)
				if encErr != nil {
					return nil, fmt.Errorf("failed to encrypt segment %d: %w", segmentID, encErr)
				}
				results = append(results, result)
				break
			}
			return nil, fmt.Errorf("failed to read segment %d: %w", segmentID, err)
		}

		result, encErr := EncryptSegment(buf[:n], key, macKey, segmentID, compressionMode)
		if encErr != nil {
			return nil, fmt.Errorf("failed to encrypt segment %d: %w", segmentID, encErr)
		}
		results = append(results, result)
		segmentID++
	}

	return results, nil
}
