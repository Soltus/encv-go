package encv

import (
	"github.com/Soltus/encv-go/internal/v2/container/manifest"
)

// ExtractKVI 从容器文件中直接扫描并提取 KVI 块的数据，不依赖清单。
func ExtractKVI(containerPath string) ([]byte, error) {
	return manifest.ExtractKVI(containerPath)
}

// ExtractKVI_v2 是 ExtractKVI 的兼容别名（过渡期）
func ExtractKVI_v2(containerPath string) ([]byte, error) {
	return manifest.ExtractKVI(containerPath)
}

// ExtractManifest 从容器文件中直接扫描并提取 Manifest 块的数据。
func ExtractManifest(containerPath string) ([]byte, error) {
	return manifest.ExtractManifest(containerPath)
}

// ExtractManifest_v2 是 ExtractManifest 的兼容别名（过渡期）
func ExtractManifest_v2(containerPath string) ([]byte, error) {
	return manifest.ExtractManifest(containerPath)
}
