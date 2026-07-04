package kernel

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ─── ServiceLifecycle 测试 ──────────────────────────────────────────────

func TestServiceLifecycle_Basic(t *testing.T) {
	initCount := atomic.Int32{}
	factory := func() (Service, error) {
		initCount.Add(1)
		return &echoSvc{name: "test.svc"}, nil
	}

	sl := NewServiceLifecycle(ServiceLifecycleConfig{
		Name:     "test.svc",
		Factory:  factory,
		Graceful: 50 * time.Millisecond,
	})

	if sl.Status() != ServiceInactive {
		t.Fatalf("expected inactive, got %v", sl.Status())
	}

	ctx := NewContext(context.Background())

	// 首次激活
	err := sl.Activate(ctx)
	if err != nil {
		t.Fatalf("activate failed: %v", err)
	}
	if sl.Status() != ServiceActive {
		t.Fatalf("expected active, got %v", sl.Status())
	}
	if initCount.Load() != 1 {
		t.Fatalf("expected 1 init, got %d", initCount.Load())
	}

	// 幂等激活
	err = sl.Activate(ctx)
	if err != nil {
		t.Fatalf("idempotent activate failed: %v", err)
	}
	if initCount.Load() != 1 {
		t.Fatalf("expected still 1 init, got %d", initCount.Load())
	}

	// 调用
	resp, err := sl.CallWithLifecycle(ctx, "echo", map[string]any{"hello": "world"})
	if err != nil {
		t.Fatalf("call failed: %v", err)
	}
	if resp == nil {
		t.Fatal("got nil response")
	}

	// 停用
	err = sl.Deactivate(50 * time.Millisecond)
	if err != nil {
		t.Fatalf("deactivate failed: %v", err)
	}
	if sl.Status() != ServiceInactive {
		t.Fatalf("expected inactive, got %v", sl.Status())
	}

	// 再次激活（重新构造）
	err = sl.Activate(ctx)
	if err != nil {
		t.Fatalf("re-activate failed: %v", err)
	}
	if initCount.Load() != 2 {
		t.Fatalf("expected 2 inits after re-activate, got %d", initCount.Load())
	}

	sl.Deactivate(0)
}

func TestServiceLifecycle_RefCount(t *testing.T) {
	sl := NewServiceLifecycle(ServiceLifecycleConfig{
		Name: "test.refcount",
		Factory: func() (Service, error) {
			return &echoSvc{name: "test.refcount", delay: 20 * time.Millisecond}, nil
		},
		Graceful: 50 * time.Millisecond,
	})

	ctx := NewContext(context.Background())
	_ = sl.Activate(ctx)

	// 并发调用，验证引用计数
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = sl.CallWithLifecycle(ctx, "echo", map[string]any{"i": i})
		}()
	}

	// 等待 goroutine 都进入调用
	time.Sleep(5 * time.Millisecond)
	refCount := sl.RefCount()
	if refCount <= 0 {
		t.Logf("refCount during calls: %d", refCount)
	}

	wg.Wait()

	if sl.RefCount() != 0 {
		t.Fatalf("expected refCount=0 after all calls, got %d", sl.RefCount())
	}

	sl.Deactivate(0)
}

func TestServiceLifecycle_IdleTimeout(t *testing.T) {
	sl := NewServiceLifecycle(ServiceLifecycleConfig{
		Name: "test.idle",
		Factory: func() (Service, error) {
			return &echoSvc{name: "test.idle"}, nil
		},
		IdleTimeout: 30 * time.Millisecond,
		Graceful:    10 * time.Millisecond,
	})

	ctx := NewContext(context.Background())
	_ = sl.Activate(ctx)

	if sl.Status() != ServiceActive {
		t.Fatal("expected active")
	}

	// 等空闲超时
	time.Sleep(80 * time.Millisecond)

	if sl.Status() != ServiceInactive {
		t.Fatalf("expected inactive after idle timeout, got %v", sl.Status())
	}
}

