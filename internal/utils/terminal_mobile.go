//go:build android

package utils

import (
	"fmt"
	"log/slog"
)

func PrintSuccess(format string, args ...any) {
	slog.Info(fmt.Sprintf(format, args...))
}

func PrintError(format string, args ...any) {
	slog.Error(fmt.Sprintf(format, args...))
}

func PrintInfo(format string, args ...any) {
	slog.Info(fmt.Sprintf(format, args...))
}

func PrintWarning(format string, args ...any) {
	slog.Warn(fmt.Sprintf(format, args...))
}

func PrintHeader(text string) {
	slog.Info(text)
}

func PrintSection(text string) {
	slog.Info(text)
}

type SpinnerStub struct{}

func (s *SpinnerStub) Stop() error                      { return nil }
func (s *SpinnerStub) Success(text string) *SpinnerStub { return s }
func (s *SpinnerStub) Fail(text string) *SpinnerStub    { return s }

func NewSpinner(text string) (*SpinnerStub, error) {
	slog.Info(text)
	return &SpinnerStub{}, nil
}

func PrintTable(header []string, data [][]string) {
	slog.Info("table output not supported on mobile")
}

func PrintBox(title, content string) {
	slog.Info(title, "content", content)
}

func PrintKV(key, value string) {
	slog.Info(key, "value", value)
}

func Green(text string) string  { return text }
func Yellow(text string) string { return text }
func Cyan(text string) string   { return text }
