package server

// preview_assets.go —— 容器预览页（encv-preview）的**可热更新**静态托管。
//
// 需求：把预览页集成进安卓应用，且**不更换主 APK 就能更新页面**。
//
// 为什么不能像 `/preview`、`/simverse` 那样用 //go:embed：
// embed 是编译期把资源打进 Go 二进制（也就是打进 APK 里的 libencv-go.so），
// 改一行 JS 都得重打 APK —— 那正是要避免的。
//
// 做法（照 `/themes` 的既有模式）：
//  1. 资源放在**可写数据目录** `config.AppDataDir("preview-assets")`
//     （Android = `<ENCV_APP_FILES_DIR>/.encv/preview-assets`，桌面 dev = `~/.local/share/encv-dev/preview-assets`），
//     既不在 APK 里，也不需要存储权限；
//  2. 从这里用 http.ServeContent 提供 —— 它自带 Range，而"大容器按需取字节"全靠 Range；
//  3. 更新 = 往这个目录写新内容（远端 zip 或设备上已有的 zip/目录），写完即生效：
//     `POST /api/preview-assets/update`（远端）、`POST /api/preview-assets/import`（本地 zip）。
//
// ⚠️ 与首包的关系：APK 里仍然要带一份初始资源（否则第一次打开是空的），
// 但那是**种子**，由安卓侧从 assets 释放后调 import 接口写入这个目录；
// 之后每次更新只动这个目录，APK 与 Go 二进制都不用换。

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Soltus/encv-go/internal/config"
	"github.com/Soltus/encv-go/internal/skills"
	"github.com/gin-gonic/gin"
)

const (
	// previewAssetsSubdir 是可写数据目录下的子目录名（也是环境变量 ENCV_PREVIEW_ASSETS_DIR 的来源）。
	previewAssetsSubdir = "preview-assets"

	// 预览页整包（含 8MB 的 wasm）的下载上限：给足余量，但不能无限。
	previewAssetsMaxZipBytes = 128 << 20
	previewAssetsDownloadTO  = 120 * time.Second
	previewAssetsTimeout     = 60 * time.Second
)

// 预览页至少要有的东西，缺一样就认为"没装好"（前端据此提示导入）。
var previewAssetsRequired = []string{"index.html", "main.js", "wasm/encv-container.wasm"}

var previewAssetsMu sync.Mutex

func init() {
	// Go 的 mime 表默认没有 .wasm。页面用 Go 的 wasm_exec.js（instantiate 而非
	// instantiateStreaming），不靠 MIME 也能加载，但正确的类型对调试和缓存策略都有用。
	_ = mime.AddExtensionType(".wasm", "application/wasm")
}

// previewAssetsDir 返回预览页资源目录（可写，不在 APK 内）。
func previewAssetsDir() string { return config.AppDataDir(previewAssetsSubdir) }

// previewAssetsVersionFile 记录当前版本（更新接口写入）。
func previewAssetsVersionFile() string { return filepath.Join(previewAssetsDir(), "version.json") }

type previewAssetsVersion struct {
	Version   string `json:"version"`
	Source    string `json:"source,omitempty"`
	UpdatedAt string `json:"updatedAt,omitempty"`
}

// handlePreviewAssetsGin 是 GET /preview-assets/*filepath：从可写目录提供静态资源。
//
// 与 internal/openlist/web 那种 embed 版本的关键差别：ServeContent **支持 Range**，
// 而预览页的"大容器流式打开（openStream）"就是靠 Range 逐区间取字节的。
func (s *Server) handlePreviewAssetsGin(c *gin.Context) {
	rel := strings.TrimPrefix(c.Param("filepath"), "/")
	if rel == "" {
		rel = "index.html"
	}
	base := previewAssetsDir()
	abs := filepath.Join(base, filepath.FromSlash(rel))
	// 防穿越：解析后必须仍在资源目录内
	if abs != base && !strings.HasPrefix(abs, base+string(os.PathSeparator)) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad path"})
		return
	}

	st, err := os.Stat(abs)
	if err == nil && st.IsDir() {
		abs = filepath.Join(abs, "index.html")
		if _, err := os.Stat(abs); err != nil {
			s.writePreviewAssetsNotInstalled(c)
			return
		}
	}
	if _, err := os.Stat(abs); err != nil {
		s.writePreviewAssetsNotInstalled(c)
		return
	}

	f, err := os.Open(abs)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer f.Close()
	st, err = f.Stat()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	// ServeContent 自动处理 Range / If-Modified-Since / HEAD
	http.ServeContent(c.Writer, c.Request, filepath.Base(abs), st.ModTime(), f)
}