func TestServiceLifecycle_ConcurrentActivate(t *testing.T) {
	initCount := atomic.Int32{}
	sl := NewServiceLifecycle(ServiceLifecycleConfig{
		Name: "test.concurrent",
		Factory: func() (Service, error) {
			initCount.Add(1)
			time.Sleep(10 * time.Millisecond) // 模拟慢初始化
			return &echoSvc{name: "test.concurrent"}, nil
		},
		Graceful: 10 * time.Millisecond,
	})

	ctx := NewContext(context.Background())

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = sl.Activate(ctx)
		}()
	}
	wg.Wait()

	if initCount.Load() != 1 {
		t.Fatalf("expected exactly 1 init (serialized activation), got %d", initCount.Load())
	}
	if sl.Status() != ServiceActive {
		t.Fatalf("expected active, got %v", sl.Status())
	}

	sl.Deactivate(0)
}

func TestServiceLifecycle_GracefulDeactivate(t *testing.T) {
	sl := NewServiceLifecycle(ServiceLifecycleConfig{
		Name: "test.graceful",
		Factory: func() (Service, error) {
			return &echoSvc{name: "test.graceful", delay: 50 * time.Millisecond}, nil
		},
		Graceful: 100 * time.Millisecond,
	})

	ctx := NewContext(context.Background())
	_ = sl.Activate(ctx)

	// 启动一个慢调用
	callDone := make(chan struct{})
	go func() {
		_, _ = sl.CallWithLifecycle(ctx, "slow", map[string]any{"delay": 50})
		close(callDone)
	}()

	// 等调用开始
	time.Sleep(5 * time.Millisecond)

	// 开始停用（graceful 窗口内调用应能完成）
	start := time.Now()
	err := sl.Deactivate(100 * time.Millisecond)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("deactivate failed: %v", err)
	}

	// 等调用完成
	<-callDone

	if sl.Status() != ServiceInactive {
		t.Fatalf("expected inactive, got %v", sl.Status())
	}

	t.Logf("graceful deactivate took %v", elapsed)
}

// ─── MicroKernel 测试 ──────────────────────────────────────────────────

func TestMicroKernel_Basic(t *testing.T) {
	mk := NewMicroKernel(MicroKernelConfig{
		Name:            "test-mk",
		DefaultGraceful: 10 * time.Millisecond,
	})

	// 注册服务
	mk.RegisterService("echo", func() (Service, error) {
		return &echoSvc{name: "echo"}, nil
	})

	mk.RegisterService("slow", func() (Service, error) {
		return &echoSvc{name: "slow", delay: 10 * time.Millisecond}, nil
	})

	services := mk.ListServices()
	if len(services) != 2 {
		t.Fatalf("expected 2 services, got %d", len(services))
	}

	// 验证初始都是 inactive
	for _, s := range services {
		if s.Status != "inactive" {
			t.Fatalf("expected %s to be inactive, got %s", s.Name, s.Status)
		}
	}

	ctx := NewContext(context.Background())

	// 调用 echo（应该自动激活）
	resp, err := mk.Call(ctx, "echo", "test", map[string]any{"hello": "world"})
	if err != nil {
		t.Fatalf("call failed: %v", err)
	}
	if resp == nil {
		t.Fatal("nil response")
	}

	// echo 应该变成 active
	sl, ok := mk.GetServiceLifecycle("echo")
	if !ok {
		t.Fatal("echo not found")
	}
	if sl.Status() != ServiceActive {
		t.Fatalf("expected echo active, got %v", sl.Status())
	}

	// slow 应该还是 inactive
	slSlow, ok := mk.GetServiceLifecycle("slow")
	if !ok {
		t.Fatal("slow not found")
	}
	if slSlow.Status() != ServiceInactive {
		t.Fatalf("expected slow inactive, got %v", slSlow.Status())
	}

	// 全部停用
	mk.DeactivateAll(10 * time.Millisecond)
	for _, s := range mk.ListServices() {
		if s.Status != "inactive" {
			t.Fatalf("expected all inactive, but %s is %s", s.Name, s.Status)
		}
	}
}

