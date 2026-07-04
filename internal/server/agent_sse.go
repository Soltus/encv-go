package server

// agent_sse.go — 拆分自 agent_api.go

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

func (s *Server) setSSEHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	// 注释：初始 SSE comment 用于确认连接建立
	// 如果写入失败（client 已断开），调用方 Safe 函数会检测到
	_, _ = w.Write([]byte(": agent ok\n\n"))
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}

func (s *Server) sendSSEEvent(w http.ResponseWriter, eventType string, data interface{}) {
	raw, _ := json.Marshal(data)
	fmt.Fprintf(w, "data: {\"type\": \"%s\", \"data\": %s}\n\n", eventType, raw)
	w.(http.Flusher).Flush()
}

func (s *Server) sendSSEEventSafe(w http.ResponseWriter, flusher http.Flusher, eventType string, data interface{}) {
	raw, _ := json.Marshal(data) // json.Marshal 不追加 \n（与 json.Encoder.Encode 不同！）
	n, err := fmt.Fprintf(w, "data: {\"type\": \"%s\", \"data\": %s}\n\n", eventType, raw)
	if err != nil || n < 0 {
		slog.Warn("agent: sse write failed (client disconnected?)", "error", err)
		return
	}
	flusher.Flush()
}

func (s *Server) sendAndCache(sess *agentSession, w http.ResponseWriter, flusher http.Flusher, eventType string, data interface{}) {
	if sess != nil {
		sess.mu.Lock()
		sess.eventIDCounter++
		eventID := sess.eventIDCounter
		sess.EventCache = append(sess.EventCache, AgentEvent{ID: eventID, Type: eventType, Data: data})
		sess.mu.Unlock()
	}
	s.sendSSEEventSafe(w, flusher, eventType, data)
}

func (s *Server) streamText(w http.ResponseWriter, text string, chunkSize int, delayMs time.Duration) {
	runes := []rune(text)
	seq := 0
	for i := 0; i < len(runes); i += chunkSize {
		end := i + chunkSize
		if end > len(runes) {
			end = len(runes)
		}
		seq++
		s.sendSSEEvent(w, "text_delta", map[string]interface{}{"seq": seq, "text": string(runes[i:end])})
		time.Sleep(delayMs)
	}
}

func (s *Server) streamTextSafe(w http.ResponseWriter, flusher http.Flusher, text string, chunkSize int, delayMs time.Duration) {
	runes := []rune(text)
	seq := 0
	for i := 0; i < len(runes); i += chunkSize {
		end := i + chunkSize
		if end > len(runes) {
			end = len(runes)
		}
		seq++
		s.sendSSEEventSafe(w, flusher, "text_delta", map[string]interface{}{"seq": seq, "text": string(runes[i:end])})
		time.Sleep(delayMs)
	}
}
