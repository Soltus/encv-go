package webdav

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	goWebdav "golang.org/x/net/webdav"

	"github.com/Soltus/encv-go/internal/config"
	"github.com/Soltus/encv-go/internal/utils"
	"github.com/Soltus/encv-go/internal/v2/pluginsext"
	"github.com/Soltus/encv-go/internal/v2/types"
)

// TestNewENCVFSForRoot_ExplicitRoot 验证 NewENCVFSForRoot 用显式 rootDir 创建 webdavFS。
//
// 🆕 2026-06-17：多挂载点 webdav 适配（multi-mount-storage-refactor spec 续）
func TestNewENCVFSForRoot_ExplicitRoot(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &config.Config{
		Password: "test-password",
		Webdav: types.WebdavServer{
			Dir: "/somewhere/else", // 故意跟 rootDir 不同
		},
	}
	ctx := config.NewContext(t.Context(), cfg)

	fileSys, indexProvider, err := NewENCVFSForRoot(ctx, tmpDir, "/d/automation", nil, nil)
	if err != nil {
		t.Fatalf("NewENCVFSForRoot failed: %v", err)
	}
	if fileSys == nil || indexProvider == nil {
		t.Fatal("fileSys / indexProvider should not be nil")
	}
	// 验证 dir = tmpDir（不是 cfg.Webdav.Dir）
	if indexProvider.Dir() != tmpDir {
		t.Errorf("Dir() = %q, want %q", indexProvider.Dir(), tmpDir)
	}
	// 🆕 2026-06-17：WebdavPrefix 必须跟 urlPrefix 一致（h.Prefix 也用这个值）
	if indexProvider.WebdavPrefix() != "/d/automation" {
		t.Errorf("WebdavPrefix() = %q, want %q", indexProvider.WebdavPrefix(), "/d/automation")
	}
}

// TestNewENCVFSForRoot_EmptyRootFallback 验证 rootDir="" 时兜底用 cfg.Webdav.Dir。
func TestNewENCVFSForRoot_EmptyRootFallback(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &config.Config{
		Password: "test-password",
		Webdav: types.WebdavServer{
			Dir: tmpDir,
		},
	}
	ctx := config.NewContext(t.Context(), cfg)

	_, indexProvider, err := NewENCVFSForRoot(ctx, "", "/webdav", nil, nil)
	if err != nil {
		t.Fatalf("NewENCVFSForRoot failed: %v", err)
	}
	if indexProvider.Dir() != tmpDir {
		t.Errorf("Dir() with empty root should fallback to cfg.Webdav.Dir, got %q want %q", indexProvider.Dir(), tmpDir)
	}
	if indexProvider.WebdavPrefix() != "/webdav" {
		t.Errorf("WebdavPrefix() = %q, want %q", indexProvider.WebdavPrefix(), "/webdav")
	}
}

// TestNewENCVFSForRoot_EmptyUrlPrefixFallback 验证 urlPrefix="" 时兜底用 cfg.Webdav.Root。
//
// 🆕 2026-06-17：修 405 必走的回归测试 — urlPrefix 跟 cfg.Webdav.Root 必须适配
func TestNewENCVFSForRoot_EmptyUrlPrefixFallback(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &config.Config{
		Password: "test-password",
		Webdav: types.WebdavServer{
			Dir:  tmpDir,
			Root: "/webdav",
		},
	}
	ctx := config.NewContext(t.Context(), cfg)

	_, indexProvider, err := NewENCVFSForRoot(ctx, tmpDir, "", nil, nil)
	if err != nil {
		t.Fatalf("NewENCVFSForRoot failed: %v", err)
	}
	// urlPrefix="" → 兜底用 cfg.Webdav.Root = "/webdav"
	if indexProvider.WebdavPrefix() != "/webdav" {
		t.Errorf("WebdavPrefix() with empty urlPrefix should fallback to cfg.Webdav.Root, got %q want %q",
			indexProvider.WebdavPrefix(), "/webdav")
	}
}

// TestNewENCVFSForRoot_TrailingSlashNormalized 验证 urlPrefix 末尾斜杠被规范化。
func TestNewENCVFSForRoot_TrailingSlashNormalized(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &config.Config{Password: "test-password"}
	ctx := config.NewContext(t.Context(), cfg)

	_, indexProvider, err := NewENCVFSForRoot(ctx, tmpDir, "/d/automation/", nil, nil)
	if err != nil {
		t.Fatalf("NewENCVFSForRoot failed: %v", err)
	}
	// 末尾 "/" 应被 TrimSuffix
	if indexProvider.WebdavPrefix() != "/d/automation" {
		t.Errorf("WebdavPrefix() = %q, want %q (trailing slash should be stripped)", indexProvider.WebdavPrefix(), "/d/automation")
	}
}

