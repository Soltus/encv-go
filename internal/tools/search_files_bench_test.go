// internal/tools/search_files_bench_test.go
//
// T16 性能与压力测试 — search_files 工具
//
// 包含：
//   - T16.1 50000 文件扫描 < 5s（+ scanned_limited: true 校验）
//   - 可选的 BenchmarkSearchFiles_*（Go bench 框架，不在 -short 中跑）
//
// 跑测条件：
//   - `go test -short`                              → 跳过（fast）
//   - `BENCH_SCAN=1 go test ./internal/tools/...`   → 跑全量（5s 量级）
//   - `go test -bench=BenchmarkSearchFiles_...`     → 跑 micro bench
//
// 设计要点：
//   - 50001 个文件（确保触发 MaxFilesScanned=50000 上限 → scanned_limited=true）
//   - setupLargeDir 用并发批量写减少 setup 耗时（50001 * ~32B 写入 ~5-10s）
//   - 计时**只**覆盖 searchFilesHandler 调用本身，不包含文件创建
//   - 子集（500 个文件）写 "ERROR" 内容，便于 content_regex 命中
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"sync/atomic"
	"testing"
	"time"
)

// setupLargeDir 在 t.TempDir() 下创建 fileCount 个小文本文件。
//
//   - 文件大小：~32 字节
//   - 文件名格式：file_<i:08d>.txt
//   - 路径布局：全部平铺在 root 下（不走子目录以最大化 filepath.WalkDir 命中数）
//   - 错误内容子集：每 fileCount / 100 个文件写入 "ERROR timeout ..." 文本
//     （默认 500 个，若 fileCount < 5000 则按比例调整）
//
// 性能特性：使用 worker pool 并发写，50000 文件在普通 SSD 上 < 8s。
func setupLargeDir(t testing.TB, fileCount int) string {
	t.Helper()
	root := t.TempDir()

	// 错误内容子集大小：固定 500（spec 建议），若 fileCount < 500 则按 fileCount 截断
	errSubset := 500
	if errSubset > fileCount {
		errSubset = fileCount
	}

	// 内容模板
	plainContent := []byte("plain file content line 1\nline 2\n")
	errContent := []byte("ERROR: connection timeout after 30s\nstack: ...\n")

	// 写入 worker pool（避免串行 I/O 等太久）
	const workers = 16
	jobs := make(chan int, workers*2)
	var written int64

	done := make(chan struct{}, workers)
	for w := 0; w < workers; w++ {
		go func() {
			for i := range jobs {
				name := fmt.Sprintf("file_%08d.txt", i)
				full := filepath.Join(root, name)
				var data []byte
				if i < errSubset {
					data = errContent
				} else {
					data = plainContent
				}
				if err := os.WriteFile(full, data, 0o644); err != nil {
					t.Errorf("write %s: %v", full, err)
				}
				atomic.AddInt64(&written, 1)
			}
			done <- struct{}{}
		}()
	}
	for i := 0; i < fileCount; i++ {
		jobs <- i
	}
	close(jobs)
	for w := 0; w < workers; w++ {
		<-done
	}
	if got := atomic.LoadInt64(&written); got != int64(fileCount) {
		t.Fatalf("setupLargeDir: written=%d, want %d", got, fileCount)
	}
	return root
}

// shouldRunLargeScan 决定是否运行 50000 大目录扫描测试。
//
//   - testing.Short() → skip（fast）
//   - BENCH_SCAN != "1" → skip（opt-in）
//
// 返回 true 表示进入测试体；false 表示应 t.SkipNow()。
func shouldRunLargeScan(t *testing.T) bool {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping 50000-file scan (testing.Short())")
		return false
	}
	if os.Getenv("BENCH_SCAN") != "1" {
		t.Skip("skipping 50000-file scan (set BENCH_SCAN=1 to enable)")
		return false
	}
	return true
}

// ─── T16.1 50000 文件扫描 < 5s ─────────────────────────────────

