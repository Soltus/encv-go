package types

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"io"
)

const (
	EnvelopeHeaderSize_v4 = 2048
	EnvelopeFooterSize_v4 = 12
	SpecialIDMaxLenV4     = 1992
)

const (
	ContainerTypeUnknown  uint16 = 0
	ContainerTypeVideo    uint16 = 1
	ContainerTypeAudio    uint16 = 2
	ContainerTypeImage    uint16 = 3
	ContainerTypeDocument uint16 = 4
	ContainerTypeText     uint16 = 5
)

// EnvelopeHeaderV4 是 ENCV v4 容器（.sccg* 系列）的固定大小信封头（2048 字节）。
//
// 字段布局（所有偏移相对容器起始）：
//
//	Offset  Size  Field           说明
//	0       4     Magic           容器魔数 'E','N','C','V'（详见 MagicHeader_v2）
//	4       2     Version         容器版本号，当前为 0x04
//	6       2     Flags           主容器/物理分片/文件名加密等位标志
//	8       2     ContainerType   video/audio/image/document/text
//	10      1     IsSeekable      0/1 标识
//	11      1     Reserved1       保留
//	12      4     IDType          ID 编码类型（Raw/CBOR/...）
//	16      4     IDLength        ID 实际有效长度
//	20      16    PasswordHint    密码提示（用于快速密码错误感知）
//	36      1992  SpecialID       业务元数据/占位 ID
//	2028    4     ManifestOffset  Manifest 在容器中的字节偏移
//	2032    4     ManifestLength  Manifest 长度
//	2036    4     HeaderCRC32     头部 CRC32-IEEE（计算范围 [0, 2036)）
//	2040    2     CipherMode      v4 新增：AES 密钥长度（0=AES-128, 1=AES-256）
//	2042    6     Reserved2       保留填充至 2048 字节
//	Total:  2048 bytes
//
// 兼容性设计：
//   - CipherMode 复用了原本 Reserved2 区域的前 2 字节，不影响任何已分配字段偏移
//   - HeaderCRC32 的计算范围 [0, 2036) 不变，CRC 校验不破坏磁盘格式
//   - 旧 v4 容器（无 CipherMode 字段）的该位置为 0x0000 → 读为 CipherModeAES128CTR (0)
//     注意：旧 v4 容器原本硬编码 AES-256-CTR，升级后按 0 解析为 AES-128 是 spec 接受的
//     trade-off（参见 v4-container-capability-upgrade spec）
//
// MacSalt 字段不在 Header 中（重要！）：
//   - spec 原文要求 mac_salt 存于 v4 Header 偏移 36-52，但该位置被 PasswordHint (20-36)
//     和 SpecialID (36-2028) 完全占据，物理上无法再插入 16 字节
//   - **因此采取实际可行方案**：mac_salt 存于 Manifest（v4 容器的可变长元数据区域）
//   - 读取器通过 Manifest.MACSaltBase64 字段取出，base64 解码后调用 crypto.DeriveMACKey
//   - 旧 v4 容器（无 mac_salt 字段）的 Manifest 中 MACSaltBase64 = "" → fallback 到
//     encrypt salt（KVI.salt_base64）
//   - 详见 Manifest_v4.MACSaltBase64 字段文档
//   - HasMACSalt() helper 集中实现"是否存在 mac_salt"的判断逻辑
type EnvelopeHeaderV4 struct {
	Magic          [4]byte
	Version        uint16
	Flags          uint16
	ContainerType  uint16
	IsSeekable     uint8
	Reserved1      uint8
	IDType         uint32
	IDLength       uint32
	PasswordHint   [16]byte
	SpecialID      [SpecialIDMaxLenV4]byte
	ManifestOffset uint32
	ManifestLength uint32
	HeaderCRC32    uint32
	// CipherMode 标识 v4 容器使用的 AES 密钥长度（CTR 模式）。
	// 字段值定义见 internal/v2/crypto.CipherMode_v4。
	// 默认值 0 = AES-128-CTR（v4 新默认）。
	// 旧 v4 容器（无此字段）该位置为 0x0000，读时 fallback 到 0 = AES-128-CTR。
	CipherMode uint16
	Reserved2  [6]byte
}

type EnvelopeFooterV4 struct {
	Magic       [4]byte
	GlobalCRC32 uint32
	Reserved    [4]byte
}

// CipherModeOffsetV4 给出 CipherMode 字段在 v4 Header 二进制中的字节偏移。
// 设计说明：复用了原本 Reserved2 区域的前 2 字节（offset 2040-2042），
// 这样：
//   - 所有已分配字段（Magic/Version/Flags/.../HeaderCRC32）的偏移不变
//   - HeaderCRC32 的计算范围 [0, 2036) 不变（不覆盖 CipherMode/Reserved2）
//   - 旧 v4 容器的 CipherMode 位置 = 0x0000（Go 结构体零值），读时按 AES-128 解析
const (
	CipherModeOffsetV4 = 2040
	HeaderCRC32EndV4   = 2040 // HeaderCRC32 写入结束位置（不包含 CipherMode 区域）
)

