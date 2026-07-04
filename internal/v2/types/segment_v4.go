package types

import (
	"bytes"
	"encoding"
	"encoding/json"
	"fmt"
	"sync"
)

// SegmentHeaderSize 是 v4 容器每个 Segment 头的固定字节大小（34 字节）。
//
// 历史沿革：
//   - v4 legacy 布局：18 字节（SegmentID + DataLength + NonceSize + DataCRC32）
//   - v4 升级布局（v4-container-capability-upgrade spec）：34 字节
//   - 本常量已从 18 升级为 34，是破坏性磁盘格式变更，但 spec 接受这个 trade-off
//
// 字段布局（所有偏移相对 Segment 起始，Little-Endian）：
//
//	Offset  Size  Field               Description
//	0       4     SegmentID           Segment 唯一标识
//	4       8     DataLength          密文长度（不含 MAC）
//	12      2     NonceSize           16
//	14      2     ModeFlags           位字段（bit0=Encrypted, bit1=Compression）
//	16      2     MACSize             10（HMAC-SHA1-80 截断长度）
//	18      4     DataCRC32           密文 CRC32（可选）
//	22      2     CompressedBlockSize seekable zstd 块大小（0=无压缩）
//	24      2     Reserved
//	26      4     SeekTableOffset     seek table 相对 Segment 起始的偏移（zstd 时有效）
//	30      4     SeekTableLength     seek table 长度
//	Total: 34 bytes
//
// 实现说明：
//   - 使用手动 binary.LittleEndian.PutXxx 序列化（不走 binary.Read/Write）
//   - 不受 Go 结构体字段 padding 影响（unsafe.Sizeof ≠ 34，但序列化产物是精确 34 字节）
//   - 默认 ModeFlags = 0（明文 + 不压缩），加密 Segment 必须显式设 ModeFlagEncrypted
const SegmentHeaderSize = 34

// ModeFlags 位字段定义（仿 WinZip "mixed" 模式）。
//
// 16 位宽度布局：
//
//	bit 0      : Encrypted（1=加密 Segment，0=明文 Segment）
//	bit 1      : CompressionZstd（1=使用 seekable zstd 压缩，0=不压缩）
//	bit 2..15  : 保留（必须为 0）
//
// 默认值 0x0000 表示「明文 + 不压缩」。任何加密 Segment 必须显式设置 ModeFlagEncrypted。
const (
	// ModeFlagEncrypted bit0：标识该 Segment 是否加密。
	// 1=加密（需要 MAC 校验、AES-CTR 解密）；0=明文（跳过 MAC/解密）。
	ModeFlagEncrypted uint16 = 1 << 0
	// ModeFlagCompressionZstd bit1：标识该 Segment 数据是否经过 seekable zstd 压缩。
	// 1=已压缩（读取时需解压）；0=未压缩。
	ModeFlagCompressionZstd uint16 = 1 << 1
)

// 常用 ModeFlags 组合（便利常量）。
const (
	// ModeFlagsPlaintext = 0x0000：明文 + 不压缩
	ModeFlagsPlaintext uint16 = 0
	// ModeFlagsEncryptedNoCompression = ModeFlagEncrypted：加密 + 不压缩
	ModeFlagsEncryptedNoCompression uint16 = ModeFlagEncrypted
	// ModeFlagsEncryptedZstd = ModeFlagEncrypted | ModeFlagCompressionZstd：加密 + zstd 压缩
	ModeFlagsEncryptedZstd uint16 = ModeFlagEncrypted | ModeFlagCompressionZstd
)

type SegmentHeader struct {
	SegmentID           uint32
	DataLength          uint64
	NonceSize           uint16
	ModeFlags           uint16
	MACSize             uint16
	DataCRC32           uint32
	CompressedBlockSize uint16
	Reserved            uint16
	SeekTableOffset     uint32
	SeekTableLength     uint32
}

type KeyframeEntry struct {
	Offset uint64  `json:"offset"`
	Time   float64 `json:"time"`
}

type Segment_v4 struct {
	ID           string          `json:"id"`
	Offset       uint64          `json:"offset"`
	Size         uint64          `json:"size"`
	StartTime    float64         `json:"start_time"`
	Duration     float64         `json:"duration"`
	Nonce        string          `json:"nonce"`
	KeyframeInfo []KeyframeEntry `json:"keyframe_info,omitempty"`
}

