package types

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
)

const (
	// L1: Envelope

	EnvelopeHeaderSize_v2 = 16
	EnvelopeFooterSize_v2 = 32

	// L2/L3/L4: Block
	BlockTypeManifest_v2 uint16 = 0x0001
	BlockTypeKVI_v2      uint16 = 0x0002
	BlockTypeData_v2     uint16 = 0x0003
	BlockTypeRecovery_v2 uint16 = 0x0004

	// Crypto
	SaltSize_v2 = 16
	KeySize_v2  = 32
	IVSize_v2   = 16
)

const (
	// ContainerECv2 / ContainerECv3 / ContainerECv4 是 ENCV 容器格式的版本号。
	// 命名规则：ECv = ENCV Container，大写 EC，小写 v，避免与项目内 v2 架构命名混淆。
	ContainerECv2            = 2
	ContainerECv3            = 3
	ContainerECv4            = 4
	DefaultContainerVersion  = ContainerECv4
)

type VersionStatus string

const (
	VersionStatusDeprecated  VersionStatus = "deprecated"
	VersionStatusStable      VersionStatus = "stable"
	VersionStatusRecommended VersionStatus = "recommended"
)

func GetBlockTypeName(blockType uint32) string {
	switch blockType {
	case uint32(BlockTypeData_v2):
		return "Data"
	case uint32(BlockTypeManifest_v2):
		return "Manifest"
	case uint32(BlockTypeKVI_v2):
		return "KVI"
	case uint32(BlockTypeRecovery_v2):
		return "Recovery"
	default:
		return fmt.Sprintf("Unknown(%d)", blockType)
	}
}

var (
	ByteOrder_v2         = binary.LittleEndian
	ErrInvalidMagic_v2   = errors.New("invalid magic number")
	ErrWrongPassword     = errors.New("wrong password: password hint mismatch")
	ErrDataCorrupted     = errors.New("data corrupted: integrity check failed")
	ErrDeprecatedVersion = errors.New("container version is deprecated")
	MagicHeader_v2       = [4]byte{'E', 'N', 'C', 'V'}
	MagicFooter_v2       = [4]byte{'E', 'N', 'C', 'V'}

	manifestJSONBufferPool_v2 = sync.Pool{
		New: func() interface{} {
			return bytes.NewBuffer(make([]byte, 0, 4096))
		},
	}
)

// SupportedVersions 是当前**支持创建新容器**的版本列表。
// ECv2 (2024 旧版) 已从项目中移除（仅保留底层读路径供存量 ECv2 容器在野外被读取）。
// ECv3 标记为 deprecated（仍可创建/读取，但强烈建议迁 ECv4），ECv4 是当前推荐版本。
var SupportedVersions = []int{ContainerECv3, ContainerECv4}

// GetVersionStatus 返回版本在「支持列表」中的状态：
//   - supported version（ECv3/ECv4）：返回对应状态（deprecated/recommended）
//   - unsupported version（含 ECv2、其他未知）：返回空字符串
//
// 注意：ECv2 已从 SupportedVersions 移除，但保留 ContainerECv2 常量供 detector 识别存量文件。
func GetVersionStatus(version int) VersionStatus {
	switch version {
	case ContainerECv3:
		return VersionStatusDeprecated
	case ContainerECv4:
		return VersionStatusRecommended
	default:
		return ""
	}
}

func IsValidVersion(version int) bool {
	for _, v := range SupportedVersions {
		if v == version {
			return true
		}
	}
	return false
}

func IsDeprecatedVersion(version int) bool {
	return GetVersionStatus(version) == VersionStatusDeprecated
}

// IsRecommendedVersion 返回容器版本是否是当前推荐版本（仅 ECv4）
// 与 IsDeprecatedVersion 互斥：
//   - IsRecommendedVersion(v) = true  ⇔ v == ContainerECv4
//   - IsDeprecatedVersion(v)  = true  ⇔ v ∈ {ContainerECv2, ContainerECv3}
//   - 二者都 = false                  ⇔ v 是 SupportedVersions 之外的未知版本
func IsRecommendedVersion(version int) bool {
	return GetVersionStatus(version) == VersionStatusRecommended
}

// FragmentType 定义分片的用途类型，使其意图更加明确
type FragmentType string

// FragmentType_v2 是 FragmentType 的兼容别名（过渡期）
type FragmentType_v2 = FragmentType