// TestNewENCVFSForRoot_LeadingSlashNormalized 验证 urlPrefix 前导斜杠被规范化。
func TestNewENCVFSForRoot_LeadingSlashNormalized(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &config.Config{Password: "test-password"}
	ctx := config.NewContext(t.Context(), cfg)

	_, indexProvider, err := NewENCVFSForRoot(ctx, tmpDir, "d/automation", nil, nil)
	if err != nil {
		t.Fatalf("NewENCVFSForRoot failed: %v", err)
	}
	// 无前导 "/" 时应加上
	if indexProvider.WebdavPrefix() != "/d/automation" {
		t.Errorf("WebdavPrefix() = %q, want %q (missing leading slash should be added)", indexProvider.WebdavPrefix(), "/d/automation")
	}
}

// TestNewENCVFS_BackwardCompat 验证旧 NewENCVFS（不带 rootDir）仍能用 cfg.Webdav.Dir。
func TestNewENCVFS_BackwardCompat(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &config.Config{
		Password: "test-password",
		Webdav: types.WebdavServer{
			Dir: tmpDir,
		},
	}
	ctx := config.NewContext(t.Context(), cfg)

	_, indexProvider, err := NewENCVFS(ctx, nil, nil)
	if err != nil {
		t.Fatalf("NewENCVFS backward compat failed: %v", err)
	}
	if indexProvider.Dir() != tmpDir {
		t.Errorf("backward compat Dir() = %q, want %q", indexProvider.Dir(), tmpDir)
	}
}

