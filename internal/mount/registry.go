package mount

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// MountRegistry 是挂载点注册表：内存态 + 线程安全。
//
// 路径解析策略：
//   - 虚拟路径 /d/<mount_path>/<sub_path>
//   - Registry 维护一个 mount 列表，按 MountPath 长度倒序排列
//   - Resolve 时用 longest-prefix 匹配找到最具体的 mount
//   - O(M) where M = mount 数量（实际 < 10；可以接受）
//
// Driver 注入：
//   - 构造时通过 RegisterDriverFactory(name, factory) 注入
//   - factory 返回 Driver 实例（每次新建以避免状态污染）
//   - 这样 mount 包不依赖 drivers 子包，避免 import cycle
//
// 后续可优化：用 radix tree 做 O(log M) 查询（spec §6 性能目标）
type MountRegistry struct {
	mu       sync.RWMutex
	mounts   []*Mount                  // 已排序：按 MountPath 长度倒序
	byID     map[string]*Mount
	byName   map[string]*Mount
	drivers  map[string]DriverFactory  // driver name → factory
	cfg      ConfigProvider
	dataPath string // mounts.json 路径；空表示不持久化
	clock    func() time.Time
	uuidFn   func() string
}

// NewRegistry 构造一个空 registry。**不会**自动注册任何 driver。
// 调用方必须显式 RegisterDriverFactory 才能让 Create/Update 工作。
func NewRegistry(cfg ConfigProvider, dataPath string) *MountRegistry {
	return &MountRegistry{
		byID:     make(map[string]*Mount),
		byName:   make(map[string]*Mount),
		drivers:  make(map[string]DriverFactory),
		cfg:      cfg,
		dataPath: dataPath,
		clock:    time.Now,
		uuidFn:   func() string { return uuid.New().String() },
	}
}

// RegisterDriverFactory 注册 driver 工厂。
func (r *MountRegistry) RegisterDriverFactory(name string, factory DriverFactory) {
	r.drivers[name] = factory
}

// ListDrivers 返回所有已注册的 driver 名称。
func (r *MountRegistry) ListDrivers() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.drivers))
	for k := range r.drivers {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// instantiate 通过 factory 创建 driver 实例。每次新建避免状态污染。
func (r *MountRegistry) instantiate(name string) (Driver, error) {
	f, ok := r.drivers[name]
	if !ok {
		return nil, fmt.Errorf("mount: unknown driver %q (registered: %v)", name, r.ListDrivers())
	}
	return f(), nil
}

// add（内部）添加一个 Mount 到 registry，触发 driver.Init。
func (r *MountRegistry) add(m *Mount) error {
	driver, err := r.instantiate(m.Driver)
	if err != nil {
		return err
	}
	if err := driver.Init(context.Background(), m, r.cfg); err != nil {
		return fmt.Errorf("init driver: %w", err)
	}
	if m.ID == "" {
		m.ID = r.uuidFn()
	}
	now := r.clock()
	if m.CreatedAt.IsZero() {
		m.CreatedAt = now
	}
	m.UpdatedAt = now
	r.mu.Lock()
	defer r.mu.Unlock()
	r.mounts = append(r.mounts, m)
	r.byID[m.ID] = m
	r.byName[m.Name] = m
	sort.SliceStable(r.mounts, func(i, j int) bool {
		return len(r.mounts[i].MountPath) > len(r.mounts[j].MountPath)
	})
	return nil
}

// List 返回所有挂载点（按 MountPath 长度倒序）。线程安全。
func (r *MountRegistry) List() []*Mount {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*Mount, len(r.mounts))
	copy(out, r.mounts)
	return out
}

// GetByID 按 ID 查找。
func (r *MountRegistry) GetByID(id string) *Mount {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.byID[id]
}

// GetByName 按 name 查找。
func (r *MountRegistry) GetByName(name string) *Mount {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.byName[name]
}

// GetByMountPath 按虚拟挂载路径查找。
func (r *MountRegistry) GetByMountPath(mp string) *Mount {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, m := range r.mounts {
		if m.MountPath == mp {
			return m
		}
	}
	return nil
}

