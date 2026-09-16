package server

// mobile_themes.go — 续43 修订：本地优先主题存储的 Go 后端落地。
//
// 前端（shared-components）通过 ThemeStorage 端口只声明意图（pullToLocal /
// removeLocal），具体「字节下载到哪、怎么存」由本文件在 Go 后端实现，与框架解耦。
//
// 契约（前端 registerSharedThemeStorage 调用）：
//   - POST /api/themes/pull  body {id, sourceUrl, manifest{css,js,assets...}, cssOnly}
//       → 把远程主题下载到 s.themesDir/<id>/（即 themeDataPath()，见下；同源 /themes/<id>/）
//   - DELETE /api/themes/:id → 删除该本地目录
//   - GET  /themes/*filepath → 只从 s.themesDir(数据目录) 提供用户安装主题；绝不读 servingDir(用户媒体)
//
// 关键：s.themesDir 是【数据目录】（themeDataPath() 派生，与 kernel/simverse 同脉络），
// **绝不**是 servingDir（静态 web 根）。Android 落在 app 私有 files 目录（可写、不污染媒体视图），
// 桌面端走 XDG / 标准路径。主题「本地同一目录」即 Go 后端托管的同源 /themes/<id>/，
// 与内置主题同形态、同加载机制；与 dev 网关（preview-gateway 默认把未知路径转发到 encv-go :2025）
// 天然兼容，无需额外代理。

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"log/slog"
)

const (
	// themeMaxFileBytes 单个主题文件大小上限（防滥用）。
	themeMaxFileBytes = 10 << 20 // 10 MB
	// themeDownloadTimeout 单个文件下载超时。
	themeDownloadTimeout = 30 * time.Second
)

// themePullMu 串行化「拉取」动作（低频管理操作，避免同 id 并发半写）。
var themePullMu sync.Mutex

var themeIDRe = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// themePullRequest 是 POST /api/themes/pull 的请求体。
// manifest.css / manifest.js 由前端 fetchThemeManifest 解析为绝对 URL 后传入。
type themePullRequest struct {
	ID        string `json:"id"`
	SourceURL string `json:"sourceUrl"`
	Manifest  struct {
		ID     string   `json:"id"`
		Name   string   `json:"name"`
		CSS    string   `json:"css"`
		JS     string   `json:"js"`
		Assets []string `json:"assets"`
	} `json:"manifest"`
	CssOnly bool `json:"cssOnly"`
}

// themesRoot 返回所有主题落盘的根目录（来自 s.themesDir，即 themeDataPath() 派生，
// 与 servingDir 严格分离 —— 见文件头注释）。
func (s *Server) themesRoot() string {
	return s.themesDir
}

// themeDir 返回某主题落盘的本地目录。
func (s *Server) themeDir(id string) string {
	return filepath.Join(s.themesDir, id)
}

func isValidThemeID(id string) bool {
	return themeIDRe.MatchString(id)
}

func isSafeHTTPURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	return u.Scheme == "http" || u.Scheme == "https"
}

// resolveAssetURL 把清单里的相对 asset 路径解析为绝对 URL。
// 以 manifest.css 所在目录（即 theme.css 的父目录）为基准。
func resolveAssetURL(cssURL, asset string) (string, bool) {
	base := strings.TrimSuffix(cssURL, "/theme.css")
	if base == cssURL {
		return "", false
	}
	asset = strings.TrimPrefix(asset, "./")
	asset = strings.TrimPrefix(asset, "/")
	return base + "/" + asset, true
}

// downloadToFile 下载 url 到 dest（带超时 + 大小上限 + 路径安全）。
func downloadToFile(ctx context.Context, rawURL, dest string) error {
	client := &http.Client{Timeout: themeDownloadTimeout}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("remote returned status %d", resp.StatusCode)
	}
	// 先落到临时文件，成功后再 rename，避免半截文件被静态路由服务出去。
	tmp := dest + ".part"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	limited := io.LimitReader(resp.Body, themeMaxFileBytes+1)
	n, err := io.Copy(f, limited)
	if cerr := f.Close(); cerr != nil && err == nil {
		err = cerr
	}
	if err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if n > themeMaxFileBytes {
		_ = os.Remove(tmp)
		return fmt.Errorf("file too large (>%d bytes)", themeMaxFileBytes)
	}
	return os.Rename(tmp, dest)
}

