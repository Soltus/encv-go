package server

// peerlink_search_test.go —— P3 联邦搜索端到端（spec desktop-web-android-pairing）
//
// 验证：
//  1. Hub 能通过 Edge 的**主动出网连接**反向查询对端索引，结果带 peer 标注
//  2. 对端离线 → 503 peer_offline（前端降级为"该端无结果"，不阻塞本端搜索）
//  3. 未知 peer → 404；缺参 → 400
//  4. 明确**不是**挂载：远端结果原样带回，不改写成本地路径语义

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Soltus/encv-go/internal/peerlink"
	"github.com/gin-gonic/gin"
)

// startPairedEdge 在**给定的同一个 Hub** 上建立"已配对 + Edge 已连上"的场景。
//
// ⚠️ ticket/pair 必须走与对外服务**同一个** Hub 实例：否则对端身份只存在于另一个
//
//	Hub 的内存里，查 peer 必然 404（2026-10-02 我自己写错过一次）。
func startPairedEdge(
	t *testing.T,
	r *gin.Engine,
	s *Server,
	hubURL string,
	onSearch func(peerlink.SearchRequest) (json.RawMessage, error),
	onRead func(peerlink.ReadRequest) ([]byte, error),
) (string, func()) {
	return startPairedEdgeFull(t, r, s, hubURL, onSearch, onRead, nil)
}

// startPairedEdgeFull 同上，额外支持注入 P4 的远程 Agent 处理器（执行端）。
func startPairedEdgeFull(
	t *testing.T,
	r *gin.Engine,
	s *Server,
	hubURL string,
	onSearch func(peerlink.SearchRequest) (json.RawMessage, error),
	onRead func(peerlink.ReadRequest) ([]byte, error),
	onAgent func(peerlink.AgentInvokeRequest) (json.RawMessage, error),
) (string, func()) {
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
		"deviceId":  "search-device-0001",
		"name":      "Pixel",
		"platform":  "android",
		"proof":     peerlink.Proof(psk, tk.PairingID, "search-device-0001"),
	}))))
	var pr struct {
		PeerID string `json:"peerId"`
		Token  string `json:"token"`
	}
	json.Unmarshal(rec.Body.Bytes(), &pr)

	edge := peerlink.NewEdge(peerlink.EdgeOptions{
		HubURL:              hubURL,
		Token:               pr.Token,
		DeviceID:            "search-device-0001",
		HeartbeatForeground: 80 * time.Millisecond,
		MinBackoff:          10 * time.Millisecond,
		MaxBackoff:          50 * time.Millisecond,
		OnSearch:            onSearch,
		OnRead:              onRead,
		OnAgentInvoke:       onAgent,
	})
	ctx, cancel := context.WithCancel(context.Background())
	go edge.Start(ctx)

	// 等连接真正建立（Hub 侧登记 conn）
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if _, ok := s.peerConns.Get(pr.PeerID); ok {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	return pr.PeerID, func() {
		edge.Close()
		cancel()
	}
}

func TestPeerlinkSearch_Federated_OK(t *testing.T) {
	// Hub 侧服务（本机运维身份调用）
	r, s := newPeerlinkRouter()
	srv := httptest.NewServer(r)
	defer srv.Close()

	peerID, stop := startPairedEdge(t, r, s, srv.URL+"/api/peerlink", func(req peerlink.SearchRequest) (json.RawMessage, error) {
		if req.Q == "" {
			return nil, peerlink.ErrBadProof // 借用 sentinel 表达"参数不合法"
		}
		// 对端本地索引的返回结果（远端路径原样返回，不伪装成本地路径）
		return json.Marshal([]map[string]any{
			{"path": "/sdcard/Download/报告.pdf", "name": "报告.pdf", "size": 1024},
		})
	}, nil)
	defer stop()

	req, _ := http.NewRequest("GET", srv.URL+"/api/peerlink/search?peerId="+peerID+"&q=报告&kind=fulltext&limit=20", nil)
	req.Header.Set("X-Peerlink-Operator", "1")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("search 请求失败: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("search 期望 200, got %d", resp.StatusCode)
	}
	var out struct {
		Peer struct {
			ID       string `json:"id"`
			Name     string `json:"name"`
			Platform string `json:"platform"`
		} `json:"peer"`
		Items []map[string]any `json:"items"`
	}
	json.NewDecoder(resp.Body).Decode(&out)
	if out.Peer.Platform != "android" {
		t.Fatalf("结果应带 peer 标注(platform=android), got %+v", out.Peer)
	}
	if len(out.Items) != 1 {
		t.Fatalf("应返回 1 条远端结果, got %d", len(out.Items))
	}
	if p, _ := out.Items[0]["path"].(string); p != "/sdcard/Download/报告.pdf" {
		t.Fatalf("远端路径应原样返回（不伪装成本地路径）, got %q", p)
	}
	_ = s
}

func TestPeerlinkSearch_PeerOffline_503(t *testing.T) {
	r, s := newPeerlinkRouter()
	srv := httptest.NewServer(r)
	defer srv.Close()

	peerID, stop := startPairedEdge(t, r, s, srv.URL+"/api/peerlink", func(peerlink.SearchRequest) (json.RawMessage, error) {
		return json.Marshal([]any{})
	}, nil)
	// 立刻断开，模拟息屏/断网
	stop()
	time.Sleep(200 * time.Millisecond)

	req, _ := http.NewRequest("GET", srv.URL+"/api/peerlink/search?peerId="+peerID+"&q=报告", nil)
	req.Header.Set("X-Peerlink-Operator", "1")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("search 请求失败: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("对端离线应 503, got %d", resp.StatusCode)
	}
}

func TestPeerlinkSearch_UnknownPeer_404_And_MissingParams_400(t *testing.T) {
	r, _ := newPeerlinkRouter()
	srv := httptest.NewServer(r)
	defer srv.Close()

	req, _ := http.NewRequest("GET", srv.URL+"/api/peerlink/search?peerId=nope&q=x", nil)
	req.Header.Set("X-Peerlink-Operator", "1")
	resp, _ := http.DefaultClient.Do(req)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("未知 peer 应 404, got %d", resp.StatusCode)
	}

	req2, _ := http.NewRequest("GET", srv.URL+"/api/peerlink/search?q=x", nil)
	req2.Header.Set("X-Peerlink-Operator", "1")
	resp2, _ := http.DefaultClient.Do(req2)
	resp2.Body.Close()
	if resp2.StatusCode != http.StatusBadRequest {
		t.Fatalf("缺 peerId 应 400, got %d", resp2.StatusCode)
	}
}
