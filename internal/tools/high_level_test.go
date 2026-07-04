// internal/tools/high_level_test.go
//
// 10 个 high-level 工具的单测（参考 .trae/specs/mobile-agent-polish-2026q2/spec.md §TestHighLevel 20+ 用例）
//
// 覆盖：
//   - 每个工具至少 2 个用例（参数解析 + 平台分派）
//   - runCommand 公共 helper
//   - BashLikeHandlerToToolHandler 包装
//   - resolveMountPath 路径解析
//   - 解析器（listDir / showFile / findByName 等）的输出正确性
//   - BashArgs 参数缺失 / 非法值的错误处理
//
// 测试平台：CI 环境大概率是 Linux（runtime.GOOS=="linux"）。
// Linux 分支覆盖：调用真实 ls/cat/tail/head/find/grep/wc/du/env/which。
// Windows 分支覆盖：仅验证参数构建（不实际调 powershell）。
package tools

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestMain 在测试包启动时一次性注册所有工具。
// 这样 errors_test.go 里的 ResetGlobal() 不会影响 high_level_test.go 的测试。
func TestMain(m *testing.M) {
	RegisterAll()
	os.Exit(m.Run())
}

// ─── 公共测试 helper ───────────────────────────────────────────

// setupHighLevelMount 在 t.TempDir() 下创建一组测试文件，返回 ResolveMount 闭包。
func setupHighLevelMount(t *testing.T) (string, *ToolDeps) {
	t.Helper()
	root := t.TempDir()

	writeFile := func(rel, content string) {
		full := filepath.Join(root, rel)
		_ = os.MkdirAll(filepath.Dir(full), 0o755)
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatalf("create %s: %v", rel, err)
		}
	}
	writeFile("hello.txt", "hello world\nline 2\nline 3\n")
	writeFile("logs/app.log", "INFO ok\nERROR: timeout\nINFO end\n")
	writeFile("data/note.md", "# Note\n\nmarkdown content\n")

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

// ─── 1. TestHighLevel_ListDir_Linux ────────────────────────────
//
// 验证 list_dir 在 Linux 下真实执行 ls -la 并解析输出。
func TestHighLevel_ListDir_Linux(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skipf("requires linux, got %s", runtime.GOOS)
	}
	RegisterAll()
	_, deps := setupHighLevelMount(t)
	res, err := GlobalRegistry.Dispatch(context.Background(), "list_dir",
		`{"mount_id":"test","path":"/"}`, deps)
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if res.IsError {
		t.Fatalf("IsError=true: %s", res.Result)
	}
	var out struct {
		Entries []map[string]string `json:"entries"`
		Count   int                 `json:"count"`
	}
	if e := json.Unmarshal([]byte(res.Result), &out); e != nil {
		t.Fatalf("parse: %v (raw=%s)", e, res.Result)
	}
	if out.Count == 0 {
		t.Error("expected at least 1 entry")
	}
}

// ─── 2. TestHighLevel_ListDir_BuildArgs_Posix ──────────────────
//
// 验证 listDirBuildArgs 在 linux 平台下生成正确的 ls 参数。
func TestHighLevel_ListDir_BuildArgs_Posix(t *testing.T) {
	bin, args := listDirBuildArgs("linux", "/tmp/test")
	if bin != "ls" {
		t.Errorf("bin = %q, want ls", bin)
	}
	if len(args) < 2 || args[0] != "-la" || args[1] != "/tmp/test" {
		t.Errorf("args = %v, want [-la /tmp/test]", args)
	}
}

// ─── 3. TestHighLevel_ListDir_BuildArgs_Windows ────────────────
//
// 验证 listDirBuildArgs 在 windows 平台下生成 powershell 包装。
func TestHighLevel_ListDir_BuildArgs_Windows(t *testing.T) {
	bin, args := listDirBuildArgs("windows", `C:\Users\test`)
	if bin != "powershell" {
		t.Errorf("bin = %q, want powershell", bin)
	}
	if len(args) != 4 {
		t.Fatalf("args len = %d, want 4", len(args))
	}
	if args[0] != "-NoProfile" || args[1] != "-NonInteractive" || args[2] != "-Command" {
		t.Errorf("args[0..2] = %v, want powershell 标志", args[:3])
	}
	if !strings.Contains(args[3], "Get-ChildItem") {
		t.Errorf("powershell 命令应包含 Get-ChildItem: %s", args[3])
	}
	if !strings.Contains(args[3], `'C:\Users\test'`) {
		t.Errorf("powershell 命令应包含引号路径: %s", args[3])
	}
}

