//go:build !race
// +build !race

// 性能测试文件仅在非 -race 模式下运行。
//
// 为什么：Go race detector 会拦截所有内存访问，导致 BulkInsert / Search 性能
// 退化 40-50x（实测 1w 条 396ms → 18.5s）。这是 race detector 的预期行为，
// 不是性能 regression。性能指标应该用 `go test -count=1` 验证（无 -race）。
//
// 使用：
//   go test -count=1 ./internal/fts/                          # 跑所有测试 + 性能
//   go test -race -count=1 ./internal/fts/                    # 跑所有测试，跳过性能
//   go test -count=1 -run TestFileIndex_BulkInsertPerformance ./internal/fts/  # 单独跑性能

package fts

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestFileIndex_BulkInsertPerformance(t *testing.T) {
	idx := newTestIndex(t)
	ctx := context.Background()

	const N = 10000
	entries := make([]FileEntry, N)
	for i := 0; i < N; i++ {
		entries[i] = FileEntry{
			Path:    filepath.ToSlash(filepath.Join("/d", "f", "file_"+intToStr(i)+".txt")),
			Name:    "file_" + intToStr(i) + ".txt",
			Size:    int64(i * 100),
			Content: "content for file " + intToStr(i) + " with keyword 在线 and 高清",
		}
	}

	start := time.Now()
	if err := idx.BulkInsert(ctx, entries); err != nil {
		t.Fatalf("BulkInsert: %v", err)
	}
	elapsed := time.Since(start)
	t.Logf("BulkInsert %d entries: %v (%.2f us/entry)", N, elapsed, float64(elapsed.Microseconds())/float64(N))

	// 性能目标：1w 条目应在 1s 内
	if elapsed > time.Second {
		t.Errorf("BulkInsert too slow: %v > 1s for %d entries", elapsed, N)
	}

	// 搜索性能
	searchStart := time.Now()
	results, err := idx.Search(ctx, "在线", SearchOptions{Limit: 100})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	searchElapsed := time.Since(searchStart)
	t.Logf("Search '在线' returned %d results in %v", len(results), searchElapsed)

	// 性能目标：搜索应 <100ms
	if searchElapsed > 100*time.Millisecond {
		t.Errorf("Search too slow: %v > 100ms", searchElapsed)
	}
}
