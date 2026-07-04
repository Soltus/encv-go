package types

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"io"

	"github.com/fxamacker/cbor/v2"
)

// IDType 定义 ID 的编码类型，用于解析器判断
type IDType uint32

const (
	EnvelopeHeaderSize_v3        = 2048      // 新的固定头部大小：2KB
	SpecialIDMaxLen              = 2000      // 特殊 ID 的最大物理容量
	IDType_Raw            IDType = 0         // 原始二进制/哈希（占位符或不可逆 ID）
	IDType_CBOR           IDType = 1         // CBOR 编码的结构化元数据（可逆）
	IDType_Reserved       IDType = 2         // 预留给 JSON、Protobuf 等
	FlagIsMainContainer   uint16 = 1 << iota // Bit 0: 这是一个主容器文件
	FlagIsPhysicalChunk                      // Bit 1: 这是一个物理数据分片
	FlagFilenameEncrypted                    // Bit 4: Manifest.original_name 是 ENC-FN 编码存储的
)

var ErrInvalidHeader = errors.New("invalid header")

// EnvelopeHeaderV3 v3 版本的信封头部（2048 字节）
type EnvelopeHeaderV3 struct {
	Magic   [4]byte
	Version uint16 // 0x03
	Flags   uint16 // 标志位：区分主文件与分片

	IDType   uint32  // ID 编码类型 (0=Raw, 1=CBOR)
	IDLength uint32  // ID 的实际有效长度（<= 2000）
	_        [8]byte // 预留给未来的扩展控制标志

	SpecialID [SpecialIDMaxLen]byte // 足够存储极大的元数据，如长字符串许可号、多级 JSON/CBOR 结构等

	HeaderCRC32 uint32   // 头部本身的 CRC32
	_           [20]byte // 填充至 2048 字节，未雨绸缪
}

// GetHeaderSize 根据版本号返回对应的 Header 大小
// 这是一个辅助函数，用于统一处理 V2/V3 Header 的跳过逻辑
func GetHeaderSize(version int) int64 {
	switch version {
	case 4:
		return EnvelopeHeaderSize_v4
	case 3:
		return EnvelopeHeaderSize_v3
	case 2:
		return EnvelopeHeaderSize_v2
	default:
		return 0
	}
}

// DetectHeaderInfoFromReaderAt 从 io.ReaderAt (通常是 *os.File) 读取头部
// 并返回版本号和头部大小。
// 它假设从偏移量 0 开始读取。
func DetectHeaderInfoFromReaderAt(ra io.ReaderAt) (version int, headerSize int64, err error) {
	// 读取前 6 字节 (4B Magic + 2B Version)
	headerBytes := make([]byte, 6)
	if _, err := ra.ReadAt(headerBytes, 0); err != nil {
		return 0, 0, err
	}

	version = DetectHeaderVersion(headerBytes)
	headerSize = GetHeaderSize(version)

	if headerSize == 0 {
		return 0, 0, ErrInvalidHeader
	}

	return version, headerSize, nil
}

// DetectHeaderInfoFromBytes 从字节数组直接解析头部信息
// 用于远程流或内存中的数据
func DetectHeaderInfoFromBytes(data []byte) (version int, headerSize int64, err error) {
	if len(data) < 6 {
		return 0, 0, io.ErrUnexpectedEOF
	}

	version = DetectHeaderVersion(data)
	headerSize = GetHeaderSize(version)

	if headerSize == 0 {
		return 0, 0, ErrInvalidHeader
	}

	return version, headerSize, nil
}

// CreateHeaderV3 创建 V3 头部的工厂函数
// isMain: true=主容器, false=物理分片
// idType: ID 编码类型
// idData: ID 内容，如果为 nil 且 idType 为 IDType_Raw，则自动生成占位符
func CreateHeaderV3(isMain bool, idType IDType, idData []byte) (*EnvelopeHeaderV3, error) {
	// 1. 处理 ID 数据（如果未提供且是 Raw 类型，自动生成）
	if idData == nil {
		if idType == IDType_Raw {
			var err error
			idData, err = GeneratePlaceholderIDV3()
			if err != nil {
				return nil, fmt.Errorf("failed to generate placeholder ID: %w", err)
			}
		} else {
			// 如果是 CBOR 但没给数据，返回错误或构建空对象，这里选择报错
			return nil, fmt.Errorf("idData is required for IDType %d", idType)
		}
	}

	// 2. 设置标志位
	flags := uint16(0)
	if isMain {
		flags |= FlagIsMainContainer
	} else {
		flags |= FlagIsPhysicalChunk
	}

	// 3. 构建结构
	return BuildHeaderV3(flags, idType, idData)
}

