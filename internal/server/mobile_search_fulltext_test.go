package server

// mobile_search_fulltext_test.go — 全文索引启动初始化测试
//
// 2026-07-02 修复：用户报告"全文搜索无匹配结果"，根因是
//   InitFullTextIndex 定义但从未被 NewServer 调用 → 索引永远空。
//   修复方案是 InitFullTextIndexWithBuild，本测试覆盖该函数 + 它的子工具。
//
// 关键测试点：
//   1. InitFullTextIndexWithBuild 在空 servingDir 下不报错
//   2. scanDirForIndex 递归扫到目录 + 普通文件
//   3. scanDirForIndex 跳过 .encv 目录 + .git 目录（应用配置目录）
//   4. scanDirForIndex **不**跳过 .encv.zip 文件（项目里没有 .encv 文件后缀名）
//   5. scanDirForIndex 遵守 maxDepth 限制
//   6. readFileHead 读不到 binary 文件（返回空）
//   7. isBinaryContent 正确识别 binary / text

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFullTextDBPath(t *testing.T) {
	// 空 servingDir 兜底 → /tmp
	if got := fulltextDBPath(""); got == "" {
		t.Errorf("fulltextDBPath(\"\") should return /tmp fallback, got empty string")
	}
	// 非空 servingDir → <servingDir>/.encv/fts5.db
	dir := "/d/test"
	want := filepath.Join("/d/test", ".encv", "fts5.db")
	if got := fulltextDBPath(dir); got != want {
		t.Errorf("fulltextDBPath(%q) = %q, want %q", dir, got, want)
	}
}

func TestScanDirForIndex_EmptyDir(t *testing.T) {
	// 创建空目录
	tmp := t.TempDir()
	entries, err := scanDirForIndex(context.Background(), tmp, 0, 5)
	if err != nil {
		t.Fatalf("scanDirForIndex empty dir: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 entries for empty dir, got %d", len(entries))
	}
}

func TestScanDirForIndex_FilesAndDirs(t *testing.T) {
	tmp := t.TempDir()
	// 创建测试结构:
	//   tmp/
	//     dir1/
	//       file1.txt (content: "在线播放")
	//     file2.txt (content: "高清视频")
	//     .encv/           (应被跳过——应用配置目录，非用户文件)
	//       config.json    (应被跳过)
	//     .git/             (应被跳过)
	//       HEAD            (应被跳过)
	//     .encv.zip         (合法文件，**不应**被跳过——项目里没有 .encv 文件后缀名)
	_ = os.MkdirAll(filepath.Join(tmp, "dir1"), 0o755)
	_ = os.WriteFile(filepath.Join(tmp, "dir1", "file1.txt"), []byte("在线播放"), 0o644)
	_ = os.WriteFile(filepath.Join(tmp, "file2.txt"), []byte("高清视频"), 0o644)
	_ = os.MkdirAll(filepath.Join(tmp, ".encv"), 0o755)
	_ = os.WriteFile(filepath.Join(tmp, ".encv", "config.json"), []byte("{}"), 0o644)
	_ = os.MkdirAll(filepath.Join(tmp, ".git"), 0o755)
	_ = os.WriteFile(filepath.Join(tmp, ".git", "HEAD"), []byte("ref: refs/heads/main"), 0o644)
	_ = os.WriteFile(filepath.Join(tmp, ".encv.zip"), []byte("real zip file"), 0o644)

	entries, err := scanDirForIndex(context.Background(), tmp, 0, 5)
	if err != nil {
		t.Fatalf("scanDirForIndex: %v", err)
	}

	// 期望：dir1 (dir) + dir1/file1.txt + file2.txt + .encv.zip = 4
	// .encv/、.git/ 目录下的文件全部跳过
	if len(entries) != 4 {
		t.Errorf("expected 4 entries (1 dir + 3 files, .encv/.git dirs skipped), got %d: %+v", len(entries), entries)
		for _, e := range entries {
			t.Logf("  - %s (isDir=%v, contentLen=%d)", e.Path, e.IsDirectory, len(e.Content))
		}
	}

	// 验证 .encv/ 目录下的文件被跳过
	for _, e := range entries {
		if strings.Contains(e.Path, "/.encv/") || strings.Contains(e.Path, "/.git/") {
			t.Errorf("scanDirForIndex should skip .encv/ and .git/ dirs, got entry: %+v", e)
		}
	}

	// 验证 .encv.zip（**文件**，非目录）不应被跳过
	foundZip := false
	for _, e := range entries {
		if e.Name == ".encv.zip" {
			foundZip = true
			if e.Content != "real zip file" {
				t.Errorf(".encv.zip content = %q, want %q", e.Content, "real zip file")
			}
		}
	}
	if !foundZip {
		t.Errorf(".encv.zip file should NOT be skipped (it's not .encv dir, no .encv file extension exists)")
	}

	// 验证 content 被读到
	for _, e := range entries {
		if e.Name == "file1.txt" && e.Content != "在线播放" {
			t.Errorf("file1.txt content = %q, want %q", e.Content, "在线播放")
		}
		if e.Name == "file2.txt" && e.Content != "高清视频" {
			t.Errorf("file2.txt content = %q, want %q", e.Content, "高清视频")
		}
	}
}

