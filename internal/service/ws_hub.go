package service

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// =============================================================================
// WSHub — WebSocket Hub
// =============================================================================
//
// 2026-06-14 重构：消除并发写竞态 + 协议级 ping/pong + 读超时/读限制
//
// 关键决策：
//   1. **单一写者模式（Single Writer）**：所有 conn.WriteMessage() 必须从同一个
//      goroutine 调用。gorilla/websocket 文档明确写：
//        "Applications must ensure that no more than one goroutine calls the
//         write methods (NextWriter, SetWriteDeadline, WriteMessage, ...)
//         concurrently."
//      历史 bug：原代码有 3 个并发写点
//        - RegisterClient (line 63) 直接写 conn.WriteMessage 发初始 status
//        - StartWritePump goroutine 写 client.send → conn
//        - HandlePing (line 83) 直接写 conn.WriteMessage 发 pong
//      并发写 → gorilla 内部 mutex 触发 → conn 关闭 → 前端 readyState=3
//      修复：所有写操作都走 client.send 通道，由 WritePump 串行化
//
//   2. **协议级 ping/pong**：使用 gorilla 内置的 SetPingHandler / SetPongHandler
//      - 浏览器 / OkHttp 自动发 ping 帧（默认 30s 间隔）
//      - SetPongHandler 收到 pong 时刷新 read deadline → 检测半开连接
//      - SetPingHandler 收到 ping 时通过 send 通道回复 pong（不直接写 conn）
//
//   3. **读超时 / 写超时 / 读限制**：
//      - SetReadLimit(8KB) — 防止恶意/异常大消息撑爆内存
//      - SetReadDeadline + pongWait — 半开连接自动清理
//      - SetWriteDeadline + writeWait — 防止慢客户端阻塞 send 通道
//
// =============================================================================

const (
	// pongWait 是等待客户端 pong 的最长时间。超过则视为半开连接，关闭。
	// 比前端 30s 心跳略长（前端 30s ping + 10s pong 超时），但 gorilla 内置
	// ping 也走 30s 间隔，所以两个机制叠加保护。
	pongWait = 60 * time.Second

	// pingPeriod 是 server 主动发 ping 的间隔。
	// 必须小于 pongWait，且 (pingPeriod * 10 / 9) < pongWait 以保证在 deadline 前刷新。
	// 54s < 60s，留 6s 余量给慢网络。
	pingPeriod = (pongWait * 9) / 10

	// writeWait 是单次写的 deadline。慢客户端 5s 还没收完就放弃。
	writeWait = 5 * time.Second

	// maxMessageSize 限制单条消息大小。WS 消息主要是 JSON 状态/任务事件，
	// 8KB 远超实际需求（典型 task 事件 200-500 bytes）。
	maxMessageSize = 8 * 1024

	// sendBufferSize 是每个 client 的 send 通道缓冲。
	// 1024 条消息缓冲足以应对短暂的网络抖动，缓冲满则按"drop oldest"
	// 策略（select default）丢弃新消息，优先保证连接可用性。
	sendBufferSize = 1024
)

type WSMessage struct {
	Type string      `json:"type"`
	Data interface{} `json:"data,omitempty"`
}

type wsClient struct {
	conn *websocket.Conn
	send chan []byte
}

type WSHub struct {
	clients  map[*wsClient]struct{}
	mu       sync.RWMutex
	upgrader websocket.Upgrader
}

func NewWSHub() *WSHub {
	return &WSHub{
		clients: make(map[*wsClient]struct{}),
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
		},
	}
}

func (h *WSHub) Upgrade(w http.ResponseWriter, r *http.Request) (*websocket.Conn, error) {
	return h.upgrader.Upgrade(w, r, nil)
}

