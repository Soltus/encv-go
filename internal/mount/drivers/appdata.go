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

// ComputeRootPath 根据 cfg + subpath 计算 Android 真机 / dev fallback 的物理绝对路径。
//
// 🆕 2026-06-16 拆出：让测试能在不实际 mkdir 的情况下验证路径逻辑。
// 旧实现把路径计算和 mkdir 混在 Init 里，单元测试无法验证路径字符串。
//   - Android 优先 cfg.DataDir()（Kotlin 注入 ENCV_APP_FILES_DIR = context.filesDir）
//   - Android 兜底：硬编码 /data/user/<uid>/<pkg>/files（仅用于 cfg=nil 的退化场景）
//   - Dev fallback：cfg.AppDataFallbackDir() 或 $TMPDIR/encv-appdata
//
// 返回 (rootPath, subpath, usedAndroid)。如果 cfg 不可用，usedAndroid=false 走 dev fallback。
func (d *AppDataDriver) ComputeRootPath(cfg mount.ConfigProvider, subpath string) (root string, usedAndroid bool) {
	if d.isAndroid {
		var base string
		if cfg != nil {
			base = cfg.DataDir()
		}
		if base == "" {
			uid := os.Getuid()
			pkg := "com.encvgo.app"
			if cfg != nil && cfg.AndroidPackageName() != "" {
				pkg = cfg.AndroidPackageName()
			}
			base = fmt.Sprintf("/data/user/%d/%s/files", uid, pkg)
		}
		return filepath.Join(base, subpath), true
	}
	var fallback string
	if cfg != nil {
		fallback = cfg.AppDataFallbackDir()
	}
	if fallback == "" {
		fallback = filepath.Join(os.TempDir(), "encv-appdata")
	}
	return filepath.Join(fallback, subpath), false
}

func (d *AppDataDriver) Init(ctx context.Context, m *mount.Mount, cfg mount.ConfigProvider) error {
	// 1. 读 DriverConfig.subpath
	sub, _ := m.DriverConfig["subpath"].(string)
	if sub == "" {
		sub = "encv-appdata" // 默认子目录
	}
	d.subpath = sub

	// 2. 计算 RootPath（拆出 ComputeRootPath 给测试用）
	d.root, _ = d.ComputeRootPath(cfg, sub)

	// 3. 创建 root 目录
	//   🆕 2026-06-16 修复真机 appdata 权限问题：
	//   旧实现：os.MkdirAll(d.root, 0755) — 在真机上撞到 EACCES
	//     真机路径：/data/user/<uid>/<pkg>/files/<subpath>
	//     根因：/data/user/<uid>/ 在某些 Android 设备上是 sdcardfs/fuse remap 的特殊挂载点
	//     os.MkdirAll 走到 /data/user/<uid>/ 时 stat 已存在但 PathEvaluation 后 EACCES
	//     → "mkdir /data/user/10490: permission denied"
	//   新实现：先 os.Stat(d.root) 判断是否已存在；不存在时只 os.Mkdir 叶子节点（不递归创建父目录）
	//     因为 Android 系统在 app 安装时会自动创建 /data/user/<uid>/<pkg>/files/，我们只管自己那层
	if d.isAndroid {
		if _, err := os.Stat(d.root); err == nil {
			// 已存在 — 通常发生在老 mounts.json 持久化路径命中，或重启后系统已创建
		} else if !os.IsNotExist(err) {
			// stat 失败原因不明（EACCES 自身 / EIO 等）— 透传给上层
			return fmt.Errorf("appdata driver: stat root %s: %w", d.root, err)
		} else {
			// 不存在 → 只 mkdir 叶子（不 MkdirAll 父目录）
			// Android 系统的 /data/user/<uid>/<pkg>/files/ 必定存在（app install 时由 pm 创建）
			// 若父目录缺失（极罕见）→ 我们会得到 ENOENT，提示用户重装 app
			if err := os.Mkdir(d.root, 0755); err != nil && !os.IsExist(err) {
				return fmt.Errorf("appdata driver: create root %s: %w (Android 系统 files/ 父目录可能缺失，请重装 app)", d.root, err)
			}
		}
	} else {
		// dev 沙箱：保留 MkdirAll（AppDataFallbackDir 比如 os.TempDir() 没预创建父目录）
		if err := os.MkdirAll(d.root, 0755); err != nil {
			fmt.Fprintf(os.Stderr, "[mount] appdata driver: dev fallback mkdir failed: %v\n", err)
		}
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
