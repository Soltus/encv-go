// internal/v2/writer/container_writer_v2.go
package writer

import (
	"github.com/Soltus/encv-go/internal/v2/types"
)

// ContainerWriter_v2 定义了写入容器组件的通用接口
type ContainerWriter_v2 interface {
	// WriteKVI 写入 KVI (Key and IV) 元数据块
	WriteKVI(kviData []byte) error
	// WriteFragment 写入一个数据分片
	WriteFragment(frag *types.Fragment, data []byte) error
	// WriteManifest 写入清单块，并记录其位置信息
	WriteManifest(manifest *types.Manifest) error
	// Close 写入 Footer 并关闭文件
	Close() error
}
