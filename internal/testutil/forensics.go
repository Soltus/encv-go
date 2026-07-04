// Package testutil/forensics.go
// =====================================================
// 测试失败取证：失败时自动保存 stack + heap + env + test log 到 .test-runs/crashes/。
//
// 背景：之前失败只能看 stderr 一行；现在失败现场必须能离线分析。
// 本文件提供：
//   - OnFailureHook: 注册 t.Cleanup，失败时落盘
//   - CaptureEnv: 把关键 env 写到 env.txt
//   - CaptureLastLogs: 抓取最近 N 行 t.Log
//
// 用法（在 testutil.Mark(t)() 旁加一行）：
//
//	func TestSomething(t *testing.T) {
//	    defer testutil.Mark(t)()
//	    testutil.OnFailureHook(t)
//	    // ...
//	}
//
// 2026-06-15 创建（test-architecture-refactor-defense-awareness Sprint 2）
// =====================================================

package testutil

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"strings"
	"testing"
	"time"
)

// forensicsKeyEnvList 取证时写入 env.txt 的环境变量列表
var forensicsKeyEnvList = []string{
	"PATH", "HOME", "USER", "PWD",
	"CI", "GITHUB_ACTIONS", "GIT_BRANCH",
	"GOOS", "GOARCH", "GOVERSION",
	"ENCV_MOBILE", "ENCV_DEV_PREVIEW",
	"ENCV_TEST_CRASH_DIR",
}

// OnFailureHook 注册测试退出时的取证钩子。
// 失败时自动落盘到 <CrashDir>/<testName>/：
//   - goroutine.stack: 所有 goroutine 堆栈
//   - heap.pprof:      heap profile
//   - env.txt:         关键环境变量
//   - test.log:        t.Log 缓冲（如果可获取）
//   - reason.txt:      失败原因简述
//
// 用法：
//
//	func TestXxx(t *testing.T) {
//	    testutil.OnFailureHook(t)
//	    // ... 测试逻辑
//	}
func OnFailureHook(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		if !t.Failed() {
			return
		}
		dir := filepath.Join(CrashDir(), sanitizeFilename(t.Name()))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Logf("[OnFailureHook] mkdir %s: %v", dir, err)
			return
		}

		// 1) reason
		_ = os.WriteFile(filepath.Join(dir, "reason.txt"),
			[]byte(fmt.Sprintf("test failed at %s\n", time.Now().Format(time.RFC3339Nano))),
			0o644)

		// 2) goroutine stack
		stackPath := filepath.Join(dir, "goroutine.stack")
		if f, err := os.Create(stackPath); err == nil {
			_ = pprof.Lookup("goroutine").WriteTo(f, 2)
			f.Close()
		}

		// 3) heap profile
		heapPath := filepath.Join(dir, "heap.pprof")
		if f, err := os.Create(heapPath); err == nil {
			_ = pprof.WriteHeapProfile(f)
			f.Close()
		}

		// 4) env
		_ = os.WriteFile(filepath.Join(dir, "env.txt"),
			[]byte(CaptureEnv()), 0o644)

		// 5) resource snapshot
		_ = os.WriteFile(filepath.Join(dir, "resources.txt"),
			[]byte(fmt.Sprintf("rss_mb=%d goroutine=%d fds=%d\ngo_ver=%s goos=%s goarch=%s\n",
				bytesToMB(readRSSBytes()), runtime.NumGoroutine(), countFDs(),
				runtime.Version(), runtime.GOOS, runtime.GOARCH)), 0o644)

		t.Logf("[FORENSICS] dumped to %s", dir)
	})
}

// CaptureEnv 抓取关键环境变量为可读文本。
func CaptureEnv() string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("# captured at %s\n", time.Now().Format(time.RFC3339Nano)))
	for _, k := range forensicsKeyEnvList {
		v := os.Getenv(k)
		if v == "" {
			v = "<unset>"
		}
		b.WriteString(fmt.Sprintf("%s=%s\n", k, v))
	}
	return b.String()
}

// CaptureStack 当前 goroutine 堆栈（text 格式）。
func CaptureStack() string {
	buf := make([]byte, 64*1024)
	n := runtime.Stack(buf, false)
	return string(buf[:n])
}

// CaptureAllStacks 所有 goroutine 堆栈（text 格式）。
func CaptureAllStacks() string {
	buf := make([]byte, 256*1024)
	n := runtime.Stack(buf, true)
	return string(buf[:n])
}