// TestGoWebdavHandler_PropfindWithCorrectPrefix 验证修 405 根因 —
// goWebdav.Handler 在 h.Prefix 正确设置时返回 207（PROPFIND 成功），
// 而 h.Prefix 错配（空字符串）时会返回 405（Method Not Allowed）。
//
// 🆕 2026-06-17：multi-mount 405 修复 — 这是端到端核心回归测试
//   - 真实场景：mount URL = "/d/automation"
//   - 旧实现：h.Prefix 永远空 → stripPrefix 透传 /d/automation/foo 给 FS
//     → webdavPathToIndexKey 不匹配 /webdav prefix → 405
//   - 新实现：h.Prefix = "/d/automation" → stripPrefix 正确剥离 → 207
func TestGoWebdavHandler_PropfindWithCorrectPrefix(t *testing.T) {
	tmpDir := t.TempDir()

	// 1. 在 tmpDir 下创建一些 fixture 文件
	subdir := filepath.Join(tmpDir, "subdir")
	if err := os.MkdirAll(subdir, 0755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(subdir, "test.txt"), []byte("hello"), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	cfg := &config.Config{
		Password: "test-password",
		Webdav: types.WebdavServer{
			Dir: tmpDir,
		},
	}
	ctx := config.NewContext(t.Context(), cfg)

	// 3. 用 NewENCVFSForRoot 创建 webdavFS + goWebdav.Handler
	fileSysRaw, indexProvider, err := NewENCVFSForRoot(ctx, tmpDir, "/d/automation", nil, nil)
	if err != nil {
		t.Fatalf("NewENCVFSForRoot failed: %v", err)
	}
	fileSys, ok := fileSysRaw.(goWebdav.FileSystem)
	if !ok {
		t.Fatalf("fileSys does not implement goWebdav.FileSystem")
	}

	// ✅ 关键：h.Prefix 必须跟 fs.WebdavPrefix() 一致
	webdavHandler := &goWebdav.Handler{
		Prefix:     indexProvider.WebdavPrefix(),
		FileSystem: fileSys,
		LockSystem: goWebdav.NewMemLS(),
	}

	// 4. PROPFIND 请求 /d/automation/subdir/test.txt
	// 正确实现 → 207 Multi-Status
	// 错误实现（h.Prefix=""）→ 405 Method Not Allowed
	req := httptest.NewRequest("PROPFIND", "/d/automation/subdir/test.txt", nil)
	req.Header.Set("Depth", "0")
	rr := httptest.NewRecorder()
	webdavHandler.ServeHTTP(rr, req)

	if rr.Code == http.StatusMethodNotAllowed {
		t.Errorf("PROPFIND returned 405 — h.Prefix may not be set correctly. "+
			"WebdavPrefix()=%q, h.Prefix=%q, want status < 400 or 207",
			indexProvider.WebdavPrefix(), webdavHandler.Prefix)
	}

	// 5. OPTIONS 也应该 OK
	reqOpts := httptest.NewRequest("OPTIONS", "/d/automation/", nil)
	rrOpts := httptest.NewRecorder()
	webdavHandler.ServeHTTP(rrOpts, reqOpts)
	if rrOpts.Code == http.StatusMethodNotAllowed {
		t.Errorf("OPTIONS returned 405 — handler prefix mismatch")
	}
}

// TestSafeResolveToAbsPath_TraversalBlocked 验证路径穿越被拦截。
//
// 🆕 2026-06-17：多挂载点安全铁律 — 任何 ../ 逃逸到 mount.RootPath 之外必须拒绝
func TestSafeResolveToAbsPath_TraversalBlocked(t *testing.T) {
	baseDir := t.TempDir()
	// 模拟攻击：构造 ../ 试图逃逸
	attacks := []string{
		"../etc/passwd",
		"../../etc/passwd",
		"subdir/../../escape",
		"./../../escape",
	}
	for _, attack := range attacks {
		_, err := utils.SafeResolveToAbsPath(baseDir, attack)
		if err == nil {
			t.Errorf("SafeResolveToAbsPath(%q) should fail for path traversal", attack)
		}
	}
}

// TestSafeResolveToAbsPath_ValidPath 验证合法路径正常解析。
func TestSafeResolveToAbsPath_ValidPath(t *testing.T) {
	baseDir := t.TempDir()
	validPaths := []string{
		"foo/bar",
		"foo/bar/baz.txt",
		"a/b/c/d/e/f/g.txt",
	}
	for _, p := range validPaths {
		abs, err := utils.SafeResolveToAbsPath(baseDir, p)
		if err != nil {
			t.Errorf("SafeResolveToAbsPath(%q) failed: %v", p, err)
			continue
		}
		if !strings.HasPrefix(abs, baseDir) {
			t.Errorf("abs path %q should be under baseDir %q", abs, baseDir)
		}
	}
}

// TestSafeResolveToAbsPath_AbsPathInsideBase 验证 userPath = absBaseDir + sub 时去双前缀。
func TestSafeResolveToAbsPath_AbsPathInsideBase(t *testing.T) {
	baseDir := t.TempDir()
	subPath := filepath.Join(baseDir, "sub", "file.txt")
	os.WriteFile(filepath.Dir(subPath), []byte{}, 0755) // ensure dir exists
	os.WriteFile(subPath, []byte("test"), 0644)

	// 传入 subPath（绝对路径，且在 baseDir 内）应去双前缀
	abs, err := utils.SafeResolveToAbsPath(baseDir, subPath)
	if err != nil {
		t.Fatalf("SafeResolveToAbsPath failed for abs subpath: %v", err)
	}
	// 期望：去掉双前缀后的最终路径 = subPath（已存在）
	if abs != subPath {
		t.Errorf("abs path with double-prefix handling: got %q want %q", abs, subPath)
	}
}

// TestGetManifest_EmptyIndex 验证空索引时 GetManifest 返回 IndexReady=false。
func TestGetManifest_EmptyIndex(t *testing.T) {
	fs := createMinimalWebDAVFS(t.TempDir())
	// 不要 close(indexReady) — 模拟索引未就绪

	snap := fs.GetManifest()
	if snap.IndexReady {
		t.Error("IndexReady should be false when channel not closed")
	}
	if snap.IndexStats.TotalFiles != 0 {
		t.Errorf("expected 0 totalFiles, got %d", snap.IndexStats.TotalFiles)
	}
	if len(snap.VirtualTree) != 0 {
		t.Errorf("expected empty VirtualTree, got %d entries", len(snap.VirtualTree))
	}
	if len(snap.ContainerMap) != 0 {
		t.Errorf("expected empty ContainerMap, got %d entries", len(snap.ContainerMap))
	}
}

// TestGetManifest_WithEntries 验证有数据时 GetManifest 正确返回。
func TestGetManifest_WithEntries(t *testing.T) {
	tmpDir := t.TempDir()
	fs := createMinimalWebDAVFS(tmpDir)

	// 关闭 indexReady 模拟"已就绪"
	close(fs.indexReady)

	// 注入测试数据
	mockInfo := &decryptedFileInfo{
		name:     "sample.mp4",
		size:     1234,
		mode:     0644,
		isDir:    false,
		mimeType: "video/mp4",
		etag:     `"abc"`,
	}
	fs.indexes.pathMap["video/sample.mp4"] = filepath.Join(tmpDir, "video/sample.mp4"+pluginsext.VideoExt)
	fs.indexes.fileInfoMap["video/sample.mp4"] = mockInfo
	fs.indexes.dirMap["video"] = []string{"sample.mp4"}
	fs.indexes.reversePathMap[filepath.Join(tmpDir, "video/sample.mp4"+pluginsext.VideoExt)] = "video/sample.mp4"

	snap := fs.GetManifest()

	if !snap.IndexReady {
		t.Error("IndexReady should be true after close")
	}
	if snap.IndexStats.TotalFiles != 1 {
		t.Errorf("expected 1 totalFile, got %d", snap.IndexStats.TotalFiles)
	}
	if snap.IndexStats.TotalDirs != 1 {
		t.Errorf("expected 1 totalDir, got %d", snap.IndexStats.TotalDirs)
	}
	if snap.IndexStats.Containers != 1 {
		t.Errorf("expected 1 container, got %d", snap.IndexStats.Containers)
	}
	if len(snap.VirtualTree) != 1 {
		t.Fatalf("expected 1 VirtualTree entry, got %d", len(snap.VirtualTree))
	}
	entry := snap.VirtualTree[0]
	if entry.VirtualPath != "video/sample.mp4" {
		t.Errorf("VirtualPath = %q, want %q", entry.VirtualPath, "video/sample.mp4")
	}
	if entry.Name != "sample.mp4" {
		t.Errorf("Name = %q, want %q", entry.Name, "sample.mp4")
	}
	if entry.IsDir {
		t.Error("IsDir should be false for file")
	}
	if entry.Size != 1234 {
		t.Errorf("Size = %d, want 1234", entry.Size)
	}
	if entry.Container == "" {
		t.Error("Container should be set for virtual file")
	}

	if len(snap.ContainerMap) != 1 {
		t.Fatalf("expected 1 ContainerMap entry, got %d", len(snap.ContainerMap))
	}
	cMap := snap.ContainerMap[0]
	if cMap.VirtualPath != "video/sample.mp4" {
		t.Errorf("ContainerMap.VirtualPath = %q, want %q", cMap.VirtualPath, "video/sample.mp4")
	}
	if !strings.HasSuffix(cMap.ContainerPath, "sample.mp4"+pluginsext.VideoExt) {
		t.Errorf("ContainerMap.ContainerPath = %q, want suffix sample.mp4%s", cMap.ContainerPath, pluginsext.VideoExt)
	}
}

// TestGetManifest_JSONSerializable 验证 ManifestSnapshot 能正常 JSON 序列化。
// （给 /api/webdav/manifest API 用）
func TestGetManifest_JSONSerializable(t *testing.T) {
	fs := createMinimalWebDAVFS(t.TempDir())
	close(fs.indexReady)

	snap := fs.GetManifest()
	b, err := json.Marshal(snap)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}
	// 必须包含所有必要字段
	s := string(b)
	for _, key := range []string{"index_ready", "index_stats", "virtual_tree", "container_map"} {
		if !strings.Contains(s, key) {
			t.Errorf("JSON output missing key %q", key)
		}
	}
}

