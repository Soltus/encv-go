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
	"strconv"
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
	// ABI 目标架构（仅 go-binary 需要；换执行体必须声明架构，执行端据此校验）。
	ABI string `json:"abi,omitempty"`
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

// splitBundleFileName 把 `<name>-<version>` 切回包名与版本。
//
// ⚠️ 两种文件名都会遇到，规则必须同时成立：
//   - 版本号常带连字符：`web-v0.0.1-test` ⇒ name=web, version=v0.0.1-test
//     （用 LastIndex('-') 会切成 name="web-v0.0.1" version="test" ⇒ 设备拉包 404，
//      2026-10-05 真机首测抓到过）；
//   - 包名也可能带连字符：`go-binary-v0.0.5-logs` ⇒ 必须切成 name=go-binary，
//     否则清单里只有 "go" ⇒ 云控 push{name:"go-binary"} 得到 bundle_not_found
//     （2026-10-05 下发 Go 二进制热更时抓到）。
//
// 判据优先级（从最可靠到兜底）：
//   ① 最后一个 "-v"：版本号以 v 开头是本项目约定 ⇒ 它之后全是版本；
//   ② 第一段以数字开头的 "-"：如 preview-assets-1.2.3；
//   ③ 兜底：第一个 "-"。
func splitBundleFileName(base string) (name, version string, ok bool) {
	base = strings.TrimSpace(base)
	if base == "" {
		return "", "", false
	}
	// ① 版本以 v 开头（v0.0.7-devshell / v1 / v2-rc1）
	if i := strings.LastIndex(base, "-v"); i > 0 {
		return base[:i], base[i+1:], true
	}
	// ② 第一段是数字（1.2.3 / 2024.10.05）
	parts := strings.Split(base, "-")
	for i := 1; i < len(parts); i++ {
		p := parts[i]
		if p != "" && p[0] >= '0' && p[0] <= '9' {
			return strings.Join(parts[:i], "-"), strings.Join(parts[i:], "-"), true
		}
	}
	// ③ 兜底
	if i := strings.Index(base, "-"); i > 0 {
		return base[:i], base[i+1:], true
	}
	return "", "", false
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
		name, version, splitOK := splitBundleFileName(base)
		if !splitOK {
			continue
		}
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
			ABI:       readTextSidecar(filepath.Join(dir, e.Name()+".abi")),
			UpdatedAt: st.ModTime().Format(time.RFC3339),
		}
		// ⚠️ 不能用字典序比版本（真机/本机实测踩到）：
		//   "v0.0.1-test" > "v0.0.1-smoketest"、"v0.0.9" > "v0.0.10"
		//   ⇒ 会把**旧包**当成最新版下发给设备。必须按语义版本比较。
		if cur, ok := latest[name]; !ok || versionNewer(item.Version, cur.Version) {
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

// readTextSidecar 读取同名 sidecar 文本（如 .abi）。
func readTextSidecar(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// versionNewer 判断 a 是否比 b 新（语义版本优先，比不出来再退字典序）。
//
// 规则：忽略前导 'v' / 'V'；按 '.' 切段，逐段按**数值**比较；
// 某段不是数字（如 "0.0.1-rc1" 的 "1-rc1"）则该段按字符串比；
// 前缀相同而 a 段数更多 ⇒ a 更新（"1.2.3.1" > "1.2.3"）。
func versionNewer(a, b string) bool {
	as := strings.Split(strings.TrimPrefix(strings.TrimPrefix(strings.TrimSpace(a), "v"), "V"), ".")
	bs := strings.Split(strings.TrimPrefix(strings.TrimPrefix(strings.TrimSpace(b), "v"), "V"), ".")
	for i := 0; i < len(as) || i < len(bs); i++ {
		if i >= len(bs) {
			return true
		}
		if i >= len(as) {
			return false
		}
		an, ae := strconv.Atoi(as[i])
		bn, be := strconv.Atoi(bs[i])
		switch {
		case ae == nil && be == nil:
			if an != bn {
				return an > bn
			}
		case ae == nil && be != nil:
			return true // 纯数字段优先于含非数字的段（1 > 1-rc1）
		case ae != nil && be == nil:
			return false
		default:
			if as[i] != bs[i] {
				return as[i] > bs[i]
			}
		}
	}
	return false
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
	// ⚠️ 判据是"**真的生效了某版**"，不是"没回滚"：
	//   - 失败的更新：Ok=false 且 AppliedVersion 为空 ⇒ 不动（设备还在旧版）
	//   - 云控**回滚成功**：Ok=true 且 Version 是退回后的版本 ⇒ **必须更新**，
	//     否则云控视角里设备会永远停在坏包那一版，下次"要不要再推"的判断全错。
	if rep.Ok && strings.TrimSpace(rep.Version) != "" {
		r.lastVer[rep.PeerID] = rep.Name + "@" + rep.Version
	}
}

// load 用落盘内容覆盖内存台账（启动时恢复用）。
func (r *bundleReports) load(items []bundleReport, versions map[string]string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byPeer = map[string][]bundleReport{}
	for _, it := range items {
		list := append(r.byPeer[it.PeerID], it)
		if len(list) > r.cap {
			list = list[len(list)-r.cap:]
		}
		r.byPeer[it.PeerID] = list
	}
	r.lastVer = map[string]string{}
	for k, v := range versions {
		r.lastVer[k] = v
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

// ── 台账持久化（2026-10-05）───────────────────────────────────────
//
// 原台账是**纯内存**：真机实测后端重启后 `reportCount` 从 1 变 0 ——
// "这台设备装到哪版了""上次下发成功还是失败"全部凭空消失，
// 云控就退化成"每次都当没推过"，既不敢自动重试也无法判断要不要回滚。
// 落盘纪律与 peers.json 完全一致（应用私有目录 / 0600 / 原子写）。

type bundleReportSnapshot struct {
	Items    []bundleReport    `json:"items"`
	Versions map[string]string `json:"versions"`
}

func (s *Server) saveBundleReports() {
	if !s.peerStore.Enabled() {
		return
	}
	items, versions := globalBundleReports.snapshot()
	snap := bundleReportSnapshot{Items: items, Versions: versions}
	if snap.Items == nil {
		snap.Items = []bundleReport{}
	}
	if err := s.peerStore.SaveState("bundle-reports.json", snap); err != nil {
		slog.Warn("peerlink bundle reports save failed", "error", err.Error())
	}
}

// restoreBundleReports 启动时把台账读回，返回恢复条数。
func (s *Server) restoreBundleReports() int {
	if !s.peerStore.Enabled() {
		return 0
	}
	var snap bundleReportSnapshot
	ok, err := s.peerStore.LoadState("bundle-reports.json", &snap)
	if err != nil {
		slog.Warn("peerlink bundle reports load failed", "error", err.Error())
		return 0
	}
	if !ok {
		return 0
	}
	globalBundleReports.load(snap.Items, snap.Versions)
	if n := len(snap.Items); n > 0 {
		slog.Info("peerlink bundle reports restored", "reports", n, "devices", len(snap.Versions))
	}
	return len(snap.Items)
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
	// 落盘（2026-10-05）：重启即丢的台账等于没有台账（实测 1 → 0）
	s.saveBundleReports()
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

// bundleRequiredFiles 某个包"必须有"的文件（缺了就判定包不完整）。
//
// ⚠️ 2026-10-05：此前 Hub 下发**从不带** Required，而执行端 ApplyFile 会直接取
//    `Required[0]` ⇒ 换执行体（go-binary）时 panic ⇒ RPC 永不回包 ⇒ 云端只看到
//    504 peer_timeout（排查时会误判成"包太大/网络慢"，实际是空数组越界）。
//    这里按包名给出必含文件，执行端也就有了"这个包对不对"的判据。
func bundleRequiredFiles(name string) []string {
	switch strings.TrimSpace(name) {
	case "go-binary":
		return []string{"encv-go"}
	case "web", "preview-assets":
		// Kotlin 侧也是判定 index.html（I4：没有它就不切换）⇒ 两边判据一致
		return []string{"index.html"}
	}
	return nil
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
		Name:     item.Name,
		Version:  item.Version,
		SHA256:   item.SHA256,
		Size:     item.Size,
		ABI:      item.ABI,
		Required: bundleRequiredFiles(item.Name),
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
	s.saveBundleReports()
	c.JSON(http.StatusOK, out)
}

// handlePeerlinkBundleRollback —— POST /api/peerlink/bundle/rollback（运维）
//
// **云控一键回滚**（2026-10-05）：把某台设备上的某个包退回上一版。
//
// 为什么必须是独立端点而不是"再推一个旧版本"：
//   - 推旧版本要重新打包 + 重新走一遍下载/校验/切换（分钟级），期间设备一直是坏的；
//   - 设备本地本来就有上一次的备份，回滚只是搬回来（秒级）；
//   - 坏包往往还伴随"校验过不了/装不完整"，此时根本推不动任何新包。
//
// 入参：{ peerId, name, version? }；version 仅用于台账记录（执行端以本地备份为准）。
func (s *Server) handlePeerlinkBundleRollback(c *gin.Context) {
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
	if body.PeerId == "" || body.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request", "message": "peerId 与 name 必填"})
		return
	}
	if _, ok := s.peerHub.PeerByID(body.PeerId); !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "peer_not_found"})
		return
	}
	if !s.peerCircuitAllow(c, body.PeerId) {
		return
	}

	req := peerlink.BundleRollbackRequest{Name: body.Name, Version: strings.TrimSpace(body.Version)}
	res, err := s.peerCalls.Call(c.Request.Context(), body.PeerId, peerlink.MethodBundleRollback, req, peerlink.BundleCallTimeout)
	if err != nil {
		if peerBusyIfErr(c, body.PeerId, err) {
			return
		}
		// 与 bundle_update 同一套纪律：对端"健康地拒绝"（包名不认识 / **没备份可退**）
		// ⇒ 400 且**不计入熔断**（"没备份"不是设备故障）。
		if s.peerRejectedIfErr(c, body.PeerId, err) {
			return
		}
		s.peerCircuitRecord(body.PeerId, err.Error())
		switch {
		case errors.Is(err, peerlink.ErrPeerOffline):
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "peer_offline", "peerId": body.PeerId})
		case errors.Is(err, peerlink.ErrCallTimeout):
			c.JSON(http.StatusGatewayTimeout, gin.H{"error": "peer_timeout", "peerId": body.PeerId})
		default:
			c.JSON(http.StatusBadGateway, gin.H{"error": "peer_call_failed", "detail": err.Error(), "peerId": body.PeerId})
		}
		return
	}
	s.peerCircuitRecordSuccess(body.PeerId)

	var out peerlink.BundleRollbackResult
	if err := json.Unmarshal(res, &out); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "bad_peer_payload", "detail": err.Error()})
		return
	}
	// 台账：回滚本身是一次"变更"，必须留痕且标记 rolledBack
	// （否则设备版本在云控视角里会停在坏包那一版，下次判断"要不要再推"就全错）
	ver := out.Version
	if ver == "" {
		ver = strings.TrimSpace(body.Version)
	}
	globalBundleReports.add(bundleReport{
		PeerID: body.PeerId, Name: out.Name, Version: ver,
		Ok: out.Ok, RolledBack: true, PreviousVersion: out.PreviousVersion,
		Error: truncateForReport(out.Error, 200),
	})
	s.saveBundleReports()
	c.JSON(http.StatusOK, out)
}