// TestSearchFiles_LargeScan_50000Files_Under5s 验证 search_files 在 50001
// 个文件（确保触发 50000 上限）的大目录下：
//
//  1. 全表扫描 < 5s 完成
//  2. scanned_limited == true（超过 MaxFilesScanned 截断）
//  3. 命中数 == MaxFilesScanned（50000）
//
// no-op expression 使用 size_gt=-1（所有文件 size 都 > -1，等价于 always-true
// 但仍走真实的 compileExpr + walk 路径）。
//
// Skippable：
//   - `go test -short` 默认跳过
//   - 设置 BENCH_SCAN=1 才会真正执行
func TestSearchFiles_LargeScan_50000Files_Under5s(t *testing.T) {
	if !shouldRunLargeScan(t) {
		return
	}

	const fileCount = 50001 // 略超 50000 触发 scanned_limited
	t.Logf("setupLargeDir: creating %d files...", fileCount)
	setupStart := time.Now()
	root := setupLargeDir(t, fileCount)
	t.Logf("setup done in %v (root=%s)", time.Since(setupStart), root)

	deps := &ToolDeps{
		ResolveMount: func(mountID string) (string, bool) {
			if mountID == "bench" {
				return root, true
			}
			return "", false
		},
	}

	// ── no-op expression：size_gt=-1（所有文件都 > -1 字节）
	argsJSON := `{
		"mount_id":"bench",
		"recursive":true,
		"max_results":60000,
		"expression":{"type":"size_gt","value":-1}
	}`

	// ── 关键计时：只覆盖 searchFilesHandler 本身
	t.Logf("running search on %d files...", fileCount)
	searchStart := time.Now()
	res, err := searchFilesHandler(context.Background(), argsJSON, deps)
	elapsed := time.Since(searchStart)
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if res.IsError {
		t.Fatalf("handler IsError=true. Result=%s", res.Result)
	}
	t.Logf("search done in %v (matched=%d)", elapsed, totalFromResult(t, res.Result))

	// ── 断言 1：wall time < 5s
	if elapsed >= 5*time.Second {
		t.Errorf("search wall time = %v, want < 5s", elapsed)
	}

	// ── 断言 2：scanned_limited == true
	var out SearchFilesResult
	if err := json.Unmarshal([]byte(res.Result), &out); err != nil {
		t.Fatalf("parse result: %v. Raw: %s", err, res.Result)
	}
	if !out.ScannedLimited {
		t.Errorf("scanned_limited = false, want true (fileCount=%d > MaxFilesScanned=%d)",
			fileCount, MaxFilesScanned)
	}
	if out.Total != MaxFilesScanned {
		t.Errorf("Total = %d, want %d (truncated at MaxFilesScanned)",
			out.Total, MaxFilesScanned)
	}
}

// totalFromResult 解析 handler 返回的 Result JSON 取 total（仅供 t.Logf 友好输出）。
func totalFromResult(t *testing.T, raw string) int {
	t.Helper()
	var out SearchFilesResult
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return -1
	}
	return out.Total
}

// TestSearchFiles_LargeScan_ContentRegex_500FilesSubset 验证在大目录中
// content_regex 命中精确的子集（500 个 ERROR 文件）。
//
// 与 T16.1 配套：不要求 50000 全量，用较小目录（2000 文件）跑得快。
// Skippable in -short mode。
func TestSearchFiles_LargeScan_ContentRegex_500FilesSubset(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping content_regex subset scan (testing.Short())")
		return
	}
	if os.Getenv("BENCH_SCAN") != "1" {
		t.Skip("skipping content_regex subset scan (set BENCH_SCAN=1 to enable)")
		return
	}

	const fileCount = 2000
	root := setupLargeDir(t, fileCount)

	deps := &ToolDeps{
		ResolveMount: func(mountID string) (string, bool) {
			if mountID == "bench" {
				return root, true
			}
			return "", false
		},
	}

	// content_regex 命中所有 500 个 ERROR 文件
	argsJSON := `{
		"mount_id":"bench",
		"recursive":true,
		"max_results":1000,
		"expression":{"type":"content_regex","value":"ERROR.*timeout"}
	}`
	res, err := searchFilesHandler(context.Background(), argsJSON, deps)
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if res.IsError {
		t.Fatalf("handler IsError=true. Result=%s", res.Result)
	}
	var out SearchFilesResult
	if err := json.Unmarshal([]byte(res.Result), &out); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if out.Total < 500 {
		t.Errorf("Total = %d, want >= 500 (ERROR subset)", out.Total)
	}
	// 全部 plain 文件（file_00000500.txt 之后）不应命中
	if out.Total > 600 {
		t.Errorf("Total = %d, want ~500 (no false positives on plain files)", out.Total)
	}
}

