package kernel

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// ─── TaskStore 接口 ──────────────────────────────────────────────────────
//
// 任务记录存储。实现方：
//   - MemoryTaskStore：内存版（测试用 / 轻量场景）
//   - FileTaskStore：文件版（生产用，跨进程持久化）
//
// 设计原则：
//   - 写入是 append-only（创建+更新，不删除，除非显式清理）
//   - 查询支持多维度过滤（类型/服务/状态/时间/租户/触发者）
//   - 统计支持聚合（按类型/服务/租户分组）

type TaskStore interface {
	// Create 创建任务记录（ID 必须唯一）
	Create(record *TaskRecord) error

	// Update 更新任务记录（按 ID 全量替换，或部分更新）
	Update(id string, fn func(*TaskRecord)) error

	// Get 按 ID 获取
	Get(id string) (*TaskRecord, error)

	// List 按过滤条件查询
	List(filter TaskRecordFilter) ([]*TaskRecord, error)

	// Count 按过滤条件计数
	Count(filter TaskRecordFilter) (int64, error)

	// Stats 获取任务统计
	Stats(filter TaskRecordFilter) (*TaskStats, error)

	// Delete 删除指定任务（通常用不到，主要用于测试清理）
	Delete(id string) error

	// DeleteOlderThan 删除 N 天前的任务（定期清理，避免无限增长）
	DeleteOlderThan(days int) (int64, error)
}

// ─── MemoryTaskStore：内存版 ─────────────────────────────────────────────

type MemoryTaskStore struct {
	mu    sync.RWMutex
	items map[string]*TaskRecord // key: task ID
}

// NewMemoryTaskStore 创建内存版任务存储
func NewMemoryTaskStore() *MemoryTaskStore {
	return &MemoryTaskStore{items: make(map[string]*TaskRecord)}
}

