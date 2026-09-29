package server

// 预览页资源（/preview-assets）的回归锁。
//
// 要守住的核心承诺：**页面资源在可写数据目录里，不在 Go 二进制 / APK 里**，
// 所以更新页面不需要换 APK。三条用例分别对应该承诺的三个环节：
//   1. 装了资源后能正常提供（含 Range —— 大容器流式打开全靠它）；
//   2. 从 zip / 目录导入后立刻生效，且是**原子替换**（失败不留半个新版本）；
//   3. 资源目录外的东西取不到（路径穿越）。

import (
	"archive/zip"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Soltus/encv-go/internal/config"
	"github.com/gin-gonic/gin"
)

// seedPreviewAssets 造一个最小可用的预览页资源目录（够 ServeContent 与 require 校验用）。
func seedPreviewAssets(t *testing.T, root string, body string) {
	t.Helper()
	mustWrite := func(rel, content string) {
		abs := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", abs, err)
		}
		if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", abs, err)
		}
	}
	mustWrite("index.html", "<html><body>"+body+"</body></html>")
	mustWrite("main.js", "console.log('"+body+"')")
	mustWrite("wasm/encv-container.wasm", "fake-wasm-"+body)
}

func doPreviewAssetsGet(t *testing.T, s *Server, path string, header http.Header) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/preview-assets"+path, nil)
	for k, vs := range header {
		for _, v := range vs {
			c.Request.Header.Add(k, v)
		}
	}
	c.Params = gin.Params{{Key: "filepath", Value: path}}
	s.handlePreviewAssetsGin(c)
	return w
}

func TestPreviewAssets_NotInstalledGivesActionableHint(t *testing.T) {
	empty := t.TempDir()
	t.Setenv("ENCV_PREVIEW_ASSETS_DIR", filepath.Join(empty, "preview-assets"))

	w := doPreviewAssetsGet(t, &Server{}, "/", nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("未装资源时应 404，实际 %d（body=%s）", w.Code, w.Body.String())
	}
	// 报错必须告诉调用方"怎么装上"，否则这条路径只能靠猜
	for _, want := range []string{"/api/preview-assets/import", "preview assets not installed"} {
		if !strings.Contains(w.Body.String(), want) {
			t.Errorf("404 响应应包含 %q，实际：%s", want, w.Body.String())
		}
	}
}

func TestPreviewAssets_ServesIndexAndRange(t *testing.T) {
	dir := t.TempDir()
	assets := filepath.Join(dir, "assets")
	seedPreviewAssets(t, assets, "v1")
	t.Setenv("ENCV_PREVIEW_ASSETS_DIR", filepath.Join(dir, "preview-assets"))

	s := &Server{servingDir: dir}
	body, err := json.Marshal(previewAssetsUpdateRequest{Path: assets})
	if err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/preview-assets/import", strings.NewReader(string(body)))
	c.Request.Header.Set("Content-Type", "application/json")
	s.handlePreviewAssetsImportGin(c)
	if w.Code != http.StatusOK {
		t.Fatalf("import 失败：%d %s", w.Code, w.Body.String())
	}

	// ① 根路径与显式 index.html 都要能拿到页面
	for _, p := range []string{"/", "/index.html"} {
		got := doPreviewAssetsGet(t, s, p, nil)
		if got.Code != http.StatusOK {
			t.Fatalf("GET %s 应 200，实际 %d", p, got.Code)
		}
		if !strings.Contains(got.Body.String(), "v1") {
			t.Errorf("GET %s 内容不对：%s", p, got.Body.String())
		}
	}

	// ② Range：大容器"按需取字节"依赖 206，没有它流式打开会退化成整个下载
	ranged := doPreviewAssetsGet(t, s, "/index.html", http.Header{"Range": []string{"bytes=0-9"}})
	if ranged.Code != http.StatusPartialContent {
		t.Fatalf("带 Range 应 206，实际 %d", ranged.Code)
	}
	if cr := ranged.Header().Get("Content-Range"); !strings.HasPrefix(cr, "bytes 0-9/") {
		t.Errorf("206 必须带 Content-Range，实际 %q", cr)
	}
	if ranged.Body.Len() != 10 {
		t.Errorf("206 应只返回 10 字节，实际 %d", ranged.Body.Len())
	}

	// ③ wasm 的 MIME（浏览器侧不依赖它，但排查时一眼能看出类型对不对）
	wasm := doPreviewAssetsGet(t, s, "/wasm/encv-container.wasm", nil)
	if got := wasm.Header().Get("Content-Type"); got != "application/wasm" {
		t.Errorf("wasm 的 Content-Type 应为 application/wasm，实际 %q", got)
	}

	// ④ 路径穿越
	trap := doPreviewAssetsGet(t, s, "/../../etc/passwd", nil)
	if trap.Code == http.StatusOK {
		t.Fatalf("越界路径不应 200，实际 %d（body=%s）", trap.Code, trap.Body.String())
	}
}

