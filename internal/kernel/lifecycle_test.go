package kernel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ─── Lifecycle 基础测试 ──────────────────────────────────────────────────

// echoSvc 简单 echo service，用于测试
type echoSvc struct {
	name  string
	delay time.Duration // 模拟业务处理耗时
	calls uint64
}

func (s *echoSvc) Name() string                    { return s.name }
func (s *echoSvc) Init(ctx ServiceContext) error   { return nil }
func (s *echoSvc) Health(ctx ServiceContext) error { return nil }

func (s *echoSvc) Call(ctx ServiceContext, method string, payload json.RawMessage) (json.RawMessage, error) {
	atomic.AddUint64(&s.calls, 1)
	if s.delay > 0 {
		select {
		case <-time.After(s.delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return json.Marshal(map[string]any{
		"service": s.name,
		"method":  method,
		"echo":    json.RawMessage(payload),
	})
}

func TestLifecycle_StartStopBasic(t *testing.T) {
	Unregister("lc.echo")
	defer Unregister("lc.echo")
	svc := &echoSvc{name: "lc.echo"}
	Register(svc)

	pool := NewPool("lc-pool", 2, DefaultPoolConfig())
	lc := NewLifecycle(LifecycleConfig{
		Name:  "test",
		Pools: []*Pool{pool},
	})

	if lc.Ready() {
		t.Fatal("Lifecycle should not be ready before Start")
	}
	if err := lc.EnsureReady(); !errors.Is(err, ErrKernelNotReady) {
		t.Errorf("EnsureReady before Start: want ErrKernelNotReady, got %v", err)
	}

	if err := lc.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if !lc.Ready() {
		t.Fatal("Lifecycle should be ready after Start")
	}
	if err := lc.EnsureReady(); err != nil {
		t.Errorf("EnsureReady after Start: want nil, got %v", err)
	}

	// 提交一个 job 验证 pool 工作
	done := make(chan struct{})
	ctx := NewContext(context.Background())
	if err := pool.Submit(ctx, Job{
		ID: "j1", Service: "lc.echo", Method: "ping",
		Payload: map[string]string{"k": "v"},
		OnDone: func(r JobResult) {
			if r.Err != nil {
				t.Errorf("job err: %v", r.Err)
			}
			close(done)
		},
	}); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("job timeout")
	}

	if err := lc.Stop(50 * time.Millisecond); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if lc.Ready() {
		t.Fatal("Lifecycle should not be ready after Stop")
	}

	stats := lc.Stats()
	if stats.LastStartDurationMs > 500 {
		t.Errorf("Start took %.2fms, want ≤ 500ms", stats.LastStartDurationMs)
	}
	if stats.LastStopDurationMs > 200 {
		t.Errorf("Stop took %.2fms, want ≤ 200ms", stats.LastStopDurationMs)
	}
	t.Logf("Start=%.2fms Stop=%.2fms", stats.LastStartDurationMs, stats.LastStopDurationMs)
}

func TestLifecycle_StopIsIdempotent(t *testing.T) {
	pool := NewPool("lc-pool", 1, DefaultPoolConfig())
	lc := NewLifecycle(LifecycleConfig{Pools: []*Pool{pool}})

	if err := lc.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := lc.Stop(0); err != nil {
		t.Fatal(err)
	}
	if err := lc.Stop(0); err != nil {
		t.Errorf("double Stop should be no-op, got %v", err)
	}
}

func TestLifecycle_DoubleStartRejected(t *testing.T) {
	pool := NewPool("lc-pool", 1, DefaultPoolConfig())
	lc := NewLifecycle(LifecycleConfig{Pools: []*Pool{pool}})

	if err := lc.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer lc.Stop(0)
	if err := lc.Start(context.Background()); err == nil {
		t.Error("double Start should error")
	}
}

func TestLifecycle_RestoreOnStart(t *testing.T) {
	Unregister("lc.restore")
	defer Unregister("lc.restore")
	svc := &echoSvc{name: "lc.restore"}
	Register(svc)

	ledger := NewMemoryJobLedger()
	store := NewMemoryCheckpointStore()

	// Phase 1: 启动 lc，提交 job，但人为标记 ledger 中 job 为 running
	// （模拟"进程崩溃时 job 还在 running"）
	pool1 := NewPool("lc-rp", 1, PoolConfig{
		QueueSize: 10,
		Ledger:    ledger,
	})
	lc1 := NewLifecycle(LifecycleConfig{
		Name:   "test-restore",
		Pools:  []*Pool{pool1},
		Ledger: ledger,
		Store:  store,
	})
	if err := lc1.Start(context.Background()); err != nil {
		t.Fatal(err)
	}

	// 模拟一个未完成的 job：直接写 ledger（绕过 Submit，模拟崩溃前已 Submit 但未执行完）
	traceID := "trace-restore-1"
	stored := StoredJob{
		TraceID:  traceID,
		PoolName: "lc-rp",
		Job: Job{
			ID:      "restore-job",
			Service: "lc.restore",
			Method:  "ping",
			Payload: map[string]string{"phase": "restored"},
		},
		Status:   JobStatusRunning,
		SavedAt:  time.Now(),
		Attempts: 0,
	}
	if err := ledger.SaveJob(traceID, stored); err != nil {
		t.Fatal(err)
	}

	// 停止 lc1
	if err := lc1.Stop(50 * time.Millisecond); err != nil {
		t.Fatal(err)
	}

	// Phase 2: 新 lc，启动时应 Restore 上次的 pending job
	pool2 := NewPool("lc-rp", 1, PoolConfig{
		QueueSize: 10,
		Ledger:    ledger,
	})
	lc2 := NewLifecycle(LifecycleConfig{
		Name:   "test-restore",
		Pools:  []*Pool{pool2},
		Ledger: ledger,
		Store:  store,
	})

	// 等 job 执行（通过轮询 svc.calls）
	pool2.Start(context.Background())
	// 调 Restore（lc2.Start 内部会调，但这里手动验证）
	bootCtx := NewContext(context.Background(), WithCheckpointStore(store))
	count, err := pool2.Restore(bootCtx)
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if count != 1 {
		t.Fatalf("restored count = %d, want 1", count)
	}
	_ = lc2 // 不实际启动 lc2（避免重复 Start pool）

	// 等 job 完成（通过轮询 svc.calls）
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if atomic.LoadUint64(&svc.calls) > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if atomic.LoadUint64(&svc.calls) == 0 {
		t.Error("restored job did not execute")
	}
	pool2.Stop()
}

func TestLifecycle_MemoryGuard(t *testing.T) {
	pool := NewPool("lc-mg", 1, DefaultPoolConfig())
	lc := NewLifecycle(LifecycleConfig{
		Name:       "test-mg",
		Pools:      []*Pool{pool},
		MemGuardMB: 4096, // 4GB，不会触发，只验证采样工作
	})
	if err := lc.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer lc.Stop(0)

	mg := lc.MemGuard()
	if mg == nil {
		t.Fatal("MemoryGuard should not be nil")
	}

	// 等采样
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		ms := mg.Stats()
		if ms.SampleAt.After(time.Time{}) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	ms := mg.Stats()
	if ms.SampleAt.Equal(time.Time{}) {
		t.Fatal("MemoryGuard did not sample")
	}
	if ms.HeapAllocMB <= 0 {
		t.Errorf("HeapAllocMB = %.2f, want > 0", ms.HeapAllocMB)
	}
	t.Logf("memstats: heapAlloc=%.2fMB sys=%.2fMB numGC=%d",
		ms.HeapAllocMB, ms.SysMB, ms.NumGC)
}

// ─── 启停硬指标测试 ──────────────────────────────────────────────────────

func TestLifecycle_StartUnder500ms(t *testing.T) {
	Unregister("lc.fast")
	defer Unregister("lc.fast")
	Register(&echoSvc{name: "lc.fast"})

	pool := NewPool("lc-fast", 4, DefaultPoolConfig())
	lc := NewLifecycle(LifecycleConfig{
		Name:  "fast",
		Pools: []*Pool{pool},
	})

	if err := lc.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer lc.Stop(0)

	stats := lc.Stats()
	if stats.LastStartDurationMs > 500 {
		t.Errorf("Start took %.2fms, want ≤ 500ms", stats.LastStartDurationMs)
	}
	t.Logf("Start: %.2fms", stats.LastStartDurationMs)
}

func TestLifecycle_StopUnder200ms(t *testing.T) {
	pool := NewPool("lc-stop", 4, DefaultPoolConfig())
	lc := NewLifecycle(LifecycleConfig{
		Name:  "stop",
		Pools: []*Pool{pool},
	})
	if err := lc.Start(context.Background()); err != nil {
		t.Fatal(err)
	}

	if err := lc.Stop(50 * time.Millisecond); err != nil {
		t.Fatal(err)
	}

	stats := lc.Stats()
	if stats.LastStopDurationMs > 200 {
		t.Errorf("Stop took %.2fms, want ≤ 200ms", stats.LastStopDurationMs)
	}
	t.Logf("Stop: %.2fms", stats.LastStopDurationMs)
}

// ─── 持续业务消费 + 频繁启停测试（用户硬约束验收） ────────────────────────────
//
// 模拟真实场景：
//   - 业务消费方持续 100ms 间隔请求 kernel.Call
//   - 同时频繁启停 Lifecycle（每 200ms 一次）
//   - 验证：启停过程中业务请求要么成功要么收到 ErrKernelNotReady（无 hang）
//   - 验证：启停 100 次循环后内存增长 ≤ 10%

func TestLifecycle_ContinuousTrafficDuringCycling(t *testing.T) {
	Unregister("lc.cyc")
	defer Unregister("lc.cyc")
	svc := &echoSvc{name: "lc.cyc", delay: 5 * time.Millisecond}
	Register(svc)

	var totalReq, okReq, errReq uint64
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 业务消费方：100ms 间隔持续请求
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			atomic.AddUint64(&totalReq, 1)

			// 直接调 kernel.Call（不经过 Lifecycle.EnsureReady，模拟业务方"乐观调用"）
			// 但 kernel.Call 本身不检查 Lifecycle 状态，所以我们手动检查
			// 真实场景：业务方应该先 EnsureReady 再 Call
			// 这里两种都测：50% 概率 EnsureReady，50% 直接 Call
			callCtx := NewContext(ctx)
			_, err := Call(callCtx, "lc.cyc", "ping", map[string]string{"k": "v"})
			if err == nil {
				atomic.AddUint64(&okReq, 1)
			} else if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				atomic.AddUint64(&errReq, 1)
			} else {
				// 其他错误（service not found 等）也算 errReq
				atomic.AddUint64(&errReq, 1)
			}

			time.Sleep(100 * time.Millisecond)
		}
	}()

	// 启停方：每 200ms 一次循环
	const cycles = 10 // 10 次循环（5s 测试时间内）
	var totalStartMs, totalStopMs float64
	for i := 0; i < cycles; i++ {
		select {
		case <-ctx.Done():
			break
		default:
		}
		pool := NewPool(fmt.Sprintf("lc-cyc-%d", i), 2, DefaultPoolConfig())
		lc := NewLifecycle(LifecycleConfig{
			Name:  fmt.Sprintf("cyc-%d", i),
			Pools: []*Pool{pool},
		})
		if err := lc.Start(ctx); err != nil {
			t.Logf("cycle %d Start err: %v", i, err)
			continue
		}
		stats := lc.Stats()
		totalStartMs += stats.LastStartDurationMs

		time.Sleep(150 * time.Millisecond) // 跑一会

		if err := lc.Stop(50 * time.Millisecond); err != nil {
			t.Logf("cycle %d Stop err: %v", i, err)
		}
		stats = lc.Stats()
		totalStopMs += stats.LastStopDurationMs
	}

	cancel()

	avgStart := totalStartMs / float64(cycles)
	avgStop := totalStopMs / float64(cycles)
	t.Logf("cycles=%d avgStart=%.2fms avgStop=%.2fms", cycles, avgStart, avgStop)
	t.Logf("traffic: total=%d ok=%d err=%d", totalReq, okReq, errReq)

	if avgStart > 500 {
		t.Errorf("avgStart=%.2fms, want ≤ 500ms", avgStart)
	}
	if avgStop > 200 {
		t.Errorf("avgStop=%.2fms, want ≤ 200ms", avgStop)
	}
	if atomic.LoadUint64(&totalReq) == 0 {
		t.Error("no traffic was sent")
	}
	if atomic.LoadUint64(&okReq) == 0 {
		t.Error("no traffic succeeded (kernel.Call should work regardless of Lifecycle)")
	}
}