// WriteHeaderV3 将头部结构写入指定的 writer
// 注意：它会先计算 CRC32 并写入结构体，然后序列化
func WriteHeaderV3(w io.Writer, h *EnvelopeHeaderV3) error {
	// 1. 清空 HeaderCRC32 字段以进行计算（虽然结构体刚创建时是0，但为了安全）
	// 我们计算的是前 2048 - 4 字节的 CRC
	h.HeaderCRC32 = 0

	// 2. 序列化为字节流（不含最后的 CRC32 字段）
	buf := make([]byte, EnvelopeHeaderSize_v3)

	// 写入固定部分
	copy(buf[0:4], h.Magic[:])
	binary.LittleEndian.PutUint16(buf[4:6], h.Version)
	binary.LittleEndian.PutUint16(buf[6:8], h.Flags)

	binary.LittleEndian.PutUint32(buf[8:12], h.IDType)
	binary.LittleEndian.PutUint32(buf[12:16], h.IDLength)
	// 16-24 Reserved

	copy(buf[24:24+SpecialIDMaxLen], h.SpecialID[:])

	// 2024-2048 Reserved (最后4字节是CRC)

	// 3. 计算前 2044 字节的 CRC32
	crc := crc32.ChecksumIEEE(buf[:EnvelopeHeaderSize_v3-4])

	// 4. 将 CRC 写入 buffer 和 结构体
	binary.LittleEndian.PutUint32(buf[EnvelopeHeaderSize_v3-4:EnvelopeHeaderSize_v3], crc)
	h.HeaderCRC32 = crc

	// 5. 写入 Writer
	_, err := w.Write(buf)
	return err
}

// ReadHeaderV3 从 Reader 中读取 V3 头部
// 它会自动校验 CRC32 和 Magic Number
func ReadHeaderV3(r io.Reader) (*EnvelopeHeaderV3, error) {
	// 1. 读取固定长度的头部数据 (2048 字节)
	buf := make([]byte, EnvelopeHeaderSize_v3)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, fmt.Errorf("failed to read header bytes: %w", err)
	}

	// 2. 提取存储的 CRC32 (位于最后 4 字节)
	storedCRC := binary.LittleEndian.Uint32(buf[EnvelopeHeaderSize_v3-4 : EnvelopeHeaderSize_v3])

	// 3. 计算前 2044 字节的 CRC32 并对比
	calculatedCRC := crc32.ChecksumIEEE(buf[:EnvelopeHeaderSize_v3-4])
	if storedCRC != calculatedCRC {
		return nil, fmt.Errorf("header CRC32 mismatch: stored=%08x, calculated=%08x", storedCRC, calculatedCRC)
	}

	// 4. 反序列化到结构体
	// 使用 binary.Read 将字节流直接映射到结构体
	header := &EnvelopeHeaderV3{}
	if err := binary.Read(bytes.NewReader(buf), binary.LittleEndian, header); err != nil {
		return nil, fmt.Errorf("failed to unmarshal header: %w", err)
	}

	// 5. 额外校验 Magic Number (虽然 CRC 校验过了，但双重保险有助于调试)
	if header.Magic != MagicHeader_v2 {
		return nil, ErrInvalidMagic_v2
	}

	return header, nil
}

// ParseSpecialID 从头部解析 SpecialID
func (h *EnvelopeHeaderV3) ParseSpecialID() (map[string]interface{}, error) {
	payload := h.SpecialID[:h.IDLength]

	switch IDType(h.IDType) {
	case IDType_CBOR:
		// 使用标准 CBOR 库（如 fxamacker/cbor）反序列化
		var meta map[string]interface{}
		if err := cbor.Unmarshal(payload, &meta); err != nil {
			return nil, err
		}
		return meta, nil

	case IDType_Raw:
		// 占位符或不可逆 ID，尝试计算 Hash 用于路由
		hash := sha256.Sum256(payload)
		return map[string]interface{}{"_hash_route": hash[:]}, nil

	default:
		return nil, fmt.Errorf("unknown ID type: %d", h.IDType)
	}
}

// GeneratePlaceholderIDV3 生成一个 2000 字节的随机占位 ID
func GeneratePlaceholderIDV3() ([]byte, error) {
	buf := make([]byte, SpecialIDMaxLen)
	if _, err := rand.Read(buf); err != nil {
		return nil, err
	}
	return buf, nil
}

// BuildHeaderV3 构建 v3 头部
func BuildHeaderV3(flags uint16, idType IDType, idData []byte) (*EnvelopeHeaderV3, error) {
	if len(idData) > SpecialIDMaxLen {
		return nil, fmt.Errorf("special ID data exceeds max length %d", SpecialIDMaxLen)
	}

	header := &EnvelopeHeaderV3{
		Magic:   MagicHeader_v2,
		Version: 0x03,
		Flags:   flags,
		IDType:  uint32(idType),
	}
	copy(header.SpecialID[:], idData)
	header.IDLength = uint32(len(idData))
	return header, nil
}

// DetectHeaderVersion 从文件头部探测版本
func DetectHeaderVersion(data []byte) int {
	if len(data) < 4 {
		return 0
	}
	magic := string(data[:4])
	if magic == string(MagicHeader_v2[:]) {
		if len(data) >= 6 {
			version := binary.LittleEndian.Uint16(data[4:6])
			switch version {
			case 0x04:
				return 4
			case 0x03:
				return 3
			}
		}
		return 2
	}
	return 0
}
