package kernel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"
)

// ─── MicroKernel：微内核运行时 ────────────────────────────────────────────
//
// 2026-07-03 spec microkernel-split-start-stop Phase 4.2
//
// 设计定位：
//   微内核 = 服务注册中心 + 服务级生命周期管理 + 租户隔离 + 工具调度
//
// 与全局 registry 的区别：
//   - 全局 registry（kernel.Register）：静态注册，进程启动时全部注册，一直活着
//   - MicroKernel：动态生命周期管理，服务按需激活/停用，支持多租户
//
// 核心能力：
//   1. RegisterService(name, factory)：注册服务工厂（不实例化，占内存极小）
//   2. Call(ctx, service, method, payload)：调用服务（按需激活 + 引用计数）
//   3. Activate(service) / Deactivate(service)：手动控制服务生命周期
//   4. ListServices()：列出所有服务 + 状态 + 引用计数
//   5. 多租户：tenant 维度的资源配额 + 并发限制
//
// 冷启动目标：单个服务激活 ≤ 100ms（大部分服务是纯内存构造 + Init）
// 整内核冷启动（3 个服务）：≤ 500ms
type MicroKernel struct {
	name   string
	services map[string]*ServiceLifecycle // name → lifecycle
	servicesMu sync.RWMutex

	tenants map[string]*TenantContext // tenantID → tenant
	tenantsMu sync.RWMutex

	memGuard *MemoryGuard
	memGuardMB int

	defaultIdleTimeout time.Duration
	defaultGraceful    time.Duration

	// taskRecorder 任务记录器配置（nil = 不记录）。
	// 非 nil 时，所有 Call 调用都会被记录为任务写入 Store。
	taskRecorder *TaskRecordingConfig
}

// MicroKernelConfig 微内核配置
type MicroKernelConfig struct {
	Name               string
	MemGuardMB         int           // 内存守卫阈值（0 = 不启用）
	DefaultIdleTimeout time.Duration // 默认空闲超时（0 = 不自动停用）
	DefaultGraceful    time.Duration // 默认优雅停用窗口
}

// NewMicroKernel 构造微内核（初始无服务，占内存极小）
func NewMicroKernel(cfg MicroKernelConfig) *MicroKernel {
	if cfg.DefaultGraceful <= 0 {
		cfg.DefaultGraceful = 100 * time.Millisecond
	}
	mk := &MicroKernel{
		name:               cfg.Name,
		services:           make(map[string]*ServiceLifecycle),
		tenants:            make(map[string]*TenantContext),
		memGuardMB:         cfg.MemGuardMB,
		defaultIdleTimeout: cfg.DefaultIdleTimeout,
		defaultGraceful:    cfg.DefaultGraceful,
	}
	if cfg.MemGuardMB > 0 {
		mk.memGuard = NewMemoryGuard(cfg.MemGuardMB)
		mk.memGuard.Start()
	}
	return mk
}

// Name 返回微内核名称
func (mk *MicroKernel) Name() string {
	return mk.name
}

// EnableTaskRecording 启用任务记录（将所有微服务调用写入任务系统）。
// store 为 nil 时不启用。可以在运行时调用 nil 来关闭。
func (mk *MicroKernel) EnableTaskRecording(cfg *TaskRecordingConfig) {
	if cfg != nil && cfg.Store == nil {
		return
	}
	mk.taskRecorder = cfg
}

// TaskRecordingEnabled 返回是否启用了任务记录。
func (mk *MicroKernel) TaskRecordingEnabled() bool {
	return mk.taskRecorder != nil && mk.taskRecorder.Store != nil
}

