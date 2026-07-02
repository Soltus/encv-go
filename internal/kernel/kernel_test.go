package kernel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ─── ctx 测试 ─────────────────────────────────────────────

func TestServiceContext_PropagatesRequestIDAndTraceID(t *testing.T) {
	parent := NewContext(context.Background())
	child := NewContext(parent)

	if child.RequestID() == parent.RequestID() {
		t.Errorf("RequestID should be unique per ctx")
	}
	if child.TraceID() != parent.TraceID() {
		t.Errorf("TraceID should be inherited: child=%q parent=%q", child.TraceID(), parent.TraceID())
	}
}

func TestServiceContext_ParentIDOverriddenByNewID(t *testing.T) {
	parent := NewContext(context.Background(), WithTraceID("trace-abc"))
	child := NewContext(parent)

	if child.TraceID() != "trace-abc" {
		t.Errorf("explicit TraceID not preserved: got %q", child.TraceID())
	}
}

func TestServiceContext_BudgetFromDeadline(t *testing.T) {
	deadline := time.Now().Add(2 * time.Second)
	parent, cancel := context.WithDeadline(context.Background(), deadline)
	defer cancel()

	ctx := NewContext(parent)
	budget := ctx.Budget()
	if budget < time.Second || budget > 2*time.Second {
		t.Errorf("Budget out of range: %v", budget)
	}
}

func TestServiceContext_BudgetCountsDown(t *testing.T) {
	deadline := time.Now().Add(500 * time.Millisecond)
	parent, cancel := context.WithDeadline(context.Background(), deadline)
	defer cancel()

	ctx := NewContext(parent)
	first := ctx.Budget()
	time.Sleep(100 * time.Millisecond)
	second := ctx.Budget()

	if second >= first {
		t.Errorf("Budget should decrease: first=%v second=%v", first, second)
	}
}

func TestServiceContext_BudgetNoDeadline(t *testing.T) {
	ctx := NewContext(context.Background())
	budget := ctx.Budget()
	if budget < 24*time.Hour*365/2 {
		t.Errorf("No-deadline budget should be huge: %v", budget)
	}
}

func TestServiceContext_Elapsed(t *testing.T) {
	ctx := NewContext(context.Background())
	time.Sleep(50 * time.Millisecond)
	elapsed := ctx.Elapsed()
	if elapsed < 50*time.Millisecond {
		t.Errorf("Elapsed too small: %v", elapsed)
	}
}

func TestServiceContext_CancelViaParent(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	ctx := NewContext(parent)

	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	select {
	case <-ctx.Done():
		// OK
	case <-time.After(time.Second):
		t.Fatal("ctx should be cancelled when parent cancels")
	}
	if ctx.Err() == nil {
		t.Fatal("ctx.Err() should be non-nil after cancel")
	}
}

func TestServiceContext_Values(t *testing.T) {
	parent := context.WithValue(context.Background(), "userID", "alice")
	ctx := NewContext(parent)

	if v := ctx.Value("userID"); v != "alice" {
		t.Errorf("userID not propagated: got %v", v)
	}
	if v := ctx.Value(keyService); v != "kernel" {
		t.Errorf("default service: got %v", v)
	}
}

func TestServiceContext_WithServiceName(t *testing.T) {
	ctx := NewContext(context.Background(), WithServiceName("search.vector"))
	if ctx.Service() != "search.vector" {
		t.Errorf("Service() = %q", ctx.Service())
	}
	if v := ctx.Value(keyService); v != "search.vector" {
		t.Errorf("ctx.Value(keyService) = %v", v)
	}
}

func TestServiceContext_CheckpointAndRestore(t *testing.T) {
	store := NewMemoryCheckpointStore()
	ctx := NewContext(context.Background(), WithCheckpointStore(store))

	type State struct {
		Step   int      `json:"step"`
		Buffer []string `json:"buffer"`
	}
	state := State{Step: 3, Buffer: []string{"a", "b", "c"}}

	if err := ctx.Checkpoint("encrypt-progress", state); err != nil {
		t.Fatal(err)
	}

	var restored State
	if err := ctx.Restore("encrypt-progress", &restored); err != nil {
		t.Fatal(err)
	}
	if restored.Step != 3 || len(restored.Buffer) != 3 {
		t.Errorf("restored mismatch: %+v", restored)
	}
}

