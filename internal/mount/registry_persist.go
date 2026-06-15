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

const mountFileVersion = 1

// Load 从 dataPath 加载挂载点列表。
//
// 行为：
//   - 文件不存在：返回 nil（首次启动，由 Bootstrap 创建默认）
//   - 文件存在但解析失败：返回错误（让调用方决定是否回退 Bootstrap）
//   - 文件为空：返回空列表
//   - 成功：覆盖当前 registry 状态（不合并）
func (r *MountRegistry) Load() error {
	if r.dataPath == "" {
		return nil // 不持久化
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
	ctx := context.Background()
	r.mu.Lock()
	for _, m := range r.mounts {
		driver, err := r.instantiate(m.Driver)
		if err != nil {
			r.mu.Unlock()
			return fmt.Errorf("mount: load %s: instantiate %s: %w", m.Name, m.Driver, err)
		}
		if err := driver.Init(ctx, m, r.cfg); err != nil {
			// Init 失败不致命（dev sandbox 某些路径可能暂时不可用）
			// 只记日志，不返回错误
			fmt.Fprintf(os.Stderr, "[mount] load: init %s failed: %v\n", m.Name, err)
		}
	}
	r.mu.Unlock()
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
