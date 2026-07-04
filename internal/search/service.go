package vectorsearch

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"sync"
)

// SearchService 向量搜索服务。
//
// 提供文件和任务的语义搜索能力，基于 Turso 原生向量检索。
//
// 降级机制（L2 级，见 graceful-degradation.md）：
//   - turso/libsql 驱动首选 TursoStore（用原生 vector_distance_cos）
//   - 若运行时实测 vector_distance_cos 不可用（极少数自定义编译），
//     Search 第一次出错时自动切换到 SQLiteStore（Go 层计算）
//   - 降级后通过 degraded=true 暴露状态，调用方可查询
type SearchService struct {
	store        Store
	fallbackStore Store // 仅 turso/libsql 时非 nil（SQLiteStore 实例）
	degraded     bool   // 是否已降级到 fallbackStore
	db           *sql.DB
	mu           sync.RWMutex
	indexedFiles int
	indexedTasks int
}

// NewSearchService 创建搜索服务（传入已有的 *sql.DB）。
func NewSearchService(db *sql.DB, driver string) (*SearchService, error) {
	store, err := NewStore(db, driver)
	if err != nil {
		return nil, fmt.Errorf("create search store: %w", err)
	}

	svc := &SearchService{store: store, db: db}

	// turso/libsql 准备 fallbackStore（SQLiteStore），运行时降级用
	if driver == "turso" || driver == "libsql" {
		svc.fallbackStore = &SQLiteStore{db: db}
	}

	ctx := context.Background()
	if err := store.Init(ctx); err != nil {
		return nil, fmt.Errorf("init search store: %w", err)
	}
	// fallbackStore 共享同一 db，schema 相同，无需重复 Init

	slog.Info("[vectorsearch] service initialized", "driver", driver)
	return svc, nil
}

// NewSearchServiceFromPath 从数据库文件路径创建搜索服务。
func NewSearchServiceFromPath(dbPath, driver string) (*SearchService, error) {
	var dsn string
	switch driver {
	case "turso", "libsql":
		dsn = dbPath
	case "sqlite":
		dsn = fmt.Sprintf(
			"file:%s?_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=busy_timeout(30000)",
			dbPath,
		)
	default:
		return nil, fmt.Errorf("unsupported driver: %s", driver)
	}

	db, err := sql.Open(driver, dsn)
	if err != nil {
		return nil, fmt.Errorf("open db for search: %w", err)
	}

	if driver == "turso" || driver == "libsql" {
		db.SetMaxOpenConns(8)
		db.SetMaxIdleConns(4)
		// Turso pragma 设置
		pragmas := []string{
			"PRAGMA journal_mode=WAL",
			"PRAGMA synchronous=NORMAL",
			"PRAGMA busy_timeout=30000",
		}
		for _, p := range pragmas {
			db.Exec(p)
		}
	} else {
		db.SetMaxOpenConns(1)
	}

	return NewSearchService(db, driver)
}

// IndexFile 索引单个文件。
func (s *SearchService) IndexFile(ctx context.Context, path, name, size, mtime string) error {
	content := fmt.Sprintf("%s %s", name, path)
	item := &IndexItem{
		RefID:     path,
		Title:     name,
		Content:   content,
		Extra:     fmt.Sprintf(`{"size":"%s","mtime":"%s"}`, size, mtime),
		IndexType: IndexTypeFile,
	}
	return s.store.Upsert(ctx, item)
}

// IndexFilesBatch 批量索引文件。
func (s *SearchService) IndexFilesBatch(ctx context.Context, items []FileIndexItem) error {
	if len(items) == 0 {
		return nil
	}

	indexItems := make([]*IndexItem, len(items))
	for i, f := range items {
		content := fmt.Sprintf("%s %s", f.Name, f.Path)
		indexItems[i] = &IndexItem{
			RefID:     f.Path,
			Title:     f.Name,
			Content:   content,
			Extra:     fmt.Sprintf(`{"size":%d,"mtime":"%s"}`, f.Size, f.MTime),
			IndexType: IndexTypeFile,
		}
	}

	if err := s.store.UpsertBatch(ctx, indexItems); err != nil {
		return err
	}

	s.mu.Lock()
	s.indexedFiles += len(items)
	s.mu.Unlock()
	return nil
}

// FileIndexItem 文件索引项。
type FileIndexItem struct {
	Path  string
	Name  string
	Size  int64
	MTime string
}