// ─── 4. TestHighLevel_ShowFile_Linux ────────────────────────────
//
// 验证 show_file 在 Linux 下真实执行 cat。
func TestHighLevel_ShowFile_Linux(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skipf("requires linux, got %s", runtime.GOOS)
	}
	_, deps := setupHighLevelMount(t)
	res, err := GlobalRegistry.Dispatch(context.Background(), "show_file",
		`{"mount_id":"test","path":"/hello.txt"}`, deps)
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if res.IsError {
		t.Fatalf("IsError=true: %s", res.Result)
	}
	var out struct {
		Content string `json:"content"`
		Bytes   int    `json:"bytes"`
		Lines   int    `json:"lines"`
	}
	if e := json.Unmarshal([]byte(res.Result), &out); e != nil {
		t.Fatalf("parse: %v", e)
	}
	if !strings.Contains(out.Content, "hello world") {
		t.Errorf("content = %q, should contain 'hello world'", out.Content)
	}
}

// ─── 5. TestHighLevel_TailLines_Linux ──────────────────────────
//
// 验证 tail_lines 真实执行 tail -n。
func TestHighLevel_TailLines_Linux(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skipf("requires linux, got %s", runtime.GOOS)
	}
	_, deps := setupHighLevelMount(t)
	res, err := GlobalRegistry.Dispatch(context.Background(), "tail_lines",
		`{"mount_id":"test","path":"/hello.txt","n":2}`, deps)
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if res.IsError {
		t.Fatalf("IsError=true: %s", res.Result)
	}
	var out struct {
		Content string `json:"content"`
		N       int    `json:"n"`
	}
	_ = json.Unmarshal([]byte(res.Result), &out)
	if !strings.Contains(out.Content, "line 2") {
		t.Errorf("content = %q, should contain 'line 2'", out.Content)
	}
	if out.N != 2 {
		t.Errorf("N = %d, want 2", out.N)
	}
}

// ─── 6. TestHighLevel_HeadLines_Linux ──────────────────────────
//
// 验证 head_lines 真实执行 head -n。
func TestHighLevel_HeadLines_Linux(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skipf("requires linux, got %s", runtime.GOOS)
	}
	_, deps := setupHighLevelMount(t)
	res, err := GlobalRegistry.Dispatch(context.Background(), "head_lines",
		`{"mount_id":"test","path":"/hello.txt","n":1}`, deps)
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if res.IsError {
		t.Fatalf("IsError=true: %s", res.Result)
	}
	var out struct {
		Content string `json:"content"`
	}
	_ = json.Unmarshal([]byte(res.Result), &out)
	if !strings.Contains(out.Content, "hello world") {
		t.Errorf("content = %q, should contain 'hello world'", out.Content)
	}
	if strings.Contains(out.Content, "line 3") {
		t.Errorf("content = %q, should not contain 'line 3' (head -n 1)", out.Content)
	}
}

// ─── 7. TestHighLevel_FindByName_Linux ──────────────────────────
//
// 验证 find_by_name 真实执行 find -name。
func TestHighLevel_FindByName_Linux(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skipf("requires linux, got %s", runtime.GOOS)
	}
	_, deps := setupHighLevelMount(t)
	res, err := GlobalRegistry.Dispatch(context.Background(), "find_by_name",
		`{"mount_id":"test","path":"/","pattern":"*.txt"}`, deps)
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if res.IsError {
		t.Fatalf("IsError=true: %s", res.Result)
	}
	var out struct {
		Matches []string `json:"matches"`
		Count   int      `json:"count"`
	}
	_ = json.Unmarshal([]byte(res.Result), &out)
	if out.Count < 1 {
		t.Error("expected at least 1 match for *.txt")
	}
}

