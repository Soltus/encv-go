package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// makeTestMount 在 t.TempDir() 下创建一组测试文件，返回 ResolveMount 闭包。
// 文件结构：
//
//	root/
//	  big.mp4            200 MB
//	  small.txt          1 KB
//	  Movies/
//	    ep1.mkv          10 MB
//	    ep2.mkv          50 MB
//	    err.log          4 KB  (content: "ERROR: connection timeout after 30s")
//	    a.mp4            12 MB
//	  logs/
//	    app.log          5 MB
//	  trash/old.txt      1 KB
func makeTestMount(t *testing.T) (string, *ToolDeps) {
	t.Helper()
	root := t.TempDir()

	writeFile := func(rel string, size int, content string) {
		full := filepath.Join(root, rel)
		_ = os.MkdirAll(filepath.Dir(full), 0o755)
		f, err := os.Create(full)
		if err != nil {
			t.Fatalf("create %s: %v", rel, err)
		}
		defer f.Close()
		if content != "" {
			_, _ = f.WriteString(content)
		}
		// padding
		if size > 0 {
			_, _ = f.Write(make([]byte, size))
		}
	}

	writeFile("big.mp4", 200*1024*1024, "")
	writeFile("small.txt", 1024, "hello world")
	writeFile("Movies/ep1.mkv", 10*1024*1024, "")
	writeFile("Movies/ep2.mkv", 50*1024*1024, "")
	writeFile("Movies/err.log", 4096, "INFO: start\nERROR: connection timeout after 30s\nINFO: end\n")
	writeFile("Movies/a.mp4", 12*1024*1024, "")
	writeFile("logs/app.log", 5*1024*1024, "INFO ok")
	writeFile("trash/old.txt", 1024, "stale")

	deps := &ToolDeps{
		ResolveMount: func(mountID string) (string, bool) {
			if mountID == "test" {
				return root, true
			}
			return "", false
		},
	}
	return root, deps
}

func runSearch(t *testing.T, deps *ToolDeps, argsJSON string) SearchFilesResult {
	t.Helper()
	res, err := searchFilesHandler(context.Background(), argsJSON, deps)
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if res.IsError {
		t.Fatalf("handler returned IsError=true. Result = %s", res.Result)
	}
	var out SearchFilesResult
	if err := json.Unmarshal([]byte(res.Result), &out); err != nil {
		t.Fatalf("parse result: %v. Raw: %s", err, res.Result)
	}
	return out
}

// ─── 1. TestSearchFiles_NameGlob ───────────────────────────────

func TestSearchFiles_NameGlob(t *testing.T) {
	_, deps := makeTestMount(t)
	out := runSearch(t, deps, `{
		"mount_id":"test",
		"expression":{"type":"name_glob","value":"*.mp4"}
	}`)
	if out.Total == 0 {
		t.Fatal("expected at least 1 match")
	}
	for _, m := range out.Matches {
		if !strings.HasSuffix(m.Path, ".mp4") {
			t.Errorf("path %s is not .mp4", m.Path)
		}
	}
	// 应该有 big.mp4 + Movies/a.mp4
	if out.Total < 2 {
		t.Errorf("Total = %d, want >= 2", out.Total)
	}
}

// ─── 2. TestSearchFiles_NameRegex ──────────────────────────────

func TestSearchFiles_NameRegex(t *testing.T) {
	_, deps := makeTestMount(t)
	out := runSearch(t, deps, `{
		"mount_id":"test",
		"expression":{"type":"name_regex","value":"ep[0-9]+\\.mkv"}
	}`)
	if out.Total != 2 {
		t.Errorf("Total = %d, want 2 (ep1.mkv, ep2.mkv)", out.Total)
	}
	for _, m := range out.Matches {
		if !strings.HasSuffix(m.Path, ".mkv") {
			t.Errorf("unexpected match: %s", m.Path)
		}
	}
}

// ─── 3. TestSearchFiles_ContentRegex ───────────────────────────

func TestSearchFiles_ContentRegex(t *testing.T) {
	_, deps := makeTestMount(t)
	out := runSearch(t, deps, `{
		"mount_id":"test",
		"expression":{"type":"content_regex","value":"ERROR.*timeout"}
	}`)
	if out.Total != 1 {
		t.Fatalf("Total = %d, want 1 (Movies/err.log)", out.Total)
	}
	if !strings.HasSuffix(out.Matches[0].Path, "err.log") {
		t.Errorf("path = %s, want err.log", out.Matches[0].Path)
	}
}

