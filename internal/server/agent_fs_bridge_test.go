// internal/server/agent_fs_bridge_test.go
//
// fs 桥接测试。
//
// 重点覆盖：
//   - ListFSMounts / resolveMount 在不同配置下的行为
//   - 5 个 fs tool 的 happy path（list_mounts / list_files / read_file / stat_file / get_storage_info）
//   - 错误路径：path_forbidden（../ 越界）、mount_unavailable、is_directory、too_large
//   - 二进制嗅探：NUL 字节 → base64 + is_binary=true
//   - 容器嗅探：.encv 文件头 → is_encrypted=true
//   - executeFSTool 路由（unknown tool）
//   - ListAgentTools 合并 + executeAgentTool 派发
package server

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ─── 工具函数 ─────────────────────────────────────────────────

// newTestServerWithDirs 构造一个 Server 实例，servingDir/webdavDir 指向临时目录
// 返回 Server + 临时目录 cleanup 函数
func newTestServerWithDirs(t *testing.T, servingDir, webdavDir string) *Server {
	t.Helper()
	return &Server{
		servingDir: servingDir,
		webdavDir:  webdavDir,
		webdavPath: "/webdav",
	}
}

// writeTestFile 在 dir 下写一个文件，返回绝对路径
func writeTestFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	abs := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatalf("MkdirAll 失败: %v", err)
	}
	if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile 失败: %v", err)
	}
	return abs
}

// ─── ListFSMounts ─────────────────────────────────────────────

func TestListFSMounts_ServingOnly(t *testing.T) {
	dir := t.TempDir()
	srv := newTestServerWithDirs(t, dir, "")

	mounts := srv.ListFSMounts()
	if len(mounts) != 1 {
		t.Fatalf("只配 servingDir 时挂载数 = %d, want 1", len(mounts))
	}
	m := mounts[0]
	if m.ID != "serving" {
		t.Errorf("ID = %q, want serving", m.ID)
	}
	if !m.Available {
		t.Errorf("Available 应为 true（目录存在可读）")
	}
	if m.Type != "serving" {
		t.Errorf("Type = %q, want serving", m.Type)
	}
}

func TestListFSMounts_ServingAndWebdav(t *testing.T) {
	srv := newTestServerWithDirs(t, t.TempDir(), t.TempDir())
	mounts := srv.ListFSMounts()
	if len(mounts) != 2 {
		t.Fatalf("期望 2 个挂载点, got %d", len(mounts))
	}
	ids := []string{mounts[0].ID, mounts[1].ID}
	if !contains(ids, "serving") || !contains(ids, "webdav") {
		t.Errorf("挂载点应含 serving 和 webdav, got %v", ids)
	}
}

func TestListFSMounts_ServingEqualsWebdav_NoDuplicate(t *testing.T) {
	// 如果 webdavDir == servingDir → 视为同一个挂载点，不重复暴露
	dir := t.TempDir()
	srv := newTestServerWithDirs(t, dir, dir)
	mounts := srv.ListFSMounts()
	if len(mounts) != 1 {
		t.Errorf("servingDir == webdavDir 时应只暴露 1 个挂载点, got %d", len(mounts))
	}
}

func TestListFSMounts_Unavailable(t *testing.T) {
	// servingDir 指向不存在的目录 → Available=false
	srv := newTestServerWithDirs(t, "/nonexistent/encv-fs-test-12345", "")
	mounts := srv.ListFSMounts()
	if len(mounts) != 1 {
		t.Fatalf("挂载数 = %d, want 1", len(mounts))
	}
	if mounts[0].Available {
		t.Errorf("不存在的目录 Available 应为 false")
	}
}

// ─── resolveMount ─────────────────────────────────────────────

func TestResolveMount_ValidServing(t *testing.T) {
	dir := t.TempDir()
	srv := newTestServerWithDirs(t, dir, "")
	root, ok := srv.resolveMount("serving")
	if !ok {
		t.Fatal("resolveMount('serving') 应返回 ok=true")
	}
	// 路径可能是经过 filepath.Abs 的，统一用 Eval 比较
	abs, _ := filepath.Abs(dir)
	if !pathsEqual(root, abs) {
		t.Errorf("resolveMount('serving') = %q, want %q", root, abs)
	}
}

