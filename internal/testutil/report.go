// Package testutil/report.go
// =====================================================
// 测试结构化报告：每条 test 的 duration / status / resource 摘要 → JSON。
//
// 背景：之前失败时只能看 stderr 文本，无法横向比较、无法画趋势。
// 本文件提供：
//   - TestReport: 一条测试的完整报告
//   - TestReporter: 全局 reporter，自动收集（通过 t.Cleanup 注册）
//   - WriteReport: 把当前所有 report 合并到一份 JSON
//
// 2026-06-15 创建（test-architecture-refactor-defense-awareness Sprint 1）
// =====================================================

package testutil

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// TestStatus 测试结果状态
type TestStatus string

const (
	StatusPass     TestStatus = "pass"
	StatusFail     TestStatus = "fail"
	StatusSkip     TestStatus = "skip"
	StatusPanic    TestStatus = "panic"
	StatusTimeout  TestStatus = "timeout"
	StatusAborted  TestStatus = "aborted"
)

// TestReport 一条测试的完整报告
type TestReport struct {
	Name           string     `json:"name"`
	Package        string     `json:"package"`
	Status         TestStatus `json:"status"`
	DurationMS     int64      `json:"duration_ms"`
	StartTime      string     `json:"start_time"`
	EndTime        string     `json:"end_time"`
	RSS_MB_Peak    int        `json:"rss_mb_peak,omitempty"`
	Goroutine_Peak int        `json:"goroutine_peak,omitempty"`
	ErrorMsg       string     `json:"error_msg,omitempty"`
	StackFile      string     `json:"stack_file,omitempty"`
}

// Reporter 全局测试报告收集器。
// 一个 binary 进程共享一个 reporter，结束时由 FinalizeAll 写出。
type Reporter struct {
	mu      sync.Mutex
	reports []TestReport
	dir     string
}

// 默认全局 reporter
var defaultReporter = &Reporter{}

// ReportDir 返回报告输出目录。
// 自动发现项目根（含 go.mod 的目录），这样从子包跑 go test 也能正确写入。
// 可通过 ENCV_TEST_REPORT_DIR 强制覆盖。
func ReportDir() string {
	if d := os.Getenv("ENCV_TEST_REPORT_DIR"); d != "" {
		return d
	}
	if cached := reportDirCache; cached != "" {
		return cached
	}
	cwd, _ := os.Getwd()
	root := findProjectRoot(cwd)
	if root == "" {
		root = cwd
	}
	reportDirCache = filepath.Join(root, ".test-runs")
	return reportDirCache
}

var reportDirCache string

// findProjectRoot 从 start 向上找含 go.mod 的目录。
func findProjectRoot(start string) string {
	dir := start
	for i := 0; i < 10; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
	return ""
}

// record 内部记录一条报告。
func (r *Reporter) record(report TestReport) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.reports = append(r.reports, report)
}

// Mark 自动根据 t 的状态生成 TestReport 并记录。
// 推荐用法：defer testutil.Mark(t)() —— 立即记录开始时间，defer 触发时记录结束。
func Mark(t *testing.T) func() {
	t.Helper()
	start := time.Now()
	return func() {
		end := time.Now()
		status := StatusPass
		errMsg := ""
		if t.Skipped() {
			status = StatusSkip
		} else if t.Failed() {
			status = StatusFail
		}
		defaultReporter.record(TestReport{
			Name:       t.Name(),
			Status:     status,
			DurationMS: end.Sub(start).Milliseconds(),
			StartTime:  start.Format(time.RFC3339Nano),
			EndTime:    end.Format(time.RFC3339Nano),
			ErrorMsg:   errMsg,
		})
	}
}

// MarkFailure 不依赖 t.Failed()，显式记录一条失败。
// 用于 SafeGo / 资源超限等特殊场景。
func MarkFailure(t *testing.T, status TestStatus, msg string) {
	t.Helper()
	defaultReporter.record(TestReport{
		Name:      t.Name(),
		Status:    status,
		ErrorMsg:  msg,
		StartTime: time.Now().Format(time.RFC3339Nano),
		EndTime:   time.Now().Format(time.RFC3339Nano),
	})
}

// FinalizeAll 把所有 report 写到 <dir>/reports-<ts>.json。
// 通常由 test-go.sh 在 go test 结束后调用（通过 -test.run trick）。
// 在 test 内部调用：testutil.FinalizeAll(t)
func FinalizeAll(t *testing.T) {
	t.Helper()
	if err := defaultReporter.flush(); err != nil {
		t.Logf("[FinalizeAll] flush: %v", err)
	}
}

// flush 把内存中所有 report 写盘。
// 文件路径：<ReportDir()>/reports-<ts>.json
func (r *Reporter) flush() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.reports) == 0 {
		return nil
	}
	dir := r.dir
	if dir == "" {
		dir = ReportDir()
	}
	if dir == "" {
		return fmt.Errorf("flush: report dir is empty (cwd=%s)", mustGetwdReport())
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	fname := filepath.Join(dir, "reports-"+time.Now().Format("20060102-150405")+".json")
	data, err := json.MarshalIndent(r.reports, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(fname, data, 0o644)
}

func mustGetwdReport() string {
	wd, _ := os.Getwd()
	return wd
}

// Reset 清空所有 report（用于多次 FinalizeAll 之间）。
func Reset() {
	defaultReporter.mu.Lock()
	defaultReporter.reports = nil
	defaultReporter.mu.Unlock()
}

// Summarize 返回当前内存中所有 report 的统计摘要。
func Summarize() (total, passed, failed, skipped int) {
	defaultReporter.mu.Lock()
	defer defaultReporter.mu.Unlock()
	for _, r := range defaultReporter.reports {
		total++
		switch r.Status {
		case StatusPass:
			passed++
		case StatusFail, StatusPanic, StatusTimeout, StatusAborted:
			failed++
		case StatusSkip:
			skipped++
		}
	}
	return
}
