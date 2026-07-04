package mount

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// mountFile 是 mounts.json 的顶层结构。
type mountFile struct {
	Version int       `json:"version"`
	Mounts  []*Mount  `json:"mounts"`
	SavedAt time.Time `json:"saved_at"`
}

// mountFileVersion 是 mounts.json 的 schema 版本号。
// 升级时按版本号分支处理（v1 字段结构见 mountFile）。
const mountFileVersion = 1

// legacyDataBasename 是 mount 子系统历史数据文件 basename。
//
// 2026-06-15 历史背景：原本 mounts.json 存放在 serving dir 根目录（cfg.Server.Dir/mounts.json），
// 导致 Files 视图会列出该 json（用户能"看到"系统配置），迁移后放在
// serving dir 的 .encv/ 隐藏子目录里（dotfile filter 自动隐藏）。
const legacyDataBasename = "mounts.json"

// migrateLegacyDataPath 一次性处理老路径数据，迁移到新路径或移到 forensic 备份。
//
// 触发条件：老路径文件存在（servingDir 范围内的 dev 沙箱老用法 + v1 散落）
//
// 候选 legacy 路径（按优先级检查，找到第一个存在的就处理）：
//  1. <servingDir>/mounts.json                  — v1 之前散落根目录
//  2. <servingDir>/.encv/mounts.json            — dev 沙箱老用法
//
// 操作矩阵（每个候选 legacy）：
//
//	老文件  新文件   操作
//	✓       ✗       老 → 新（原子 rename）
//	✓       ✓       atomic swap：老 → 新（用户真实数据），新 → .migrated-<unix>
//	✗       *       skip（继续检查下一个候选）
//
// 2026-06-15 修复根因（三重 bug）：
//  1. 持久化路径选错：把 cfg.Server.Dir（用户媒体目录）当 dataDir → mounts.json 污染用户视图
//     — 重设计 mountRegistryDataPath：分平台/分环境选 app data 目录
//  2. "新文件存在就跳过" → 老文件永远残留
//     — 改为 atomic swap
//  3. 启动流程不调 Load → 即使迁移成功，用户数据也没恢复
//     — MigrateFromServingDir 显式调 Load
//
// 安全性：
//   - 不删任何文件（rename only）→ 用户数据可恢复
//   - 失败的中间状态：rename 失败返回 error，调用方 log 后继续（不阻塞启动）
//   - dev / production 隔离：只迁移 servingDir 范围内的老文件，不动 XDG/AppData 的 production 数据
func (r *MountRegistry) migrateLegacyDataPath() error {
	if r.dataPath == "" {
		return nil
	}
	// 收集所有可能的 legacy 路径
	candidates := r.legacyDataPathCandidates()
	if len(candidates) == 0 {
		return nil
	}

	for _, legacyPath := range candidates {
		if legacyPath == r.dataPath {
			continue // 同一路径
		}
		// 老文件不存在 → skip
		if _, err := os.Stat(legacyPath); err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			return fmt.Errorf("stat legacy %s: %w", legacyPath, err)
		}

		// 老文件存在 → 看新文件存不存在
		newExists := false
		if _, err := os.Stat(r.dataPath); err == nil {
			newExists = true
		} else if !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("stat new %s: %w", r.dataPath, err)
		}

		// 创建目标目录
		if err := os.MkdirAll(filepath.Dir(r.dataPath), 0755); err != nil {
			return fmt.Errorf("mkdir %s: %w", filepath.Dir(r.dataPath), err)
		}

		if newExists {
			// 两个文件都存在 → atomic swap：以老文件（用户真实数据）为准
			bakPath := fmt.Sprintf("%s.migrated-%d", r.dataPath, time.Now().Unix())
			if err := renameAcrossDevices(r.dataPath, bakPath); err != nil {
				return fmt.Errorf("rename new to backup %s: %w", bakPath, err)
			}
			if err := renameAcrossDevices(legacyPath, r.dataPath); err != nil {
				return fmt.Errorf("rename legacy %s -> %s: %w", legacyPath, r.dataPath, err)
			}
			fmt.Fprintf(os.Stderr, "[mount] legacy mounts.json swapped in (user data restored): %s -> %s, previous new file archived to %s\n",
				legacyPath, r.dataPath, bakPath)
		} else {
			// 新文件不存在 → 老 → 新原子 rename
			if err := renameAcrossDevices(legacyPath, r.dataPath); err != nil {
				return fmt.Errorf("rename %s -> %s: %w", legacyPath, r.dataPath, err)
			}
			fmt.Fprintf(os.Stderr, "[mount] migrated legacy mounts.json: %s -> %s\n", legacyPath, r.dataPath)
		}
		// 处理完一个 legacy 后返回（避免多个 legacy 互相打架）
		return nil
	}
	return nil
}

