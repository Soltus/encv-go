package fts

// fileindex_search_test.go — Search() 端到端非乐观测试。
//
// 2026-07-02 阶段 7 补充：覆盖现有 TestFileIndex_BulkInsertAndSearch 没测的边界 case。
//
// 测试目标（每个 case 必须能独立失败 → 定位到具体问题）：
//   1. NOT 过滤：substring 排除（不区分大小写）
//   2. 多 NOT term 累加过滤
//   3. NOT 作用在 content（不只是 name）
//   4. PathPrefix 过滤
//   5. Limit 截断（limit < 总数）
//   6. Limit = 0 → 返回全部
//   7. Snippet 必须包含 `<<` 和 `>>`（命中位置）
//   8. HitCount >= 1
//   9. BM25 Score < 0（负数越小越相关）
//  10. 纯 regex 全表扫：返回文件名匹配项
//  11. 纯 regex 限制 limit
//  12. 复杂查询 AND + NOT
//  13. phrase + NOT（短语命中 + 排除）

import (
	"context"
	"strings"
	"testing"
)

func TestSearch_NOTFilterSubstring(t *testing.T) {
	idx := newTestIndex(t)
	ctx := context.Background()

	entries := []FileEntry{
		{Path: "/a.txt", Name: "a.txt", Content: "在线 高清 视频"},
		{Path: "/b.txt", Name: "b.txt", Content: "在线 高清 摄影"},
		{Path: "/c.txt", Name: "c.txt", Content: "在线 标清 视频"},
		{Path: "/d.txt", Name: "d.txt", Content: "在线 标清 摄影"},
	}
	if err := idx.BulkInsert(ctx, entries); err != nil {
		t.Fatalf("BulkInsert: %v", err)
	}

	// 排除 "视频" → 只剩 b/d
	results, err := idx.Search(ctx, "在线 NOT 视频", SearchOptions{})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("NOT 视频 should keep 2 (b/d), got %d: %v", len(results), resultPaths(results))
	}
	for _, r := range results {
		if r.Path == "/a.txt" || r.Path == "/c.txt" {
			t.Errorf("NOT failed: %s should be excluded (contains 视频)", r.Path)
		}
	}

	// 排除 "摄影" → 只剩 a/c
	results, err = idx.Search(ctx, "在线 NOT 摄影", SearchOptions{})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	for _, r := range results {
		if r.Path == "/b.txt" || r.Path == "/d.txt" {
			t.Errorf("NOT failed: %s should be excluded (contains 摄影)", r.Path)
		}
	}
}

func TestSearch_MultipleNOTTerms(t *testing.T) {
	idx := newTestIndex(t)
	ctx := context.Background()

	entries := []FileEntry{
		{Path: "/a.txt", Name: "a.txt", Content: "在线 高清 视频 广告"},
		{Path: "/b.txt", Name: "b.txt", Content: "在线 高清 视频"}, // 没有广告
		{Path: "/c.txt", Name: "c.txt", Content: "在线 高清 摄影"},
	}
	if err := idx.BulkInsert(ctx, entries); err != nil {
		t.Fatalf("BulkInsert: %v", err)
	}

	// 排除 "视频" 和 "摄影" → 只剩 b（在线 高清 视频 不含 摄影，但含 视频；所以排除 → 没了）
	// 等等：b 含 视频 → 排除。a 含 视频 → 排除。c 含 摄影 → 排除。0 结果
	results, err := idx.Search(ctx, "在线 NOT 视频 NOT 摄影", SearchOptions{})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("multi-NOT should keep 0, got %d: %v", len(results), resultPaths(results))
	}
}

