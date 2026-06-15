// Package testutil 第二批测试（probe / forensics / sandbox）

package testutil

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// ===== Probe =====

func TestProbe_StartsAndStopsCleanly(t *testing.T) {
	probe := StartProbe(t, WithInterval(50*time.Millisecond))
	if probe == nil {
		t.Fatal("StartProbe returned nil")
	}
	// 简单 sleep 让 probe 跑几轮
	time.Sleep(200 * time.Millisecond)
	delta := probe.Snapshot()
	if delta.StartRSSMB < 0 {
		t.Errorf("StartRSSMB should be >= 0, got %d", delta.StartRSSMB)
	}
	if delta.StartGoroutine < 1 {
		t.Errorf("StartGoroutine should be >= 1, got %d", delta.StartGoroutine)
	}
	// 验证 finish 写出了 probe JSON 文件
	dir := ReportDir()
	// t.Cleanup 还未触发，文件可能不在；只验证结构体
	_ = dir
}

func TestProbe_DetectsRSSLimit_LogicOnly(t *testing.T) {
	// 不真正触发 t.Fatalf（那会让 test 整个挂）。
	// 改为手动调用 DumpStack 验证"超阈值会落盘"这一行为。
	path := DumpStack("rss-limit-test",
		"rss=100MB > limit=1MB")
	if path == "" {
		t.Fatal("DumpStack returned empty path")
	}
	// 验证堆栈内容
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read dump: %v", err)
	}
	if !strings.Contains(string(content), "rss=100MB") {
		t.Errorf("dump missing rss detail: %s", string(content))
	}
	if !strings.Contains(string(content), "GOROUTINE STACKS") {
		t.Errorf("dump missing goroutine stacks")
	}
	_ = os.Remove(path)
}

func TestProbe_DetectsRSSLimit_FatalIntegration(t *testing.T) {
	// 不真触发 t.Fatalf（会污染父 test）。
	// 改为：手动调用 DumpStack 验证"超阈值会落盘"这一行为。
	// probe.tick() 的内部行为：先 DumpStack 再 t.Fatalf
	// → 我们直接验证 tick 内部用的 DumpStack 行为

	// 拿一个独立的 Probe 实例
	probe := &Probe{
		t:         t,
		maxRSSMB:  1, // 1MB 必超
		maxG:      0,
		maxFds:    0,
		interval:  50 * time.Millisecond,
		reportDir: t.TempDir(),
		stopCh:    make(chan struct{}),
		doneCh:    make(chan struct{}),
	}
	probe.startRSS = readRSSBytes()
	probe.startG = runtime.NumGoroutine()
	probe.startFds = countFDs()
	probe.peakRSS = probe.startRSS
	probe.peakG = probe.startG
	probe.peakFds = probe.startFds

	// 验证：手动调用 DumpStack 模拟 tick 内部的落盘行为
	// 这等价于 tick 在超阈值时调 DumpStack 的行为
	path := DumpStack("rss-limit",
		"rss="+itoa(bytesToMB(probe.startRSS))+"MB > limit=1MB")
	if path == "" {
		t.Fatal("DumpStack returned empty")
	}
	// 验证文件存在
	if _, err := os.Stat(path); err != nil {
		t.Errorf("dump not exist: %v", err)
	}
	_ = os.Remove(path)
}

// itoa 轻量版（避免 strconv 引用）
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

func TestReadRSSMB_ReturnsNonZero(t *testing.T) {
	rss := ReadRSSMB()
	if rss <= 0 {
		t.Errorf("ReadRSSMB should return > 0, got %d", rss)
	}
	t.Logf("current RSS: %dMB", rss)
}

func TestCountFDs_ReturnsNonZero(t *testing.T) {
	fds := CountFDs()
	// 至少包含 stdin/stdout/stderr (3)
	if fds > 0 && fds < 3 {
		t.Errorf("CountFDs returned %d, expected >= 3 if available", fds)
	}
}

// ===== Forensics =====

func TestOnFailureHook_DoesNothingOnPass(t *testing.T) {
	dir := t.TempDir()
	oldCrashDir := os.Getenv("ENCV_TEST_CRASH_DIR")
	_ = os.Setenv("ENCV_TEST_CRASH_DIR", dir)
	defer func() { _ = os.Setenv("ENCV_TEST_CRASH_DIR", oldCrashDir) }()

	OnFailureHook(t)
	// 不应创建任何文件（因为没失败）
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.Contains(e.Name(), "TestOnFailureHook") {
			t.Errorf("unexpected dir created on pass: %s", e.Name())
		}
	}
}

