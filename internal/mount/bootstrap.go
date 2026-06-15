package mount

import (
	"context"
	"fmt"
)

// Bootstrap 根据 ConfigProvider 在空 registry 上创建默认 mount。
//
// 规则（2026-06-15 重设计：所有 mount 总是在所有模式下创建，避免「mount 缺失导致 mock generate 403」）：
//   - primary        → local driver，root = cfg.ServingDir()（dev sandbox 不变） 或 cfg.DataDir()（兜底）
//   - automation     → cfg.AutomationDriver()（默认 appdata），root 由 driver 计算
//                      桌面：cfg.AppDataFallbackDir()/encv-automation
//                      真机：/data/user/<uid>/<pkg>/files/encv-automation
//   - sandbox        → sandbox driver（dev only；prod 模式不创建）
//   - 已存在的同名 mount 不覆盖
//
// 历史（用户 22:29 反馈「Mock generate rejected: invalid mount path，因为没有创建对应挂载」）：
//   - 旧版 automation mount 仅在 cfg.IsMobile()=true 时创建
//   - 真机 APK 启动时如果 mobile overlay 还没生效（cfg.Mobile=nil）→ IsMobile()=false
//   - 或 ENCV_DEV=1 dev 模式 → IsMobile()=false
//   - automation mount 不创建 → 用户跑自动化测试时 POST /api/mock/generate {root:"/d/automation"} → 403
//   - 修复：**无视 IsMobile() 条件**，automation mount 总创建，desktop 模式 appdata driver 自动
//     fallback 到 cfg.AppDataFallbackDir()（默认 os.TempDir()）
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

	// 1. primary（始终创建，root 优先 ServingDir 否则用 DataDir 兜底）
	if r.GetByName(NamePrimary) == nil {
		root := r.cfg.ServingDir()
		if root == "" {
			// 兜底：dev/prod 都没配 serving dir 时用 dataDir
			// （不应该发生，但避免 mount 列表为空）
			root = r.cfg.DataDir()
		}
		if err := r.Create(&Mount{
			Name:      NamePrimary,
			MountPath: "/d/" + NamePrimary, // 🆕 2026-06-15 multi-mount 统一规范：所有 mount 必须以 /d/ 前缀
			Driver:    DriverLocal,
			RootPath:  root,
			Enabled:   true,
		}); err != nil {
			return fmt.Errorf("bootstrap primary: %w", err)
		}
	}

	// 2. automation（始终创建 — appdata driver 自己处理 desktop 真机差异）
	//    🆕 2026-06-15 修复：去掉 cfg.IsMobile() 条件门，避免 mobile overlay 未生效时漏创建
	if r.GetByName(NameAutomation) == nil {
		if err := r.Create(&Mount{
			Name:         NameAutomation,
			MountPath:    "/d/" + NameAutomation, // 🆕 2026-06-15 同上
			Driver:       r.cfg.AutomationDriver(), // 默认 "appdata"，可改 "local"
			Enabled:      true,
			DriverConfig: map[string]any{"subpath": "encv-automation"},
		}); err != nil {
			return fmt.Errorf("bootstrap automation: %w", err)
		}
	}

	// 3. sandbox（dev only — sandbox driver 在非 dev 模式下没意义）
	if r.cfg.IsDev() {
		if r.GetByName(NameSandbox) == nil {
			if err := r.Create(&Mount{
				Name:      NameSandbox,
				MountPath: "/d/" + NameSandbox, // 🆕 2026-06-15 同上
				Driver:    DriverSandbox,
				Enabled:   true,
			}); err != nil {
				return fmt.Errorf("bootstrap sandbox: %w", err)
			}
		}
	}

	return nil
}
