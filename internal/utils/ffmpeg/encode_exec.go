//go:build !android

package ffmpeg

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Encode 在沙箱/dev 环境下直接 os.Exec 系统 ffmpeg binary。
//
// 与真机 worker subprocess 路径的区别：
//   - 沙箱：os.Exec 启动 ffmpeg 子进程 → ffmpeg binary 写文件 → wait → return
//   - 真机：spawn worker (C 实现) → worker 内部 cgo dlopen libffmpeg.so → ffmpeg_run
//
// 错误：返回 (nil, err) 时不保证 stdout 非空（异常时可能 stdout 是空）；调用方应
// 同时检查 result.ExitCode 和 err。
func Encode(ctx context.Context, args ...string) (*EncodeResult, error) {
	bin := locateFFmpegSystem()
	if bin == "" {
		return nil, fmt.Errorf("ffmpeg binary not found in PATH (set ENCV_FFMPEG_BIN or install ffmpeg)")
	}

	start := time.Now()
	cmd := exec.CommandContext(ctx, bin, args...)
	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	err := cmd.Run()
	exitCode := 0
	if exitErr, ok := err.(*exec.ExitError); ok {
		exitCode = exitErr.ExitCode()
	}
	return &EncodeResult{
		Stdout:     stdoutBuf.Bytes(),
		Stderr:     stderrBuf.String(),
		ExitCode:   exitCode,
		DurationMs: time.Since(start).Milliseconds(),
	}, err
}

// locateFFprobeSystem 找系统 ffprobe binary（沙箱走）。
func locateFFprobeSystem() string {
	if v := os.Getenv("ENCV_FFPROBE_BIN"); v != "" {
		if _, err := os.Stat(v); err == nil {
			return v
		}
	}
	if dir := os.Getenv("ENCV_BIN_DIR"); dir != "" {
		p := filepath.Join(dir, "ffprobe")
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	if path, err := exec.LookPath("ffprobe"); err == nil {
		return path
	}
	return ""
}

// Probe 在沙箱/dev 环境下直接 os.Exec 系统 ffprobe binary。
func Probe(ctx context.Context, args ...string) ([]byte, error) {
	bin := locateFFprobeSystem()
	if bin == "" {
		return nil, fmt.Errorf("ffprobe binary not found in PATH (set ENCV_FFPROBE_BIN or install ffprobe)")
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	return cmd.Output()
}

// IsAvailable 探活：检查沙箱 /usr/bin/ffmpeg + /usr/bin/ffprobe 是否在。
func IsAvailable() (ffmpegOk, ffprobeOk bool, errMsg string) {
	ffmpegPath := locateFFmpegSystem()
	ffprobePath := locateFFprobeSystem()

	var errMsgs []string
	if _, err := os.Stat(ffmpegPath); err != nil {
		errMsgs = append(errMsgs, fmt.Sprintf("ffmpeg not found at %s", ffmpegPath))
	} else {
		ffmpegOk = true
	}
	if _, err := os.Stat(ffprobePath); err != nil {
		errMsgs = append(errMsgs, fmt.Sprintf("ffprobe not found at %s", ffprobePath))
	} else {
		ffprobeOk = true
	}
	return ffmpegOk, ffprobeOk, strings.Join(errMsgs, "; ")
}
