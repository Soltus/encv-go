package server

// peerlink_edge_runtime_test.go —— P2a Task 2.2 接线端到端
//
// 场景（**两个独立进程内 Server + 真实 WebSocket**）：
//
//	A = Hub（cnb 侧）：出票据 → 被扫码配对 → 可反向查询/调用对端
//	B = Edge（手机侧）：拿票据去 A 配对 → 启动 Edge 守护（主动出网）→ 常驻
//
// 证明：P3 联邦搜索 / P4 远程 Agent / P3.4 远端读 的**完整链路**真的通了
// （此前这些只在本进程内用 startPairedEdge 测过，Edge 从未真正由接线代码启动）。

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Soltus/encv-go/internal/peerlink"
	"github.com/gin-gonic/gin"
)

// startEdgeServer 起一个"要作为 Edge 连出去"的 server（B 侧），可注入处理器。
func startEdgeServer(t *testing.T, h peerEdgeHandlers) (*Server, *httptest.Server) {
	t.Helper()
	r, s := newPeerlinkRouter()
	s.peerEdgeHandlersOverride = &h
	srv := httptest.NewServer(r)
	return s, srv
}

// hubTicket 在 Hub（A 侧）申请票据，返回 pairingId + psk。
func hubTicket(t *testing.T, r *gin.Engine) (string, string) {
	t.Helper()
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest("POST", "/api/peerlink/ticket", strings.NewReader(`{}`)))
	var tk struct {
		PairingID string `json:"pairingId"`
		PSK       string `json:"psk"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &tk); err != nil || tk.PairingID == "" {
		t.Fatalf("票据申请失败: %s", rec.Body.String())
	}
	return tk.PairingID, tk.PSK
}

func TestPeerlinkEdgeRuntime_PairThenHubQueriesEdge(t *testing.T) {
	// ── A：Hub ──
	rA, _ := newPeerlinkRouter()
	srvA := httptest.NewServer(rA)
	defer srvA.Close()

	pairingID, psk := hubTicket(t, rA)

	// ── B：Edge（注入本端处理器，避免依赖真实索引/磁盘）──
	_, srvB := startEdgeServer(t, peerEdgeHandlers{
		OnSearch: func(req peerlink.SearchRequest) (json.RawMessage, error) {
			return json.Marshal([]map[string]any{
				{"path": "/sdcard/Download/报告.pdf", "name": "报告.pdf", "size": 1024},
			})
		},
		OnRead: func(req peerlink.ReadRequest) ([]byte, error) {
			return []byte("REMOTE-BYTES"), nil
		},
		OnAgentInvoke: func(req peerlink.AgentInvokeRequest) peerlink.AgentInvokeOutcome {
			return peerlink.AgentInvokeOutcome{
				Decision: peerlink.DecisionAccept,
				Result:   json.RawMessage(`{"echo":"` + req.Tool + `"}`),
			}
		},
	})
	defer srvB.Close()

	// ── B 拿票据去 A 配对并启动 Edge ──
	body, _ := json.Marshal(map[string]string{
		"hub":       srvA.URL,
		"pairingId": pairingID,
		"psk":       psk,
		"deviceId":  "edge-device-0001",
		"name":      "Pixel",
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
	}
	json.NewDecoder(resp.Body).Decode(&paired)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !paired.OK || paired.PeerID == "" {
		t.Fatalf("edge/pair 失败: status=%d body=%+v", resp.StatusCode, paired)
	}

	// ── 等 Edge 真正连上（A 侧 peers 显示 online）──
	var online bool
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
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
		if online {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !online {
		t.Fatalf("Edge 未连上 Hub（A 侧未见 online peer %s）", paired.PeerID)
	}

	// ── P3：A 联邦搜索 B ──
	r3, _ := http.NewRequest("GET", srvA.URL+"/api/peerlink/search?peerId="+paired.PeerID+"&q=%E6%8A%A5%E5%91%8A&kind=fulltext&limit=10", nil)
	r3.Header.Set("X-Peerlink-Operator", "1")
	sr, err := http.DefaultClient.Do(r3)
	if err != nil {
		t.Fatalf("search 请求失败: %v", err)
	}
	var sd struct {
		Items []struct {
			Path string `json:"path"`
		} `json:"items"`
		Peer struct {
			Name string `json:"name"`
		} `json:"peer"`
	}
	json.NewDecoder(sr.Body).Decode(&sd)
	sr.Body.Close()
	if sr.StatusCode != http.StatusOK || len(sd.Items) == 0 {
		t.Fatalf("联邦搜索失败: status=%d %+v", sr.StatusCode, sd)
	}
	// 契约：远端路径**原样**返回（不改写成本地路径语义）
	if !strings.HasPrefix(sd.Items[0].Path, "/sdcard/") {
		t.Fatalf("远端路径被改写: %q", sd.Items[0].Path)
	}
	if sd.Peer.Name != "Pixel" {
		t.Fatalf("缺少对端标注: %+v", sd.Peer)
	}

	// ── P3.4：A 读 B 的文件分片 ──
	r4, _ := http.NewRequest("GET", srvA.URL+"/api/peerlink/file?peerId="+paired.PeerID+"&path=%2Fsdcard%2Fa.txt&length=32", nil)
	r4.Header.Set("X-Peerlink-Operator", "1")
	fr, err := http.DefaultClient.Do(r4)
	if err != nil {
		t.Fatalf("file 请求失败: %v", err)
	}
	var buf bytes.Buffer
	buf.ReadFrom(fr.Body)
	fr.Body.Close()
	if fr.StatusCode != http.StatusOK || buf.String() != "REMOTE-BYTES" {
		t.Fatalf("远端读失败: status=%d body=%q", fr.StatusCode, buf.String())
	}
	if fr.Header.Get("X-Peer-Id") != paired.PeerID {
		t.Fatalf("远端读响应缺少来源标注: %q", fr.Header.Get("X-Peer-Id"))
	}

	// ── P4：A 远程 invoke B ──
	ib, _ := json.Marshal(map[string]string{"peerId": paired.PeerID, "tool": "read_file", "args": "{}"})
	r5, _ := http.NewRequest("POST", srvA.URL+"/api/peerlink/agent/invoke", bytes.NewReader(ib))
	r5.Header.Set("Content-Type", "application/json")
	r5.Header.Set("X-Peerlink-Operator", "1")
	ir, err := http.DefaultClient.Do(r5)
	if err != nil {
		t.Fatalf("agent/invoke 请求失败: %v", err)
	}
	var out peerlink.AgentInvokeResult
	json.NewDecoder(ir.Body).Decode(&out)
	ir.Body.Close()
	if ir.StatusCode != http.StatusOK || !out.Ok || out.Decision != peerlink.DecisionAccept {
		t.Fatalf("远程 invoke 失败: status=%d %+v", ir.StatusCode, out)
	}
	if !strings.Contains(string(out.Result), "read_file") {
		t.Fatalf("远程 invoke 结果未回传: %s", string(out.Result))
	}

	// ── 状态接口 + 停止 ──
	r6, _ := http.NewRequest("GET", srvB.URL+"/api/peerlink/edge/status", nil)
	r6.Header.Set("X-Peerlink-Operator", "1")
	st, _ := http.DefaultClient.Do(r6)
	var stt struct {
		Running   bool   `json:"running"`
		Connected bool   `json:"connected"`
		Hub       string `json:"hub"`
	}
	json.NewDecoder(st.Body).Decode(&stt)
	st.Body.Close()
	if !stt.Running || !stt.Connected {
		t.Fatalf("edge/status 应 running+connected: %+v", stt)
	}

	r7, _ := http.NewRequest("POST", srvB.URL+"/api/peerlink/edge/stop", nil)
	r7.Header.Set("X-Peerlink-Operator", "1")
	sp, _ := http.DefaultClient.Do(r7)
	sp.Body.Close()
	if sp.StatusCode != http.StatusOK {
		t.Fatalf("edge/stop 失败: %d", sp.StatusCode)
	}
	r8, _ := http.NewRequest("GET", srvB.URL+"/api/peerlink/edge/status", nil)
	r8.Header.Set("X-Peerlink-Operator", "1")
	st2, _ := http.DefaultClient.Do(r8)
	var stt2 struct {
		Running bool `json:"running"`
	}
	json.NewDecoder(st2.Body).Decode(&stt2)
	st2.Body.Close()
	if stt2.Running {
		t.Fatalf("stop 后应不再 running")
	}
}

// TestPeerlinkEdgeRuntime_RejectsPlainHTTPHub —— R3：跨端地址禁止明文 http
func TestPeerlinkEdgeRuntime_RejectsPlainHTTPHub(t *testing.T) {
	_, srvB := startEdgeServer(t, peerEdgeHandlers{})
	defer srvB.Close()

	body, _ := json.Marshal(map[string]string{
		"hub":       "http://192.168.1.50:2025",
		"pairingId": "x",
		"psk":       "y",
	})
	req, _ := http.NewRequest("POST", srvB.URL+"/api/peerlink/edge/pair", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Peerlink-Operator", "1")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("明文 http 的 Hub 应 400, got %d", resp.StatusCode)
	}
}

// TestPeerlinkEdgeRuntime_RequiresOperator —— 未带运维头不得操作 Edge
func TestPeerlinkEdgeRuntime_RequiresOperator(t *testing.T) {
	_, srvB := startEdgeServer(t, peerEdgeHandlers{})
	defer srvB.Close()

	resp, err := http.Get(srvB.URL + "/api/peerlink/edge/status")
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("edge/status 未带运维头应 401, got %d", resp.StatusCode)
	}
}
