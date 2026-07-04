package kernel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// ─── ServiceLifecycle：微内核服务级生命周期管理 ────────────────────────────
//
// 2026-07-03 spec microkernel-split-start-stop Phase 4.2
//
// 核心问题（用户原话）：
//   "安卓端依旧是一个完整的内核，适配workManger也没多大用，
//    无法满足第三方AI Agent工具调用、频繁多租户只拉起微内核
//    处理小任务等业务消费需求"
//
// 设计目标：
//   1. **按需激活**：服务默认未激活（inactive），首次调用时自动激活
//      （冷启动 ≤ 500ms），空闲超时后自动停用
//   2. **独立启停**：每个服务有自己的生命周期，不影响其他服务
//   3. **引用计数**：多租户同时使用时计数 +1，全部释放后才停用
//   4. **优雅停用**：有 in-flight 调用时等待完成（graceful 窗口），
//      超时后强制 cancel ctx
//   5. **零端口**：全部进程内，不消耗 TCP 端口
//
// 与 Lifecycle 的区别：
//   - Lifecycle：整内核级启停（启停所有 Pool + MemoryGuard），
//     对应"整个 Go 进程要不要跑"
//   - ServiceLifecycle：单服务级启停（激活/停用单个 Service），
//     对应"这个功能要不要加载"——这才是真正的微内核
//
// 状态机：
//
//	inactive → activating → active → deactivating → inactive
//	                          ↑         ↓
//	                          └── ref_count > 0 ──┘
type ServiceLifecycle struct {
	name     string
	factory  ServiceFactory // 延迟构造（首次激活时才 New）
	svc      Service
	svcMu    sync.RWMutex
	state    atomic.Uint32 // 0=inactive, 1=activating, 2=active, 3=deactivating
	refCount atomic.Int32  // 当前引用数（活跃调用数）

	idleTimeout time.Duration // 空闲超时（0 = 不自动停用）
	graceful    time.Duration // 优雅停用窗口

	idleTimer *time.Timer
	idleMu    sync.Mutex

	activateCh chan struct{} // 序列化激活（防止并发激活）
	mu         sync.Mutex    // 保护状态转换

	// stats
	activatedAt   atomic.Int64
	deactivatedAt atomic.Int64
	activateNs    atomic.Int64 // 上次激活耗时（ns）
	deactivateNs  atomic.Int64 // 上次停用耗时（ns）
	activateCount atomic.Uint64
}

// ServiceStatus 服务状态
type ServiceStatus int

const (
	ServiceInactive     ServiceStatus = 0
	ServiceActivating   ServiceStatus = 1
	ServiceActive       ServiceStatus = 2
	ServiceDeactivating ServiceStatus = 3
)

func (s ServiceStatus) String() string {
	switch s {
	case ServiceInactive:
		return "inactive"
	case ServiceActivating:
		return "activating"
	case ServiceActive:
		return "active"
	case ServiceDeactivating:
		return "deactivating"
	default:
		return "unknown"
	}
}

// ServiceFactory 服务工厂（延迟构造用）
// 返回的 Service 尚未 Init，由 ServiceLifecycle 在激活时调用 Init
type ServiceFactory func() (Service, error)

// ServiceLifecycleConfig 配置
type ServiceLifecycleConfig struct {
	Name        string
	Factory     ServiceFactory
	IdleTimeout time.Duration // 空闲超时自动停用（0 = 不自动停用，手动管理）
	Graceful    time.Duration // 优雅停用窗口（默认 100ms）
}

// NewServiceLifecycle 构造（服务初始为 inactive 状态，不占资源）
func NewServiceLifecycle(cfg ServiceLifecycleConfig) *ServiceLifecycle {
	if cfg.Graceful <= 0 {
		cfg.Graceful = 100 * time.Millisecond
	}
	sl := &ServiceLifecycle{
		name:        cfg.Name,
		factory:     cfg.Factory,
		idleTimeout: cfg.IdleTimeout,
		graceful:    cfg.Graceful,
		activateCh:  make(chan struct{}, 1),
	}
	sl.state.Store(uint32(ServiceInactive))
	return sl
}

// Name 服务名
func (sl *ServiceLifecycle) Name() string {
	return sl.name
}