func TestPreviewAssets_UpdateFromZipReplacesAtomically(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ENCV_PREVIEW_ASSETS_DIR", filepath.Join(dir, "preview-assets"))
	s := &Server{servingDir: dir}

	// 先装一个 v1（从目录）
	v1 := filepath.Join(dir, "v1")
	seedPreviewAssets(t, v1, "v1")
	importDir(t, s, v1)

	// 打一个 v2 的 zip（带一层包裹目录，模拟真实打包形态）
	src := filepath.Join(dir, "v2src", "encv-preview")
	seedPreviewAssets(t, src, "v2")
	zipPath := filepath.Join(dir, "v2.zip")
	makeZip(t, filepath.Join(dir, "v2src"), zipPath)

	// 用一个真 HTTP 服务提供 zip（走 update 的下载路径，而不是直接读文件）
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, zipPath)
	}))
	defer srv.Close()

	updateZip(t, s, srv.URL+"/v2.zip")

	got := doPreviewAssetsGet(t, s, "/index.html", nil)
	if got.Code != http.StatusOK || !strings.Contains(got.Body.String(), "v2") {
		t.Fatalf("更新后应拿到 v2，实际 %d %s", got.Code, got.Body.String())
	}

	// 版本号应被写入（前端据此判断是否需要更新）
	raw, err := os.ReadFile(previewAssetsVersionFile())
	if err != nil {
		t.Fatalf("更新后应写入 version.json：%v", err)
	}
	var v previewAssetsVersion
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatalf("version.json 解析失败：%v", err)
	}
	if v.Version == "" || v.Source == "" {
		t.Errorf("version.json 内容不完整：%+v", v)
	}

	// 坏包（缺 index.html）必须失败，并且**旧版本还在**（原子替换 + 回滚）
	badSrc := filepath.Join(dir, "badsrc", "encv-preview")
	if err := os.MkdirAll(badSrc, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(badSrc, "main.js"), []byte("only-js"), 0o644); err != nil {
		t.Fatal(err)
	}
	badZip := filepath.Join(dir, "bad.zip")
	makeZip(t, filepath.Join(dir, "badsrc"), badZip)
	badSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, badZip)
	}))
	defer badSrv.Close()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/preview-assets/update", strings.NewReader(`{"url":"`+badSrv.URL+`/bad.zip"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	s.handlePreviewAssetsUpdateGin(c)
	if w.Code == http.StatusOK {
		t.Fatalf("缺文件的资源包不应被接受：%s", w.Body.String())
	}
	still := doPreviewAssetsGet(t, s, "/index.html", nil)
	if still.Code != http.StatusOK || !strings.Contains(still.Body.String(), "v2") {
		t.Fatalf("更新失败后旧版本必须还在（原子替换），实际 %d %s", still.Code, still.Body.String())
	}
}

// 一键更新：不传 url 时用配置里的 preview.assets_url（前端不必知道资源在哪台机器上）。
func TestPreviewAssets_UpdateUsesConfiguredURL(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ENCV_PREVIEW_ASSETS_DIR", filepath.Join(dir, "preview-assets"))

	src := filepath.Join(dir, "v3src", "encv-preview")
	seedPreviewAssets(t, src, "v3")
	zipPath := filepath.Join(dir, "v3.zip")
	makeZip(t, filepath.Join(dir, "v3src"), zipPath)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, zipPath)
	}))
	defer srv.Close()

	s := &Server{
		servingDir: dir,
		cfg:        &config.Config{Preview: &config.PreviewConfig{AssetsURL: srv.URL + "/v3.zip"}},
	}

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/preview-assets/update", strings.NewReader(`{}`))
	c.Request.Header.Set("Content-Type", "application/json")
	s.handlePreviewAssetsUpdateGin(c)
	if w.Code != http.StatusOK {
		t.Fatalf("不带 url 时应走配置地址：%d %s", w.Code, w.Body.String())
	}
	got := doPreviewAssetsGet(t, s, "/index.html", nil)
	if !strings.Contains(got.Body.String(), "v3") {
		t.Fatalf("更新后应拿到 v3：%s", got.Body.String())
	}
}

