package server

// peerlink_bundle_rollback_test.go —— 热更**记录持久化**与**回滚**的回归锁（2026-10-05）
//
// 交接待办 ②（docs/HANDOVER-cloud-hot-update.md §5）：
//   - 记录：globalBundleReports 是纯内存，真机实测重启后 reportCount 1 → 0
//     ⇒ "这台设备装到哪版了""上次下发成功还是失败"全部消失，云控退化成"每次都当没推过"；
//   - 回滚：此前只有设备端 `RollbackFile`（装坏自保），**云端没有任何一键回滚入口**。
//
// 锁的语义：
//  1. 台账落盘 ⇒ 重启后仍在（且"设备当前版本"也一起恢复）
//  2. 云控回滚是**独立 RPC**，执行端真收到 bundle_rollback 指令（含接线锁，防 2026-10-05 那种漏传）
//  3. 回滚必须进台账且标 rolledBack（否则云控视角里设备版本停在坏包那一版）
//  4. 本端（设备侧）可查自己装了哪版、能否回滚，并可自回滚

import (
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

// TestPeerlinkBundleReports_PersistAcrossRestart —— 台账重启后仍在（实测 1 → 0 的那个 bug）
func TestPeerlinkBundleReports_PersistAcrossRestart(t *testing.T) {
	dir := t.TempDir()
	// 用干净台账起测（globalBundleReports 是包级变量，别被别的用例污染）
	globalBundleReports = &bundleReports{cap: 50, byPeer: map[string][]bundleReport{}, lastVer: map[string]string{}}
	t.Cleanup(func() {
		globalBundleReports = &bundleReports{cap: 50, byPeer: map[string][]bundleReport{}, lastVer: map[string]string{}}
	})

	s := &Server{peerStore: peerlink.NewStore(dir)}
	globalBundleReports.add(bundleReport{PeerID: "peer-1", Name: "web", Version: "v1", Ok: true})
	s.saveBundleReports()

	// 重启：全新内存台账，只靠磁盘恢复
	globalBundleReports = &bundleReports{cap: 50, byPeer: map[string][]bundleReport{}, lastVer: map[string]string{}}
	if n := s.restoreBundleReports(); n != 1 {
		t.Fatalf("重启后应恢复 1 条台账, got %d", n)
	}
	_, versions := globalBundleReports.snapshot()
	if versions["peer-1"] != "web@v1" {
		t.Fatalf("设备当前版本必须一起恢复（云控据此判断要不要再推）, got %+v", versions)
	}
}

// TestPeerlinkBundleRollback_CloudEndToEnd —— 云控一键回滚：指令真的到执行端，且进台账
func TestPeerlinkBundleRollback_CloudEndToEnd(t *testing.T) {
	globalBundleReports = &bundleReports{cap: 50, byPeer: map[string][]bundleReport{}, lastVer: map[string]string{}}
	t.Cleanup(func() {
		globalBundleReports = &bundleReports{cap: 50, byPeer: map[string][]bundleReport{}, lastVer: map[string]string{}}
	})

	r, s := newPeerlinkRouter()
	srv := httptest.NewServer(r)
	defer srv.Close()
	s.peerStore = peerlink.NewStore(t.TempDir())

	var got peerlink.BundleRollbackRequest
	peerID := startPairedEdgeWithRollback(t, r, s, srv.URL+"/api/peerlink",
		func(req peerlink.BundleRollbackRequest) peerlink.BundleRollbackResult {
			got = req
			return peerlink.BundleRollbackResult{
				Ok: true, Name: req.Name, Version: "v0.0.9", PreviousVersion: "v0.0.10",
			}
		})

	body, _ := json.Marshal(map[string]string{"peerId": peerID, "name": "web"})
	req, _ := http.NewRequest("POST", srv.URL+"/api/peerlink/bundle/rollback", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Peerlink-Operator", "1")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("rollback 失败: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("rollback 期望 200, got %d", resp.StatusCode)
	}
	if got.Name != "web" {
		t.Fatalf("执行端未收到回滚指令: %+v", got)
	}
	// 台账：回滚本身是一次变更，必须留痕并标 rolledBack
	items, versions := globalBundleReports.snapshot()
	found := false
	for _, it := range items {
		if it.PeerID == peerID && it.Name == "web" && it.RolledBack {
			found = true
			if it.Version != "v0.0.9" || it.PreviousVersion != "v0.0.10" {
				t.Fatalf("台账要能回答从哪版退到哪版: %+v", it)
			}
		}
	}
	if !found {
		t.Fatalf("回滚未进台账: %+v", items)
	}
	if versions[peerID] != "web@v0.0.9" {
		t.Fatalf("回滚后云控视角的设备版本必须更新为退回后的版本, got %q", versions[peerID])
	}
}

// TestPeerEdge_WiresBundleRollback —— 接线锁：回滚 handler 必须真的传给 Edge
//
// 2026-10-05 真机首测就在 bundle_update 上栽过：handler 只在映射里加了、没传进
// EdgeOptions ⇒ 设备一律回 not_supported ⇒ Hub 侧 502。同一个坑对回滚一样成立。
func TestPeerEdge_WiresBundleRollback(t *testing.T) {
	r, s := newPeerlinkRouter()
	_ = r
	s.startEdgeLocked("http://127.0.0.1:1/api/peerlink", "peer-y", "dev-y", "tok-y")
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
	for _, m := range []string{peerlink.MethodBundleUpdate, peerlink.MethodBundleRollback} {
		if !rt.edge.Supports(m) {
			t.Errorf("Supports(%q) = false，执行端会回 not_supported（= 云控回滚永远失败）", m)
		}
	}
}

// startPairedEdgeWithRollback 建立"已配对 + Edge 已连上"，注入云控回滚处理器。
//
// ⚠️ 与 startPairedEdgeWithBundle 的区别只有注入的 handler（它注入 update，这里注入 rollback）。
func startPairedEdgeWithRollback(
	t *testing.T,
	r *gin.Engine,
	s *Server,
	hubURL string,
	onRollback func(peerlink.BundleRollbackRequest) peerlink.BundleRollbackResult,
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
		"deviceId":  "bundle-device-0003",
		"name":      "Pixel",
		"platform":  "android",
		"proof":     peerlink.Proof(psk, tk.PairingID, "bundle-device-0003"),
	}))))
	var pr struct {
		PeerID string `json:"peerId"`
		Token  string `json:"token"`
	}
	json.Unmarshal(rec.Body.Bytes(), &pr)

	edge := peerlink.NewEdge(peerlink.EdgeOptions{
		HubURL:              hubURL,
		Token:               pr.Token,
		DeviceID:            "bundle-device-0003",
		HeartbeatForeground: 80 * time.Millisecond,
		MinBackoff:          10 * time.Millisecond,
		MaxBackoff:          50 * time.Millisecond,
		OnBundleRollback:    onRollback,
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

// TestPeerlinkBundleLocal_StatesAndSelfRollback —— 设备端：查得到版本，且能自回滚
func TestPeerlinkBundleLocal_StatesAndSelfRollback(t *testing.T) {
	target := t.TempDir()
	t.Setenv("ENCV_WEB_BUNDLE_DIR", target)

	// ① 造一个"已装 v1"的现场：手工放好 v1 的内容与 version.json，再把 v2 应用进去
	//    （Apply 会把当前目录整体移到 web-backup ⇒ 这就是回滚要取回的那一份）
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "index.html"), []byte("<html>v1</html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "version.json"), []byte(`{"version":"v1"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	bundles := t.TempDir()
	_, sha := writeTestBundle(t, bundles, "web", "v2", map[string]string{"index.html": "<html>v2</html>"})
	if _, err := bundle.Apply(bundle.Spec{
		Name: "web", TargetDir: target, Version: "v2", SHA256: sha, Required: []string{"index.html"},
	}, filepath.Join(bundles, "web-v2.zip"), bundle.Options{}); err != nil {
		t.Fatalf("预置 v2 失败: %v", err)
	}

	r, _ := newPeerlinkRouter()
	// ② 本端状态：装的是 v2，且有备份可退
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/peerlink/bundle/local", nil)
	req.Header.Set("X-Peerlink-Operator", "1")
	r.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("/bundle/local 期望 200, got %d", rec.Code)
	}
	var st struct {
		Items []struct {
			Name     string `json:"name"`
			Version  string `json:"version"`
			Rollable bool   `json:"rollable"`
		} `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &st); err != nil {
		t.Fatal(err)
	}
	var web *struct {
		Name     string `json:"name"`
		Version  string `json:"version"`
		Rollable bool   `json:"rollable"`
	}
	for i := range st.Items {
		if st.Items[i].Name == "web" {
			web = &st.Items[i]
		}
	}
	if web == nil {
		t.Fatalf("/bundle/local 未列出 web: %s", rec.Body.String())
	}
	if web.Version != "v2" {
		t.Fatalf("生效版本应为 v2, got %q", web.Version)
	}
	if !web.Rollable {
		t.Fatal("有备份时 rollable 应为 true（UI 据此才敢显示回滚按钮）")
	}

	// ③ 自回滚：退回 v1，且磁盘内容真的是 v1
	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest("POST", "/api/peerlink/bundle/local/rollback", strings.NewReader(`{"name":"web"}`))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("X-Peerlink-Operator", "1")
	r.ServeHTTP(rec2, req2)
	if rec2.Code != 200 {
		t.Fatalf("本地回滚期望 200, got %d: %s", rec2.Code, rec2.Body.String())
	}
	var out peerlink.BundleRollbackResult
	if err := json.Unmarshal(rec2.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if !out.Ok || out.Version != "v1" || out.PreviousVersion != "v2" {
		t.Fatalf("回滚结果不对: %+v", out)
	}
	got, err := os.ReadFile(filepath.Join(target, "index.html"))
	if err != nil || string(got) != "<html>v1</html>" {
		t.Fatalf("回滚后磁盘内容必须是上一版, got %q err=%v", string(got), err)
	}
}

// TestPeerlinkBundleLocalRollback_NoBackupIsBadRequest —— 没备份可退 ⇒ 400，不是 500
func TestPeerlinkBundleLocalRollback_NoBackupIsBadRequest(t *testing.T) {
	t.Setenv("ENCV_WEB_BUNDLE_DIR", t.TempDir())

	r, _ := newPeerlinkRouter()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/peerlink/bundle/local/rollback", strings.NewReader(`{"name":"web"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Peerlink-Operator", "1")
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("没有备份可退应 400（不是设备故障）, got %d: %s", rec.Code, rec.Body.String())
	}
	var out struct {
		Error  string `json:"error"`
		Reason string `json:"reason"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if !strings.Contains(out.Reason, "no_backup") {
		t.Fatalf("原因应含 no_backup, got %+v", out)
	}
}
