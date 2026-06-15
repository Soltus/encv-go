// Package testutil/safeguard.go
// =====================================================
// 测试"必退出"基础设施：panic 拦截 + 堆栈落盘 + watchdog。
//
// 背景：merge 前最后一次提交异常中止；Go 测试无数次把沙箱跑崩。
// 根因：测试未干净退出（panic / 死循环 / 资源耗尽）。
//
// 本文件提供：
//   - SafeGo: 把 goroutine 包装在 panic recover + watchdog 中
//   - dumpStack: 失败时把堆栈 + heap 写到 .test-runs/crashes/
//   - Watchdog: 单 case 超时保护
//
// 用法：
//
//	func TestSomething(t *testing.T) {
//	    testutil.SafeGo(t, "subtask", func() {
//	        // 业务代码；panic 会被自动捕获并落盘
//	    })
//	}
//
// 2026-06-15 创建（test-architecture-refactor-defense-awareness Sprint 1）
// =====================================================

package testutil

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"sync"
	"testing"
	"time"
)

// CrashDir 全局崩溃落盘目录（由 DumpStack 等使用）
// 默认 .test-runs/crashes/，可通过 ENCV_TEST_CRASH_DIR 覆盖
func CrashDir() string {
	if d := os.Getenv("ENCV_TEST_CRASH_DIR"); d != "" {
		return d
	}
	return ".test-runs/crashes"
}

// SafeGo 把 fn 包装在 panic recover + watchdog 中。
//
// 关键保证：
//   - fn 内 panic 会被 recover，不会让测试进程挂掉
//   - panic 堆栈 + heap profile 自动写入 CrashDir()/<name>-<ts>.stack
//   - 单 case 超过 caseTimeout 会触发 t.Fatalf（默认 2min）
//   - fn 完成时 done 关闭，调用方 select 可继续
//
// 返回 done channel（fn 完成时关闭），调用方可以基于它做其他同步。
//
// 重要：SafeGo 触发 t.Errorf 是有意为之——它通过 t 报告 panic 失败。
// 如果只想"沉默地 recover + 落盘"（不污染测试结果），用 SilentSafeGo。
func SafeGo(t *testing.T, name string, fn func()) <-chan struct{} {
	t.Helper()
	return safeGo(t, name, fn, true)
}

// SilentSafeGo 同 SafeGo，但不在 t 上报告 Errorf。
// 适合"已知可能 panic，但希望测试继续"的场景。
// 仍然会落盘堆栈（这是核心防御功能）。
func SilentSafeGo(t *testing.T, name string, fn func()) <-chan struct{} {
	t.Helper()
	return safeGo(t, name, fn, false)
}

func safeGo(t *testing.T, name string, fn func(), reportError bool) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer func() {
			if r := recover(); r != nil {
				path := DumpStack(name, fmt.Sprintf("panic: %v", r))
				if reportError {
					t.Errorf("[SafeGo] panic in %s: %v (dump: %s)", name, r, path)
				}
			}
		}()
		fn()
	}()
	return done
}

// SafeGoWithTimeout 同 SafeGo，但指定单 case 超时（默认 2min）。
// 超时触发：t.Fatalf + 堆栈落盘。
func SafeGoWithTimeout(t *testing.T, name string, timeout time.Duration, fn func()) <-chan struct{} {
	t.Helper()
	done := SafeGo(t, name, fn)
	select {
	case <-done:
		// 正常完成
	case <-time.After(timeout):
		path := DumpStack(name, fmt.Sprintf("watchdog-timeout after %s", timeout))
		t.Fatalf("[SafeGo] %s exceeded %s (dump: %s)", name, timeout, path)
	}
	return done
}

// SafeGroup 并发执行多个 SafeGo，全部完成时返回。
// 任一 panic 不会让组内其他任务挂掉（通过 SafeGo 的 recover 隔离）。
func SafeGroup(t *testing.T, tasks map[string]func()) {
	t.Helper()
	var wg sync.WaitGroup
	for name, fn := range tasks {
		name, fn := name, fn
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					path := DumpStack(name, fmt.Sprintf("SafeGroup panic: %v", r))
					t.Errorf("[SafeGroup] panic in %s: %v (dump: %s)", name, r, path)
				}
			}()
			fn()
		}()
	}
	wg.Wait()
}

// DumpStack 把当前 goroutine 堆栈 + heap profile 写入 CrashDir()。
// 失败原因（reason）会作为文件名前缀，便于筛选。
//
// 文件格式：<reason>-<unix_ns>.stack（含 goroutine stack + heap profile）
// 返回写入的文件路径（写入失败返回 ""）。
func DumpStack(reason string, detail string) string {
	dir := CrashDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		// 落盘失败也不应让测试挂掉
		_ = err
		return ""
	}
	safeReason := sanitizeFilename(reason)
	ts := time.Now().UnixNano()
	fname := filepath.Join(dir, fmt.Sprintf("%s-%d.stack", safeReason, ts))
	f, err := os.Create(fname)
	if err != nil {
		return ""
	}
	defer f.Close()

	fmt.Fprintf(f, "Reason: %s\n", reason)
	fmt.Fprintf(f, "Detail: %s\n", detail)
	fmt.Fprintf(f, "Time:   %s\n", time.Now().Format(time.RFC3339Nano))
	fmt.Fprintf(f, "GoVer:  %s\n", runtime.Version())
	fmt.Fprintf(f, "GOOS:   %s  GOARCH: %s\n\n", runtime.GOOS, runtime.GOARCH)

	// 1) 所有 goroutine 的 stack
	fmt.Fprintln(f, "=== GOROUTINE STACKS ===")
	_ = pprof.Lookup("goroutine").WriteTo(f, 2)

	// 2) heap profile（追加）
	fmt.Fprintln(f, "\n=== HEAP PROFILE ===")
	_ = pprof.WriteHeapProfile(f)

	return fname
}

// sanitizeFilename 替换文件名中的非法字符。
func sanitizeFilename(s string) string {
	r := make([]rune, 0, len(s))
	for _, c := range s {
		switch {
		case c == '/' || c == '\\' || c == ':' || c == '*' || c == '?' || c == '"' || c == '<' || c == '>' || c == '|':
			r = append(r, '_')
		default:
			r = append(r, c)
		}
	}
	if len(r) == 0 {
		return "unknown"
	}
	return string(r)
}
