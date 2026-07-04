// Package testutil/demo_test.go
// =====================================================
// 演示：完整跑通 Mark → 报告 → 资源监控 → 失败落盘 全链路。
//
// 目的：让 scripts/test-all-go.sh 的 report-all.json 真实有数据。
// 同时作为这套基础设施的"集成测试"。
// =====================================================

package testutil

import (
	"fmt"
	"os"
	"runtime"
	"testing"
	"time"
)

// TestDemo_FullPipeline_Pass 演示：测试通过 + 资源监控 + Mark 报告
func TestDemo_FullPipeline_Pass(t *testing.T) {
	defer Mark(t)()
	OnFailureHook(t)
	probe := StartProbe(t, WithInterval(100*time.Millisecond))
	_ = probe

	// 模拟一些工作
	time.Sleep(200 * time.Millisecond)
	for i := 0; i < 1000; i++ {
		_ = make([]byte, 1024)
	}
	t.Logf("cwd=%s ReportDir=%s", mustGetwd(), ReportDir())
}

func mustGetwd() string {
	wd, _ := os.Getwd()
	return wd
}

// TestDemo_FullPipeline_ResourceChange 演示：资源变化被记录
func TestDemo_FullPipeline_ResourceChange(t *testing.T) {
	defer Mark(t)()
	probe := StartProbe(t, WithInterval(50*time.Millisecond))

	// 分配 30MB 内存
	mem := make([]byte, 30*1024*1024)
	for i := range mem {
		mem[i] = byte(i)
	}
	_ = mem
	runtime.GC()
	time.Sleep(200 * time.Millisecond)
	delta := probe.Snapshot()
	t.Logf("RSS: %dMB → %dMB (delta=%dMB), Goroutine: %d → %d",
		delta.StartRSSMB, delta.EndRSSMB, delta.RSSDeltaMB,
		delta.StartGoroutine, delta.EndGoroutine)
}

// TestDemo_FullPipeline_Report 演示：FinalizeAll 写出 JSON
func TestDemo_FullPipeline_Report(t *testing.T) {
	defer Mark(t)()
	MarkFailure(t, StatusPass, "demo pass")
	MarkFailure(t, StatusFail, "demo fail")
	MarkFailure(t, StatusSkip, "")
	MarkFailure(t, StatusTimeout, "demo timeout")
	total, passed, failed, skipped := Summarize()
	if total < 4 {
		t.Errorf("expected total >= 4, got %d", total)
	}
	t.Logf("summary: total=%d passed=%d failed=%d skipped=%d", total, passed, failed, skipped)
	FinalizeAll(t)
}

// TestDemo_FullPipeline_Stack 演示：CaptureAllStacks + DumpStack
func TestDemo_FullPipeline_Stack(t *testing.T) {
	defer Mark(t)()
	stacks := CaptureAllStacks()
	if len(stacks) < 100 {
		t.Errorf("expected non-trivial stacks, got %d bytes", len(stacks))
	}
	t.Logf("captured %d bytes of goroutine stacks", len(stacks))
	// 同时落盘一份
	path := DumpStack("demo", fmt.Sprintf("manual dump %d bytes", len(stacks)))
	if path == "" {
		t.Error("DumpStack returned empty")
	} else {
		t.Logf("dump written: %s", path)
	}
}