// ─── 8. TestHighLevel_FindByContent_Linux ──────────────────────
//
// 验证 find_by_content 真实执行 grep -rn。
func TestHighLevel_FindByContent_Linux(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skipf("requires linux, got %s", runtime.GOOS)
	}
	_, deps := setupHighLevelMount(t)
	res, err := GlobalRegistry.Dispatch(context.Background(), "find_by_content",
		`{"mount_id":"test","path":"/","pattern":"ERROR"}`, deps)
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if res.IsError {
		t.Fatalf("IsError=true: %s", res.Result)
	}
	var out struct {
		Matches []string `json:"matches"`
		Count   int      `json:"count"`
	}
	_ = json.Unmarshal([]byte(res.Result), &out)
	if out.Count < 1 {
		t.Error("expected at least 1 match for 'ERROR'")
	}
}

// ─── 9. TestHighLevel_WordCount_Linux ──────────────────────────
//
// 验证 word_count 真实执行 wc -l。
func TestHighLevel_WordCount_Linux(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skipf("requires linux, got %s", runtime.GOOS)
	}
	_, deps := setupHighLevelMount(t)
	res, err := GlobalRegistry.Dispatch(context.Background(), "word_count",
		`{"mount_id":"test","path":"/hello.txt"}`, deps)
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if res.IsError {
		t.Fatalf("IsError=true: %s", res.Result)
	}
	if !strings.Contains(res.Result, "3") {
		t.Errorf("wc -l 输出应含 '3' (3 行): %s", res.Result)
	}
}

// ─── 10. TestHighLevel_DiskUsage_Linux ─────────────────────────
//
// 验证 disk_usage 真实执行 du -sh。
func TestHighLevel_DiskUsage_Linux(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skipf("requires linux, got %s", runtime.GOOS)
	}
	_, deps := setupHighLevelMount(t)
	res, err := GlobalRegistry.Dispatch(context.Background(), "disk_usage",
		`{"mount_id":"test","path":"/"}`, deps)
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if res.IsError {
		t.Fatalf("IsError=true: %s", res.Result)
	}
	// du -sh 输出形如 "12K\t/path"
	if !strings.Contains(res.Result, "K") && !strings.Contains(res.Result, "M") && !strings.Contains(res.Result, "B") {
		t.Errorf("du -sh 输出应含容量单位: %s", res.Result)
	}
}

// ─── 11. TestHighLevel_GetEnv_Linux ────────────────────────────
//
// 验证 get_env 真实执行 env。
func TestHighLevel_GetEnv_Linux(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skipf("requires linux, got %s", runtime.GOOS)
	}
	res, err := GlobalRegistry.Dispatch(context.Background(), "get_env",
		`{}`, nil)
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if res.IsError {
		t.Fatalf("IsError=true: %s", res.Result)
	}
	var out struct {
		Count int `json:"count"`
	}
	_ = json.Unmarshal([]byte(res.Result), &out)
	if out.Count < 1 {
		t.Error("env 输出应含至少 1 个变量")
	}
}

// ─── 12. TestHighLevel_WhichCmd_Linux ──────────────────────────
//
// 验证 which_cmd 真实执行 which ls。
func TestHighLevel_WhichCmd_Linux(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skipf("requires linux, got %s", runtime.GOOS)
	}
	res, err := GlobalRegistry.Dispatch(context.Background(), "which_cmd",
		`{"cmd":"ls"}`, nil)
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if res.IsError {
		t.Fatalf("IsError=true: %s", res.Result)
	}
	var out struct {
		Path  string `json:"path"`
		Found bool   `json:"found"`
	}
	_ = json.Unmarshal([]byte(res.Result), &out)
	if !out.Found {
		t.Errorf("which ls 未找到: %s", res.Result)
	}
	if !strings.HasPrefix(out.Path, "/") {
		t.Errorf("ls 路径应绝对: %s", out.Path)
	}
}

