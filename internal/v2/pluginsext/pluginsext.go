// Package pluginsext 集中导出各 plugin 真实容器扩展名常量。
//
// 权威来源：internal/v2/plugins/<plugin>/plugin.go 的 GetContainerExtension() 默认值
// （这些常量是该默认值的镜像，必须随 plugin 源码同步）。
//
// 为什么需要这个包：
//   - 避免在测试代码（internal/v2/container/detector、internal/v2/writer 等）中
//     硬编码 alist 历史后缀或 sccg 系列等具体扩展名
//   - 避免测试代码直接 import internal/v2/plugins（plugins 包导入 detector 等
//     子包，会形成 import cycle）
//   - 本包是 leaf package（无任何 import），所有测试包都能安全 import
//
// 同步校验：internal/v2/plugins/pluginsext_sync_test.go 在测试时
// 读取 plugin.GetContainerExtension() 并断言本包常量与之一致，
// 任何不一致都会让 CI 红灯。
//
// 不要在本包中放置任何非容器扩展名的常量或逻辑——保持极简。
package pluginsext

// 各 plugin 默认容器扩展名（镜像 plugin.GetContainerExtension() 默认值）。
// 历史注：alist_encrypt plugin 默认 .bin，但兼容 alist v2 历史扩展名
// （plugin.go L100 注释）；本常量反映 plugin 默认 .bin。
const (
	VideoExt = ".sccgv" // video plugin 默认
	AudioExt = ".sccga" // audio plugin 默认
	ImageExt = ".sccgi" // image plugin 默认
	TextExt  = ".sccgt" // text plugin 默认
	PdfExt   = ".sccgpdf"
	WpsExt   = ".sccgwps"
	AlistExt = ".bin" // alist_encrypt plugin 默认（兼容 alist v2 历史）
)