func (m *MemoryTaskStore) Create(record *TaskRecord) error {
	if record == nil || record.ID == "" {
		return errors.New("kernel: task record has no ID")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.items[record.ID]; exists {
		return fmt.Errorf("kernel: task %q already exists", record.ID)
	}
	record.UpdatedAt = record.CreatedAt
	m.items[record.ID] = record
	return nil
}

func (m *MemoryTaskStore) Update(id string, fn func(*TaskRecord)) error {
	if id == "" {
		return errors.New("kernel: Update with empty ID")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	rec, exists := m.items[id]
	if !exists {
		return fmt.Errorf("kernel: task %q not found", id)
	}
	fn(rec)
	rec.UpdatedAt = time.Now()
	return nil
}

func (m *MemoryTaskStore) Get(id string) (*TaskRecord, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	rec, exists := m.items[id]
	if !exists {
		return nil, fmt.Errorf("kernel: task %q not found", id)
	}
	return rec, nil
}

func (m *MemoryTaskStore) List(filter TaskRecordFilter) ([]*TaskRecord, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// 收集所有匹配项
	var matched []*TaskRecord
	for _, r := range m.items {
		if !taskMatchesFilter(r, filter) {
			continue
		}
		matched = append(matched, r)
	}

	// 排序
	sort.Slice(matched, func(i, j int) bool {
		asc := filter.SortOrder == "asc"
		switch filter.SortBy {
		case "updatedAt":
			if asc {
				return matched[i].UpdatedAt.Before(matched[j].UpdatedAt)
			}
			return matched[i].UpdatedAt.After(matched[j].UpdatedAt)
		case "durationMs":
			if asc {
				return matched[i].DurationMs < matched[j].DurationMs
			}
			return matched[i].DurationMs > matched[j].DurationMs
		case "priority":
			if asc {
				return matched[i].Priority < matched[j].Priority
			}
			return matched[i].Priority > matched[j].Priority
		default: // createdAt
			if asc {
				return matched[i].CreatedAt.Before(matched[j].CreatedAt)
			}
			return matched[i].CreatedAt.After(matched[j].CreatedAt)
		}
	})

	// 分页
	limit := filter.Limit
	if limit <= 0 || limit > 500 {
		limit = 50 // 默认 50，最大 500
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}
	if offset >= len(matched) {
		return []*TaskRecord{}, nil
	}
	end := offset + limit
	if end > len(matched) {
		end = len(matched)
	}
	return matched[offset:end], nil
}

func (m *MemoryTaskStore) Count(filter TaskRecordFilter) (int64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var count int64
	for _, r := range m.items {
		if taskMatchesFilter(r, filter) {
			count++
		}
	}
	return count, nil
}

func (m *MemoryTaskStore) Stats(filter TaskRecordFilter) (*TaskStats, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	stats := &TaskStats{
		ByType:    make(map[TaskType]int64),
		ByService: make(map[string]int64),
		ByTenant:  make(map[string]int64),
	}
	var durations []int64

	for _, r := range m.items {
		if !taskMatchesFilter(r, filter) {
			continue
		}
		stats.Total++
		switch r.Status {
		case TaskStatusPending:
			stats.Pending++
		case TaskStatusRunning:
			stats.Running++
		case TaskStatusSuccess:
			stats.Success++
		case TaskStatusFailed:
			stats.Failed++
		case TaskStatusCancelled:
			stats.Cancelled++
		}
		if r.DurationMs > 0 {
			durations = append(durations, r.DurationMs)
		}
		stats.ByType[r.Type]++
		stats.ByService[r.Service]++
		if r.TenantID != "" {
			stats.ByTenant[r.TenantID]++
		}
	}

	// 计算耗时统计
	if len(durations) > 0 {
		sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
		var sum int64
		for _, d := range durations {
			sum += d
		}
		stats.AvgDurationMs = sum / int64(len(durations))
		stats.MaxDurationMs = durations[len(durations)-1]
		p95Idx := int(float64(len(durations)) * 0.95)
		if p95Idx >= len(durations) {
			p95Idx = len(durations) - 1
		}
		if p95Idx >= 0 {
			stats.P95DurationMs = durations[p95Idx]
		}
	}

	return stats, nil
}

func (m *MemoryTaskStore) Delete(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.items, id)
	return nil
}

func (m *MemoryTaskStore) DeleteOlderThan(days int) (int64, error) {
	if days <= 0 {
		return 0, errors.New("kernel: days must be positive")
	}
	cutoff := time.Now().AddDate(0, 0, -days)
	m.mu.Lock()
	defer m.mu.Unlock()
	var deleted int64
	for id, r := range m.items {
		if r.CreatedAt.Before(cutoff) {
			delete(m.items, id)
			deleted++
		}
	}
	return deleted, nil
}

// Snapshot 返回所有记录（测试用）
func (m *MemoryTaskStore) Snapshot() map[string]*TaskRecord {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make(map[string]*TaskRecord, len(m.items))
	for k, v := range m.items {
		out[k] = v
	}
	return out
}

// ─── FileTaskStore：文件版 ───────────────────────────────────────────────
//
// 目录结构：
//
//	<root>/
//	  by_id/
//	    <id>.json        ← 单条任务记录（写多读少，每条一个文件）
//	  index/
//	    by_date/         ← 按日期分桶（加速按时间范围查询）
//	      2026-07-03/
//	        <id>.json
//
// 写入：先写 by_id 主文件，再更新日期索引
// 查询：先看能不能走日期索引，不行就遍历 by_id
//
// 设计权衡：
//   - 用单文件 per-task 而不是单大文件，避免单文件锁竞争
//   - 日期分桶索引加速按时间范围查询
//   - 其他维度过滤只能遍历，但任务量通常不大（几千条级别）

type FileTaskStore struct {
	root string
	mu   sync.RWMutex
}

// NewFileTaskStore 创建文件版任务存储
func NewFileTaskStore(root string) (*FileTaskStore, error) {
	if err := os.MkdirAll(filepath.Join(root, "by_id"), 0755); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Join(root, "index", "by_date"), 0755); err != nil {
		return nil, err
	}
	return &FileTaskStore{root: root}, nil
}