func TestSearch_PathPrefix(t *testing.T) {
	idx := newTestIndex(t)
	ctx := context.Background()

	entries := []FileEntry{
		{Path: "/photos/2024/a.jpg", Name: "a.jpg", Content: "在线 高清"},
		{Path: "/photos/2023/b.jpg", Name: "b.jpg", Content: "在线 高清"},
		{Path: "/videos/c.mp4", Name: "c.mp4", Content: "在线 高清"},
	}
	if err := idx.BulkInsert(ctx, entries); err != nil {
		t.Fatalf("BulkInsert: %v", err)
	}

	results, err := idx.Search(ctx, "在线", SearchOptions{PathPrefix: "/photos"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("PathPrefix /photos should keep 2 (a/b), got %d: %v", len(results), resultPaths(results))
	}
	for _, r := range results {
		if !strings.HasPrefix(r.Path, "/photos") {
			t.Errorf("PathPrefix failed: %s not under /photos", r.Path)
		}
	}
}

func TestSearch_Limit(t *testing.T) {
	idx := newTestIndex(t)
	ctx := context.Background()

	entries := make([]FileEntry, 10)
	for i := 0; i < 10; i++ {
		entries[i] = FileEntry{
			Path:    "/f" + string(rune('0'+i)) + ".txt",
			Name:    "f" + string(rune('0'+i)) + ".txt",
			Content: "在线 高清",
		}
	}
	if err := idx.BulkInsert(ctx, entries); err != nil {
		t.Fatalf("BulkInsert: %v", err)
	}

	// limit=3 → 只返 3 个
	results, err := idx.Search(ctx, "在线", SearchOptions{Limit: 3})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 3 {
		t.Errorf("limit=3 should return 3, got %d", len(results))
	}

	// limit=0 → 返全部
	results, err = idx.Search(ctx, "在线", SearchOptions{Limit: 0})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 10 {
		t.Errorf("limit=0 should return all 10, got %d", len(results))
	}
}

func TestSearch_SnippetFormat(t *testing.T) {
	idx := newTestIndex(t)
	ctx := context.Background()

	entries := []FileEntry{
		{Path: "/a.txt", Name: "a.txt", Content: "这是一段很长的文字 在线 高清 视频 后续内容继续填充用于测试FTS5的snippet生成功能"},
	}
	if err := idx.BulkInsert(ctx, entries); err != nil {
		t.Fatalf("BulkInsert: %v", err)
	}

	results, err := idx.Search(ctx, "在线", SearchOptions{})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) < 1 {
		t.Fatalf("expected at least 1 result, got %d", len(results))
	}
	r := results[0]
	// snippet 必须含 `<<` 和 `>>` 标记（除非内容极短没有 tokenize 价值）
	if !strings.Contains(r.Snippet, "<<") || !strings.Contains(r.Snippet, ">>") {
		t.Errorf("snippet missing <<...>> markers: %q", r.Snippet)
	}
	// 至少一对 <<>>
	open := strings.Count(r.Snippet, "<<")
	close := strings.Count(r.Snippet, ">>")
	if open != close {
		t.Errorf("unbalanced markers: <<=%d, >>=%d, snippet=%q", open, close, r.Snippet)
	}
	if r.HitCount < 1 {
		t.Errorf("expected HitCount >= 1, got %d", r.HitCount)
	}
}

func TestSearch_BM25Score(t *testing.T) {
	idx := newTestIndex(t)
	ctx := context.Background()

	entries := []FileEntry{
		{Path: "/a.txt", Name: "a.txt", Content: "在线 高清 视频"},
	}
	if err := idx.BulkInsert(ctx, entries); err != nil {
		t.Fatalf("BulkInsert: %v", err)
	}

	results, err := idx.Search(ctx, "在线", SearchOptions{})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) < 1 {
		t.Fatalf("expected at least 1 result, got %d", len(results))
	}
	// BM25 score 必须是负数（越小越相关）
	if results[0].Score >= 0 {
		t.Errorf("BM25 score should be negative, got %f", results[0].Score)
	}
}

