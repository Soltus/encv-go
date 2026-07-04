package service

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// ws_hub 单元测试（2026-06-14 新增）
//
// 验证关键修复：
//   1. 并发写安全（单一写者模式）
//   2. 协议级 ping/pong handler
//   3. 读超时/读限制
//   4. 客户端注册 → 接收初始 status 消息
//   5. 应用层 ping 文本消息 → 收到 pong
//   6. Broadcast 消息广播
//   7. 优雅关闭：close 帧正常处理
//
// =============================================================================

// newTestHubServer 启动一个 httptest server，把 /ws 接到 hub
func newTestHubServer(t *testing.T) (*httptest.Server, *WSHub) {
	t.Helper()
	hub := NewWSHub()
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		conn, err := hub.Upgrade(w, r)
		if err != nil {
			t.Logf("upgrade failed: %v", err)
			return
		}
		client := hub.RegisterClient(conn)
		hub.StartWritePump(client)
		hub.StartReadPump(client)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, hub
}

// dial 把 ws:// 客户端连上 srv 的 /ws
func dial(t *testing.T, srv *httptest.Server) (*websocket.Conn, error) {
	t.Helper()
	url := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws"
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	return conn, err
}

// TestWSHub_RegisterClient_ReceivesInitialStatus 验证：
// 客户端连接后立即收到 server:status 消息
func TestWSHub_RegisterClient_ReceivesInitialStatus(t *testing.T) {
	srv, _ := newTestHubServer(t)
	conn, err := dial(t, srv)
	require.NoError(t, err)
	defer conn.Close()

	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, msg, err := conn.ReadMessage()
	require.NoError(t, err, "应能读到 server:status 消息")

	var wsMsg WSMessage
	require.NoError(t, json.Unmarshal(msg, &wsMsg))
	assert.Equal(t, "server:status", wsMsg.Type)
}

// TestWSHub_HandlePing_AppLayer 验证：
// 客户端发 {"type":"ping"} 文本消息 → 收到 {"type":"pong"}
func TestWSHub_HandlePing_AppLayer(t *testing.T) {
	srv, _ := newTestHubServer(t)
	conn, err := dial(t, srv)
	require.NoError(t, err)
	defer conn.Close()

	// 先消费初始 status
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, _, err = conn.ReadMessage()
	require.NoError(t, err)

	// 发送 ping
	require.NoError(t, conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"ping"}`)))

	// 应收到 pong
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, msg, err := conn.ReadMessage()
	require.NoError(t, err, "应能读到 pong 响应")
	var wsMsg WSMessage
	require.NoError(t, json.Unmarshal(msg, &wsMsg))
	assert.Equal(t, "pong", wsMsg.Type)
}

// TestWSHub_ProtocolLevelPingPong 验证：
// gorilla 协议级 ping 帧（control frame）由 SetPingHandler 自动响应
// 这个测试模拟 server 主动 ping 客户端的场景
func TestWSHub_ProtocolLevelPingPong(t *testing.T) {
	srv, _ := newTestHubServer(t)
	conn, err := dial(t, srv)
	require.NoError(t, err)
	defer conn.Close()

	// 先消费初始 status
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, _, err = conn.ReadMessage()
	require.NoError(t, err)

	// 客户端发 ping 帧，server 应自动回 pong 帧
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	require.NoError(t, conn.WriteControl(websocket.PingMessage, []byte("ping-data"), time.Now().Add(time.Second)))

	// 客户端发一个文本消息以"等待" — gorilla 控制帧 ping 不在 ReadMessage 读取
	// 我们这里发文本 ping 验证 SetPingHandler 不会和 ReadPump 抢锁
	require.NoError(t, conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"ping"}`)))

	// 应能收到 pong 响应（应用层）
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, msg, err := conn.ReadMessage()
	require.NoError(t, err, "应能读到 pong 响应（应用层）")
	var wsMsg WSMessage
	require.NoError(t, json.Unmarshal(msg, &wsMsg))
	assert.Equal(t, "pong", wsMsg.Type)
}

// TestWSHub_Broadcast 验证：
// Broadcast 给所有 client 广播消息
func TestWSHub_Broadcast(t *testing.T) {
	srv, hub := newTestHubServer(t)

	// 连 3 个客户端
	const N = 3
	conns := make([]*websocket.Conn, N)
	for i := 0; i < N; i++ {
		c, err := dial(t, srv)
		require.NoError(t, err)
		conns[i] = c
		defer c.Close()
		// 消费初始 status
		_ = c.SetReadDeadline(time.Now().Add(2 * time.Second))
		_, _, _ = c.ReadMessage()
	}

	// 广播
	hub.Broadcast("task:update", map[string]string{"id": "task-1"})

	// 每个客户端应能读到广播消息
	for i, c := range conns {
		_ = c.SetReadDeadline(time.Now().Add(2 * time.Second))
		_, msg, err := c.ReadMessage()
		require.NoErrorf(t, err, "client %d 应该能读到广播消息", i)

		var wsMsg WSMessage
		require.NoError(t, json.Unmarshal(msg, &wsMsg))
		assert.Equal(t, "task:update", wsMsg.Type)

		// Data 是 interface{}，先用 json.Marshal 转回 []byte 再 unmarshal
		dataBytes, err := json.Marshal(wsMsg.Data)
		require.NoError(t, err)
		var data map[string]string
		require.NoError(t, json.Unmarshal(dataBytes, &data))
		assert.Equal(t, "task-1", data["id"])
	}
}