// RegisterService 注册一个服务工厂（不实例化）。
// 同名重复注册会 panic（开发者错误）。
// 注册本身开销极小（只存 factory 函数指针）。
func (mk *MicroKernel) RegisterService(name string, factory ServiceFactory) {
	if name == "" {
		panic("kernel: RegisterService with empty name")
	}
	if factory == nil {
		panic("kernel: RegisterService with nil factory")
	}
	mk.servicesMu.Lock()
	defer mk.servicesMu.Unlock()
	if _, exists := mk.services[name]; exists {
		panic(fmt.Sprintf("kernel: service %q already registered", name))
	}
	mk.services[name] = NewServiceLifecycle(ServiceLifecycleConfig{
		Name:        name,
		Factory:     factory,
		IdleTimeout: mk.defaultIdleTimeout,
		Graceful:    mk.defaultGraceful,
	})
}

// GetServiceLifecycle 获取服务生命周期（用于手动控制）
func (mk *MicroKernel) GetServiceLifecycle(name string) (*ServiceLifecycle, bool) {
	mk.servicesMu.RLock()
	defer mk.servicesMu.RUnlock()
	sl, ok := mk.services[name]
	return sl, ok
}

// Call 调用服务方法（微内核核心入口）。
//
// 自动处理：
//   - 服务未激活 → 自动激活（冷启动）
//   - 引用计数 +1/-1
//   - 租户配额检查（如果 ctx 带 tenant）
//   - ctx 透传 + 子 ctx 派生
func (mk *MicroKernel) Call(ctx ServiceContext, serviceName, method string, payload any) (json.RawMessage, error) {
	if ctx == nil {
		return nil, errors.New("kernel: nil context")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// 内存守卫检查
	if mk.memGuard != nil && mk.memGuard.Triggered() {
		return nil, errors.New("kernel: memory guard triggered, rejecting new calls")
	}

	// 租户配额检查
	tenantID := TenantFromContext(ctx)
	if tenantID != "" {
		if err := mk.checkTenantQuota(tenantID, serviceName); err != nil {
			return nil, err
		}
	}

	mk.servicesMu.RLock()
	sl, ok := mk.services[serviceName]
	mk.servicesMu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrServiceNotFound, serviceName)
	}

	callFn := func() (json.RawMessage, error) {
		return sl.CallWithLifecycle(ctx, method, payload)
	}

	return mk.recordMicroserviceTask(ctx, serviceName, method, payload, callFn)
}

// MkCallTyped 类型安全的调用 wrapper（MicroKernel 版本）
func MkCallTyped[Req, Resp any](
	ctx ServiceContext,
	mk *MicroKernel,
	serviceName, method string,
	req Req,
) (Resp, error) {
	var zero Resp
	raw, err := mk.Call(ctx, serviceName, method, req)
	if err != nil {
		return zero, err
	}
	var resp Resp
	if err := json.Unmarshal(raw, &resp); err != nil {
		return zero, fmt.Errorf("kernel: unmarshal %s.%s response: %w", serviceName, method, err)
	}
	return resp, nil
}

// Activate 手动激活服务（预热）
func (mk *MicroKernel) Activate(ctx context.Context, name string) error {
	sl, ok := mk.GetServiceLifecycle(name)
	if !ok {
		return fmt.Errorf("%w: %s", ErrServiceNotFound, name)
	}
	return sl.Activate(ctx)
}

// Deactivate 手动停用服务
func (mk *MicroKernel) Deactivate(name string, graceful time.Duration) error {
	sl, ok := mk.GetServiceLifecycle(name)
	if !ok {
		return fmt.Errorf("%w: %s", ErrServiceNotFound, name)
	}
	return sl.Deactivate(graceful)
}

// DeactivateAll 停用所有服务（整内核优雅关闭）
func (mk *MicroKernel) DeactivateAll(graceful time.Duration) error {
	if graceful <= 0 {
		graceful = mk.defaultGraceful
	}
	mk.servicesMu.RLock()
	names := make([]string, 0, len(mk.services))
	for name := range mk.services {
		names = append(names, name)
	}
	mk.servicesMu.RUnlock()

	var wg sync.WaitGroup
	for _, name := range names {
		wg.Add(1)
		go func(n string) {
			defer wg.Done()
			if sl, ok := mk.GetServiceLifecycle(n); ok {
				_ = sl.Deactivate(graceful)
			}
		}(name)
	}
	wg.Wait()

	if mk.memGuard != nil {
		mk.memGuard.Stop()
	}
	return nil
}