// HandleThemePull 是 POST /api/themes/pull 的入口：把远程主题拉取到后端本地同一目录。
func (s *Server) HandleThemePull(c *gin.Context) {
	if c.Request.Method != http.MethodPost {
		c.JSON(http.StatusMethodNotAllowed, gin.H{"error": "method not allowed"})
		return
	}
	var req themePullRequest
	if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON: " + err.Error()})
		return
	}
	if !isValidThemeID(req.ID) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid theme id (allowed: [A-Za-z0-9_-], ≤64)"})
		return
	}
	if req.Manifest.CSS == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "manifest.css is required"})
		return
	}
	if !isSafeHTTPURL(req.Manifest.CSS) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "manifest.css must be an http(s) URL"})
		return
	}

	themePullMu.Lock()
	defer themePullMu.Unlock()

	dir := s.themeDir(req.ID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "mkdir themes dir: " + err.Error()})
		return
	}

	if req.CssOnly {
		// 裸 .css 直链：只下载单个 css，无 assets / theme.js。
		if err := downloadToFile(c.Request.Context(), req.Manifest.CSS, filepath.Join(dir, "theme.css")); err != nil {
			_ = os.RemoveAll(dir)
			c.JSON(http.StatusBadGateway, gin.H{"error": "download css failed: " + err.Error()})
			return
		}
		writeThemeManifest(dir, req.ID, "", nil)
	} else {
		if err := downloadToFile(c.Request.Context(), req.Manifest.CSS, filepath.Join(dir, "theme.css")); err != nil {
			_ = os.RemoveAll(dir)
			c.JSON(http.StatusBadGateway, gin.H{"error": "download css failed: " + err.Error()})
			return
		}
		// theme.js 钩子（最佳努力：失败不阻断 css 主题）。
		if req.Manifest.JS != "" && isSafeHTTPURL(req.Manifest.JS) {
			if err := downloadToFile(c.Request.Context(), req.Manifest.JS, filepath.Join(dir, "theme.js")); err != nil {
				slog.Warn("theme pull: js download failed, continue with css-only", "id", req.ID, "err", err)
			}
		}
		// 镜像清单，使目录自描述（与 themeLoader 的 themes/<id>/theme.json 契约一致）。
		writeThemeManifest(dir, req.Manifest.ID, req.Manifest.Name, req.Manifest.Assets)
		// 下载清单列出的 assets（最佳努力）。
		for _, asset := range req.Manifest.Assets {
			if aurl, ok := resolveAssetURL(req.Manifest.CSS, asset); ok && isSafeHTTPURL(aurl) {
				dest := filepath.Join(dir, sanitizeRelPath(asset))
				if err := os.MkdirAll(filepath.Dir(dest), 0o755); err == nil {
					if err := downloadToFile(c.Request.Context(), aurl, dest); err != nil {
						slog.Warn("theme pull: asset download failed, skip", "id", req.ID, "asset", asset, "err", err)
					}
				}
			}
		}
	}

	slog.Info("theme pulled to local", "id", req.ID, "cssOnly", req.CssOnly, "dir", dir)
	c.JSON(http.StatusOK, gin.H{"ok": true, "id": req.ID, "path": "/themes/" + req.ID + "/"})
}

// writeThemeManifest 写一份自描述 theme.json 到主题目录。
func writeThemeManifest(dir, id, name string, assets []string) {
	out := map[string]any{"id": id, "css": "theme.css"}
	if name != "" {
		out["name"] = name
	}
	if _, err := os.Stat(filepath.Join(dir, "theme.js")); err == nil {
		out["js"] = "theme.js"
	}
	if len(assets) > 0 {
		out["assets"] = assets
	}
	if b, err := json.MarshalIndent(out, "", "  "); err == nil {
		_ = os.WriteFile(filepath.Join(dir, "theme.json"), b, 0o644)
	}
}

// sanitizeRelPath 把清单里的相对 asset 路径规整为无穿越的安全相对路径。
func sanitizeRelPath(asset string) string {
	clean := filepath.Clean("/" + strings.TrimPrefix(asset, "./"))
	clean = strings.TrimPrefix(clean, "/")
	return clean
}

// HandleThemeDelete 是 DELETE /api/themes/:id 的入口：删除后端本地主题目录。
func (s *Server) HandleThemeDelete(c *gin.Context) {
	id := c.Param("id")
	if !isValidThemeID(id) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid theme id"})
		return
	}
	dir := s.themeDir(id)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, gin.H{"error": "theme not installed"})
		return
	}
	if err := os.RemoveAll(dir); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "remove theme dir: " + err.Error()})
		return
	}
	slog.Info("theme removed from local", "id", id)
	c.JSON(http.StatusOK, gin.H{"ok": true, "id": id})
}

// HandleThemeStatic 是 GET /themes/*filepath 的入口：只从【数据目录】s.themesDir 提供。
//
// 关键约束（2026-07-16 用户硬要求）：s.servingDir 是【用户媒体/内容】目录
// （dev=/workspace、Android mobile overlay=/storage/emulated/0），**应用任何数据都不许存在**。
// 因此本路由【绝不】回退到 servingDir —— 即便 servingDir/themes 里碰巧有文件也忽略（返回 404）。
//
// 内置主题不归 encv-go 管：它们随 SPA 打包，由 SPA 自己的资产管线提供
// （dev: Vite 从 public/themes 服务；生产: WebView 从 file:///android_asset 加载）。
// 用户安装的远程主题才落 s.themesDir（数据目录），由本路由同源 /themes/<id>/ 提供。
//
// 只做数据目录内的路径穿越防护；命中即返回，否则 404。
func (s *Server) HandleThemeStatic(c *gin.Context) {
	rel := c.Param("filepath") // 带前导斜杠，如 /remote-cool/theme.css
	segs := strings.Split(strings.Trim(rel, "/"), "/")
	if len(segs) == 0 || !isValidThemeID(segs[0]) {
		c.Status(http.StatusNotFound)
		return
	}
	root := s.themesRoot() // = s.themesDir（数据目录，与 servingDir 严格分离）
	full := filepath.Join(root, rel)
	// 防穿越：确保解析结果仍在数据目录内。
	if !strings.HasPrefix(full, root+string(os.PathSeparator)) {
		c.Status(http.StatusNotFound)
		return
	}
	info, err := os.Stat(full)
	if err != nil || info.IsDir() {
		c.Status(http.StatusNotFound)
		return
	}
	c.Header("Cache-Control", "public, max-age=3600")
	c.File(full)
}
