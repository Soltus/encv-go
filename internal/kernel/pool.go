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
	OnDone   func(JobResult) // 回调（异步执行；nil = 不通知）
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

	jobCh chan jobEnvelope
	wg    sync.WaitGroup

	ctx    context.Context
	cancel context.CancelFunc

	// metrics
	submitted uint64
	finished  uint64
	failed    uint64
	retried   uint64
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
	if p.cancel != nil {
		p.cancel()
	}
	close(p.jobCh)
	p.wg.Wait()
}

// Submit 提交一个 job。返回 ErrPoolClosed 表示 pool 已关闭。
func (p *Pool) Submit(ctx ServiceContext, job Job) error {
	if ctx == nil {
		return errors.New("kernel: Submit with nil ctx")
	}
	if p.ctx == nil {
		return errors.New("kernel: pool not started")
	}
	select {
	case <-p.ctx.Done():
		return fmt.Errorf("kernel: pool %q closed", p.name)
	default:
	}
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
	Name      string `json:"name"`
	Size      int    `json:"size"`
	QueueLen  int    `json:"queueLen"`
	Submitted uint64 `json:"submitted"`
	Finished  uint64 `json:"finished"`
	Failed    uint64 `json:"failed"`
	Retried   uint64 `json:"retried"`
}

// Stats 取统计
func (p *Pool) Stats() PoolStats {
	return PoolStats{
		Name:      p.name,
		Size:      p.size,
		QueueLen:  len(p.jobCh),
		Submitted: atomic.LoadUint64(&p.submitted),
		Finished:  atomic.LoadUint64(&p.finished),
		Failed:    atomic.LoadUint64(&p.failed),
		Retried:   atomic.LoadUint64(&p.retried),
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

	jobStart := time.Now()
	raw, err := Call(childCtx, job.Service, job.Method, job.Payload)
	elapsed := time.Since(jobStart)

	if err == nil {
		atomic.AddUint64(&p.finished, 1)
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
			return
		case <-childCtx.Done():
			return
		}
		atomic.AddUint64(&p.retried, 1)
		env.attempts++
		// 重投到队列头（用一个 goroutine 避免阻塞 worker）
		go func() {
			select {
			case p.jobCh <- env:
			case <-p.ctx.Done():
			}
		}()
		return
	}

	// 最终失败
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
