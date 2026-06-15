package server

import (
	"os"

	"github.com/Soltus/encv-go/internal/config"
	"github.com/Soltus/encv-go/internal/mount"
)

// configMountProvider 把 *config.Config 适配为 mount.ConfigProvider。
//
// 字段映射：
//   - ServingDir       → cfg.Server.Dir（mobile overlay 后是 /storage/emulated/0）
//   - IsMobile         → cfg.Mobile != nil（mobile overlay 已应用）
//   - IsDev            → 环境变量 ENCV_DEV=1（dev 模式启用 sandbox mount）
//   - AndroidPackageName → "com.encvgo.app"（硬编码；与 Kotlin EncvGoService 一致）
//   - DataDir          → cfg.Server.DataDir（mounts.json 持久化位置）
//   - AppDataFallbackDir → $TMPDIR/encv-appdata（dev/sandbox 下的 appdata 落点）
//   - DevSandboxDir    → $WORKSPACE/.sandbox 或 $TMPDIR/sandbox-mount
//   - AutomationDriver → "appdata" 默认（用户可在 mounts.json 改 "local" 让 mock 数据真机可见）
type configMountProvider struct {
	cfg *config.Config
}

func (p *configMountProvider) IsMobile() bool {
	return p.cfg.Mobile != nil
}

func (p *configMountProvider) IsDev() bool {
	return os.Getenv("ENCV_DEV") == "1" || os.Getenv("ENCV_DEV_PREVIEW") == "1"
}

func (p *configMountProvider) AndroidPackageName() string {
	// 硬编码：与 android/app/build.gradle.kts applicationId 一致
	return "com.encvgo.app"
}

func (p *configMountProvider) DataDir() string {
	if p.cfg != nil && p.cfg.Server.Dir != "" {
		return p.cfg.Server.Dir
	}
	return os.TempDir()
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
	return mount.DriverAppData // 默认不可见（真机安全）
}

// 编译期断言
var _ mount.ConfigProvider = (*configMountProvider)(nil)