func (f *FileTaskStore) idPath(id string) string {
	return filepath.Join(f.root, "by_id", sanitizeID(id)+".json")
}

func (f *FileTaskStore) dateBucket(t time.Time) string {
	return t.Format("2006-01-02")
}

func (f *FileTaskStore) dateIndexPath(date string) string {
	return filepath.Join(f.root, "index", "by_date", date)
}

func (f *FileTaskStore) Create(record *TaskRecord) error {
	if record == nil || record.ID == "" {
		return errors.New("kernel: task record has no ID")
	}
	f.mu.Lock()
	defer f.mu.Unlock()

	record.UpdatedAt = record.CreatedAt

	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return fmt.Errorf("kernel: marshal task: %w", err)
	}

	// 写主文件
	idPath := f.idPath(record.ID)
	if _, err := os.Stat(idPath); err == nil {
		return fmt.Errorf("kernel: task %q already exists", record.ID)
	}
	tmpPath := idPath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, idPath); err != nil {
		return err
	}

	// 更新日期索引
	dateDir := f.dateIndexPath(f.dateBucket(record.CreatedAt))
	if err := os.MkdirAll(dateDir, 0755); err != nil {
		return err
	}
	linkPath := filepath.Join(dateDir, sanitizeID(record.ID)+".json")
	// 用硬链接（同文件系统）或复制
	if err := os.Link(idPath, linkPath); err != nil {
		// 硬链接失败就复制
		_ = os.WriteFile(linkPath, data, 0644)
	}

	return nil
}

func (f *FileTaskStore) Update(id string, fn func(*TaskRecord)) error {
	if id == "" {
		return errors.New("kernel: Update with empty ID")
	}
	f.mu.Lock()
	defer f.mu.Unlock()

	// 读取
	idPath := f.idPath(id)
	data, err := os.ReadFile(idPath)
	if err != nil {
		return fmt.Errorf("kernel: task %q not found: %w", id, err)
	}
	var rec TaskRecord
	if err := json.Unmarshal(data, &rec); err != nil {
		return fmt.Errorf("kernel: unmarshal task %q: %w", id, err)
	}

	// 修改
	fn(&rec)
	rec.UpdatedAt = time.Now()

	// 写回
	out, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return err
	}
	tmpPath := idPath + ".tmp"
	if err := os.WriteFile(tmpPath, out, 0644); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, idPath); err != nil {
		return err
	}

	// 更新日期索引（同一份数据，重新写）
	dateDir := f.dateIndexPath(f.dateBucket(rec.CreatedAt))
	linkPath := filepath.Join(dateDir, sanitizeID(id)+".json")
	_ = os.WriteFile(linkPath, out, 0644)

	return nil
}

func (f *FileTaskStore) Get(id string) (*TaskRecord, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	data, err := os.ReadFile(f.idPath(id))
	if err != nil {
		return nil, fmt.Errorf("kernel: task %q not found: %w", id, err)
	}
	var rec TaskRecord
	if err := json.Unmarshal(data, &rec); err != nil {
		return nil, fmt.Errorf("kernel: unmarshal task %q: %w", id, err)
	}
	return &rec, nil
}