const (
	// FragmentType_Metadata 用于存储 KVI 等元数据，通常只有一个
	FragmentType_Metadata FragmentType = "metadata"
	// FragmentType_SeekableStream 用于存储可寻址的数据流，如视频、大型日志文件
	// 支持随机访问，其 GlobalStartOffset 字段有效
	FragmentType_SeekableStream FragmentType = "seekable_stream"
	// FragmentType_AtomicFile 用于存储不可分割的原子文件，如文档、图片、数据库
	// 必须完整读取，其 GlobalStartOffset 字段无效
	FragmentType_AtomicFile FragmentType = "atomic_file"
)

// Fragment 定义了清单中的一个分片项
type Fragment struct {
	ID       string       `json:"id"`                 // 唯一标识符
	Type     FragmentType `json:"type"`               // 【更新】分片类型，使用枚举
	Filename string       `json:"filename,omitempty"` // 可选，用于外部文件
	Length   uint64       `json:"length"`             // 数据长度（字节）

	// GlobalStartOffset 仅在 Type 为 FragmentType_SeekableStream 时有效。
	// 它表示该分片在整个虚拟数据流中的起始字节位置，用于 O(1) 寻址。
	GlobalStartOffset uint64 `json:"global_start_offset,omitempty"`
	PhysicalPath      string `json:"physical_path,omitempty"` // 作为提示，TODO: 后续改为使用 header 关联物理分片
	// 【新增】PhysicalOffset 记录该 Fragment 对应的 Block Header 在物理文件中的绝对位置。
	// 注意：这是 Block Header 的起始位置，而非 Payload 的位置。
	// 这样设计是为了让解密器能够直接定位到数据，无需复杂的扫描逻辑。
	// 如果 Fragment 在外部文件中，此字段记录的是在该外部文件中的偏移。
	PhysicalOffset uint64 `json:"physical_offset,omitempty"`

	// 【关键新增】该片段对应加密数据块的 CRC32 校验和
	// 这是验证物理文件是否正确的“指纹”，与文件名无关
	DataCRC32 uint32 `json:"data_crc32"`
}

// Fragment_v2 是 Fragment 的兼容别名（过渡期）
type Fragment_v2 = Fragment

type EnvelopeHeader_v2 struct {
	Magic    [4]byte
	Version  uint16
	Flags    uint16
	Reserved [8]byte
}

type EnvelopeFooter_v2 struct {
	Magic          [4]byte
	ManifestOffset uint64
	ManifestLength uint64
	ManifestCRC32  uint32
	GlobalCRC32    uint32
	Reserved       [4]byte
}

// ContainerDescriptor 描述了一个容器的元信息
type ContainerDescriptor struct {
	FilePath   string
	IsSeekable bool
}

// KVIProvider 定义了从 KVI 数据中获取所需信息的通用接口
// Manifest 将依赖此接口，而不是具体的结构体，从而实现解耦
type KVIProvider interface {
	// 获取索引类型，用于快速路由和判断
	GetKind() IndexKind
	// GetEncryptionInfo 返回用于序列化到 Manifest 的 KVI 数据
	GetEncryptionInfo() KVI

	// GetIndex 返回实现了 types.Index 接口的文件索引，供上层服务使用
	GetIndex() Index
}

// KVI 加密信息
type KVI struct {
	SaltBase64 string `json:"salt_base64"`
	IVBase64   string `json:"iv_base64"`
}

// KVI_v2 是 KVI 的兼容别名（过渡期）
type KVI_v2 = KVI

// Manifest 容器清单，这是json的最外层
type Manifest struct {
	Version int64 `json:"version"`
	// Kind 现在是 Manifest 的顶级字段，用于标识 KVI 类型
	Kind IndexKind `json:"kind"`
	// KVI 字段持有原始的 JSON 数据，其具体类型由上层处理
	KVI        json.RawMessage `json:"kvi"` // index在里面
	Fragments  []Fragment      `json:"fragments"`
	Redundancy struct {
		KVIBackupCRC string `json:"kvi_backup_crc,omitempty"`
	} `json:"redundancy,omitempty"`
}

// Manifest_v2 是 Manifest 的兼容别名（过渡期）
type Manifest_v2 = Manifest