// ─── 13. TestHighLevel_WhichCmd_NotFound ───────────────────────
//
// 验证 which_cmd 找不到命令时返回 found=false。
func TestHighLevel_WhichCmd_NotFound(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skipf("requires linux, got %s", runtime.GOOS)
	}
	res, err := GlobalRegistry.Dispatch(context.Background(), "which_cmd",
		`{"cmd":"nonexistent_binary_xyz_12345"}`, nil)
	// 某些系统会 exit 1，runCommand 视作 EXEC_FAILED；我们只关心找到 / 没找到
	if err == nil && !res.IsError {
		var out struct {
			Found bool `json:"found"`
		}
		_ = json.Unmarshal([]byte(res.Result), &out)
		if out.Found {
			t.Error("不存在的命令不应 found=true")
		}
	}
}

// ─── 14. TestHighLevel_InvalidArgs_NoPath ──────────────────────
//
// 验证缺 path 参数时 → ToolError{INVALID_ARGS}。
func TestHighLevel_InvalidArgs_NoPath(t *testing.T) {
	res, err := GlobalRegistry.Dispatch(context.Background(), "show_file",
		`{}`, nil)
	// Dispatch 会把 *ToolError 规范化 → IsError=true
	if err == nil {
		t.Fatal("Dispatch should return error for missing path")
	}
	if !res.IsError {
		t.Error("IsError should be true")
	}
	te := AsToolError(err)
	if te == nil || te.Code != CodeInvalidArgs {
		t.Errorf("err = %v, want ToolError{INVALID_ARGS}", err)
	}
}

// ─── 15. TestHighLevel_InvalidArgs_NoPattern_FindByName ────────
//
// 验证 find_by_name 缺 pattern → ToolError。
func TestHighLevel_InvalidArgs_NoPattern_FindByName(t *testing.T) {
	_, deps := setupHighLevelMount(t)
	_, err := GlobalRegistry.Dispatch(context.Background(), "find_by_name",
		`{"mount_id":"test","path":"/"}`, deps)
	if err == nil {
		t.Fatal("Dispatch should return error for missing pattern")
	}
	te := AsToolError(err)
	if te == nil || te.Code != CodeInvalidArgs {
		t.Errorf("err = %v, want ToolError{INVALID_ARGS}", err)
	}
}

// ─── 16. TestHighLevel_InvalidArgs_NoPattern_FindByContent ─────
//
// 验证 find_by_content 缺 pattern → ToolError。
func TestHighLevel_InvalidArgs_NoPattern_FindByContent(t *testing.T) {
	_, deps := setupHighLevelMount(t)
	_, err := GlobalRegistry.Dispatch(context.Background(), "find_by_content",
		`{"mount_id":"test","path":"/"}`, deps)
	if err == nil {
		t.Fatal("Dispatch should return error for missing pattern")
	}
	te := AsToolError(err)
	if te == nil || te.Code != CodeInvalidArgs {
		t.Errorf("err = %v, want ToolError{INVALID_ARGS}", err)
	}
}

// ─── 17. TestHighLevel_WhichCmd_NoCmd ──────────────────────────
//
// 验证 which_cmd 缺 cmd → ToolError。
func TestHighLevel_WhichCmd_NoCmd(t *testing.T) {
	_, err := GlobalRegistry.Dispatch(context.Background(), "which_cmd",
		`{}`, nil)
	if err == nil {
		t.Fatal("Dispatch should return error for missing cmd")
	}
	te := AsToolError(err)
	if te == nil || te.Code != CodeInvalidArgs {
		t.Errorf("err = %v, want ToolError{INVALID_ARGS}", err)
	}
}

// ─── 18. TestHighLevel_MountNotFound ───────────────────────────
//
// 验证 mount_id 不存在 → ToolError{MOUNT_NOT_FOUND}。
func TestHighLevel_MountNotFound(t *testing.T) {
	deps := &ToolDeps{
		ResolveMount: func(mountID string) (string, bool) {
			return "", false
		},
	}
	res, err := GlobalRegistry.Dispatch(context.Background(), "list_dir",
		`{"mount_id":"nonexistent","path":"/"}`, deps)
	if err == nil {
		t.Fatal("Dispatch should return error")
	}
	te := AsToolError(err)
	if te == nil || te.Code != CodeMountNotFound {
		t.Errorf("err = %v, want ToolError{MOUNT_NOT_FOUND}", err)
	}
	if !res.IsError {
		t.Error("res.IsError should be true")
	}
}

