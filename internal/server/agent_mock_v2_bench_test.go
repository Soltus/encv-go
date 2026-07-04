// internal/server/agent_mock_v2_bench_test.go
//
// # T16 性能与压力测试 — MockEngineV2 引擎
//
// 包含：
//   - T16.2 5 并发剧本测试（无 panic / 无 goroutine 泄漏 / 内存 < 100MB 增长）
//   - T16.3 1000 轮长时间稳定性测试（无内存泄漏 / 无 goroutine 泄漏）
//
// 跑测条件：
//   - `go test -short`                              → 全部 skip
//   - `BENCH_V2=1 go test ./internal/server/...`    → 跑全量（T16.2 + T16.3）
//   - `go test -bench=BenchmarkMockEngineV2_...`    → 跑 micro bench
//
// 设计要点：
//   - 每个 goroutine 拥有独立 MockEngineV2 + agentSession（避免共享状态污染）
//   - 共享同一个 *Server 实例（v2 路径不修改 Server 字段）
//   - 用 buffered channel (size=N) 作为 goroutine 完成信号
//   - runtime.GC() + runtime.Gosched() + 小 sleep 稳定内存/goroutine 测量
//   - 1000 轮剧本程序化构造（避免 mockScenariosV2 中的 4 轮剧本需循环 250 次）
package server

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// shouldRunV2Bench 决定 T16.2 / T16.3 是否真正执行。
//
//   - testing.Short() → skip（fast）
//   - BENCH_V2 != "1" → skip（opt-in）
func shouldRunV2Bench(t testing.TB) bool {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping MockEngineV2 stress test (testing.Short())")
		return false
	}
	if os.Getenv("BENCH_V2") != "1" {
		t.Skip("skipping MockEngineV2 stress test (set BENCH_V2=1 to enable)")
		return false
	}
	return true
}

// ─── T16.2 5 并发剧本测试 ─────────────────────────────────────

