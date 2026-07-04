// Package mount implements the multi-mount storage system.
//
// 🆕 2026-06-15: 多挂载点存储系统（multi-mount-storage-refactor spec）
//
// 设计目标：
//   - 替代单根 servingDir 架构（Android scoped storage / 多用户 FUSE remap 不可靠）
//   - 每个挂载点独立 root path + driver + enabled + read_only
//   - 向后兼容：cfg.ServingDir 自动迁移为 "primary" 挂载点
//   - 自动化测试走独立 "automation" 挂载点（appdata driver），不污染用户数据
//
// 调用方：
//   - mobile_service / task_manager：路径解析（Phase D 切）
//   - mock_generator：mock 数据写到 automation mount
//   - HTTP API：GET/POST/PUT/DELETE /api/mounts
//   - agent_fs_bridge：ListFSMounts 仍走 serving/webdav，新 ListFSMounts2 可读 registry
//
// 路径形式：
//   - 虚拟路径：/d/<mount_path>/<sub_path>  (e.g. /d/automation/01-plain-media/video/sample.mp4)
//   - 解析后绝对路径：<mount.RootPath>/<sub_path> (e.g. /data/user/0/com.encvgo.app/files/encv-automation/01-plain-media/video/sample.mp4)
package mount

import (
	"errors"
	"strings"
	"time"
)

// Mount 是单个挂载点的描述。JSON 标签用 snake_case 与前端约定一致。
//
// 字段语义：
//   - ID: 全局唯一 UUID，由 registry.Create 时生成
//   - Name: 业务名（slug，唯一）。预置：primary / automation / sandbox
//   - MountPath: 虚拟 URL 前缀，必须以 "/" 开头，不含 ".."
//   - Driver: driver 名，参见 driver.go
//   - RootPath: 物理绝对路径。Driver.Init 时根据 DriverConfig 计算后回填
//   - Enabled: false 时 Registry.Resolve 返回 ErrDisabled
//   - ReadOnly: true 时 driver.Write* 全部返回 ErrReadOnly
//   - DriverConfig: driver 私有配置（如 appdata 的 subpath、local 的 mount flags）
//   - CreatedAt / UpdatedAt: 持久化时间戳
type Mount struct {
	ID           string         `json:"id"`
	Name         string         `json:"name"`
	MountPath    string         `json:"mount_path"`
	Driver       string         `json:"driver"`
	RootPath     string         `json:"root_path"`
	Enabled      bool           `json:"enabled"`
	ReadOnly     bool           `json:"read_only"`
	DriverConfig map[string]any `json:"driver_config,omitempty"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
}

// 预置 driver 名常量。
const (
	DriverLocal   = "local"   // 本地 FS（替代 cfg.ServingDir）
	DriverAppData = "appdata" // Android app-private dir
	DriverSandbox = "sandbox" // dev/sandbox only
)

// 预置 mount name 常量。
const (
	NamePrimary   = "primary"   // 主服务根（替代 cfg.ServingDir）
	NameAutomation = "automation" // 自动化测试命名空间
	NameSandbox   = "sandbox"   // dev/sandbox 工作区
)

// 错误定义。
var (
	ErrNotFound       = errors.New("mount: not found")
	ErrDisabled       = errors.New("mount: disabled")
	ErrReadOnly       = errors.New("mount: read-only")
	ErrNameExists     = errors.New("mount: name already exists")
	ErrMountPathExists = errors.New("mount: mount_path already exists")
	ErrInvalidName    = errors.New("mount: invalid name")
	ErrInvalidPath    = errors.New("mount: invalid mount_path")
	ErrInvalidDriver  = errors.New("mount: invalid driver")
	ErrPrimaryProtected = errors.New("mount: cannot delete or rename 'primary'")
)

// Validate 检查 Mount 字段合法性。Create / Update 时调用。
func (m *Mount) Validate() error {
	if !isValidSlug(m.Name) {
		return ErrInvalidName
	}
	if !strings.HasPrefix(m.MountPath, "/") || strings.Contains(m.MountPath, "..") || strings.Contains(m.MountPath, "//") {
		return ErrInvalidPath
	}
	switch m.Driver {
	case DriverLocal, DriverAppData, DriverSandbox:
		// ok
	default:
		return ErrInvalidDriver
	}
	return nil
}

// isValidSlug 检查 name 是否为合法 slug（小写字母 + 数字 + 下划线 + 中划线，1-64 字符）。
func isValidSlug(s string) bool {
	if s == "" || len(s) > 64 {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= '0' && r <= '9':
		case r == '_' || r == '-':
		default:
			return false
		}
	}
	return true
}