type DisasterZone struct {
	Name   string `json:"name"`
	Offset int64  `json:"offset"`
	Size   int64  `json:"size"`
}

type EDLEntry struct {
	Time    float64 `json:"time"`
	Action  string  `json:"action"`
	Segment string  `json:"segment"`
}

type ChapterInfo_v4 struct {
	Time  float64 `json:"time"`
	Title string  `json:"title"`
}

type Manifest_v4 struct {
	Version           uint16              `json:"version"`
	ContainerID       string              `json:"container_id"`
	ContainerType     string              `json:"container_type"`
	IsSeekable        bool                `json:"is_seekable"`
	OriginalDuration  float64             `json:"original_duration,omitempty"`
	Segments          []Segment_v4        `json:"segments"`
	Playlists         map[string][]string `json:"playlists"`
	Chapters          []ChapterInfo_v4    `json:"chapters,omitempty"`
	DisasterZones     []DisasterZone      `json:"disaster_zones,omitempty"`
	KVI               json.RawMessage     `json:"kvi"`
	EDLHistory        []EDLEntry          `json:"edl_history,omitempty"`
	OriginalName      string              `json:"original_name,omitempty"`
	FilenameAlgorithm string              `json:"filename_alg,omitempty"`

	// MACSaltBase64 是 v4 容器用于 HMAC-SHA1-80 校验的独立 salt（base64 编码）。
	//
	// 字段用途：
	//   - 写入器：container_writer_v4 在 WriteV4Container 时调用 crypto.GenerateMACSalt()
	//     生成 16 字节随机 mac_salt，base64 编码后写入此字段。
	//   - 读取器：OpenV4Container 反序列化 Manifest 后从此字段取出 mac_salt，
	//     调用 crypto.DeriveMACKey 派生 mac_key，用于 Segment 的 HMAC 校验。
	//
	// 为什么不放 Header？
	//   v4 Header 偏移 36-2028 被 SpecialID 完全占据，物理上无法再插入 16 字节。
	//   即使放到 Reserved 区域（[0,36) 或 [2028,2048)），也必须修改 CRC 计算范围，
	//   不向后兼容。因此 mac_salt 改存于 Manifest（v4 容器的可变长元数据区域），
	//   并随 Manifest 一同被 XOR 混淆，提供额外的机密性保护。
	//
	// 向后兼容：
	//   - 旧 v4 容器（升级前创建）没有 mac_salt 字段 → JSON 反序列化后此字段为空字符串
	//   - 调用方应使用 types.HasMACSalt() 判断，或在 OpenV4Container 内 fallback
	//     到 encrypt salt（KVI.salt_base64）
	//
	// JSON tag 使用 omitempty：旧 v4 容器的 Manifest 序列化结果不会包含此字段，
	// 保持向前兼容的 JSON 形态（diff 友好）。
	MACSaltBase64 string `json:"mac_salt_base64,omitempty"`
}

var manifestV4BufferPool = sync.Pool{
	New: func() interface{} {
		return bytes.NewBuffer(make([]byte, 0, 4096))
	},
}

