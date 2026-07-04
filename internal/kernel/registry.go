package kernel

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"sync/atomic"
	"time"
)

// ─── Service：服务接口 ──────────────────────────────────────
//
// 实现者示例：
//
//	type SearchVectorService struct {
//	    cache *searchResultCache
//	}
//
//	func (s *SearchVectorService) Name() string { return "search.vector" }
//
//	func (s *SearchVectorService) Call(ctx ServiceContext, method string, payload json.RawMessage) (json.RawMessage, error) {
//	    switch method {
//	    case "vector":
//	        var req VectorSearchRequest
//	        if err := json.Unmarshal(payload, &req); err != nil { ... }
//	        return s.vectorSearch(ctx, req)
//	    }
//	    return nil, fmt.Errorf("unknown method %q", method)
//	}
type Service interface {
	// Name 服务名（在 bus topic / 日志 / 路由里使用）
	Name() string

	// Init 启动时调用一次（替代构造函数里的副作用）
	Init(ctx ServiceContext) error

	// Health 健康检查（用于 /api/health 聚合所有 service 状态）
	Health(ctx ServiceContext) error

	// Call 通过 JSON 调度方法。
	// payload 是入参的 json.RawMessage，返回值也是 json.RawMessage。
	// （用 json 而非泛型是避免 init cycle + 支持动态注册）
	Call(ctx ServiceContext, method string, payload json.RawMessage) (json.RawMessage, error)
}

// Register 注册一个服务。同名重复注册会 panic（开发者错误，必须修复）。
func Register(s Service) {
	if s == nil {
		panic("kernel: Register(nil)")
	}
	name := s.Name()
	if name == "" {
		panic("kernel: Register with empty Name()")
	}
	registryMu.Lock()
	defer registryMu.Unlock()
	if _, exists := registry[name]; exists {
		panic(fmt.Sprintf("kernel: service %q already registered", name))
	}
	registry[name] = s
}

// MustGet 取一个服务，找不到 panic（仅用于启动期 init 阶段）
func MustGet(name string) Service {
	s, ok := Get(name)
	if !ok {
		panic(fmt.Sprintf("kernel: service %q not registered", name))
	}
	return s
}

// Get 取一个服务
func Get(name string) (Service, bool) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	s, ok := registry[name]
	return s, ok
}

// List 列出所有已注册服务名（用于 /api/health / debug）
func List() []string {
	registryMu.RLock()
	defer registryMu.RUnlock()
	out := make([]string, 0, len(registry))
	for n := range registry {
		out = append(out, n)
	}
	return out
}

// Unregister 注销服务（测试用）
func Unregister(name string) {
	registryMu.Lock()
	defer registryMu.Unlock()
	delete(registry, name)
}

// ─── 跨服务调用的核心：Call（ctx 透传 + 错误包装） ──────────────────────

// Call 调用一个服务的指定方法。
// 自动从 ctx 派生子 ctx（service 字段更新为被调方），RequestID/TraceID 保持。
//
// 返回值是 json.RawMessage；调用方按需 Unmarshal。
// 设计取舍：kernel 不强加泛型，避免 init cycle；调用方包一层薄 wrapper（CallTyped）。
func Call(ctx ServiceContext, serviceName, method string, payload any) (json.RawMessage, error) {
	if ctx == nil {
		return nil, errors.New("kernel: nil context")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	svc, ok := Get(serviceName)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrServiceNotFound, serviceName)
	}

	// 派生子 ctx：service 字段更新为被调方
	childCtx := &serviceCtx{
		parent:    ctx,
		service:   serviceName,
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
			return nil, fmt.Errorf("kernel: marshal payload for %s.%s: %w", serviceName, method, err)
		}
	}

	start := time.Now()
	resp, err := svc.Call(childCtx, method, raw)
	elapsed := time.Since(start)

	// 埋点（轻量 metrics）
	recordCall(serviceName, method, err, elapsed)

	if err != nil {
		return nil, fmt.Errorf("kernel: %s.%s failed after %v: %w", serviceName, method, elapsed, err)
	}
	return resp, nil
}

// CallTyped 类型安全的调用 wrapper。
//
//	resp, err := kernel.CallTyped[VectorSearchRequest, VectorSearchResponse](
//	    ctx, "search.vector", "vector", req,
//	)
//
// 内部仍走 json 序列化（不可避免，因为 Service 接口的 payload 是 json.RawMessage）。
// 但调用方拿到的是结构化类型，IDE 跳转 / 重构友好。
func CallTyped[Req, Resp any](ctx ServiceContext, serviceName, method string, req Req) (Resp, error) {
	var zero Resp
	raw, err := Call(ctx, serviceName, method, req)
	if err != nil {
		return zero, err
	}
	var resp Resp
	if err := json.Unmarshal(raw, &resp); err != nil {
		return zero, fmt.Errorf("kernel: unmarshal %s.%s response: %w", serviceName, method, err)
	}
	return resp, nil
}

// ─── HealthCheck 聚合所有已注册 service 的健康状态 ────────────────

// HealthStatus 单个 service 的健康状态
type HealthStatus struct {
	Name    string        `json:"name"`
	OK      bool          `json:"ok"`
	Error   string        `json:"error,omitempty"`
	Latency time.Duration `json:"latency"`
}

// HealthAll 聚合健康检查（并行调用每个 service.Health）
// 用于 /api/health 返回所有 service 状态。
func HealthAll(ctx ServiceContext) []HealthStatus {
	services := List()
	results := make([]HealthStatus, len(services))
	var wg sync.WaitGroup
	for i, name := range services {
		i, name := i, name
		wg.Add(1)
		go func() {
			defer wg.Done()
			svc, _ := Get(name)
			start := time.Now()
			err := svc.Health(ctx)
			results[i] = HealthStatus{
				Name:    name,
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

// ─── 埋点（轻量 metrics） ─────────────────────────────────

var (
	callCount   sync.Map // key: "service.method" → *uint64
	callLatency sync.Map // key: "service.method" → *uint64 (累计 ns)
)

func recordCall(service, method string, err error, elapsed time.Duration) {
	key := service + "." + method
	cnt, _ := callCount.LoadOrStore(key, new(uint64))
	atomic.AddUint64(cnt.(*uint64), 1)
	lat, _ := callLatency.LoadOrStore(key, new(uint64))
	atomic.AddUint64(lat.(*uint64), uint64(elapsed))
}

// CallStats 返回 (count, totalLatency)
func CallStats(service, method string) (uint64, time.Duration) {
	key := service + "." + method
	cnt, _ := callCount.Load(key)
	lat, _ := callLatency.Load(key)
	var c uint64
	if cnt != nil {
		c = atomic.LoadUint64(cnt.(*uint64))
	}
	var l uint64
	if lat != nil {
		l = atomic.LoadUint64(lat.(*uint64))
	}
	return c, time.Duration(l)
}

// ─── 内部 helpers ─────────────────────────────────────────

// checkpointStoreFrom 从 parent ctx 提取 CheckpointStore
func checkpointStoreFrom(ctx ServiceContext) CheckpointStore {
	if ctx == nil {
		return nil
	}
	if sc, ok := ctx.(*serviceCtx); ok {
		return sc.store
	}
	return nil
}

// 防止 import 错误：reflect 已被 json tag 隐式使用
var _ = reflect.TypeOf
