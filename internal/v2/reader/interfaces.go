package reader

import (
	"errors"
	"io"

	"github.com/Soltus/encv-go/internal/v2/types"
)

// DecryptReader 是所有解密器的基础接口
// 它只包含所有解密器都必须具备的能力：读取和关闭
type DecryptReader interface {
	io.Reader
	io.Closer
}

// SeekableDecryptReader 是一个扩展接口，用于可寻址的解密器
// 它在基础接口之上增加了寻址能力
type SeekableDecryptReader interface {
	DecryptReader
	io.Seeker
}

// ErrOperationNotSupported 表示当前操作（如 Seek）不被该类型的读取器支持。
var ErrOperationNotSupported = errors.New("operation not supported by this reader type")

// EncryptedContainerReader 定义了从加密容器中读取原始数据块的最低级接口。
// 它的职责是：根据 Fragment ID，提供一个校验过的原始数据流。
// 它应该是无状态的，或者其状态由调用者管理。
type EncryptedContainerReader interface {
	// GetFragmentReader 根据 Fragment ID，返回一个读取该 Fragment 原始加密数据的 io.ReadCloser。
	// 调用者负责关闭返回的 ReadCloser。
	// 此方法应该是线程安全的。
	GetFragmentReader(fragID string) (io.ReadCloser, error)

	// GetManifest 返回已解析的容器清单。
	// 这是获取容器元数据（如 Fragment 列表、KVI）的唯一入口。
	GetManifest() *types.Manifest

	// GetKVIProvider 返回解析后的 KVI 提供者接口
	GetKVIProvider() (types.KVIProvider, error)

	// GetFragments 返回 Manifest 中的所有片段定义
	GetFragments() []types.Fragment

	// Close 关闭容器及其打开的所有底层资源（如文件句柄）。
	Close() error
}
