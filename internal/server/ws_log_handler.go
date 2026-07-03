package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"runtime"
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

		fullMessage := r.Message
		hasError := false
		hasTask := false
		if r.NumAttrs() > 0 {
			parts := make([]string, 0, r.NumAttrs())
			r.Attrs(func(a slog.Attr) bool {
				if a.Key == "error" || a.Key == "err" {
					hasError = true
				}
				if a.Key == "taskId" || a.Key == "task_id" || a.Key == "id" && strings.Contains(strings.ToLower(fullMessage), "task") {
					hasTask = true
				}
				parts = append(parts, formatAttr(a))
				return true
			})
			if len(parts) > 0 {
				fullMessage = r.Message + " " + strings.Join(parts, " ")
			}
		}

		tags := deriveBackendTags(r, fullMessage, hasError, hasTask)

		timestamp := time.Now().Format("15:04:05")

		data := map[string]any{
			"level":     levelStr,
			"message":   fullMessage,
			"timestamp": timestamp,
			"tags":      tags,
			"source":    "ws_log_handler",
		}

		msg, _ := json.Marshal(map[string]any{
			"type": "log",
			"data": data,
		})
		h.hub.BroadcastRaw(msg)
		// 同步写入 ring buffer，供 http-poll 降级模式 GET /api/logs/recent 拉取
		logger.DefaultLogBuffer.Push(map[string]string{
			"level":     levelStr,
			"message":   fullMessage,
			"timestamp": timestamp,
			"tags":      strings.Join(tags, ","),
			"source":    "ws_log_handler",
		})
	}

	return h.inner.Handle(ctx, r)
}

// deriveBackendTags 从 slog.Record 派生出多维度标签。
//
// 标签维度（不含级别 — 级别是独立字段，不放在 tags 里）：
//   - 来源大类：backend
//   - 子系统：api / websocket / task / mount / db / kernel / service / agent / general
//   - 模块包名：pkg.xxx / internal.xxx（从 PC 解析）
//   - 特殊标记：has-error / has-task
func deriveBackendTags(r slog.Record, fullMessage string, hasError, hasTask bool) []string {
	tags := []string{"backend"}

	// 从消息前缀提取子系统分类
	msgLower := strings.ToLower(r.Message)
	switch {
	case strings.HasPrefix(msgLower, "api:") || strings.HasPrefix(msgLower, "http:"):
		tags = append(tags, "api")
	case strings.HasPrefix(msgLower, "ws:") || strings.HasPrefix(msgLower, "websocket:"):
		tags = append(tags, "websocket")
	case strings.HasPrefix(msgLower, "task:") || strings.Contains(msgLower, "task"):
		tags = append(tags, "task")
	case strings.HasPrefix(msgLower, "mount:"):
		tags = append(tags, "mount")
	case strings.HasPrefix(msgLower, "db:") || strings.Contains(msgLower, "database") || strings.Contains(msgLower, "sqlite") || strings.Contains(msgLower, "libsql"):
		tags = append(tags, "db")
	case strings.HasPrefix(msgLower, "kernel:") || strings.Contains(msgLower, "plugin"):
		tags = append(tags, "kernel")
	case strings.HasPrefix(msgLower, "service:"):
		tags = append(tags, "service")
	case strings.HasPrefix(msgLower, "agent:") || strings.Contains(msgLower, "agent") || strings.Contains(msgLower, "ai_agent"):
		tags = append(tags, "agent")
	default:
		tags = append(tags, "general")
	}

	// 从 PC 解析包名（模块维度，越细越好）
	if r.PC != 0 {
		if fn := runtime.FuncForPC(r.PC); fn != nil {
			fullName := fn.Name()
			// 格式：github.com/Soltus/encv-go/internal/server.(*Server).HandleLogs-fm
			// 提取包路径部分
			if idx := strings.Index(fullName, "encv-go/"); idx >= 0 {
				pkgPath := fullName[idx+len("encv-go/"):]
				// 去掉方法名，保留包路径
				if dotIdx := strings.Index(pkgPath, "."); dotIdx >= 0 {
					pkgPath = pkgPath[:dotIdx]
				}
				// 替换 / 为 .，方便标签展示
				pkgTag := strings.ReplaceAll(pkgPath, "/", ".")
				tags = append(tags, pkgTag)
			}
		}
	}

	// 特殊属性标记
	if hasError {
		tags = append(tags, "has-error")
	}
	if hasTask {
		tags = append(tags, "has-task")
	}

	return tags
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