// ─── 4. TestSearchFiles_SizeGt / SizeLt / SizeEq ───────────────

func TestSearchFiles_SizeGt(t *testing.T) {
	_, deps := makeTestMount(t)
	out := runSearch(t, deps, `{
		"mount_id":"test",
		"expression":{"type":"size_gt","value":104857600}
	}`)
	if out.Total < 1 {
		t.Errorf("Total = %d, want >= 1 (big.mp4 is 200MB)", out.Total)
	}
}

func TestSearchFiles_SizeLt(t *testing.T) {
	_, deps := makeTestMount(t)
	out := runSearch(t, deps, `{
		"mount_id":"test",
		"expression":{"type":"size_lt","value":10000}
	}`)
	if out.Total < 1 {
		t.Errorf("Total = %d, want >= 1 (small.txt is 1KB)", out.Total)
	}
}

func TestSearchFiles_SizeEq(t *testing.T) {
	_, deps := makeTestMount(t)
	// 创建一个精确 4096 字节的文件
	root, _ := deps.ResolveMount("test")
	exact := filepath.Join(root, "exact.bin")
	if err := os.WriteFile(exact, make([]byte, 4096), 0o644); err != nil {
		t.Fatalf("create exact.bin: %v", err)
	}
	out := runSearch(t, deps, `{
		"mount_id":"test",
		"expression":{"type":"size_eq","value":4096}
	}`)
	if out.Total < 1 {
		t.Errorf("Total = %d, want >= 1 (exact.bin is 4096 bytes)", out.Total)
	}
}

// ─── 5. TestSearchFiles_MtimeAfter / MtimeBefore ───────────────

func TestSearchFiles_MtimeAfter(t *testing.T) {
	_, deps := makeTestMount(t)
	// 用 1970 时间，所有文件都应该 After
	out := runSearch(t, deps, `{
		"mount_id":"test",
		"expression":{"type":"mtime_after","value":"1970-01-01T00:00:00Z"}
	}`)
	if out.Total == 0 {
		t.Error("Total = 0, want all files")
	}
}

func TestSearchFiles_MtimeBefore(t *testing.T) {
	_, deps := makeTestMount(t)
	// 用未来时间，所有文件都 Before
	future := time.Now().Add(24 * time.Hour).Format(time.RFC3339)
	out := runSearch(t, deps, `{
		"mount_id":"test",
		"expression":{"type":"mtime_before","value":"`+future+`"}
	}`)
	if out.Total == 0 {
		t.Error("Total = 0, want all files")
	}
}

// ─── 6. TestSearchFiles_ExtEq ──────────────────────────────────

func TestSearchFiles_ExtEq(t *testing.T) {
	_, deps := makeTestMount(t)
	out := runSearch(t, deps, `{
		"mount_id":"test",
		"expression":{"type":"ext_eq","value":"log"}
	}`)
	if out.Total < 1 {
		t.Errorf("Total = %d, want >= 1 (Movies/err.log, logs/app.log)", out.Total)
	}
	for _, m := range out.Matches {
		if m.Ext != "log" {
			t.Errorf("path %s ext = %s, want log", m.Path, m.Ext)
		}
	}
}

// ─── 7. TestSearchFiles_PathContains / PathNotContains ──────────

func TestSearchFiles_PathContains(t *testing.T) {
	_, deps := makeTestMount(t)
	out := runSearch(t, deps, `{
		"mount_id":"test",
		"expression":{"type":"path_contains","value":"Movies"}
	}`)
	if out.Total < 3 {
		t.Errorf("Total = %d, want >= 3 (ep1.mkv, ep2.mkv, err.log, a.mp4)", out.Total)
	}
}

func TestSearchFiles_PathNotContains(t *testing.T) {
	_, deps := makeTestMount(t)
	out := runSearch(t, deps, `{
		"mount_id":"test",
		"expression":{"type":"path_not_contains","value":"trash"}
	}`)
	for _, m := range out.Matches {
		if strings.Contains(m.Path, "trash") {
			t.Errorf("path_not_contains=trash, but matched %s", m.Path)
		}
	}
}

// ─── 8. TestSearchFiles_And_ShortCircuit ────────────────────────