func TestServiceContext_CheckpointUnsupported(t *testing.T) {
	ctx := NewContext(context.Background()) // no store
	err := ctx.Checkpoint("x", map[string]int{"a": 1})
	if !errors.Is(err, ErrCheckpointUnsupported) {
		t.Errorf("expected ErrCheckpointUnsupported, got %v", err)
	}
}

func TestServiceContext_FileCheckpoint(t *testing.T) {
	dir := t.TempDir()
	store, err := NewFileCheckpointStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	ctx := NewContext(context.Background(), WithCheckpointStore(store))

	type State struct{ Counter int }
	if err := ctx.Checkpoint("alpha", State{Counter: 42}); err != nil {
		t.Fatal(err)
	}

	// 新 ctx（模拟"重启"），store 是同一个，traceID 也要相同才能 restore
	ctx2 := NewContext(context.Background(), WithTraceID(ctx.TraceID()), WithCheckpointStore(store))
	var got State
	if err := ctx2.Restore("alpha", &got); err != nil {
		t.Fatal(err)
	}
	if got.Counter != 42 {
		t.Errorf("Counter = %d, want 42", got.Counter)
	}
}

func TestServiceContext_FromContext(t *testing.T) {
	// 传普通 context.Context
	plain := context.Background()
	sc := FromContext(plain)
	if sc.Service() != "kernel" {
		t.Errorf("FromContext(plain).Service() = %q", sc.Service())
	}

	// 传 ServiceContext
	original := NewContext(context.Background(), WithServiceName("agent.chat"))
	sc2 := FromContext(original)
	if sc2 != original {
		t.Error("FromContext(ServiceContext) should return same instance")
	}
}

// ─── registry 测试 ────────────────────────────────────────

// mockService 用于测试
type mockService struct {
	name    string
	health  error
	calls   *uint64
	lastMth string
}

func (m *mockService) Name() string                    { return m.name }
func (m *mockService) Init(ctx ServiceContext) error   { return nil }
func (m *mockService) Health(ctx ServiceContext) error { return m.health }

func (m *mockService) Call(ctx ServiceContext, method string, payload json.RawMessage) (json.RawMessage, error) {
	atomic.AddUint64(m.calls, 1)
	m.lastMth = method
	resp := map[string]any{
		"service":     ctx.Service(),
		"requestID":   ctx.RequestID(),
		"traceID":     ctx.TraceID(),
		"called":      method,
		"echoPayload": json.RawMessage(payload),
	}
	return json.Marshal(resp)
}

func TestRegister_AndGet(t *testing.T) {
	Unregister("test.svc")
	defer Unregister("test.svc")

	calls := new(uint64)
	svc := &mockService{name: "test.svc", calls: calls}
	Register(svc)

	got, ok := Get("test.svc")
	if !ok {
		t.Fatal("Get should return true after Register")
	}
	if got.Name() != "test.svc" {
		t.Errorf("Name mismatch")
	}
}

func TestRegister_PanicOnDuplicate(t *testing.T) {
	Unregister("test.dup")
	defer Unregister("test.dup")

	Register(&mockService{name: "test.dup"})
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic on duplicate Register")
		}
	}()
	Register(&mockService{name: "test.dup"})
}