// ListServices 列出所有服务 + 状态
func (mk *MicroKernel) ListServices() []ServiceLifecycleStats {
	mk.servicesMu.RLock()
	defer mk.servicesMu.RUnlock()
	out := make([]ServiceLifecycleStats, 0, len(mk.services))
	for _, sl := range mk.services {
		out = append(out, sl.Stats())
	}
	return out
}

// HealthAll 聚合所有已激活服务的健康检查
func (mk *MicroKernel) HealthAll(ctx ServiceContext) []HealthStatus {
	services := mk.ListServices()
	activeNames := make([]string, 0)
	for _, s := range services {
		if s.Status == "active" {
			activeNames = append(activeNames, s.Name)
		}
	}

	results := make([]HealthStatus, len(services))
	var wg sync.WaitGroup
	for i, s := range services {
		i, s := i, s
		wg.Add(1)
		go func() {
			defer wg.Done()
			if s.Status != "active" {
				results[i] = HealthStatus{
					Name:  s.Name,
					OK:    true, // inactive 不算不健康
					Latency: 0,
					Error: "",
				}
				return
			}
			sl, ok := mk.GetServiceLifecycle(s.Name)
			if !ok {
				results[i] = HealthStatus{Name: s.Name, OK: false, Error: "not found"}
				return
			}
			sl.svcMu.RLock()
			svc := sl.svc
			sl.svcMu.RUnlock()
			if svc == nil {
				results[i] = HealthStatus{Name: s.Name, OK: false, Error: "svc nil"}
				return
			}
			start := time.Now()
			err := svc.Health(ctx)
			results[i] = HealthStatus{
				Name:    s.Name,
				OK:      err == nil,
				Latency: time.Since(start),
			}
			if err != nil {
				results[i].Error = err.Error()
			}
		}()
	}
	wg.Wait()
	return results
}

// checkTenantQuota 检查租户配额
func (mk *MicroKernel) checkTenantQuota(tenantID, serviceName string) error {
	if tenantID == "" {
		return nil
	}
	tc := mk.getOrCreateTenant(tenantID)
	return tc.Allocate(serviceName, 1)
}

func (mk *MicroKernel) getOrCreateTenant(tenantID string) *TenantContext {
	mk.tenantsMu.RLock()
	tc, ok := mk.tenants[tenantID]
	mk.tenantsMu.RUnlock()
	if ok {
		return tc
	}

	mk.tenantsMu.Lock()
	defer mk.tenantsMu.Unlock()
	if tc, ok = mk.tenants[tenantID]; ok {
		return tc
	}
	tc = NewTenantContext(TenantConfig{
		ID:          tenantID,
		MaxConcurrency: 10, // 默认单租户最大并发 10
	})
	mk.tenants[tenantID] = tc
	return tc
}

// GetTenant 获取租户上下文
func (mk *MicroKernel) GetTenant(tenantID string) (*TenantContext, bool) {
	mk.tenantsMu.RLock()
	defer mk.tenantsMu.RUnlock()
	tc, ok := mk.tenants[tenantID]
	return tc, ok
}

// ListTenants 列出所有租户
func (mk *MicroKernel) ListTenants() []TenantStats {
	mk.tenantsMu.RLock()
	defer mk.tenantsMu.RUnlock()
	out := make([]TenantStats, 0, len(mk.tenants))
	for _, tc := range mk.tenants {
		out = append(out, tc.Stats())
	}
	return out
}

// MemGuard 返回内存守卫
func (mk *MicroKernel) MemGuard() *MemoryGuard {
	return mk.memGuard
}
