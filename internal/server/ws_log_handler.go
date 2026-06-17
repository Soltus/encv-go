package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
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
		// 🆕 2026-06-17：level 映射 bug 修复
		// 旧实现：case r.Level >= slog.LevelDebug: levelStr = "debug" + default: "info"
		// → slog.LevelInfo (0) 也 >= slog.LevelDebug (-4) → 所有 INFO 误标 "debug"
		// 新实现：依次 case，从高到低（Error → Warn → Info → Debug）
		levelStr := "info"
		switch {
		case r.Level >= slog.LevelError:
			levelStr = "error"
		case r.Level >= slog.LevelWarn:
			levelStr = "warn"
		case r.Level >= slog.LevelInfo:
			levelStr = "info"
		case r.Level >= slog.LevelDebug:
			levelStr = "debug"
		}

		// 🆕 2026-06-17：把 Record 的 attrs 序列化成 key=value 串附加到 message
		// 旧实现：只发 message，结构化字段（method/path/status/...）被丢
		// 浏览器端 DevLogs 才能看到完整日志
		fullMessage := r.Message
		if r.NumAttrs() > 0 {
			parts := make([]string, 0, r.NumAttrs())
			r.Attrs(func(a slog.Attr) bool {
				parts = append(parts, formatAttr(a))
				return true
			})
			if len(parts) > 0 {
				fullMessage = r.Message + " " + strings.Join(parts, " ")
			}
		}

		timestamp := time.Now().Format("15:04:05")

		msg, _ := json.Marshal(map[string]any{
			"type": "log",
			"data": map[string]string{
				"level":     levelStr,
				"message":   fullMessage,
				"timestamp": timestamp,
			},
		})
		h.hub.BroadcastRaw(msg)
		// 同步写入 ring buffer，供 http-poll 降级模式 GET /api/logs/recent 拉取
		logger.DefaultLogBuffer.Push(map[string]string{
			"level":     levelStr,
			"message":   fullMessage,
			"timestamp": timestamp,
		})
	}

	return h.inner.Handle(ctx, r)
}

// formatAttr 把 slog.Attr 序列化为 "key=value"（递归处理 group）
//
// 🆕 2026-06-17：WSLogHandler 现在透传 attrs，必须序列化
// - string 字段：直接输出
// - 其他类型：fmt.Sprintf("%v", ...) 输出
// - slog.LogValuer：调用 LogValue() 递归
func formatAttr(a slog.Attr) string {
	if a.Equal(slog.Attr{}) {
		return ""
	}
	if a.Value.Kind() == slog.KindGroup {
		// group attrs 不常见，简化处理：flat 化
		parts := make([]string, 0, len(a.Value.Group()))
		for _, sub := range a.Value.Group() {
			parts = append(parts, formatAttr(sub))
		}
		return a.Key + "={" + strings.Join(parts, " ") + "}"
	}
	if a.Value.Kind() == slog.KindString {
		// 含空格/特殊字符的 string 字段用引号包裹
		v := a.Value.String()
		if strings.ContainsAny(v, " \t\n\"'") {
			return fmt.Sprintf("%s=%q", a.Key, v)
		}
		return a.Key + "=" + v
	}
	return a.Key + "=" + a.Value.String()
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
