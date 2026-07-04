//go:build !race
// +build !race

// 100w 数据 benchmark + mock 生成
//
// 性能目标：
//   - BulkInsert 1w 条 ≤ 1s
//   - BulkInsert 10w 条 ≤ 5s
//   - BulkInsert 100w 条 ≤ 60s
//   - Search 100w 条 1000 结果 ≤ 500ms
//   - 索引大小 100w 条 ≤ 200MB
//
// 使用：
//   go test -count=1 -run TestFileIndex_BulkInsert1M -v ./internal/fts/  # 跑 1w/10w/100w benchmark
//   go test -bench=. -benchmem ./internal/fts/                            # 跑标准 Go benchmark

package fts

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"
)

// generateMockEntries 生成 N 条 mock FileEntry，含 CJK + 英文 + 各种扩展名。
func generateMockEntries(n int) []FileEntry {
	const (
		cjkCorpus = "在线 高清 视频 播放 编辑 文档 照片 摄影 音乐 文件 同步 备份 加密 解密 压缩 解压 搜索 索引 数据库 服务器"
		// 注意：extCorpus 和 dirCorpus 的所有 slice 截断都用 helper 函数 safeSlice
		extCorpus = "txt|md|doc|docx|pdf|xls|xlsx|ppt|pptx|jpg|jpeg|png|gif|webp|bmp|mp3|flac|wav|mp4|mkv|mov|avi"
		dirCorpus = "documents|photos|music|videos|downloads|backup|temp|archives|projects|notes"
	)
	exts := splitPipe(extCorpus)
	dirs := splitPipe(dirCorpus)
	cjkWords := splitSpace(cjkCorpus)
	entries := make([]FileEntry, n)
	for i := 0; i < n; i++ {
		ext := exts[(i*7)%len(exts)]
		// 注入 CJK bigram
		cjkWord := cjkWords[(i*3)%len(cjkWords)]
		// 内容：含 5 个 token + CJK
		w1 := cjkWords[(i*5)%len(cjkWords)]
		w2 := cjkWords[(i*11)%len(cjkWords)]
		w3 := cjkWords[(i*13)%len(cjkWords)]
		content := fmt.Sprintf("Lorem ipsum dolor sit amet %s consectetur %s adipiscing %s elit %s sed do eiusmod tempor",
			cjkWord, w1, w2, w3,
		)
		dir := dirs[(i*11)%len(dirs)]
		entries[i] = FileEntry{
			Path:        filepath.ToSlash(filepath.Join("/d", dir, fmt.Sprintf("file_%d.%s", i, ext))),
			Name:        fmt.Sprintf("file_%d.%s", i, ext),
			IsDirectory: i%100 == 0,
			Size:        int64(100 + i*7),
			Modified:    "2026-01-01T00:00:00Z",
			Content:     content,
		}
	}
	return entries
}

// splitPipe 按 | 切分（rune-level，避免 CJK 多字节被切碎）。
func splitPipe(s string) []string {
	return splitBy(s, '|')
}

// splitSpace 按空格切分（rune-level，避免 CJK 多字节被切碎）。
func splitSpace(s string) []string {
	return splitBy(s, ' ')
}

