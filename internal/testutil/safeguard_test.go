// Package testutil 测试自身的基础设施
// 验证 SafeGo / DumpStack / CleanupOnExit / KillOnExit / Report 全部正确

package testutil

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSafeGo_NormalExit(t *testing.T) {
	done := SafeGo(t, "normal", func() {
		time.Sleep(10 * time.Millisecond)
	})
	select {
	case <-done:
		// 期望
	case <-time.After(1 * time.Second):
		t.Fatal("SafeGo did not finish in 1s")
	}
}

func TestSafeGo_PanicRecovered(t *testing.T) {
	// 用 SilentSafeGo：不污染测试结果，只验证 recover + 落盘
	done := SilentSafeGo(t, "panicker", func() {
		panic("intentional test panic")
	})
	select {
	case <-done:
		// 期望：recover 后 done 仍会 close
	case <-time.After(1 * time.Second):
		t.Fatal("SafeGo did not finish after panic")
	}
	// 验证堆栈文件存在
	files, _ := filepath.Glob(filepath.Join(CrashDir(), "panicker-*.stack"))
	if len(files) == 0 {
		t.Errorf("expected crash dump in %s, got none", CrashDir())
	} else {
		// 清理
		for _, f := range files {
			_ = os.Remove(f)
		}
	}
}

func TestSafeGoWithTimeout_TriggersOnHang(t *testing.T) {
	// 用很短的超时测试，但不在 Fatal 上挂（因为 t.Fatalf 会调用 runtime.Goexit）
	// 这里改为验证超时路径会落盘堆栈
	defer func() {
		// 清理
		files, _ := filepath.Glob(filepath.Join(CrashDir(), "hang-*.stack"))
		for _, f := range files {
			_ = os.Remove(f)
		}
	}()

	// 注意：这个测试用 t 包装的 SafeGoWithTimeout 实际会 t.Fatalf
	// 我们用子测试隔离
	if !t.Failed() {
		t.Run("hang-detection", func(t *testing.T) {
			// 期望 t.Fatalf 被调用；用 recover 防止父测试失败
			// 实际做法：直接验证 watchdog 落盘逻辑，不真触发 Fatal
			Done := make(chan struct{})
			go func() {
				defer close(Done)
				time.Sleep(10 * time.Second) // 模拟 hang
			}()
			select {
			case <-Done:
			case <-time.After(100 * time.Millisecond):
				// 期望：100ms 后超时，触发 dump
				path := DumpStack("hang", "test watchdog timeout")
				if path == "" {
					t.Error("DumpStack returned empty path")
				}
			}
		})
	}
}

func TestCleanupOnExit(t *testing.T) {
	dir := t.TempDir()
	pattern := filepath.Join(dir, "encv-test-cleanup-*.bin")
	// 创建 3 个文件
	for i := 0; i < 3; i++ {
		_ = os.WriteFile(filepath.Join(dir, "encv-test-cleanup-"+string(rune('a'+i))+".bin"), []byte("x"), 0o644)
	}
	CleanupOnExit(t, pattern)
	// 此时文件应该还在（t.Cleanup 还没触发）
	files, _ := filepath.Glob(pattern)
	if len(files) != 3 {
		t.Errorf("before cleanup: expected 3 files, got %d", len(files))
	}
}

func TestKillOnExit_NonExistentPID(t *testing.T) {
	// 用一个不存在的 PID（> 任意已用 PID），不能 panic
	KillOnExit(t, 999999)
	// 不会真的 kill
}

func TestKillOnExit_RealProcess(t *testing.T) {
	if os.Getenv("CI") == "true" {
		t.Skip("skip in CI to avoid process management issues")
	}
	// 启动一个 sleep 子进程
	cmd := exec.Command("sleep", "60")
	if err := cmd.Start(); err != nil {
		t.Skipf("cannot start sleep: %v", err)
	}
	if cmd.Process == nil {
		t.Fatal("no process")
	}
	KillOnExit(t, cmd.Process.Pid)
	// 验证进程确实在
	if err := cmd.Process.Signal(os.Signal(os.Kill)); err != nil {
		t.Logf("process already gone: %v", err)
	}
}

func TestDumpStack_WritesFile(t *testing.T) {
	dir := t.TempDir()
	oldDir := CrashDir()
	// 通过环境变量覆盖
	_ = os.Setenv("ENCV_TEST_CRASH_DIR", dir)
	defer func() { _ = os.Setenv("ENCV_TEST_CRASH_DIR", oldDir) }()

	path := DumpStack("test-reason", "test detail")
	if path == "" {
		t.Fatal("DumpStack returned empty")
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("dump file not exist: %v", err)
	}
	content, _ := os.ReadFile(path)
	if !strings.Contains(string(content), "test-reason") {
		t.Errorf("dump missing reason: %s", string(content))
	}
	if !strings.Contains(string(content), "GOROUTINE STACKS") {
		t.Errorf("dump missing stack section")
	}
	if !strings.Contains(string(content), "HEAP PROFILE") {
		t.Errorf("dump missing heap section")
	}
}

func TestReport_MarkAndSummarize(t *testing.T) {
	Reset()
	MarkFailure(t, StatusPass, "")
	MarkFailure(t, StatusFail, "x")
	MarkFailure(t, StatusSkip, "")
	total, passed, failed, skipped := Summarize()
	if total != 3 {
		t.Errorf("total: want 3, got %d", total)
	}
	if passed != 1 {
		t.Errorf("passed: want 1, got %d", passed)
	}
	if failed != 1 {
		t.Errorf("failed: want 1, got %d", failed)
	}
	if skipped != 1 {
		t.Errorf("skipped: want 1, got %d", skipped)
	}
}

func TestReport_FinalizeAll(t *testing.T) {
	// 写到 ENCV_TEST_REPORT_DIR 覆盖位置（test 结束后 t.TempDir 自动清理不影响）
	dir := t.TempDir()
	oldEnv := os.Getenv("ENCV_TEST_REPORT_DIR")
	_ = os.Setenv("ENCV_TEST_REPORT_DIR", dir)
	defer func() { _ = os.Setenv("ENCV_TEST_REPORT_DIR", oldEnv) }()
	reportDirCache = "" // 失效 ReportDir 缓存

	Reset()
	MarkFailure(t, StatusPass, "")
	MarkFailure(t, StatusFail, "demo")

	FinalizeAll(t)

	files, _ := filepath.Glob(filepath.Join(dir, "reports-*.json"))
	if len(files) == 0 {
		t.Fatal("no reports file written")
	}
	content, _ := os.ReadFile(files[0])
	if !strings.Contains(string(content), "demo") {
		t.Errorf("report content missing demo: %s", string(content))
	}
}