func (f *FileTaskStore) List(filter TaskRecordFilter) ([]*TaskRecord, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	// 确定要扫描的 ID 列表
	var ids []string

	// 优化：如果有日期过滤，只扫描对应日期桶
	if filter.CreatedAfter != nil || filter.CreatedBefore != nil {
		ids = f.scanDateIndex(filter)
	} else {
		// 全量扫描 by_id
		entries, err := os.ReadDir(filepath.Join(f.root, "by_id"))
		if err != nil {
			if os.IsNotExist(err) {
				return []*TaskRecord{}, nil
			}
			return nil, err
		}
		for _, e := range entries {
			if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
				continue
			}
			name := e.Name()
			id := name[:len(name)-len(".json")]
			ids = append(ids, id)
		}
	}

	// 加载并过滤
	var matched []*TaskRecord
	for _, id := range ids {
		data, err := os.ReadFile(f.idPath(id))
		if err != nil {
			continue
		}
		var r TaskRecord
		if err := json.Unmarshal(data, &r); err != nil {
			continue
		}
		if taskMatchesFilter(&r, filter) {
			matched = append(matched, &r)
		}
	}

	// 排序 + 分页（复用内存版的逻辑）
	sort.Slice(matched, func(i, j int) bool {
		asc := filter.SortOrder == "asc"
		switch filter.SortBy {
		case "updatedAt":
			if asc {
				return matched[i].UpdatedAt.Before(matched[j].UpdatedAt)
			}
			return matched[i].UpdatedAt.After(matched[j].UpdatedAt)
		case "durationMs":
			if asc {
				return matched[i].DurationMs < matched[j].DurationMs
			}
			return matched[i].DurationMs > matched[j].DurationMs
		default: // createdAt
			if asc {
				return matched[i].CreatedAt.Before(matched[j].CreatedAt)
			}
			return matched[i].CreatedAt.After(matched[j].CreatedAt)
		}
	})

	limit := filter.Limit
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}
	if offset >= len(matched) {
		return []*TaskRecord{}, nil
	}
	end := offset + limit
	if end > len(matched) {
		end = len(matched)
	}
	return matched[offset:end], nil
}

// scanDateIndex 扫描日期索引，返回候选 ID 列表
func (f *FileTaskStore) scanDateIndex(filter TaskRecordFilter) []string {
	dateIndexRoot := filepath.Join(f.root, "index", "by_date")
	entries, err := os.ReadDir(dateIndexRoot)
	if err != nil {
		return nil
	}

	var ids []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dateStr := e.Name()
		// 解析日期目录名
		d, err := time.Parse("2006-01-02", dateStr)
		if err != nil {
			continue
		}
		// 日期范围过滤
		if filter.CreatedAfter != nil && d.AddDate(0, 0, 1).Before(*filter.CreatedAfter) {
			continue
		}
		if filter.CreatedBefore != nil && d.After(*filter.CreatedBefore) {
			continue
		}
		// 收集该日期桶的所有 ID
		dir := filepath.Join(dateIndexRoot, dateStr)
		files, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, f2 := range files {
			if f2.IsDir() || filepath.Ext(f2.Name()) != ".json" {
				continue
			}
			name := f2.Name()
			id := name[:len(name)-len(".json")]
			ids = append(ids, id)
		}
	}
	return ids
}

func (f *FileTaskStore) Count(filter TaskRecordFilter) (int64, error) {
	records, err := f.List(TaskRecordFilter{
		Types:         filter.Types,
		Services:      filter.Services,
		Statuses:      filter.Statuses,
		TenantID:      filter.TenantID,
		TriggeredBy:   filter.TriggeredBy,
		RunID:         filter.RunID,
		CreatedAfter:  filter.CreatedAfter,
		CreatedBefore: filter.CreatedBefore,
		Limit:         50000, // 上限，避免内存爆炸
	})
	if err != nil {
		return 0, err
	}
	return int64(len(records)), nil
}

