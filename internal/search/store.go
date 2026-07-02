package vectorsearch

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"time"
)

// IndexType 搜索索引类型。
type IndexType string

const (
	IndexTypeFile   IndexType = "file"   // 文件索引
	IndexTypeTask   IndexType = "task"   // 任务索引
	IndexTypeCustom IndexType = "custom" // 自定义索引
)

// SearchResult 搜索结果。
type SearchResult struct {
	RefID     string  // 引用 ID（文件路径 / 任务 ID）
	Title     string  // 标题
	Extra     string  // 额外信息（JSON 格式）
	Score     float64 // 相似度分数 (0-1, 越大越相似)
	IndexType IndexType
}

// IndexItem 待索引条目。
type IndexItem struct {
	RefID     string
	Title     string
	Content   string // 用于生成向量的完整文本（标题 + 描述 + 标签等）
	Extra     string
	IndexType IndexType
}

// Store 向量搜索存储接口。
type Store interface {
	// Init 初始化索引表
	Init(ctx context.Context) error

	// Upsert 插入或更新索引条目
	Upsert(ctx context.Context, item *IndexItem) error

	// UpsertBatch 批量插入/更新
	UpsertBatch(ctx context.Context, items []*IndexItem) error

	// Delete 删除索引条目
	Delete(ctx context.Context, indexType IndexType, refID string) error

	// Search 向量相似度搜索
	Search(ctx context.Context, indexType IndexType, query string, limit int) ([]SearchResult, error)

	// Clear 清空指定类型的索引
	Clear(ctx context.Context, indexType IndexType) error

	// Close 关闭存储
	Close() error
}

// NewStore 创建向量搜索存储。
//
// 根据驱动自动选择实现：
//   - turso：使用原生 vector_distance_cos 函数（Turso Cloud / libSQL Server）
//   - libsql / sqlite：使用纯 Go 实现的余弦相似度计算
//
// 注意：本地嵌入式 libsql（CGO .so）不支持 vector_distance_cos 函数，
// 该函数是 Turso Cloud / libSQL Server 的特性。因此 libsql 走 SQLiteStore
// 在 Go 层计算余弦相似度，避免 SQL 函数不存在的错误。
func NewStore(db *sql.DB, driver string) (Store, error) {
	switch driver {
	case "turso":
		return &TursoStore{db: db}, nil
	case "libsql", "sqlite":
		return &SQLiteStore{db: db}, nil
	default:
		return nil, fmt.Errorf("unsupported driver: %s", driver)
	}
}

// ─── Turso 实现 ─────────────────────────────────────────────────

// TursoStore 基于 Turso 原生向量函数的实现。
type TursoStore struct {
	db *sql.DB
}

func (s *TursoStore) Init(ctx context.Context) error {
	schema := `
	CREATE TABLE IF NOT EXISTS search_index (
		ref_id      TEXT NOT NULL,
		index_type  TEXT NOT NULL,
		title       TEXT NOT NULL,
		extra       TEXT,
		embedding   BLOB NOT NULL,
		updated_at  DATETIME NOT NULL,
		PRIMARY KEY (index_type, ref_id)
	);
	CREATE INDEX IF NOT EXISTS idx_search_index_type ON search_index(index_type);
	`
	_, err := s.db.ExecContext(ctx, schema)
	if err != nil {
		return fmt.Errorf("init turso search schema: %w", err)
	}
	return nil
}

func (s *TursoStore) Upsert(ctx context.Context, item *IndexItem) error {
	vec := TextToVector(item.Title + " " + item.Content)
	blob := EncodeVector(vec)
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO search_index (ref_id, index_type, title, extra, embedding, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(index_type, ref_id) DO UPDATE SET
			title = excluded.title,
			extra = excluded.extra,
			embedding = excluded.embedding,
			updated_at = excluded.updated_at
	`, item.RefID, string(item.IndexType), item.Title, item.Extra, blob, time.Now())
	return err
}

func (s *TursoStore) UpsertBatch(ctx context.Context, items []*IndexItem) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO search_index (ref_id, index_type, title, extra, embedding, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(index_type, ref_id) DO UPDATE SET
			title = excluded.title,
			extra = excluded.extra,
			embedding = excluded.embedding,
			updated_at = excluded.updated_at
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	now := time.Now()
	for _, item := range items {
		vec := TextToVector(item.Title + " " + item.Content)
		blob := EncodeVector(vec)
		if _, err := stmt.ExecContext(ctx, item.RefID, string(item.IndexType), item.Title, item.Extra, blob, now); err != nil {
			return fmt.Errorf("upsert batch item %s: %w", item.RefID, err)
		}
	}

	return tx.Commit()
}

func (s *TursoStore) Delete(ctx context.Context, indexType IndexType, refID string) error {
	_, err := s.db.ExecContext(ctx,
		"DELETE FROM search_index WHERE index_type = ? AND ref_id = ?",
		string(indexType), refID,
	)
	return err
}

func (s *TursoStore) Search(ctx context.Context, indexType IndexType, query string, limit int) ([]SearchResult, error) {
	if limit <= 0 {
		limit = 20
	}

	queryVec := BuildQueryVector(query)
	queryBlob := EncodeVector(queryVec)

	rows, err := s.db.QueryContext(ctx, `
		SELECT ref_id, title, extra,
		       1.0 - vector_distance_cos(embedding, ?) AS score
		FROM search_index
		WHERE index_type = ?
		ORDER BY score DESC
		LIMIT ?
	`, queryBlob, string(indexType), limit)
	if err != nil {
		return nil, fmt.Errorf("turso vector search: %w", err)
	}
	defer rows.Close()

	var results []SearchResult
	for rows.Next() {
		var r SearchResult
		var score float64
		if err := rows.Scan(&r.RefID, &r.Title, &r.Extra, &score); err != nil {
			return nil, err
		}
		r.Score = score
		r.IndexType = indexType
		results = append(results, r)
	}
	return results, rows.Err()
}

func (s *TursoStore) Clear(ctx context.Context, indexType IndexType) error {
	_, err := s.db.ExecContext(ctx,
		"DELETE FROM search_index WHERE index_type = ?",
		string(indexType),
	)
	return err
}

func (s *TursoStore) Close() error {
	return s.db.Close()
}

// ─── SQLite 实现（纯 Go 计算相似度）────────────────────────────

// SQLiteStore 基于 SQLite 的实现（Go 层计算向量相似度）。
type SQLiteStore struct {
	db *sql.DB
}

func (s *SQLiteStore) Init(ctx context.Context) error {
	schema := `
	CREATE TABLE IF NOT EXISTS search_index (
		ref_id      TEXT NOT NULL,
		index_type  TEXT NOT NULL,
		title       TEXT NOT NULL,
		extra       TEXT,
		embedding   BLOB NOT NULL,
		updated_at  DATETIME NOT NULL,
		PRIMARY KEY (index_type, ref_id)
	);
	CREATE INDEX IF NOT EXISTS idx_search_index_type ON search_index(index_type);
	`
	_, err := s.db.ExecContext(ctx, schema)
	if err != nil {
		return fmt.Errorf("init sqlite search schema: %w", err)
	}
	return nil
}

func (s *SQLiteStore) Upsert(ctx context.Context, item *IndexItem) error {
	vec := TextToVector(item.Title + " " + item.Content)
	blob := EncodeVector(vec)
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO search_index (ref_id, index_type, title, extra, embedding, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(index_type, ref_id) DO UPDATE SET
			title = excluded.title,
			extra = excluded.extra,
			embedding = excluded.embedding,
			updated_at = excluded.updated_at
	`, item.RefID, string(item.IndexType), item.Title, item.Extra, blob, time.Now())
	return err
}