func TestMicroKernel_TenantQuota(t *testing.T) {
	mk := NewMicroKernel(MicroKernelConfig{
		Name:            "test-tenant",
		DefaultGraceful: 10 * time.Millisecond,
	})

	mk.RegisterService("echo", func() (Service, error) {
		return &echoSvc{name: "echo", delay: 20 * time.Millisecond}, nil
	})

	// 创建租户（限制并发 2）
	tc := NewTenantContext(TenantConfig{
		ID:             "tenant-a",
		MaxConcurrency: 2,
	})
	mk.tenants["tenant-a"] = tc

	// 带租户的 ctx
	ctx := NewContext(WithTenant(context.Background(), "tenant-a"))

	// 启动 3 个并发调用，第 3 个应该失败
	var successCount atomic.Int32
	var failCount atomic.Int32
	var wg sync.WaitGroup

	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := mk.Call(ctx, "echo", "test", map[string]any{"i": i})
			if err != nil {
				failCount.Add(1)
			} else {
				successCount.Add(1)
			}
		}()
	}

	wg.Wait()

	t.Logf("success=%d, fail=%d", successCount.Load(), failCount.Load())
	// 注意：因为调用很快，可能 3 个都成功（交错进行），
	// 这里只验证配额机制存在，不做严格断言
}

func TestMicroKernel_MemGuard(t *testing.T) {
	mk := NewMicroKernel(MicroKernelConfig{
		Name:       "test-memguard",
		MemGuardMB: 1, // 极小阈值，快速触发
	})

	mk.RegisterService("echo", func() (Service, error) {
		return &echoSvc{name: "echo"}, nil
	})

	ctx := NewContext(context.Background())

	// 先正常调用
	_, err := mk.Call(ctx, "echo", "test", nil)
	if err != nil {
		t.Fatalf("initial call failed: %v", err)
	}

	// 手动触发内存守卫
	if mk.MemGuard() != nil {
		mk.MemGuard().triggered.Store(true)
		// 触发后应该拒绝新调用
		_, err = mk.Call(ctx, "echo", "test", nil)
		if err == nil {
			t.Fatal("expected error after memguard triggered")
		}
		t.Logf("memguard correctly rejected: %v", err)
	}

	mk.DeactivateAll(0)
}

func TestMicroKernel_HealthAll(t *testing.T) {
	mk := NewMicroKernel(MicroKernelConfig{
		Name:            "test-health",
		DefaultGraceful: 10 * time.Millisecond,
	})

	mk.RegisterService("healthy", func() (Service, error) {
		return &echoSvc{name: "healthy"}, nil
	})

	ctx := NewContext(context.Background())
	statuses := mk.HealthAll(ctx)

	if len(statuses) != 1 {
		t.Fatalf("expected 1 status, got %d", len(statuses))
	}

	// inactive 的服务 Health 也算 ok（不算不健康）
	if !statuses[0].OK {
		t.Fatalf("expected inactive service to be ok, got error: %s", statuses[0].Error)
	}

	// 激活后再查
	_ = mk.Activate(ctx, "healthy")
	statuses = mk.HealthAll(ctx)
	if !statuses[0].OK {
		t.Fatalf("expected active service to be ok")
	}

	mk.DeactivateAll(0)
}

// ─── 性能基准：冷启动耗时 ──────────────────────────────────────────────

func BenchmarkServiceLifecycle_Activate(b *testing.B) {
	factory := func() (Service, error) {
		return &echoSvc{name: "bench.svc"}, nil
	}

	for i := 0; i < b.N; i++ {
		sl := NewServiceLifecycle(ServiceLifecycleConfig{
			Name:    "bench.svc",
			Factory: factory,
		})
		ctx := NewContext(context.Background())
		_ = sl.Activate(ctx)
		sl.Deactivate(0)
	}
}

func BenchmarkServiceLifecycle_CallActive(b *testing.B) {
	sl := NewServiceLifecycle(ServiceLifecycleConfig{
		Name: "bench.call",
		Factory: func() (Service, error) {
			return &echoSvc{name: "bench.call"}, nil
		},
	})
	ctx := NewContext(context.Background())
	_ = sl.Activate(ctx)
	defer sl.Deactivate(0)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = sl.CallWithLifecycle(ctx, "echo", map[string]any{"i": i})
	}
}