// Create 添加新挂载点。校验：name / mount_path 唯一 + 字段合法。
func (r *MountRegistry) Create(m *Mount) error {
	if m == nil {
		return fmt.Errorf("mount: nil")
	}
	if err := m.Validate(); err != nil {
		return err
	}
	r.mu.RLock()
	if _, exists := r.byName[m.Name]; exists {
		r.mu.RUnlock()
		return ErrNameExists
	}
	for _, exist := range r.mounts {
		if exist.MountPath == m.MountPath {
			r.mu.RUnlock()
			return ErrMountPathExists
		}
	}
	r.mu.RUnlock()
	return r.add(m)
}

// Update 修改挂载点字段。保护 primary 不可改名/改 mount_path。
func (r *MountRegistry) Update(m *Mount) error {
	if m == nil || m.ID == "" {
		return fmt.Errorf("mount: ID required")
	}
	r.mu.RLock()
	old, exists := r.byID[m.ID]
	r.mu.RUnlock()
	if !exists {
		return ErrNotFound
	}
	if old.Name == NamePrimary && (m.Name != old.Name || m.MountPath != old.MountPath || m.Driver != old.Driver) {
		return ErrPrimaryProtected
	}
	if err := m.Validate(); err != nil {
		return err
	}
	r.mu.RLock()
	if m.Name != old.Name {
		if _, exists := r.byName[m.Name]; exists {
			r.mu.RUnlock()
			return ErrNameExists
		}
	}
	for _, exist := range r.mounts {
		if exist.ID == m.ID {
			continue
		}
		if exist.MountPath == m.MountPath {
			r.mu.RUnlock()
			return ErrMountPathExists
		}
	}
	r.mu.RUnlock()

	// 如果 driver 变了，验证新 driver
	if m.Driver != old.Driver {
		if _, err := r.instantiate(m.Driver); err != nil {
			return err
		}
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	// 更新索引
	delete(r.byName, old.Name)
	old.Name = m.Name
	old.MountPath = m.MountPath
	old.Driver = m.Driver
	old.Enabled = m.Enabled
	old.ReadOnly = m.ReadOnly
	old.DriverConfig = m.DriverConfig
	old.UpdatedAt = r.clock()
	r.byName[old.Name] = old
	sort.SliceStable(r.mounts, func(i, j int) bool {
		return len(r.mounts[i].MountPath) > len(r.mounts[j].MountPath)
	})
	// 如果 RootPath 是空，重新 init
	if old.RootPath == "" {
		d, _ := r.instantiate(old.Driver)
		_ = d.Init(context.Background(), old, r.cfg)
	}
	return nil
}

// Delete 删除挂载点。primary 不可删。
func (r *MountRegistry) Delete(id string) error {
	r.mu.RLock()
	m, exists := r.byID[id]
	r.mu.RUnlock()
	if !exists {
		return ErrNotFound
	}
	if m.Name == NamePrimary {
		return ErrPrimaryProtected
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, mm := range r.mounts {
		if mm.ID == id {
			r.mounts = append(r.mounts[:i], r.mounts[i+1:]...)
			break
		}
	}
	delete(r.byID, id)
	delete(r.byName, m.Name)
	return nil
}

// ResolveResult 是 Resolve 的返回值。
type ResolveResult struct {
	Mount   *Mount
	AbsPath string // 物理绝对路径
	RelPath string // 相对 Mount.RootPath 的路径
}

// Resolve 把虚拟路径解析为 (mount, abs_path)。
//
// 规则：
//   - /d/<mount_path>/<sub>  →  longest prefix 匹配
//   - /<sub>                  →  fallback 到 primary mount（兼容旧 API）
//   - 空 / 不以 / 开头        →  ErrInvalidPath
//
// 安全：
//   - 解析后的 abs path 必须仍在 mount.RootPath 内（防 ../ 逃逸）
//   - mount disabled 时返回 ErrDisabled
func (r *MountRegistry) Resolve(virtualPath string) (*ResolveResult, error) {
	if virtualPath == "" {
		return nil, ErrInvalidPath
	}
	if !strings.HasPrefix(virtualPath, "/") {
		return nil, ErrInvalidPath
	}
	r.mu.RLock()
	defer r.mu.RUnlock()

	// 1. longest prefix 匹配
	var match *Mount
	var sub string
	for _, m := range r.mounts {
		if m.MountPath == "/" {
			// primary 兼容：所有路径默认 primary
			if match == nil {
				match = m
				sub = strings.TrimPrefix(virtualPath, "/")
			}
			continue
		}
		if strings.HasPrefix(virtualPath, m.MountPath+"/") || virtualPath == m.MountPath {
			match = m
			if virtualPath == m.MountPath {
				sub = ""
			} else {
				sub = strings.TrimPrefix(virtualPath, m.MountPath)
				sub = strings.TrimPrefix(sub, "/")
			}
			break
		}
	}

	if match == nil {
		return nil, fmt.Errorf("mount: no mount matches %q", virtualPath)
	}
	if !match.Enabled {
		return nil, fmt.Errorf("%w: %s", ErrDisabled, match.Name)
	}

	// 2. 计算 abs path 并校验
	abs := filepath.Join(match.RootPath, filepath.Clean("/"+sub))
	rel, err := filepath.Rel(match.RootPath, abs)
	if err != nil {
		return nil, fmt.Errorf("mount: escape check: %w", err)
	}
	if strings.HasPrefix(rel, "..") {
		return nil, fmt.Errorf("mount: path escapes root: %q", virtualPath)
	}

	return &ResolveResult{Mount: match, AbsPath: abs, RelPath: rel}, nil
}

// AbsToVirtual 把物理绝对路径反向解析为虚拟路径 /d/<mount_path>/<sub_path>。
//
// 🆕 v3 2026-06-18 Task 8：后端统一虚拟路径
//   - task.OutputPath / step.Detail 存虚拟路径，前端 Files.vue 直接消费
//   - 找不到匹配 mount → 返回 ("", ErrNoMount) — 调用方 fallback 到原 absPath
//
// 规则：
//   - 遍历所有 mount，找 RootPath 是 absPath 的最长前缀的 mount
//   - 校验 absPath 仍在 mount.RootPath 内（防 ../ 逃逸）
//   - disabled mount 仍可匹配（产物路径回显不应因 mount 被禁用而丢失）
func (r *MountRegistry) AbsToVirtual(absPath string) (string, error) {
	if absPath == "" {
		return "", ErrInvalidPath
	}
	if !filepath.IsAbs(absPath) {
		return "", fmt.Errorf("mount: AbsToVirtual requires absolute path, got %q", absPath)
	}
	r.mu.RLock()
	defer r.mu.RUnlock()

	// clean 一次，确保后续前缀比较稳定
	absClean := filepath.Clean(absPath)

	var match *Mount
	var matchRel string
	matchLen := -1
	for _, m := range r.mounts {
		root := filepath.Clean(m.RootPath)
		if root == "" {
			continue
		}
		// 必须是 root 本身或 root/... 的子路径
		var rel string
		if absClean == root {
			rel = ""
		} else if strings.HasPrefix(absClean, root+string(filepath.Separator)) {
			rel = strings.TrimPrefix(absClean, root+string(filepath.Separator))
		} else {
			continue
		}
		// 选 RootPath 最长（最具体）的 mount
		// 注意：rel 必须与 match 同步更新，避免被后续非最优 mount 覆盖
		if len(root) > matchLen {
			match = m
			matchRel = rel
			matchLen = len(root)
		}
	}

	if match == nil {
		return "", fmt.Errorf("mount: no mount matches abs path %q", absPath)
	}

	// 构造虚拟路径：match.MountPath + "/" + matchRel
	// 注意：MountPath 形如 /d/primary，matchRel 是相对子路径（可能为空）
	if matchRel == "" {
		return match.MountPath, nil
	}
	// MountPath 已是 /d/<name> 形式，无需再加 /d 前缀
	virtual := match.MountPath + "/" + matchRel
	// clean 一下防止 // 等
	virtual = filepath.Clean(virtual)
	return virtual, nil
}