// IndexTask 索引单个任务。
func (s *SearchService) IndexTask(ctx context.Context, taskID, name, taskType, sourcePath, status string) error {
	content := fmt.Sprintf("%s %s %s %s", name, taskType, sourcePath, status)
	item := &IndexItem{
		RefID:     taskID,
		Title:     name,
		Content:   content,
		Extra:     fmt.Sprintf(`{"type":"%s","source_path":"%s","status":"%s"}`, taskType, sourcePath, status),
		IndexType: IndexTypeTask,
	}
	return s.store.Upsert(ctx, item)
}

// IndexTasksBatch 批量索引任务。
func (s *SearchService) IndexTasksBatch(ctx context.Context, items []TaskIndexItem) error {
	if len(items) == 0 {
		return nil
	}

	indexItems := make([]*IndexItem, len(items))
	for i, t := range items {
		content := fmt.Sprintf("%s %s %s %s", t.Name, t.TaskType, t.SourcePath, t.Status)
		indexItems[i] = &IndexItem{
			RefID:     t.ID,
			Title:     t.Name,
			Content:   content,
			Extra:     fmt.Sprintf(`{"type":"%s","source_path":"%s","status":"%s"}`, t.TaskType, t.SourcePath, t.Status),
			IndexType: IndexTypeTask,
		}
	}

	if err := s.store.UpsertBatch(ctx, indexItems); err != nil {
		return err
	}

	s.mu.Lock()
	s.indexedTasks += len(items)
	s.mu.Unlock()
	return nil
}

// TaskIndexItem 任务索引项。
type TaskIndexItem struct {
	ID         string
	Name       string
	TaskType   string
	SourcePath string
	Status     string
}

// SearchFiles 搜索文件。
func (s *SearchService) SearchFiles(ctx context.Context, query string, limit int) ([]SearchResult, error) {
	if limit <= 0 {
		limit = 50
	}
	return s.searchWithFallback(ctx, IndexTypeFile, query, limit)
}

// SearchTasks 搜索任务。
func (s *SearchService) SearchTasks(ctx context.Context, query string, limit int) ([]SearchResult, error) {
	if limit <= 0 {
		limit = 50
	}
	return s.searchWithFallback(ctx, IndexTypeTask, query, limit)
}

// searchWithFallback 执行搜索，若主 store 出错且 fallbackStore 可用则降级。
//
// 降级场景：turso/libsql 的 .so 编译时未启用向量支持 → vector_distance_cos 报错。
// 降级是 L2 级体验（见 graceful-degradation.md）：
//   - 第一次出错时 WARN 日志记录原因
//   - 后续请求直接走 fallbackStore（避免每次都试错）
//   - IsDegraded() 暴露状态给调用方
func (s *SearchService) searchWithFallback(ctx context.Context, indexType IndexType, query string, limit int) ([]SearchResult, error) {
	s.mu.RLock()
	degraded := s.degraded
	store := s.store
	fallback := s.fallbackStore
	s.mu.RUnlock()

	if degraded && fallback != nil {
		return fallback.Search(ctx, indexType, query, limit)
	}

	results, err := store.Search(ctx, indexType, query, limit)
	if err == nil {
		return results, nil
	}

	// 主 store 出错，尝试降级
	if fallback == nil {
		return nil, err
	}

	slog.Warn("[vectorsearch] primary store search failed, degrading to SQLiteStore",
		"error", err, "indexType", indexType, "query", query)

	s.mu.Lock()
	if !s.degraded {
		s.degraded = true
	}
	s.mu.Unlock()

	return fallback.Search(ctx, indexType, query, limit)
}

// IsDegraded 返回是否已降级到 fallbackStore。
// 调用方（如 /api/runtime）应暴露此状态，让用户知道向量搜索走了降级路径。
func (s *SearchService) IsDegraded() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.degraded
}

// DeleteTask 从索引中删除任务。
func (s *SearchService) DeleteTask(ctx context.Context, taskID string) error {
	return s.store.Delete(ctx, IndexTypeTask, taskID)
}

// DeleteFile 从索引中删除文件。
func (s *SearchService) DeleteFile(ctx context.Context, path string) error {
	return s.store.Delete(ctx, IndexTypeFile, path)
}

// ClearFileIndex 清空文件索引。
func (s *SearchService) ClearFileIndex(ctx context.Context) error {
	s.mu.Lock()
	s.indexedFiles = 0
	s.mu.Unlock()
	return s.store.Clear(ctx, IndexTypeFile)
}

// ClearTaskIndex 清空任务索引。
func (s *SearchService) ClearTaskIndex(ctx context.Context) error {
	s.mu.Lock()
	s.indexedTasks = 0
	s.mu.Unlock()
	return s.store.Clear(ctx, IndexTypeTask)
}

// Stats 返回索引统计。
func (s *SearchService) Stats() map[string]int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return map[string]int{
		"files": s.indexedFiles,
		"tasks": s.indexedTasks,
	}
}
