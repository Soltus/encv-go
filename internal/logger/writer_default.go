//go:build !js

package logger

import (
	"io"
	"os"
)

// stderrWriter 是日志的默认输出目标。
// js/wasm 目标另有实现（见 writer_js.go）：那里不能写 os.Stderr。
func stderrWriter() io.Writer { return os.Stderr }
