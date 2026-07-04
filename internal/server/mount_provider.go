package server

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Soltus/encv-go/internal/config"
	"github.com/Soltus/encv-go/internal/mount"
	mobileservice "github.com/Soltus/encv-go/internal/service"
)

// configMountProvider 把 *config.Config 适配为 mount.ConfigProvider。
//
// 字段映射（2026-06-15 重设计，data dir 与 serving dir 完全分离）：
//   - ServingDir         → cfg.Server.Dir（用户媒体目录，dev=/workspace / mobile overlay=/storage/emulated/0）
//   - IsMobile           → cfg.Mobile != nil
//   - IsDev              → 环境变量 ENCV_DEV=1 或 ENCV_DEV_PREVIEW=1
//   - AndroidPackageName → "com.encvgo.app"（硬编码；可被 ENCV_PACKAGE_NAME 覆盖）
//   - DataDir            → 应用私有 data 目录（Android: /data/user/0/<pkg>/files；桌面: XDG_DATA_HOME/encv[/dev]）
//   - AppDataFallbackDir → $TMPDIR/encv-appdata（dev/sandbox 下的 appdata 落点，appdata driver 用）
//   - DevSandboxDir      → $ENCV_SANDBOX_DIR 或空
//   - AutomationDriver   → "appdata" 默认
type configMountProvider struct {
	cfg *config.Config
}

func (p *configMountProvider) IsMobile() bool {
	if p.cfg == nil {
		// 测试 / 启动期 cfg 还没注入：用 env 兜底（与 defaultMountRegistryDataPath 一致）
		return os.Getenv("ENCV_MOBILE") == "1"
	}
	return p.cfg.Mobile != nil
}

func (p *configMountProvider) IsDev() bool {
	return os.Getenv("ENCV_DEV") == "1" || os.Getenv("ENCV_DEV_PREVIEW") == "1"
}

func (p *configMountProvider) AndroidPackageName() string {
	if v := os.Getenv("ENCV_PACKAGE_NAME"); v != "" {
		return v
	}
	// 硬编码：与 android/app/build.gradle.kts applicationId 一致
	return "com.encvgo.app"
}

// DataDir 返回应用私有 data 目录。
//
// 平台分支（2026-06-15 用户反馈"放应用 data 路径"，与 servingDir 解耦）：
//   - Android: 优先 ENCV_APP_FILES_DIR（Kotlin 端 EncvGoService 注入 `context.filesDir.absolutePath`）
//     → fallback: /data/user/0/<pkg>/files
//   - 桌面:  XDG_DATA_HOME/encv[/dev]
//
// 为什么不返回 cfg.Server.Dir：
//   - cfg.Server.Dir 在 dev 模式是项目工作目录（/workspace）
//   - cfg.Server.Dir 在 mobile overlay 后是 /storage/emulated/0（Android 公共存储根）
//   - 这两个都不是 app 私有目录，会污染用户视图
func (p *configMountProvider) DataDir() string {
	if p.IsMobile() {
		filesDir := os.Getenv("ENCV_APP_FILES_DIR")
		if filesDir == "" {
			filesDir = filepath.Join("/data/user/0", p.AndroidPackageName(), "files")
		}
		return filesDir
	}
	// 桌面 dev/production 走 XDG（与 mountRegistryDataPath 保持一致）
	return desktopDataDirForMount(p.IsDev())
}

// desktopDataDirForMount 桌面端 data dir 复用 mountRegistryDataPath 的解析逻辑。
func desktopDataDirForMount(isDev bool) string {
	base := os.Getenv("XDG_DATA_HOME")
	if base == "" {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, ".local", "share")
	}
	sub := "encv"
	if isDev {
		sub = "encv-dev"
	}
	return filepath.Join(base, sub)
}

func (p *configMountProvider) AppDataFallbackDir() string {
	if v := os.Getenv("ENCV_APPDATA_DIR"); v != "" {
		return v
	}
	return os.TempDir()
}

func (p *configMountProvider) DevSandboxDir() string {
	if v := os.Getenv("ENCV_SANDBOX_DIR"); v != "" {
		return v
	}
	return ""
}