// WriteHeaderV4 将 EnvelopeHeaderV4 序列化为 2048 字节写入 w。
//
// 字节布局（与 EnvelopeHeaderV4 字段布局一一对应）：
//
//	[0, 4)        Magic
//	[4, 6)        Version
//	[6, 8)        Flags
//	[8, 10)       ContainerType
//	[10, 11)      IsSeekable
//	[11, 12)      Reserved1
//	[12, 16)      IDType
//	[16, 20)      IDLength
//	[20, 36)      PasswordHint
//	[36, 2028)    SpecialID
//	[2028, 2032)  ManifestOffset
//	[2032, 2036)  ManifestLength
//	[2036, 2040)  HeaderCRC32（计算范围 [0, 2036) 的 CRC32-IEEE）
//	[2040, 2042)  CipherMode（v4 新增，0=AES-128-CTR / 1=AES-256-CTR）
//	[2042, 2048)  Reserved2
//
// CipherMode 字段值会在写入前被规范化：仅接受 {0, 1}，其他值 fallback 到 0。
// 这一规范化保证磁盘上不会出现非法 CipherMode 值。
func WriteHeaderV4(w io.Writer, h *EnvelopeHeaderV4) error {
	h.HeaderCRC32 = 0

	buf := make([]byte, EnvelopeHeaderSize_v4)

	copy(buf[0:4], h.Magic[:])
	binary.LittleEndian.PutUint16(buf[4:6], h.Version)
	binary.LittleEndian.PutUint16(buf[6:8], h.Flags)
	binary.LittleEndian.PutUint16(buf[8:10], h.ContainerType)
	buf[10] = h.IsSeekable
	buf[11] = h.Reserved1
	binary.LittleEndian.PutUint32(buf[12:16], h.IDType)
	binary.LittleEndian.PutUint32(buf[16:20], h.IDLength)
	copy(buf[20:36], h.PasswordHint[:])
	copy(buf[36:36+SpecialIDMaxLenV4], h.SpecialID[:])
	binary.LittleEndian.PutUint32(buf[2028:2032], h.ManifestOffset)
	binary.LittleEndian.PutUint32(buf[2032:2036], h.ManifestLength)

	// CRC32 计算范围是 [0, 2036)，不包含 HeaderCRC32 / CipherMode / Reserved2
	crc := crc32.ChecksumIEEE(buf[:EnvelopeHeaderSize_v4-12])
	binary.LittleEndian.PutUint32(buf[2036:2040], crc)
	h.HeaderCRC32 = crc

	// CipherMode 字段：写入前规范化到 {0, 1}，避免磁盘上出现非法值
	cipherMode := normalizeCipherModeV4(h.CipherMode)
	h.CipherMode = cipherMode
	binary.LittleEndian.PutUint16(buf[CipherModeOffsetV4:CipherModeOffsetV4+2], cipherMode)

	// Reserved2 序列化（结构体中为 6 字节，偏移 2042-2048）
	copy(buf[CipherModeOffsetV4+2:EnvelopeHeaderSize_v4], h.Reserved2[:])

	_, err := w.Write(buf)
	return err
}

// ReadHeaderV4 从 r 读取 2048 字节并解析为 EnvelopeHeaderV4。
//
// 处理流程：
//  1. 读取 2048 字节
//  2. 校验 HeaderCRC32（范围 [0, 2036)）
//  3. binary.Read 反序列化为结构体
//  4. 校验 Magic == MagicHeader_v2
//  5. 规范化 CipherMode：非 {0, 1} 值 fallback 到 0
func ReadHeaderV4(r io.Reader) (*EnvelopeHeaderV4, error) {
	buf := make([]byte, EnvelopeHeaderSize_v4)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, fmt.Errorf("failed to read header bytes: %w", err)
	}

	storedCRC := binary.LittleEndian.Uint32(buf[2036:2040])
	calculatedCRC := crc32.ChecksumIEEE(buf[:2036])
	if storedCRC != calculatedCRC {
		return nil, fmt.Errorf("header CRC32 mismatch: stored=%08x, calculated=%08x", storedCRC, calculatedCRC)
	}

	header := &EnvelopeHeaderV4{}
	if err := binary.Read(bytes.NewReader(buf), binary.LittleEndian, header); err != nil {
		return nil, fmt.Errorf("failed to unmarshal header: %w", err)
	}

	if header.Magic != MagicHeader_v2 {
		return nil, ErrInvalidMagic_v2
	}

	// 向后兼容：旧 v4 容器的 CipherMode 位置 = 0x0000（结构体零值），
	// 读时按 0 = AES-128-CTR 解析。这是 spec 接受的 trade-off。
	// 同时对明显的非法值（如 0xFFFF）也 fallback 到 0，避免下游逻辑崩溃。
	header.CipherMode = normalizeCipherModeV4(header.CipherMode)

	return header, nil
}

