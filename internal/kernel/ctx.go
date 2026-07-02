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

// ─── ServiceContext：跨服务 ctx 一等公民 ──────────────────────────────
//
// 关键设计：ServiceContext 是一个完整的 context.Context，
// 任何需要 ctx 的地方（DB 查询、HTTP 调用、tool 调度、worker 提交）都应传它。
//
// 5 个能力（与普通 context.Context 的区别）：
//
//  1. Service()：当前所在服务名（用于日志、bus topic 路由、debug）
//  2. RequestID()：本次请求全链路唯一 ID（gin request → agent tool → DB 共享）
//  3. TraceID()：跨多次请求的追踪 ID（agent multi-turn、WorkManager 重试链）
//  4. Budget()：剩余 deadline（自动扣减 elapsed），调用方决定是否继续
//  5. Checkpoint(name, state) / Restore(name, dst)：WorkManager 风格断点续跑
type ServiceContext interface {
	context.Context

	// Service 当前所在的服务名。例：ctx 进入 search.vector 服务后，
	// ctx.Service() 返回 "search.vector"。日志/slog 默认带此字段。
	Service() string

	// RequestID 单次请求 ID（gin 一次 request 一个）。
	// AI agent 多轮对话的每轮请求应共享同一 TraceID、不同 RequestID。
	RequestID() string

	// TraceID 跨多次请求的追踪 ID（agent session ID、WorkManager chain ID）。
	TraceID() string

	// Budget 剩余 deadline。等于 ctx.Deadline() - now()。
	// 调用方可在 Budget < 阈值时快速 fail（如 vector search 100ms budget 时跳过缓存检查）。
	Budget() time.Duration

	// Elapsed 从 ctx 创建到现在经过的时间。
	Elapsed() time.Duration

	// Checkpoint 把 state 序列化到 ctx 关联的 store。WorkManager 杀进程后下次启动 Restore 继续。
	// 返回 ErrCheckpointUnsupported 表示当前 ctx 不支持（gin request ctx 通常不支持）。
	Checkpoint(name string, state any) error

	// Restore 读取上次 Checkpoint 写入的 state（json.Unmarshal 到 dst）。
	Restore(name string, dst any) error
}

// ─── 私有 ctx 实现 ────────────────────────────────────────

type serviceCtx struct {
	parent    context.Context
	service   string
	requestID string
	traceID   string
	created   time.Time

	// 可选 checkpoint store（nil = 不支持）
	store   CheckpointStore
	storeMu sync.Mutex // 序列化 checkpoint/restore 调用
}

// ContextOption ctx 构造选项
type ContextOption func(*serviceCtx)

// WithServiceName 显式指定服务名（默认 "kernel"）
func WithServiceName(name string) ContextOption {
	return func(c *serviceCtx) { c.service = name }
}

// WithTraceID 显式指定 TraceID（默认 "trace-<id>"）
func WithTraceID(id string) ContextOption {
	return func(c *serviceCtx) { c.traceID = id }
}

// WithCheckpointStore 启用 checkpoint（用于 WorkManager 风格断点续跑）
func WithCheckpointStore(store CheckpointStore) ContextOption {
	return func(c *serviceCtx) { c.store = store }
}

