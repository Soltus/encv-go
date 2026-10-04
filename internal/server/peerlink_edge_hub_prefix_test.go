package server

// peerlink_edge_hub_prefix_test.go —— 2026-10-04 真机事故的两条回归锁
//
// 真因 1（P2b 连接失败）：票据里的 hub **自带** `/api/peerlink` 后缀
//   （如 http://host/api/peerlink），而 startEdgeLocked 又拼了一次 ⇒
//   Edge 去连 `/api/peerlink/api/peerlink/ws`（双前缀）⇒ 404 / bad handshake
//   ⇒ 手机端永远"连接中…"、桌面端显示离线 ⇒ 用户看到"信任设备后还是连接失败"。
//   ⚠️ 旧测试用的是 `srvA.URL`（**不带**后缀），所以这个 bug 从来没被覆盖到 ——
//      本测试刻意用**真实票据形态**的 hub（带后缀）。
//
// 真因 2（P2b 手机端无 SAS）：远端 /pair 会回 sas，但 /edge/pair 没透出
//   ⇒ 桌面端在核对安全码，手机端却什么都不显示。本测试锁定 sas 必须双向一致。

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

// hubTicketWithHub 在 Hub 侧申请票据，并返回票据里的 hub（**带 /api/peerlink 后缀**）。
func hubTicketFull(t *testing.T, r *gin.Engine, hubURL string) (pairingID, psk, hub string) {
	t.Helper()
	body := `{}`
	if hubURL != "" {
		b, _ := json.Marshal(map[string]string{"hub": hubURL})
		body = string(b)
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest("POST", "/api/peerlink/ticket", strings.NewReader(body)))
	var tk struct {
		PairingID string `json:"pairingId"`
		PSK       string `json:"psk"`
		Hub       string `json:"hub"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &tk); err != nil || tk.PairingID == "" {
		t.Fatalf("票据申请失败: %s", rec.Body.String())
	}
	return tk.PairingID, tk.PSK, tk.Hub
}

func TestPeerlinkEdge_HubWithApiPeerlinkPrefix_ConnectsWithoutDoublePrefix(t *testing.T) {
	// ── A：Hub ──
	rA, _ := newPeerlinkRouter()
	srvA := httptest.NewServer(rA)
	defer srvA.Close()

	// 票据的 hub = 真实形态（带 /api/peerlink 后缀），这是本测试的关键
	pairingID, psk, hub := hubTicketFull(t, rA, srvA.URL+"/api/peerlink")
	if !strings.HasSuffix(hub, "/api/peerlink") {
		t.Fatalf("票据 hub 形态不符合真实场景（应带 /api/peerlink 后缀）: %q", hub)
	}

	// ── B：Edge ──
	_, srvB := startEdgeServer(t, peerEdgeHandlers{})
	defer srvB.Close()

	body, _ := json.Marshal(map[string]string{
		"hub":       hub,
		"pairingId": pairingID,
		"psk":       psk,
		"deviceId":  "prefix-probe-0001",
		"name":      "PrefixProbe",
	})
	req, _ := http.NewRequest("POST", srvB.URL+"/api/peerlink/edge/pair", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Peerlink-Operator", "1")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("edge/pair 请求失败: %v", err)
	}
	var paired struct {
		OK     bool   `json:"ok"`
		PeerID string `json:"peerId"`
		SAS    string `json:"sas"`
	}
	json.NewDecoder(resp.Body).Decode(&paired)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !paired.OK {
		t.Fatalf("edge/pair 失败: status=%d body=%+v", resp.StatusCode, paired)
	}

	// ① Edge 的 hub 必须归一化（不得出现 /api/peerlink/api/peerlink）
	rSt, _ := http.NewRequest("GET", srvB.URL+"/api/peerlink/edge/status", nil)
	rSt.Header.Set("X-Peerlink-Operator", "1")
	stResp, err := http.DefaultClient.Do(rSt)
	if err != nil {
		t.Fatalf("edge/status 请求失败: %v", err)
	}
	var stt struct {
		Running   bool   `json:"running"`
		Connected bool   `json:"connected"`
		Hub       string `json:"hub"`
		LastErr   string `json:"lastErr"`
	}
	json.NewDecoder(stResp.Body).Decode(&stt)
	stResp.Body.Close()

	if strings.Contains(stt.Hub, "/api/peerlink/api/peerlink") {
		t.Fatalf("Edge hub 出现双前缀: %q", stt.Hub)
	}
	if stt.Hub != srvA.URL+"/api/peerlink" {
		t.Fatalf("Edge hub 未归一化: got %q", stt.Hub)
	}

	// ② 必须真的连上（A 侧看到 online）—— 双前缀时这里会一直 false
	var online bool
	deadline := time.Now().Add(6 * time.Second)
	for time.Now().Before(deadline) && !online {
		r2, _ := http.NewRequest("GET", srvA.URL+"/api/peerlink/peers", nil)
		r2.Header.Set("X-Peerlink-Operator", "1")
		pr, err := http.DefaultClient.Do(r2)
		if err == nil {
			var d struct {
				Items []struct {
					ID     string `json:"id"`
					Online bool   `json:"online"`
				} `json:"items"`
			}
			json.NewDecoder(pr.Body).Decode(&d)
			pr.Body.Close()
			for _, it := range d.Items {
				if it.ID == paired.PeerID && it.Online {
					online = true
					break
				}
			}
		}
		time.Sleep(60 * time.Millisecond)
	}
	if !online {
		t.Fatalf("带 /api/peerlink 后缀的 hub 未能连上（lastErr=%q hub=%q）—— 疑似双前缀回归", stt.LastErr, stt.Hub)
	}

	// ③ SAS 必须透到扫码端，且与 Hub 侧一致（手机端要显示给用户核对）
	if paired.SAS == "" {
		t.Fatalf("/edge/pair 未返回 sas —— 手机端将无从核对")
	}
	rPs, _ := http.NewRequest("GET", srvA.URL+"/api/peerlink/pairing/status?pairingId="+pairingID, nil)
	rPs.Header.Set("X-Peerlink-Operator", "1")
	psResp, err := http.DefaultClient.Do(rPs)
	if err != nil {
		t.Fatalf("pairing/status 请求失败: %v", err)
	}
	var psOut struct {
		SAS string `json:"sas"`
	}
	json.NewDecoder(psResp.Body).Decode(&psOut)
	psResp.Body.Close()
	if psOut.SAS == "" {
		t.Fatalf("Hub 侧 pairing/status 未返回 sas")
	}
	if paired.SAS != psOut.SAS {
		t.Fatalf("两端 SAS 不一致: edge=%q hub=%q", paired.SAS, psOut.SAS)
	}
}