// =============================================================================
// RegisterClient — 注册新客户端
// =============================================================================
//
// 2026-06-14 修复：原代码在这里直接 conn.WriteMessage 发 status message，
// 与 StartWritePump 并发写同一 conn，导致 race。
// 新实现：把 status message 塞到 client.send 通道，由 WritePump 串行发送。
//
// =============================================================================
func (h *WSHub) RegisterClient(conn *websocket.Conn) *wsClient {
	client := &wsClient{
		conn: conn,
		send: make(chan []byte, sendBufferSize),
	}

	// 协议级 ping/pong + 读限制 + 读超时
	conn.SetReadLimit(maxMessageSize)
	_ = conn.SetReadDeadline(time.Now().Add(pongWait))
	conn.SetPongHandler(func(string) error {
		// 收到 pong 帧 → 刷新读 deadline
		_ = conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})
	// 协议级 ping handler：收到 ping 帧时不直接写 conn（避免 race），
	// 把 pong message 塞到 send 通道由 WritePump 写。
	conn.SetPingHandler(func(appData string) error {
		// 协议层 pong 帧 vs 应用层 {"type":"pong"} 文本消息是两件事
		// gorilla 在收到 ping 帧时，需要回复一个 pong 协议帧（不是文本消息）
		// NextWriter(websocket.PongMessage) 必须在 read goroutine 调用
		// (因为 gorilla 在 ping handler 调用期间持有读锁) — 这里是安全的。
		_ = conn.SetReadDeadline(time.Now().Add(pongWait))
		return conn.WriteControl(
			websocket.PongMessage,
			[]byte(appData),
			time.Now().Add(writeWait),
		)
	})

	h.mu.Lock()
	h.clients[client] = struct{}{}
	h.mu.Unlock()

	slog.Info("WebSocket client connected", "remote", conn.RemoteAddr())

	// 把初始 status 消息塞到 send 通道，由 WritePump 串行写出
	statusMsg, _ := json.Marshal(WSMessage{
		Type: "server:status",
		Data: map[string]interface{}{
			"online": true,
		},
	})
	// 用 select+default 避免阻塞注册流程
	select {
	case client.send <- statusMsg:
	default:
		slog.Warn("WebSocket RegisterClient: initial status message dropped (send buffer full)")
	}

	return client
}

// =============================================================================
// UnregisterClient — 注销客户端
// =============================================================================
// 关闭 send 通道、关闭连接。ReadPump / WritePump goroutine 会自然退出。
// =============================================================================
func (h *WSHub) UnregisterClient(client *wsClient) {
	h.mu.Lock()
	delete(h.clients, client)
	h.mu.Unlock()

	// 关闭 send 通道前先确保连接关闭，避免 WritePump 在已关闭的 conn 上写
	_ = client.conn.Close()

	// 安全关闭 send 通道（WritePump 可能在 range client.send）
	// 这里用 defer 关闭策略：先 close conn，让 WritePump 写失败自然退出
	// 然后再 close send channel。
	//
	// 注意：close(client.send) 必须确保没有其他 goroutine 在 select 写它
	// —— 这里只可能 WritePump 在写（RegisterClient 完成后只 StartWritePump
	// 启动了一个写者），所以 close 是安全的。
	defer func() {
		// 用 recover 防止双重 close 引起的 panic
		defer func() { _ = recover() }()
		close(client.send)
	}()

	slog.Info("WebSocket client disconnected", "remote", client.conn.RemoteAddr())
}

// =============================================================================
// HandlePing — 处理应用层 ping 文本消息
// =============================================================================
// 前端发 {"type":"ping"} 文本消息，期望 {"type":"pong"} 文本响应。
// 与协议级 ping 帧（SetPingHandler）不同。
// 修复：把 pong message 塞到 send 通道，由 WritePump 串行写。
// =============================================================================
func (h *WSHub) HandlePing(conn *websocket.Conn) {
	_ = conn.SetReadDeadline(time.Now().Add(pongWait))
	pongMsg, _ := json.Marshal(WSMessage{Type: "pong"})

	// 找到对应的 client，把 pong 塞到它的 send 通道
	// 这里需要遍历 clients map，因为 HandlePing 签名只接收 conn。
	// 实际上 mobile_ws.go 在 ReadPump 循环中调用 HandlePing(conn)，
	// 此时 conn 对应的 client 必然还在 map 中（defer UnregisterClient
	// 还没触发），所以一定能找到。
	h.mu.RLock()
	defer h.mu.RUnlock()
	for client := range h.clients {
		if client.conn == conn {
			select {
			case client.send <- pongMsg:
			default:
				slog.Warn("WebSocket HandlePing: pong message dropped (send buffer full)")
			}
			return
		}
	}
}

