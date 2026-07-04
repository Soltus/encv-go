package mount

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
)

// MigrateFromServingDir 是 cfg.ServingDir → primary mount 的迁移入口。
//
// 行为（2026-06-15 修复启动加载缺失）：
//  1. migrateLegacyDataPath：老路径 serving_dir/mounts.json → 新路径 serving_dir/.encv/mounts.json
//  2. Load：把持久化文件里的 mount 列表恢复到内存
//  3. 检查是否需要 Bootstrap（仅当 Load 之后内存仍没有 primary mount 才补齐）
//  4. 完成后 Save 一次（仅在 Bootstrap 触发时）
//
// 启动流程（server.NewServer）：
//   NewRegistry
//     → RegisterDriverFactory x 3
//     → MigrateFromServingDir（migrate → load → bootstrap? → save?）
//
// 2026-06-15 修复：原版只 os.Stat 检查 dataPath + 内存 GetByName，**Load 从未被调用**。
// 后果是：每次启动都走 Bootstrap，用户的自定义 mount 配置被默认配置覆盖。
// 老路径 serving_dir/mounts.json 也永远没人读取（成了孤儿文件，但还在原位）。
// 现在显式 migrate + load，再判断 bootstrap 必要性。
func (r *MountRegistry) MigrateFromServingDir(ctx context.Context) error {
	// Step 1: 迁移老路径数据到新路径（一次性，原子 rename）
	if err := r.migrateLegacyDataPath(); err != nil {
		// 迁移失败不致命：继续尝试从 dataPath 读（也许 dataPath 已经存在）
		// 🆕 2026-06-16：slog.Warn 让 DevLogs 也能看到（不只 stderr）
		slog.Warn("mount migrate: legacy migration failed", "err", err)
	}

	// Step 2: 加载持久化数据到内存（如果存在）
	if err := r.Load(); err != nil {
		// Load 失败不致命：fallback 到 Bootstrap（全新数据）
		// 🆕 2026-06-16：slog.Warn 让 DevLogs 也能看到（不只 stderr）
		slog.Warn("mount migrate: load failed (will bootstrap)", "err", err)
	}

	// Step 3: 检查持久化文件 + 关键 mount 是否齐全
	//
	// 🆕 2026-06-16 修复：原版只看 primary 缺失 → 触发 Bootstrap。但真机历史上
	// 持久化的 mounts.json 可能只含 primary（早期版本没 automation / 用户手动
	// 删了 / 写盘时被截断），Load 恢复后内存里 automation 不存在 → needBootstrap
	// 判定为 false → 永远不补齐 automation → API 只看到 1 个 mount。
	//
	// 修复：needBootstrap 判定为「primary 或 automation 任意一个缺失」都触发
	// Bootstrap（BootstrapFromConfig 内部每个 mount 都用 GetByName 判定是否补齐，
	// 已存在的不会被覆盖，用户的自定义 mount 保留）。
	needBootstrap := false
	if r.dataPath == "" {
		// 不持久化 → 总是 Bootstrap
		needBootstrap = true
	} else {
		if _, err := os.Stat(r.dataPath); os.IsNotExist(err) {
			needBootstrap = true
		} else {
			// 文件存在但内存里缺 primary 或 automation → 补齐
			// （sandbox 是 dev-only，缺了不强制补；BootstrapFromConfig 内部按 IsDev 决定）
			if r.GetByName(NamePrimary) == nil || r.GetByName(NameAutomation) == nil {
				needBootstrap = true
			}
		}
	}

	if needBootstrap {
		// 🆕 2026-06-16：记录 Bootstrap 触发前的 mount 列表，便于排查「哪些 mount 是补齐的」
		// 推到 WSLogHandler → DevLogs 能看到「automation mount 补齐: ...」
		beforeList := make([]string, 0, len(r.List()))
		for _, m := range r.List() {
			beforeList = append(beforeList, m.Name)
		}
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
		// 🆕 2026-06-16：slog.Info 让 DevLogs 看到 bootstrap 补齐结果
		//   - 真机调试：用户能直接在 DevLogs 后端日志面板看到「automation mount 已补齐」
		//   - 排查：mounts.json 缺 mount → Bootstrap 触发 → 这里打 INFO 级别日志
		afterList := make([]string, 0, len(r.List()))
		for _, m := range r.List() {
			afterList = append(afterList, m.Name)
		}
		added := DiffStrings(beforeList, afterList)
		if len(added) > 0 {
			slog.Info("mount bootstrap: 补齐缺失的挂载点", "added", added, "total", afterList)
		} else {
			slog.Info("mount bootstrap: 完整，无需补齐", "total", afterList)
		}
	}
	return nil
}

// DiffStrings 返回 in b 但不在 a 的元素（a/b 都是 mount name 列表）
// 用于「调用前后 mount 列表对比 → 找出补齐项」，目前主要在 migrate.go 内部使用
// 2026-06-16：原 handleRefreshMountsGin 已删除（/api/mounts/refresh 端点被移除）
//   DiffStrings 保留导出以便未来如果需要再次暴露"补齐 diff"能力时复用
func DiffStrings(a, b []string) []string {
	inA := make(map[string]struct{}, len(a))
	for _, s := range a {
		inA[s] = struct{}{}
	}
	var added []string
	for _, s := range b {
		if _, ok := inA[s]; !ok {
			added = append(added, s)
		}
	}
	return added
}