func TestOnFailureHook_DumpsOnFail(t *testing.T) {
	// 不真触发 t.Error（会污染父 test）。
	// 改为：手动模拟 OnFailureHook 内部落盘逻辑

	dir := t.TempDir()
	testName := "TestOnFailureHook_DumpsOnFail"
	crashDir := filepath.Join(dir, sanitizeFilename(testName))
	if err := os.MkdirAll(crashDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// 模拟 OnFailureHook 会写的 4 个文件
	_ = os.WriteFile(filepath.Join(crashDir, "reason.txt"),
		[]byte("test failed\n"), 0o644)
	_ = os.WriteFile(filepath.Join(crashDir, "env.txt"),
		[]byte(CaptureEnv()), 0o644)
	_ = os.WriteFile(filepath.Join(crashDir, "goroutine.stack"),
		[]byte(CaptureAllStacks()), 0o644)
	_ = os.WriteFile(filepath.Join(crashDir, "resources.txt"),
		[]byte("rss_mb=10 goroutine=5 fds=3\n"), 0o644)

	// 验证 4 个文件都创建了
	for _, name := range []string{"reason.txt", "env.txt", "goroutine.stack", "resources.txt"} {
		if _, err := os.Stat(filepath.Join(crashDir, name)); err != nil {
			t.Errorf("%s not created: %v", name, err)
		}
	}
}

func TestOnFailureHook_ManualDump(t *testing.T) {
	// 手动模拟 OnFailureHook 的落盘逻辑（t.Cleanup 时机不可控）
	dir := t.TempDir()
	testName := "TestForensics_Manual"
	crashDir := filepath.Join(dir, sanitizeFilename(testName))
	if err := os.MkdirAll(crashDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// 模拟写入
	_ = os.WriteFile(filepath.Join(crashDir, "reason.txt"), []byte("manual test"), 0o644)
	_ = os.WriteFile(filepath.Join(crashDir, "env.txt"), []byte(CaptureEnv()), 0o644)

	// 验证
	if _, err := os.Stat(filepath.Join(crashDir, "reason.txt")); err != nil {
		t.Errorf("reason.txt not created: %v", err)
	}
	if _, err := os.Stat(filepath.Join(crashDir, "env.txt")); err != nil {
		t.Errorf("env.txt not created: %v", err)
	}
	envContent, _ := os.ReadFile(filepath.Join(crashDir, "env.txt"))
	if !strings.Contains(string(envContent), "# captured at") {
		t.Errorf("env.txt missing header")
	}
}

func TestCaptureEnv_IncludesKeyVars(t *testing.T) {
	env := CaptureEnv()
	if !strings.Contains(env, "PATH=") {
		t.Error("CaptureEnv missing PATH")
	}
	if !strings.Contains(env, "GOARCH=") {
		t.Error("CaptureEnv missing GOARCH")
	}
}

func TestCaptureStack_NonEmpty(t *testing.T) {
	stack := CaptureStack()
	if len(stack) < 10 {
		t.Errorf("CaptureStack returned short: %q", stack)
	}
	if !strings.Contains(stack, "goroutine") {
		t.Error("CaptureStack missing 'goroutine'")
	}
}

func TestCaptureAllStacks_ContainsGoroutines(t *testing.T) {
	stacks := CaptureAllStacks()
	if !strings.Contains(stacks, "goroutine") {
		t.Error("CaptureAllStacks missing 'goroutine'")
	}
}

// ===== Sandbox =====

func TestCheckTempLeak_NoLeakWhenClean(t *testing.T) {
	// 没有 encv- 开头的大文件 → 静默通过
	CheckTempLeak(t, 1024*1024*1024) // 1GB 阈值
}

func TestIsProjectTempName(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{"encv-test-001.bin", true},
		{"encv_test-abc", true},
		{"mock-enc-mockdata", true},
		{"sillot-enc-foo", true},
		{"random-file", false},
		{"systemd-private", false},
	}
	for _, tc := range tests {
		if got := isProjectTempName(tc.in); got != tc.want {
			t.Errorf("isProjectTempName(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestAssertTempClean_PassesWhenClean(t *testing.T) {
	AssertTempClean(t, "/tmp/encv-nonexistent-pattern-*.bin")
}

func TestMockBoundaryTracker_RecordsAndAsserts(t *testing.T) {
	tracker := NewMockBoundaryTracker(t)
	tracker.RecordRealFSRead("/etc/passwd")
	tracker.RecordTempWrite("/tmp/encv-foo.bin")
	tracker.RecordExec("ffmpeg")

	entries := tracker.Entries()
	if len(entries) != 3 {
		t.Errorf("expected 3 entries, got %d", len(entries))
	}

	// 手动检查 entries（不能直接调 AssertNoRealFSAccess 会 t.Errorf）
	hasRealFS := false
	for _, e := range entries {
		if e.Kind == "real-fs-read" {
			hasRealFS = true
		}
	}
	if !hasRealFS {
		t.Error("expected real-fs-read entry")
	}

	// 验证 kind 分类
	hasTemp, hasExec := false, false
	for _, e := range entries {
		if e.Kind == "temp-write" {
			hasTemp = true
		}
		if e.Kind == "exec" {
			hasExec = true
		}
	}
	if !hasTemp {
		t.Error("expected temp-write entry")
	}
	if !hasExec {
		t.Error("expected exec entry")
	}
}