func TestResolveMount_ValidWebdav(t *testing.T) {
	serving := t.TempDir()
	webdav := t.TempDir()
	srv := newTestServerWithDirs(t, serving, webdav)

	_, ok := srv.resolveMount("webdav")
	if !ok {
		t.Fatal("resolveMount('webdav') 应返回 ok=true")
	}
}

func TestResolveMount_UnknownMount(t *testing.T) {
	srv := newTestServerWithDirs(t, t.TempDir(), "")
	_, ok := srv.resolveMount("nonexistent_mount")
	if ok {
		t.Errorf("不存在的挂载点应返回 ok=false")
	}
}

func TestResolveMount_UnavailableMount(t *testing.T) {
	srv := newTestServerWithDirs(t, "/nonexistent/encv-fs-test-67890", "")
	_, ok := srv.resolveMount("serving")
	if ok {
		t.Errorf("不可用挂载点应返回 ok=false")
	}
}

// ─── fsListMounts 工具 ───────────────────────────────────────

func TestFsListMounts_OutputStructure(t *testing.T) {
	srv := newTestServerWithDirs(t, t.TempDir(), "")
	raw, err := srv.fsListMounts("{}")
	if err != nil {
		t.Fatalf("fsListMounts 不应返回 error: %v", err)
	}

	var out map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatalf("fsListMounts 输出非合法 JSON: %v\n%s", err, raw)
	}
	if out["count"] == nil {
		t.Errorf("缺 count 字段: %s", raw)
	}
	if out["items"] == nil {
		t.Errorf("缺 items 数组: %s", raw)
	}
	if out["note"] == nil {
		t.Errorf("缺 note 提示: %s", raw)
	}
}

// ─── fsListFiles 工具 ────────────────────────────────────────

func TestFsListFiles_HappyPath(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "a.txt", "hello")
	writeTestFile(t, dir, "b.encv", "ENCVfake")
	if err := os.MkdirAll(filepath.Join(dir, "subdir"), 0o755); err != nil {
		t.Fatal(err)
	}

	srv := newTestServerWithDirs(t, dir, "")
	raw, err := srv.fsListFiles(`{"mount_id":"serving","rel_path":"/"}`)
	if err != nil {
		t.Fatalf("fsListFiles 不应返回 error: %v", err)
	}

	var out map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatalf("输出非合法 JSON: %v\n%s", err, raw)
	}
	if out["mount_id"] != "serving" {
		t.Errorf("mount_id = %v, want serving", out["mount_id"])
	}
	items, ok := out["items"].([]interface{})
	if !ok {
		t.Fatalf("items 不是数组: %v", out["items"])
	}
	// 至少应该有 a.txt 和 subdir（b.encv 因为有 ENCV 头会被标记 is_encrypted=true，但仍然返回）
	if len(items) < 2 {
		t.Errorf("items 数量 = %d, want >= 2", len(items))
	}

	// 找到 a.txt 和 b.encv
	foundTxt, foundEncv := false, false
	for _, it := range items {
		item := it.(map[string]interface{})
		name := item["name"].(string)
		if name == "a.txt" {
			foundTxt = true
			if item["is_dir"] != false {
				t.Errorf("a.txt is_dir = %v, want false", item["is_dir"])
			}
			if item["is_encrypted"] != false {
				t.Errorf("a.txt 不应是加密容器")
			}
		}
		if name == "b.encv" {
			foundEncv = true
			if item["is_encrypted"] != true {
				t.Errorf("b.encv 头部 ENCV 应被识别为加密容器")
			}
		}
	}
	if !foundTxt {
		t.Errorf("未找到 a.txt in items")
	}
	if !foundEncv {
		t.Errorf("未找到 b.encv in items")
	}
}

func TestFsListFiles_DefaultRelPath(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "z.txt", "x")
	srv := newTestServerWithDirs(t, dir, "")

	// rel_path 缺省 → 默认 "/"
	raw, _ := srv.fsListFiles(`{"mount_id":"serving"}`)
	if !strings.Contains(raw, "z.txt") {
		t.Errorf("缺省 rel_path 应默认到根: %s", raw)
	}
}

