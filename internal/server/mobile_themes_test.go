package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// newThemeTestServer 起一个假「远程主题源」——提供 theme.css / theme.js / theme.json。
func newThemeTestServer() *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/themes/cool/theme.css", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/css")
		_, _ = w.Write([]byte("body{color:red}"))
	})
	mux.HandleFunc("/themes/cool/theme.js", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/javascript")
		_, _ = w.Write([]byte("export const mount=()=>{}"))
	})
	mux.HandleFunc("/themes/cool/theme.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"cool","name":"Cool","css":"theme.css","js":"theme.js"}`))
	})
	return httptest.NewServer(mux)
}

func newThemeTestRouter(s *Server) *gin.Engine {
	r := gin.New()
	r.POST("/api/themes/pull", s.HandleThemePull)
	r.DELETE("/api/themes/:id", s.HandleThemeDelete)
	r.GET("/themes/*filepath", s.HandleThemeStatic)
	return r
}

// newTestServerWithBothDirs 构造同时设置 servingDir（用户媒体）与 themesDir（数据目录）的 Server，
// 用于验证「应用数据绝不落 servingDir」这一硬约束。
func newTestServerWithBothDirs(t *testing.T) (*Server, string, string) {
	t.Helper()
	tmp := t.TempDir()
	serving := filepath.Join(tmp, "serving") // 模拟用户媒体（/workspace 或 /storage/emulated/0）
	themes := filepath.Join(tmp, "themes-data")
	return &Server{servingDir: serving, themesDir: themes}, serving, themes
}

