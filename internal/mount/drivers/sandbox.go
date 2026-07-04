package drivers

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Soltus/encv-go/internal/mount"
)

// SandboxDriver 封装 dev 沙箱工作区。
//
// 仅 dev 模式可用：真机 release 上 Init 直接失败 → Registry 标记 ErrDisabled。
// 用途：dev 测试时给一个独立的、可读写的 mount，区别于 primary。
type SandboxDriver struct {
	root string
}

// NewSandboxDriver 创建一个 SandboxDriver。
func NewSandboxDriver() *SandboxDriver {
	return &SandboxDriver{}
}

func (d *SandboxDriver) Name() string { return mount.DriverSandbox }

func (d *SandboxDriver) Init(ctx context.Context, m *mount.Mount, cfg mount.ConfigProvider) error {
	// 真机 / 非 dev 模式直接拒绝
	if cfg == nil || !cfg.IsDev() {
		return fmt.Errorf("sandbox driver: only available in dev mode")
	}
	root := m.RootPath
	if root == "" {
		root = cfg.DevSandboxDir()
	}
	if root == "" {
		// 兜底：$TMPDIR/sandbox-mount
		root = filepath.Join(os.TempDir(), "sandbox-mount")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("sandbox driver: resolve abs path: %w", err)
	}
	if err := os.MkdirAll(abs, 0755); err != nil {
		return fmt.Errorf("sandbox driver: create root %s: %w", abs, err)
	}
	d.root = abs
	m.RootPath = abs
	return nil
}

func (d *SandboxDriver) ResolveRoot() string { return d.root }

func (d *SandboxDriver) CheckPermission() error {
	if d.root == "" {
		return fmt.Errorf("sandbox driver: root not initialized")
	}
	info, err := os.Stat(d.root)
	if err != nil {
		return fmt.Errorf("sandbox driver: stat root: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("sandbox driver: root is not a directory: %s", d.root)
	}
	return nil
}

func (d *SandboxDriver) Stat(relPath string) (os.FileInfo, error) {
	return os.Stat(d.absPath(relPath))
}

func (d *SandboxDriver) ReadDir(relPath string) ([]os.DirEntry, error) {
	return os.ReadDir(d.absPath(relPath))
}

func (d *SandboxDriver) ReadFile(relPath string) ([]byte, error) {
	return os.ReadFile(d.absPath(relPath))
}

func (d *SandboxDriver) WriteFile(relPath string, data []byte, perm os.FileMode) error {
	abs := d.absPath(relPath)
	if err := os.MkdirAll(filepath.Dir(abs), 0755); err != nil {
		return err
	}
	return os.WriteFile(abs, data, perm)
}

func (d *SandboxDriver) MkdirAll(relPath string, perm os.FileMode) error {
	return os.MkdirAll(d.absPath(relPath), perm)
}

func (d *SandboxDriver) Remove(relPath string) error {
	return os.Remove(d.absPath(relPath))
}

func (d *SandboxDriver) Rename(oldRelPath, newRelPath string) error {
	return os.Rename(d.absPath(oldRelPath), d.absPath(newRelPath))
}

func (d *SandboxDriver) Reload(m *mount.Mount) error {
	return d.Init(context.Background(), m, nil)
}

func (d *SandboxDriver) absPath(relPath string) string {
	cleaned := filepath.Clean("/" + relPath)
	return filepath.Join(d.root, cleaned)
}