func TestFsListFiles_MissingMountID(t *testing.T) {
	dir := t.TempDir()
	srv := newTestServerWithDirs(t, dir, "")
	raw, _ := srv.fsListFiles(`{}`)
	if !strings.Contains(raw, `"error":"missing_args"`) {
		t.Errorf("缺 mount_id 应返回 missing_args: %s", raw)
	}
}

func TestFsListFiles_InvalidArgsJSON(t *testing.T) {
	srv := newTestServerWithDirs(t, t.TempDir(), "")
	raw, _ := srv.fsListFiles(`not-json{`)
	if !strings.Contains(raw, `"error":"invalid_args"`) {
		t.Errorf("非法 JSON 应返回 invalid_args: %s", raw)
	}
}

func TestFsListFiles_MountUnavailable(t *testing.T) {
	// 故意不配挂载点
	srv := &Server{}
	raw, _ := srv.fsListFiles(`{"mount_id":"serving","rel_path":"/"}`)
	if !strings.Contains(raw, `"error":"mount_unavailable"`) {
		t.Errorf("挂载点不可用应返回 mount_unavailable: %s", raw)
	}
}

func TestFsListFiles_PathForbidden(t *testing.T) {
	// 尝试用 ../ 逃出 servingDir
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "subdir"), 0o755); err != nil {
		t.Fatal(err)
	}
	srv := newTestServerWithDirs(t, dir, "")

	raw, _ := srv.fsListFiles(`{"mount_id":"serving","rel_path":"subdir/../../etc"}`)
	if !strings.Contains(raw, `"error":"path_forbidden"`) {
		t.Errorf("../ 越界应返回 path_forbidden: %s", raw)
	}
}

func TestFsListFiles_MaxEntriesTruncate(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 5; i++ {
		writeTestFile(t, dir, "f"+string(rune('0'+i))+".txt", "x")
	}
	srv := newTestServerWithDirs(t, dir, "")

	raw, _ := srv.fsListFiles(`{"mount_id":"serving","rel_path":"/","max_entries":2}`)
	var out map[string]interface{}
	_ = json.Unmarshal([]byte(raw), &out)
	if out["truncated"] != true {
		t.Errorf("max_entries=2 时 truncated 应为 true: %s", raw)
	}
	items := out["items"].([]interface{})
	if len(items) != 2 {
		t.Errorf("items 数量 = %d, want 2", len(items))
	}
}

func TestFsListFiles_SkipsHiddenFiles(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, ".hidden", "x")
	writeTestFile(t, dir, "visible.txt", "y")
	srv := newTestServerWithDirs(t, dir, "")

	raw, _ := srv.fsListFiles(`{"mount_id":"serving","rel_path":"/"}`)
	if strings.Contains(raw, ".hidden") {
		t.Errorf("list_files 不应返回 .hidden: %s", raw)
	}
	if !strings.Contains(raw, "visible.txt") {
		t.Errorf("应返回 visible.txt: %s", raw)
	}
}

// ─── fsReadFile 工具 ─────────────────────────────────────────

func TestFsReadFile_TextFile(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "hello.txt", "hello world")
	srv := newTestServerWithDirs(t, dir, "")

	raw, _ := srv.fsReadFile(`{"mount_id":"serving","rel_path":"/hello.txt"}`)
	var out map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatalf("输出非合法 JSON: %v\n%s", err, raw)
	}
	if out["is_binary"] != false {
		t.Errorf("文本文件 is_binary 应为 false: %s", raw)
	}
	if out["content"] != "hello world" {
		t.Errorf("content = %v, want 'hello world'", out["content"])
	}
}

func TestFsReadFile_BinaryFile(t *testing.T) {
	dir := t.TempDir()
	// 二进制文件：头 4 字节含 NUL
	bin := []byte{0x89, 'P', 'N', 'G', 0x00, 0x01, 0x02}
	if err := os.WriteFile(filepath.Join(dir, "img.bin"), bin, 0o644); err != nil {
		t.Fatal(err)
	}
	srv := newTestServerWithDirs(t, dir, "")

	raw, _ := srv.fsReadFile(`{"mount_id":"serving","rel_path":"/img.bin"}`)
	var out map[string]interface{}
	_ = json.Unmarshal([]byte(raw), &out)
	if out["is_binary"] != true {
		t.Errorf("含 NUL 的文件 is_binary 应为 true: %s", raw)
	}
	if out["content_b64"] == nil || out["content_b64"] == "" {
		t.Errorf("二进制文件应返回 content_b64: %s", raw)
	}
	if out["content"] != "" {
		t.Errorf("二进制文件 content 应为空: %v", out["content"])
	}
}