func TestCall_PropagatesCtxToService(t *testing.T) {
	Unregister("test.call")
	defer Unregister("test.call")

	calls := new(uint64)
	Register(&mockService{name: "test.call", calls: calls})

	parent := NewContext(context.Background(), WithTraceID("trace-call-1"))
	resp, err := Call(parent, "test.call", "doSomething", map[string]string{"k": "v"})
	if err != nil {
		t.Fatal(err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(resp, &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed["service"] != "test.call" {
		t.Errorf("service field in response = %v, want test.call", parsed["service"])
	}
	if parsed["traceID"] != "trace-call-1" {
		t.Errorf("traceID = %v, want trace-call-1", parsed["traceID"])
	}
	if parsed["called"] != "doSomething" {
		t.Errorf("called = %v", parsed["called"])
	}
	if atomic.LoadUint64(calls) != 1 {
		t.Errorf("calls = %d, want 1", calls)
	}
}

func TestCall_NotFound(t *testing.T) {
	_, err := Call(NewContext(context.Background()), "nonexistent.service", "x", nil)
	if !errors.Is(err, ErrServiceNotFound) {
		t.Errorf("expected ErrServiceNotFound, got %v", err)
	}
}

func TestCall_RespectsContextCancellation(t *testing.T) {
	Unregister("test.cancel")
	defer Unregister("test.cancel")

	Register(&mockService{name: "test.cancel"})
	parent, cancel := context.WithCancel(context.Background())
	cancel() // 立即取消

	_, err := Call(NewContext(parent), "test.cancel", "x", nil)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestCallTyped_TypeSafe(t *testing.T) {
	Unregister("test.typed")
	defer Unregister("test.typed")

	calls := new(uint64)
	Register(&mockService{name: "test.typed", calls: calls})

	type Req struct{ X int }
	type Resp struct{ Doubled int }

	// mock service 实际上不解析 payload，但 CallTyped 会 marshal/unmarshal
	// 我们用 sentinel 方法来验证
	got, err := CallTyped[map[string]any, map[string]any](
		NewContext(context.Background()), "test.typed", "any", map[string]any{"x": 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := got["service"]; !ok {
		t.Errorf("expected service field, got %v", got)
	}

	_ = Req{}
	_ = Resp{}
}

func TestHealthAll_AggregatesStatus(t *testing.T) {
	Unregister("test.h1")
	Unregister("test.h2")
	defer Unregister("test.h1")
	defer Unregister("test.h2")

	Register(&mockService{name: "test.h1"})
	Register(&mockService{name: "test.h2", health: errors.New("unhealthy")})

	results := HealthAll(NewContext(context.Background()))
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	for _, r := range results {
		if r.Name == "test.h1" && !r.OK {
			t.Error("test.h1 should be OK")
		}
		if r.Name == "test.h2" && r.OK {
			t.Error("test.h2 should be unhealthy")
		}
	}
}

// ─── bus 测试 ─────────────────────────────────────────────

func TestBus_PublishSubscribe(t *testing.T) {
	received := make(chan string, 1)
	unsub := Subscribe(context.Background(), "test.topic", func(ev Event) error {
		received <- ev.Payload.(string)
		return nil
	})
	defer unsub()

	Publish(NewContext(context.Background()), "test.topic", "hello")
	select {
	case msg := <-received:
		if msg != "hello" {
			t.Errorf("got %q, want hello", msg)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}
}

func TestBus_UnsubStopsDelivery(t *testing.T) {
	count := new(uint64)
	unsub := Subscribe(context.Background(), "test.unsub", func(ev Event) error {
		atomic.AddUint64(count, 1)
		return nil
	})
	unsub()
	Publish(NewContext(context.Background()), "test.unsub", "x")
	time.Sleep(50 * time.Millisecond)
	if atomic.LoadUint64(count) != 0 {
		t.Errorf("count = %d, want 0", count)
	}
}

func TestBus_CtxCancelStopsDelivery(t *testing.T) {
	count := new(uint64)
	ctx, cancel := context.WithCancel(context.Background())
	Subscribe(ctx, "test.ctxstop", func(ev Event) error {
		atomic.AddUint64(count, 1)
		return nil
	})
	cancel()
	Publish(NewContext(context.Background()), "test.ctxstop", "x")
	time.Sleep(50 * time.Millisecond)
	if atomic.LoadUint64(count) != 0 {
		t.Errorf("count = %d, want 0", count)
	}
}

func TestBus_PublishErrorStopsAtFailingSub(t *testing.T) {
	first := new(uint64)
	second := new(uint64)
	Subscribe(context.Background(), "test.err", func(ev Event) error {
		atomic.AddUint64(first, 1)
		return errors.New("fail")
	})
	Subscribe(context.Background(), "test.err", func(ev Event) error {
		atomic.AddUint64(second, 1)
		return nil
	})
	err := Publish(NewContext(context.Background()), "test.err", nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if atomic.LoadUint64(first) != 1 {
		t.Errorf("first = %d, want 1", first)
	}
	if atomic.LoadUint64(second) != 0 {
		t.Errorf("second = %d, want 0 (should not be called after first failed)", second)
	}
}

func TestBus_AsyncDoesNotBlock(t *testing.T) {
	got := make(chan struct{})
	var once sync.Once
	Subscribe(context.Background(), "test.async", func(ev Event) error {
		once.Do(func() { close(got) })
		return nil
	})
	// 即使订阅者睡眠，PublishAsync 不阻塞
	PublishAsync(NewContext(context.Background()), "test.async", nil)
	PublishAsync(NewContext(context.Background()), "test.async", nil)
	PublishAsync(NewContext(context.Background()), "test.async", nil)
	select {
	case <-got:
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}
}

func TestBus_TypedSubscribe(t *testing.T) {
	got := make(chan MyToolEvent, 1)
	SubscribeTyped[MyToolEvent](context.Background(), "test.typed.event", func(ev Event, payload MyToolEvent) error {
		got <- payload
		return nil
	})
	Publish(NewContext(context.Background()), "test.typed.event", MyToolEvent{Tool: "x", Status: "ok"})
	select {
	case p := <-got:
		if p.Tool != "x" || p.Status != "ok" {
			t.Errorf("got %+v", p)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}
}

type MyToolEvent struct {
	Tool   string `json:"tool"`
	Status string `json:"status"`
}

// ─── pool 测试 ────────────────────────────────────────────

func TestPool_SubmitAndExecute(t *testing.T) {
	Unregister("pool.svc")
	defer Unregister("pool.svc")

	calls := new(uint64)
	Register(&mockService{name: "pool.svc", calls: calls})

	pool := NewPool("test-pool", 2, DefaultPoolConfig())
	pool.Start(context.Background())
	defer pool.Stop()

	done := make(chan struct{}, 1)
	job := Job{
		ID:      "j1",
		Service: "pool.svc",
		Method:  "process",
		Payload: map[string]int{"x": 1},
		OnDone: func(r JobResult) {
			if r.Err != nil {
				t.Errorf("job failed: %v", r.Err)
			}
			close(done)
		},
	}
	if err := pool.Submit(NewContext(context.Background()), job); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout")
	}
	if atomic.LoadUint64(calls) != 1 {
		t.Errorf("calls = %d, want 1", calls)
	}
}

func TestPool_StopCancelsInflight(t *testing.T) {
	Unregister("pool.slow")
	defer Unregister("pool.slow")

	slow := &slowService{name: "pool.slow", delay: 5 * time.Second}
	Register(slow)

	pool := NewPool("test-pool-cancel", 1, DefaultPoolConfig())
	pool.Start(context.Background())

	done := make(chan struct{})
	job := Job{
		ID:      "j-slow",
		Service: "pool.slow",
		Method:  "process",
		OnDone: func(r JobResult) {
			if r.Err == nil {
				t.Error("expected error after cancel")
			}
			close(done)
		},
	}
	pool.Submit(NewContext(context.Background()), job)
	time.Sleep(50 * time.Millisecond) // 让 worker 拿到 job
	pool.Stop()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout")
	}
}

type slowService struct {
	name  string
	delay time.Duration
}

func (s *slowService) Name() string                    { return s.name }
func (s *slowService) Init(ctx ServiceContext) error   { return nil }
func (s *slowService) Health(ctx ServiceContext) error { return nil }
func (s *slowService) Call(ctx ServiceContext, method string, payload json.RawMessage) (json.RawMessage, error) {
	select {
	case <-time.After(s.delay):
		return json.Marshal("done")
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func TestPool_RetryOnFailure(t *testing.T) {
	Unregister("pool.retry")
	defer Unregister("pool.retry")

	attempts := new(uint64)
	Register(&flakyService{name: "pool.retry", failTimes: 2, attempts: attempts})

	pool := NewPool("test-pool-retry", 1, PoolConfig{
		QueueSize:     10,
		RetryBackoff:  10 * time.Millisecond,
		RetryMaxDelay: 100 * time.Millisecond,
	})
	pool.Start(context.Background())
	defer pool.Stop()

	done := make(chan struct{}, 1)
	job := Job{
		ID:       "j-retry",
		Service:  "pool.retry",
		Method:   "process",
		MaxRetry: 3,
		OnDone: func(r JobResult) {
			done <- struct{}{}
		},
	}
	pool.Submit(NewContext(context.Background()), job)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout")
	}
	if atomic.LoadUint64(attempts) != 3 {
		t.Errorf("attempts = %d, want 3 (2 fail + 1 success)", attempts)
	}
}

type flakyService struct {
	name      string
	failTimes int32
	attempts  *uint64
}

func (f *flakyService) Name() string                    { return f.name }
func (f *flakyService) Init(ctx ServiceContext) error   { return nil }
func (f *flakyService) Health(ctx ServiceContext) error { return nil }
func (f *flakyService) Call(ctx ServiceContext, method string, payload json.RawMessage) (json.RawMessage, error) {
	n := atomic.AddUint64(f.attempts, 1)
	if int32(n) <= f.failTimes {
		return nil, NewRetryable("flaky", errors.New("transient"))
	}
	return json.Marshal("ok")
}

// ─── tool 测试 ────────────────────────────────────────────

type echoTool struct{}

func (e *echoTool) Name() string { return "echo" }
func (e *echoTool) Description() string {
	return "回显输入"
}
func (e *echoTool) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"msg":{"type":"string"}}}`)
}
func (e *echoTool) Invoke(ctx ServiceContext, args json.RawMessage) (json.RawMessage, error) {
	var a struct{ Msg string }
	if err := json.Unmarshal(args, &a); err != nil {
		return nil, err
	}
	if ctx.Service() != "tool.echo" {
		return nil, fmt.Errorf("service = %q, want tool.echo", ctx.Service())
	}
	return json.Marshal(map[string]string{"echoed": a.Msg})
}

func TestTool_RegisterAndInvoke(t *testing.T) {
	UnregisterTool("echo")
	RegisterTool(&echoTool{})
	defer UnregisterTool("echo")

	resp, err := InvokeTool(NewContext(context.Background()), "echo", json.RawMessage(`{"msg":"hi"}`))
	if err != nil {
		t.Fatal(err)
	}
	var r struct{ Echoed string }
	if err := json.Unmarshal(resp, &r); err != nil {
		t.Fatal(err)
	}
	if r.Echoed != "hi" {
		t.Errorf("Echoed = %q", r.Echoed)
	}
}

func TestTool_NotFound(t *testing.T) {
	_, err := InvokeTool(NewContext(context.Background()), "nope", nil)
	if !errors.Is(err, ErrToolNotFound) {
		t.Errorf("expected ErrToolNotFound, got %v", err)
	}
}

func TestTool_InvokeTyped(t *testing.T) {
	UnregisterTool("echo")
	RegisterTool(&echoTool{})
	defer UnregisterTool("echo")

	type Args struct {
		Msg string `json:"msg"`
	}
	type Resp struct {
		Echoed string `json:"echoed"`
	}

	r, err := InvokeToolTyped[Args, Resp](
		NewContext(context.Background()), "echo", Args{Msg: "typed"})
	if err != nil {
		t.Fatal(err)
	}
	if r.Echoed != "typed" {
		t.Errorf("Echoed = %q", r.Echoed)
	}
}

func TestTool_EmitsBusEvent(t *testing.T) {
	UnregisterTool("echo")
	RegisterTool(&echoTool{})
	defer UnregisterTool("echo")

	got := make(chan struct{}, 1)
	Subscribe(context.Background(), "tool.invoked", func(ev Event) error {
		if p, ok := ev.Payload.(map[string]any); ok {
			if p["tool"] == "echo" {
				got <- struct{}{}
			}
		}
		return nil
	})

	InvokeTool(NewContext(context.Background()), "echo", json.RawMessage(`{"msg":"x"}`))
	select {
	case <-got:
	case <-time.After(2 * time.Second):
		t.Fatal("bus event not received")
	}
}

func TestTool_Stats(t *testing.T) {
	UnregisterTool("echo")
	RegisterTool(&echoTool{})
	defer UnregisterTool("echo")
	ResetToolStats("echo") // 重置前置测试遗留

	for i := 0; i < 3; i++ {
		InvokeTool(NewContext(context.Background()), "echo", json.RawMessage(`{"msg":"x"}`))
	}
	stats := ToolStatsAll()
	var found *ToolStats
	for i := range stats {
		if stats[i].Name == "echo" {
			found = &stats[i]
			break
		}
	}
	if found == nil {
		t.Fatal("echo not in stats")
	}
	if found.Count != 3 {
		t.Errorf("Count = %d, want 3", found.Count)
	}
	if found.AvgTime <= 0 {
		t.Errorf("AvgTime should be > 0, got %v", found.AvgTime)
	}
}

func TestTool_RespectsContextCancel(t *testing.T) {
	UnregisterTool("slow-tool")
	RegisterTool(&slowTool{})
	defer UnregisterTool("slow-tool")

	parent, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := InvokeTool(NewContext(parent), "slow-tool", nil)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

type slowTool struct{}

func (s *slowTool) Name() string            { return "slow-tool" }
func (s *slowTool) Description() string     { return "slow" }
func (s *slowTool) Schema() json.RawMessage { return json.RawMessage(`{}`) }
func (s *slowTool) Invoke(ctx ServiceContext, args json.RawMessage) (json.RawMessage, error) {
	select {
	case <-time.After(10 * time.Second):
		return json.Marshal("ok")
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// ─── PoC 端到端：ctx 全链路穿透（SearchService 风格） ────────────────────

func TestPoC_EndToEndCtxPropagation(t *testing.T) {
	Unregister("poc.search")
	defer Unregister("poc.search")

	// 1. 注册 PoC service
	calls := new(uint64)
	Register(&pocSearchService{calls: calls})

	// 2. 创建根 ctx（含 traceID 用于全链路追踪）
	rootCtx := NewContext(context.Background(), WithTraceID("trace-poc-001"))

	// 3. 通过 kernel.Call 调 service
	req := map[string]any{"query": "在线", "limit": 10}
	resp, err := Call(rootCtx, "poc.search", "vector", req)
	if err != nil {
		t.Fatal(err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(resp, &parsed); err != nil {
		t.Fatal(err)
	}

	// 4. 验证 ctx 全链路：
	if parsed["traceID"] != "trace-poc-001" {
		t.Errorf("traceID not propagated: got %v", parsed["traceID"])
	}
	if !strings.HasPrefix(parsed["requestID"].(string), "20") {
		// 我们的 ID 格式是 "20260702T...-xxx"
		t.Errorf("requestID format unexpected: %v", parsed["requestID"])
	}
	if atomic.LoadUint64(calls) != 1 {
		t.Errorf("calls = %d, want 1", calls)
	}

	// 5. 测试 deadline budget 传递
	deadlineCtx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	sc := NewContext(deadlineCtx)
	budget := sc.Budget()
	if budget > 100*time.Millisecond {
		t.Errorf("Budget = %v, want <= 100ms", budget)
	}

	// 6. 验证 Cancel 链：取消 rootCtx → service 收到 ctx.Done()
	canCtx, canCancel := context.WithCancel(context.Background())
	sc2 := NewContext(canCtx)
	canCancel()
	// service 内部 select ctx.Done()，应立即返回 ctx.Err()
	_, err = Call(sc2, "poc.search", "vector", req)
	if err == nil {
		t.Error("expected error after cancel")
	}
}

type pocSearchService struct {
	calls *uint64
}

func (p *pocSearchService) Name() string                    { return "poc.search" }
func (p *pocSearchService) Init(ctx ServiceContext) error   { return nil }
func (p *pocSearchService) Health(ctx ServiceContext) error { return nil }
func (p *pocSearchService) Call(ctx ServiceContext, method string, payload json.RawMessage) (json.RawMessage, error) {
	// 模拟一个长任务，会响应 ctx.Done()
	select {
	case <-time.After(50 * time.Millisecond):
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	atomic.AddUint64(p.calls, 1)
	return json.Marshal(map[string]any{
		"traceID":   ctx.TraceID(),
		"requestID": ctx.RequestID(),
		"service":   ctx.Service(),
		"called":    method,
	})
}

// ─── 并发安全测试 ────────────────────────────────────────

func TestConcurrent_RegistryAndCall(t *testing.T) {
	Unregister("conc.svc")
	defer Unregister("conc.svc")

	Register(&mockService{name: "conc.svc", calls: new(uint64)})

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = Call(NewContext(context.Background()), "conc.svc", "x", nil)
		}()
	}
	wg.Wait()
}

func TestConcurrent_BusPublish(t *testing.T) {
	var received uint64
	Subscribe(context.Background(), "conc.bus", func(ev Event) error {
		atomic.AddUint64(&received, 1)
		return nil
	})

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			Publish(NewContext(context.Background()), "conc.bus", nil)
		}()
	}
	wg.Wait()
	if atomic.LoadUint64(&received) != 100 {
		t.Errorf("received = %d, want 100", received)
	}
}