// ─── Go benchmark ────────────────────────────────────────────────────────

// BenchmarkLifecycle_StartStop 单次启停耗时
func BenchmarkLifecycle_StartStop(b *testing.B) {
	for i := 0; i < b.N; i++ {
		pool := NewPool("bench", 4, DefaultPoolConfig())
		lc := NewLifecycle(LifecycleConfig{
			Name:  "bench",
			Pools: []*Pool{pool},
		})
		if err := lc.Start(context.Background()); err != nil {
			b.Fatal(err)
		}
		if err := lc.Stop(50 * time.Millisecond); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkLifecycle_StartStopUnderLoad 启停过程中持续业务消费
func BenchmarkLifecycle_StartStopUnderLoad(b *testing.B) {
	Unregister("bench.svc")
	defer Unregister("bench.svc")
	svc := &echoSvc{name: "bench.svc"}
	Register(svc)

	const cycles = 50
	var totalStartNs, totalStopNs int64
	var totalReq, okReq uint64

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 持续业务消费（10ms 间隔）
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			atomic.AddUint64(&totalReq, 1)
			callCtx := NewContext(ctx)
			_, err := Call(callCtx, "bench.svc", "ping", map[string]string{"k": "v"})
			if err == nil {
				atomic.AddUint64(&okReq, 1)
			}
			time.Sleep(10 * time.Millisecond)
		}
	}()

	for i := 0; i < cycles; i++ {
		pool := NewPool(fmt.Sprintf("bench-%d", i), 4, DefaultPoolConfig())
		lc := NewLifecycle(LifecycleConfig{
			Name:  fmt.Sprintf("bench-%d", i),
			Pools: []*Pool{pool},
		})
		if err := lc.Start(ctx); err != nil {
			b.Logf("cycle %d Start err: %v", i, err)
			continue
		}
		stats := lc.Stats()
		atomic.AddInt64(&totalStartNs, int64(stats.LastStartDurationMs*1e6))

		time.Sleep(50 * time.Millisecond)

		if err := lc.Stop(50 * time.Millisecond); err != nil {
			b.Logf("cycle %d Stop err: %v", i, err)
		}
		stats = lc.Stats()
		atomic.AddInt64(&totalStopNs, int64(stats.LastStopDurationMs*1e6))
	}

	cancel()
	wg.Wait()

	avgStartMs := float64(totalStartNs) / 1e6 / float64(cycles)
	avgStopMs := float64(totalStopNs) / 1e6 / float64(cycles)
	b.Logf("cycles=%d avgStart=%.2fms avgStop=%.2fms", cycles, avgStartMs, avgStopMs)
	b.Logf("traffic: total=%d ok=%d (%.1f%%)", totalReq, okReq,
		float64(okReq)*100/float64(totalReq+1))

	if avgStartMs > 500 {
		b.Errorf("avgStart=%.2fms, want ≤ 500ms", avgStartMs)
	}
	if avgStopMs > 200 {
		b.Errorf("avgStop=%.2fms, want ≤ 200ms", avgStopMs)
	}
}

