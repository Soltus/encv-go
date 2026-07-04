// Package testutil/sandbox.go
// =====================================================
// 测试隔离性检测：临时文件泄漏 + 真实 fs 污染。
//
// 背景：测试跨次累积 /tmp 数十 GB；mock 写真实路径污染环境。
// 本文件提供：
//   - CheckTempLeak: 扫 /tmp 看是否有大文件
//   - AssertTempClean: 配合 CleanupOnExit 验证清理生效
//   - MockBoundaryTracker: 记录 mock 走的真实路径（mock 污染治理用）
//
// 用法（在测试主入口或 cleanup 时调）：
//
//	defer testutil.CheckTempLeak(t, 100*1024*1024) // 100MB 阈值
//
// 2026-06-15 创建（test-architecture-refactor-defense-awareness Sprint 2）
// =====================================================

package testutil

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// defaultTempLeakThreshold /tmp 大文件阈值（默认 100MB）
const defaultTempLeakThreshold = 100 * 1024 * 1024

// defaultTempScanRoots 扫描根目录
var defaultTempScanRoots = []string{"/tmp"}

// CheckTempLeak 扫常见临时目录，超过阈值的文件视为泄漏。
// 用 t.Logf 报告，不 t.Fatal（因为这通常不是 test 自己的错）。
//
// 用法（在 t.Cleanup 中调用）：
//
//	t.Cleanup(func() { testutil.CheckTempLeak(t, 100*1024*1024) })
//
// 阈值 0 表示用默认值 100MB。
func CheckTempLeak(t *testing.T, threshold int64) {
	t.Helper()
	if threshold <= 0 {
		threshold = defaultTempLeakThreshold
	}

	var leaks []string
	for _, root := range defaultTempScanRoots {
		entries, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		for _, e := range entries {
			// 只关心本项目前缀
			name := e.Name()
			if !isProjectTempName(name) {
				continue
			}
			info, err := e.Info()
			if err != nil {
				continue
			}
			if info.Size() >= threshold {
				leaks = append(leaks, fmt.Sprintf("%s/%s (%dMB)",
					root, name, info.Size()/1024/1024))
			}
		}
	}

	if len(leaks) > 0 {
		t.Logf("[TEMP-LEAK] %d files exceed threshold %dMB: %v",
			len(leaks), threshold/1024/1024, leaks)
	}
}

// isProjectTempName 判断是否是本项目测试产生的临时文件。
// 命名规则：encv-*, encv_test-*, mock-enc-*
func isProjectTempName(name string) bool {
	prefixes := []string{"encv-", "encv_test-", "mock-enc-", "sillot-enc-"}
	for _, p := range prefixes {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	return false
}

// AssertTempClean 断言所有已知 pattern 在 t.Cleanup 后已被清理。
// 用法：
//
//	testutil.CleanupOnExit(t, "/tmp/encv-test-*.bin")
//	t.Cleanup(func() { testutil.AssertTempClean(t, "/tmp/encv-test-*.bin") })
func AssertTempClean(t *testing.T, patterns ...string) {
	t.Helper()
	for _, p := range patterns {
		matches, err := filepath.Glob(p)
		if err != nil {
			t.Errorf("[AssertTempClean] invalid glob %q: %v", p, err)
			continue
		}
		if len(matches) > 0 {
			t.Errorf("[AssertTempClean] expected no files matching %q, found %d: %v",
				p, len(matches), matches)
		}
	}
}

// MockBoundaryTracker 记录 mock 调用时走的"真实路径"（污染检测）。
//
// 背景：mock 应该走 t.TempDir() 或注入的 stub。直接读写 /workspace 或 /tmp
// 是污染行为，需要在 Sprint 3 整改。
//
// 用法：
//
//	tracker := testutil.NewMockBoundaryTracker(t)
//	// 任何 mock 操作记录：
//	tracker.RecordRealFSRead("/etc/passwd")  // 不应发生
//	tracker.RecordTempWrite("/workspace/real-data.txt")  // 不应发生
//
//	t.Cleanup(func() { tracker.AssertNoRealFSAccess() })
type MockBoundaryTracker struct {
	t       *testing.T
	mu      sync.Mutex
	entries []mockEntry
}

type mockEntry struct {
	Kind     string // "real-fs-read", "real-fs-write", "temp-write", "exec"
	Path     string
	CallerFn string
}

// NewMockBoundaryTracker 创建 mock 边界追踪器
func NewMockBoundaryTracker(t *testing.T) *MockBoundaryTracker {
	return &MockBoundaryTracker{t: t}
}

// RecordRealFSRead 记录一次"读真实 fs"操作。
// 用于检测 mock 是否绕过 t.TempDir()。
func (m *MockBoundaryTracker) RecordRealFSRead(path string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entries = append(m.entries, mockEntry{Kind: "real-fs-read", Path: path})
}

// RecordRealFSWrite 同上，写。
func (m *MockBoundaryTracker) RecordRealFSWrite(path string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entries = append(m.entries, mockEntry{Kind: "real-fs-write", Path: path})
}

// RecordTempWrite 记录一次"写 /tmp"操作（应改为 t.TempDir）。
func (m *MockBoundaryTracker) RecordTempWrite(path string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entries = append(m.entries, mockEntry{Kind: "temp-write", Path: path})
}

// RecordExec 记录一次"exec 真实命令"操作（应改为 in-process stub）。
func (m *MockBoundaryTracker) RecordExec(cmd string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entries = append(m.entries, mockEntry{Kind: "exec", Path: cmd})
}

// AssertNoRealFSAccess 断言没有"读 /workspace 真实数据"行为。
// 注意：写 /tmp 单独可允许（pre-flight 会清）。
func (m *MockBoundaryTracker) AssertNoRealFSAccess() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, e := range m.entries {
		if e.Kind == "real-fs-read" || e.Kind == "real-fs-write" {
			m.t.Errorf("[MockBoundaryTracker] %s on %s — should use t.TempDir() or stub",
				e.Kind, e.Path)
		}
	}
}

// Entries 返回所有记录（用于报告）。
func (m *MockBoundaryTracker) Entries() []mockEntry {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]mockEntry, len(m.entries))
	copy(out, m.entries)
	return out
}