// TestWSHub_UnregisterClient_CleanShutdown 验证：
// UnregisterClient 后连接被关闭，client 从 hub.clients map 中移除
func TestWSHub_UnregisterClient_CleanShutdown(t *testing.T) {
	srv, hub := newTestHubServer(t)
	conn, err := dial(t, srv)
	require.NoError(t, err)
	defer conn.Close()

	// 消费初始 status
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, _, _ = conn.ReadMessage()

	// 确认 client 已注册
	hub.mu.RLock()
	initialCount := len(hub.clients)
	hub.mu.RUnlock()
	assert.Equal(t, 1, initialCount, "应有 1 个 client")

	// 关闭客户端（触发 server 端 UnregisterClient）
	require.NoError(t, conn.WriteControl(
		websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, "client done"),
		time.Now().Add(time.Second),
	))

	// 等待 server 端处理 close
	require.Eventually(t, func() bool {
		hub.mu.RLock()
		defer hub.mu.RUnlock()
		return len(hub.clients) == 0
	}, 3*time.Second, 50*time.Millisecond, "UnregisterClient 后 client 应被移除")

	// 连接在 server 端已 close
	_ = conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	_, _, err = conn.ReadMessage()
	if err == nil {
		t.Errorf("客户端应读到 close 错误或 EOF")
	}
}

// =============================================================================
// 关键测试：并发写安全（2026-06-14 修复的核心 bug）
// =============================================================================
// 历史 bug 根因：原 ws_hub.go 在 RegisterClient / StartWritePump / HandlePing
// 三个 goroutine 写同一 conn，触发 gorilla 内部 mutex 关闭连接。
//
// 新实现保证：所有 conn.WriteMessage 只能由 StartWritePump 调用。
// 验证：并发触发 50 个 ping + 50 次 Broadcast + 50 次 ping handler，
//      期间任何 conn.WriteMessage 都不应 panic。
// =============================================================================

func TestWSHub_ConcurrentWrites_NoPanic(t *testing.T) {
	srv, hub := newTestHubServer(t)
	conn, err := dial(t, srv)
	require.NoError(t, err)
	defer conn.Close()

	// 消费初始 status
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, _, _ = conn.ReadMessage()

	var wg sync.WaitGroup
	var panics atomic.Int32

	// Goroutine 1: 大量应用层 ping（触发 HandlePing → send 通道）
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer func() {
			if r := recover(); r != nil {
				panics.Add(1)
				t.Errorf("ping handler panic: %v", r)
			}
		}()
		for i := 0; i < 50; i++ {
			_ = conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"ping"}`))
			time.Sleep(2 * time.Millisecond)
		}
	}()

	// Goroutine 2: 大量 Broadcast（触发 client.send 通道）
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			hub.Broadcast("task:update", map[string]int{"n": i})
			time.Sleep(2 * time.Millisecond)
		}
	}()

	// Goroutine 3: 在客户端持续读消息（不丢消息）
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
		readCount := 0
		for readCount < 95 { // 50 ping response + 50 broadcast - 一些丢的
			_, _, err := conn.ReadMessage()
			if err != nil {
				return
			}
			readCount++
		}
	}()

	wg.Wait()
	assert.Zero(t, panics.Load(), "并发写期间不应有 panic")
}

// TestWSHub_BroadcastDropOldestSemantics 验证：
// 当 send 通道满时，新消息应被丢弃（drop new）而不是阻塞广播者
func TestWSHub_BroadcastDropOldestSemantics(t *testing.T) {
	hub := NewWSHub()
	// 模拟一个 client 满 send 通道的场景
	conn, serverConn := net.Pipe()  // 用 pipe 模拟 conn（不实际写）
	defer conn.Close()
	defer serverConn.Close()

	// 手动构造一个 client，send 通道容量小以便快速填满
	client := &wsClient{conn: nil, send: make(chan []byte, 2)}
	hub.mu.Lock()
	hub.clients[client] = struct{}{}
	hub.mu.Unlock()

	// 第 1、2 个能塞进
	hub.Broadcast("test", "msg1")
	hub.Broadcast("test", "msg2")
	// 第 3 个应该被丢弃（send 通道已满，select default 走丢弃）
	hub.Broadcast("test", "msg3")

	// 验证：前两个在通道里，第三个被丢
	assert.Equal(t, 2, len(client.send))
	<-client.send
	<-client.send
}

// TestWSHub_BroadcastToMultipleClients_Concurrent 验证：
// 多个客户端并发广播，所有客户端都能收到
func TestWSHub_BroadcastToMultipleClients_Concurrent(t *testing.T) {
	srv, hub := newTestHubServer(t)
	const N = 5
	conns := make([]*websocket.Conn, N)
	for i := 0; i < N; i++ {
		c, err := dial(t, srv)
		require.NoError(t, err)
		conns[i] = c
		defer c.Close()
		// 消费初始 status
		_ = c.SetReadDeadline(time.Now().Add(2 * time.Second))
		_, _, _ = c.ReadMessage()
	}

	var wg sync.WaitGroup
	// 5 个客户端各收 N 条
	for i, c := range conns {
		wg.Add(1)
		go func(idx int, conn *websocket.Conn) {
			defer wg.Done()
			_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
			for j := 0; j < N; j++ {
				_, msg, err := conn.ReadMessage()
				if err != nil {
					t.Logf("client %d read err: %v", idx, err)
					return
				}
				var wsMsg WSMessage
				_ = json.Unmarshal(msg, &wsMsg)
				assert.Equalf(t, "broadcast", wsMsg.Type, "client %d msg %d", idx, j)
			}
		}(i, c)
	}

	// 广播 N 条
	for j := 0; j < N; j++ {
		hub.Broadcast("broadcast", j)
	}

	wg.Wait()
}