// ── 本端（设备侧）热更状态与自回滚（2026-10-05）────────────────────
//
// 移动端 UI 需要"我这台机器装了哪版 / 能不能退回去"，而这两个问题**只有设备自己**
// 答得出来（Hub 只知道它推过什么，不知道设备本地生效的是哪一版、有没有备份）。

// localBundleState 本端一个包的生效状态。
type localBundleState struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Kind    string `json:"kind"` // dir（资源目录） / file（执行体）
	// Rollable 有没有上一版可退（没有备份 ⇒ 回滚必然被拒绝，UI 应直接禁用按钮）
	Rollable bool `json:"rollable"`
}

// bundleLocalStates 扫描本端登记在册的包，返回各自生效版本与可否回滚。
func bundleLocalStates() []localBundleState {
	// ⚠️ 与 bundleTargetDir / bundleTargetFile 保持同一份清单，别各写一份
	names := []string{"web", "preview-assets", "go-binary"}
	out := make([]localBundleState, 0, len(names))
	for _, name := range names {
		target, isFile := "", false
		if dir, ok := bundleTargetDirFor(name); ok {
			target, isFile = dir, false
		} else if f, ok2 := bundleTargetFileFor(name); ok2 {
			target, isFile = f, true
		} else {
			continue // 该包在本端不可用（如桌面端没有 go-binary 目标）
		}
		kind := "dir"
		if isFile {
			kind = "file"
		}
		out = append(out, localBundleState{
			Name:     name,
			Version:  localBundleVersion(target, isFile),
			Kind:     kind,
			Rollable: bundleHasBackup(name, target, isFile),
		})
	}
	return out
}

