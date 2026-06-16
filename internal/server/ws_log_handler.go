package server

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/Soltus/encv-go/internal/logger"
	"github.com/Soltus/encv-go/internal/service"
)

type WSLogHandler struct {
	inner    slog.Handler
	hub      *service.WSHub
	minLevel slog.Level
}

func NewWSLogHandler(inner slog.Handler, hub *service.WSHub, minLevel slog.Level) *WSLogHandler {
	return &WSLogHandler{inner: inner, hub: hub, minLevel: minLevel}
}

func (h *WSLogHandler) SetMinLevel(level slog.Level) {
	h.minLevel = level
}

func (h *WSLogHandler) Enabled(ctx context.Context, level slog.Level) bool {
	if level < h.minLevel {
		return false
	}
	return h.inner.Enabled(ctx, level)
}

func (h *WSLogHandler) Handle(ctx context.Context, r slog.Record) error {
	if r.Level >= h.minLevel && h.hub != nil {
		levelStr := "info"
		switch {
		case r.Level >= slog.LevelError:
			levelStr = "error"
		case r.Level >= slog.LevelWarn:
			levelStr = "warn"
		case r.Level >= slog.LevelDebug:
			levelStr = "debug"
		default:
			levelStr = "info"
		}

		msg, _ := json.Marshal(map[string]any{
			"type": "log",
			"data": map[string]string{
				"level":     levelStr,
				"message":   r.Message,
				"timestamp": time.Now().Format("15:04:05"),
			},
		})
		h.hub.BroadcastRaw(msg)
		// 🆕 2026-06-16: 同步写入 ring buffer，供 http-poll 降级模式 GET /api/logs/recent 拉取
		logger.DefaultLogBuffer.Push(map[string]string{
			"level":     levelStr,
			"message":   r.Message,
			"timestamp": time.Now().Format("15:04:05"),
		})
	}

	return h.inner.Handle(ctx, r)
}

func (h *WSLogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &WSLogHandler{
		inner: h.inner.WithAttrs(attrs),
		hub:   h.hub,
	}
}

func (h *WSLogHandler) WithGroup(name string) slog.Handler {
	return &WSLogHandler{
		inner: h.inner.WithGroup(name),
		hub:   h.hub,
	}
}
