// Package testutil/probe.go
// =====================================================
// 测试资源监控：RSS / Goroutine / FD 采样 + 硬限。
//
// 背景：之前「为什么沙箱 OOM」全靠猜；测试跨次累积导致资源耗尽。
// 本文件提供：
//   - Probe: 周期采样资源 + 超阈值 t.Fatal + t.Cleanup 报告
//   - StartProbe: 一行启用
//   - ResourceSnapshot: 一次性快照
//
// 用法：
//
//	func TestSomething(t *testing.T) {
//	    probe := testutil.StartProbe(t,
//	        testutil.WithMaxRSSMB(512),       // 512MB 上限
//	        testutil.WithMaxGoroutine(200),   // 200 个 goroutine 上限
//	    )
//	    defer probe.Snapshot()
//	    // ... 测试逻辑
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
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// ProbeOption Probe 配置选项
type ProbeOption func(*Probe)

// WithMaxRSSMB 设置 RSS 上限（MB）。默认 2048。
func WithMaxRSSMB(mb uint64) ProbeOption {
	return func(p *Probe) { p.maxRSSMB = mb }
}

// WithMaxGoroutine 设置 goroutine 数上限。默认 5000。
func WithMaxGoroutine(n int) ProbeOption {
	return func(p *Probe) { p.maxG = n }
}

// WithMaxFDs 设置 FD 数上限。默认 10000。
func WithMaxFDs(n int) ProbeOption {
	return func(p *Probe) { p.maxFds = n }
}

// WithInterval 设置采样间隔。默认 500ms。
func WithInterval(d time.Duration) ProbeOption {
	return func(p *Probe) { p.interval = d }
}

// WithReportDir 设置报告目录。默认 ReportDir()（自动发现项目根）。
func WithReportDir(dir string) ProbeOption {
	return func(p *Probe) { p.reportDir = dir }
}

// Probe 资源监控器
type Probe struct {
	t         *testing.T
	maxRSSMB  uint64
	maxG      int
	maxFds    int
	interval  time.Duration
	reportDir string

	startRSS  uint64
	startG    int
	startFds  int
	peakRSS   uint64
	peakG     int
	peakFds   int

	mu      sync.Mutex
	stopped bool
	stopCh  chan struct{}
	doneCh  chan struct{}
}

// StartProbe 启动资源监控（推荐用法）。
//
// 自动注册 t.Cleanup，测试结束自动写报告。
// 默认值：RSS 2GB、Goroutine 5000、FD 10000、interval 500ms。
func StartProbe(t *testing.T, opts ...ProbeOption) *Probe {
	t.Helper()
	p := &Probe{
		t:         t,
		maxRSSMB:  2048,
		maxG:      5000,
		maxFds:    10000,
		interval:  500 * time.Millisecond,
		reportDir: ReportDir(),
		stopCh:    make(chan struct{}),
		doneCh:    make(chan struct{}),
	}
	for _, o := range opts {
		o(p)
	}
	p.startRSS = readRSSBytes()
	p.startG = runtime.NumGoroutine()
	p.startFds = countFDs()
	p.peakRSS = p.startRSS
	p.peakG = p.startG
	p.peakFds = p.startFds

	go p.loop()
	t.Cleanup(p.finish)
	return p
}

// Snapshot 主动打一次快照（返回当前 delta 报告）。
// 通常 defer 调用一次即可。
func (p *Probe) Snapshot() ResourceDelta {
	p.t.Helper()
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.snapshotLocked()
}

func (p *Probe) snapshotLocked() ResourceDelta {
	return ResourceDelta{
		Name:           p.t.Name(),
		StartRSSMB:     bytesToMB(p.startRSS),
		EndRSSMB:       bytesToMB(p.peakRSS),
		RSSDeltaMB:     bytesToMB(p.peakRSS - p.startRSS),
		StartGoroutine: p.startG,
		EndGoroutine:   p.peakG,
		GoroutineDelta: p.peakG - p.startG,
		StartFDs:       p.startFds,
		EndFDs:         p.peakFds,
		FDsDelta:       p.peakFds - p.startFds,
	}
}

// ResourceDelta 资源 delta 报告
type ResourceDelta struct {
	Name           string `json:"name"`
	StartRSSMB     int    `json:"start_rss_mb"`
	EndRSSMB       int    `json:"end_rss_mb"`
	RSSDeltaMB     int    `json:"rss_delta_mb"`
	StartGoroutine int    `json:"start_goroutine"`
	EndGoroutine   int    `json:"end_goroutine"`
	GoroutineDelta int    `json:"goroutine_delta"`
	StartFDs       int    `json:"start_fds"`
	EndFDs         int    `json:"end_fds"`
	FDsDelta       int    `json:"fds_delta"`
}