func (m *Manifest_v4) SerializeToJSON_v4() ([]byte, error) {
	buf := manifestV4BufferPool.Get().(*bytes.Buffer)
	buf.Reset()
	defer manifestV4BufferPool.Put(buf)

	enc := json.NewEncoder(buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(m); err != nil {
		return nil, err
	}

	size := buf.Len()
	if size == 0 {
		return nil, nil
	}
	out := make([]byte, size-1)
	copy(out, buf.Bytes()[:size-1])
	return out, nil
}

func DeserializeManifest_v4(data []byte) (*Manifest_v4, error) {
	var m Manifest_v4
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

func (m *Manifest_v4) GetSegmentByID(id string) (*Segment_v4, error) {
	for _, seg := range m.Segments {
		if seg.ID == id {
			return &seg, nil
		}
	}
	return nil, fmt.Errorf("segment with ID '%s' not found in manifest", id)
}

func (m *Manifest_v4) GetPlaylist(name string) ([]string, error) {
	if name == "" {
		name = "default"
	}
	ids, ok := m.Playlists[name]
	if !ok {
		return nil, fmt.Errorf("playlist '%s' not found in manifest", name)
	}
	return ids, nil
}

func (m *Manifest_v4) ResolvePlaylist(name string) ([]Segment_v4, error) {
	ids, err := m.GetPlaylist(name)
	if err != nil {
		return nil, err
	}

	segments := make([]Segment_v4, 0, len(ids))
	for _, id := range ids {
		seg, err := m.GetSegmentByID(id)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve segment '%s' in playlist: %w", id, err)
		}
		segments = append(segments, *seg)
	}
	return segments, nil
}

func (m *Manifest_v4) GetOriginalName() string      { return m.OriginalName }
func (m *Manifest_v4) GetFilenameAlgorithm() string { return m.FilenameAlgorithm }

// MarshalBinary 将 SegmentHeader 序列化为精确 SegmentHeaderSize 字节的二进制。
//
// 字节布局（与 SegmentHeader 字段布局一一对应，Little-Endian）：
//
//	[0, 4)        SegmentID
//	[4, 12)       DataLength
//	[12, 14)      NonceSize
//	[14, 16)      ModeFlags
//	[16, 18)      MACSize
//	[18, 22)      DataCRC32
//	[22, 24)      CompressedBlockSize
//	[24, 26)      Reserved
//	[26, 30)      SeekTableOffset
//	[30, 34)      SeekTableLength
//
// 使用手动 PutXxx 而非 binary.Write，避免 Go 结构体字段 padding 影响序列化大小。
// 序列化产物长度严格等于 SegmentHeaderSize（34 字节），与 unsafe.Sizeof(SegmentHeader{}) 不同。
func (h *SegmentHeader) MarshalBinary() ([]byte, error) {
	buf := make([]byte, SegmentHeaderSize)
	ByteOrder_v2.PutUint32(buf[0:4], h.SegmentID)
	ByteOrder_v2.PutUint64(buf[4:12], h.DataLength)
	ByteOrder_v2.PutUint16(buf[12:14], h.NonceSize)
	ByteOrder_v2.PutUint16(buf[14:16], h.ModeFlags)
	ByteOrder_v2.PutUint16(buf[16:18], h.MACSize)
	ByteOrder_v2.PutUint32(buf[18:22], h.DataCRC32)
	ByteOrder_v2.PutUint16(buf[22:24], h.CompressedBlockSize)
	ByteOrder_v2.PutUint16(buf[24:26], h.Reserved)
	ByteOrder_v2.PutUint32(buf[26:30], h.SeekTableOffset)
	ByteOrder_v2.PutUint32(buf[30:34], h.SeekTableLength)
	return buf, nil
}

// UnmarshalBinary 从 data 反序列化为 SegmentHeader。
//
// 要求 len(data) >= SegmentHeaderSize（34 字节），否则返回明确错误。
// 不会就地修改 data 之外的任何状态。
func (h *SegmentHeader) UnmarshalBinary(data []byte) error {
	if len(data) < SegmentHeaderSize {
		return fmt.Errorf("segment header requires at least %d bytes, got %d", SegmentHeaderSize, len(data))
	}
	h.SegmentID = ByteOrder_v2.Uint32(data[0:4])
	h.DataLength = ByteOrder_v2.Uint64(data[4:12])
	h.NonceSize = ByteOrder_v2.Uint16(data[12:14])
	h.ModeFlags = ByteOrder_v2.Uint16(data[14:16])
	h.MACSize = ByteOrder_v2.Uint16(data[16:18])
	h.DataCRC32 = ByteOrder_v2.Uint32(data[18:22])
	h.CompressedBlockSize = ByteOrder_v2.Uint16(data[22:24])
	h.Reserved = ByteOrder_v2.Uint16(data[24:26])
	h.SeekTableOffset = ByteOrder_v2.Uint32(data[26:30])
	h.SeekTableLength = ByteOrder_v2.Uint32(data[30:34])
	return nil
}

var _ encoding.BinaryMarshaler = (*SegmentHeader)(nil)
var _ encoding.BinaryUnmarshaler = (*SegmentHeader)(nil)