func (s *Server) writePreviewAssetsNotInstalled(c *gin.Context) {
	c.JSON(http.StatusNotFound, gin.H{
		"error": "preview assets not installed",
		"dir":   previewAssetsDir(),
		"hint":  "用 POST /api/preview-assets/update（远端 zip）或 /api/preview-assets/import（设备上 zip/目录）写入资源；这一步不需要换 APK",
	})
}

// handlePreviewAssetsVersionGin 是 GET /api/preview-assets/version：装了没有、版本号、缺哪些文件。
func (s *Server) handlePreviewAssetsVersionGin(c *gin.Context) {
	dir := previewAssetsDir()
	info := gin.H{"dir": dir, "installed": true, "missing": []string{}}
	var missing []string
	for _, need := range previewAssetsRequired {
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(need))); err != nil {
			missing = append(missing, need)
		}
	}
	info["missing"] = missing
	info["installed"] = len(missing) == 0

	var v previewAssetsVersion
	if raw, err := os.ReadFile(previewAssetsVersionFile()); err == nil {
		_ = json.Unmarshal(raw, &v)
	}
	info["version"] = v.Version
	info["source"] = v.Source
	info["updatedAt"] = v.UpdatedAt
	c.JSON(http.StatusOK, info)
}

type previewAssetsUpdateRequest struct {
	URL  string `json:"url"`
	Path string `json:"path"` // import 用：设备上已有的 zip（或目录）
}

// handlePreviewAssetsImportGin 是 POST /api/preview-assets/import：把设备上的 zip/目录装进资源目录。
//
// 这是"不换 APK"最直接的一条路：APK 自带的种子资源、用户放到下载目录的新版本，
// 都通过它进入资源目录 —— 全程只是写文件，不涉及 APK / Go 二进制。
func (s *Server) handlePreviewAssetsImportGin(c *gin.Context) {
	var req previewAssetsUpdateRequest
	if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON: " + err.Error()})
		return
	}
	if req.Path == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path is required"})
		return
	}
	target, err := s.resolveUserPath(req.Path)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path 不在可访问的挂载目录内：" + err.Error()})
		return
	}
	st, err := os.Stat(target)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "找不到该文件：" + err.Error()})
		return
	}

	previewAssetsMu.Lock()
	defer previewAssetsMu.Unlock()

	if st.IsDir() {
		if err := installPreviewAssetsFromDir(target); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true, "dir": previewAssetsDir(), "source": "dir:" + target})
		return
	}
	if err := installPreviewAssetsFromZip(target); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "dir": previewAssetsDir(), "source": "zip:" + target})
}

// handlePreviewAssetsUpdateGin 是 POST /api/preview-assets/update：从远端 zip 拉取新版本。
func (s *Server) handlePreviewAssetsUpdateGin(c *gin.Context) {
	var req previewAssetsUpdateRequest
	if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON: " + err.Error()})
		return
	}
	if req.URL == "" {
		// 不传就走配置里的默认地址（preview.assets_url）——
		// 前端"一键更新"不需要知道资源在哪台机器上。
		s.configMu.Lock()
		if s.cfg != nil && s.cfg.Preview != nil {
			req.URL = s.cfg.Preview.AssetsURL
		}
		s.configMu.Unlock()
	}
	if req.URL == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "url is required（或在配置 preview.assets_url 里给一个默认地址）",
			"hint":  "没有远端地址时可以用 POST /api/preview-assets/import 手动导入 zip",
		})
		return
	}
	if !isSafeHTTPURL(req.URL) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "url must be http(s)"})
		return
	}

	previewAssetsMu.Lock()
	defer previewAssetsMu.Unlock()

	tmpZip := filepath.Join(config.AppDataDir("tmp"), "preview-assets-update.zip")
	if err := os.MkdirAll(filepath.Dir(tmpZip), 0o755); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), previewAssetsDownloadTO)
	defer cancel()
	if err := downloadFileLimited(ctx, req.URL, tmpZip, previewAssetsMaxZipBytes); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "下载失败：" + err.Error()})
		return
	}
	defer os.Remove(tmpZip)

	if err := installPreviewAssetsFromZip(tmpZip); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "dir": previewAssetsDir(), "source": "url:" + req.URL})
}

// installPreviewAssetsFromZip 解压 zip 到临时目录，校验完整后**原子替换**资源目录。
//
// 原子性很重要：中途失败不能留下一个"半个新版本"的目录被静态路由服务出去
// （页面会白屏，而且很难排查）。旧目录改名保留，失败时回滚。
func installPreviewAssetsFromZip(zipPath string) error {
	parent := config.AppDataDir("tmp")
	staging := filepath.Join(parent, "preview-assets-staging")
	if err := os.RemoveAll(staging); err != nil {
		return fmt.Errorf("清理暂存目录失败：%w", err)
	}
	// SafeUnzip 防 ZIP-slip（恶意 entry 越界解压）
	if err := skills.SafeUnzip(zipPath, staging); err != nil {
		return fmt.Errorf("解压失败：%w", err)
	}
	return commitPreviewAssets(staging, "zip:"+zipPath)
}