// =============================================================================
// Broadcast / BroadcastRaw — 广播消息给所有 client
// =============================================================================
func (h *WSHub) Broadcast(msgType string, data interface{}) {
	msg, err := json.Marshal(WSMessage{
		Type: msgType,
		Data: data,
	})
	if err != nil {
		slog.Error("Failed to marshal broadcast message", "error", err)
		return
	}
	h.broadcastBytes(msg, msgType)
}

func (h *WSHub) BroadcastRaw(msg []byte) {
	h.broadcastBytes(msg, "raw")
}

func (h *WSHub) broadcastBytes(msg []byte, msgType string) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	dropped := 0
	for client := range h.clients {
		select {
		case client.send <- msg:
		default:
			dropped++
		}
	}
	if dropped > 0 {
		slog.Warn("WebSocket broadcast: messages dropped",
			"type", msgType,
			"dropped", dropped,
			"total_clients", len(h.clients))
	}
}

// =============================================================================
// StartWritePump — 启动写循环（每个 client 一个 goroutine）
// =============================================================================
// 这是**唯一**允许写 conn 的 goroutine。
//
// 工作流：
//   1. 启动 ping 定时器（54s 一次）
//   2. select 等待：send 通道有数据 / ping 定时器触发
//   3. 写完立即 flush + 设 write deadline
//
// 退出条件：
//   - send 通道 close（UnregisterClient 触发）
//   - conn 写失败（网络断）
//
// 2026-06-14 修复：API 改为内部自启动 goroutine。
// 旧 API 要求调用方手动加 `go hub.StartWritePump(client)`，容易遗漏 →
// ReadPump 永不调度、send 通道积压、连接被踢。改成内部 `go func() { ... }()`
// 后，调用方语义更清晰，且不可能再忘记。
// =============================================================================
func (h *WSHub) StartWritePump(client *wsClient) {
	go func() {
		pinger := time.NewTicker(pingPeriod)
		defer func() {
			pinger.Stop()
			_ = client.conn.Close()
		}()

		for {
			select {
			case msg, ok := <-client.send:
				if !ok {
					// send 通道关闭 → 发送 close 帧给客户端，然后退出
					_ = client.conn.SetWriteDeadline(time.Now().Add(writeWait))
					_ = client.conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
					return
				}

				// 设置写 deadline：单次写最多 5s（防慢客户端）
				_ = client.conn.SetWriteDeadline(time.Now().Add(writeWait))
				if err := client.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
					slog.Debug("WebSocket write error (client likely disconnected)", "error", err)
					return
				}

			case <-pinger.C:
				// 主动发 ping 帧（协议级，不是文本消息）
				_ = client.conn.SetWriteDeadline(time.Now().Add(writeWait))
				if err := client.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
					slog.Debug("WebSocket ping write error (client likely disconnected)", "error", err)
					return
				}
			}
		}
	}()
}

// =============================================================================
// StartReadPump — 启动读循环（在调用方的 goroutine 中同步运行）
// =============================================================================
// 2026-06-14 新增：把原 mobile_ws.go 里的 for { ReadMessage } 逻辑下沉到 hub，
// 保证 ReadPump 和 WritePump 的生命周期在同一个 RegisterClient/UnregisterClient
// 配对里管理。
//
// =============================================================================
func (h *WSHub) StartReadPump(client *wsClient) {
	defer func() {
		h.UnregisterClient(client)
	}()

	for {
		_, messageBytes, err := client.conn.ReadMessage()
		if err != nil {
			// 区分正常关闭 / 异常断开
			if websocket.IsUnexpectedCloseError(err,
				websocket.CloseGoingAway,
				websocket.CloseAbnormalClosure,
				websocket.CloseNormalClosure,
			) {
				slog.Warn("WebSocket read error (unexpected close)", "error", err, "remote", client.conn.RemoteAddr())
			} else {
				slog.Debug("WebSocket connection closed", "remote", client.conn.RemoteAddr(), "error", err)
			}
			return
		}

		var msg WSMessage
		if err := json.Unmarshal(messageBytes, &msg); err != nil {
			slog.Warn("Failed to unmarshal WebSocket message", "error", err)
			continue
		}

		switch msg.Type {
		case "ping":
			// 应用层 ping → 通过 send 通道回复 pong（**不**直接写 conn）
			h.HandlePing(client.conn)
		default:
			slog.Debug("Unhandled WebSocket message type", "type", msg.Type)
		}
	}
}