func TestThemePull_AndStatic_AndDelete(t *testing.T) {
	gin.SetMode(gin.TestMode)
	remote := newThemeTestServer()
	defer remote.Close()

	s, serving, themes := newTestServerWithBothDirs(t)
	r := newThemeTestRouter(s)

	// 1) pull（完整：css + js + manifest 镜像）
	body, _ := json.Marshal(map[string]any{
		"id":        "cool",
		"sourceUrl": remote.URL + "/themes/cool",
		"manifest": map[string]any{
			"id":  "cool",
			"css": remote.URL + "/themes/cool/theme.css",
			"js":  remote.URL + "/themes/cool/theme.js",
		},
		"cssOnly": false,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/themes/pull", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("pull status = %d, body=%s", rec.Code, rec.Body.String())
	}

	dir := s.themeDir("cool")
	if _, err := os.Stat(filepath.Join(dir, "theme.css")); err != nil {
		t.Errorf("theme.css not pulled: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "theme.js")); err != nil {
		t.Errorf("theme.js not pulled: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "theme.json")); err != nil {
		t.Errorf("theme.json mirror not written: %v", err)
	}

	// 1b) 硬约束：pull 只写数据目录 themes，绝不写 servingDir（用户媒体）。
	if _, err := os.Stat(filepath.Join(serving, "themes", "cool")); !os.IsNotExist(err) {
		t.Errorf("VIOLATION: pull leaked into servingDir/themes (user media): %s", filepath.Join(serving, "themes", "cool"))
	}
	if _, err := os.Stat(filepath.Join(serving, ".encv")); !os.IsNotExist(err) {
		t.Errorf("VIOLATION: pull created .encv under servingDir: %s", filepath.Join(serving, ".encv"))
	}
	// 且数据必须真的落在 themes 数据目录。
	if !strings.HasPrefix(dir, themes) {
		t.Errorf("pull did not land in themes data dir: %s", dir)
	}

	// 2) 同源静态服务 /themes/cool/theme.css（只查数据目录）
	reqS := httptest.NewRequest(http.MethodGet, "/themes/cool/theme.css", nil)
	recS := httptest.NewRecorder()
	r.ServeHTTP(recS, reqS)
	if recS.Code != http.StatusOK {
		t.Fatalf("static serve status = %d", recS.Code)
	}
	if recS.Body.String() != "body{color:red}" {
		t.Errorf("static content mismatch: %q", recS.Body.String())
	}

	// 3) 路径穿越防护：../ 应 404
	reqT := httptest.NewRequest(http.MethodGet, "/themes/cool/../../server.go", nil)
	recT := httptest.NewRecorder()
	r.ServeHTTP(recT, reqT)
	if recT.Code == http.StatusOK {
		t.Errorf("path traversal should be blocked, got 200")
	}

	// 4) 非法 id 拒绝
	reqBad := httptest.NewRequest(http.MethodPost, "/api/themes/pull", bytes.NewReader(mustJSON(map[string]any{
		"id": "../evil", "manifest": map[string]any{"css": remote.URL + "/themes/cool/theme.css"},
	})))
	recBad := httptest.NewRecorder()
	r.ServeHTTP(recBad, reqBad)
	if recBad.Code != http.StatusBadRequest {
		t.Errorf("invalid id should be 400, got %d", recBad.Code)
	}

	// 5) delete
	reqD := httptest.NewRequest(http.MethodDelete, "/api/themes/cool", nil)
	recD := httptest.NewRecorder()
	r.ServeHTTP(recD, reqD)
	if recD.Code != http.StatusOK {
		t.Fatalf("delete status = %d", recD.Code)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("theme dir should be removed")
	}
}

func TestThemePull_CssOnly(t *testing.T) {
	gin.SetMode(gin.TestMode)
	remote := newThemeTestServer()
	defer remote.Close()

	s, serving, themes := newTestServerWithBothDirs(t)
	r := newThemeTestRouter(s)

	body, _ := json.Marshal(map[string]any{
		"id":        "bare",
		"sourceUrl": remote.URL + "/themes/cool/theme.css",
		"manifest":  map[string]any{"css": remote.URL + "/themes/cool/theme.css"},
		"cssOnly":   true,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/themes/pull", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("cssOnly pull status = %d, body=%s", rec.Code, rec.Body.String())
	}
	dir := s.themeDir("bare")
	if _, err := os.Stat(filepath.Join(dir, "theme.css")); err != nil {
		t.Errorf("cssOnly: theme.css not pulled: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "theme.js")); !os.IsNotExist(err) {
		t.Errorf("cssOnly: theme.js should NOT be downloaded")
	}
	// 硬约束：cssOnly 也不许落 servingDir。
	if _, err := os.Stat(filepath.Join(serving, "themes", "bare")); !os.IsNotExist(err) {
		t.Errorf("VIOLATION: cssOnly pull leaked into servingDir/themes")
	}
	if !strings.HasPrefix(dir, themes) {
		t.Errorf("cssOnly pull did not land in themes data dir: %s", dir)
	}
}

// TestThemeStatic_NeverServesFromServingDir 锁硬约束：即使用户媒体目录 servingDir/themes
// 里碰巧有文件，HandleThemeStatic 也【绝不】服务它（返回 404）—— app 数据不许从 servingDir 出。
func TestThemeStatic_NeverServesFromServingDir(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s, serving, _ := newTestServerWithBothDirs(t)
	// 故意在 servingDir/themes/builtin/theme.css 放一份「内置主题」——模拟历史错误落点。
	builtinDir := filepath.Join(serving, "themes", "builtin")
	if err := os.MkdirAll(builtinDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(builtinDir, "theme.css"), []byte("*{color:blue}"), 0o644); err != nil {
		t.Fatal(err)
	}

	r := newThemeTestRouter(s)
	req := httptest.NewRequest(http.MethodGet, "/themes/builtin/theme.css", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code == http.StatusOK {
		t.Fatalf("VIOLATION: HandleThemeStatic must NOT serve from servingDir (user media), got 200 with body %q", rec.Body.String())
	}
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404 for app data in servingDir, got %d", rec.Code)
	}
}

// TestThemeDataPath_NotServingDir 锁定用户要求：用户主题数据必须落在【数据目录】，
// 绝不混进 servingDir（静态 web 根/用户媒体；Android 上是私有只读打包资产，写不进去也不该混）。
// 续43 二次修订：最初错把主题写进 servingDir/.encv/themes，用户指出后改为 themeDataPath() 派生。
func TestThemeDataPath_NotServingDir(t *testing.T) {
	// Linux dev：应落在 $XDG_DATA_HOME/encv-dev/themes，与 servingDir 无关。
	t.Setenv("ENCV_MOBILE", "")
	t.Setenv("ENCV_DEV", "1")
	t.Setenv("XDG_DATA_HOME", "/home/x/.local/share")
	t.Setenv("HOME", "/home/x")
	got := themeDataPath()
	want := "/home/x/.local/share/encv-dev/themes"
	if got != want {
		t.Errorf("linux dev themeDataPath mismatch:\n got %q\nwant %q", got, want)
	}
	if strings.Contains(got, "servingDir") {
		t.Errorf("themeDataPath must not reference servingDir: %q", got)
	}

	// Android：应落在 app 私有 files 目录（可写、不污染媒体视图），而非 /storage/emulated/0。
	t.Setenv("ENCV_MOBILE", "1")
	t.Setenv("ENCV_APP_FILES_DIR", "/data/user/0/com.encvgo.app/files")
	gotA := themeDataPath()
	wantA := "/data/user/0/com.encvgo.app/files/.encv/themes"
	if gotA != wantA {
		t.Errorf("android themeDataPath mismatch:\n got %q\nwant %q", gotA, wantA)
	}
	if !strings.Contains(gotA, "/data/user/0/") {
		t.Errorf("android themeDataPath must be app-private path: %q", gotA)
	}
}

func mustJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}