func TestFsReadFile_IsDirectory(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	srv := newTestServerWithDirs(t, dir, "")

	raw, _ := srv.fsReadFile(`{"mount_id":"serving","rel_path":"/sub"}`)
	if !strings.Contains(raw, `"error":"is_directory"`) {
		t.Errorf("读目录应返回 is_directory: %s", raw)
	}
}

func TestFsReadFile_TooLarge(t *testing.T) {
	dir := t.TempDir()
	// 写 100 字节文件，用 max_bytes=50 → too_large
	big := strings.Repeat("a", 100)
	writeTestFile(t, dir, "big.txt", big)
	srv := newTestServerWithDirs(t, dir, "")

	raw, _ := srv.fsReadFile(`{"mount_id":"serving","rel_path":"/big.txt","max_bytes":50}`)
	if !strings.Contains(raw, `"error":"too_large"`) {
		t.Errorf("超 max_bytes 应返回 too_large: %s", raw)
	}
}

func TestFsReadFile_NotFound(t *testing.T) {
	dir := t.TempDir()
	srv := newTestServerWithDirs(t, dir, "")

	raw, _ := srv.fsReadFile(`{"mount_id":"serving","rel_path":"/nonexistent.txt"}`)
	if !strings.Contains(raw, `"error":"stat_failed"`) {
		t.Errorf("读不存在文件应返回 stat_failed: %s", raw)
	}
}

func TestFsReadFile_PathForbidden(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "subdir"), 0o755); err != nil {
		t.Fatal(err)
	}
	srv := newTestServerWithDirs(t, dir, "")

	raw, _ := srv.fsReadFile(`{"mount_id":"serving","rel_path":"subdir/../../../etc/passwd"}`)
	if !strings.Contains(raw, `"error":"path_forbidden"`) {
		t.Errorf("../ 越界应返回 path_forbidden: %s", raw)
	}
}

func TestFsReadFile_DefaultMaxBytes(t *testing.T) {
	dir := t.TempDir()
	// 写一个 50 字节的文件（远小于 64KiB 默认值）
	content := strings.Repeat("a", 50)
	writeTestFile(t, dir, "small.txt", content)
	srv := newTestServerWithDirs(t, dir, "")

	raw, _ := srv.fsReadFile(`{"mount_id":"serving","rel_path":"/small.txt"}`)
	if !strings.Contains(raw, `"size":50`) {
		t.Errorf("默认 max_bytes=64K 应能读 50 字节文件: %s", raw)
	}
}

// ─── fsStatFile 工具 ─────────────────────────────────────────

func TestFsStatFile_File(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "f.txt", "abc")
	srv := newTestServerWithDirs(t, dir, "")

	raw, _ := srv.fsStatFile(`{"mount_id":"serving","rel_path":"/f.txt"}`)
	var out map[string]interface{}
	_ = json.Unmarshal([]byte(raw), &out)
	if out["is_dir"] != false {
		t.Errorf("文件 is_dir 应为 false: %s", raw)
	}
	if out["size"] == nil {
		t.Errorf("缺 size 字段: %s", raw)
	}
	if out["modified"] == nil {
		t.Errorf("缺 modified 字段: %s", raw)
	}
}

func TestFsStatFile_Directory(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	srv := newTestServerWithDirs(t, dir, "")

	raw, _ := srv.fsStatFile(`{"mount_id":"serving","rel_path":"/sub"}`)
	if !strings.Contains(raw, `"is_dir":true`) {
		t.Errorf("目录 stat 应含 is_dir=true: %s", raw)
	}
}

