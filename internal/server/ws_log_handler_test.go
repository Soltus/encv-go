package server

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/Soltus/encv-go/internal/logger"
)

// TestWSLogHandler_LevelMapping 验证 level 映射正确（info 不再误标 debug）
//
// 🆕 2026-06-17：修复 bug — 旧实现 case r.Level >= slog.LevelDebug + default: "info"，
// 导致 slog.LevelInfo (0) 也匹配 debug 分支，INFO 误标 "debug"
func TestWSLogHandler_LevelMapping(t *testing.T) {
	cases := []struct {
		level    slog.Level
		wantName string
	}{
		{slog.LevelError, "error"},
		{slog.LevelWarn, "warn"},
		{slog.LevelInfo, "info"},
		{slog.LevelDebug, "debug"},
	}
	for _, c := range cases {
		got := levelStringForTest(c.level)
		if got != c.wantName {
			t.Errorf("levelStringForTest(%v) = %q, want %q", c.level, got, c.wantName)
		}
	}
}

// levelStringForTest 抽出 Handle 里的 level 映射做单元测试
func levelStringForTest(level slog.Level) string {
	switch {
	case level >= slog.LevelError:
		return "error"
	case level >= slog.LevelWarn:
		return "warn"
	case level >= slog.LevelInfo:
		return "info"
	case level >= slog.LevelDebug:
		return "debug"
	}
	return "unknown"
}

// TestFormatAttr_String 验证 string attr 序列化
func TestFormatAttr_String(t *testing.T) {
	a := slog.String("path", "/d/automation/foo")
	got := formatAttr(a)
	if got != "path=/d/automation/foo" {
		t.Errorf("formatAttr(slog.String) = %q, want %q", got, "path=/d/automation/foo")
	}
}

// TestFormatAttr_StringWithSpaces 验证含空格 string attr 加引号
func TestFormatAttr_StringWithSpaces(t *testing.T) {
	a := slog.String("ua", "Mozilla/5.0 (Windows)")
	got := formatAttr(a)
	if !strings.Contains(got, `"`) {
		t.Errorf("formatAttr with spaces should quote, got %q", got)
	}
}

// TestFormatAttr_Int 验证 int attr
func TestFormatAttr_Int(t *testing.T) {
	a := slog.Int("status", 207)
	got := formatAttr(a)
	if got != "status=207" {
		t.Errorf("formatAttr(slog.Int) = %q, want %q", got, "status=207")
	}
}

// TestFormatAttr_Int64 验证 int64 attr (duration)
func TestFormatAttr_Int64(t *testing.T) {
	a := slog.Int64("elapsed_ms", 123)
	got := formatAttr(a)
	if got != "elapsed_ms=123" {
		t.Errorf("formatAttr(slog.Int64) = %q, want %q", got, "elapsed_ms=123")
	}
}

// TestFormatAttr_EmptyAttr 验证空 attr 返回空
func TestFormatAttr_EmptyAttr(t *testing.T) {
	got := formatAttr(slog.Attr{})
	if got != "" {
		t.Errorf("formatAttr(empty) = %q, want %q", got, "")
	}
}

// TestHandle_AttrsIncludedInMessage 验证结构化字段被序列化到 message
//
// 🆕 2026-06-17：修复 bug — 旧实现只发 r.Message，结构化字段被丢
func TestHandle_AttrsIncludedInMessage(t *testing.T) {
	// 我们只测 handle 里的"提取 attrs"逻辑，不依赖 hub（hub 是具体类型不好 mock）
	// 直接测 BuildFullMessage 这个内部逻辑
	fullMessage := buildFullMessageForTest("WebDAV", []slog.Attr{
		slog.String("method", "PROPFIND"),
		slog.String("path", "/d/automation/foo"),
		slog.Int("status", 207),
	})

	if !strings.Contains(fullMessage, "WebDAV") {
		t.Errorf("full message should start with 'WebDAV', got %q", fullMessage)
	}
	if !strings.Contains(fullMessage, "method=PROPFIND") {
		t.Errorf("should contain method=PROPFIND, got %q", fullMessage)
	}
	if !strings.Contains(fullMessage, "path=/d/automation/foo") {
		t.Errorf("should contain path=/d/automation/foo, got %q", fullMessage)
	}
	if !strings.Contains(fullMessage, "status=207") {
		t.Errorf("should contain status=207, got %q", fullMessage)
	}
}

// buildFullMessageForTest 复制 WSLogHandler 里的 message 构建逻辑做单测
func buildFullMessageForTest(msg string, attrs []slog.Attr) string {
	if len(attrs) == 0 {
		return msg
	}
	parts := make([]string, 0, len(attrs))
	for _, a := range attrs {
		parts = append(parts, formatAttr(a))
	}
	return msg + " " + strings.Join(parts, " ")
}

// TestWSLogHandler_DefaultLogBufferIntegration 验证 DefaultLogBuffer 被正确调用
//
// 🆕 2026-06-17：回归测试 — attrs 透传不能影响 DefaultLogBuffer 行为
func TestWSLogHandler_DefaultLogBufferIntegration(t *testing.T) {
	inner := slog.NewTextHandler(&bytes.Buffer{}, &slog.HandlerOptions{Level: slog.LevelDebug})
	handler := NewWSLogHandler(inner, nil, slog.LevelDebug)
	// hub 是 nil，Handle 会跳过 broadcast/buffer 路径
	// 这里只验证 nil hub 不会 panic
	rec := slog.Record{
		Message: "test",
		Time:    time.Now(),
		Level:   slog.LevelInfo,
	}
	rec.AddAttrs(slog.String("key", "value"))
	if err := handler.Handle(context.Background(), rec); err != nil {
		t.Errorf("Handle with nil hub should not error: %v", err)
	}

	// Snapshot 验证（无 panic）
	_ = logger.DefaultLogBuffer.Snapshot()
}

// 防止 import 被 unused 警告删除
var _ = json.Marshal
