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

// ─── Pool：WorkManager 风格 worker pool（ctx-cancelable + checkpointable） ───────
//
// 关键设计（与 Android WorkManager 对齐）：
//
//  1. **可取消**：每个 Job 都有自己的 ctx（从 ServiceContext 派生子 ctx），
//     pool 关闭时所有 in-flight job 自动 cancel。
//
//  2. **可断点续跑**：Job 内部可调用 ctx.Checkpoint(name, state) 序列化中间状态。
//     WorkManager 杀进程后，pool 启动时从 checkpoint store 恢复上次未完成的 job。
//     （本骨架先实现提交/等待，restore 由上层 Playbook/ReplayJob 调）
//
//  3. **有界并发**：固定 worker 数 + buffered queue，避免 goroutine 爆炸。
//
//  4. **ctx 跨 worker 传播**：worker 收到的 ServiceContext 完整保留
//     RequestID/TraceID/Budget，便于日志聚合和 deadline 预算。
//
//  5. **失败重试**：Job 配置 maxRetries，失败后自动 backoff 重试（指数 backoff + ctx cancel 立即放弃）。
//
// 用法：
//
//	pool := kernel.NewPool("encrypt", 4, kernel.PoolConfig{QueueSize: 100})
//	pool.Start(ctx)
//	defer pool.Stop()
//
//	pool.Submit(ctx, kernel.Job{
//	    ID:      "encrypt-123",
//	    Service: "encrypt.video",
//	    Method:  "encrypt",
//	    Payload: req,
//	})
//
// 与 gin handler 的连接：
//
//	func handleEncrypt(c *gin.Context) {
//	    ctx := kernel.NewContext(c.Request.Context(), "encrypt.handler")
//	    job := kernel.Job{...}
//	    if err := encryptPool.Submit(ctx, job); err != nil {
//	        c.JSON(500, gin.H{"error": err.Error()})
//	        return
//	    }
//	    c.JSON(202, gin.H{"jobId": job.ID})
//	}
type Job struct {
	ID       string          // 业务 ID（用于日志 / checkpoint key）
	Service  string          // 调用的 service name
	Method   string          // 调用的 method
	Payload  any             // 入参（json.Marshal-able）
	MaxRetry int             // 最大重试次数（0 = 不重试）
	OnDone   func(JobResult) `json:"-"` // 回调（异步执行；nil = 不通知）；不参与序列化
}

type JobResult struct {
	Job    Job
	Result json.RawMessage
	Err    error
}

// PoolConfig pool 配置
type PoolConfig struct {
	QueueSize     int           // 队列容量（0 = unbuffered）
	JobTimeout    time.Duration // 单 job 超时（0 = 不超时，依赖 ctx deadline）
	RetryBackoff  time.Duration // 重试 backoff 基础（指数：base, 2*base, 4*base, ...）
	RetryMaxDelay time.Duration // 重试 backoff 上限

	// Ledger 可选 job 账本（启用后 Submit/execute 自动记录状态，支持断点续跑）。
	// nil = 不持久化（默认，与原行为兼容）。
	// 非 nil = Submit 时写 "submitted"，execute 完成时写 "done"/"failed"/"cancelled"，
	//   pool.Restore() 可加载未完成 job 并重投。
	Ledger JobLedger
}

// DefaultPoolConfig 默认配置
func DefaultPoolConfig() PoolConfig {
	return PoolConfig{
		QueueSize:     64,
		JobTimeout:    5 * time.Minute,
		RetryBackoff:  500 * time.Millisecond,
		RetryMaxDelay: 30 * time.Second,
	}
}

// Pool 协程安全的 worker pool
type Pool struct {
	name string
	size int
	cfg  PoolConfig

	jobCh  chan jobEnvelope
	wg     sync.WaitGroup
	closed atomic.Bool // 是否已 Stop（防止 retry goroutine 写已关闭 channel）

	ctx    context.Context
	cancel context.CancelFunc

	// metrics
	submitted uint64
	finished  uint64
	failed    uint64
	retried   uint64

	// Lifecycle 钩子（可选）：Stop 时把 in-flight job 委托给 Ledger
	lastRestoreCount int
	lastRestoreAt    time.Time
}

