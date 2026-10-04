package server

// peerlink_bundle.go —— 云控热更新的 **Hub 侧**（2026-10-04）
//
// 已有基础：容器预览页（preview-assets）早就做到"不换 APK 整包替换页面资源"
// （可写数据目录 + 远端 zip + staging 原子替换，见 internal/server/preview_assets.go
//  与 PreviewAssetsActivity 头注释）。本文件把这条能力**抬到云控层**：
//
//	Hub（云/桌面）持有资源包仓库  →  GET  /api/peerlink/bundle/manifest   设备查可用版本
//	                             →  GET  /api/peerlink/bundle/download   设备拉 zip（数据面，HTTP）
//	                             →  POST /api/peerlink/bundle/push        云控下发：让某设备升到某版本
//	                             →  POST /api/peerlink/bundle/report      设备回报成功/失败/回滚
//	                             →  GET  /api/peerlink/bundle/status      云控视角：谁装到哪版了
//
// 为什么 push 走 WS RPC、download 走 HTTP：
//   - push 是**控制面**指令，几十字节，走已有 Hub→Edge 长连接（peerCalls）最省事，
//     且手机在 NAT 后、没有可被 Hub 直连的地址 ⇒ 只能走它主动出网的那条连接；
//   - zip 是**数据面**，几 MB～几十 MB，走 HTTP 能带 Content-Length / 断点续传，
//     也不会长时间占住那条控制连接（R13：控制面小报文）。
//
// 安全：
//   - manifest/download/report 都要 **peer token**（未配对一律 401，与其它 peerlink 端点同纪律）
//   - push/status 是**运维**接口（X-Peerlink-Operator），因为"让谁升级"是控制动作
//   - download 的 name/version 必须过白名单校验，绝不能拼进路径（防穿越）

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Soltus/encv-go/internal/config"
	"github.com/Soltus/encv-go/internal/peerlink"
	"github.com/gin-gonic/gin"
)

// bundlesSubdir Hub 侧资源包仓库所在子目录（可写应用数据目录）。
const bundlesSubdir = "bundles"

// bundleMaxZipBytes 单包上限（与 preview-assets 同量级：整包含 wasm 也不超过它）。
const bundleMaxZipBytes = 128 << 20

// BundleManifestItem 清单里的一项。
type BundleManifestItem struct {
	Name      string `json:"name"`
	Version   string `json:"version"`
	Size      int64  `json:"size"`
	SHA256    string `json:"sha256"`
	UpdatedAt string `json:"updatedAt,omitempty"`
}

// bundlesDir 返回 Hub 侧资源包仓库目录。
func bundlesDir() string { return config.AppDataDir(bundlesSubdir) }

// bundleFileName 把 name/version 变成仓库里的文件名。
//
// ⚠️ name / version 只允许 [A-Za-z0-9._-]：它们会被拼进文件路径，
// 而"云控下发的名字"是外部可控输入（哪怕来自已配对设备，也不该信任到能穿越目录）。
func bundleFileName(name, version string) (string, bool) {
	if !isSafeBundleToken(name) || !isSafeBundleToken(version) {
		return "", false
	}
	return fmt.Sprintf("%s-%s.zip", name, version), true
}

func isSafeBundleToken(s string) bool {
	if s == "" || len(s) > 64 {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '.', r == '-', r == '_':
		default:
			return false
		}
	}
	return !strings.Contains(s, "..")
}

// loadBundleManifest 扫描仓库目录生成清单（按版本字典序，取每个包的**最新**一版）。
func loadBundleManifest() []BundleManifestItem {
	dir := bundlesDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return []BundleManifestItem{}
	}
	latest := map[string]BundleManifestItem{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".zip") {
			continue
		}
		base := strings.TrimSuffix(e.Name(), ".zip")
		idx := strings.LastIndex(base, "-")
		if idx <= 0 {
			continue
		}
		name, version := base[:idx], base[idx+1:]
		if !isSafeBundleToken(name) || !isSafeBundleToken(version) {
			continue
		}
		st, err := e.Info()
		if err != nil {
			continue
		}
		if st.Size() > bundleMaxZipBytes {
			continue
		}
		item := BundleManifestItem{
			Name:      name,
			Version:   version,
			Size:      st.Size(),
			SHA256:    readSHA256Sidecar(filepath.Join(dir, e.Name()+".sha256")),
			UpdatedAt: st.ModTime().Format(time.RFC3339),
		}
		if cur, ok := latest[name]; !ok || item.Version > cur.Version {
			latest[name] = item
		}
	}
	out := make([]BundleManifestItem, 0, len(latest))
	for _, v := range latest {
		out = append(out, v)
	}
	return out
}

