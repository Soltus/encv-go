package fts

import (
	"context"
	"strings"
	"testing"
)

func newTestIndex(t *testing.T) *FileIndex {
	t.Helper()
	idx, err := NewFileIndex("")
	if err != nil {
		t.Fatalf("NewFileIndex: %v", err)
	}
	t.Cleanup(func() { _ = idx.Close() })
	return idx
}

func TestFileIndex_BulkInsertAndSearch(t *testing.T) {
	idx := newTestIndex(t)
	ctx := context.Background()

	entries := []FileEntry{
		{Path: "/photos/2024/sunset.jpg", Name: "sunset.jpg", IsDirectory: false, Size: 1024, Content: "在线播放 高清 视频"},
		{Path: "/photos/2024/mountain.jpg", Name: "mountain.jpg", IsDirectory: false, Size: 2048, Content: "高清 摄影"},
		{Path: "/photos/2023/old.jpg", Name: "old.jpg", IsDirectory: false, Size: 512, Content: "在线 老照片"},
		{Path: "/docs/readme.txt", Name: "readme.txt", IsDirectory: false, Size: 256, Content: "本指南介绍如何使用在线编辑功能"},
		{Path: "/empty.txt", Name: "empty.txt", IsDirectory: false, Size: 0, Content: ""},
		{Path: "/photos", Name: "photos", IsDirectory: true, Size: 0, Content: ""},
	}

	if err := idx.BulkInsert(ctx, entries); err != nil {
		t.Fatalf("BulkInsert: %v", err)
	}

	// 测试 1: 单 token（隐式 AND 也只一个）
	results, err := idx.Search(ctx, "在线", SearchOptions{})
	if err != nil {
		t.Fatalf("Search 在线: %v", err)
	}
	if len(results) < 1 {
		t.Errorf("expected at least 1 result for '在线', got %d", len(results))
	}

	// 测试 2: 多 token 隐式 AND
	results, err = idx.Search(ctx, "在线 高清", SearchOptions{})
	if err != nil {
		t.Fatalf("Search 在线 高清: %v", err)
	}
	if len(results) < 1 {
		t.Errorf("expected at least 1 result for '在线 高清' (AND), got %d", len(results))
	}
	for _, r := range results {
		if r.Path != "/photos/2024/sunset.jpg" {
			t.Errorf("expected first result sunset.jpg, got %s", r.Path)
		}
	}

	// 测试 3: 显式 AND
	results, err = idx.Search(ctx, "在线 AND 高清", SearchOptions{})
	if err != nil {
		t.Fatalf("Search AND: %v", err)
	}
	if len(results) < 1 {
		t.Errorf("expected at least 1 result for explicit AND, got %d", len(results))
	}

	// 测试 4: 显式 OR
	results, err = idx.Search(ctx, "在线 OR 摄影", SearchOptions{})
	if err != nil {
		t.Fatalf("Search OR: %v", err)
	}
	if len(results) < 2 {
		t.Errorf("expected at least 2 results for OR, got %d", len(results))
	}

	// 测试 5: NOT
	results, err = idx.Search(ctx, "高清 NOT 视频", SearchOptions{})
	if err != nil {
		t.Fatalf("Search NOT: %v", err)
	}
	for _, r := range results {
		if r.Path == "/photos/2024/sunset.jpg" {
			t.Errorf("NOT failed: sunset.jpg should be excluded, got %s", r.Path)
		}
	}

	// 测试 6: 精确短语
	results, err = idx.Search(ctx, `"在线播放"`, SearchOptions{})
	if err != nil {
		t.Fatalf("Search phrase: %v", err)
	}
	if len(results) < 1 {
		t.Errorf("expected at least 1 result for phrase, got %d", len(results))
	}

	// 测试 7: regex 二次过滤
	results, err = idx.Search(ctx, `regex:^sun`, SearchOptions{})
	if err != nil {
		t.Fatalf("Search regex: %v", err)
	}
	if len(results) < 1 {
		t.Errorf("expected at least 1 result for regex ^sun, got %d", len(results))
	}
	for _, r := range results {
		if !strings.HasPrefix(r.Name, "sun") {
			t.Errorf("regex filter failed: %s should match ^sun", r.Name)
		}
	}

	// 测试 8: 路径前缀过滤
	results, err = idx.Search(ctx, "高清", SearchOptions{PathPrefix: "/photos/2024"})
	if err != nil {
		t.Fatalf("Search path prefix: %v", err)
	}
	if len(results) < 1 {
		t.Errorf("expected at least 1 result for path prefix, got %d", len(results))
	}
	for _, r := range results {
		if !strings.HasPrefix(r.Path, "/photos/2024") {
			t.Errorf("path prefix filter failed: %s", r.Path)
		}
	}

	// 测试 9: 包含目录
	results, err = idx.Search(ctx, "photos", SearchOptions{IncludeDirs: true})
	if err != nil {
		t.Fatalf("Search include dirs: %v", err)
	}
	if len(results) < 1 {
		t.Errorf("expected at least 1 result for include dirs, got %d", len(results))
	}
	hasDir := false
	for _, r := range results {
		if r.Path == "/photos" {
			hasDir = true
		}
	}
	if !hasDir {
		t.Errorf("expected /photos in results, got: %v", results)
	}
}