type jobEnvelope struct {
	ctx      ServiceContext
	job      Job
	attempts int
}

// NewPool 构造 pool（未启动）
func NewPool(name string, size int, cfg PoolConfig) *Pool {
	if size < 1 {
		size = 1
	}
	if cfg.QueueSize < 1 {
		cfg.QueueSize = 1
	}
	if cfg.RetryBackoff == 0 {
		cfg.RetryBackoff = 500 * time.Millisecond
	}
	if cfg.RetryMaxDelay == 0 {
		cfg.RetryMaxDelay = 30 * time.Second
	}
	return &Pool{
		name:  name,
		size:  size,
		cfg:   cfg,
		jobCh: make(chan jobEnvelope, cfg.QueueSize),
	}
}

// Start 启动 worker。ctx 用于控制 pool 整体生命周期。
func (p *Pool) Start(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}
	p.ctx, p.cancel = context.WithCancel(ctx)
	for i := 0; i < p.size; i++ {
		p.wg.Add(1)
		go p.worker(i)
	}
}

// Stop 优雅关闭（等待 in-flight job 完成，拒收新 job）
func (p *Pool) Stop() {
	if !p.closed.CompareAndSwap(false, true) {
		return // 已关闭（幂等）
	}
	if p.cancel != nil {
		p.cancel()
	}
	close(p.jobCh)
	p.wg.Wait()
}

// StopWithTimeout 限时优雅关闭：先给 in-flight job 一个 grace 窗口自然完成，
// 超时后强制 cancel + 把未完成 job 委托给 Ledger（供下次 Restore 续跑）。
//
// 用于 Lifecycle.Stop(graceful) — 满足"停止 ≤ 200ms"硬指标。
//
// grace ≤ 0 时等价于 Stop()（立即 cancel + 等待 worker drain）。
// 注意：即使 grace=0，worker 也会因为 ctx 被 cancel 而快速退出（假设 svc 尊重 ctx）。
func (p *Pool) StopWithTimeout(grace time.Duration) error {
	if !p.closed.CompareAndSwap(false, true) {
		return nil // 已关闭（幂等）
	}

	// 阶段 1：标记 closed，新 Submit 立即返回 ErrPoolClosed
	// （closed 已由 CAS 设置）

	// 阶段 2：给 in-flight job 一个 grace 窗口（不 cancel ctx，让它们自然完成）
	if grace > 0 {
		waitCh := make(chan struct{})
		go func() {
			p.wg.Wait() // 等所有 worker 退出（但 worker 此时还在跑 job，不会退出）
			close(waitCh)
		}()
		// 这里不真的等 wg，而是等 grace 超时
		// worker 在 grace 期间继续处理 job，自然完成的算"完成"
		// grace 超时后我们 cancel ctx，worker 收到 ctx.Done() 后退出
		select {
		case <-waitCh:
			// 所有 worker 在 grace 内自然退出（罕见，pool 空闲时）
			close(p.jobCh)
			return nil
		case <-time.After(grace):
			// 进入强制阶段
		}
	}

	// 阶段 3：把 jobCh 中尚未处理的 job 委托给 Ledger（如果有 Ledger）
	if p.cfg.Ledger != nil {
		p.drainPendingToLedger()
	}

	// 阶段 4：cancel ctx（worker 收到后立即退出）
	if p.cancel != nil {
		p.cancel()
	}

	// 阶段 5：关闭 jobCh + 等 worker
	close(p.jobCh)
	p.wg.Wait()
	return nil
}

// drainPendingToLedger 把 jobCh 中尚未被 worker 消费的 job 写回 Ledger（status=submitted）。
// 调用前提：pool 已标记 closed（新 Submit 被拒绝），且 worker 还没被 cancel。
func (p *Pool) drainPendingToLedger() {
	if p.cfg.Ledger == nil {
		return
	}
	for {
		select {
		case env, ok := <-p.jobCh:
			if !ok {
				return
			}
			traceID := env.ctx.TraceID()
			if traceID == "" {
				continue
			}
			stored := StoredJob{
				TraceID:  traceID,
				PoolName: p.name,
				Job:      env.job,
				Status:   JobStatusSubmitted,
				SavedAt:  time.Now(),
				Attempts: env.attempts,
			}
			if err := p.cfg.Ledger.SaveJob(traceID, stored); err != nil {
				fmt.Printf("[kernel] drainPendingToLedger: SaveJob failed traceID=%s err=%v\n", traceID, err)
			}
		default:
			return
		}
	}
}