// installPreviewAssetsFromDir 把一个已展开的目录装进资源目录（同样走原子替换）。
func installPreviewAssetsFromDir(src string) error {
	parent := config.AppDataDir("tmp")
	staging := filepath.Join(parent, "preview-assets-staging")
	if err := os.RemoveAll(staging); err != nil {
		return fmt.Errorf("清理暂存目录失败：%w", err)
	}
	if err := copyDir(src, staging); err != nil {
		return fmt.Errorf("复制目录失败：%w", err)
	}
	return commitPreviewAssets(staging, "dir:"+src)
}

// commitPreviewAssets 校验 staging 内容后原子替换正式目录。
func commitPreviewAssets(staging, source string) error {
	// zip 里常见一层包裹目录（encv-preview/…），自动下潜一层
	root := staging
	if entries, err := os.ReadDir(staging); err == nil && len(entries) == 1 && entries[0].IsDir() {
		if _, err := os.Stat(filepath.Join(staging, entries[0].Name(), "index.html")); err == nil {
			root = filepath.Join(staging, entries[0].Name())
		}
	}
	var missing []string
	for _, need := range previewAssetsRequired {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(need))); err != nil {
			missing = append(missing, need)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("资源包缺文件：%s（需要 %s）", strings.Join(missing, "、"), strings.Join(previewAssetsRequired, "、"))
	}

	dir := previewAssetsDir()
	backup := dir + ".old"
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		return fmt.Errorf("创建数据目录失败：%w", err)
	}
	_ = os.RemoveAll(backup)
	if _, err := os.Stat(dir); err == nil {
		if err := os.Rename(dir, backup); err != nil {
			return fmt.Errorf("备份旧资源失败：%w", err)
		}
	}
	if err := os.Rename(root, dir); err != nil {
		// 回滚：旧版本还在 backup 里
		if _, serr := os.Stat(backup); serr == nil {
			_ = os.Rename(backup, dir)
		}
		return fmt.Errorf("替换资源目录失败（已回滚）：%w", err)
	}
	_ = os.RemoveAll(backup)

	// ⚠️ 包的版本号以**构建产物**为准：CI 打 zip 时会写一份 version.json
	// （见 .github/workflows/preview-assets.yml），导入时应当采用它 ——
	// 否则每台设备上看到的安装时间都不同，却无法区分它们装的是不是同一个版本。
	// 注意读的是 **dir**（正式目录）：此时 root 已经被 rename 进来了，
	// 再读 root 只会拿到 ENOENT，然后静默回退到时间戳版本号。
	if raw, err := os.ReadFile(filepath.Join(dir, "version.json")); err == nil {
		var v previewAssetsVersion
		if err := json.Unmarshal(raw, &v); err == nil && v.Version != "" {
			if v.Source == "" {
				v.Source = source
			}
			written, _ := json.Marshal(v)
			_ = os.WriteFile(previewAssetsVersionFile(), written, 0o644)
			return nil
		}
	}

	v := previewAssetsVersion{Version: time.Now().Format("20060102-150405"), Source: source, UpdatedAt: time.Now().UTC().Format(time.RFC3339)}
	raw, _ := json.Marshal(v)
	_ = os.WriteFile(previewAssetsVersionFile(), raw, 0o644)
	return nil
}

// copyDir 递归复制目录（资源量不大，直接顺序复制即可）。
func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode()&0o777)
		if err != nil {
			return err
		}
		defer out.Close()
		_, err = io.Copy(out, in)
		return err
	})
}

// downloadFileLimited 下载到 dest（带超时与大小上限，先写 .part 再 rename）。
//
// 与 themes 的 downloadToFile 同一套约束，只是上限不同（预览页整包比主题大得多）。
func downloadFileLimited(ctx context.Context, rawURL, dest string, maxBytes int64) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: previewAssetsDownloadTO}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("remote returned status %d", resp.StatusCode)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	tmp := dest + ".part"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	n, err := io.Copy(f, io.LimitReader(resp.Body, maxBytes+1))
	if cerr := f.Close(); cerr != nil && err == nil {
		err = cerr
	}
	if err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if n > maxBytes {
		_ = os.Remove(tmp)
		return fmt.Errorf("file too large (>%d bytes)", maxBytes)
	}
	return os.Rename(tmp, dest)
}