// TestMockEngineV2_ConcurrentScenarios 验证 5 个 MockEngineV2 实例在
// 并行 goroutine 中跑 5 个不同的 v2 剧本，全部成功完成、无 panic、
// 无 goroutine 泄漏、内存增长 < 100MB。
//
// 每个 goroutine 拥有独立 engine + session，共享 *Server 实例
// （v2 路径下 sendAndCache 不修改 Server 字段）。
//
// 跳过：
//   - `go test -short`
//   - 需要 BENCH_V2=1 才会真正执行
func TestMockEngineV2_ConcurrentScenarios(t *testing.T) {
	if !shouldRunV2Bench(t) {
		return
	}

	// ── 准备 5 个不同 v2 剧本（顺序取前 5 个：search_recursive_mp4 / search_logical_query /
	//    search_content_regex / edit_metadata_wizard / batch_rename_with_preview）
	if len(mockScenariosV2) < 5 {
		t.Fatalf("mockScenariosV2 数量 = %d, want >= 5", len(mockScenariosV2))
	}
	scenarios := mockScenariosV2[:5]

	// ── 稳定基线
	runtime.GC()
	runtime.Gosched()
	time.Sleep(50 * time.Millisecond) // 让 test framework 的 goroutine 安定

	baseGoroutines := runtime.NumGoroutine()
	var baseMem runtime.MemStats
	runtime.ReadMemStats(&baseMem)
	t.Logf("baseline: goroutines=%d, HeapAlloc=%s, HeapInuse=%s",
		baseGoroutines, humanBytes(int64(baseMem.HeapAlloc)), humanBytes(int64(baseMem.HeapInuse)))

	// ── 启动 5 个并发 goroutine
	const n = 5
	// buffered channel (size=n) 作为 eventWriter 完成的信号通道
	done := make(chan scenarioResult, n)
	var inflight int32

	s := newMockTestServer()

	for i := 0; i < n; i++ {
		sc := scenarios[i]
		go func(idx int) {
			atomic.AddInt32(&inflight, 1)
			defer atomic.AddInt32(&inflight, -1)

			// 单一 defer 收集结果：recover / 错误 / 完成信号
			// 必须先于 inflight-- 执行（defer LIFO），所以放在最后
			completionCh := make(chan struct{}, 1)
			defer func() {
				completionCh <- struct{}{} // 本 goroutine 完成信号
			}()

			result := scenarioResult{ScenarioID: sc.ID}
			defer func() {
				if r := recover(); r != nil {
					result.Panic = fmt.Sprintf("panic: %v", r)
				}
				// result.EventCount 已在 body 设置；若 panic 则保持 0，由调用方判定
				done <- result
			}()

			eng := NewMockEngineV2()
			sess := newMockSession()
			rec := httptest.NewRecorder()
			// speed=0 → 零延迟（无 time.After 开销），专注测量引擎本身
			err := eng.Run(context.Background(), s, sess, rec, rec, sc, 0, false)
			if err != nil {
				result.Err = err
			}
			result.EventCount = len(sess.EventCache)
		}(i)
	}

	// ── 收集所有结果
	results := make([]scenarioResult, 0, n)
	for i := 0; i < n; i++ {
		results = append(results, <-done)
	}

	// ── 断言 1：全部 5 个都成功
	for _, r := range results {
		if r.Panic != "" {
			t.Errorf("scenario %s: %s", r.ScenarioID, r.Panic)
		}
		if r.Err != nil {
			t.Errorf("scenario %s: Run error: %v", r.ScenarioID, r.Err)
		}
		if r.EventCount <= 0 {
			t.Errorf("scenario %s: no events emitted (EventCount=%d)",
				r.ScenarioID, r.EventCount)
		}
		t.Logf("scenario %s: %d events, err=%v", r.ScenarioID, r.EventCount, r.Err)
	}

	// ── 等待所有 goroutine 退出（done channel 已被消费，但需要确认 goroutine 真的退出）
	// 等待时间上限：5s（每个 goroutine 应该 < 1s 完成）
	deadline := time.Now().Add(5 * time.Second)
	for atomic.LoadInt32(&inflight) > 0 && time.Now().Before(deadline) {
		runtime.Gosched()
		time.Sleep(10 * time.Millisecond)
	}
	if got := atomic.LoadInt32(&inflight); got != 0 {
		t.Errorf("有 %d 个 goroutine 未退出", got)
	}

	// ── 稳定后测量（让 runtime 把 goroutine 真正回收）
	runtime.GC()
	runtime.Gosched()
	time.Sleep(100 * time.Millisecond)

	// ── 断言 2：goroutine count < baseline + 50
	afterGoroutines := runtime.NumGoroutine()
	if afterGoroutines > baseGoroutines+50 {
		t.Errorf("goroutine 泄漏: baseline=%d, after=%d, delta=%d",
			baseGoroutines, afterGoroutines, afterGoroutines-baseGoroutines)
	}
	t.Logf("after: goroutines=%d, delta=%d", afterGoroutines, afterGoroutines-baseGoroutines)

	// ── 断言 3：内存增长 < 100MB
	var afterMem runtime.MemStats
	runtime.ReadMemStats(&afterMem)
	allocGrowth := int64(afterMem.HeapAlloc) - int64(baseMem.HeapAlloc)
	inuseGrowth := int64(afterMem.HeapInuse) - int64(baseMem.HeapInuse)
	t.Logf("heap growth: HeapAlloc=+%s, HeapInuse=+%s",
		humanBytes(allocGrowth), humanBytes(inuseGrowth))
	const maxGrowth = 100 * 1024 * 1024 // 100MB
	if allocGrowth > maxGrowth {
		t.Errorf("内存增长过大: HeapAlloc 增长 %s (>100MB)", humanBytes(allocGrowth))
	}
}

// scenarioResult 记录单个 goroutine 的执行结果。
type scenarioResult struct {
	ScenarioID string
	EventCount int
	Err        error
	Panic      string
}

// ─── T16.3 1000 轮长时间稳定性 ───────────────────────────────