// CancelJob 取消一个未完成的 job。
//
// 行为：
//   - job 已终态（done/failed/cancelled in ledger）：返回 false（没找到/不能取消）
//   - job 还在 ledger 但不在 jobCh（已被 worker 拿走）：标记 cancelled，依赖 svc 尊重 ctx.Done()
//   - job 还在 jobCh：从 ledger 标记 cancelled（worker 仍会处理但 OnDone 会收到 cancelled 错误）
//
// 当前实现是"best effort cancel"：标记 ledger + 日志，不真正从 jobCh 摘除（channel 不支持随机删除）。
// 真正的取消依赖 svc.Call 内部检查 ctx.Done() — 但 pool 的 ctx 是 pool 级别的，不是 job 级别的。
// 完整的 job 级 cancel 需要 ctx per job（未来扩展）。
//
// 返回 true 表示 ledger 中找到了该 job 并标记 cancelled。
func (p *Pool) CancelJob(traceID string) bool {
	if p.cfg.Ledger == nil {
		return false
	}
	if traceID == "" {
		return false
	}
	if err := p.cfg.Ledger.MarkStatus(traceID, JobStatusCancelled); err != nil {
		return false
	}
	return true
}

// LastRestoreInfo 上次 Restore 的信息（count + 时间），供 /api/kernel/pools 报告
func (p *Pool) LastRestoreInfo() (count int, at time.Time) {
	return p.lastRestoreCount, p.lastRestoreAt
}

// SetLastRestoreInfo 内部用：Restore 成功后记录
func (p *Pool) SetLastRestoreInfo(count int, at time.Time) {
	p.lastRestoreCount = count
	p.lastRestoreAt = at
}

