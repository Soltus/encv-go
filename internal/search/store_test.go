package vectorsearch

import (
	"context"
	"database/sql"
	"os"
	"testing"

	_ "turso.tech/database/tursogo"
)

func TestTursoStore(t *testing.T) {
	// 创建临时数据库
	tmpFile := t.TempDir() + "/test-search.db"

	db, err := sql.Open("turso", tmpFile)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(8)

	store := &TursoStore{db: db}
	ctx := context.Background()

	if err := store.Init(ctx); err != nil {
		t.Fatal(err)
	}

	// 插入测试数据
	items := []*IndexItem{
		{RefID: "1", Title: "加密视频文件", Content: "使用 AES-256 加密的 MP4 视频文件", IndexType: IndexTypeFile},
		{RefID: "2", Title: "解密文档", Content: "PDF 文档的解密处理流程", IndexType: IndexTypeFile},
		{RefID: "3", Title: "图片压缩", Content: "JPEG 图片压缩工具", IndexType: IndexTypeFile},
		{RefID: "4", Title: "加密任务", Content: "批量加密视频文件的任务", IndexType: IndexTypeTask},
		{RefID: "5", Title: "备份任务", Content: "数据库备份任务", IndexType: IndexTypeTask},
	}

	if err := store.UpsertBatch(ctx, items); err != nil {
		t.Fatal(err)
	}

	// 测试文件搜索
	results, err := store.Search(ctx, IndexTypeFile, "视频加密", 5)
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("搜索 '视频加密' 结果:")
	for _, r := range results {
		t.Logf("  [%.4f] %s - %s", r.Score, r.RefID, r.Title)
	}

	if len(results) == 0 {
		t.Error("expected at least one result")
	}

	// 第一个结果应该是"加密视频文件"
	if results[0].RefID != "1" {
		t.Errorf("top result should be '加密视频文件' (ref 1), got ref %s (%s)", results[0].RefID, results[0].Title)
	}

	// 测试任务搜索
	taskResults, err := store.Search(ctx, IndexTypeTask, "加密", 5)
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("任务搜索 '加密' 结果:")
	for _, r := range taskResults {
		t.Logf("  [%.4f] %s - %s", r.Score, r.RefID, r.Title)
	}

	if len(taskResults) == 0 || taskResults[0].RefID != "4" {
		t.Error("task search should return '加密任务' as top result")
	}

	// 测试删除
	if err := store.Delete(ctx, IndexTypeFile, "3"); err != nil {
		t.Fatal(err)
	}

	resultsAfterDelete, _ := store.Search(ctx, IndexTypeFile, "压缩", 5)
	for _, r := range resultsAfterDelete {
		if r.RefID == "3" {
			t.Error("deleted item should not appear in results")
		}
	}

	_ = os.Remove(tmpFile)
}

func TestSQLiteStore(t *testing.T) {
	// 用现代的 SQLite 驱动测试
	tmpFile := t.TempDir() + "/test-search-sqlite.db"

	db, err := sql.Open("sqlite", "file:"+tmpFile+"?_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)")
	if err != nil {
		t.Skip("sqlite driver not available:", err)
		return
	}
	defer db.Close()
	db.SetMaxOpenConns(1)

	store := &SQLiteStore{db: db}
	ctx := context.Background()

	if err := store.Init(ctx); err != nil {
		t.Fatal(err)
	}

	items := []*IndexItem{
		{RefID: "1", Title: "加密视频文件", Content: "AES 加密视频", IndexType: IndexTypeFile},
		{RefID: "2", Title: "解密文档", Content: "PDF 解密", IndexType: IndexTypeFile},
		{RefID: "3", Title: "图片压缩", Content: "JPEG 压缩", IndexType: IndexTypeFile},
	}

	if err := store.UpsertBatch(ctx, items); err != nil {
		t.Fatal(err)
	}

	results, err := store.Search(ctx, IndexTypeFile, "视频加密", 5)
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("SQLite 搜索 '视频加密' 结果:")
	for _, r := range results {
		t.Logf("  [%.4f] %s - %s", r.Score, r.RefID, r.Title)
	}

	if len(results) == 0 {
		t.Error("expected at least one result")
	}

	if results[0].RefID != "1" {
		t.Errorf("top result should be ref 1, got ref %s", results[0].RefID)
	}
}