// NewContext 创建一个 ServiceContext。
//
// 用法 1（gin handler）：
//
//	ctx := kernel.NewContext(c.Request.Context(), "search.vector")
//	kernel.Call[VectorSearchRequest, VectorSearchResponse](ctx, "search.vector", req)
//
// 用法 2（agent tool）：
//
//	func (t *SearchTool) Invoke(ctx kernel.ServiceContext, args json.RawMessage) (json.RawMessage, error) {
//	    innerCtx := kernel.NewContext(ctx, "search.vector") // 保留 RequestID/TraceID
//	    return kernel.CallTyped[SearchReq, SearchResp](innerCtx, "search.vector", req)
//	}
//
// 用法 3（WorkManager 风格）：
//
//	store := kernel.NewFileCheckpointStore("/tmp/agent-checkpoints")
//	ctx := kernel.NewContext(context.Background(),
//	    kernel.WithServiceName("agent.chat"),
//	    kernel.WithCheckpointStore(store),
//	)
func NewContext(parent context.Context, opts ...ContextOption) ServiceContext {
	if parent == nil {
		parent = context.Background()
	}
	c := &serviceCtx{
		parent:  parent,
		service: "kernel",
		created: time.Now(),
	}
	// 继承父 ctx 的 RequestID / TraceID（若父是 ServiceContext）
	// 父是普通 context.Context 时，生成新的
	if psc, ok := parent.(ServiceContext); ok {
		c.requestID = nextID()           // RequestID 总是新生成（每次 request 唯一）
		c.traceID = psc.TraceID()        // TraceID 继承（跨请求追踪）
	} else {
		c.requestID = nextID()
		c.traceID = "trace-" + nextID()
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// FromContext 从一个 context.Context 中提取 ServiceContext。
// 如果 ctx 本身是 ServiceContext，直接返回；否则包装为新的。
// 用于库的边界（接受 context.Context 但内部用 ServiceContext 的能力）。
func FromContext(ctx context.Context) ServiceContext {
	if ctx == nil {
		return NewContext(context.Background())
	}
	if sc, ok := ctx.(ServiceContext); ok {
		return sc
	}
	return NewContext(ctx)
}

// ─── ServiceContext 接口实现 ───────────────────────────────

func (c *serviceCtx) Deadline() (time.Time, bool) { return c.parent.Deadline() }
func (c *serviceCtx) Done() <-chan struct{}       { return c.parent.Done() }
func (c *serviceCtx) Err() error                  { return c.parent.Err() }

func (c *serviceCtx) Value(key any) any {
	if k, ok := key.(ctxKey); ok {
		switch k {
		case keyService:
			return c.service
		case keyRequestID:
			return c.requestID
		case keyTraceID:
			return c.traceID
		}
	}
	return c.parent.Value(key)
}

func (c *serviceCtx) Service() string        { return c.service }
func (c *serviceCtx) RequestID() string      { return c.requestID }
func (c *serviceCtx) TraceID() string        { return c.traceID }
func (c *serviceCtx) Elapsed() time.Duration { return time.Since(c.created) }

func (c *serviceCtx) Budget() time.Duration {
	dl, ok := c.parent.Deadline()
	if !ok {
		return time.Hour * 24 * 365 // 无 deadline → 假装一年
	}
	remaining := time.Until(dl)
	if remaining < 0 {
		return 0
	}
	return remaining
}

func (c *serviceCtx) Checkpoint(name string, state any) error {
	if c.store == nil {
		return ErrCheckpointUnsupported
	}
	c.storeMu.Lock()
	defer c.storeMu.Unlock()
	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("kernel: marshal checkpoint %q: %w", name, err)
	}
	return c.store.Put(c.traceID, name, data)
}

func (c *serviceCtx) Restore(name string, dst any) error {
	if c.store == nil {
		return ErrCheckpointUnsupported
	}
	c.storeMu.Lock()
	defer c.storeMu.Unlock()
	data, err := c.store.Get(c.traceID, name)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, dst)
}

// ─── context.Value key ────────────────────────────────────

type ctxKey int

const (
	keyService ctxKey = iota + 1
	keyRequestID
	keyTraceID
)

// ─── CheckpointStore ──────────────────────────────────────

// CheckpointStore checkpoint 持久化（WorkManager 风格）
// 内置实现：FileCheckpointStore（写到磁盘），MemoryCheckpointStore（测试用）
type CheckpointStore interface {
	Put(traceID, name string, data []byte) error
	Get(traceID, name string) ([]byte, error)
}

// NewMemoryCheckpointStore 内存版（测试 / 短期任务）
func NewMemoryCheckpointStore() CheckpointStore {
	return &memStore{data: map[string]map[string][]byte{}}
}

type memStore struct {
	mu   sync.Mutex
	data map[string]map[string][]byte
}

func (m *memStore) Put(traceID, name string, data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.data[traceID] == nil {
		m.data[traceID] = map[string][]byte{}
	}
	m.data[traceID][name] = data
	return nil
}

func (m *memStore) Get(traceID, name string) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	scope, ok := m.data[traceID]
	if !ok {
		return nil, errors.New("kernel: no checkpoints for trace " + traceID)
	}
	data, ok := scope[name]
	if !ok {
		return nil, errors.New("kernel: no checkpoint " + name)
	}
	return data, nil
}

// ─── 便捷方法 ─────────────────────────────────────────────

// BudgetFromContext 快速从一个普通 context 拿剩余 deadline。
// 用于尚未迁移到 ServiceContext 的旧代码。
func BudgetFromContext(ctx context.Context) time.Duration {
	if ctx == nil {
		return time.Hour * 24 * 365
	}
	if sc, ok := ctx.(ServiceContext); ok {
		return sc.Budget()
	}
	dl, ok := ctx.Deadline()
	if !ok {
		return time.Hour * 24 * 365
	}
	remaining := time.Until(dl)
	if remaining < 0 {
		return 0
	}
	return remaining
}

// 内部：原子计数（ctx id 序号）
var ctxSeq uint64

func init() {
	// 占位，确保 import 顺序
	_ = atomic.AddUint64
}
