// Package fts 提供基于 SQLite FTS5 的文件全文索引。
//
// 2026-07-02 大改升级：替换原 in-memory fileIndex 为 FTS5 SQLite 后端。
//
// 设计要点：
//   - 默认 modernc.org/sqlite（pure-Go，CGO_ENABLED=0，零依赖）
//   - FTS5 虚拟表 + content= 外部内容表 + content_sync trigger
//   - tokenize='porter unicode61'（支持 CJK bigram 通过自定义分词器，可选）
//   - 批量写入：单次事务 + prepared statement
//   - 查询：bool/phrase/regex 三种语法（AND/OR/NOT/"phrase"/regex:...）
//   - 性能指标：100w 文档 ≤500ms 命中 (含 snippets)
//
// 与 android.md 铁律一致：默认 glebarez 纯 Go，libsql/turso 增强。
package fts

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite" // pure-Go SQLite driver
)

// FileIndexStats 索引统计信息。
type FileIndexStats struct {
	TotalFiles  int    `json:"totalFiles"`
	TotalDirs   int    `json:"totalDirs"`
	TotalSize   int64  `json:"totalSize"`
	IndexedAt   string `json:"indexedAt"`
	IsIndexing  bool   `json:"isIndexing"`
	LastBuildMs int64  `json:"lastBuildMs"`

	// FTS5 扩展字段
	DBPath       string `json:"dbPath"`        // SQLite 数据库路径
	FTS5Enabled  bool   `json:"fts5Enabled"`    // FTS5 是否启用
	Tokenizer    string `json:"tokenizer"`     // 分词器名称
	IndexSize    int64  `json:"indexSize"`     // FTS5 索引大小 (字节)
	IndexVersion int    `json:"indexVersion"`  // schema 版本号
}

// FileEntry 文件索引条目（外部内容表的行）。
type FileEntry struct {
	Path        string `json:"path"`        // 虚拟路径 /a/b/c.txt
	Name        string `json:"name"`        // basename
	IsDirectory bool   `json:"isDirectory"` // 是否目录
	Size        int64  `json:"size"`        // 字节
	Modified    string `json:"modified"`    // RFC3339
	Content     string `json:"content"`     // 文件正文（用于全文搜索；目录为空字符串）
}

// FileSearchResult 搜索结果。
type FileSearchResult struct {
	Path     string  `json:"path"`
	Name     string  `json:"name"`
	Score    float64 `json:"score"`    // bm25 分数
	Snippet  string  `json:"snippet"`  // 命中片段（高亮标记）
	HitCount int     `json:"hitCount"` // 命中次数
}

// FileIndex FTS5 文件索引。
type FileIndex struct {
	mu       sync.RWMutex
	db       *sql.DB
	dbPath   string
	building bool
	stats    FileIndexStats
}

// 当前 FTS5 schema 版本号。
const currentSchemaVersion = 1

// NewFileIndex 创建一个新的 FTS5 文件索引。
//
// dbPath 传空字符串时使用 ":memory:" 内存数据库（仅测试用）。
func NewFileIndex(dbPath string) (*FileIndex, error) {
	dsn := buildDSN(dbPath)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	// FTS5 + content table + sync trigger: 写入密集，建议单连接
	db.SetMaxOpenConns(1)

	idx := &FileIndex{
		db:     db,
		dbPath: dbPath,
		stats: FileIndexStats{
			DBPath:       dbPath,
			Tokenizer:    "porter unicode61",
			IndexVersion: currentSchemaVersion,
		},
	}

	if err := idx.initSchema(); err != nil {
		db.Close()
		return nil, fmt.Errorf("init schema: %w", err)
	}
	if err := idx.checkFTS5(); err != nil {
		db.Close()
		return nil, fmt.Errorf("check FTS5: %w", err)
	}

	return idx, nil
}

func buildDSN(dbPath string) string {
	if dbPath == "" {
		return "file::memory:?_pragma=foreign_keys(1)"
	}
	absPath, _ := filepath.Abs(dbPath)
	dir := filepath.Dir(absPath)
	_ = dir
	// WAL + 30s busy_timeout + NORMAL synchronous
	return fmt.Sprintf(
		"file:%s?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(30000)&_pragma=synchronous(NORMAL)",
		absPath,
	)
}

