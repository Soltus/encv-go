package server

// peerlink_bundle_test.go —— 云控热更新的回归锁（2026-10-04）
//
// 覆盖三层：
//  1. Hub 侧端点鉴权：manifest/download 必须 peer token（未配对 401）；download 的名字必须过白名单
//  2. 云控下发（push）端到端：Hub --WS--> Edge 的 bundle_update 指令真的能跑到执行端并回传结果
//  3. 执行端（设备）安装器：真的从 Hub 拉 zip → 校验 → 原子生效；摘要不对 ⇒ 失败且旧版完好
//
// ⚠️ 真机复验状态：本环境 adb 无设备、手机在 NAT 后 ⇒ 设备端只能在单测里用 httptest 模拟 Hub，
//    真机端到端要等引导版 APK（含本轮 Go/Kotlin 改动）构建安装后补。

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Soltus/encv-go/internal/bundle"
	"github.com/Soltus/encv-go/internal/peerlink"
	"github.com/gin-gonic/gin"
)

// pairPeerOnHub 在同一个 Hub 上完成 ticket+pair，返回可用 token（不需要起 Edge）。
func pairPeerOnHub(t *testing.T, r *gin.Engine) string {
	t.Helper()
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest("POST", "/api/peerlink/ticket", strings.NewReader(`{}`)))
	var tk struct {
		PairingID string `json:"pairingId"`
		PSK       string `json:"psk"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &tk); err != nil || tk.PairingID == "" {
		t.Fatalf("ticket 失败: %s", rec.Body.String())
	}
	psk, _ := peerlink.DecodePSK(tk.PSK)
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest("POST", "/api/peerlink/pair", strings.NewReader(jsonBody(t, map[string]string{
		"pairingId": tk.PairingID,
		"deviceId":  "bundle-device-0001",
		"name":      "Pixel",
		"platform":  "android",
		"proof":     peerlink.Proof(psk, tk.PairingID, "bundle-device-0001"),
	}))))
	var pr struct {
		PeerID string `json:"peerId"`
		Token  string `json:"token"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &pr); err != nil || pr.Token == "" {
		t.Fatalf("pair 失败: %s", rec.Body.String())
	}
	return pr.Token
}

// writeTestBundle 往 Hub 资源包仓库写一个 zip + sha256 sidecar，返回 (zip 字节数, sha256)。
func writeTestBundle(t *testing.T, dir, name, version string, entries map[string]string) (int64, string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("建仓库目录失败: %v", err)
	}
	zipPath := filepath.Join(dir, name+"-"+version+".zip")
	f, err := os.Create(zipPath)
	if err != nil {
		t.Fatalf("建 zip 失败: %v", err)
	}
	zw := zip.NewWriter(f)
	for n, c := range entries {
		w, err := zw.Create(n)
		if err != nil {
			t.Fatalf("zip entry 失败: %v", err)
		}
		if _, err := w.Write([]byte(c)); err != nil {
			t.Fatalf("zip write 失败: %v", err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zip close 失败: %v", err)
	}
	_ = f.Close()
	sha, err := bundle.SHA256File(zipPath)
	if err != nil {
		t.Fatalf("算 sha256 失败: %v", err)
	}
	if err := os.WriteFile(zipPath+".sha256", []byte(sha), 0o644); err != nil {
		t.Fatalf("写 sidecar 失败: %v", err)
	}
	st, _ := os.Stat(zipPath)
	return st.Size(), sha
}

func TestPeerlinkBundle_Manifest_RequiresPeerToken(t *testing.T) {
	r, _ := newPeerlinkRouter()
	srv := httptest.NewServer(r)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/peerlink/bundle/manifest")
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("未配对访问 manifest 应 401, got %d", resp.StatusCode)
	}
}

