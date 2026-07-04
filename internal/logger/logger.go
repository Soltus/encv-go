// internal/logger/logger.go
// 提供结构化日志功能，独立于其他包以避免循环依赖
package logger

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// LogLevel 定义日志级别
type LogLevel int

const (
	LevelDebug LogLevel = iota
	LevelInfo
	LevelWarn
	LevelError
)

// ANSI 颜色代码
const (
	colorReset  = "\033[0m"
	colorGray   = "\033[90m"
	colorBlue   = "\033[34m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorRed    = "\033[31m"
	colorCyan   = "\033[36m"
)

var (
	defaultLogger *slog.Logger
	mu            sync.Mutex
)

// Init 初始化结构化日志
// level: 日志级别
// logFile: 日志文件路径，为空则只输出到控制台
// 可以多次调用以更新日志配置（例如启动时先初始化基础日志，再根据配置更新）
func Init(level LogLevel, logFile string) error {
	mu.Lock()
	defer mu.Unlock()

	slogLevel := mapLogLevel(level)

	// 创建处理器
	var handler slog.Handler

	// 如果指定了日志文件，同时输出到控制台和文件
	if logFile != "" {
		// 确保目录存在
		dir := filepath.Dir(logFile)
		if dir != "" && dir != "." {
			if err := os.MkdirAll(dir, 0755); err != nil {
				return fmt.Errorf("failed to create log directory: %w", err)
			}
		}

		file, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return fmt.Errorf("failed to open log file: %w", err)
		}

		// 控制台使用带颜色的文本格式，文件使用 JSON 格式
		handler = &multiFormatHandler{
			consoleHandler: newTextHandler(os.Stderr, slogLevel, true),
			fileHandler:    newJSONHandler(file, slogLevel),
			level:          slogLevel,
		}
	} else {
		// 只输出到控制台，带颜色
		handler = newTextHandler(os.Stderr, slogLevel, true)
	}

	defaultLogger = slog.New(handler)
	slog.SetDefault(defaultLogger)

	return nil
}

