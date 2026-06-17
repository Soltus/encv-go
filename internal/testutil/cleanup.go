// Package testutil/cleanup.go
// =====================================================
// 测试退出清理工具：临时文件 / 子进程 / 自定义资源。
//
// 背景：测试跨次累积导致沙箱资源耗尽（/tmp 数十 GB、僵尸子进程）。
// 本文件提供：
//   - CleanupOnExit: 退出时按 glob 清理文件
//   - KillOnExit:    退出时按 PID 杀进程（不只 SIGTERM，会 SIGKILL）
//   - CleanupFunc:   通用 defer 注册（任意资源释放）
//
// 用法：
//
//	func TestSomething(t *testing.T) {
//	    testutil.CleanupOnExit(t, "/tmp/encv-test-*.bin")
//	    testutil.KillOnExit(t, cmd.Process.Pid)
//	    testutil.CleanupFunc(t, func() { db.Close() })
//	}
//
// 2026-06-15 创建（test-architecture-refactor-defense-awareness Sprint 1）
// =====================================================

package testutil

import (
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"testing"
	"time"
)

// CleanupOnExit 注册测试退出时按 glob pattern 清理文件/目录。
//
// patterns 示例：
//   - "/tmp/encv-test-*.bin"        — 匹配所有 /tmp/encv-test-XXX.bin
//   - "/tmp/encv-test-*"             — 匹配所有 /tmp/encv-test-XXX（无后缀）
//   - "/tmp/scratch-*/"              — 匹配所有 /tmp/scratch-XXX/ 目录
//
// 注意：
//   - 已注册到 t.Cleanup；test 失败也会触发
//   - 匹配失败（无文件）静默通过
//   - 权限不足静默跳过（不污染测试结果）
func CleanupOnExit(t *testing.T, patterns ...string) {
	t.Helper()
	t.Cleanup(func() {
		for _, p := range patterns {
			matches, err := filepath.Glob(p)
			if err != nil {
				// glob 语法错
				t.Logf("[CleanupOnExit] invalid glob %q: %v", p, err)
				continue
			}
			for _, m := range matches {
				if err := os.RemoveAll(m); err != nil {
					// 权限不足等静默跳过
					t.Logf("[CleanupOnExit] remove %q: %v", m, err)
				}
			}
		}
	})
}

// KillOnExit 注册测试退出时 SIGKILL 指定 PID。
//
// 用于测试中启动的子进程（如 cmd := exec.Command(...)）。
// 之所以用 SIGKILL 而非 SIGTERM：
//   - SIGTERM 给的 grace period 可能让子进程 hang
//   - 测试进程退出时，子进程已无父，必须强杀
//
// ⚠️ 【P1-7 限制】只接受 PID 不接受 *exec.Cmd → 无法调用 cmd.Wait()，
//
//	进程 reap 责任在沙箱 init(1)。多次跑后 PID 表可能满（僵尸累积）。
//	若需要 reap 语义，请用 KillCmdOnExit(cmd) 直接传 *exec.Cmd。
//
// 用法：
//
//	cmd := exec.Command("ffmpeg", ...)
//	cmd.Start()
//	testutil.KillOnExit(t, cmd.Process.Pid)
func KillOnExit(t *testing.T, pids ...int) {
	t.Helper()
	t.Cleanup(func() {
		for _, pid := range pids {
			if pid <= 0 {
				continue
			}
			// 检查进程是否存在（避免 signal to non-existent process 报错）
			if err := syscall.Kill(pid, syscall.Signal(0)); err != nil {
				continue
			}
			// 先 SIGTERM 1s，再 SIGKILL
			_ = syscall.Kill(pid, syscall.SIGTERM)
		}
		// 给 SIGTERM 一点时间（50ms），但不能阻塞太久
		// t.Cleanup 中不能用 sleep 长；用 SIGKILL 兜底
		for _, pid := range pids {
			if pid <= 0 {
				continue
			}
			_ = syscall.Kill(pid, syscall.SIGKILL)
		}
	})
}

// KillCmdOnExit 注册测试退出时按 SIGTERM→SIGKILL 杀 cmd，并 Wait() reap 进程。
//
// 【P1-7 修复】补全 KillOnExit 的 reap 语义：
//   - 先 SIGTERM 给 1s grace period，让子进程清理 tmpfile / 关闭 fd
//   - 兜底 SIGKILL
//   - cmd.Wait() 回收 zombie（避免 init(1) reap 累积）
//
// 与 KillOnExit 的取舍：KillOnExit 只接 PID（轻量），但无 reap；
// KillCmdOnExit 接 *exec.Cmd（有 reap），但调用方需持有 cmd 引用。
func KillCmdOnExit(t *testing.T, cmds ...*exec.Cmd) {
	t.Helper()
	t.Cleanup(func() {
		var wg sync.WaitGroup
		for _, cmd := range cmds {
			if cmd == nil || cmd.Process == nil {
				continue
			}
			wg.Add(1)
			go reapCmd(cmd, &wg)
		}
		wg.Wait() // 整体 cleanup 阻塞上限 ≈ 1s
	})
}

// reapCmd Wait() cmd → SIGTERM 等 1s → SIGKILL 兜底 → 再 Wait() reap。
// Wait() 在 cmd 生命周期内只能成功调一次，所以用 done chan 包装。
func reapCmd(cmd *exec.Cmd, wg *sync.WaitGroup) {
	defer wg.Done()
	done := make(chan struct{})
	go func() {
		_, _ = cmd.Process.Wait() //nolint:errcheck // 用 close 通知
		close(done)
	}()
	// 进程已死（Wait 成功返回 err=nil 或 err="wait: no child processes"），立即返
	// 但要确保 reap 完成，所以仍 select 一次
	_ = cmd.Process.Signal(syscall.SIGTERM)
	timer := time.NewTimer(1 * time.Second)
	defer timer.Stop()
	select {
	case <-done:
		// 已自然退出 / reap 完成
	case <-timer.C:
		_ = cmd.Process.Kill()
		<-done
	}
}

// CleanupFunc 注册一个测试退出时的任意清理函数。
// 比 t.Cleanup 直接调用更明确（"这是退出清理"）。
func CleanupFunc(t *testing.T, name string, fn func()) {
	t.Helper()
	t.Cleanup(func() {
		defer func() {
			if r := recover(); r != nil {
				path := DumpStack("cleanup-"+name, fmt_Sprint("panic in cleanup %s: %v", name, r))
				t.Logf("[CleanupFunc] panic in %s: %v (dump: %s)", name, r, path)
			}
		}()
		fn()
	})
}

// fmt_Sprint 内部用的轻量 sprintf 包装，避免再 import fmt
// （本文件头部不 import fmt 是为了减少 side effect 概率）
func fmt_Sprint(format string, args ...interface{}) string {
	// 用最朴素的实现，调用方保证安全
	return fmtSprintf(format, args...)
}

// fmtSprintf 等价 fmt.Sprintf
func fmtSprintf(format string, args ...interface{}) string {
	return sprintfImpl(format, args...)
}