func bundleHasBackup(name, target string, isFile bool) bool {
	if isFile {
		_, err := os.Stat(filepath.Join(filepath.Dir(target), name+"-backup"))
		return err == nil
	}
	st, err := os.Stat(filepath.Join(filepath.Dir(target), name+"-backup"))
	return err == nil && st.IsDir()
}

// handlePeerlinkBundleLocal —— GET /api/peerlink/bundle/local（运维/本端 UI）
func (s *Server) handlePeerlinkBundleLocal(c *gin.Context) {
	if !isOperator(c) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": bundleLocalStates()})
}

// handlePeerlinkBundleLocalRollback —— POST /api/peerlink/bundle/local/rollback
//
// 设备**自己**退回上一版（不给 Hub 发指令，也不要求在线）。
// 移动端 UI 的"回滚"按钮走这条：用户就在设备跟前，没理由绕一圈云端。
func (s *Server) handlePeerlinkBundleLocalRollback(c *gin.Context) {
	if !isOperator(c) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	var body struct {
		Name string `json:"name"`
	}
	_ = c.ShouldBindJSON(&body)
	name := strings.TrimSpace(body.Name)
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request", "message": "name 必填"})
		return
	}
	out := s.peerLocalBundleRollback(peerlink.BundleRollbackRequest{Name: name})
	if out.Rejected {
		// 包名不认识 / 没备份可退 ⇒ 400（与云控回滚同一套语义，UI 文案可复用）
		c.JSON(http.StatusBadRequest, gin.H{"error": "rollback_rejected", "name": name, "reason": out.Error})
		return
	}
	c.JSON(http.StatusOK, out)
}