// readSHA256Sidecar 读取同名 .sha256 文件（内容是十六进制摘要，允许带文件名后缀）。
func readSHA256Sidecar(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(strings.Fields(string(b))[0])
}

// findBundleItem 在清单里找指定包与版本（version 为空 = 该包最新版）。
func findBundleItem(name, version string) (BundleManifestItem, bool) {
	var found BundleManifestItem
	ok := false
	for _, it := range loadBundleManifest() {
		if it.Name != name {
			continue
		}
		if version == "" || it.Version == version {
			if !ok || it.Version > found.Version {
				found, ok = it, true
			}
		}
	}
	return found, ok
}

// ── 端点 ────────────────────────────────────────────────────────

// handlePeerlinkBundleManifest —— GET /api/peerlink/bundle/manifest（peer token）
func (s *Server) handlePeerlinkBundleManifest(c *gin.Context) {
	if _, ok := s.requirePeer(c); !ok {
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": loadBundleManifest(), "count": len(loadBundleManifest())})
}

// handlePeerlinkBundleDownload —— GET /api/peerlink/bundle/download?name=&version=（peer token）
func (s *Server) handlePeerlinkBundleDownload(c *gin.Context) {
	peerID, ok := s.requirePeer(c)
	if !ok {
		return
	}
	name := strings.TrimSpace(c.Query("name"))
	version := strings.TrimSpace(c.Query("version"))
	file, ok := bundleFileName(name, version)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request", "message": "name/version 含非法字符"})
		return
	}
	abs := filepath.Join(bundlesDir(), file)
	// 双重防穿越：文件名已白名单化，这里再确认解析后仍在仓库目录内
	if !strings.HasPrefix(abs, bundlesDir()+string(os.PathSeparator)) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request"})
		return
	}
	st, err := os.Stat(abs)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "bundle_not_found", "name": name, "version": version})
		return
	}
	slog.Info("peerlink bundle download", "peer", peerID, "name", name, "version", version, "size", st.Size())
	c.Header("Content-Type", "application/zip")
	c.Header("Content-Length", fmt.Sprintf("%d", st.Size()))
	c.File(abs)
}

// bundleReport 设备回报的一条更新结果。
type bundleReport struct {
	At              time.Time `json:"at"`
	PeerID          string    `json:"peerId"`
	Name            string    `json:"name"`
	Version         string    `json:"version"`
	Ok              bool      `json:"ok"`
	RolledBack      bool      `json:"rolledBack,omitempty"`
	PreviousVersion string    `json:"previousVersion,omitempty"`
	Error           string    `json:"error,omitempty"`
}

// bundleReports 有界内存台账（与 agentCallLog 同一套纪律：进程内存、重启即失效）。
type bundleReports struct {
	mu      sync.Mutex
	cap     int
	byPeer  map[string][]bundleReport
	lastVer map[string]string // peerID → 该设备已生效版本（云控视角）
}

var globalBundleReports = &bundleReports{cap: 50, byPeer: map[string][]bundleReport{}, lastVer: map[string]string{}}

func (r *bundleReports) add(rep bundleReport) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rep.At = time.Now()
	list := append(r.byPeer[rep.PeerID], rep)
	if len(list) > r.cap {
		list = list[len(list)-r.cap:]
	}
	r.byPeer[rep.PeerID] = list
	if rep.Ok && !rep.RolledBack {
		r.lastVer[rep.PeerID] = rep.Name + "@" + rep.Version
	}
}

func (r *bundleReports) snapshot() (items []bundleReport, versions map[string]string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, list := range r.byPeer {
		items = append(items, list...)
	}
	versions = map[string]string{}
	for k, v := range r.lastVer {
		versions[k] = v
	}
	return items, versions
}