func TestPeerlinkBundle_Manifest_ListsLatest(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ENCV_BUNDLES_DIR", dir)
	_, _ = writeTestBundle(t, dir, "web", "v1", map[string]string{"index.html": "1"})
	_, sha2 := writeTestBundle(t, dir, "web", "v2", map[string]string{"index.html": "2"})

	r, _ := newPeerlinkRouter()
	srv := httptest.NewServer(r)
	defer srv.Close()
	token := pairPeerOnHub(t, r)

	req, _ := http.NewRequest("GET", srv.URL+"/api/peerlink/bundle/manifest", nil)
	req.Header.Set("Authorization", "Peer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("manifest 期望 200, got %d", resp.StatusCode)
	}
	var out struct {
		Count int                        `json:"count"`
		Items []BundleManifestItem       `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if out.Count != 1 || len(out.Items) != 1 {
		t.Fatalf("同名多版应只暴露最新一版, got %+v", out.Items)
	}
	if out.Items[0].Version != "v2" || out.Items[0].SHA256 != sha2 {
		t.Fatalf("应取 v2: %+v", out.Items[0])
	}
}

// TestPeerlinkBundle_Manifest_SplitsNameVersion —— 版本号带连字符时的切分（真机首测抓到的 bug）
//
// 真机现象：包文件 `web-v0.0.1-test.zip` 被切成 name="web-v0.0.1" version="test"
// ⇒ 设备按这个名字拼下载 URL ⇒ Hub 侧 404（bundle_not_found）。
func TestPeerlinkBundle_Manifest_SplitsNameVersion(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ENCV_BUNDLES_DIR", dir)
	_, sha := writeTestBundle(t, dir, "web", "v0.0.1-test", map[string]string{"index.html": "1"})

	r, _ := newPeerlinkRouter()
	srv := httptest.NewServer(r)
	defer srv.Close()
	token := pairPeerOnHub(t, r)

	req, _ := http.NewRequest("GET", srv.URL+"/api/peerlink/bundle/manifest", nil)
	req.Header.Set("Authorization", "Peer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}
	defer resp.Body.Close()
	var out struct {
		Items []BundleManifestItem `json:"items"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if len(out.Items) != 1 {
		t.Fatalf("应有 1 项, got %+v", out.Items)
	}
	it := out.Items[0]
	if it.Name != "web" || it.Version != "v0.0.1-test" || it.SHA256 != sha {
		t.Fatalf("包名/版本切分错误: name=%q version=%q（应为 web / v0.0.1-test）", it.Name, it.Version)
	}
}

// TestPeerlinkBundle_Manifest_LatestBySemver —— 最新版必须按**语义版本**选，不能按字典序
//
// 字典序会把 v0.0.9 判成比 v0.0.10 新（本机实测：v0.0.1-test 也压过 v0.0.1-smoketest）
// ⇒ 云控会把**旧包**下发给设备。
func TestPeerlinkBundle_Manifest_LatestBySemver(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ENCV_BUNDLES_DIR", dir)
	_, _ = writeTestBundle(t, dir, "web", "v0.0.9", map[string]string{"index.html": "old"})
	_, shaNew := writeTestBundle(t, dir, "web", "v0.0.10", map[string]string{"index.html": "new"})
	_, _ = writeTestBundle(t, dir, "web", "v0.0.2", map[string]string{"index.html": "older"})

	r, _ := newPeerlinkRouter()
	srv := httptest.NewServer(r)
	defer srv.Close()
	token := pairPeerOnHub(t, r)

	req, _ := http.NewRequest("GET", srv.URL+"/api/peerlink/bundle/manifest", nil)
	req.Header.Set("Authorization", "Peer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}
	defer resp.Body.Close()
	var out struct {
		Items []BundleManifestItem `json:"items"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if len(out.Items) != 1 {
		t.Fatalf("同名多版只应暴露最新一版, got %+v", out.Items)
	}
	if out.Items[0].Version != "v0.0.10" || out.Items[0].SHA256 != shaNew {
		t.Fatalf("最新版应为 v0.0.10, got %q", out.Items[0].Version)
	}
}

// TestVersionNewer —— 版本比较的边界（纯函数）
func TestVersionNewer(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"v0.0.10", "v0.0.9", true},
		{"v0.0.9", "v0.0.10", false},
		{"1.2.3", "1.2.3", false},
		{"1.2.4", "1.2.3", true},
		{"1.10.0", "1.9.0", true},
		{"v1.0.0", "1.0.0", false}, // 前导 v 忽略 ⇒ 相等
		{"1.2.3.1", "1.2.3", true},
		{"1.2", "1.2.0", false},
		{"1.2.0", "1.2.0-rc1", true}, // 数字段 > 含后缀段
		{"2.0.0", "10.0.0", false},
	}
	for _, c := range cases {
		if got := versionNewer(c.a, c.b); got != c.want {
			t.Errorf("versionNewer(%q,%q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

func TestPeerlinkBundle_Download_RejectsTraversalName(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ENCV_BUNDLES_DIR", dir)
	_, _ = writeTestBundle(t, dir, "web", "v1", map[string]string{"index.html": "1"})

	r, _ := newPeerlinkRouter()
	srv := httptest.NewServer(r)
	defer srv.Close()
	token := pairPeerOnHub(t, r)

	for _, bad := range []string{"../web", "web/../../x", "we b", ""} {
		req, _ := http.NewRequest("GET", srv.URL+"/api/peerlink/bundle/download?name="+bad+"&version=v1", nil)
		req.Header.Set("Authorization", "Peer "+token)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("请求失败: %v", err)
		}
		resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			t.Fatalf("非法 name %q 不应放行", bad)
		}
	}
}

// startPairedEdgeWithBundle 建立"已配对 + Edge 已连上"场景，额外注入云控热更新处理器。
func startPairedEdgeWithBundle(
	t *testing.T,
	r *gin.Engine,
	s *Server,
	hubURL string,
	onBundle func(peerlink.BundleUpdateRequest) peerlink.BundleUpdateResult,
) string {
	t.Helper()
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest("POST", "/api/peerlink/ticket", strings.NewReader(`{}`)))
	var tk struct {
		PairingID string `json:"pairingId"`
		PSK       string `json:"psk"`
	}
	json.Unmarshal(rec.Body.Bytes(), &tk)
	psk, _ := peerlink.DecodePSK(tk.PSK)
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest("POST", "/api/peerlink/pair", strings.NewReader(jsonBody(t, map[string]string{
		"pairingId": tk.PairingID,
		"deviceId":  "bundle-device-0002",
		"name":      "Pixel",
		"platform":  "android",
		"proof":     peerlink.Proof(psk, tk.PairingID, "bundle-device-0002"),
	}))))
	var pr struct {
		PeerID string `json:"peerId"`
		Token  string `json:"token"`
	}
	json.Unmarshal(rec.Body.Bytes(), &pr)

	edge := peerlink.NewEdge(peerlink.EdgeOptions{
		HubURL:              hubURL,
		Token:               pr.Token,
		DeviceID:            "bundle-device-0002",
		HeartbeatForeground: 80 * time.Millisecond,
		MinBackoff:          10 * time.Millisecond,
		MaxBackoff:          50 * time.Millisecond,
		OnBundleUpdate:      onBundle,
	})
	ctx, cancel := context.WithCancel(context.Background())
	go edge.Start(ctx)
	t.Cleanup(func() {
		edge.Close()
		cancel()
	})

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if _, ok := s.peerConns.Get(pr.PeerID); ok {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	return pr.PeerID
}

func TestPeerlinkBundlePush_EndToEnd_OK(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ENCV_BUNDLES_DIR", dir)
	size, sha := writeTestBundle(t, dir, "web", "v1", map[string]string{"index.html": "1"})

	r, s := newPeerlinkRouter()
	srv := httptest.NewServer(r)
	defer srv.Close()

	var got peerlink.BundleUpdateRequest
	peerID := startPairedEdgeWithBundle(t, r, s, srv.URL+"/api/peerlink",
		func(req peerlink.BundleUpdateRequest) peerlink.BundleUpdateResult {
			got = req
			return peerlink.BundleUpdateResult{Ok: true, Name: req.Name, AppliedVersion: req.Version, PreviousVersion: "v0"}
		})

	body, _ := json.Marshal(map[string]string{"peerId": peerID, "name": "web"})
	req, _ := http.NewRequest("POST", srv.URL+"/api/peerlink/bundle/push", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Peerlink-Operator", "1")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("push 失败: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("push 期望 200, got %d", resp.StatusCode)
	}
	// 指令必须带上版本与摘要 —— 没有摘要执行端应拒绝（见下一条用例）
	if got.Name != "web" || got.Version != "v1" || got.SHA256 != sha || got.Size != size {
		t.Fatalf("下发的指令不完整: %+v", got)
	}
	// 云控台账：成功也要留痕（否则"下发过"在 status 里查不到）
	_, versions := globalBundleReports.snapshot()
	if versions[peerID] != "web@v1" {
		t.Fatalf("status 未记录生效版本: %+v", versions)
	}
}

// TestPeerlinkBundlePush_Rejected_IsBadRequest —— "包不存在/执行端不认" 是 400，不是 502，且不计熔断
func TestPeerlinkBundlePush_Rejected_IsBadRequest(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ENCV_BUNDLES_DIR", dir)
	_, _ = writeTestBundle(t, dir, "web", "v1", map[string]string{"index.html": "1"})

	r, s := newPeerlinkRouter()
	srv := httptest.NewServer(r)
	defer srv.Close()

	calls := 0
	peerID := startPairedEdgeWithBundle(t, r, s, srv.URL+"/api/peerlink",
		func(req peerlink.BundleUpdateRequest) peerlink.BundleUpdateResult {
			calls++
			if calls == 1 {
				return peerlink.BundleUpdateResult{Name: req.Name, Rejected: true, Error: "unknown_bundle:" + req.Name}
			}
			return peerlink.BundleUpdateResult{Ok: true, Name: req.Name, AppliedVersion: req.Version}
		})

	push := func() *http.Response {
		body, _ := json.Marshal(map[string]string{"peerId": peerID, "name": "web"})
		req, _ := http.NewRequest("POST", srv.URL+"/api/peerlink/bundle/push", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Peerlink-Operator", "1")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("push 失败: %v", err)
		}
		return resp
	}

	resp := push()
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("被拒绝应 400（不得当成对端故障）, got %d", resp.StatusCode)
	}
	// 关键反向锁：被拒绝**不得**连坐熔断 —— 紧接着的合法调用必须仍然成功
	resp2 := push()
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("被拒绝后不应熔断，第二次合法 push 应 200, got %d", resp2.StatusCode)
	}
}

// TestPeerEdge_WiresBundleUpdate —— 接线锁：startEdgeLocked 必须把 OnBundleUpdate 传给 Edge
//
// 真机首测（2026-10-05）翻车：handler 只在 peerEdgeHandlers / edgeHandlers() 里加了，
// 没传进 NewEdge 的 EdgeOptions ⇒ 设备端一律回 "not_supported" ⇒ Hub 侧 push 得 502。
func TestPeerEdge_WiresBundleUpdate(t *testing.T) {
	r, s := newPeerlinkRouter()
	_ = r
	// 用一个必然连不上的地址：本例只断言**接线**，不要求真的连上
	s.startEdgeLocked("http://127.0.0.1:1/api/peerlink", "peer-x", "dev-x", "tok-x")
	s.peerEdgeMu.Lock()
	rt := s.peerEdge
	s.peerEdgeMu.Unlock()
	if rt == nil || rt.edge == nil {
		t.Fatal("startEdgeLocked 未建立 Edge")
	}
	defer func() {
		if rt.cancel != nil {
			rt.cancel()
		}
	}()
	for _, m := range []string{peerlink.MethodBundleUpdate, "read", "search", "agent_invoke", "no_such_method"} {
		want := m != "no_such_method"
		if got := rt.edge.Supports(m); got != want {
			t.Errorf("Supports(%q) = %v, want %v", m, got, want)
		}
	}
}

// TestPeerLocalBundleUpdate_DownloadsAndApplies —— 执行端真拉包并原子生效
func TestPeerLocalBundleUpdate_DownloadsAndApplies(t *testing.T) {
	bundles := t.TempDir()
	t.Setenv("ENCV_BUNDLES_DIR", bundles)
	target := t.TempDir()
	t.Setenv("ENCV_PREVIEW_ASSETS_DIR", target)
	t.Setenv("ENCV_TMP_DIR", t.TempDir())

	_, sha := writeTestBundle(t, bundles, "preview-assets", "v2", map[string]string{
		"index.html": "<html>v2</html>",
		"main.js":    "console.log(2)",
	})

	r, s := newPeerlinkRouter()
	srv := httptest.NewServer(r)
	defer srv.Close()
	token := pairPeerOnHub(t, r)

	// 造"本端已连着该 Hub"的运行时（不需要真的起 Edge：这里验证的是下载+安装）
	hub := srv.URL + "/api/peerlink"
	s.peerEdge = &edgeRuntime{
		hubURL: hub,
		edge:   peerlink.NewEdge(peerlink.EdgeOptions{HubURL: hub, Token: token}),
	}

	out := s.peerLocalBundleUpdate(peerlink.BundleUpdateRequest{
		Name:     "preview-assets",
		Version:  "v2",
		SHA256:   sha,
		Required: []string{"index.html", "main.js"},
	})
	if !out.Ok {
		t.Fatalf("执行端更新应成功: %+v", out)
	}
	if out.AppliedVersion != "v2" {
		t.Fatalf("生效版本应为 v2, got %q", out.AppliedVersion)
	}
	b, err := os.ReadFile(filepath.Join(target, "index.html"))
	if err != nil || string(b) != "<html>v2</html>" {
		t.Fatalf("目标目录内容不对: %q err=%v", string(b), err)
	}
}

// TestPeerLocalBundleUpdate_BadChecksum_KeepsOld —— 摘要不符：失败且旧版完好（不白屏）
func TestPeerLocalBundleUpdate_BadChecksum_KeepsOld(t *testing.T) {
	bundles := t.TempDir()
	t.Setenv("ENCV_BUNDLES_DIR", bundles)
	target := t.TempDir()
	t.Setenv("ENCV_PREVIEW_ASSETS_DIR", target)
	t.Setenv("ENCV_TMP_DIR", t.TempDir())
	// 先装一版旧资源
	if err := os.WriteFile(filepath.Join(target, "index.html"), []byte("OLD"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, _ = writeTestBundle(t, bundles, "preview-assets", "v3", map[string]string{"index.html": "NEW"})

	r, s := newPeerlinkRouter()
	srv := httptest.NewServer(r)
	defer srv.Close()
	token := pairPeerOnHub(t, r)
	hub := srv.URL + "/api/peerlink"
	s.peerEdge = &edgeRuntime{hubURL: hub, edge: peerlink.NewEdge(peerlink.EdgeOptions{HubURL: hub, Token: token})}

	out := s.peerLocalBundleUpdate(peerlink.BundleUpdateRequest{
		Name:    "preview-assets",
		Version: "v3",
		SHA256:  strings.Repeat("0", 64), // 故意错
	})
	if out.Ok {
		t.Fatal("摘要不符不应成功")
	}
	if out.Error == "" {
		t.Fatal("失败必须给出原因（云控侧要看得到）")
	}
	b, _ := os.ReadFile(filepath.Join(target, "index.html"))
	if string(b) != "OLD" {
		t.Fatalf("失败后旧版必须完好, got %q", string(b))
	}
}

// TestPeerLocalGoBinary_RejectedWhenUnknownOrNoABI —— I3：换执行体的两道门禁
func TestPeerLocalGoBinary_RejectedWhenUnknownOrNoABI(t *testing.T) {
	// ① 没注入 ENCV_APP_FILES_DIR ⇒ 不知道 filesDir ⇒ 必须拒绝（绝不能猜路径）
	st := &Server{}
	out := st.peerLocalBundleUpdate(peerlink.BundleUpdateRequest{
		Name: "go-binary", Version: "v1", SHA256: "x", ABI: "arm64-v8a",
	})
	if !out.Rejected {
		t.Fatal("拿不到 filesDir 时必须 rejected（不得猜路径写执行体）")
	}

	// ② 有 filesDir 但没带 ABI ⇒ 拒绝（架构不明的二进制可能让设备起不来后端）
	files := t.TempDir()
	t.Setenv("ENCV_APP_FILES_DIR", files)
	out = st.peerLocalBundleUpdate(peerlink.BundleUpdateRequest{
		Name: "go-binary", Version: "v1", SHA256: "x",
	})
	if !out.Rejected || out.Error != "missing_abi" {
		t.Fatalf("缺 ABI 应 rejected: %+v", out)
	}
}

// TestPeerLocalBundleUpdate_UnknownName_Rejected —— 包名没登记 ⇒ rejected（不是"设备故障"）
func TestPeerLocalBundleUpdate_UnknownName_Rejected(t *testing.T) {
	// 直接构造 Server：这两个分支在拿到目标目录之前就返回，不需要路由/连接
	st := &Server{}
	out := st.peerLocalBundleUpdate(peerlink.BundleUpdateRequest{Name: "not-a-bundle", SHA256: "x"})
	if !out.Rejected {
		t.Fatal("未知包名应标记为 Rejected（发起端要拿到 400 而不是 502）")
	}
	out = st.peerLocalBundleUpdate(peerlink.BundleUpdateRequest{Name: "preview-assets"})
	if !out.Rejected || out.Error != "missing_sha256" {
		t.Fatalf("缺摘要应 rejected: %+v", out)
	}
}