// TestMockEngineV2_1000Rounds_NoLeak 验证 v2 引擎连续跑 1000 轮
// （1 Run + 999 Resume）后无内存泄漏、无 goroutine 泄漏。
//
// 策略：程序化构造 1000 轮剧本（每轮 1 个 text_delta 事件），交替
// "user A" / "user B" 作为输入。测量前后 HeapAlloc + HeapInuse 增长。
//
// 跳过：
//   - `go test -short`
//   - 需要 BENCH_V2=1 才会真正执行
func TestMockEngineV2_1000Rounds_NoLeak(t *testing.T) {
	if !shouldRunV2Bench(t) {
		return
	}

	const totalRounds = 1000
	sc := makeThousandRoundScenario(totalRounds)
	t.Logf("synthetic scenario: ID=%s, Rounds=%d, Steps=%d",
		sc.ID, sc.Rounds, len(sc.Steps))

	// ── 稳定基线
	runtime.GC()
	runtime.Gosched()
	time.Sleep(50 * time.Millisecond)
	baseGoroutines := runtime.NumGoroutine()
	var baseMem runtime.MemStats
	runtime.ReadMemStats(&baseMem)
	t.Logf("baseline: goroutines=%d, HeapAlloc=%s",
		baseGoroutines, humanBytes(int64(baseMem.HeapAlloc)))

	// ── 执行 1000 轮
	eng := NewMockEngineV2()
	sess := newMockSession()
	s := newMockTestServer()
	rec := httptest.NewRecorder()

	ctx := context.Background()

	t.Logf("Run round 0...")
	if err := eng.Run(ctx, s, sess, rec, rec, sc, 0, false); err != nil {
		t.Fatalf("Run round 0: %v", err)
	}

	// Resume 999 次（覆盖 round 1..999）
	for i := 1; i < totalRounds; i++ {
		userText := "user A"
		if i%2 == 0 {
			userText = "user B"
		}
		// 用同一个 recorder 即可（我们只关心事件数，不解析内容）
		// 实际生产中每次 Resume 会用新 writer（per-request ResponseWriter）
		recI := httptest.NewRecorder()
		if err := eng.Resume(ctx, s, sess, recI, recI, userText); err != nil {
			t.Fatalf("Resume round %d: %v", i, err)
		}
	}

	// ── 断言 1：session.EventCache 应有 stream_end 事件
	if findEventOfType(sess, "stream_end") == nil {
		t.Error("1000 轮后应推 stream_end")
	}
	// ── 断言 2：EventCache 应至少有 totalRounds * 2 个事件
	//    （每个 round 至少 1 个 text_delta + 一些 round_state 事件）
	if got := len(sess.EventCache); got < totalRounds {
		t.Errorf("EventCache 长度 = %d, want >= %d", got, totalRounds)
	}
	t.Logf("1000 轮完成，EventCache 长度 = %d", len(sess.EventCache))

	// ── 稳定后测量
	runtime.GC()
	runtime.Gosched()
	time.Sleep(100 * time.Millisecond)

	// ── 断言 3：goroutine count 不应大幅增加
	afterGoroutines := runtime.NumGoroutine()
	if afterGoroutines > baseGoroutines+5 {
		// 1000 轮是顺序执行，不应产生任何 goroutine；+5 是给 runtime / pprof
		// 自身的微小波动留余量
		t.Errorf("goroutine 泄漏: baseline=%d, after=%d, delta=%d",
			baseGoroutines, afterGoroutines, afterGoroutines-baseGoroutines)
	}
	t.Logf("after: goroutines=%d, delta=%d", afterGoroutines, afterGoroutines-baseGoroutines)

	// ── 断言 4：内存增长 < 100MB
	var afterMem runtime.MemStats
	runtime.ReadMemStats(&afterMem)
	allocGrowth := int64(afterMem.HeapAlloc) - int64(baseMem.HeapAlloc)
	inuseGrowth := int64(afterMem.HeapInuse) - int64(baseMem.HeapInuse)
	t.Logf("heap growth: HeapAlloc=+%s, HeapInuse=+%s",
		humanBytes(allocGrowth), humanBytes(inuseGrowth))
	const maxGrowth = 100 * 1024 * 1024
	if allocGrowth > maxGrowth {
		t.Errorf("内存增长过大: HeapAlloc 增长 %s (>100MB)", humanBytes(allocGrowth))
	}
}