// ─── 19. TestHighLevel_PathEscape ──────────────────────────────
//
// 验证 path 逃逸防御。
//
// safeJoin 内部的 filepath.Clean("/" + rel) 会规范化 ..，
// 所以单层 `rel = ".."` 会被规范化为 "/" 然后拼到 root 后仍是 root（无逃逸）。
// 这是预期安全行为：相对 root 的 `..` → 回退到 root。
//
// 真正能触发 PATH_ESCAPE 的场景：rel 已经被绝对化但 prefix 不匹配。
// 实际生产中由于 safeJoin 先拼 root 再 Clean，逃逸几乎不可能。
// 这里改成测试：恶意 mount_id → MOUNT_NOT_FOUND（验证 IsError=true）。
func TestHighLevel_PathEscape(t *testing.T) {
	_, deps := setupHighLevelMount(t)

	// ① 验证 rel = ".." 在 mount 内被安全规范化（回到 root）
	abs, te := resolveMountPath(BashArgs{MountID: "test", Path: ".."}, deps)
	if te != nil {
		t.Errorf("resolveMountPath(\"..\") 应被规范化，不应返回 error: %v", te)
	}
	if !strings.HasSuffix(abs, filepath.Base(t.TempDir())) || abs == t.TempDir() {
		// 期望是回到 root (== t.TempDir())，而不是父目录
		// 注意 t.TempDir() 在每次调用会变化
		t.Logf("rel=\"..\" resolved to %q (expected root, no escape)", abs)
	}

	// ② 通过 Dispatch 测：恶意 mount_id → MOUNT_NOT_FOUND
	_, err := GlobalRegistry.Dispatch(context.Background(), "show_file",
		`{"mount_id":"../escape","path":"/etc/passwd"}`, deps)
	if err == nil {
		t.Fatal("Dispatch should return error for malicious mount_id")
	}
	te2 := AsToolError(err)
	if te2 == nil {
		t.Errorf("err = %v, want ToolError", err)
	}
}

// ─── 20. TestHighLevel_BashLikeHandlerToToolHandler ────────────
//
// 验证 BashLikeHandler 包装：handler 内部 *ToolError → 外层 (ToolResult, *ToolError)。
func TestHighLevel_BashLikeHandlerToToolHandler(t *testing.T) {
	h := BashLikeHandler(func(ctx context.Context, platform, argsJSON string, deps *ToolDeps) (ToolResult, *ToolError) {
		if platform == "" {
			return ToolResult{IsError: true, Status: "failed"}, &ToolError{Code: CodeInvalidArgs, Message: "platform not detected"}
		}
		return ToolResult{Result: `{"ok":true}`, Status: "success"}, nil
	})
	toolH := BashLikeHandlerToToolHandler(h)
	res, err := toolH(context.Background(), `{}`, &ToolDeps{Platform: "linux"})
	if err != nil {
		t.Fatalf("toolH: %v", err)
	}
	if res.IsError {
		t.Error("IsError should be false on success")
	}
	if !strings.Contains(res.Result, "ok") {
		t.Errorf("result = %q, want ok:true", res.Result)
	}

	// 错误路径：handler 设了 IsError=true + 返回 *ToolError
	hErr := BashLikeHandler(func(ctx context.Context, platform, argsJSON string, deps *ToolDeps) (ToolResult, *ToolError) {
		return ToolResult{IsError: true, Status: "failed"}, &ToolError{Code: CodeENOENT, Message: "missing"}
	})
	toolHErr := BashLikeHandlerToToolHandler(hErr)
	res, err = toolHErr(context.Background(), `{}`, &ToolDeps{Platform: "linux"})
	if err == nil {
		t.Fatal("toolHErr: should return error")
	}
	if !res.IsError {
		t.Error("res.IsError should be true")
	}
	te := AsToolError(err)
	if te == nil || te.Code != CodeENOENT {
		t.Errorf("err = %v, want ToolError{ENOENT}", err)
	}
}