func TestFsStatFile_EncryptedContainer(t *testing.T) {
	dir := t.TempDir()
	// 写一个带 ENCV 头的"容器"
	if err := os.WriteFile(filepath.Join(dir, "x.encv"), []byte("ENCVxxxxxx"), 0o644); err != nil {
		t.Fatal(err)
	}
	srv := newTestServerWithDirs(t, dir, "")

	raw, _ := srv.fsStatFile(`{"mount_id":"serving","rel_path":"/x.encv"}`)
	if !strings.Contains(raw, `"is_encrypted":true`) {
		t.Errorf("ENCV 头文件 stat 应标 is_encrypted=true: %s", raw)
	}
}

func TestFsStatFile_PathForbidden(t *testing.T) {
	dir := t.TempDir()
	// 创建 subdir 后用 subdir/../../escape 真的逃出 baseDir
	// 不带前导 / 才能让 filepath.Clean 看到 .. 的逃逸意图
	if err := os.MkdirAll(filepath.Join(dir, "subdir"), 0o755); err != nil {
		t.Fatal(err)
	}
	srv := newTestServerWithDirs(t, dir, "")

	raw, _ := srv.fsStatFile(`{"mount_id":"serving","rel_path":"subdir/../../escape"}`)
	if !strings.Contains(raw, `"error":"path_forbidden"`) {
		t.Errorf("../ 越界应返回 path_forbidden: %s", raw)
	}
}

// ─── fsGetStorageInfo 工具 ───────────────────────────────────

func TestFsGetStorageInfo_HappyPath(t *testing.T) {
	srv := newTestServerWithDirs(t, t.TempDir(), "")
	raw, _ := srv.fsGetStorageInfo("{}")
	var out map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatalf("输出非合法 JSON: %v\n%s", err, raw)
	}
	if out["count"] == nil {
		t.Errorf("缺 count 字段: %s", raw)
	}
	if out["items"] == nil {
		t.Errorf("缺 items 数组: %s", raw)
	}
}

func TestFsGetStorageInfo_NoMounts(t *testing.T) {
	// 无挂载点
	srv := &Server{}
	raw, _ := srv.fsGetStorageInfo("{}")
	if !strings.Contains(raw, `"error":"no_mounts"`) {
		t.Errorf("无挂载点应返回 no_mounts: %s", raw)
	}
}

// ─── executeFSTool 路由 ──────────────────────────────────────

func TestExecuteFSTool_UnknownTool(t *testing.T) {
	srv := newTestServerWithDirs(t, t.TempDir(), "")
	_, err := srv.executeFSTool(context.Background(), "nonexistent_fs_tool", "{}")
	if err == nil {
		t.Fatal("executeFSTool(未知工具) 应返回 error")
	}
	if !strings.Contains(err.Error(), "unknown fs tool") {
		t.Errorf("error 应含 'unknown fs tool': %v", err)
	}
}

func TestExecuteFSTool_DispatchesAllFiveTools(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "x.txt", "abc")
	srv := newTestServerWithDirs(t, dir, "")

	// 不同工具的 args 模板
	argsTemplates := map[string]string{
		"list_mounts":      `{}`,
		"list_files":       `{"mount_id":"serving","rel_path":"/"}`,
		"read_file":        `{"mount_id":"serving","rel_path":"/x.txt"}`,
		"stat_file":        `{"mount_id":"serving","rel_path":"/x.txt"}`,
		"get_storage_info": `{}`,
	}
	// 验证：result 应不包含 "error"（每个工具对合法 args 都应成功）
	for name, args := range argsTemplates {
		t.Run(name, func(t *testing.T) {
			raw, err := srv.executeFSTool(context.Background(), name, args)
			if err != nil {
				t.Fatalf("executeFSTool(%s) error: %v", name, err)
			}
			if strings.Contains(raw, `"error":`) {
				t.Errorf("executeFSTool(%s) 不应返回 error, got: %s", name, raw)
			}
			if !strings.Contains(raw, `"mount_id":"serving"`) && name != "list_mounts" && name != "get_storage_info" {
				t.Errorf("executeFSTool(%s) 应含 mount_id 字段, got: %s", name, raw)
			}
		})
	}
}

// ─── ListFSTools 元信息 ─────────────────────────────────────

