package server

// peerlink_api_test.go —— P2a HTTP/WS 契约测试（spec desktop-web-android-pairing）
//
// 覆盖：hello 未鉴权 / ticket 一次性 / pair 成功与失败分支 /
//       未配对一律 401 / ping / unpair 后 token 立即失效 / WS 建连与 401。

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Soltus/encv-go/internal/peerlink"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

func newPeerlinkRouter() (*gin.Engine, *Server) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	s := &Server{peerHub: peerlink.NewHub("test-hub", "test")}
	// ⚠️ 必须手动补齐 Hub 的连接表/RPC：NewServer 才会初始化它们，
	//    测试辅助直接构造 &Server{} 时是 nil（WS 处理器会 nil 解引用）。
	s.peerConns = peerlink.NewConnRegistry()
	s.peerCalls = peerlink.NewCaller(s.peerConns)
	// 与生产共用同一注册点，杜绝"路由表两套"的漂移
	registerPeerlinkRoutes(s, r)
	return r, s
}

func jsonBody(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(b)
}

func TestPeerlink_Hello_Unauthenticated(t *testing.T) {
	r, _ := newPeerlinkRouter()
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/api/peerlink/hello", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("hello 期望 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("hello 响应不是 JSON: %v", err)
	}
	if resp["peerId"] == "" {
		t.Fatalf("hello 应返回 peerId, got %+v", resp)
	}
}

func TestPeerlink_Ticket_Pair_Ping_Unpair(t *testing.T) {
	r, _ := newPeerlinkRouter()

	// ① 桌面端申请票据
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("POST", "/api/peerlink/ticket", strings.NewReader(`{}`)))
	if w.Code != http.StatusOK {
		t.Fatalf("ticket 期望 200, got %d: %s", w.Code, w.Body.String())
	}
	var tk struct {
		PairingID string `json:"pairingId"`
		PSK       string `json:"psk"`
		Hub       string `json:"hub"`
		ExpiresIn int    `json:"expiresIn"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &tk); err != nil {
		t.Fatalf("ticket 响应解析失败: %v", err)
	}
	if tk.PairingID == "" || tk.PSK == "" || tk.Hub == "" {
		t.Fatalf("ticket 字段不完整: %+v", tk)
	}
	if strings.Contains(tk.Hub, "192.168.") || strings.Contains(tk.Hub, "10.") {
		t.Fatalf("票据不得包含内网地址（R3）: %s", tk.Hub)
	}

	// ② 扫码方配对（正确 proof）
	psk, err := peerlink.DecodePSK(tk.PSK)
	if err != nil {
		t.Fatalf("DecodePSK: %v", err)
	}
	proof := peerlink.Proof(psk, tk.PairingID, "android-device-0001")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("POST", "/api/peerlink/pair",
		strings.NewReader(jsonBody(t, map[string]string{
			"pairingId": tk.PairingID,
			"deviceId":  "android-device-0001",
			"name":      "Pixel",
			"platform":  "android",
			"proof":     proof,
		}))))
	if w.Code != http.StatusOK {
		t.Fatalf("pair 期望 200, got %d: %s", w.Code, w.Body.String())
	}
	var pr struct {
		PeerID string `json:"peerId"`
		Token  string `json:"token"`
		SAS    string `json:"sas"`
	}
	json.Unmarshal(w.Body.Bytes(), &pr)
	if pr.Token == "" || len(pr.SAS) != 6 {
		t.Fatalf("pair 返回不完整: %+v", pr)
	}

	// ③ 票据一次性：重放必须 401
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("POST", "/api/peerlink/pair",
		strings.NewReader(jsonBody(t, map[string]string{
			"pairingId": tk.PairingID, "deviceId": "android-device-0001", "proof": proof,
		}))))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("重放票据期望 401, got %d", w.Code)
	}

	// ④ 无 token 访问受保护端点一律 401
	for _, ep := range []struct{ method, path string }{
		{"GET", "/api/peerlink/peers"},
		{"POST", "/api/peerlink/ping"},
		{"POST", "/api/peerlink/unpair"},
	} {
		w = httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(ep.method, ep.path, strings.NewReader(`{}`)))
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("%s %s 无 token 期望 401, got %d", ep.method, ep.path, w.Code)
		}
	}

	// ⑤ 带 token：ping / peers 应 200
	req := httptest.NewRequest("POST", "/api/peerlink/ping", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Peer "+pr.Token)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("ping 期望 200, got %d: %s", w.Code, w.Body.String())
	}

	// ⑥ 解配后 token 立即失效
	req = httptest.NewRequest("POST", "/api/peerlink/unpair", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Peer "+pr.Token)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("unpair 期望 200, got %d", w.Code)
	}
	req = httptest.NewRequest("POST", "/api/peerlink/ping", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Peer "+pr.Token)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("解配后 ping 期望 401, got %d", w.Code)
	}
}

func TestPeerlink_Pair_BadProof(t *testing.T) {
	r, _ := newPeerlinkRouter()
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("POST", "/api/peerlink/ticket", strings.NewReader(`{}`)))
	var tk struct {
		PairingID string `json:"pairingId"`
	}
	json.Unmarshal(w.Body.Bytes(), &tk)

	wrong := make([]byte, 32)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("POST", "/api/peerlink/pair",
		strings.NewReader(jsonBody(t, map[string]string{
			"pairingId": tk.PairingID,
			"deviceId":  "dev-x",
			"proof":     peerlink.Proof(wrong, tk.PairingID, "dev-x"),
		}))))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("错误 proof 期望 401, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "bad_proof") {
		t.Fatalf("应返回 bad_proof, got %s", w.Body.String())
	}
}

func TestPeerlink_WS_ConnectAndPing(t *testing.T) {
	r, s := newPeerlinkRouter()

	// 先取一个有效 token
	tk, err := s.peerHub.CreateTicket("https://hub.test/api/peerlink", peerlink.DefaultTicketTTL)
	if err != nil {
		t.Fatalf("CreateTicket: %v", err)
	}
	psk, _ := peerlink.DecodePSK(tk.PSKHex)
	res, err := s.peerHub.Pair(tk.PairingID, "dev-ws-1", "Pixel", "android", peerlink.Proof(psk, tk.PairingID, "dev-ws-1"))
	if err != nil {
		t.Fatalf("Pair: %v", err)
	}

	srv := httptest.NewServer(r)
	defer srv.Close()

	// ① 无 token → 拒绝升级（401）
	if _, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http")+"/api/peerlink/ws", nil); err == nil {
		t.Fatal("无 token 的 WS 应被拒绝")
	}

	// ② 有 token → hello_ok + ping/pong
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http")+"/api/peerlink/ws?token="+res.Token, nil)
	if err != nil {
		t.Fatalf("WS 建连失败: %v", err)
	}
	defer conn.Close()

	var hello map[string]any
	if err := conn.ReadJSON(&hello); err != nil {
		t.Fatalf("读 hello_ok 失败: %v", err)
	}
	if hello["type"] != "hello_ok" {
		t.Fatalf("期望 hello_ok, got %+v", hello)
	}

	if err := conn.WriteJSON(map[string]string{"type": "ping"}); err != nil {
		t.Fatalf("写 ping 失败: %v", err)
	}
	var pong map[string]any
	if err := conn.ReadJSON(&pong); err != nil {
		t.Fatalf("读 pong 失败: %v", err)
	}
	if pong["type"] != "pong" {
		t.Fatalf("期望 pong, got %+v", pong)
	}
}