// Submit 提交一个 job。返回 ErrPoolClosed 表示 pool 已关闭。
func (p *Pool) Submit(ctx ServiceContext, job Job) error {
	if ctx == nil {
		return errors.New("kernel: Submit with nil ctx")
	}
	if p.ctx == nil {
		return errors.New("kernel: pool not started")
	}
	if p.closed.Load() {
		return fmt.Errorf("kernel: pool %q closed", p.name)
	}
	select {
	case <-p.ctx.Done():
		return fmt.Errorf("kernel: pool %q closed", p.name)
	default:
	}
	// 启用 ledger 时持久化 job spec（断点续跑用）
	p.ledgerOnSubmit(ctx, job)
	atomic.AddUint64(&p.submitted, 1)
	envelope := jobEnvelope{ctx: ctx, job: job, attempts: 0}
	select {
	case p.jobCh <- envelope:
		return nil
	case <-p.ctx.Done():
		return fmt.Errorf("kernel: pool %q closed", p.name)
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Stats pool 统计
type PoolStats struct {
	Name             string    `json:"name"`
	Size             int       `json:"size"`
	QueueLen         int       `json:"queueLen"`
	QueueSize        int       `json:"queueSize"`
	Submitted        uint64    `json:"submitted"`
	Finished         uint64    `json:"finished"`
	Failed           uint64    `json:"failed"`
	Retried          uint64    `json:"retried"`
	LedgerEnabled    bool      `json:"ledgerEnabled"`
	LastRestoreCount int       `json:"lastRestoreCount"`
	LastRestoreAt    time.Time `json:"lastRestoreAt"`
}

// Stats 取统计
func (p *Pool) Stats() PoolStats {
	rc, at := p.LastRestoreInfo()
	return PoolStats{
		Name:             p.name,
		Size:             p.size,
		QueueLen:         len(p.jobCh),
		QueueSize:        cap(p.jobCh),
		Submitted:        atomic.LoadUint64(&p.submitted),
		Finished:         atomic.LoadUint64(&p.finished),
		Failed:           atomic.LoadUint64(&p.failed),
		Retried:          atomic.LoadUint64(&p.retried),
		LedgerEnabled:    p.cfg.Ledger != nil,
		LastRestoreCount: rc,
		LastRestoreAt:    at,
	}
}

// worker 主循环
func (p *Pool) worker(id int) {
	defer p.wg.Done()
	for env := range p.jobCh {
		p.execute(id, env)
	}
}

func (p *Pool) execute(workerID int, env jobEnvelope) {
	ctx := env.ctx
	job := env.job

	// 派生子 ctx：service 字段更新为 pool name（用于日志）
	// 关键：parent 必须是 p.ctx，这样 pool.Stop() 取消时 job 也跟着取消
	childCtx := &serviceCtx{
		parent:    p.ctx,
		service:   p.name,
		requestID: ctx.RequestID(),
		traceID:   ctx.TraceID(),
		created:   time.Now(),
		store:     checkpointStoreFrom(ctx),
	}

	// 单 job timeout
	if p.cfg.JobTimeout > 0 {
		var cancel context.CancelFunc
		var timeoutCtx context.Context
		timeoutCtx, cancel = context.WithTimeout(childCtx, p.cfg.JobTimeout)
		_ = cancel // defer via ctx deadline
		// childCtx 的 service/requestID/traceID 仍由 base 持有
		// 重新构造 serviceCtx，包装 timeoutCtx
		childCtx = &serviceCtx{
			parent:    timeoutCtx,
			service:   p.name,
			requestID: ctx.RequestID(),
			traceID:   ctx.TraceID(),
			created:   time.Now(),
			store:     checkpointStoreFrom(ctx),
		}
	}

	// ledger：标记进入 running
	p.ledgerOnStart(childCtx)

	jobStart := time.Now()
	raw, err := Call(childCtx, job.Service, job.Method, job.Payload)
	elapsed := time.Since(jobStart)

	if err == nil {
		atomic.AddUint64(&p.finished, 1)
		p.ledgerOnDone(childCtx)
		if job.OnDone != nil {
			go job.OnDone(JobResult{Job: job, Result: raw})
		}
		return
	}

	// 错误：决定是否重试
	atomic.AddUint64(&p.failed, 1)
	maxAttempts := job.MaxRetry + 1
	if env.attempts+1 < maxAttempts && !isCancelled(childCtx) {
		// 退避 backoff
		delay := p.cfg.RetryBackoff << env.attempts
		if delay > p.cfg.RetryMaxDelay {
			delay = p.cfg.RetryMaxDelay
		}
		select {
		case <-time.After(delay):
		case <-p.ctx.Done():
			p.ledgerOnCancel(childCtx)
			return
		case <-childCtx.Done():
			p.ledgerOnCancel(childCtx)
			return
		}
		atomic.AddUint64(&p.retried, 1)
		env.attempts++
		// 重投到队列（用 goroutine 避免阻塞 worker）
		// 检查 closed 防止写已关闭 channel（race: Stop 时 retry goroutine 还在 select）
		go func() {
			if p.closed.Load() {
				// pool 已关闭：把 job 委托给 Ledger
				if p.cfg.Ledger != nil {
					traceID := env.ctx.TraceID()
					if traceID != "" {
						stored := StoredJob{
							TraceID:  traceID,
							PoolName: p.name,
							Job:      env.job,
							Status:   JobStatusSubmitted,
							SavedAt:  time.Now(),
							Attempts: env.attempts,
						}
						_ = p.cfg.Ledger.SaveJob(traceID, stored)
					}
				}
				return
			}
			select {
			case p.jobCh <- env:
			case <-p.ctx.Done():
			}
		}()
		return
	}

	// 最终失败：区分 cancel vs 真失败
	if isCancelled(childCtx) {
		p.ledgerOnCancel(childCtx)
	} else {
		p.ledgerOnFinalFailure(childCtx)
	}
	if job.OnDone != nil {
		go job.OnDone(JobResult{Job: job, Err: fmt.Errorf("kernel: pool %s worker %d: %w (elapsed=%v)", p.name, workerID, err, elapsed)})
	}
}

func isCancelled(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return true
	default:
		return false
	}
}
