package server

// peerlink_edge_pair_reason_test.go —— 扫码端「失败原因必须带回」的回归锁（2026-10-05）
//
// 真机事故：手机拿一张已经失效的码去配对，远端 Hub 回 401（ticket_expired），
//   而本端 `/edge/pair` 只把它包成一句 `pair_rejected:401` ⇒ 前端只能显示"连接失败"，
//   用户拿着过期码反复扫，界面**没有任何"刷新二维码"的引导**（真机反馈：不知道该怎么办）。
//
// 修复：把远端响应体里的 error 码带回来，并在 /edge/pair 上标 `refreshQr:true`。

import (
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestPeerlinkEdgePair_TicketFailureCarriesReason —— 票据类失败必须带 reason + refreshQr
func TestPeerlinkEdgePair_TicketFailureCarriesReason(t *testing.T) {
	var gotBody string
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"ticket_expired","message":"配对码已过期，请刷新二维码"}`))
	}))
	defer remote.Close()

	r, _ := newPeerlinkRouter()
	pskHex := hex.EncodeToString(make([]byte, 32))
	rec := httptest.NewRecorder()
	edgeReq := httptest.NewRequest("POST", "/api/peerlink/edge/pair", strings.NewReader(jsonBody(t, map[string]string{
		"hub":       remote.URL + "/api/peerlink",
		"pairingId": strings.Repeat("a", 32),
		"psk":       pskHex,
		"deviceId":  "reason-device-0001",
	})))
	edgeReq.Header.Set("X-Peerlink-Operator", "1") // /edge/pair 是本机 UI 端点（运维身份）
	r.ServeHTTP(rec, edgeReq)
	gotBody = rec.Body.String()
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("远端拒绝 ⇒ 本端应是 502, got %d: %s", rec.Code, gotBody)
	}
	var out struct {
		Error     string `json:"error"`
		Reason    string `json:"reason"`
		RefreshQr bool   `json:"refreshQr"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("响应不是 JSON: %v (%s)", err, gotBody)
	}
	if out.Reason != "ticket_expired" {
		t.Fatalf("必须把远端错误码带回来, got reason=%q", out.Reason)
	}
	if !out.RefreshQr {
		t.Fatal("票据类失败必须标 refreshQr=true（前端据此提示刷新二维码）")
	}
}

// TestPeerlinkEdgePair_OldHubWithoutErrorCode —— 旧后端（响应体无 error 字段）不得误标 refreshQr
func TestPeerlinkEdgePair_OldHubWithoutErrorCode(t *testing.T) {
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`boom`))
	}))
	defer remote.Close()

	r, _ := newPeerlinkRouter()
	rec := httptest.NewRecorder()
	edgeReq := httptest.NewRequest("POST", "/api/peerlink/edge/pair", strings.NewReader(jsonBody(t, map[string]string{
		"hub":       remote.URL + "/api/peerlink",
		"pairingId": strings.Repeat("b", 32),
		"psk":       hex.EncodeToString(make([]byte, 32)),
		"deviceId":  "reason-device-0002",
	})))
	edgeReq.Header.Set("X-Peerlink-Operator", "1")
	r.ServeHTTP(rec, edgeReq)
	var out struct {
		Reason    string `json:"reason"`
		RefreshQr bool   `json:"refreshQr"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if out.RefreshQr {
		t.Fatalf("非票据类失败不得标 refreshQr（会把连不上误导成刷新二维码）: %s", rec.Body.String())
	}
	if strings.TrimSpace(out.Reason) != "" {
		t.Fatalf("取不到错误码时 reason 应为空, got %q", out.Reason)
	}
}
