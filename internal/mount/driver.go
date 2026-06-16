package mount

import (
	"context"
	"os"
)

// Driver 是挂载点的存储后端实现。
//
// 约束：
//   - 所有 FS 操作接收的 path 参数是 **相对挂载点根** 的相对路径（不是绝对路径）
//   - Driver 内部完成相对路径 → 物理绝对路径的拼装
//   - 这样上层不需要知道 RootPath 是什么（解耦）
//
// 线程安全：Driver 实例在 Registry 中是只读的（启动时 Init 一次），
// 所有 FS 操作应当线程安全（os.File* 本身就是）。
type Driver interface {
	// Name 返回 driver 类型名（与 Mount.Driver 一致）。
	Name() string

	// Init 在 Registry.Bootstrap 时调用一次，传入 Mount 和全局 Config。
	// Driver 应当解析 Mount.RootPath（如果为空，根据 DriverConfig 计算后回填）。
	Init(ctx context.Context, m *Mount, cfg ConfigProvider) error

	// ResolveRoot 返回挂载点的物理绝对根路径。
	ResolveRoot() string

	// CheckPermission 在 Create/Update 后以及每次 Resolve 时调用。
	// 用途：appdata driver 检查 app 是否有权限；local driver 检查目录存在。
	CheckPermission() error

	// FS operations
	Stat(relPath string) (os.FileInfo, error)
	ReadDir(relPath string) ([]os.DirEntry, error)
	ReadFile(relPath string) ([]byte, error)
	WriteFile(relPath string, data []byte, perm os.FileMode) error
	MkdirAll(relPath string, perm os.FileMode) error
	Remove(relPath string) error

	// Rename 仅在两个相对路径都在同一挂载点内时支持。
	// 跨挂载点的 move 应由上层做 copy + delete 语义。
	Rename(oldRelPath, newRelPath string) error

	// Reload 重新计算 RootPath（Android uid 变化、env var 变化等场景）。
// 默认实现：no-op。
Reload(m *Mount) error
}

// DriverFactory 是 driver 的无参构造函数。
// 每次调用应返回新的 driver 实例以避免状态污染。
type DriverFactory func() Driver

// ConfigProvider 抽象出 Driver 需要的全局配置（uid / 数据目录 / dev 标志等）。
// 用 interface 而非 *config.Config 直引，是为了避免 mount 包反向依赖 config 包。
type ConfigProvider interface {
	// IsMobile 是否移动模式（mobile overlay 启用）
	IsMobile() bool
	// IsDev 是否开发模式
	IsDev() bool
	// AndroidPackageName Android 包名（dev fallback 不需要）
	AndroidPackageName() string
	// DataDir 数据目录（mounts.json 持久化位置）
	DataDir() string
	// AppDataFallbackDir sandbox/dev 下 appdata 落点
	AppDataFallbackDir() string
	// DevSandboxDir dev sandbox 工作区根
	DevSandboxDir() string
	// ServingDir 主服务根（替代旧 cfg.ServingDir；空字符串表示不创建 primary mount）
	ServingDir() string
	// AutomationDriver automation mount 用的 driver 名（默认 "appdata"；可改 "local" 让 mock 数据真机可见）
	AutomationDriver() string
}