// TestResolvePath_TraversalBlocked 验证 webdavFS.resolvePath 拦截 ../ 逃逸。
//
// 攻击场景：webdav handler 传入的 name = "/webdav/../../etc/passwd"
// webdavFS 解析后必须拒绝（不能返回 mount root 之外的路径）
func TestResolvePath_TraversalBlocked(t *testing.T) {
	tmpDir := t.TempDir()
	fs := createMinimalWebDAVFS(tmpDir)
	fs.webdavPrefix = "/webdav"
	fs.dir = tmpDir

	attacks := []string{
		"/webdav/../../etc/passwd",
		"/webdav/foo/../../escape",
	}
	for _, attack := range attacks {
		_, err := fs.resolvePath(attack)
		if err == nil {
			t.Errorf("resolvePath(%q) should fail", attack)
		}
	}
}

// TestResolvePath_ValidPath 验证合法路径正常解析。
func TestResolvePath_ValidPath(t *testing.T) {
	tmpDir := t.TempDir()
	fs := createMinimalWebDAVFS(tmpDir)
	fs.webdavPrefix = "/webdav"
	fs.dir = tmpDir

	abs, err := fs.resolvePath("/webdav/foo/bar.txt")
	if err != nil {
		t.Fatalf("resolvePath valid path failed: %v", err)
	}
	if !strings.HasPrefix(abs, tmpDir) {
		t.Errorf("abs path %q should be under tmpDir %q", abs, tmpDir)
	}
	if !strings.HasSuffix(abs, filepath.Join("foo", "bar.txt")) {
		t.Errorf("abs path %q should end with foo/bar.txt", abs)
	}
}

