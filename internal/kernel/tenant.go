package kernel

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// ─── TenantContext：多租户隔离 ────────────────────────────────────────────
//
// 2026-07-03 spec microkernel-split-start-stop Phase 4.3
//
// 设计目标：
//   1. 租户间资源隔离：每个租户有独立的并发上限、调用计数
//   2. 轻量级：不创建独立 goroutine / 独立内存池，只做计数 + 配额
//   3. 可观测：每个租户的调用次数、活跃并发、最近活动时间
//   4. 自动过期：长时间不活动的租户自动清理（避免内存泄漏）
//
// 与 ServiceLifecycle 的关系：
//   - ServiceLifecycle 管"服务级"的激活/停用（服务实例维度）
//   - TenantContext 管"租户级"的配额/隔离（逻辑租户维度）
//   - 两者正交：一个服务可以被多个租户使用，每个租户有自己的配额
//
// ctx key：使用 context.Value 传递 tenantID，业务代码不需要显式传 tenant
type tenantCtxKey struct{}

// TenantConfig 租户配置
type TenantConfig struct {
	ID             string
	MaxConcurrency int           // 最大并发调用数（0 = 不限制）
	MaxCallsPerMin int           // 每分钟最大调用数（0 = 不限制）
	IdleTimeout    time.Duration // 空闲超时自动清理（0 = 不自动清理）
}

// TenantContext 租户上下文
type TenantContext struct {
	id             string
	maxConcurrency int
	maxCallsPerMin int

	activeConcurrency atomic.Int32  // 当前活跃并发数
	totalCalls        atomic.Uint64 // 总调用数
	lastActivity      atomic.Int64  // 最后活动时间（unix nano）

	// 每分钟调用计数（滑动窗口）
	callWindowMu sync.Mutex
	callWindow   []time.Time

	// 服务级配额（可选）
	serviceQuotas   map[string]int  // serviceName → max concurrency
	serviceQuotasMu sync.RWMutex
}

// NewTenantContext 构造租户上下文
func NewTenantContext(cfg TenantConfig) *TenantContext {
	if cfg.MaxConcurrency <= 0 {
		cfg.MaxConcurrency = 100 // 默认 100 并发
	}
	tc := &TenantContext{
		id:             cfg.ID,
		maxConcurrency: cfg.MaxConcurrency,
		maxCallsPerMin: cfg.MaxCallsPerMin,
		serviceQuotas:  make(map[string]int),
	}
	tc.lastActivity.Store(time.Now().UnixNano())
	return tc
}

// ID 租户 ID
func (tc *TenantContext) ID() string {
	return tc.id
}

// Allocate 分配一个调用配额。
// 返回 nil 表示配额可用，调用完成后必须调用 Release()
func (tc *TenantContext) Allocate(serviceName string, count int) error {
	// 全局并发限制
	if tc.maxConcurrency > 0 {
		current := tc.activeConcurrency.Load()
		if current >= int32(tc.maxConcurrency) {
			return fmt.Errorf("kernel: tenant %q concurrency limit reached (%d/%d)",
				tc.id, current, tc.maxConcurrency)
		}
	}

	// 每分钟调用限制
	if tc.maxCallsPerMin > 0 {
		if err := tc.checkRateLimit(); err != nil {
			return err
		}
	}

	tc.activeConcurrency.Add(int32(count))
	tc.totalCalls.Add(uint64(count))
	tc.lastActivity.Store(time.Now().UnixNano())
	return nil
}

// Release 释放一个调用配额
func (tc *TenantContext) Release(serviceName string, count int) {
	tc.activeConcurrency.Add(int32(-count))
	tc.lastActivity.Store(time.Now().UnixNano())
}

func (tc *TenantContext) checkRateLimit() error {
	tc.callWindowMu.Lock()
	defer tc.callWindowMu.Unlock()

	now := time.Now()
	cutoff := now.Add(-1 * time.Minute)

	// 清理 1 分钟前的记录
	valid := tc.callWindow[:0]
	for _, t := range tc.callWindow {
		if t.After(cutoff) {
			valid = append(valid, t)
		}
	}
	tc.callWindow = valid

	if len(tc.callWindow) >= tc.maxCallsPerMin {
		return fmt.Errorf("kernel: tenant %q rate limit reached (%d calls/min)",
			tc.id, tc.maxCallsPerMin)
	}

	tc.callWindow = append(tc.callWindow, now)
	return nil
}

// SetServiceQuota 设置服务级配额
func (tc *TenantContext) SetServiceQuota(serviceName string, maxConcurrency int) {
	tc.serviceQuotasMu.Lock()
	defer tc.serviceQuotasMu.Unlock()
	tc.serviceQuotas[serviceName] = maxConcurrency
}

// ActiveConcurrency 当前活跃并发数
func (tc *TenantContext) ActiveConcurrency() int32 {
	return tc.activeConcurrency.Load()
}

// TotalCalls 总调用数
func (tc *TenantContext) TotalCalls() uint64 {
	return tc.totalCalls.Load()
}

// LastActivity 最后活动时间
func (tc *TenantContext) LastActivity() time.Time {
	return time.Unix(0, tc.lastActivity.Load())
}

// TenantStats 租户统计
type TenantStats struct {
	ID               string    `json:"id"`
	ActiveConcurrency int32    `json:"activeConcurrency"`
	TotalCalls       uint64    `json:"totalCalls"`
	LastActivity     time.Time `json:"lastActivity"`
	MaxConcurrency   int       `json:"maxConcurrency"`
	MaxCallsPerMin   int       `json:"maxCallsPerMin"`
}

// Stats 返回统计
func (tc *TenantContext) Stats() TenantStats {
	return TenantStats{
		ID:               tc.id,
		ActiveConcurrency: tc.activeConcurrency.Load(),
		TotalCalls:       tc.totalCalls.Load(),
		LastActivity:     time.Unix(0, tc.lastActivity.Load()),
		MaxConcurrency:   tc.maxConcurrency,
		MaxCallsPerMin:   tc.maxCallsPerMin,
	}
}

// ─── Context 辅助函数 ────────────────────────────────────────────────────

// WithTenant 把租户 ID 写入 ctx
func WithTenant(ctx context.Context, tenantID string) context.Context {
	return context.WithValue(ctx, tenantCtxKey{}, tenantID)
}

// TenantFromContext 从 ctx 读取租户 ID
func TenantFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if id, ok := ctx.Value(tenantCtxKey{}).(string); ok {
		return id
	}
	return ""
}

// ErrTenantQuotaExceeded 租户配额超限
var ErrTenantQuotaExceeded = errors.New("kernel: tenant quota exceeded")
