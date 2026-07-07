package physical

import (
	"context"
	"io"

	"github.com/Soltus/encv-go/internal/v2/namer"
	"github.com/Soltus/encv-go/internal/v2/reader"
	"github.com/Soltus/encv-go/internal/v2/types"
)

// PhysicalPacker 定义了物理分片的打包接口
type PhysicalPacker interface {
	// Pack 执行完整的物理打包，包括数据分片和 Manifest 写入
	// 【关键修改】接收 manifest 作为参数，并负责完成所有写入
	Pack(manifest *types.Manifest, req *PackRequest) (mainChunkPath string, err error)
}

// PhysicalUnpacker 定义了物理分片的解包接口
// 它负责处理物理分片，并返回一个可以像单一文件一样读取的容器路径
type PhysicalUnpacker interface {
	// Unpack 接收主容器文件路径（可能是第一个分片），并返回一个统一的容器文件路径
	// 返回的路径可以被 NewFileContainerReader_v2 直接使用
	// 同时返回一个 cleanup 函数，用于在操作完成后清理临时资源
	Unpack(ctx context.Context, mainContainerPath string) (unifiedContainerPath string, cleanup func(), err error)
}

// Unpacker 定义了从容器文件中解包数据的接口
type Unpacker interface {
	// Unpack 打开容器，并返回一个用于创建解密流的工厂、文件索引和原始大小
	// 【关键修改】直接返回 reader 包的工厂接口
	Unpack(ctx context.Context, containerPath string) (reader.DecryptReaderFactory, types.Index, int64, error)
}

// Packer 定义了将加密数据打包到容器的接口
type Packer interface {
	// Pack 将加密数据和元数据打包到容器文件
	Pack(ctx context.Context, req *PackRequest) error
}

// PackRequest 是打包请求的参数集合
type PackRequest struct {
	// 数据源
	EncryptedDataReader io.Reader

	// 索引与加密参数
	Index types.Index
	Salt  []byte
	IV    []byte

	// 命名与输出配置
	BaseName      string // 不带容器扩展名的基础文件名，例如 "321.4pm"
	OutputDir     string
	Namer         namer.ChunkNamer
	FinalFileName string // 可选，用于单文件模式。如果设置了此字段，Packer 将直接使用它，忽略 Namer。

	// 物理分片配置
	StartIdx              int
	LightMainChunkEnabled bool // 是否启用轻量级主分片，启用后主分片只包含清单，不包含源数据

	// V3/V4 头部配置
	HeaderVersion int
	ContainerType uint16
	IsSeekable    bool
	SpecialID     []byte // 可选，如果不提供且 HeaderVersion=3，将自动生成占位符
	SpecialIDType types.IDType

	PasswordHint [16]byte

	WrappedDEK *types.WrappedDEK
}