// legacyDataPathCandidates 收集可能的 legacy 路径。
//
// 范围：cfg.ServingDir() 内的 mounts.json 散落文件
//  - v1 之前 binary 把 mounts.json 写在 <servingDir>/ 根目录
//  - 中间过渡版本写到 <servingDir>/.encv/mounts.json（dev 沙箱）
//
// 不在范围：
//  - XDG / AppData 目录（那是 production / dev 的新位置，dev 启动时不应覆盖 production）
//  - /tmp（兜底路径，每次重启就清空）
func (r *MountRegistry) legacyDataPathCandidates() []string {
	if r.cfg == nil {
		return nil
	}
	servingDir := r.cfg.ServingDir()
	if servingDir == "" {
		return nil
	}
	return []string{
		filepath.Join(servingDir, legacyDataBasename),                   // <servingDir>/mounts.json
		filepath.Join(servingDir, ".encv", legacyDataBasename),         // <servingDir>/.encv/mounts.json
	}
}

// Load 从 dataPath 加载挂载点列表。
//
// 行为：
//   - 文件不存在：返回 nil（首次启动，由 Bootstrap 创建默认）
//   - 文件存在但解析失败：返回错误（让调用方决定是否回退 Bootstrap）
//   - 文件为空：返回空列表
//   - 成功：覆盖当前 registry 状态（不合并）
//
// 2026-06-15：一次性迁移 —— 如果新路径不存在 + 旧路径（serving_dir/mounts.json）存在，
// 先迁移到新路径（serving_dir/.encv/mounts.json），再继续 Load。
// 迁移后旧文件 rename 走（原子），失败时新路径有正确数据 + 旧文件保留（forensic）。
func (r *MountRegistry) Load() error {
	if r.dataPath == "" {
		return nil // 不持久化
	}
	if err := r.migrateLegacyDataPath(); err != nil {
		// 迁移失败不致命：继续尝试从 dataPath 读（也许 dataPath 已经存在）
		fmt.Fprintf(os.Stderr, "[mount] load: legacy migration failed: %v\n", err)
	}
	data, err := os.ReadFile(r.dataPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("mount: read %s: %w", r.dataPath, err)
	}
	if len(data) == 0 {
		return nil
	}
	var mf mountFile
	if err := json.Unmarshal(data, &mf); err != nil {
		return fmt.Errorf("mount: parse %s: %w", r.dataPath, err)
	}

	// 替换现有 mount 列表
	r.mu.Lock()
	r.mounts = nil
	r.byID = make(map[string]*Mount)
	r.byName = make(map[string]*Mount)
	for _, m := range mf.Mounts {
		if m == nil {
			continue
		}
		r.mounts = append(r.mounts, m)
		r.byID[m.ID] = m
		r.byName[m.Name] = m
	}
	r.mu.Unlock()

	// 重新 init driver（让 RootPath 按当前环境重算）
	//
	// 2026-06-15 修复：原版在持 r.mu.Lock() 写锁时调 instantiate，instantiate fail path
	// 会调 ListDrivers 拿 RLock → 写锁 + 读锁自死锁。改成 RLock + instantiate 内部
	// ListDrivers 也 RLock（RWMutex 允许嵌套 RLock，不会死锁）。
	// drivers map 启动期写、运行期只读，RLock 安全。
	ctx := context.Background()
	r.mu.RLock()
	for _, m := range r.mounts {
		driver, err := r.instantiate(m.Driver)
		if err != nil {
			r.mu.RUnlock()
			return fmt.Errorf("mount: load %s: instantiate %s: %w", m.Name, m.Driver, err)
		}
		if err := driver.Init(ctx, m, r.cfg); err != nil {
			// Init 失败不致命（dev sandbox 某些路径可能暂时不可用）
			// 只记日志，不返回错误
			fmt.Fprintf(os.Stderr, "[mount] load: init %s failed: %v\n", m.Name, err)
		}
	}
	r.mu.RUnlock()
	return nil
}