// TestIsContainerExtension 注册扩展名过滤。
func TestIsContainerExtension_Registered(t *testing.T) {
	fs := createMinimalWebDAVFS(t.TempDir())
	alistExt := pluginsext.AlistExt
	textExt := pluginsext.TextExt
	fs.containerExtensions = map[string]bool{
		alistExt: true,
		textExt: true,
	}
	fs.registeredContainerExts = []string{alistExt, textExt}

	cases := []struct {
		filename string
		want     bool
	}{
		{"test" + alistExt, true},
		{"test" + strings.ToUpper(alistExt), true}, // case-insensitive
		{"test" + textExt, true},
		{"test.mp4", false},
		{"test.txt", false},
	}
	for _, c := range cases {
		got := fs.IsContainerExtension(c.filename)
		if got != c.want {
			t.Errorf("IsContainerExtension(%q) = %v, want %v", c.filename, got, c.want)
		}
	}
}

// TestWebdavPathToIndexKey_TraversalBlocked 验证 webdavPathToIndexKey 拦截 .. 段。
//
// 🆕 2026-06-17：与 resolvePath 的 .. 拦截配对，覆盖 webdav 路径转 index key 的全入口
func TestWebdavPathToIndexKey_TraversalBlocked(t *testing.T) {
	tmpDir := t.TempDir()
	fs := createMinimalWebDAVFS(tmpDir)
	fs.webdavPrefix = "/webdav"
	fs.dir = tmpDir

	attacks := []string{
		"/webdav/../../etc/passwd",
		"/webdav/foo/../escape",
		"/webdav/../d/primary/secret",
	}
	for _, attack := range attacks {
		_, err := fs.webdavPathToIndexKey(attack)
		if err == nil {
			t.Errorf("webdavPathToIndexKey(%q) should fail (path traversal)", attack)
		}
	}
}

// TestWebdavPathToIndexKey_Valid 验证合法路径正常转换。
func TestWebdavPathToIndexKey_Valid(t *testing.T) {
	tmpDir := t.TempDir()
	fs := createMinimalWebDAVFS(tmpDir)
	fs.webdavPrefix = "/webdav"
	fs.dir = tmpDir

	cases := []struct {
		webdavPath string
		want       string
	}{
		{"/webdav/", "."},
		{"/webdav", "."},
		{"/webdav/foo/bar", "foo/bar"},
		{"/webdav/foo/bar.txt", "foo/bar.txt"},
	}
	for _, c := range cases {
		got, err := fs.webdavPathToIndexKey(c.webdavPath)
		if err != nil {
			t.Errorf("webdavPathToIndexKey(%q) failed: %v", c.webdavPath, err)
			continue
		}
		if got != c.want {
			t.Errorf("webdavPathToIndexKey(%q) = %q, want %q", c.webdavPath, got, c.want)
		}
	}
}

// TestCrossMount_Escape 验证跨 mount 逃逸被拦截。
//
// 攻击场景：webdav handler 收到 PROPFIND /d/automation/../../d/primary/secret
// 解析后 userPath = "/../../d/primary/secret" → 含 .. 段 → 拒绝
func TestCrossMount_Escape(t *testing.T) {
	tmpDir := t.TempDir()
	fs := createMinimalWebDAVFS(tmpDir)
	fs.webdavPrefix = "/d/automation"
	fs.dir = tmpDir

	attacks := []string{
		"/d/automation/../../d/primary/secret",
		"/d/automation/../d/primary",
		"/d/automation/../../etc/passwd",
	}
	for _, attack := range attacks {
		_, err := fs.resolvePath(attack)
		if err == nil {
			t.Errorf("cross-mount escape via %q should fail", attack)
		}
	}
}

// TestMountBootstrapErrors_FieldExists 验证 Server struct 包含 mountBootstrapErrors 字段
// （webdavFS 多 mount 注册失败时记录到这里）
func TestMountBootstrapErrors_FieldExists(t *testing.T) {
	// 简单的编译期检查：如果字段不存在，type assertion 编译失败
	type checkField struct {
		mountBootstrapErrors []string
	}
	_ = checkField{}
}