// SerializeToJSON 将清单序列化为 JSON 字节
func (m *Manifest) SerializeToJSON() ([]byte, error) {
	buf := manifestJSONBufferPool_v2.Get().(*bytes.Buffer)
	buf.Reset()
	defer manifestJSONBufferPool_v2.Put(buf)

	enc := json.NewEncoder(buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(m); err != nil {
		return nil, err
	}

	// json.Encoder 会追加 '\n'
	size := buf.Len()
	if size == 0 {
		return nil, nil
	}
	out := make([]byte, size-1)
	copy(out, buf.Bytes()[:size-1])
	return out, nil
}

// GetFragmentByID 根据 ID 查找并返回一个 Fragment 的副本
func (m *Manifest) GetFragmentByID(id string) (*Fragment, error) {
	for _, frag := range m.Fragments {
		if frag.ID == id {
			// 返回一个副本以防止外部修改
			return &frag, nil
		}
	}
	return nil, fmt.Errorf("fragment with ID '%s' not found in manifest", id)
}

// KVIProviderFactory 是一个工厂函数，它知道如何从原始 JSON 数据创建一个特定的 KVIProvider
type KVIProviderFactory func(rawKVI json.RawMessage) (KVIProvider, error)

// kviProviderRegistry 是一个私有的中央注册表，用于存储所有已注册的 KVI 提供者工厂
var kviProviderRegistry = make(map[IndexKind]KVIProviderFactory)

// RegisterKVIProvider 允许外部插件注册自己。
// 通常在插件的 init() 函数中调用。
// 如果重复注册同一个 Kind，将会 panic，以帮助开发者及早发现配置错误。
func RegisterKVIProvider(kind IndexKind, factory KVIProviderFactory) {
	if _, exists := kviProviderRegistry[kind]; exists {
		panic(fmt.Sprintf("attempted to register duplicate KVI provider for kind: %s", kind))
	}
	kviProviderRegistry[kind] = factory
}

// NewKVIProviderFromManifest 【重构】使用注册表动态创建 KVIProvider
// 现在这个函数是通用的，不再需要为每个新插件修改它。
func NewKVIProviderFromManifest(manifest *Manifest) (KVIProvider, error) {
	factory, exists := kviProviderRegistry[manifest.Kind]
	if !exists {
		// 提供一个友好的错误信息，列出所有已注册的 Kind
		registeredKinds := make([]string, 0, len(kviProviderRegistry))
		for k := range kviProviderRegistry {
			registeredKinds = append(registeredKinds, string(k))
		}
		return nil, fmt.Errorf("unsupported or unknown index kind: '%s'. Registered kinds are: %v", manifest.Kind, registeredKinds)
	}

	// 调用对应插件注册的工厂函数来创建实例
	return factory(manifest.KVI)
}

// NewManifest 是一个工厂函数，用于创建 Manifest 实例
// 它接收一个 KVIProvider 接口，并将其序列化为 json.RawMessage
func NewManifest(kviProvider KVIProvider, fragments []Fragment) (*Manifest, error) {
	// 1. 将 KVIProvider 接口实例序列化为 JSON 字节切片
	// json.Marshal 可以处理任何具有可导出字段的结构体，或者实现了 json.Marshaler 接口的类型
	// 我们的 VideoKVI_v2 完全符合这个条件
	kviBytes, err := json.Marshal(kviProvider)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal KVI provider: %w", err)
	}

	// 2. 将字节切片转换为 json.RawMessage
	// json.RawMessage 本质上就是 []byte，这个转换是类型安全的
	rawKVI := json.RawMessage(kviBytes)

	// 3. 创建并返回 Manifest 实例
	manifest := &Manifest{
		Version:   ContainerVersion,
		Kind:      kviProvider.GetKind(),
		KVI:       rawKVI,
		Fragments: fragments,
	}

	return manifest, nil
}

// IndexKind 定义 KVI 的类型
type IndexKind string

const (
	// ManifestSchemaVersion 是 Manifest JSON schema 的版本号（注意：不是容器格式版本！）
	ManifestSchemaVersion int64 = 2
)

// 兼容别名：旧名 ContainerVersion 仍有误导性，保留过渡
const ContainerVersion = ManifestSchemaVersion // deprecated: use ManifestSchemaVersion

// Index 是所有 KVI 结构体的通用接口
type Index interface {
	GetOriginalFilename() string
	GetOriginalFileSize() int64
	GetOriginalFileMD5() string
	GetEncryptedFileMD5() string
	GetMimeType() string // 重要方法，实现错误会影响前端预览
}

// NoOpIndex 是一个安全的、无操作的 Index 实现，用于在发生严重内部错误时防止 panic。
type NoOpIndex struct{}

func (i *NoOpIndex) GetMimeType() string         { return "application/octet-stream" }
func (i *NoOpIndex) GetOriginalFilename() string { return "corrupted" }
func (i *NoOpIndex) GetOriginalFileSize() int64  { return 0 }
func (i *NoOpIndex) GetOriginalFileMD5() string  { return "" }
func (i *NoOpIndex) GetEncryptedFileMD5() string { return "" }
