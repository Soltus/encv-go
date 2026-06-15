package drivers

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/Soltus/encv-go/internal/mount"
)

// AppDataDriver 封装 Android app-private 目录。
//
// 真机路径：/data/user/<uid>/<package>/files/<subpath>
// Dev/sandbox fallback：<cfg.AppDataFallbackDir>/<subpath>
//
// 多用户安全：uid 从 os.Getuid() 拿，Linux/Android 上自动正确。
// 永远不依赖 /storage/emulated/<n>/ 路径。
type AppDataDriver struct {
	root       string
	subpath    string
	isAndroid  bool
}

// NewAppDataDriver 创建一个 AppDataDriver。
func NewAppDataDriver() *AppDataDriver {
	return &AppDataDriver{isAndroid: runtime.GOOS == "android"}
}

func (d *AppDataDriver) Name() string { return mount.DriverAppData }

func (d *AppDataDriver) Init(ctx context.Context, m *mount.Mount, cfg mount.ConfigProvider) error {
	// 1. 读 DriverConfig.subpath
	sub, _ := m.DriverConfig["subpath"].(string)
	if sub == "" {
		sub = "encv-appdata" // 默认子目录
	}
	d.subpath = sub

	// 2. 计算 RootPath
	if d.isAndroid {
		uid := os.Getuid()
		pkg := ""
		if cfg != nil {
			pkg = cfg.AndroidPackageName()
		}
		if pkg == "" {
			pkg = "com.encvgo.app" // 兜底
		}
		d.root = fmt.Sprintf("/data/user/%d/%s/files/%s", uid, pkg, sub)
	} else {
		// Dev / sandbox fallback
		var fallback string
		if cfg != nil {
			fallback = cfg.AppDataFallbackDir()
		}
		if fallback == "" {
			fallback = filepath.Join(os.TempDir(), "encv-appdata")
		}
		d.root = filepath.Join(fallback, sub)
	}

	// 3. 确保目录存在
	if err := os.MkdirAll(d.root, 0755); err != nil {
		// dev 沙箱下即使失败也不 panic（用户可能在 readonly fs 上跑）
		// 真机会直接报错让 Registry 知道 mount 不可用
		if d.isAndroid {
			return fmt.Errorf("appdata driver: create root %s: %w", d.root, err)
		}
		// dev 下记录到 stderr，但不阻止 init
		fmt.Fprintf(os.Stderr, "[mount] appdata driver: dev fallback mkdir failed: %v\n", err)
	}

	m.RootPath = d.root
	return nil
}

func (d *AppDataDriver) ResolveRoot() string { return d.root }

func (d *AppDataDriver) CheckPermission() error {
	if d.root == "" {
		return fmt.Errorf("appdata driver: root not initialized")
	}
	info, err := os.Stat(d.root)
	if err != nil {
		return fmt.Errorf("appdata driver: stat root %s: %w", d.root, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("appdata driver: root is not a directory: %s", d.root)
	}
	return nil
}

func (d *AppDataDriver) Stat(relPath string) (os.FileInfo, error) {
	return os.Stat(d.absPath(relPath))
}

func (d *AppDataDriver) ReadDir(relPath string) ([]os.DirEntry, error) {
	return os.ReadDir(d.absPath(relPath))
}

func (d *AppDataDriver) ReadFile(relPath string) ([]byte, error) {
	return os.ReadFile(d.absPath(relPath))
}

func (d *AppDataDriver) WriteFile(relPath string, data []byte, perm os.FileMode) error {
	abs := d.absPath(relPath)
	if err := os.MkdirAll(filepath.Dir(abs), 0755); err != nil {
		return err
	}
	return os.WriteFile(abs, data, perm)
}

func (d *AppDataDriver) MkdirAll(relPath string, perm os.FileMode) error {
	return os.MkdirAll(d.absPath(relPath), perm)
}

func (d *AppDataDriver) Remove(relPath string) error {
	return os.Remove(d.absPath(relPath))
}

func (d *AppDataDriver) Rename(oldRelPath, newRelPath string) error {
	return os.Rename(d.absPath(oldRelPath), d.absPath(newRelPath))
}

func (d *AppDataDriver) Reload(m *mount.Mount) error {
	oldRoot := d.root
	if err := d.Init(context.Background(), m, nil); err != nil {
		return err
	}
	// Reload 后 RootPath 变了，记录一次
	if oldRoot != d.root {
		fmt.Fprintf(os.Stderr, "[mount] appdata driver: reloaded root %s -> %s\n", oldRoot, d.root)
	}
	return nil
}

func (d *AppDataDriver) absPath(relPath string) string {
	cleaned := filepath.Clean("/" + relPath)
	return filepath.Join(d.root, cleaned)
}