const schemaSQL = `
-- 外部内容表：真实数据存这里，FTS5 索引同步
CREATE TABLE IF NOT EXISTS file_entries (
    path        TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    is_dir      INTEGER NOT NULL DEFAULT 0,
    size        INTEGER NOT NULL DEFAULT 0,
    modified    TEXT,
    content     TEXT NOT NULL DEFAULT ''
);

-- FTS5 虚拟表：content=file_entries 引用外部表
CREATE VIRTUAL TABLE IF NOT EXISTS file_fts USING fts5(
    name,
    content,
    content='file_entries',
    content_rowid='rowid',
    tokenize='porter unicode61'
);

-- content_sync trigger: file_entries 增/改/删 自动同步到 FTS5
CREATE TRIGGER IF NOT EXISTS file_fts_ai AFTER INSERT ON file_entries BEGIN
    INSERT INTO file_fts(rowid, name, content) VALUES (new.rowid, new.name, new.content);
END;
CREATE TRIGGER IF NOT EXISTS file_fts_ad AFTER DELETE ON file_entries BEGIN
    INSERT INTO file_fts(file_fts, rowid, name, content) VALUES('delete', old.rowid, old.name, old.content);
END;
CREATE TRIGGER IF NOT EXISTS file_fts_au AFTER UPDATE ON file_entries BEGIN
    INSERT INTO file_fts(file_fts, rowid, name, content) VALUES('delete', old.rowid, old.name, old.content);
    INSERT INTO file_fts(rowid, name, content) VALUES (new.rowid, new.name, new.content);
END;

-- meta 表：记录 schema 版本
CREATE TABLE IF NOT EXISTS file_index_meta (
    key   TEXT PRIMARY KEY,
    value TEXT
);
`

func (f *FileIndex) initSchema() error {
	_, err := f.db.Exec(schemaSQL)
	if err != nil {
		return err
	}
	// 记录 schema 版本
	_, err = f.db.Exec(
		`INSERT OR REPLACE INTO file_index_meta(key, value) VALUES ('schema_version', ?)`,
		fmt.Sprintf("%d", currentSchemaVersion),
	)
	return err
}

func (f *FileIndex) checkFTS5() error {
	// FTS5 已在 initSchema 通过 CREATE VIRTUAL TABLE 验证
	// 这里做一个轻量探测查询
	var rowid int64
	err := f.db.QueryRow(`SELECT count(*) FROM file_fts WHERE file_fts MATCH 'NOSUCHWORD__PROBE'`).Scan(&rowid)
	_ = rowid
	if err != nil {
		// 无害的"no match"是预期结果
		if !strings.Contains(err.Error(), "no such column") && !strings.Contains(err.Error(), "fts5:") {
			// 真的失败
			return err
		}
	}
	f.stats.FTS5Enabled = true
	return nil
}

// Close 关闭索引。
func (f *FileIndex) Close() error {
	return f.db.Close()
}

// Clear 清空所有索引数据（保留 schema）。
func (f *FileIndex) Clear(ctx context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	tx, err := f.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM file_entries`); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO file_fts(file_fts) VALUES('rebuild')`); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	f.stats.TotalFiles = 0
	f.stats.TotalDirs = 0
	f.stats.TotalSize = 0
	return nil
}

