package chunker

import (
	"github.com/Soltus/encv-go/internal/v2/types"
)

// isSingleFileContainer 判断是否为单文件容器
// 核心思想：如果任何 fragment 引用了外部文件，则不是单文件容器。
func IsSingleFileContainer(manifest *types.Manifest_v2) bool {
	for _, frag := range manifest.Fragments {
		// 检查所有可能指向外部文件的字段
		// PhysicalPath 是物理分片时使用的字段
		// Filename 是另一种可能的分片策略使用的字段
		if frag.PhysicalPath != "" || frag.Filename != "" {
			return false // 发现了外部文件引用，这是物理分片容器
		}
	}
	return true // 所有 fragment 都没有外部文件引用，这是单文件容器
}
