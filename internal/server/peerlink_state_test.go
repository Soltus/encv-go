package server

// peerlink_state_test.go —— 互联状态持久化与**重启自恢复**的回归锁（2026-10-04）
//
// 锁的语义（用户拍板：设计过于谨慎，服务端重启要能自动恢复连接）：
//  1. 配对后落盘 ⇒ 新进程启动能恢复出同一台设备、同一个 token
//  2. 恢复出来的设备 **Online 恒为 false**（在线只能由长连接证明，不能从磁盘读出来）
//  3. 解配 ⇒ 落盘同步删除（否则"被解配的设备"重启后又活了）
//  4. ENCV_PEERLINK_PERSIST=0 ⇒ 完全不落盘，行为退回旧纪律
//
// ⚠️ 真机复验：需引导版 APK（含本轮 Go 改动）装机后验"APP 冷启动自动重连"。

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Soltus/encv-go/internal/peerlink"
)

// newHubServerWithStore 造一个带持久化目录的 Server（模拟"进程启动"）。
func newHubServerWithStore(t *testing.T, dir string) *Server {
	t.Helper()
	return &Server{
		peerHub:   peerlink.NewHub("test-hub", "test"),
		peerStore: peerlink.NewStore(dir),
	}
}

func TestPeerlinkState_PersistAndRestoreAcrossRestart(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ENCV_PEERLINK_DIR", dir)

	// ── 进程 1：配对 ──
	r, s := newPeerlinkRouter()
	s.peerStore = peerlink.NewStore(dir)
	srv := httptest.NewServer(r)
	defer srv.Close()

	token := pairPeerOnHub(t, r)
	if _, err := os.Stat(filepath.Join(dir, "peers.json")); err != nil {
		t.Fatalf("配对后应落盘 peers.json: %v", err)
	}
	peerID, ok := s.peerHub.PeerIDForToken(token)
	if !ok {
		t.Fatal("配对后 token 应可用")
	}

	// ── 进程 2：重启（新 Hub，只靠磁盘恢复）──
	s2 := newHubServerWithStore(t, dir)
	if n := s2.restorePeerlinkState(); n != 1 {
		t.Fatalf("重启后应恢复 1 台设备, got %d", n)
	}
	if got, ok := s2.peerHub.PeerIDForToken(token); !ok || got != peerID {
		t.Fatalf("恢复后旧 token 必须继续可用, got %q ok=%v want %q", got, ok, peerID)
	}
	// ⚠️ 在线状态绝不能从磁盘"恢复"出来 —— 设备此刻并没有连上
	if p, ok := s2.peerHub.PeerByID(peerID); !ok || p.Online {
		t.Fatalf("恢复出来的设备 Online 必须为 false: %+v", p)
	}
}

func TestPeerlinkState_UnpairRemovesPersisted(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ENCV_PEERLINK_DIR", dir)

	r, s := newPeerlinkRouter()
	s.peerStore = peerlink.NewStore(dir)
	token := pairPeerOnHub(t, r)
	peerID, _ := s.peerHub.PeerIDForToken(token)

	req := httptest.NewRequest("POST", "/api/peerlink/unpair",
		strings.NewReader(jsonBody(t, map[string]string{"peerId": peerID})))
	req.Header.Set("X-Peerlink-Operator", "1") // 运维解配（否则要求 peer token）
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("unpair 失败: %d %s", rec.Code, rec.Body.String())
	}

	// 解配后：磁盘里也不该再有它（否则重启即复活）
	s2 := newHubServerWithStore(t, dir)
	if n := s2.restorePeerlinkState(); n != 0 {
		t.Fatalf("解配后不应再恢复出设备, got %d", n)
	}
	if _, ok := s2.peerHub.PeerIDForToken(token); ok {
		t.Fatal("解配后旧 token 不得继续有效")
	}
}

func TestPeerlinkState_DisabledByEnv(t *testing.T) {
	t.Setenv("ENCV_PEERLINK_PERSIST", "0")
	if d := peerlinkStateDir(); d != "" {
		t.Fatalf("ENCV_PEERLINK_PERSIST=0 应禁用落盘, got %q", d)
	}
	st := peerlink.NewStore(peerlinkStateDir())
	if st.Enabled() {
		t.Fatal("禁用时 Enabled 应为 false")
	}
	if err := st.SavePeers(nil); err == nil {
		t.Fatal("禁用时写入应返回错误（调用方据此跳过）")
	}
}

func TestPeerlinkState_EdgeSessionSaveRestoreClear(t *testing.T) {
	dir := t.TempDir()
	s := &Server{peerHub: peerlink.NewHub("t", "t"), peerStore: peerlink.NewStore(dir)}

	// 没有会话时：恢复应安静返回 false
	if s.restoreEdgeSession() {
		t.Fatal("无会话时不应拉起 Edge")
	}
	s.saveEdgeSession("http://127.0.0.1:1/api/peerlink", "peer-0001", "dev-0001", "tok-abc")

	sess, ok, err := s.peerStore.LoadEdgeSession()
	if err != nil || !ok {
		t.Fatalf("应能读回 Edge 会话: ok=%v err=%v", ok, err)
	}
	if sess.Hub == "" || sess.Token != "tok-abc" || sess.DeviceID != "dev-0001" {
		t.Fatalf("会话内容不对: %+v", sess)
	}
	// 文件权限必须是 0600（凭据文件）
	st, err := os.Stat(filepath.Join(dir, "edge-session.json"))
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm()&0o077 != 0 {
		t.Fatalf("凭据文件权限应仅属主可读写, got %v", st.Mode().Perm())
	}

	// 主动断开 ⇒ 忘记会合点（否则下次启动又自动连回去）
	s.clearEdgeSession()
	if _, ok, _ := s.peerStore.LoadEdgeSession(); ok {
		t.Fatal("clearEdgeSession 后不应再有会话")
	}
}

// TestPeerlinkState_RestoredPeerIsNotOnline —— 反向锁：磁盘里就算写着 online 也不得算在线
func TestPeerlinkState_RestoredPeerIsNotOnline(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	// 手工造一份"在线=true"的落盘状态（模拟磁盘被改 / 旧版本写入）
	raw := `[{"peer":{"id":"p1","deviceId":"d1","name":"Pixel","platform":"android","online":true,"pairedAt":"2026-10-04T00:00:00Z"},"token":"tok","remoteKey":"","localKey":""}]`
	if err := os.WriteFile(filepath.Join(dir, "peers.json"), []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	s := newHubServerWithStore(t, dir)
	if n := s.restorePeerlinkState(); n != 1 {
		t.Fatalf("应恢复 1 台, got %d", n)
	}
	p, _ := s.peerHub.PeerByID("p1")
	if p.Online {
		t.Fatal("恢复时 Online 必须被强制为 false（在线只能由长连接证明）")
	}
	if _, ok := s.peerHub.PeerIDForToken("tok"); !ok {
		t.Fatal("恢复后 token 应可用")
	}
}

// TestPeerlinkState_CorruptedFileDoesNotBreakBoot —— 坏文件不得阻止启动
func TestPeerlinkState_CorruptedFileDoesNotBreakBoot(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "peers.json"), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := newHubServerWithStore(t, dir)
	if n := s.restorePeerlinkState(); n != 0 {
		t.Fatalf("坏文件应恢复 0 台（而不是 panic/误恢复）, got %d", n)
	}
	_ = json.Marshal // 保持 import 稳定
}