// Save 持久化当前挂载点列表到 dataPath。
//
// 行为：
//   - 创建父目录
//   - 原子写：写到 .tmp → os.Rename
//   - 失败不 panic（让调用方决定）
func (r *MountRegistry) Save() error {
	if r.dataPath == "" {
		return nil
	}
	mf := mountFile{
		Version: mountFileVersion,
		Mounts:  r.List(),
		SavedAt: r.clock(),
	}
	data, err := json.MarshalIndent(mf, "", "  ")
	if err != nil {
		return fmt.Errorf("mount: marshal: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(r.dataPath), 0755); err != nil {
		return fmt.Errorf("mount: mkdir %s: %w", filepath.Dir(r.dataPath), err)
	}
	tmp := r.dataPath + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return fmt.Errorf("mount: write tmp: %w", err)
	}
	if err := os.Rename(tmp, r.dataPath); err != nil {
		// 跨设备 rename 失败（Save tmp 在 dataPath 同 dir，但若 r.dataPath 在不同
		// mount point 的 tmpdir 上仍可能 EXDEV）→ 兜底为 copy + remove
		if err := renameAcrossDevices(tmp, r.dataPath); err != nil {
			return fmt.Errorf("mount: rename: %w", err)
		}
	}
	return nil
}

// renameAcrossDevices 跨设备安全的 rename。
//
// 行为：
//   - 优先 os.Rename（原子，快）
//   - 跨设备（EXDEV / "invalid cross-device link"）→ fallback 到 copy + remove
//     （Android: /data/user/0/<pkg>/ 和 /storage/emulated/0/ 在不同分区，rename 失败；
//      dev 沙箱：host fs 跟 mount ns 也是不同设备）
//
// 原子性：单 rename 原子；copy+remove 不是原子的（copy 成功后 remove 失败 → 留 src 残骸）
// 副作用：remove 失败时会 print warning，不 return error（让主流程继续）
func renameAcrossDevices(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	} else if !isCrossDeviceError(err) {
		return err
	}
	// 跨设备 fallback：copy + remove
	if err := copyFileAndRemove(src, dst); err != nil {
		return fmt.Errorf("cross-device copy+remove %s -> %s: %w", src, dst, err)
	}
	return nil
}

// isCrossDeviceError 判断错误是否是 EXDEV（跨设备 rename 失败）。
func isCrossDeviceError(err error) bool {
	if err == nil {
		return false
	}
	// *os.LinkError 包裹的 syscall.Errno 或 errors.Is
	if errors.Is(err, syscall.EXDEV) {
		return true
	}
	// Linux 字符串（兜底，跨语言路径）
	if msg := err.Error(); msg != "" {
		if contains(msg, "invalid cross-device link") || contains(msg, "cross-device") {
			return true
		}
	}
	return false
}

// contains 简易 substring（避免引入 strings import 的小循环）
func contains(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// copyFileAndRemove 复制 src 到 dst，然后删除 src。
// 用于跨设备 rename 失败时的兜底。
func copyFileAndRemove(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open src: %w", err)
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("create dst: %w", err)
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(dst)
		return fmt.Errorf("copy: %w", err)
	}
	if err := out.Close(); err != nil {
		return fmt.Errorf("close dst: %w", err)
	}
	// copy 成功后删除 src（失败仅 log，不阻塞）
	if err := os.Remove(src); err != nil {
		fmt.Fprintf(os.Stderr, "[mount] warning: failed to remove src after copy %s: %v\n", src, err)
	}
	return nil
}