func (p *configMountProvider) ServingDir() string {
	if p.cfg == nil {
		return ""
	}
	return p.cfg.Server.Dir
}

func (p *configMountProvider) AutomationDriver() string {
	if v := os.Getenv("ENCV_AUTOMATION_DRIVER"); v != "" {
		return v
	}
	// 🆕 2026-06-15 默认 = "appdata"（app-private 路径，真机有权限）
	//
	// 警告：改成 "local" 是致命错！
	//   - local driver 在真机写 /storage/emulated/0/encv-automation/
	//   - /storage/emulated/0/ 是 Android shared storage
	//   - Android 11+ 需要 MANAGE_EXTERNAL_STORAGE 权限
	//   - Android 13+ scoped storage 写裸路径直接 EACCES
	//   - app-private 路径（/data/user/0/<pkg>/files/）不需要任何运行时权限
	//   - 不要为了"用户在文件管理器里看到"而切到 local — 真机直接崩
	//
	// 想要 local（dev 沙箱 shell 可见）的 opt-in：ENCV_AUTOMATION_DRIVER=local
	return mount.DriverAppData
}

// 编译期断言
var _ mount.ConfigProvider = (*configMountProvider)(nil)

// primaryRootProvider 把 mount.MountRegistry 适配为 service.MountRootProvider。
//
// 用途：让 service.MobileService 拿到 primary mount 的 RootPath 用于删除守卫。
// 桥接链：server.NewServer 构造 mount.MountRegistry → 包成 primaryRootProvider
//   → 注入到 mobileSvc.SetMountRegistry(reg) → mobileSvc.primaryRootPath() 用
type primaryRootProvider struct {
	reg *mount.MountRegistry
}

func (p *primaryRootProvider) GetPrimaryRootPath() string {
	if p == nil || p.reg == nil {
		return ""
	}
	if m := p.reg.GetByName("primary"); m != nil {
		return m.RootPath
	}
	return ""
}

// 编译期断言
var _ mobileservice.MountRootProvider = (*primaryRootProvider)(nil)

// taskMountResolver 把 mount.MountRegistry 适配为 service.MountResolver。
//
// 用途：让 service.TaskManager 拿到 mount 解析后的 abs path / mount_id / sub_path。
//
// 桥接链：server.NewServer 构造 mount.MountRegistry → 包成 taskMountResolver
//   → 注入到 taskManager.SetMountResolver(r) → resolveAbsPath("/d/automation/...") 用
type taskMountResolver struct {
	reg *mount.MountRegistry
}

func (t *taskMountResolver) Resolve(virtualPath string) (*mobileservice.MountResolveResult, error) {
	if t == nil || t.reg == nil {
		return nil, fmt.Errorf("taskMountResolver: nil registry")
	}
	res, err := t.reg.Resolve(virtualPath)
	if err != nil {
		return nil, err
	}
	if res == nil || res.Mount == nil {
		return nil, fmt.Errorf("taskMountResolver: empty resolve result")
	}
	return &mobileservice.MountResolveResult{
		MountID: res.Mount.ID,
		AbsPath: res.AbsPath,
		SubPath: res.RelPath,
	}, nil
}

// 🆕 v3 2026-06-18 Task 8：absPath → virtualPath 反向解析
//   - 供 task_manager 把 task.OutputPath / step.Detail 转为虚拟路径
func (t *taskMountResolver) AbsToVirtual(absPath string) (string, error) {
	if t == nil || t.reg == nil {
		return "", fmt.Errorf("taskMountResolver: nil registry")
	}
	return t.reg.AbsToVirtual(absPath)
}

// 编译期断言
var _ mobileservice.MountResolver = (*taskMountResolver)(nil)

// 编译期断言：保证 service.MountResolveResult 是 mount.ResolveResult 的形状子集
// （任何字段变动会触发这里的不兼容 — 提示重新适配）
var _ struct {
	MountID string
	AbsPath string
	SubPath string
} = mobileservice.MountResolveResult{}
