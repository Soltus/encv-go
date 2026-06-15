package mount

import (
	"context"
	"fmt"
)

// Bootstrap 根据 ConfigProvider 在空 registry 上创建默认 mount。
//
// 规则：
//   - cfg.ServingDir != "" → 创建 primary（local driver）
//   - cfg.IsMobile() → 创建 automation（appdata driver）
//   - cfg.IsDev() → 创建 sandbox（sandbox driver）
//   - 已存在的同名 mount 不覆盖
//
// 启动顺序（在 server.NewServer 中）：
//   1. NewRegistry(cfg, dataPath)
//   2. RegisterDriverFactory(...) x 3
//   3. Load(dataPath)            // 加载持久化
//   4. 若 Load 失败 / 文件不存在 → BootstrapFromConfig(cfg)  // 用 cfg 创建默认
//   5. 启动时检查 critical mount 存在（primary 必须有）
func (r *MountRegistry) BootstrapFromConfig(ctx context.Context) error {
	if r.cfg == nil {
		return fmt.Errorf("mount: ConfigProvider is nil")
	}

	// 1. primary
	if r.cfg.ServingDir() != "" {
		if r.GetByName(NamePrimary) == nil {
			if err := r.Create(&Mount{
				Name:      NamePrimary,
				MountPath: "/" + NamePrimary,
				Driver:    DriverLocal,
				RootPath:  r.cfg.ServingDir(),
				Enabled:   true,
			}); err != nil {
				return fmt.Errorf("bootstrap primary: %w", err)
			}
		}
	}

	// 2. automation（mobile / Android 模式）
	if r.cfg.IsMobile() {
		if r.GetByName(NameAutomation) == nil {
			if err := r.Create(&Mount{
				Name:         NameAutomation,
				MountPath:    "/" + NameAutomation,
				Driver:       r.cfg.AutomationDriver(), // 默认 "appdata"，可改 "local"
				Enabled:      true,
				DriverConfig: map[string]any{"subpath": "encv-automation"},
			}); err != nil {
				return fmt.Errorf("bootstrap automation: %w", err)
			}
		}
	}

	// 3. sandbox（dev 模式）
	if r.cfg.IsDev() {
		if r.GetByName(NameSandbox) == nil {
			if err := r.Create(&Mount{
				Name:      NameSandbox,
				MountPath: "/" + NameSandbox,
				Driver:    DriverSandbox,
				Enabled:   true,
			}); err != nil {
				return fmt.Errorf("bootstrap sandbox: %w", err)
			}
		}
	}

	return nil
}