func (s *SQLiteStore) UpsertBatch(ctx context.Context, items []*IndexItem) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO search_index (ref_id, index_type, title, extra, embedding, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(index_type, ref_id) DO UPDATE SET
			title = excluded.title,
			extra = excluded.extra,
			embedding = excluded.embedding,
			updated_at = excluded.updated_at
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	now := time.Now()
	for _, item := range items {
		vec := TextToVector(item.Title + " " + item.Content)
		blob := EncodeVector(vec)
		if _, err := stmt.ExecContext(ctx, item.RefID, string(item.IndexType), item.Title, item.Extra, blob, now); err != nil {
			return fmt.Errorf("upsert batch item %s: %w", item.RefID, err)
		}
	}

	return tx.Commit()
}

func (s *SQLiteStore) Delete(ctx context.Context, indexType IndexType, refID string) error {
	_, err := s.db.ExecContext(ctx,
		"DELETE FROM search_index WHERE index_type = ? AND ref_id = ?",
		string(indexType), refID,
	)
	return err
}

func (s *SQLiteStore) Search(ctx context.Context, indexType IndexType, query string, limit int) ([]SearchResult, error) {
	if limit <= 0 {
		limit = 20
	}

	queryVec := BuildQueryVector(query)

	// 先取出所有该类型的向量（SQLite 没有向量函数，在 Go 层计算）
	rows, err := s.db.QueryContext(ctx, `
		SELECT ref_id, title, extra, embedding
		FROM search_index
		WHERE index_type = ?
	`, string(indexType))
	if err != nil {
		return nil, fmt.Errorf("sqlite vector search: %w", err)
	}
	defer rows.Close()

	type scored struct {
		res   SearchResult
		score float64
	}
	var all []scored

	for rows.Next() {
		var refID, title, extra string
		var blob []byte
		if err := rows.Scan(&refID, &title, &extra, &blob); err != nil {
			return nil, err
		}

		docVec := decodeVector(blob)
		score := CosineSimilarity(queryVec, docVec)

		all = append(all, scored{
			res: SearchResult{
				RefID:     refID,
				Title:     title,
				Extra:     extra,
				IndexType: indexType,
			},
			score: score,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// 按分数排序（简单冒泡，数据量不大）
	for i := 0; i < len(all); i++ {
		for j := i + 1; j < len(all); j++ {
			if all[j].score > all[i].score {
				all[i], all[j] = all[j], all[i]
			}
		}
	}

	// 取 top N
	if limit > len(all) {
		limit = len(all)
	}
	results := make([]SearchResult, limit)
	for i := 0; i < limit; i++ {
		results[i] = all[i].res
		results[i].Score = all[i].score
	}
	return results, nil
}

func (s *SQLiteStore) Clear(ctx context.Context, indexType IndexType) error {
	_, err := s.db.ExecContext(ctx,
		"DELETE FROM search_index WHERE index_type = ?",
		string(indexType),
	)
	return err
}

func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

// decodeVector 从 BLOB 解码 float32 向量。
func decodeVector(blob []byte) []float32 {
	n := len(blob) / 4
	vec := make([]float32, n)
	for i := 0; i < n; i++ {
		bits := uint32(blob[i*4]) |
			uint32(blob[i*4+1])<<8 |
			uint32(blob[i*4+2])<<16 |
			uint32(blob[i*4+3])<<24
		vec[i] = math.Float32frombits(bits)
	}
	return vec
}