func TestListFSTools_ContainsFiveTools(t *testing.T) {
	srv := newTestServerWithDirs(t, t.TempDir(), "")
	tools := srv.ListFSTools()
	if len(tools) != 5 {
		t.Errorf("fs 工具数 = %d, want 5", len(tools))
	}

	wantNames := map[string]bool{
		"list_mounts":      false,
		"list_files":       false,
		"read_file":        false,
		"stat_file":        false,
		"get_storage_info": false,
	}
	for _, tool := range tools {
		name := tool["name"].(string)
		if _, ok := wantNames[name]; ok {
			wantNames[name] = true
		}
		if tool["needConfirm"] != false {
			t.Errorf("fs 工具 %s needConfirm 应为 false（只读）", name)
		}
		if tool["kind"] != "fileRead" {
			t.Errorf("fs 工具 %s kind 应为 fileRead, got %v", name, tool["kind"])
		}
	}
	for name, found := range wantNames {
		if !found {
			t.Errorf("缺少 fs 工具: %s", name)
		}
	}
}

// ─── ListAgentTools 聚合 ─────────────────────────────────────

func TestListAgentTools_ContainsPluginAndFSTools(t *testing.T) {
	srv := newTestServerWithDirs(t, t.TempDir(), "")
	tools := srv.ListAgentTools()

	hasFSTool := false
	hasPluginTool := false
	fsNames := make(map[string]bool)
	for _, tool := range tools {
		name := tool["name"].(string)
		if name == "list_mounts" || name == "read_file" || name == "list_files" {
			hasFSTool = true
			fsNames[name] = true
		}
		// 任一 _encrypt 工具来自 plugin
		if strings.HasSuffix(name, "_encrypt") {
			hasPluginTool = true
		}
	}
	if !hasFSTool {
		t.Errorf("ListAgentTools 应包含 fs 工具（list_mounts / read_file / list_files）")
	}
	if !hasPluginTool {
		t.Errorf("ListAgentTools 应包含至少一个 _encrypt plugin 工具")
	}
}

func TestListAgentTools_NoDuplicates(t *testing.T) {
	srv := newTestServerWithDirs(t, t.TempDir(), "")
	tools := srv.ListAgentTools()
	seen := make(map[string]bool)
	for _, tool := range tools {
		name := tool["name"].(string)
		if seen[name] {
			t.Errorf("工具名重复: %s", name)
		}
		seen[name] = true
	}
}

// ─── agentToolsToOpenAITools 转换 ────────────────────────────

func TestAgentToolsToOpenAITools_Format(t *testing.T) {
	tools := []map[string]interface{}{
		{
			"name":        "list_mounts",
			"description": "列出挂载点",
			"parameters":  map[string]interface{}{"type": "object", "properties": map[string]interface{}{}},
			"needConfirm": false,
			"kind":        "fileRead",
		},
	}
	out := agentToolsToOpenAITools(tools)
	if len(out) != 1 {
		t.Fatalf("长度 = %d, want 1", len(out))
	}
	first := out[0]
	if first["type"] != "function" {
		t.Errorf("type = %v, want function", first["type"])
	}
	fn, ok := first["function"].(map[string]interface{})
	if !ok {
		t.Fatalf("function 字段不存在或不是 map: %v", first)
	}
	if fn["name"] != "list_mounts" {
		t.Errorf("function.name = %v, want list_mounts", fn["name"])
	}
	if fn["description"] != "列出挂载点" {
		t.Errorf("function.description = %v, want '列出挂载点'", fn["description"])
	}
	if fn["parameters"] == nil {
		t.Errorf("function.parameters 缺失")
	}
	// 内部使用的 needConfirm/kind 字段不应泄漏到 OpenAI 协议
	if _, ok := first["needConfirm"]; ok {
		t.Errorf("OpenAI 协议不应含 needConfirm 字段")
	}
	if _, ok := first["kind"]; ok {
		t.Errorf("OpenAI 协议不应含 kind 字段")
	}
}

// ─── executeAgentTool 路由 ───────────────────────────────────

func TestExecuteAgentTool_RoutesToFS(t *testing.T) {
	dir := t.TempDir()
	srv := newTestServerWithDirs(t, dir, "")

	raw, err := srv.executeAgentTool(context.Background(), "list_mounts", `{}`)
	if err != nil {
		t.Fatalf("executeAgentTool(list_mounts) error: %v", err)
	}
	// list_mounts 输出应含 items 数组（fs 路径特征）
	if !strings.Contains(raw, `"items":`) {
		t.Errorf("list_mounts 应路由到 fs 路径: %s", raw)
	}
	if strings.Contains(raw, `"error":`) {
		t.Errorf("list_mounts 不应返回 error: %s", raw)
	}
}

