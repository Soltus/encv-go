// internal/v2/provider/provider.go
package provider

import (
	"io"
)

// SeekerTo 接口，用于远程视频的快速定位
type SeekerTo interface {
	SeekTo(offset int64) error
}

// FileContentProvider 定义了文件内容提供者的通用接口
// 它屏蔽了文件来源（本地或远程）的复杂性
type FileContentProvider interface {
	// GetReader 返回一个用于读取文件内容的 io.ReadCloser
	// 调用者负责关闭它
	GetReader() io.ReadCloser

	// GetSeeker 返回一个用于随机访问的 io.Seeker 和一个布尔值表示是否支持
	GetSeeker() (io.Seeker, bool)

	// GetSeekerTo 返回一个用于快速定位的 SeekerTo 和一个布尔值表示是否支持
	GetSeekerTo() (SeekerTo, bool)

	// GetSize 返回文件的原始总大小（字节）
	GetSize() int64

	// GetName 返回文件的原始名称（包含扩展名）
	GetName() string

	// Close 用于关闭提供者及其占用的所有资源（如网络连接、文件句柄）
	Close() error
}