func TestFileIndex_ClearAndRebuild(t *testing.T) {
	idx := newTestIndex(t)
	ctx := context.Background()

	entries := []FileEntry{
		{Path: "/a.txt", Name: "a.txt", Content: "test"},
		{Path: "/b.txt", Name: "b.txt", Content: "data"},
	}
	if err := idx.BulkInsert(ctx, entries); err != nil {
		t.Fatalf("BulkInsert: %v", err)
	}

	stats := idx.Stats()
	if stats.TotalFiles < 2 {
		t.Errorf("expected TotalFiles >= 2, got %d", stats.TotalFiles)
	}

	if err := idx.Clear(ctx); err != nil {
		t.Fatalf("Clear: %v", err)
	}

	results, err := idx.Search(ctx, "test", SearchOptions{})
	if err != nil {
		t.Fatalf("Search after clear: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results after clear, got %d", len(results))
	}
}

func TestFileIndex_DeleteByPrefix(t *testing.T) {
	idx := newTestIndex(t)
	ctx := context.Background()

	entries := []FileEntry{
		{Path: "/dir1/a.txt", Name: "a.txt", Content: "hello"},
		{Path: "/dir1/b.txt", Name: "b.txt", Content: "world"},
		{Path: "/dir2/c.txt", Name: "c.txt", Content: "hello"},
	}
	if err := idx.BulkInsert(ctx, entries); err != nil {
		t.Fatalf("BulkInsert: %v", err)
	}

	if err := idx.DeleteByPrefix(ctx, "/dir1"); err != nil {
		t.Fatalf("DeleteByPrefix: %v", err)
	}

	results, _ := idx.Search(ctx, "hello", SearchOptions{})
	if len(results) != 1 {
		t.Errorf("expected 1 result (only /dir2/c.txt), got %d", len(results))
	}
	if len(results) > 0 && results[0].Path != "/dir2/c.txt" {
		t.Errorf("expected /dir2/c.txt, got %s", results[0].Path)
	}
}

// 性能测试移到 fileindex_perf_test.go（带 //go:build !race 标签）
// race 模式下不跑（race detector 性能退化 40-50x）

func intToStr(n int) string {
	const digits = "0123456789"
	if n == 0 {
		return "0"
	}
	var s string
	for n > 0 {
		s = string(digits[n%10]) + s
		n /= 10
	}
	return s
}

func TestParseQuery_Operators(t *testing.T) {
	tests := []struct {
		input      string
		expected   string
		hasRegex   bool
		expectNot  []string // 期望的 notTerms
	}{
		{"在线 高清", "在线 高清", false, nil},
		{"在线 AND 高清", "在线 AND 高清", false, nil}, // explicit AND produces FTS5 AND syntax
		{"在线 OR 摄影", "在线 OR 摄影", false, nil},
		{`"在线播放" 高清`, `"在线 线播 播放" 高清`, false, nil}, // phrase gets CJK bigram
		{`regex:^sun`, allRegexMarker, true, nil},
		{`regex:/^sun/`, allRegexMarker, true, nil},
		{"AND 在线", "在线", false, nil}, // leading AND ignored
		// NOT 被提取到 notTerms，不在 matchExpr 中（FTS5 NOT 语法限制）
		{"NOT 视频", "", false, []string{"视频"}},
		// "高清 NOT 视频" → matchExpr = "高清", notTerms = ["视频"]
		{"高清 NOT 视频", "高清", false, []string{"视频"}},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			match, regexes, notTerms, err := ParseQuery(tt.input)
			if err != nil {
				t.Fatalf("ParseQuery(%q): %v", tt.input, err)
			}
			if match != tt.expected {
				t.Errorf("matchExpr = %q, want %q", match, tt.expected)
			}
			if tt.hasRegex && len(regexes) == 0 {
				t.Errorf("expected regex filter, got none")
			}
			if !tt.hasRegex && len(regexes) > 0 {
				t.Errorf("did not expect regex, got %d", len(regexes))
			}
			if len(notTerms) != len(tt.expectNot) {
				t.Errorf("notTerms = %v, want %v", notTerms, tt.expectNot)
			} else {
				for i, n := range tt.expectNot {
					if notTerms[i] != n {
						t.Errorf("notTerms[%d] = %q, want %q", i, notTerms[i], n)
					}
				}
			}
		})
	}
}

func TestParseQuery_Errors(t *testing.T) {
	tests := []string{
		"",          // empty
		`"unclosed`, // unclosed quote
		`regex:[`,   // invalid regex
		`regex:/`,   // unclosed regex
		`regex:`,    // empty regex pattern
	}

	for _, input := range tests {
		t.Run(input, func(t *testing.T) {
			_, _, _, err := ParseQuery(input)
			if err == nil {
				t.Errorf("expected error for input %q, got nil", input)
			}
		})
	}
}
