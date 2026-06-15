package mount

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
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

// migrateLegacyDataPath 一次性处理老路径数据。
//
// 触发条件：老路径 serving_dir/mounts.json 存在
//
// 操作矩阵（2026-06-15 修复用户数据保留逻辑）：
//
//	老文件  新文件   操作
//	✓       ✗       老 → 新（原子 rename，新文件首次创建）
//	✓       ✓       atomic swap：老 → 新（用户的真实数据），新 → .migrated-<unix>（forensic 备份）
//	✗       *       no-op
//
// 2026-06-15 修复根因（双 bug）：
//  1. 原版"新文件存在就跳过" → 老文件永远残留在根目录，被 FileList 当普通文件列出
//  2. 启动流程不调 Load → 即使迁移成功，用户的 mount 数据也没恢复（被 Bootstrap 默认值覆盖）
// 现在：
//   - 老文件**总是**被处理（迁移或 swap）
//   - atomic swap 保证老文件的内容（用户真实数据）会变成新文件
//   - 之前的新文件 rename 到 .migrated-<unix> 作 forensic 备份
//   - 后续 Load 会从新文件恢复用户数据
//
// 安全性：
//   - 不删任何文件（rename only） → 用户数据可恢复
//   - 失败的中间状态：rename 失败返回 error，调用方 log 后继续（不阻塞启动）
func (r *MountRegistry) migrateLegacyDataPath() error {
	if r.dataPath == "" {
		return nil
	}
	// 老路径 = 新路径的父目录 + 老 basename
	// 例：dataPath = /workspace/.encv/mounts.json
	//     legacy = /workspace/mounts.json
	legacyPath := filepath.Join(filepath.Dir(filepath.Dir(r.dataPath)), legacyDataBasename)
	if legacyPath == r.dataPath {
		return nil // 同一路径（用户在隐藏子目录里手动放了文件）
	}
	// 老文件不存在 → no-op
	if _, err := os.Stat(legacyPath); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
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
		// 1. 新文件 rename 到 forensic 备份
		bakPath := fmt.Sprintf("%s.migrated-%d", r.dataPath, time.Now().Unix())
		if err := os.Rename(r.dataPath, bakPath); err != nil {
			return fmt.Errorf("rename new to backup %s: %w", bakPath, err)
		}
		// 2. 老文件 rename 到新文件位置（用户数据接管）
		if err := os.Rename(legacyPath, r.dataPath); err != nil {
			return fmt.Errorf("rename legacy %s -> %s: %w", legacyPath, r.dataPath, err)
		}
		fmt.Fprintf(os.Stderr, "[mount] legacy mounts.json swapped in (user data restored): %s -> %s, previous new file archived to %s\n",
			legacyPath, r.dataPath, bakPath)
	} else {
		// 新文件不存在 → 老 → 新原子 rename（一次性迁移）
		if err := os.Rename(legacyPath, r.dataPath); err != nil {
			return fmt.Errorf("rename %s -> %s: %w", legacyPath, r.dataPath, err)
		}
		fmt.Fprintf(os.Stderr, "[mount] migrated legacy mounts.json: %s -> %s\n", legacyPath, r.dataPath)
	}
	return nil
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
		return fmt.Errorf("mount: rename: %w", err)
	}
	return nil
}