// 既没传 url 也没配置地址时，报错要指出还能手动导入（否则用户就卡住了）。
func TestPreviewAssets_UpdateWithoutAnyURLGivesHint(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ENCV_PREVIEW_ASSETS_DIR", filepath.Join(dir, "preview-assets"))

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/preview-assets/update", strings.NewReader(`{}`))
	c.Request.Header.Set("Content-Type", "application/json")
	(&Server{servingDir: dir, cfg: &config.Config{}}).handlePreviewAssetsUpdateGin(c)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("应 400，实际 %d %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "/api/preview-assets/import") {
		t.Errorf("报错应指出手动导入这条路，实际：%s", w.Body.String())
	}
}

// 包的版本号以**构建产物**为准：CI 打 zip 时会写 version.json
// （见 .github/workflows/preview-assets.yml）。导入时必须采用它 ——
// 否则每台设备的版本号都是"安装时刻的时间戳"，看不出两台设备装的是不是同一个版本。
func TestPreviewAssets_KeepsVersionFromZip(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ENCV_PREVIEW_ASSETS_DIR", filepath.Join(dir, "preview-assets"))
	s := &Server{servingDir: dir}

	src := filepath.Join(dir, "vsrc", "encv-preview")
	seedPreviewAssets(t, src, "v9")
	if err := os.WriteFile(
		filepath.Join(src, "version.json"),
		[]byte(`{"version":"ci-abc123","source":"ci:preview-assets@deadbeef","updatedAt":"2026-09-30T00:00:00Z"}`),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	zipPath := filepath.Join(dir, "v9.zip")
	makeZip(t, filepath.Join(dir, "vsrc"), zipPath)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, zipPath)
	}))
	defer srv.Close()
	updateZip(t, s, srv.URL+"/v9.zip")

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/preview-assets/version", nil)
	s.handlePreviewAssetsVersionGin(c)

	var got struct {
		Version string `json:"version"`
		Source  string `json:"source"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("version 响应不是 JSON：%v（%s）", err, w.Body.String())
	}
	if got.Version != "ci-abc123" {
		t.Errorf("应采用 zip 自带的版本号，实际 %q（响应 %s）", got.Version, w.Body.String())
	}
	if !strings.Contains(got.Source, "ci:preview-assets") {
		t.Errorf("应保留 zip 里的来源，实际 %q", got.Source)
	}
}

func TestPreviewAssets_VersionReportsWhatIsMissing(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ENCV_PREVIEW_ASSETS_DIR", filepath.Join(dir, "preview-assets"))

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/preview-assets/version", nil)
	(&Server{servingDir: dir}).handlePreviewAssetsVersionGin(c)
	t.Logf("version 响应：%s（解析到的 dir=%s）", w.Body.String(), previewAssetsDir())

	var got struct {
		Installed bool     `json:"installed"`
		Missing   []string `json:"missing"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("version 响应不是 JSON：%v（%s）", err, w.Body.String())
	}
	if got.Installed {
		t.Error("空目录时 installed 应为 false")
	}
	if len(got.Missing) == 0 {
		t.Fatal("应列出缺失的必需文件")
	}
}

// --- 测试辅助 ---

func importDir(t *testing.T, s *Server, path string) {
	t.Helper()
	body, _ := json.Marshal(previewAssetsUpdateRequest{Path: path})
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/preview-assets/import", strings.NewReader(string(body)))
	c.Request.Header.Set("Content-Type", "application/json")
	s.handlePreviewAssetsImportGin(c)
	if w.Code != http.StatusOK {
		t.Fatalf("import 失败：%d %s", w.Code, w.Body.String())
	}
}

func updateZip(t *testing.T, s *Server, url string) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/preview-assets/update", strings.NewReader(`{"url":"`+url+`"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	s.handlePreviewAssetsUpdateGin(c)
	if w.Code != http.StatusOK {
		t.Fatalf("update 失败：%d %s", w.Code, w.Body.String())
	}
}

func makeZip(t *testing.T, srcDir, dest string) {
	t.Helper()
	f, err := os.Create(dest)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	err = filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		hdr, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		hdr.Name = filepath.ToSlash(rel)
		if info.IsDir() {
			hdr.Name += "/"
		} else {
			hdr.Method = zip.Deflate
		}
		w, err := zw.CreateHeader(hdr)
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		_, err = io.Copy(w, in)
		return err
	})
	if err != nil {
		t.Fatalf("打 zip 失败：%v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("关闭 zip 失败：%v", err)
	}
}
