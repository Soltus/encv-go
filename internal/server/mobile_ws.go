package server

import (
	"log/slog"
	"net/http"
)

// =============================================================================
// handleWebSocket — WebSocket HTTP 入口
// =============================================================================
//
// 2026-06-14 重构：与 ws_hub.go 的 StartReadPump 配对使用。
//
// 旧版（buggy）：在同一个 goroutine 里 for { conn.ReadMessage() }，收到 ping
// 文本消息时调 HandlePing 直接写 conn — 与 WritePump 并发写同一连接，
// 触发 gorilla 内部 mutex 关闭连接。
//
// 新版：把 ReadMessage 循环下沉到 hub 的 StartReadPump。StartWritePump 在独立
// goroutine 运行，两者通过 client.send 通道通信（单一写者模式）。
//
// =============================================================================

func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	wsHub := s.mobileSvc.GetWSHub()

	conn, err := wsHub.Upgrade(w, r)
	if err != nil {
		slog.Error("WebSocket upgrade failed", "error", err)
		return
	}

	// RegisterClient 设置 SetReadLimit/SetReadDeadline/SetPongHandler/SetPingHandler，
	// 并把初始 status 消息塞到 client.send 通道
	client := wsHub.RegisterClient(conn)

	// 启动写者 goroutine（**唯一**允许写 conn 的 goroutine）
	wsHub.StartWritePump(client)

	// 同步执行读循环，函数返回时自动 UnregisterClient
	wsHub.StartReadPump(client)
}

func (s *Server) BroadcastMessage(msgType string, data interface{}) {
	s.mobileSvc.GetWSHub().Broadcast(msgType, data)
}
