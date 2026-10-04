package peerlink

// edge_test.go —— Edge 客户端（长连接放 Go 侧，E2 决策）契约测试
//
// 覆盖：建连收到 hello_ok、心跳按间隔发送、前台↔后台间隔切换、
//       断线后指数退避重连、SendJSON、Close 停止心跳。

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// hubStub 模拟 Hub 的 WS 端点：连接即发 hello_ok，收到 ping 回 pong。
type hubStub struct {
	srv       *httptest.Server
	mu        sync.Mutex
	conns     int32
	pings     int32
	dropAfter int32 // 收到第 N 个 ping 后主动断开（测重连）
}

func newHubStub(t *testing.T, dropAfter int32) *hubStub {
	t.Helper()
	h := &hubStub{dropAfter: dropAfter}
	up := websocket.Upgrader{}
	h.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("token") == "" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		conn, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		atomic.AddInt32(&h.conns, 1)
		defer conn.Close()
		_ = conn.WriteJSON(map[string]any{"type": "hello_ok", "peerId": "peer-stub"})
		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var f struct {
				Type string `json:"type"`
			}
			if err := json.Unmarshal(msg, &f); err != nil {
				continue
			}
			if f.Type == "ping" {
				n := atomic.AddInt32(&h.pings, 1)
				_ = conn.WriteJSON(map[string]any{"type": "pong", "at": time.Now().UnixMilli()})
				if h.dropAfter > 0 && n >= h.dropAfter {
					return // 主动断开，触发客户端重连
				}
			}
		}
	}))
	t.Cleanup(h.srv.Close)
	return h
}

func TestEdge_Connect_Heartbeat_AndFrame(t *testing.T) {
	stub := newHubStub(t, 0)

	var gotHello int32
	e := NewEdge(EdgeOptions{
		HubURL:              strings.Replace(stub.srv.URL, "http", "http", 1) + "/api/peerlink",
		Token:               "tok-1",
		DeviceID:            "dev-1",
		HeartbeatForeground: 40 * time.Millisecond,
		HeartbeatBackground: 400 * time.Millisecond,
		MinBackoff:          10 * time.Millisecond,
		MaxBackoff:          50 * time.Millisecond,
		OnFrame: func(typ string, _ []byte) {
			if typ == "hello_ok" {
				atomic.AddInt32(&gotHello, 1)
			}
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		e.Start(ctx)
		close(done)
	}()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if atomic.LoadInt32(&gotHello) > 0 && atomic.LoadInt32(&stub.pings) >= 2 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if atomic.LoadInt32(&gotHello) == 0 {
		t.Fatal("应收到服务端 hello_ok")
	}
	if atomic.LoadInt32(&stub.pings) < 2 {
		t.Fatalf("心跳应持续发送, got %d", atomic.LoadInt32(&stub.pings))
	}
	if !e.Online() {
		t.Fatal("Online() 应为 true")
	}

	// SendJSON 能送达（stub 不回特定帧，只验证不报错）
	if err := e.SendJSON(map[string]string{"type": "note"}); err != nil {
		t.Fatalf("SendJSON: %v", err)
	}

	e.Close()
	cancel()
	<-done
}

func TestEdge_Reconnect_AfterDrop(t *testing.T) {
	// 第 1 个 ping 后服务端断开 → 客户端应退避重连
	stub := newHubStub(t, 1)

	e := NewEdge(EdgeOptions{
		HubURL:              stub.srv.URL + "/api/peerlink",
		Token:               "tok-2",
		HeartbeatForeground: 30 * time.Millisecond,
		MinBackoff:          10 * time.Millisecond,
		MaxBackoff:          40 * time.Millisecond,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	go e.Start(ctx)

	deadline := time.Now().Add(3500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if atomic.LoadInt32(&stub.conns) >= 2 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if atomic.LoadInt32(&stub.conns) < 2 {
		t.Fatalf("断线后应重连（连接数 ≥2）, got %d", atomic.LoadInt32(&stub.conns))
	}
	e.Close()
}

func TestEdge_Background_ReducesHeartbeat(t *testing.T) {
	stub := newHubStub(t, 0)

	e := NewEdge(EdgeOptions{
		HubURL:              stub.srv.URL + "/api/peerlink",
		Token:               "tok-3",
		HeartbeatForeground: 40 * time.Millisecond,
		HeartbeatBackground: 800 * time.Millisecond,
		MinBackoff:          10 * time.Millisecond,
		MaxBackoff:          40 * time.Millisecond,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go e.Start(ctx)

	// 前台跑 300ms，统计 ping 数
	time.Sleep(300 * time.Millisecond)
	e.SetBackground(true)
	fg := atomic.LoadInt32(&stub.pings)
	if fg < 2 {
		t.Fatalf("前台心跳应更频繁, got %d", fg)
	}

	// 后台再跑 600ms，增量应明显更少
	time.Sleep(600 * time.Millisecond)
	bg := atomic.LoadInt32(&stub.pings) - fg
	if bg > fg {
		t.Fatalf("后台心跳频率应低于前台: 前台 %d, 后台增量 %d", fg, bg)
	}
	e.Close()
}

func TestEdge_Close_StopsHeartbeat(t *testing.T) {
	stub := newHubStub(t, 0)
	e := NewEdge(EdgeOptions{
		HubURL:              stub.srv.URL + "/api/peerlink",
		Token:               "tok-4",
		HeartbeatForeground: 30 * time.Millisecond,
		MinBackoff:          10 * time.Millisecond,
		MaxBackoff:          30 * time.Millisecond,
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go e.Start(ctx)

	time.Sleep(200 * time.Millisecond)
	e.Close()
	time.Sleep(200 * time.Millisecond)
	before := atomic.LoadInt32(&stub.pings)
	time.Sleep(300 * time.Millisecond)
	if after := atomic.LoadInt32(&stub.pings); after != before {
		t.Fatalf("Close 后不应再发心跳: %d → %d", before, after)
	}
	if e.Online() {
		t.Fatal("Close 后 Online() 应为 false")
	}
}

func TestEdge_WSURL_SchemeConversion(t *testing.T) {
	e := NewEdge(EdgeOptions{HubURL: "https://hub.example/api/peerlink", Token: "T"})
	if got := e.wsURL(); got != "wss://hub.example/api/peerlink/ws?token=T" {
		t.Fatalf("https → wss 转换错误: %s", got)
	}
	e2 := NewEdge(EdgeOptions{HubURL: "http://127.0.0.1:2025/api/peerlink", Token: "T"})
	if got := e2.wsURL(); got != "ws://127.0.0.1:2025/api/peerlink/ws?token=T" {
		t.Fatalf("http → ws 转换错误: %s", got)
	}
}
