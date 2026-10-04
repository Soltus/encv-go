package server

// peerlink_ticket_restart_test.go —— 「后端重启 ⇒ 手机上正在用的配对码失效」的回归锁
//
// 真机事故（2026-10-05，见 docs/HANDOVER-cloud-hot-update.md §4）：
//
//	热更后重启后端 → 手机连不上：`edge/pair failed: 502` + `pair_rejected:401`
//	后端日志：peerlink pair rejected pairingId=… reason=peerlink: ticket not found (consumed or unknown)
//
// 根因：配对**票据只存进程内存**，而 I1 已经把"会话"落盘了 ⇒ 同一件事只做了一半。
// 本文件在 **HTTP 层**锁住修复后的行为（单测锁在 internal/peerlink/hub_ticket_persist_test.go）。

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Soltus/encv-go/internal/peerlink"
	"github.com/gin-gonic/gin"
)

// postPair 拿票据去配对，返回 (HTTP 码, 错误码)。
func postPair(t *testing.T, r *gin.Engine, pairingID, pskHex, deviceID string) (int, string) {
	t.Helper()
	psk, err := peerlink.DecodePSK(pskHex)
	if err != nil {
		t.Fatalf("psk 解码失败: %v", err)
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest("POST", "/api/peerlink/pair", strings.NewReader(jsonBody(t, map[string]string{
		"pairingId": pairingID,
		"deviceId":  deviceID,
		"name":      "Pixel",
		"platform":  "android",
		"proof":     peerlink.Proof(psk, pairingID, deviceID),
	}))))
	var body struct {
		Error string `json:"error"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	return rec.Code, body.Error
}

// TestPeerlinkTicket_SurvivesBackendRestart —— 事故主锁：重启后旧码必须还能用
func TestPeerlinkTicket_SurvivesBackendRestart(t *testing.T) {
	dir := t.TempDir()

	// ── 进程 1：桌面端出码（二维码已展示在屏幕上）──
	r, s := newPeerlinkRouter()
	s.peerStore = peerlink.NewStore(dir)
	s.peerHub.SetStore(s.peerStore)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest("POST", "/api/peerlink/ticket", strings.NewReader(`{}`)))
	var tk struct {
		PairingID string `json:"pairingId"`
		PSK       string `json:"psk"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &tk); err != nil || tk.PairingID == "" {
		t.Fatalf("ticket 失败: %s", rec.Body.String())
	}
	if _, err := os.Stat(filepath.Join(dir, "tickets.json")); err != nil {
		t.Fatalf("出码必须落盘（否则重启即丢，正是事故根因）: %v", err)
	}

	// ── 进程 2：后端重启（全新 Hub，只靠磁盘恢复）──
	s2 := newHubServerWithStore(t, dir)
	s2.peerConns = peerlink.NewConnRegistry()
	s2.peerCalls = peerlink.NewCaller(s2.peerConns)
	s2.peerHub.SetStore(s2.peerStore)
	if n := s2.restorePeerlinkTickets(); n != 1 {
		t.Fatalf("重启后应恢复 1 张待用票据, got %d", n)
	}
	r2 := gin.New()
	registerPeerlinkRoutes(s2, r2)

	if code, errCode := postPair(t, r2, tk.PairingID, tk.PSK, "restart-device-0001"); code != 200 {
		t.Fatalf("重启后旧配对码必须仍然可用, got %d (%s)", code, errCode)
	}
}

// TestPeerlinkTicket_DistinctErrorCodesOverHTTP —— 三种票据失败必须是三个不同的 error
func TestPeerlinkTicket_DistinctErrorCodesOverHTTP(t *testing.T) {
	r, s := newPeerlinkRouter()
	s.peerStore = peerlink.NewStore(t.TempDir())
	s.peerHub.SetStore(s.peerStore)

	// ① 已使用：同一张码配两次
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest("POST", "/api/peerlink/ticket", strings.NewReader(`{}`)))
	var tk struct {
		PairingID string `json:"pairingId"`
		PSK       string `json:"psk"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &tk); err != nil {
		t.Fatal(err)
	}
	if code, _ := postPair(t, r, tk.PairingID, tk.PSK, "dev-used"); code != 200 {
		t.Fatalf("首次配对应 200, got %d", code)
	}
	code, errCode := postPair(t, r, tk.PairingID, tk.PSK, "dev-used")
	if code != 401 || errCode != "ticket_used" {
		t.Fatalf("二次配对应 401 ticket_used, got %d %q", code, errCode)
	}

	// ② 不存在：一张压根没签发过的码
	code, errCode = postPair(t, r, strings.Repeat("0", 32), tk.PSK, "dev-unknown")
	if code != 401 || errCode != "ticket_not_found" {
		t.Fatalf("未知码应 401 ticket_not_found, got %d %q", code, errCode)
	}

	// ③ 已过期：直接向 Hub 签一张 20ms 的码（HTTP 出码口固定 120s，测不到过期）
	expired, err := s.peerHub.CreateTicket("http://127.0.0.1:2025/api/peerlink", 20*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(40 * time.Millisecond)
	code, errCode = postPair(t, r, expired.PairingID, expired.PSKHex, "dev-expired")
	if code != 401 || errCode != "ticket_expired" {
		t.Fatalf("过期码应 401 ticket_expired, got %d %q", code, errCode)
	}

	// 反向锁：三者互不相同（混成一个就是这次要修的 bug）
	if errCode == "ticket_used" || errCode == "ticket_not_found" {
		t.Fatal("三种票据错误必须互不相同")
	}
}

// TestPeerlinkTicket_PersistDisabledByEnv —— ENCV_PEERLINK_PERSIST=0 退回旧纪律（不落盘）
func TestPeerlinkTicket_PersistDisabledByEnv(t *testing.T) {
	t.Setenv("ENCV_PEERLINK_PERSIST", "0")
	dir := t.TempDir()

	r, s := newPeerlinkRouter()
	s.peerStore = peerlink.NewStore(peerlinkStateDir()) // 禁用 ⇒ dir 为空
	s.peerHub.SetStore(s.peerStore)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest("POST", "/api/peerlink/ticket", strings.NewReader(`{}`)))
	if rec.Code != 200 {
		t.Fatalf("禁用持久化时出码仍应可用, got %d", rec.Code)
	}
	if _, err := os.Stat(filepath.Join(dir, "tickets.json")); err == nil {
		t.Fatal("ENCV_PEERLINK_PERSIST=0 时不得写任何票据文件")
	}
}