// Status 当前状态（无锁，atomic）
func (sl *ServiceLifecycle) Status() ServiceStatus {
	return ServiceStatus(sl.state.Load())
}

// RefCount 当前引用计数
func (sl *ServiceLifecycle) RefCount() int32 {
	return sl.refCount.Load()
}

// Activate 手动激活服务（幂等）。
// 通常不需要手动调用——CallWithLifecycle 会自动激活。
// 但预热场景可以主动调。
func (sl *ServiceLifecycle) Activate(ctx context.Context) error {
	return sl.ensureActive(ctx)
}

// Deactivate 手动停用服务（幂等）。
// 有引用计数时会等引用归零后再停用。
// idleTimeout=0 时需要手动调用来释放资源。
func (sl *ServiceLifecycle) Deactivate(graceful time.Duration) error {
	if graceful <= 0 {
		graceful = sl.graceful
	}
	return sl.deactivate(graceful)
}

// ensureActive 确保服务处于 active 状态。
// 如果 inactive 则激活（调用 factory + Init）。
// 并发调用会排队（通过 activateCh 序列化）。
func (sl *ServiceLifecycle) ensureActive(ctx context.Context) error {
	current := ServiceStatus(sl.state.Load())
	if current == ServiceActive {
		return nil
	}

	// 尝试获得激活权（buffered chan 作 semaphore）
	select {
	case sl.activateCh <- struct{}{}:
		defer func() { <-sl.activateCh }()
	default:
		// 别人正在激活，等
		return sl.waitForActive(ctx)
	}

	// 双重检查
	current = ServiceStatus(sl.state.Load())
	if current == ServiceActive {
		return nil
	}
	if current == ServiceDeactivating {
		return errors.New("kernel: service is deactivating, try again later")
	}

	sl.state.Store(uint32(ServiceActivating))
	start := time.Now()

	svc, err := sl.factory()
	if err != nil {
		sl.state.Store(uint32(ServiceInactive))
		return fmt.Errorf("kernel: factory for %q failed: %w", sl.name, err)
	}

	initCtx := NewContext(ctx, WithServiceName(sl.name))
	if err := svc.Init(initCtx); err != nil {
		sl.state.Store(uint32(ServiceInactive))
		return fmt.Errorf("kernel: init %q failed: %w", sl.name, err)
	}

	sl.svcMu.Lock()
	sl.svc = svc
	sl.svcMu.Unlock()

	sl.state.Store(uint32(ServiceActive))
	sl.activatedAt.Store(time.Now().UnixNano())
	sl.activateNs.Store(time.Since(start).Nanoseconds())
	sl.activateCount.Add(1)

	sl.resetIdleTimer()
	return nil
}

func (sl *ServiceLifecycle) waitForActive(ctx context.Context) error {
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if ServiceStatus(sl.state.Load()) == ServiceActive {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(10 * time.Millisecond):
		}
	}
	return fmt.Errorf("kernel: %q activation timed out", sl.name)
}

// acquire 引用计数 +1（调用前必须确保 active）
func (sl *ServiceLifecycle) acquire() bool {
	if ServiceStatus(sl.state.Load()) != ServiceActive {
		return false
	}
	sl.refCount.Add(1)
	sl.stopIdleTimer()
	return true
}

// release 引用计数 -1
func (sl *ServiceLifecycle) release() {
	newCount := sl.refCount.Add(-1)
	if newCount <= 0 && sl.idleTimeout > 0 {
		sl.resetIdleTimer()
	}
}

func (sl *ServiceLifecycle) resetIdleTimer() {
	if sl.idleTimeout <= 0 {
		return
	}
	sl.idleMu.Lock()
	defer sl.idleMu.Unlock()
	if sl.idleTimer != nil {
		sl.idleTimer.Stop()
	}
	sl.idleTimer = time.AfterFunc(sl.idleTimeout, func() {
		if sl.refCount.Load() <= 0 {
			_ = sl.deactivate(sl.graceful)
		}
	})
}

func (sl *ServiceLifecycle) stopIdleTimer() {
	sl.idleMu.Lock()
	defer sl.idleMu.Unlock()
	if sl.idleTimer != nil {
		sl.idleTimer.Stop()
		sl.idleTimer = nil
	}
}

