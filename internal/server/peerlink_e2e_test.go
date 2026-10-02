package server

// peerlink_e2e_test.go —— Hub ⇄ Edge 端到端（spec desktop-web-android-pairing Task 2.4）
//
// 拓扑复现（不依赖真机也能证伪的部分）：
//   Hub = 本测试的 httptest 服务（等价于 cnb 上的 encv-go，公网可达）
//   Edge = internal/peerlink.Edge，**主动出网**拨号（等价于手机端在 NAT 后主动连出）
//   ⇒ 验证「手机侧主动出网 → 配对 → 心跳 → 在线状态 → 解配后失效」整条链路。
//
// 真机/移动网相关（扫码、息屏保活、IPv6-only）仍需真机验证，见 checklist。

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Soltus/encv-go/internal/peerlink"
)

func TestPeerlinkE2E_EdgeConnectsAndGoesOnline(t *testing.T) {
	r, s := newPeerlinkRouter()

	// ① 桌面端申请票据（HTTP）
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest("POST", "/api/peerlink/ticket", strings.NewReader(`{}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("ticket 期望 200, got %d", rec.Code)
	}
	var tk struct {
		PairingID string `json:"pairingId"`
		PSK       string `json:"psk"`
	}
	json.Unmarshal(rec.Body.Bytes(), &tk)

	// ② 扫码方完成配对（HTTP，带正确 proof）
	psk, err := peerlink.DecodePSK(tk.PSK)
	if err != nil {
		t.Fatalf("DecodePSK: %v", err)
	}
	proof := peerlink.Proof(psk, tk.PairingID, "e2e-device-0001")
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest("POST", "/api/peerlink/pair", strings.NewReader(
		jsonBody(t, map[string]string{
			"pairingId": tk.PairingID,
			"deviceId":  "e2e-device-0001",
			"name":      "Pixel",
			"platform":  "android",
			"proof":     proof,
		}))))
	if rec.Code != http.StatusOK {
		t.Fatalf("pair 期望 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var pr struct {
		PeerID string `json:"peerId"`
		Token  string `json:"token"`
	}
	json.Unmarshal(rec.Body.Bytes(), &pr)

	// ③ 配对状态可按 pairingID 查（桌面端没有 token 也能查）
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest("GET", "/api/peerlink/pairing/status?pairingId="+tk.PairingID, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("pairing/status 期望 200, got %d", rec.Code)
	}

	// ④ Edge（手机侧）**主动出网**连 Hub
	srv := httptest.NewServer(r)
	defer srv.Close()

	edge := peerlink.NewEdge(peerlink.EdgeOptions{
		HubURL:              srv.URL + "/api/peerlink",
		Token:               pr.Token,
		DeviceID:            "e2e-device-0001",
		HeartbeatForeground: 50 * time.Millisecond,
		MinBackoff:          10 * time.Millisecond,
		MaxBackoff:          50 * time.Millisecond,
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go edge.Start(ctx)

	// ⑤ 心跳到达后，Hub 侧应显示为在线（用运维身份查列表）
	deadline := time.Now().Add(3 * time.Second)
	var online bool
	for time.Now().Before(deadline) {
		req, _ := http.NewRequest("GET", srv.URL+"/api/peerlink/peers", nil)
		req.Header.Set("X-Peerlink-Operator", "1")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			time.Sleep(20 * time.Millisecond)
			continue
		}
		var out struct {
			Items []map[string]any `json:"items"`
		}
		json.NewDecoder(resp.Body).Decode(&out)
		resp.Body.Close()
		if len(out.Items) > 0 {
			if v, ok := out.Items[0]["online"].(bool); ok && v {
				online = true
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !online {
		t.Fatal("Edge 心跳后 Hub 侧应显示 online=true")
	}

	// ⑥ 解配 → Edge 会话失效（下一次心跳/重连被拒）
	req, _ := http.NewRequest("POST", srv.URL+"/api/peerlink/unpair", strings.NewReader(`{"peerId":"`+pr.PeerID+`"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Peerlink-Operator", "1")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("unpair 请求失败: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unpair 期望 200, got %d", resp.StatusCode)
	}

	// 解配后：Hub 侧不应再有该 peer
	req2, _ := http.NewRequest("GET", srv.URL+"/api/peerlink/peers", nil)
	req2.Header.Set("X-Peerlink-Operator", "1")
	resp2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatalf("peers 请求失败: %v", err)
	}
	var out2 struct {
		Items []map[string]any `json:"items"`
	}
	json.NewDecoder(resp2.Body).Decode(&out2)
	resp2.Body.Close()
	if len(out2.Items) != 0 {
		t.Fatalf("解配后 peers 应为空, got %d", len(out2.Items))
	}

	edge.Close()
	// s 未用到，但保持引用以表明 Hub 在同进程（与真实部署一致：Hub 就是本进程）
	_ = s
}