func (f *FileTaskStore) Stats(filter TaskRecordFilter) (*TaskStats, error) {
	records, err := f.List(TaskRecordFilter{
		Types:         filter.Types,
		Services:      filter.Services,
		Statuses:      filter.Statuses,
		TenantID:      filter.TenantID,
		TriggeredBy:   filter.TriggeredBy,
		RunID:         filter.RunID,
		CreatedAfter:  filter.CreatedAfter,
		CreatedBefore: filter.CreatedBefore,
		Limit:         50000,
	})
	if err != nil {
		return nil, err
	}

	stats := &TaskStats{
		ByType:    make(map[TaskType]int64),
		ByService: make(map[string]int64),
		ByTenant:  make(map[string]int64),
	}
	var durations []int64

	for _, r := range records {
		stats.Total++
		switch r.Status {
		case TaskStatusPending:
			stats.Pending++
		case TaskStatusRunning:
			stats.Running++
		case TaskStatusSuccess:
			stats.Success++
		case TaskStatusFailed:
			stats.Failed++
		case TaskStatusCancelled:
			stats.Cancelled++
		}
		if r.DurationMs > 0 {
			durations = append(durations, r.DurationMs)
		}
		stats.ByType[r.Type]++
		stats.ByService[r.Service]++
		if r.TenantID != "" {
			stats.ByTenant[r.TenantID]++
		}
	}

	if len(durations) > 0 {
		sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
		var sum int64
		for _, d := range durations {
			sum += d
		}
		stats.AvgDurationMs = sum / int64(len(durations))
		stats.MaxDurationMs = durations[len(durations)-1]
		p95Idx := int(float64(len(durations)) * 0.95)
		if p95Idx >= len(durations) {
			p95Idx = len(durations) - 1
		}
		if p95Idx >= 0 {
			stats.P95DurationMs = durations[p95Idx]
		}
	}

	return stats, nil
}

func (f *FileTaskStore) Delete(id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	idPath := f.idPath(id)
	// 先读一下拿到创建时间，删日期索引
	data, err := os.ReadFile(idPath)
	if err == nil {
		var rec TaskRecord
		if json.Unmarshal(data, &rec) == nil {
			dateDir := f.dateIndexPath(f.dateBucket(rec.CreatedAt))
			linkPath := filepath.Join(dateDir, sanitizeID(id)+".json")
			_ = os.Remove(linkPath)
		}
	}
	return os.Remove(idPath)
}

func (f *FileTaskStore) DeleteOlderThan(days int) (int64, error) {
	if days <= 0 {
		return 0, errors.New("kernel: days must be positive")
	}
	cutoff := time.Now().AddDate(0, 0, -days)
	f.mu.Lock()
	defer f.mu.Unlock()

	var deleted int64

	// 遍历日期索引，直接删整个桶（高效）
	dateIndexRoot := filepath.Join(f.root, "index", "by_date")
	entries, err := os.ReadDir(dateIndexRoot)
	if err == nil {
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			d, err := time.Parse("2006-01-02", e.Name())
			if err != nil {
				continue
			}
			if d.Before(cutoff) {
				// 先数一下多少条
				dir := filepath.Join(dateIndexRoot, e.Name())
				files, _ := os.ReadDir(dir)
				// 同步删 by_id 里的
				for _, f2 := range files {
					if filepath.Ext(f2.Name()) == ".json" {
						name := f2.Name()
						id := name[:len(name)-len(".json")]
						_ = os.Remove(f.idPath(id))
						deleted++
					}
				}
				// 删整个日期桶
				_ = os.RemoveAll(dir)
			}
		}
	}

	return deleted, nil
}

// ─── 公共过滤逻辑 ────────────────────────────────────────────────────────

func taskMatchesFilter(r *TaskRecord, f TaskRecordFilter) bool {
	// 类型
	if len(f.Types) > 0 {
		found := false
		for _, t := range f.Types {
			if r.Type == t {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	// 服务
	if len(f.Services) > 0 {
		found := false
		for _, s := range f.Services {
			if r.Service == s {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	// 状态
	if len(f.Statuses) > 0 {
		found := false
		for _, s := range f.Statuses {
			if r.Status == s {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	// 租户
	if f.TenantID != "" && r.TenantID != f.TenantID {
		return false
	}
	// 触发者
	if f.TriggeredBy != "" && r.TriggeredBy != f.TriggeredBy {
		return false
	}
	// RunID
	if f.RunID != "" && r.RunID != f.RunID {
		return false
	}
	// 时间范围
	if f.CreatedAfter != nil && r.CreatedAt.Before(*f.CreatedAfter) {
		return false
	}
	if f.CreatedBefore != nil && r.CreatedAt.After(*f.CreatedBefore) {
		return false
	}
	return true
}