// normalizeCipherModeV4 将任意 uint16 输入规范化到合法 CipherMode 值 {0, 1}。
// 非法输入（包括 0xFFFF 等明显无效值）fallback 到 0 = AES-128-CTR。
//
// 这一函数是 v4 Header 读/写两侧的统一入口，保证：
//   - 磁盘上不会出现非法 CipherMode
//   - 旧 v4 容器（Reserved2 区域零值）按 0 解析为 AES-128-CTR
//   - 错误填充的 CipherMode 不会让下游出现意外密钥长度
func normalizeCipherModeV4(mode uint16) uint16 {
	switch mode {
	case 0, 1:
		return mode
	default:
		return 0
	}
}

// HasMACSalt 检查 macSalt 是否为"存在"状态（即非全零且非空）。
//
// 这是 MacSalt 字段向后兼容判断的统一入口，原因：
//   - mac_salt 改存于 Manifest.MACSaltBase64（base64 编码字符串）
//   - 旧 v4 容器的 Manifest JSON 没有 mac_salt_base64 字段 → 反序列化后 MACSaltBase64 = ""
//   - 调用方对 "" 字符串的判断方式必须统一（避免每个调用方都重复实现边界判断）
//
// 返回 false 的三种情况：
//  1. macSalt 为 nil（解码失败或未设置）
//  2. macSalt 长度为 0（异常防御）
//  3. macSalt 全为零字节（结构体零值 / 显式清零的 [16]byte{}）
//
// 返回 true 的充要条件：macSalt 至少有一个字节非零。
//
// 用法：
//
//	if types.HasMACSalt(macSalt) {
//	    key := crypto.DeriveMACKey(password, macSalt)
//	} else {
//	    // 旧 v4 容器：fallback 到 encrypt salt
//	    key := crypto.DeriveMACKey(password, encryptSalt)
//	}
func HasMACSalt(macSalt []byte) bool {
	if len(macSalt) == 0 {
		return false
	}
	for _, b := range macSalt {
		if b != 0 {
			return true
		}
	}
	return false
}

func WriteFooterV4(w io.Writer, f *EnvelopeFooterV4) error {
	buf := make([]byte, EnvelopeFooterSize_v4)

	copy(buf[0:4], f.Magic[:])
	binary.LittleEndian.PutUint32(buf[4:8], f.GlobalCRC32)
	copy(buf[8:12], f.Reserved[:])

	_, err := w.Write(buf)
	return err
}

func ReadFooterV4(r io.Reader) (*EnvelopeFooterV4, error) {
	buf := make([]byte, EnvelopeFooterSize_v4)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, fmt.Errorf("failed to read footer bytes: %w", err)
	}

	footer := &EnvelopeFooterV4{}
	if err := binary.Read(bytes.NewReader(buf), binary.LittleEndian, footer); err != nil {
		return nil, fmt.Errorf("failed to unmarshal footer: %w", err)
	}

	if footer.Magic != MagicFooter_v2 {
		return nil, ErrInvalidMagic_v2
	}

	return footer, nil
}

func CreateHeaderV4(isMain bool, containerType uint16, isSeekable bool, idType IDType, idData []byte, passwordHint [16]byte) (*EnvelopeHeaderV4, error) {
	if idData == nil {
		if idType == IDType_Raw {
			idData = make([]byte, SpecialIDMaxLenV4)
			if _, err := rand.Read(idData); err != nil {
				return nil, fmt.Errorf("failed to generate placeholder ID: %w", err)
			}
		} else {
			return nil, fmt.Errorf("idData is required for IDType %d", idType)
		}
	}

	if len(idData) > SpecialIDMaxLenV4 {
		return nil, fmt.Errorf("special ID data exceeds max length %d", SpecialIDMaxLenV4)
	}

	flags := uint16(0)
	if isMain {
		flags |= FlagIsMainContainer
	} else {
		flags |= FlagIsPhysicalChunk
	}

	seekable := uint8(0)
	if isSeekable {
		seekable = 0x01
	}

	header := &EnvelopeHeaderV4{
		Magic:         MagicHeader_v2,
		Version:       0x04,
		Flags:         flags,
		ContainerType: containerType,
		IsSeekable:    seekable,
		IDType:        uint32(idType),
		PasswordHint:  passwordHint,
	}
	copy(header.SpecialID[:], idData)
	header.IDLength = uint32(len(idData))

	return header, nil
}