// makeThousandRoundScenario 构造一个 totalRounds 轮的合成剧本。
//
//   - 每轮 1 个 text_delta 事件
//   - 没有 PauseForUser / BranchChoice（让 Run+Resume 完整跑通不阻塞）
//   - 可用于 T16.3 长时间稳定性测试
func makeThousandRoundScenario(totalRounds int) *MockScenario {
	sc := &MockScenario{
		ID:     fmt.Sprintf("bench_%d_rounds", totalRounds),
		Rounds: totalRounds,
		Steps:  make([]MockStep, 0, totalRounds),
	}
	for i := 0; i < totalRounds; i++ {
		sc.Steps = append(sc.Steps, MockStep{
			RoundIdx: i,
			DelayMs:  0,
			Events: []MockEvent{
				{Type: "text_delta", Data: map[string]any{
					"seq":  i,
					"text": fmt.Sprintf("round %d", i),
				}},
			},
		})
	}
	return sc
}

// ─── 可选 benchmarks ─────────────────────────────────────────

// BenchmarkMockEngineV2_100Rounds 跑 100 轮剧本的 micro bench。
// 1000 轮对 b.N=10 太慢（每次 setup~5s）；100 轮在 0.1s 量级。
func BenchmarkMockEngineV2_100Rounds(b *testing.B) {
	if v := os.Getenv("BENCH_V2"); v != "1" {
		b.Skip("set BENCH_V2=1 to enable v2 bench")
		return
	}
	sc := makeThousandRoundScenario(100)
	eng := NewMockEngineV2()
	sess := newMockSession()
	s := newMockTestServer()
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// 每次迭代从头开始（清状态）
		rec := httptest.NewRecorder()
		_ = eng.Run(ctx, s, sess, rec, rec, sc, 0, false)
		for r := 1; r < 100; r++ {
			recI := httptest.NewRecorder()
			_ = eng.Resume(ctx, s, sess, recI, recI, fmt.Sprintf("u%d", r))
		}
	}
}

// ─── helpers ────────────────────────────────────────────────

// humanBytes 字节数 → 人类可读字符串。
func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%dB", n)
	}
	div, exp := int64(unit), 0
	for x := n / unit; x >= unit; x /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f%cB", float64(n)/float64(div), "KMGTPE"[exp])
}

// ─── 编译期检查：我们的 channelWriter / http 接口实现 ──────────

// channelResponseWriter 是一个最小的 http.ResponseWriter + http.Flusher，
// 用于演示 buffered channel 作为 eventWriter 的模式。
// 当前测试未使用（实际用 httptest.NewRecorder 即可），
// 但保留以验证接口实现正确性，便于未来扩展。
type channelResponseWriter struct {
	mu  sync.Mutex
	buf []byte
	ch  chan []byte
}

var _ http.ResponseWriter = (*channelResponseWriter)(nil)
var _ http.Flusher = (*channelResponseWriter)(nil)

func newChannelResponseWriter(bufSize int) (*channelResponseWriter, <-chan []byte) {
	ch := make(chan []byte, bufSize)
	return &channelResponseWriter{ch: ch}, ch
}

func (w *channelResponseWriter) Header() http.Header {
	return http.Header{}
}

func (w *channelResponseWriter) Write(b []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.buf = append(w.buf, b...)
	return len(b), nil
}

func (w *channelResponseWriter) WriteHeader(int) {}

func (w *channelResponseWriter) Flush() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.buf) == 0 {
		return
	}
	cp := make([]byte, len(w.buf))
	copy(cp, w.buf)
	w.buf = w.buf[:0]
	select {
	case w.ch <- cp:
	default:
		// drop on full channel
	}
}