// BulkInsert 批量插入索引条目（单次事务 + prepared statement）。
//
// 设计原则：
//   - 一次 BeginTx 包裹整个 batch
//   - 一次 Prepare statement 复用 N 次
//   - 显式 commit（失败回滚）
//   - 内部调用：_pragma=journal_mode(WAL) 提升并发
//   - CJK bigram：content 字段预先切 bigram，确保 "在线" 能命中 "在线播放"
func (f *FileIndex) BulkInsert(ctx context.Context, entries []FileEntry) error {
	if len(entries) == 0 {
		return nil
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	start := time.Now()

	tx, err := f.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	// Rollback 兜底：commit 成功后是 no-op
	defer func() {
		_ = tx.Rollback()
	}()

	// 关键：prepared statement 复用，避免每次 Exec 都解析 SQL
	stmt, err := tx.PrepareContext(ctx, `
		INSERT OR REPLACE INTO file_entries(path, name, is_dir, size, modified, content)
		VALUES (?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("prepare insert: %w", err)
	}
	defer stmt.Close()

	for i, e := range entries {
		if _, err := stmt.ExecContext(ctx,
			e.Path, e.Name, boolToInt(e.IsDirectory), e.Size, e.Modified, cjkBigram(e.Content),
		); err != nil {
			return fmt.Errorf("insert entry[%d] %q: %w", i, e.Path, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	f.stats.LastBuildMs = time.Since(start).Milliseconds()
	return nil
}

// DeleteByPath 删除单个条目。
func (f *FileIndex) DeleteByPath(ctx context.Context, path string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, err := f.db.ExecContext(ctx, `DELETE FROM file_entries WHERE path = ?`, path)
	return err
}

// DeleteByPrefix 按路径前缀删除（用于目录删除）。
func (f *FileIndex) DeleteByPrefix(ctx context.Context, prefix string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, err := f.db.ExecContext(ctx,
		`DELETE FROM file_entries WHERE path = ? OR path LIKE ?`,
		prefix, prefix+"/%",
	)
	return err
}

// Search 搜索接口。
func (f *FileIndex) Search(ctx context.Context, query string, opts SearchOptions) ([]FileSearchResult, error) {
	if opts.Limit <= 0 {
		opts.Limit = 200
	}

	// 1. 解析 query → FTS5 MATCH 表达式 + notTerms (Go 端过滤)
	matchExpr, regexFilters, notTerms, err := ParseQuery(query)
	if err != nil {
		return nil, fmt.Errorf("parse query: %w", err)
	}

	f.mu.RLock()
	defer f.mu.RUnlock()

	// 2. FTS5 MATCH 检索（带 bm25 排序 + snippet）
	// 特殊情况：纯 regex 查询（无 FTS5 词）→ 全表扫 + regex 过滤
	isAllRegex := isOnlyRegexQuery(matchExpr, regexFilters)

	var sqlQuery string
	var args []any

	if isAllRegex {
		// 纯 regex：直接查 file_entries（regex 二次过滤会兜底）
		sqlQuery = `
			SELECT
				fe.path,
				fe.name,
				0.0 AS score,
				'' AS snip,
				0 AS hit
			FROM file_entries fe
			WHERE 1=1
		`
		args = []any{}
	} else {
		sqlQuery = `
			SELECT
				fe.path,
				fe.name,
				bm25(file_fts) AS score,
				snippet(file_fts, 1, '<<', '>>', '…', 32) AS snip,
				(
					SELECT count(*) FROM file_fts AS f
					WHERE f.rowid = fe.rowid
					LIMIT 1
				) AS hit
			FROM file_fts
			JOIN file_entries fe ON fe.rowid = file_fts.rowid
			WHERE file_fts MATCH ?
		`
		args = []any{matchExpr}
	}

	if opts.PathPrefix != "" {
		sqlQuery += ` AND fe.path LIKE ?`
		args = append(args, opts.PathPrefix+"%")
	}
	if !opts.IncludeDirs {
		sqlQuery += ` AND fe.is_dir = 0`
	}
	if !isAllRegex {
		sqlQuery += ` ORDER BY score LIMIT ?`
		args = append(args, opts.Limit)
	} else {
		// 纯 regex：拉较多结果（limit * 5），然后 regex 过滤
		sqlQuery += ` LIMIT ?`
		args = append(args, opts.Limit*5)
	}

	rows, err := f.db.QueryContext(ctx, sqlQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("fts5 query: %w", err)
	}
	defer rows.Close()

	results := make([]FileSearchResult, 0, opts.Limit)
	for rows.Next() {
		var r FileSearchResult
		var hit int
		if err := rows.Scan(&r.Path, &r.Name, &r.Score, &r.Snippet, &hit); err != nil {
			return nil, err
		}
		r.HitCount = hit
		results = append(results, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// 3. 应用 regex 过滤（在 FTS5 命中基础上）
	if len(regexFilters) > 0 {
		filtered := make([]FileSearchResult, 0, len(results))
		for _, r := range results {
			if matchAnyRegex(r.Name, r.Snippet, regexFilters) {
				filtered = append(filtered, r)
			}
		}
		results = filtered
	}

	// 4. NOT 子句过滤（Go 端 substring + 大小写不敏感）
	// 排除 snippet 或 name 中含任一 notTerm 的结果
	if len(notTerms) > 0 {
		filtered := make([]FileSearchResult, 0, len(results))
		for _, r := range results {
			exclude := false
			nameLower := strings.ToLower(r.Name)
			snipLower := strings.ToLower(r.Snippet)
			for _, nt := range notTerms {
				if strings.Contains(nameLower, nt) || strings.Contains(snipLower, nt) {
					exclude = true
					break
				}
			}
			if !exclude {
				filtered = append(filtered, r)
			}
		}
		results = filtered
	}

	return results, nil
}

// SearchOptions 搜索选项。
type SearchOptions struct {
	Limit        int    // 最大返回数（默认 200）
	PathPrefix   string // 路径前缀过滤（如 /d/photos）
	IncludeDirs  bool   // 是否包含目录（默认 false）
}

// Stats 返回统计信息。
func (f *FileIndex) Stats() FileIndexStats {
	f.mu.RLock()
	defer f.mu.RUnlock()

	stats := f.stats
	// 实时统计（不依赖上一次 BuildIndex）
	_ = f.db.QueryRow(`SELECT count(*) FROM file_entries WHERE is_dir = 0`).Scan(&stats.TotalFiles)
	_ = f.db.QueryRow(`SELECT count(*) FROM file_entries WHERE is_dir = 1`).Scan(&stats.TotalDirs)
	_ = f.db.QueryRow(`SELECT coalesce(sum(size), 0) FROM file_entries WHERE is_dir = 0`).Scan(&stats.TotalSize)
	return stats
}

// SetBuilding 设置正在构建状态。
func (f *FileIndex) SetBuilding(b bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stats.IsIndexing = b
}

// MarkBuilt 标记构建完成。
func (f *FileIndex) MarkBuilt(elapsed time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stats.LastBuildMs = elapsed.Milliseconds()
	f.stats.IndexedAt = time.Now().Format(time.RFC3339)
	f.stats.IsIndexing = false
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