// deactivate 停用服务（graceful 窗口等 in-flight 完成）
func (sl *ServiceLifecycle) deactivate(graceful time.Duration) error {
	sl.mu.Lock()
	current := ServiceStatus(sl.state.Load())
	if current == ServiceInactive {
		sl.mu.Unlock()
		return nil
	}
	if current == ServiceDeactivating {
		sl.mu.Unlock()
		// 别人在停用，等它完成
		for ServiceStatus(sl.state.Load()) == ServiceDeactivating {
			time.Sleep(10 * time.Millisecond)
		}
		return nil
	}
	sl.state.Store(uint32(ServiceDeactivating))
	sl.mu.Unlock()

	start := time.Now()

	sl.stopIdleTimer()

	// graceful 窗口：等引用计数归零（in-flight 调用完成）
	deadline := time.Now().Add(graceful)
	for sl.refCount.Load() > 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}

	// 标记 inactive
	sl.state.Store(uint32(ServiceInactive))

	// 释放 svc 引用（GC 回收）
	sl.svcMu.Lock()
	sl.svc = nil
	sl.svcMu.Unlock()

	sl.deactivatedAt.Store(time.Now().UnixNano())
	sl.deactivateNs.Store(time.Since(start).Nanoseconds())
	return nil
}

// CallWithLifecycle 调用服务方法（自动激活 + 引用计数）。
// 这是微内核的核心入口——替代直接 kernel.Call。
//
// 行为：
//  1. 服务 inactive → 自动激活（冷启动）
//  2. 引用计数 +1
//  3. 调用 svc.Call
//  4. 引用计数 -1，触发 idle 计时器
func (sl *ServiceLifecycle) CallWithLifecycle(
	ctx ServiceContext,
	method string,
	payload any,
) (json.RawMessage, error) {
	if err := sl.ensureActive(ctx); err != nil {
		return nil, err
	}
	if !sl.acquire() {
		return nil, fmt.Errorf("kernel: %q not active", sl.name)
	}
	defer sl.release()

	sl.svcMu.RLock()
	svc := sl.svc
	sl.svcMu.RUnlock()
	if svc == nil {
		return nil, fmt.Errorf("kernel: %q svc is nil", sl.name)
	}

	childCtx := &serviceCtx{
		parent:    ctx,
		service:   sl.name,
		requestID: ctx.RequestID(),
		traceID:   ctx.TraceID(),
		created:   time.Now(),
		store:     checkpointStoreFrom(ctx),
	}

	var raw json.RawMessage
	if payload != nil {
		var err error
		raw, err = json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("kernel: marshal payload for %s.%s: %w", sl.name, method, err)
		}
	}

	start := time.Now()
	resp, err := svc.Call(childCtx, method, raw)
	elapsed := time.Since(start)
	recordCall(sl.name, method, err, elapsed)

	if err != nil {
		return nil, fmt.Errorf("kernel: %s.%s failed after %v: %w", sl.name, method, elapsed, err)
	}
	return resp, nil
}

// ServiceLifecycleStats 统计
type ServiceLifecycleStats struct {
	Name            string        `json:"name"`
	Status          string        `json:"status"`
	RefCount        int32         `json:"refCount"`
	ActivateCount   uint64        `json:"activateCount"`
	LastActivateMs  float64       `json:"lastActivateMs"`
	LastDeactivateMs float64      `json:"lastDeactivateMs"`
	ActivatedAt     time.Time     `json:"activatedAt,omitempty"`
	IdleTimeoutMs   int64         `json:"idleTimeoutMs"`
}

// Stats 返回统计
func (sl *ServiceLifecycle) Stats() ServiceLifecycleStats {
	status := ServiceStatus(sl.state.Load())
	stats := ServiceLifecycleStats{
		Name:            sl.name,
		Status:          status.String(),
		RefCount:        sl.refCount.Load(),
		ActivateCount:   sl.activateCount.Load(),
		LastActivateMs:  float64(sl.activateNs.Load()) / 1e6,
		LastDeactivateMs: float64(sl.deactivateNs.Load()) / 1e6,
		IdleTimeoutMs:   int64(sl.idleTimeout / time.Millisecond),
	}
	if status == ServiceActive {
		stats.ActivatedAt = time.Unix(0, sl.activatedAt.Load())
	}
	return stats
}