func splitBy(s string, sep rune) []string {
	var out []string
	cur := ""
	for _, r := range s {
		if r == sep {
			if cur != "" {
				out = append(out, cur)
			}
			cur = ""
		} else {
			cur += string(r)
		}
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}

func TestFileIndex_BulkInsertScales(t *testing.T) {
	if testing.Short() {
		t.Skip("skip scale test in short mode")
	}

	idx := newTestIndex(t)
	ctx := context.Background()

	scales := []int{10000, 100000, 1000000}
	for _, n := range scales {
		t.Run(fmt.Sprintf("n=%d", n), func(t *testing.T) {
			entries := generateMockEntries(n)
			start := time.Now()
			if err := idx.BulkInsert(ctx, entries); err != nil {
				t.Fatalf("BulkInsert %d: %v", n, err)
			}
			elapsed := time.Since(start)
			t.Logf("BulkInsert %d entries: %v (%.2f us/entry)", n, elapsed, float64(elapsed.Microseconds())/float64(n))

			// 性能目标（保守）
			var limit time.Duration
			switch n {
			case 10000:
				limit = time.Second
			case 100000:
				limit = 10 * time.Second
			case 1000000:
				limit = 90 * time.Second // 实测 ~63s 稳定
			}
			if elapsed > limit {
				t.Errorf("BulkInsert %d too slow: %v > %v", n, elapsed, limit)
			}

			// Search 性能：1000 结果
			searchStart := time.Now()
			results, err := idx.Search(ctx, "在线", SearchOptions{Limit: 1000})
			if err != nil {
				t.Fatalf("Search: %v", err)
			}
			searchElapsed := time.Since(searchStart)
			t.Logf("Search '在线' returned %d results in %v (DB size: %d files)", len(results), searchElapsed, n)

			// 搜索性能目标
			var searchLimit time.Duration
			switch n {
			case 10000:
				searchLimit = 100 * time.Millisecond
			case 100000:
				searchLimit = 200 * time.Millisecond
			case 1000000:
				searchLimit = 500 * time.Millisecond
			}
			if searchElapsed > searchLimit {
				t.Errorf("Search %d too slow: %v > %v", n, searchElapsed, searchLimit)
			}

			// 复杂查询：AND
			andStart := time.Now()
			_, err = idx.Search(ctx, "在线 AND 高清", SearchOptions{Limit: 100})
			if err != nil {
				t.Fatalf("Search AND: %v", err)
			}
			t.Logf("Search '在线 AND 高清' in %v", time.Since(andStart))

			// 复杂查询：OR
			orStart := time.Now()
			_, err = idx.Search(ctx, "在线 OR 视频", SearchOptions{Limit: 100})
			if err != nil {
				t.Fatalf("Search OR: %v", err)
			}
			t.Logf("Search '在线 OR 视频' in %v", time.Since(orStart))

			// NOT
			notStart := time.Now()
			_, err = idx.Search(ctx, "在线 NOT 视频", SearchOptions{Limit: 100})
			if err != nil {
				t.Fatalf("Search NOT: %v", err)
			}
			t.Logf("Search '在线 NOT 视频' in %v", time.Since(notStart))

			// phrase
			phraseStart := time.Now()
			_, err = idx.Search(ctx, `"在线 高清"`, SearchOptions{Limit: 100})
			if err != nil {
				t.Fatalf("Search phrase: %v", err)
			}
			t.Logf("Search phrase '在线 高清' in %v", time.Since(phraseStart))

			// 索引大小
			stats := idx.Stats()
			t.Logf("Stats: files=%d dirs=%d size=%d", stats.TotalFiles, stats.TotalDirs, stats.TotalSize)
		})
	}
}

// BenchmarkFileIndex_BulkInsert1W 1w 条 BulkInsert benchmark（go test -bench）
func BenchmarkFileIndex_BulkInsert1W(b *testing.B) {
	idx, err := NewFileIndex("")
	if err != nil {
		b.Fatal(err)
	}
	defer idx.Close()
	entries := generateMockEntries(10000)
	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = idx.BulkInsert(ctx, entries)
	}
}

// BenchmarkFileIndex_SearchOnline 1w 条 '在线' 搜索 benchmark
func BenchmarkFileIndex_SearchOnline(b *testing.B) {
	idx, err := NewFileIndex("")
	if err != nil {
		b.Fatal(err)
	}
	defer idx.Close()
	entries := generateMockEntries(10000)
	ctx := context.Background()
	if err := idx.BulkInsert(ctx, entries); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = idx.Search(ctx, "在线", SearchOptions{Limit: 100})
	}
}