// mapLogLevel 将 LogLevel 映射到 slog.Level
func mapLogLevel(level LogLevel) slog.Level {
	switch level {
	case LevelDebug:
		return slog.LevelDebug
	case LevelInfo:
		return slog.LevelInfo
	case LevelWarn:
		return slog.LevelWarn
	case LevelError:
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// textHandler 自定义的文本格式 Handler，支持颜色
type textHandler struct {
	w        io.Writer
	level    slog.Level
	useColor bool
	attrs    []slog.Attr
	groups   []string
}

// newTextHandler 创建文本格式 Handler
func newTextHandler(w io.Writer, level slog.Level, useColor bool) *textHandler {
	return &textHandler{
		w:        w,
		level:    level,
		useColor: useColor,
	}
}

func (h *textHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return level >= h.level
}

func (h *textHandler) Handle(ctx context.Context, r slog.Record) error {
	// 格式化时间: 2006-01-02 15:04:05
	timeStr := r.Time.Format("2006-01-02 15:04:05")

	// 格式化日志级别，带颜色
	levelStr := h.formatLevel(r.Level)

	// 构建属性字符串
	var attrs []string

	// 添加预定义的 attrs
	for _, attr := range h.attrs {
		attrs = append(attrs, h.formatAttr(attr))
	}

	// 添加 record 的 attrs
	r.Attrs(func(a slog.Attr) bool {
		attrs = append(attrs, h.formatAttr(a))
		return true
	})

	// 构建完整日志行
	var sb strings.Builder
	sb.WriteString("🦢 ")
	sb.WriteString(timeStr)
	sb.WriteString(" ")
	sb.WriteString(levelStr)

	// 添加 source 信息（如果存在）
	if r.PC != 0 {
		fs := runtime.CallersFrames([]uintptr{r.PC})
		frame, _ := fs.Next()
		if frame.File != "" {
			source := fmt.Sprintf("%s:%d", filepath.Base(frame.File), frame.Line)
			sb.WriteString(" ")
			if h.useColor {
				sb.WriteString(colorGray)
			}
			sb.WriteString("[")
			sb.WriteString(source)
			sb.WriteString("]")
			if h.useColor {
				sb.WriteString(colorReset)
			}
		}
	}

	// 添加其他属性
	for _, attr := range attrs {
		if attr != "" {
			sb.WriteString(" ")
			sb.WriteString(attr)
		}
	}

	// 添加消息
	sb.WriteString(" ")
	sb.WriteString(r.Message)
	sb.WriteString("\n")

	_, err := h.w.Write([]byte(sb.String()))
	return err
}

// formatLevel 格式化日志级别，带颜色
func (h *textHandler) formatLevel(level slog.Level) string {
	levelName := level.String()
	upperName := strings.ToUpper(levelName)

	if !h.useColor {
		return "[" + upperName + "]"
	}

	var color string
	switch level {
	case slog.LevelDebug:
		color = colorGray
	case slog.LevelInfo:
		color = colorGreen
	case slog.LevelWarn:
		color = colorYellow
	case slog.LevelError:
		color = colorRed
	default:
		color = colorReset
	}

	return color + "[" + upperName + "]" + colorReset
}

// formatAttr 格式化属性
func (h *textHandler) formatAttr(a slog.Attr) string {
	if a.Equal(slog.Attr{}) {
		return ""
	}

	key := a.Key
	value := a.Value.String()

	// 跳过 time 和 level，因为它们已经单独处理了
	if key == slog.TimeKey || key == slog.LevelKey {
		return ""
	}

	// 跳过 source，因为它已经单独处理了
	if key == slog.SourceKey {
		return ""
	}

	if h.useColor {
		return fmt.Sprintf("%s%s=%s%s", colorCyan, key, colorReset, value)
	}
	return fmt.Sprintf("%s=%s", key, value)
}

func (h *textHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	newHandler := &textHandler{
		w:        h.w,
		level:    h.level,
		useColor: h.useColor,
		attrs:    append(h.attrs, attrs...),
		groups:   h.groups,
	}
	return newHandler
}

func (h *textHandler) WithGroup(name string) slog.Handler {
	newHandler := &textHandler{
		w:        h.w,
		level:    h.level,
		useColor: h.useColor,
		attrs:    h.attrs,
		groups:   append(h.groups, name),
	}
	return newHandler
}

// jsonHandler 简化的 JSON Handler
type jsonHandler struct {
	w      io.Writer
	level  slog.Level
	attrs  []slog.Attr
	groups []string
}

// newJSONHandler 创建 JSON Handler
func newJSONHandler(w io.Writer, level slog.Level) *jsonHandler {
	return &jsonHandler{
		w:     w,
		level: level,
	}
}

func (h *jsonHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return level >= h.level
}

func (h *jsonHandler) Handle(ctx context.Context, r slog.Record) error {
	// 使用 slog 的 JSONHandler 来处理
	opts := &slog.HandlerOptions{
		Level:     h.level,
		AddSource: true,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey {
				if t, ok := a.Value.Any().(time.Time); ok {
					return slog.Attr{
						Key:   slog.TimeKey,
						Value: slog.StringValue(t.Format("2006-01-02 15:04:05")),
					}
				}
			}
			return a
		},
	}
	handler := slog.NewJSONHandler(h.w, opts)

	// 应用 attrs 和 groups
	for _, attr := range h.attrs {
		handler = handler.WithAttrs([]slog.Attr{attr}).(*slog.JSONHandler)
	}
	for _, group := range h.groups {
		handler = handler.WithGroup(group).(*slog.JSONHandler)
	}

	return handler.Handle(ctx, r)
}

func (h *jsonHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &jsonHandler{
		w:      h.w,
		level:  h.level,
		attrs:  append(h.attrs, attrs...),
		groups: h.groups,
	}
}

func (h *jsonHandler) WithGroup(name string) slog.Handler {
	return &jsonHandler{
		w:      h.w,
		level:  h.level,
		attrs:  h.attrs,
		groups: append(h.groups, name),
	}
}

// multiFormatHandler 同时输出不同格式到不同目标
type multiFormatHandler struct {
	consoleHandler slog.Handler
	fileHandler    slog.Handler
	level          slog.Level
}

func (h *multiFormatHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return level >= h.level
}

func (h *multiFormatHandler) Handle(ctx context.Context, r slog.Record) error {
	var firstErr error
	if err := h.consoleHandler.Handle(ctx, r); err != nil {
		firstErr = err
	}
	if err := h.fileHandler.Handle(ctx, r); err != nil && firstErr == nil {
		firstErr = err
	}
	return firstErr
}

func (h *multiFormatHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &multiFormatHandler{
		consoleHandler: h.consoleHandler.WithAttrs(attrs),
		fileHandler:    h.fileHandler.WithAttrs(attrs),
		level:          h.level,
	}
}

func (h *multiFormatHandler) WithGroup(name string) slog.Handler {
	return &multiFormatHandler{
		consoleHandler: h.consoleHandler.WithGroup(name),
		fileHandler:    h.fileHandler.WithGroup(name),
		level:          h.level,
	}
}

// Default 返回默认的 logger
func Default() *slog.Logger {
	mu.Lock()
	defer mu.Unlock()
	if defaultLogger == nil {
		handler := newTextHandler(os.Stderr, slog.LevelInfo, true)
		defaultLogger = slog.New(handler)
		slog.SetDefault(defaultLogger)
	}
	return defaultLogger
}

// WithComponent 创建一个带有组件属性的 logger
func WithComponent(component string) *slog.Logger {
	return Default().With(slog.String("component", component))
}

// Debug 输出 Debug 级别日志
func Debug(msg string, args ...any) {
	Default().Debug(msg, args...)
}

// Info 输出 Info 级别日志
func Info(msg string, args ...any) {
	Default().Info(msg, args...)
}

// Warn 输出 Warn 级别日志
func Warn(msg string, args ...any) {
	Default().Warn(msg, args...)
}

// Error 输出 Error 级别日志
func Error(msg string, args ...any) {
	Default().Error(msg, args...)
}