func TestExecuteAgentTool_RoutesToPlugin(t *testing.T) {
	// 找一个 _encrypt 工具名，验证 plugin 路径还在工作
	tools := ListPluginTools()
	var encName string
	for _, t := range tools {
		if strings.HasSuffix(t["name"].(string), "_encrypt") {
			encName = t["name"].(string)
			break
		}
	}
	if encName == "" {
		t.Skip("无 _encrypt 工具")
	}
	srv := &Server{}
	// 传非法 args → 应返回 errJSON(invalid_args)，证明走到了 plugin 路径
	raw, _ := srv.executeAgentTool(context.Background(), encName, "not-json{")
	if !strings.Contains(raw, `"error":"invalid_args"`) {
		t.Errorf("%s 应走 plugin 路径并返回 invalid_args: %s", encName, raw)
	}
}

func TestExecuteAgentTool_Unknown(t *testing.T) {
	srv := &Server{}
	_, err := srv.executeAgentTool(context.Background(), "nonexistent_tool_xyz", "{}")
	if err == nil {
		t.Fatal("executeAgentTool(unknown) 应返回 error")
	}
}

// ─── detectContainerEntry / statFS / base64Encode 内部辅助 ──

func TestDetectContainerEntry_ENCVMagic(t *testing.T) {
	dir := t.TempDir()
	encvPath := filepath.Join(dir, "x.encv")
	if err := os.WriteFile(encvPath, []byte("ENCVfake"), 0o644); err != nil {
		t.Fatal(err)
	}
	plainPath := filepath.Join(dir, "x.txt")
	if err := os.WriteFile(plainPath, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	srv := &Server{}
	if !srv.detectContainerEntry(encvPath) {
		t.Error("ENCV 头文件应被识别为容器")
	}
	if srv.detectContainerEntry(plainPath) {
		t.Error("普通文本文件不应识别为容器")
	}
}

func TestDetectContainerEntry_ShortFile(t *testing.T) {
	dir := t.TempDir()
	shortPath := filepath.Join(dir, "short.bin")
	if err := os.WriteFile(shortPath, []byte("EN"), 0o644); err != nil {
		t.Fatal(err)
	}
	srv := &Server{}
	if srv.detectContainerEntry(shortPath) {
		t.Error("< 4 字节文件不应识别为容器")
	}
}

func TestDetectContainerEntry_NonexistentFile(t *testing.T) {
	srv := &Server{}
	if srv.detectContainerEntry("/nonexistent/file.encv") {
		t.Error("不存在文件应返回 false（不 panic）")
	}
}

func TestStatFS_HappyPath(t *testing.T) {
	dir := t.TempDir()
	info := statFS(dir)
	if info == nil {
		t.Fatal("statFS 不应返回 nil")
	}
	// 临时目录一定有 free/total/used 至少一个非 0
	if info["total"] == int64(0) && info["free"] == int64(0) {
		t.Errorf("临时目录 statFS 异常: %+v", info)
	}
}

func TestStatFS_EmptyPath(t *testing.T) {
	info := statFS("")
	if info["error"] == nil {
		t.Errorf("空路径应返回 error 字段: %+v", info)
	}
}

func TestBase64Encode_KnownVectors(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"", ""},
		{"f", "Zg=="},
		{"fo", "Zm8="},
		{"foo", "Zm9v"},
		{"foob", "Zm9vYg=="},
		{"fooba", "Zm9vYmE="},
		{"foobar", "Zm9vYmFy"},
	}
	for _, tt := range tests {
		if got := base64Encode([]byte(tt.in)); got != tt.want {
			t.Errorf("base64Encode(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// ─── helper ─────────────────────────────────────────────────

func contains(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}

func pathsEqual(a, b string) bool {
	aa, _ := filepath.EvalSymlinks(a)
	bb, _ := filepath.EvalSymlinks(b)
	return aa == bb
}