func (p *Probe) loop() {
	defer close(p.doneCh)
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()
	for {
		select {
		case <-p.stopCh:
			return
		case <-ticker.C:
			p.tick()
		}
	}
}

func (p *Probe) tick() {
	rss := readRSSBytes()
	g := runtime.NumGoroutine()
	fds := countFDs()

	p.mu.Lock()
	defer p.mu.Unlock()

	if rss > p.peakRSS {
		p.peakRSS = rss
	}
	if g > p.peakG {
		p.peakG = g
	}
	if fds > p.peakFds {
		p.peakFds = fds
	}

	// 阈值检查
	if p.maxRSSMB > 0 && rss > p.maxRSSMB*1024*1024 {
		// 先落盘，再 fatal
		path := DumpStack("rss-limit",
			fmt.Sprintf("rss=%dMB > limit=%dMB", bytesToMB(rss), p.maxRSSMB))
		p.t.Fatalf("RSS limit exceeded: %dMB > %dMB (dump: %s)",
			bytesToMB(rss), p.maxRSSMB, path)
	}
	if p.maxG > 0 && g > p.maxG {
		p.t.Errorf("goroutine leak: %d > %d", g, p.maxG)
	}
	if p.maxFds > 0 && fds > p.maxFds {
		p.t.Errorf("fd leak: %d > %d", fds, p.maxFds)
	}
}

func (p *Probe) finish() {
	p.mu.Lock()
	if p.stopped {
		p.mu.Unlock()
		return
	}
	p.stopped = true
	p.mu.Unlock()

	// 停止 loop
	close(p.stopCh)
	<-p.doneCh

	// 写报告
	p.mu.Lock()
	delta := p.snapshotLocked()
	p.mu.Unlock()

	_ = os.MkdirAll(p.reportDir, 0o755)
	fname := filepath.Join(p.reportDir,
		fmt.Sprintf("probe-%s-%d.json", sanitizeFilename(p.t.Name()), time.Now().Unix()))
	data := []byte(fmt.Sprintf(
		`{"name":%q,"start_rss_mb":%d,"end_rss_mb":%d,"rss_delta_mb":%d,"start_goroutine":%d,"end_goroutine":%d,"goroutine_delta":%d,"start_fds":%d,"end_fds":%d,"fds_delta":%d}`,
		delta.Name, delta.StartRSSMB, delta.EndRSSMB, delta.RSSDeltaMB,
		delta.StartGoroutine, delta.EndGoroutine, delta.GoroutineDelta,
		delta.StartFDs, delta.EndFDs, delta.FDsDelta,
	))
	_ = os.WriteFile(fname, data, 0o644)
}

// ── 平台相关辅助 ──

// readRSSBytes 读取当前进程 RSS（字节）。
// Linux: /proc/self/statm 第二列 * pagesize
// 其他平台: 退化为 0
func readRSSBytes() uint64 {
	// 优先 /proc/self/statm（Linux 精确）
	if data, err := os.ReadFile("/proc/self/statm"); err == nil {
		fields := strings.Fields(string(data))
		if len(fields) >= 2 {
			pages, _ := strconv.ParseUint(fields[1], 10, 64)
			return pages * uint64(os.Getpagesize())
		}
	}
	// fallback: /proc/self/status VmRSS
	if data, err := os.ReadFile("/proc/self/status"); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(line, "VmRSS:") {
				parts := strings.Fields(line)
				if len(parts) >= 2 {
					kb, _ := strconv.ParseUint(parts[1], 10, 64)
					return kb * 1024
				}
			}
		}
	}
	// 跨平台：runtime.MemStats 不算 RSS 准确值，但能给个数量级
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	return ms.Sys
}

// countFDs 数 /proc/self/fd 下文件数。
// 非 Linux 返回 0。
func countFDs() int {
	entries, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		return 0
	}
	return len(entries)
}

func bytesToMB(b uint64) int {
	return int(b / 1024 / 1024)
}

// ReadRSSMB 公开方法：让用户在 t 外部也能拿 RSS（用于 debug）。
func ReadRSSMB() int {
	return bytesToMB(readRSSBytes())
}

// CountFDs 公开方法。
func CountFDs() int {
	return countFDs()
}