func TestSearch_PureRegex(t *testing.T) {
	idx := newTestIndex(t)
	ctx := context.Background()

	entries := []FileEntry{
		{Path: "/sunset.jpg", Name: "sunset.jpg", Content: "摄影"},
		{Path: "/mountain.jpg", Name: "mountain.jpg", Content: "摄影"},
		{Path: "/sunday.txt", Name: "sunday.txt", Content: "日记"},
	}
	if err := idx.BulkInsert(ctx, entries); err != nil {
		t.Fatalf("BulkInsert: %v", err)
	}

	// 纯 regex:^sun 命中所有以 sun 开头
	results, err := idx.Search(ctx, "regex:^sun", SearchOptions{})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) < 2 {
		t.Errorf("regex ^sun should match 2 (sunset, sunday), got %d: %v", len(results), resultPaths(results))
	}
	for _, r := range results {
		if !strings.HasPrefix(r.Name, "sun") {
			t.Errorf("regex filter failed: %s should match ^sun", r.Name)
		}
	}
}

func TestSearch_PureRegexLimit(t *testing.T) {
	idx := newTestIndex(t)
	ctx := context.Background()

	entries := make([]FileEntry, 20)
	for i := 0; i < 20; i++ {
		entries[i] = FileEntry{
			Path:    "/test" + string(rune('a'+i%26)) + string(rune('0'+i/26)) + ".txt",
			Name:    "test" + string(rune('a'+i%26)) + string(rune('0'+i/26)) + ".txt",
			Content: "内容",
		}
	}
	if err := idx.BulkInsert(ctx, entries); err != nil {
		t.Fatalf("BulkInsert: %v", err)
	}

	// 纯 regex:^test + limit=5 → 最多 5 个
	results, err := idx.Search(ctx, "regex:^test", SearchOptions{Limit: 5})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) > 5 {
		t.Errorf("limit=5 should return ≤5, got %d", len(results))
	}
}

func TestSearch_PhraseWithNOT(t *testing.T) {
	idx := newTestIndex(t)
	ctx := context.Background()

	entries := []FileEntry{
		// a: 包含 phrase + notTerm
		{Path: "/a.txt", Name: "a.txt", Content: "在线播放 有广告内容"},
		// b: 包含 phrase 但 notTerm 在 name（snippet 不含 name）
		{Path: "/广告-b.txt", Name: "广告-b.txt", Content: "在线播放 没有广告内容"},
		// c: 包含 phrase 不含 notTerm（snippet 也不会含 notTerm）
		{Path: "/c.txt", Name: "c.txt", Content: "在线播放 纯净内容"},
	}
	if err := idx.BulkInsert(ctx, entries); err != nil {
		t.Fatalf("BulkInsert: %v", err)
	}

	// 短语 + NOT 验证：name 含 广告 的会被排除（subtitle filter on name）
	results, err := idx.Search(ctx, `"在线播放" NOT 广告`, SearchOptions{})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	// 期望: a 被排除（snippet 包含 广告），b 被排除（name 包含 广告），c 保留
	if len(results) < 1 {
		t.Errorf("expected at least 1 result, got %d: %v", len(results), resultPaths(results))
	}
	for _, r := range results {
		if r.Path == "/a.txt" {
			t.Errorf("phrase + NOT failed: a (snippet contains 广告) should be excluded")
		}
		if r.Path == "/广告-b.txt" {
			t.Errorf("phrase + NOT failed: b (name contains 广告) should be excluded")
		}
	}
}

func TestSearch_EmptyContent(t *testing.T) {
	idx := newTestIndex(t)
	ctx := context.Background()

	entries := []FileEntry{
		{Path: "/empty.txt", Name: "empty.txt", Content: ""},
		{Path: "/a.txt", Name: "a.txt", Content: "在线"},
	}
	if err := idx.BulkInsert(ctx, entries); err != nil {
		t.Fatalf("BulkInsert: %v", err)
	}

	// 搜 "在线" → 不应包含空内容文件
	results, err := idx.Search(ctx, "在线", SearchOptions{})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 result (a.txt), got %d: %v", len(results), resultPaths(results))
	}
	if len(results) > 0 && results[0].Path != "/a.txt" {
		t.Errorf("expected /a.txt, got %s", results[0].Path)
	}
}

// resultPaths helper for error messages
func resultPaths(results []FileSearchResult) []string {
	paths := make([]string, len(results))
	for i, r := range results {
		paths[i] = r.Path
	}
	return paths
}