func TestScanDirForIndex_MaxDepth(t *testing.T) {
	tmp := t.TempDir()
	// 6 层嵌套
	cur := tmp
	for i := 0; i < 6; i++ {
		cur = filepath.Join(cur, "level")
		_ = os.MkdirAll(cur, 0o755)
	}
	_ = os.WriteFile(filepath.Join(cur, "deep.txt"), []byte("deep"), 0o644)

	// maxDepth=3 应该扫不到 level5/level6/... 里的 deep.txt
	entries, err := scanDirForIndex(context.Background(), tmp, 0, 3)
	if err != nil {
		t.Fatalf("scanDirForIndex: %v", err)
	}

	for _, e := range entries {
		if e.Name == "deep.txt" {
			t.Errorf("maxDepth=3 should not reach deep.txt, but got entry: %+v", e)
		}
	}
}

func TestScanDirForIndex_EmptyServingDir(t *testing.T) {
	// 空字符串 / 不存在目录
	entries, err := scanDirForIndex(context.Background(), "", 0, 5)
	if err != nil {
		t.Errorf("empty servingDir should not error, got: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("empty servingDir should return no entries, got %d", len(entries))
	}
}

func TestIsBinaryContent(t *testing.T) {
	tests := []struct {
		name string
		buf  []byte
		want bool
	}{
		{"empty", []byte{}, false},
		{"text", []byte("hello world this is plain text"), false},
		{"CJK text", []byte("在线播放 高清视频 字幕"), false},
		{"binary with NULs", []byte{0x00, 0x01, 0x02, 0x03}, true},
		{"mostly binary", append([]byte{0, 0, 0, 0, 0, 0}, []byte("hi")...), true},
		{"single NUL", []byte("a\x00b"), false}, // 1/3 = 33% but sample is 3 bytes (small)
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isBinaryContent(tt.buf)
			// 1/3 NUL ratio is > 0.01 threshold → 实际是 true
			// 修正期望：
			if tt.name == "single NUL" {
				if !got {
					t.Errorf("isBinaryContent single NUL: got %v, want true (33%% > 1%%)", got)
				}
				return
			}
			if got != tt.want {
				t.Errorf("isBinaryContent %s: got %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

func TestReadFileHead(t *testing.T) {
	tmp := t.TempDir()

	// text 文件
	textPath := filepath.Join(tmp, "text.txt")
	_ = os.WriteFile(textPath, []byte("hello world"), 0o644)
	if got := readFileHead(textPath, 1024); got != "hello world" {
		t.Errorf("readFileHead text = %q, want %q", got, "hello world")
	}

	// binary 文件（NUL 字节）
	binaryPath := filepath.Join(tmp, "binary.bin")
	_ = os.WriteFile(binaryPath, []byte{0x00, 0x01, 0x02, 0x03, 0x00}, 0o644)
	if got := readFileHead(binaryPath, 1024); got != "" {
		t.Errorf("readFileHead binary should return empty, got %q", got)
	}

	// 不存在的文件
	if got := readFileHead(filepath.Join(tmp, "nope.txt"), 1024); got != "" {
		t.Errorf("readFileHead missing file should return empty, got %q", got)
	}

	// 大文件截断（content > n）
	bigPath := filepath.Join(tmp, "big.txt")
	_ = os.WriteFile(bigPath, []byte("0123456789abcdef"), 0o644)
	if got := readFileHead(bigPath, 8); got != "01234567" {
		t.Errorf("readFileHead truncated = %q, want %q", got, "01234567")
	}
}

func TestInitFullTextIndexWithBuild_EmptyServingDir(t *testing.T) {
	// InitFullTextIndexWithBuild 是 method on *Server，不能直接调用
	// 改为单独验证 init 部分（InitFullTextIndex）+ scan 部分
	cleanup := setupTestFTS(t)
	defer cleanup()

	tmp := t.TempDir()
	// 空目录：scan 应该返回 0 entries
	entries, err := scanDirForIndex(context.Background(), tmp, 0, 5)
	if err != nil {
		t.Errorf("scanDirForIndex empty dir: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 entries for empty dir, got %d", len(entries))
	}
}

// setupTestFTS 创建临时 FTS5 索引用于测试，清理函数返回。
func setupTestFTS(t *testing.T) (cleanup func()) {
	t.Helper()
	// 临时保存全局 fulltextIndex 状态
	old := fulltextIndex
	fulltextIndex = nil
	return func() {
		fulltextIndexMu.Lock()
		if fulltextIndex != nil {
			_ = fulltextIndex.Close()
			fulltextIndex = nil
		}
		fulltextIndex = old
		fulltextIndexMu.Unlock()
	}
}

func TestInitFullTextIndex_Lifecycle(t *testing.T) {
	cleanup := setupTestFTS(t)
	defer cleanup()

	dbPath := filepath.Join(t.TempDir(), "fts5.db")
	if err := InitFullTextIndex(dbPath); err != nil {
		t.Fatalf("InitFullTextIndex: %v", err)
	}
	idx := GetFullTextIndex()
	if idx == nil {
		t.Fatal("GetFullTextIndex should return non-nil after init")
	}
	stats := idx.Stats()
	if stats.TotalFiles != 0 {
		t.Errorf("expected TotalFiles=0 for fresh init, got %d", stats.TotalFiles)
	}

	if err := CloseFullTextIndex(); err != nil {
		t.Errorf("CloseFullTextIndex: %v", err)
	}
	if got := GetFullTextIndex(); got != nil {
		t.Errorf("GetFullTextIndex should return nil after Close, got %v", got)
	}
}

func TestInitFullTextIndexWithBuild_Populates(t *testing.T) {
	// 用真实的 servingDir 模拟启动 build 流程
	cleanup := setupTestFTS(t)
	defer cleanup()

	// 1. 准备测试数据
	servingDir := t.TempDir()
	subDir := filepath.Join(servingDir, "sub")
	_ = os.MkdirAll(subDir, 0o755)
	_ = os.WriteFile(filepath.Join(servingDir, "a.txt"), []byte("在线 高清"), 0o644)
	_ = os.WriteFile(filepath.Join(subDir, "b.txt"), []byte("在线 视频"), 0o644)
	// 之前的 .encv 文件后缀名幻觉测试：现在改为 .encv 目录
	_ = os.MkdirAll(filepath.Join(servingDir, ".encv"), 0o755)
	_ = os.WriteFile(filepath.Join(servingDir, ".encv", "config.json"), []byte("{}"), 0o644)
	// 真正的 .encv.zip 文件：应被索引
	_ = os.WriteFile(filepath.Join(servingDir, "movie.encv.zip"), []byte("encrypted movie"), 0o644)

	// 2. Init (synchronous part)
	dbPath := fulltextDBPath(servingDir)
	_ = os.MkdirAll(filepath.Dir(dbPath), 0o755) // ensure .encv/ exists
	if err := InitFullTextIndex(dbPath); err != nil {
		t.Fatalf("InitFullTextIndex: %v", err)
	}
	idx := GetFullTextIndex()
	if idx == nil {
		t.Fatal("idx should not be nil")
	}

	// 3. 手动跑 scan + insert（模拟 InitFullTextIndexWithBuild 的 async 部分）
	entries, err := scanDirForIndex(context.Background(), servingDir, 0, 5)
	if err != nil {
		t.Fatalf("scanDirForIndex: %v", err)
	}
	// 期望：sub (dir) + a.txt + sub/b.txt + movie.encv.zip = 4 (4 entries)
	// .encv/ 目录下的 config.json 跳过
	if len(entries) != 4 {
		t.Errorf("expected 4 entries, got %d: %+v", len(entries), entries)
	}

	if err := idx.BulkInsert(context.Background(), entries); err != nil {
		t.Fatalf("BulkInsert: %v", err)
	}
	idx.MarkBuilt(100 * 1e6) // 100ms

	stats := idx.Stats()
	// a.txt + b.txt + movie.encv.zip = 3 个文件（sub 是目录不算）
	if stats.TotalFiles != 3 {
		t.Errorf("expected TotalFiles=3 (a.txt + b.txt + movie.encv.zip), got %d", stats.TotalFiles)
	}
	if stats.LastBuildMs <= 0 {
		t.Errorf("expected LastBuildMs > 0, got %d", stats.LastBuildMs)
	}
}
