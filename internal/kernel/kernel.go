// Package kernel 提供 in-process 微内核，**不依赖端口**，
// 跨服务 ctx 一等公民，支持 WorkManager 风格断点续跑 + AI tool 调用。
//
// 设计原则：
//
//  1. **无端口**：服务间调用全在进程内（method call + channel）。
//     传统微服务内核依赖 TCP 端口（HTTP/gRPC），端口分配/释放不可靠、
//     跨平台差异大（Android 上尤其头疼），本内核完全规避。
//
//  2. **ctx 一等**：每次跨服务调用都强制传 ServiceContext，
//     cancel / deadline / values 全链路自动传播。
//     AI agent tool 调用、WorkManager 重启、gin 请求取消，共享同一 ctx 链。
//
//  3. **可观测**：
//     - ServiceName() 标识当前服务（debug / 日志 / bus 过滤用）
//     - RequestID / TraceID 全链路追踪
//     - Budget() 剩余 deadline（自动扣减），调用方知道还剩多少时间
//
//  4. **可断点续跑**（WorkManager 风格）：
//     - Checkpoint(name, state) 把中间状态序列化到 ctx 关联的 store
//     - Restore(name, dst) 读取上次 checkpoint
//     - WorkManager 杀进程后下次启动自动 Restore 继续
//
//  5. **不替代 gin**：gin 仍是 HTTP 边界，handler 内调用 kernel.Call/InvokeTool
//     完成业务逻辑。新增内部服务（agent 工具、文件索引、自动化测试）走 kernel。
//
// 关键类型：
//
//   - ServiceContext：扩展 context.Context，加 ServiceName/RequestID/Budget/Checkpoint
//   - Service：服务接口（Init/Health/Call）
//   - Tool：AI agent tool 接口
//   - Pool：WorkManager 风格 worker pool
//   - Bus：in-process pub/sub
//
// 调用示例（handler 改造）：
//
//	// 旧：s := s.searchSvc.VectorSearch(...)
//	// 新：
//	resp, err := kernel.Call[VectorSearchRequest, VectorSearchResponse](
//	    ctx, "search.vector", req,
//	)
package kernel

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

// ErrServiceNotFound 服务未注册
var ErrServiceNotFound = errors.New("kernel: service not found")

// ErrToolNotFound 工具未注册
var ErrToolNotFound = errors.New("kernel: tool not found")

// ErrCheckpointUnsupported ctx 不支持 checkpoint
var ErrCheckpointUnsupported = errors.New("kernel: checkpoint not supported in this ctx")

// ─── 内部全局状态（无端口，进程内） ──────────────────────────────

var (
	registryMu sync.RWMutex
	registry   = map[string]Service{}

	toolMu sync.RWMutex
	tools  = map[string]Tool{}

	busMu       sync.RWMutex
	subscribers = map[string][]busSub{}

	poolsMu sync.RWMutex
	pools   = map[string]*Pool{}

	// 全局序列号：RequestID / TraceID 生成
	seqCounter uint64
)

type busSub struct {
	id  string
	fn  func(Event) error
	ctx context.Context // 控制订阅生命周期
}

// 内部 helper

func nextID() string {
	n := atomic.AddUint64(&seqCounter, 1)
	return formatID(n, time.Now())
}

func formatID(n uint64, t time.Time) string {
	return t.UTC().Format("20060102T150405.000000000Z") + "-" + uintToBase36(n)
}

func uintToBase36(n uint64) string {
	const digits = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	if n == 0 {
		return "0"
	}
	var buf [16]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = digits[n%36]
		n /= 36
	}
	return string(buf[i:])
}