// handlePeerlinkBundleReport —— POST /api/peerlink/bundle/report（peer token）
func (s *Server) handlePeerlinkBundleReport(c *gin.Context) {
	peerID, ok := s.requirePeer(c)
	if !ok {
		return
	}
	var body struct {
		Name            string `json:"name"`
		Version         string `json:"version"`
		Ok              bool   `json:"ok"`
		RolledBack      bool   `json:"rolledBack"`
		PreviousVersion string `json:"previousVersion"`
		Error           string `json:"error"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_json", "detail": err.Error()})
		return
	}
	globalBundleReports.add(bundleReport{
		PeerID: peerID, Name: body.Name, Version: body.Version,
		Ok: body.Ok, RolledBack: body.RolledBack, PreviousVersion: body.PreviousVersion,
		// ⚠️ 脱敏：错误文本可能含设备绝对路径，只留前 200 字符
		Error: truncateForReport(body.Error, 200),
	})
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func truncateForReport(s string, n int) string {
	s = strings.TrimSpace(s)
	if len([]rune(s)) <= n {
		return s
	}
	return string([]rune(s)[:n])
}

// handlePeerlinkBundleStatus —— GET /api/peerlink/bundle/status（运维）：云控视角
func (s *Server) handlePeerlinkBundleStatus(c *gin.Context) {
	if !isOperator(c) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	items, versions := globalBundleReports.snapshot()
	if items == nil {
		items = []bundleReport{}
	}
	c.JSON(http.StatusOK, gin.H{
		"available":   loadBundleManifest(),
		"deviceVer":   versions,
		"reports":     items,
		"reportCount": len(items),
	})
}

// handlePeerlinkBundlePush —— POST /api/peerlink/bundle/push（运维）：云控下发
//
// 入参：{ peerId, name, version? } —— version 省略 = 该包最新版。
func (s *Server) handlePeerlinkBundlePush(c *gin.Context) {
	if !isOperator(c) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	var body struct {
		PeerId  string `json:"peerId"`
		Name    string `json:"name"`
		Version string `json:"version"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_json", "detail": err.Error()})
		return
	}
	body.PeerId = strings.TrimSpace(body.PeerId)
	body.Name = strings.TrimSpace(body.Name)
	body.Version = strings.TrimSpace(body.Version)
	if body.PeerId == "" || body.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request", "message": "peerId 与 name 必填"})
		return
	}
	if _, ok := s.peerHub.PeerByID(body.PeerId); !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "peer_not_found"})
		return
	}
	item, ok := findBundleItem(body.Name, body.Version)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "bundle_not_found", "name": body.Name, "version": body.Version})
		return
	}
	if !s.peerCircuitAllow(c, body.PeerId) {
		return
	}

	req := peerlink.BundleUpdateRequest{
		Name:    item.Name,
		Version: item.Version,
		SHA256:  item.SHA256,
		Size:    item.Size,
	}
	// 更新是分钟级动作 ⇒ 超时比普通 RPC 长，但仍要封顶（不能无限等）
	res, err := s.peerCalls.Call(c.Request.Context(), body.PeerId, peerlink.MethodBundleUpdate, req, peerlink.BundleCallTimeout)
	if err != nil {
		if peerBusyIfErr(c, body.PeerId, err) {
			return
		}
		s.peerCircuitRecord(body.PeerId, err.Error())
		errType := "peer_call_failed"
		switch {
		case errors.Is(err, peerlink.ErrPeerOffline):
			errType = "peer_offline"
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": errType, "peerId": body.PeerId})
			return
		case errors.Is(err, peerlink.ErrCallTimeout):
			errType = "peer_timeout"
			c.JSON(http.StatusGatewayTimeout, gin.H{"error": errType, "peerId": body.PeerId})
			return
		case errors.Is(err, peerlink.ErrPeerRejected):
			// 对端"健康地拒绝"（包名不认识 / 没给摘要）⇒ 400，不计熔断
			c.JSON(http.StatusBadRequest, gin.H{"error": "peer_rejected", "detail": err.Error(), "peerId": body.PeerId})
			return
		}
		_ = errType
		c.JSON(http.StatusBadGateway, gin.H{"error": "peer_call_failed", "detail": err.Error(), "peerId": body.PeerId})
		return
	}
	var out peerlink.BundleUpdateResult
	if err := json.Unmarshal(res, &out); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "bad_peer_payload", "detail": err.Error()})
		return
	}
	// 云控侧留痕：不管成败都记，否则"下发过但没生效"在台账里凭空消失
	globalBundleReports.add(bundleReport{
		PeerID: body.PeerId, Name: out.Name, Version: out.AppliedVersion,
		Ok: out.Ok, RolledBack: out.RolledBack, PreviousVersion: out.PreviousVersion,
		Error: truncateForReport(out.Error, 200),
	})
	c.JSON(http.StatusOK, out)
}