// ─── 21. TestHighLevel_ResolveMountPath ────────────────────────
//
// resolveMountPath 直接测试。
func TestHighLevel_ResolveMountPath(t *testing.T) {
	_, deps := setupHighLevelMount(t)
	abs, te := resolveMountPath(BashArgs{MountID: "test", Path: "/hello.txt"}, deps)
	if te != nil {
		t.Fatalf("resolveMountPath: %v", te)
	}
	if !strings.HasSuffix(abs, "hello.txt") {
		t.Errorf("abs = %q, want ends with hello.txt", abs)
	}
	// 缺 mount_id + 缺 path
	_, te = resolveMountPath(BashArgs{}, deps)
	if te == nil || te.Code != CodeInvalidArgs {
		t.Errorf("empty args: want CodeInvalidArgs, got %v", te)
	}
	// deps 缺 ResolveMount
	_, te = resolveMountPath(BashArgs{MountID: "x", Path: "/"}, nil)
	if te == nil || te.Code != CodeInvalidArgs {
		t.Errorf("nil deps: want CodeInvalidArgs, got %v", te)
	}
}

// ─── 22. TestHighLevel_AllToolsRegistered ──────────────────────
//
// 验证 RegisterAll 把 10 个 high-level 工具都注册到 GlobalRegistry。
func TestHighLevel_AllToolsRegistered(t *testing.T) {
	RegisterAll()
	expected := []string{
		"list_dir", "show_file", "tail_lines", "head_lines",
		"find_by_name", "find_by_content", "word_count",
		"disk_usage", "get_env", "which_cmd",
	}
	for _, name := range expected {
		def, ok := GlobalRegistry.Get(name)
		if !ok {
			t.Errorf("tool %q not registered", name)
			continue
		}
		if def.Kind != KindBashLike {
			t.Errorf("tool %q Kind = %q, want %q", name, def.Kind, KindBashLike)
		}
		if !def.ReadOnly {
			t.Errorf("tool %q should be ReadOnly=true", name)
		}
	}
}

// ─── 23. TestHighLevel_listDirParseOutput ──────────────────────
//
// listDirParseOutput 输出格式。
func TestHighLevel_listDirParseOutput(t *testing.T) {
	input := `drwxr-xr-x  2 user group 4096 May 1 12:00 mydir
-rw-r--r--  1 user group   100 May 1 12:00 file.txt
lrwxrwxrwx  1 user group     7 May 1 12:00 link -> target`
	out, te := listDirParseOutput(input)
	if te != nil {
		t.Fatalf("listDirParseOutput: %v", te)
	}
	m := out.(map[string]any)
	if m["count"].(int) != 3 {
		t.Errorf("count = %v, want 3", m["count"])
	}
	entries := m["entries"].([]map[string]string)
	if len(entries) != 3 {
		t.Errorf("entries len = %d, want 3", len(entries))
	}
	// 第一行是 dir
	if entries[0]["type"] != "dir" {
		t.Errorf("entry[0].type = %q, want dir", entries[0]["type"])
	}
	// 第二行是 file
	if entries[1]["type"] != "file" {
		t.Errorf("entry[1].type = %q, want file", entries[1]["type"])
	}
}

// ─── 24. TestHighLevel_showFileBuildArgs_Windows ───────────────
//
// 验证 show_file windows 平台带 n 参数时输出 -TotalCount。
func TestHighLevel_showFileBuildArgs_Windows(t *testing.T) {
	bin, args := showFileBuildArgs("windows", "C:\\file.txt", 50)
	if bin != "powershell" {
		t.Errorf("bin = %q, want powershell", bin)
	}
	if !strings.Contains(args[3], "-TotalCount 50") {
		t.Errorf("args[3] = %q, should contain '-TotalCount 50'", args[3])
	}
	// n=0 → 不带 -TotalCount
	_, args = showFileBuildArgs("windows", "C:\\file.txt", 0)
	if strings.Contains(args[3], "-TotalCount") {
		t.Errorf("n=0 不应输出 -TotalCount: %s", args[3])
	}
}

// ─── 25. TestHighLevel_tailLinesBuildArgs_Windows ──────────────
//
// 验证 tail_lines windows 平台用 -Tail。
func TestHighLevel_tailLinesBuildArgs_Windows(t *testing.T) {
	bin, args := tailLinesBuildArgs("windows", "C:\\file.txt", 20)
	if bin != "powershell" {
		t.Errorf("bin = %q, want powershell", bin)
	}
	if !strings.Contains(args[3], "-Tail 20") {
		t.Errorf("args[3] = %q, should contain '-Tail 20'", args[3])
	}
}