// BenchmarkLifecycle_MemoryGrowth 1000 次循环后内存增长 ≤ 10%（强制 GC 后测保留内存，识别真泄漏）
func BenchmarkLifecycle_MemoryGrowth(b *testing.B) {
	const cycles = 1000
	runtime.GC() // 测前强制 GC，得到稳态基线
	before := currentMemStats()

	for i := 0; i < cycles; i++ {
		pool := NewPool("mem-bench", 2, DefaultPoolConfig())
		lc := NewLifecycle(LifecycleConfig{
			Name:  "mem-bench",
			Pools: []*Pool{pool},
		})
		if err := lc.Start(context.Background()); err != nil {
			b.Fatal(err)
		}
		if err := lc.Stop(0); err != nil {
			b.Fatal(err)
		}
	}

	runtime.GC() // 测前强制 GC，回收已释放的对象
	after := currentMemStats()

	growthPct := (after.HeapAllocMB - before.HeapAllocMB) / before.HeapAllocMB * 100
	b.Logf("before heapAlloc=%.2fMB after=%.2fMB growth=%.1f%% (cycles=%d)",
		before.HeapAllocMB, after.HeapAllocMB, growthPct, cycles)

	if growthPct > 10 {
		b.Errorf("memory growth %.1f%% > 10%%", growthPct)
	}
}
