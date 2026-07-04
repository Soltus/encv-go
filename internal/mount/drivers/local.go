// Package drivers 包含 mount 包的 driver 实现。
package drivers

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Soltus/encv-go/internal/mount"
)

// LocalDriver 封装本地文件系统（替代 cfg.ServingDir）。
//
// 适用场景：dev 沙箱 / 桌面 / Android 上 shared storage 子路径
// 不适用：真机 release 上 automation 命名空间（应改用 AppDataDriver）
type LocalDriver struct {
	root string
}

// NewLocalDriver 创建一个 LocalDriver。
func NewLocalDriver() *LocalDriver {
	return &LocalDriver{}
}

func (d *LocalDriver) Name() string { return mount.DriverLocal }

func (d *LocalDriver) Init(ctx context.Context, m *mount.Mount, cfg mount.ConfigProvider) error {
	if m.RootPath == "" {
		return fmt.Errorf("local driver: RootPath is required")
	}
	abs, err := filepath.Abs(m.RootPath)
	if err != nil {
		return fmt.Errorf("local driver: resolve abs path: %w", err)
	}
	d.root = abs
	m.RootPath = abs
	return nil
}

func (d *LocalDriver) ResolveRoot() string { return d.root }

func (d *LocalDriver) CheckPermission() error {
	if d.root == "" {
		return fmt.Errorf("local driver: root not initialized")
	}
	info, err := os.Stat(d.root)
	if err != nil {
		return fmt.Errorf("local driver: stat root: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("local driver: root is not a directory: %s", d.root)
	}
	return nil
}

func (d *LocalDriver) Stat(relPath string) (os.FileInfo, error) {
	return os.Stat(d.absPath(relPath))
}

func (d *LocalDriver) ReadDir(relPath string) ([]os.DirEntry, error) {
	return os.ReadDir(d.absPath(relPath))
}

func (d *LocalDriver) ReadFile(relPath string) ([]byte, error) {
	return os.ReadFile(d.absPath(relPath))
}

func (d *LocalDriver) WriteFile(relPath string, data []byte, perm os.FileMode) error {
	abs := d.absPath(relPath)
	if err := os.MkdirAll(filepath.Dir(abs), 0755); err != nil {
		return err
	}
	return os.WriteFile(abs, data, perm)
}

func (d *LocalDriver) MkdirAll(relPath string, perm os.FileMode) error {
	return os.MkdirAll(d.absPath(relPath), perm)
}

func (d *LocalDriver) Remove(relPath string) error {
	return os.Remove(d.absPath(relPath))
}

func (d *LocalDriver) Rename(oldRelPath, newRelPath string) error {
	return os.Rename(d.absPath(oldRelPath), d.absPath(newRelPath))
}

func (d *LocalDriver) Reload(m *mount.Mount) error {
	return d.Init(context.Background(), m, nil)
}

// absPath 拼装绝对路径并做安全检查。
//
// 安全策略：path 已被 registry 层 Resolve() 验证过不会逃逸 root。
// 这里再做一次 final 检查（防 driver 被绕过）。
func (d *LocalDriver) absPath(relPath string) string {
	cleaned := filepath.Clean("/" + relPath) // 防止 relPath 以 "/" 开头绕过
	abs := filepath.Join(d.root, cleaned)
	return abs
}