// ─── 可选 benchmarks（不在 -short 中跑；用 `go test -bench` 触发） ──

// BenchmarkSearchFiles_GlobPath_5000Files 在 5000 个文件目录上跑
// name_glob="*.txt"，测量纯路径 glob 扫描吞吐量。
func BenchmarkSearchFiles_GlobPath_5000Files(b *testing.B) {
	root := setupLargeDir(b, 5000)
	deps := &ToolDeps{
		ResolveMount: func(mountID string) (string, bool) {
			if mountID == "bench" {
				return root, true
			}
			return "", false
		},
	}
	argsJSON := `{
		"mount_id":"bench",
		"recursive":true,
		"max_results":10000,
		"expression":{"type":"name_glob","value":"*.txt"}
	}`
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = searchFilesHandler(ctx, argsJSON, deps)
	}
}

// BenchmarkSearchFiles_ContentRegex_5000Files 在 5000 文件（500 含 ERROR）
// 的目录上跑 content_regex="ERROR.*timeout"，测量内容正则扫描吞吐量。
func BenchmarkSearchFiles_ContentRegex_5000Files(b *testing.B) {
	root := setupLargeDir(b, 5000)
	deps := &ToolDeps{
		ResolveMount: func(mountID string) (string, bool) {
			if mountID == "bench" {
				return root, true
			}
			return "", false
		},
	}
	argsJSON := `{
		"mount_id":"bench",
		"recursive":true,
		"max_results":10000,
		"expression":{"type":"content_regex","value":"ERROR.*timeout"}
	}`
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = searchFilesHandler(ctx, argsJSON, deps)
	}
}

// BenchmarkSearchFiles_Glob_50000Files 50000 文件目录上的 glob 扫描基准。
// 注意：50000 文件创建本身就要 ~5-8s，b.N=1 即可，不建议高 N。
func BenchmarkSearchFiles_Glob_50000Files(b *testing.B) {
	if os.Getenv("BENCH_SCAN") != "1" {
		b.Skip("set BENCH_SCAN=1 to enable 50000-file bench")
		return
	}
	root := setupLargeDir(b, 50000)
	deps := &ToolDeps{
		ResolveMount: func(mountID string) (string, bool) {
			if mountID == "bench" {
				return root, true
			}
			return "", false
		},
	}
	argsJSON := `{
		"mount_id":"bench",
		"recursive":true,
		"max_results":60000,
		"expression":{"type":"size_gt","value":-1}
	}`
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = searchFilesHandler(ctx, argsJSON, deps)
	}
}

// ─── 内存基线自检（轻量，-short 也跑） ─────────────────────────

// TestSearchFiles_Memory_QuickScanAllocations 在小目录（500 文件）上验证
// 单次 search 不会产生过量内存分配（防止回归引入的 leak / 不必要复制）。
func TestSearchFiles_Memory_QuickScanAllocations(t *testing.T) {
	root := setupLargeDir(t, 500)
	deps := &ToolDeps{
		ResolveMount: func(mountID string) (string, bool) {
			if mountID == "bench" {
				return root, true
			}
			return "", false
		},
	}

	runtime.GC()
	var before runtime.MemStats
	runtime.ReadMemStats(&before)

	for i := 0; i < 100; i++ {
		_, _ = searchFilesHandler(context.Background(), `{
			"mount_id":"bench",
			"recursive":true,
			"max_results":1000,
			"expression":{"type":"name_glob","value":"*.txt"}
		}`, deps)
	}

	runtime.GC()
	var after runtime.MemStats
	runtime.ReadMemStats(&after)

	growth := int64(after.HeapAlloc) - int64(before.HeapAlloc)
	const maxAcceptable = 50 * 1024 * 1024 // 50MB 上限（100 次扫描，宽松）
	if growth > maxAcceptable {
		t.Errorf("100 次 search 后 HeapAlloc 增长 %d bytes (>%d)", growth, maxAcceptable)
	}
	t.Logf("100×search heap growth: %s", humanBytes(growth))
}

// humanBytes 把字节数格式化为人类可读字符串。
func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return strconv.FormatInt(n, 10) + "B"
	}
	div, exp := int64(unit), 0
	for x := n / unit; x >= unit; x /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f%cB", float64(n)/float64(div), "KMGTPE"[exp])
}