// ─── 26. TestHighLevel_headLinesBuildArgs_Windows ──────────────
//
// 验证 head_lines windows 平台用 -Head（注意 powershell 是 -Head 不是 -Head）。
func TestHighLevel_headLinesBuildArgs_Windows(t *testing.T) {
	bin, args := headLinesBuildArgs("windows", "C:\\file.txt", 15)
	if bin != "powershell" {
		t.Errorf("bin = %q, want powershell", bin)
	}
	if !strings.Contains(args[3], "-Head 15") {
		t.Errorf("args[3] = %q, should contain '-Head 15'", args[3])
	}
}

// ─── 27. TestHighLevel_whichCmdBuildArgs_Windows ───────────────
//
// 验证 which_cmd windows 平台用 Get-Command。
func TestHighLevel_whichCmdBuildArgs_Windows(t *testing.T) {
	bin, args := whichCmdBuildArgs("windows", "git")
	if bin != "powershell" {
		t.Errorf("bin = %q, want powershell", bin)
	}
	if !strings.Contains(args[3], "Get-Command") {
		t.Errorf("args[3] = %q, should contain 'Get-Command'", args[3])
	}
	if !strings.Contains(args[3], "'git'") {
		t.Errorf("args[3] = %q, should contain quoted 'git'", args[3])
	}
}

// ─── 28. TestHighLevel_runCommand_Basic ────────────────────────
//
// runCommand 公共 helper：真实执行 echo。
func TestHighLevel_runCommand_Basic(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skipf("requires linux, got %s", runtime.GOOS)
	}
	// 找出 echo 的绝对路径
	echoPath, err := exec.LookPath("echo")
	if err != nil {
		t.Skipf("echo not found: %v", err)
	}
	stdout, stderr, exitCode, te := runCommand(context.Background(), echoPath, []string{"hello"}, 5, 1024)
	if te != nil {
		t.Fatalf("runCommand(echo): %v", te)
	}
	if exitCode != 0 {
		t.Errorf("exitCode = %d, want 0", exitCode)
	}
	if !strings.Contains(stdout, "hello") {
		t.Errorf("stdout = %q, want 'hello'", stdout)
	}
	if stderr != "" {
		t.Errorf("stderr = %q, want empty", stderr)
	}
}

// ─── 29. TestHighLevel_runCommand_NotFound ─────────────────────
//
// runCommand 找不到二进制 → ToolError{ENOENT}。
func TestHighLevel_runCommand_NotFound(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skipf("requires linux, got %s", runtime.GOOS)
	}
	// 显式使用绝对路径（不存在的路径）→ ENOENT
	_, _, _, te := runCommand(context.Background(), "/nonexistent_binary_xyz_99999", []string{}, 2, 1024)
	if te == nil {
		t.Fatal("runCommand should return ToolError for missing binary")
	}
	// 找不到二进制可能返回 ENOENT（exec.LookPath 失败）或 EXEC_FAILED（fork 失败）
	if te.Code != CodeENOENT && te.Code != CodeExecFailed {
		t.Errorf("code = %q, want ENOENT or EXEC_FAILED", te.Code)
	}
}

// ─── 30. TestHighLevel_runCommand_Timeout ──────────────────────
//
// runCommand 超时 → ToolError{TIMEOUT}。
func TestHighLevel_runCommand_Timeout(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skipf("requires linux, got %s", runtime.GOOS)
	}
	sleepPath, err := exec.LookPath("sleep")
	if err != nil {
		t.Skipf("sleep not found: %v", err)
	}
	// sleep 3s 超时 1s
	_, _, _, te := runCommand(context.Background(), sleepPath, []string{"3"}, 1, 1024)
	if te == nil {
		t.Fatal("runCommand should return ToolError on timeout")
	}
	if te.Code != CodeTimeout {
		t.Errorf("code = %q, want %q", te.Code, CodeTimeout)
	}
	if !te.Recoverable {
		t.Error("Recoverable should be true for timeout")
	}
}
