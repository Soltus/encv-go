// internal/logger/ringbuffer.go
// 🆕 2026-06-16: 后端日志环形缓冲区，供 GET /api/logs/recent 端点暴露给 HTTP-poll 客户端
//   原因：WS 失败降级 http-poll 时，前端 devlogs 收不到 WS 推送的 slog
//   修复：WSLogHandler 同时写入 ring buffer → HTTP poll 定期 GET → 前端 emit('log') 事件
package logger

import (
	"container/ring"
	"sync"
)

// RingBuffer 最近 N 条结构化日志，供 DevTools "后端日志" tab 在 http-poll 降级时拉取
type RingBuffer struct {
	mu    sync.RWMutex
	r     *ring.Ring
}

// NewRingBuffer 创建容量 cap 的环形 buffer
func NewRingBuffer(capacity int) *RingBuffer {
	if capacity <= 0 {
		capacity = 500
	}
	return &RingBuffer{r: ring.New(capacity)}
}

// Push 写入一条日志
func (b *RingBuffer) Push(entry map[string]string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.r.Value = entry
	b.r = b.r.Next()
}

// Snapshot 返回最近的所有日志（按时间正序）
func (b *RingBuffer) Snapshot() []map[string]string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	out := make([]map[string]string, 0, b.r.Len())
	b.r.Do(func(v any) {
		if v != nil {
			if m, ok := v.(map[string]string); ok {
				out = append(out, m)
			}
		}
	})
	return out
}

// DefaultLogBuffer 全局默认 ring buffer（capacity=500）
var DefaultLogBuffer = NewRingBuffer(500)
