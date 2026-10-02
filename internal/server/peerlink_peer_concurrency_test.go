package server

// peerlink_peer_concurrency_test.go —— R11：单 peer 并发上限（spec desktop-web-android-pairing / P2c）
//
// 验证两件事：
//  1. 同一 peer 上**在途 RPC** 有上限：超限请求立即 429 peer_busy（背压），
//     且成功数不得超过上限，之后的请求能恢复（槽位必须归还，不能泄漏）。
//  2. 一个 peer 只允许**一条活跃会话**：同一 token 二次建连会顶掉旧连接
//     （防同一个 token 开 N 条 WS 堆资源），且旧连接退出时不得误删新连接。

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Soltus/encv-go/internal/peerlink"
	"github.com/gorilla/websocket"
)

func TestPeerlinkSearch_PerPeerConcurrencyCapped_429(t *testing.T) {
	r, s := newPeerlinkRouter()
	srv := httptest.NewServer(r)
	defer srv.Close()

	// 对端故意慢（400ms），让并发真的叠起来
	const slow = 400 * time.Millisecond
	peerID, stop := startPairedEdge(t, r, s, srv.URL+"/api/peerlink", func(peerlink.SearchRequest) (json.RawMessage, error) {
		time.Sleep(slow)
		return json.Marshal([]any{})
	}, nil)
	defer stop()

	const N = 8
	codes := make(chan int, N)
	var wg sync.WaitGroup
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req, _ := http.NewRequest("GET", srv.URL+"/api/peerlink/search?peerId="+peerID+"&q=报告", nil)
			req.Header.Set("X-Peerlink-Operator", "1")
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				codes <- -1
				return
			}
			resp.Body.Close()
			codes <- resp.StatusCode
		}()
	}
	wg.Wait()
	close(codes)

	ok, busy := 0, 0
	for c := range codes {
		switch c {
		case http.StatusOK:
			ok++
		case http.StatusTooManyRequests:
			busy++
		}
	}
	if busy == 0 {
		t.Fatalf("并发上限未生效：%d 个并发请求没有一个被 429 拒绝（got ok=%d）", N, ok)
	}
	if ok > peerlink.MaxConcurrentCallsPerPeer {
		t.Fatalf("同时在途请求超过上限：ok=%d > %d", ok, peerlink.MaxConcurrentCallsPerPeer)
	}
	// 槽位必须归还：突发结束后在途数归零（泄漏会让后续请求永远 429）
	if n := s.peerCalls.InFlight(peerID); n != 0 {
		t.Fatalf("突发结束后在途数应归零（槽位泄漏）, got %d", n)
	}
	// 且限制解除后仍能正常查询（证明是限流不是熔断）
	req, _ := http.NewRequest("GET", srv.URL+"/api/peerlink/search?peerId="+peerID+"&q=报告", nil)
	req.Header.Set("X-Peerlink-Operator", "1")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("恢复后查询失败: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("并发退去后应恢复 200, got %d", resp.StatusCode)
	}
}

func TestPeerlinkWS_SingleSessionPerPeer(t *testing.T) {
	r, s := newPeerlinkRouter()
	srv := httptest.NewServer(r)
	defer srv.Close()

	tk, err := s.peerHub.CreateTicket("https://hub.test/api/peerlink", peerlink.DefaultTicketTTL)
	if err != nil {
		t.Fatalf("CreateTicket: %v", err)
	}
	psk, _ := peerlink.DecodePSK(tk.PSKHex)
	res, err := s.peerHub.Pair(tk.PairingID, "dev-r11-1", "Pixel", "android", peerlink.Proof(psk, tk.PairingID, "dev-r11-1"))
	if err != nil {
		t.Fatalf("Pair: %v", err)
	}

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/api/peerlink/ws?token=" + res.Token
	c1, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("第一条 WS 建连失败: %v", err)
	}
	defer c1.Close()
	var hello map[string]any
	if err := c1.ReadJSON(&hello); err != nil || hello["type"] != "hello_ok" {
		t.Fatalf("第一条连接应收到 hello_ok, got %+v err=%v", hello, err)
	}

	// 同一 token 二次建连：应顶掉第一条（一个 peer 只允许一条活跃会话）
	c2, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("第二条 WS 建连失败: %v", err)
	}
	defer c2.Close()
	if err := c2.ReadJSON(&hello); err != nil || hello["type"] != "hello_ok" {
		t.Fatalf("第二条连接应收到 hello_ok, got %+v err=%v", hello, err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, _, err := c1.ReadMessage(); err != nil {
			// 旧连接被服务端关闭
			goto closed
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("旧连接未被顶掉（同一 token 开了两条活跃会话）")
closed:

	// 旧连接退出（defer）后，连接表必须**仍保留**新连接 ——
	// 这正是 R11 引入的竞态点：若按 peerID 直接删，旧连接退出会把新连接一起删掉。
	time.Sleep(400 * time.Millisecond)
	if !s.peerConns.Has(res.PeerID) {
		t.Fatal("旧连接退出后连接表被清空（旧连接的 defer 误删了新连接）")
	}

	// 且表里那条是活的：第二条连接仍能正常 ping/pong
	if err := c2.WriteJSON(map[string]string{"type": "ping"}); err != nil {
		t.Fatalf("第二条连接写 ping 失败: %v", err)
	}
	_ = c2.SetReadDeadline(time.Now().Add(3 * time.Second))
	var pong map[string]any
	if err := c2.ReadJSON(&pong); err != nil {
		t.Fatalf("第二条连接读 pong 失败: %v", err)
	}
	if pong["type"] != "pong" {
		t.Fatalf("期望 pong, got %+v", pong)
	}
}
