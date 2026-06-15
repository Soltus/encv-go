package mount

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// MigrateFromServingDir 是 cfg.ServingDir → primary mount 的迁移入口。
//
// 行为：
//   1. 检查 mounts.json 是否存在
//   2. 不存在 → 调 BootstrapFromConfig 创建默认 mount
//   3. 存在但没有 primary mount → 调 BootstrapFromConfig 补齐
//   4. 完成后 Save 一次
//
// 启动流程（server.NewServer）：
//   NewRegistry
//     → RegisterDriverFactory x 3
//     → MigrateFromServingDir
//     → Save (首次启动把 Bootstrap 落盘)
func (r *MountRegistry) MigrateFromServingDir(ctx context.Context) error {
	// Step 1: 检查持久化文件
	needBootstrap := false
	if r.dataPath == "" {
		// 不持久化 → 总是 Bootstrap
		needBootstrap = true
	} else {
		if _, err := os.Stat(r.dataPath); os.IsNotExist(err) {
			needBootstrap = true
		} else {
			// 文件存在，但可能没有 primary
			if r.GetByName(NamePrimary) == nil {
				needBootstrap = true
			}
		}
	}

	if needBootstrap {
		if err := r.BootstrapFromConfig(ctx); err != nil {
			return fmt.Errorf("migrate: bootstrap: %w", err)
		}
		// 落盘
		if r.dataPath != "" {
			if err := r.Save(); err != nil {
				return fmt.Errorf("migrate: save: %w", err)
			}
			fmt.Fprintf(os.Stderr, "[mount] migrate: created %s with %d mount(s)\n",
				filepath.Base(r.dataPath), len(r.List()))
		}
	}
	return nil
}