func TestSearchFiles_And_ShortCircuit(t *testing.T) {
	_, deps := makeTestMount(t)
	// AND: name_glob *.mp4 AND size_gt 100MB
	out := runSearch(t, deps, `{
		"mount_id":"test",
		"expression":{"type":"and","children":[
			{"type":"name_glob","value":"*.mp4"},
			{"type":"size_gt","value":104857600}
		]}
	}`)
	if out.Total != 1 {
		t.Errorf("Total = %d, want 1 (big.mp4 only)", out.Total)
	}
	if !strings.HasSuffix(out.Matches[0].Path, "big.mp4") {
		t.Errorf("path = %s, want big.mp4", out.Matches[0].Path)
	}
}

// ─── 9. TestSearchFiles_Or_ShortCircuit ────────────────────────

func TestSearchFiles_Or_ShortCircuit(t *testing.T) {
	_, deps := makeTestMount(t)
	// OR: name_glob *.log OR name_glob *.txt
	out := runSearch(t, deps, `{
		"mount_id":"test",
		"expression":{"type":"or","children":[
			{"type":"name_glob","value":"*.log"},
			{"type":"name_glob","value":"*.txt"}
		]}
	}`)
	if out.Total < 2 {
		t.Errorf("Total = %d, want >= 2 (logs + txts)", out.Total)
	}
}

// ─── 10. TestSearchFiles_Not / Nested ──────────────────────────

func TestSearchFiles_Not(t *testing.T) {
	_, deps := makeTestMount(t)
	out := runSearch(t, deps, `{
		"mount_id":"test",
		"expression":{"type":"not","child":{"type":"name_glob","value":"*.mp4"}}
	}`)
	for _, m := range out.Matches {
		if strings.HasSuffix(m.Path, ".mp4") {
			t.Errorf("NOT *.mp4 matched %s", m.Path)
		}
	}
}

func TestSearchFiles_Nested(t *testing.T) {
	_, deps := makeTestMount(t)
	// (mp4 OR mkv) AND NOT trash
	out := runSearch(t, deps, `{
		"mount_id":"test",
		"expression":{"type":"and","children":[
			{"type":"or","children":[
				{"type":"name_glob","value":"*.mp4"},
				{"type":"name_glob","value":"*.mkv"}
			]},
			{"type":"not","child":{"type":"path_contains","value":"trash"}}
		]}
	}`)
	if out.Total < 1 {
		t.Errorf("Total = %d, want >= 1 (big.mp4 or movies/*)", out.Total)
	}
	for _, m := range out.Matches {
		if strings.Contains(m.Path, "trash") {
			t.Errorf("nested NOT trash 失效: %s", m.Path)
		}
	}
}

// ─── 11. TestSearchFiles_MaxResults ───────────────────────────

func TestSearchFiles_MaxResults(t *testing.T) {
	_, deps := makeTestMount(t)
	out := runSearch(t, deps, `{
		"mount_id":"test",
		"max_results":1,
		"expression":{"type":"name_glob","value":"*.mp4"}
	}`)
	if len(out.Matches) > 1 {
		t.Errorf("len(matches) = %d, want <= 1", len(out.Matches))
	}
}

// ─── 12. TestSearchFiles_UnknownExprType ──────────────────────

func TestSearchFiles_UnknownExprType(t *testing.T) {
	_, deps := makeTestMount(t)
	res, err := searchFilesHandler(context.Background(), `{
		"mount_id":"test",
		"expression":{"type":"nonsense_type","value":"x"}
	}`, deps)
	if err != nil {
		t.Fatalf("handler returned non-nil err: %v", err)
	}
	if !res.IsError {
		t.Errorf("expected IsError=true for unknown type. Result = %s", res.Result)
	}
}

// ─── 13. TestSearchFiles_GlobCompilation ──────────────────────

func TestSearchFiles_GlobCompilation(t *testing.T) {
	cases := []struct {
		glob   string
		name   string
		expect bool
	}{
		{"*.mp4", "movie.mp4", true},
		{"*.mp4", "movie.avi", false},
		{"ep?.mkv", "ep1.mkv", true},
		{"ep?.mkv", "ep10.mkv", false}, // ? 不跨 /
		{"**/log", "foo/log", true},
		{"*.txt", "subdir/file.txt", false}, // * 不跨 /
		{"a.b", "a.b", true},
		{"a.b", "axb", false},
		{"*.go", "main.go.bak", false},
	}
	for _, c := range cases {
		re, err := globToRegex(c.glob)
		if err != nil {
			t.Errorf("globToRegex(%q) err: %v", c.glob, err)
			continue
		}
		got := re.MatchString(c.name)
		if got != c.expect {
			t.Errorf("globToRegex(%q).MatchString(%q) = %v, want %v",
				c.glob, c.name, got, c.expect)
		}
	}
}
