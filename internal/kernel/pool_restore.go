package kernel

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// ─── Pool 断点续跑（WorkManager 风格） ──────────────────────────────────
//
// 设计目标：
//
//	长任务（如 FTS 重建、批量加密、向量索引构建）在以下场景中断后，
//	pool 重启时自动恢复未完成的 job 并续跑：
//	  1. WorkManager 杀进程（Android 后台回收）
//	  2. 设备重启
//	  3. app 切后台被系统回收
//	  4. 进程 crash（panic / OOM）
//
// 工作流：
//
//	Submit 阶段：
//	  pool.Submit(ctx, job)
//	    → ledger.SaveJob(traceID, poolName, job, "submitted")
//	    → 投入 jobCh
//
//	execute 阶段：
//	  worker 拿到 job
//	    → ledger.MarkStatus(traceID, "running")
//	    → svc.Call(childCtx, ...)  ← svc 内部可 ctx.Checkpoint("progress", state)
//	    → 成功: ledger.MarkStatus(traceID, "done")
//	    → 失败（最终）: ledger.MarkStatus(traceID, "failed")
//	    → 取消: ledger.MarkStatus(traceID, "cancelled")
//
//	Restore 阶段（pool 重启时）：
//	  pool.Restore(baseCtx)
//	    → ledger.LoadPendingJobs(poolName)  // status ∈ {submitted, running}
//	    → 对每个 pending job：
//	        - 用 stored TraceID 构造新 ctx（保留 CheckpointStore）
//	        - pool.Submit(restoredCtx, job)  // 重投
//	    → svc 内部 ctx.Restore("progress", &state) 续跑
//
// 关键：
//
//  1. TraceID 必须复用： resumed job 的 ctx 用 stored TraceID，
//     这样 svc 的 ctx.Restore("progress") 才能拿到上次 checkpoint 的状态。
//
//  2. Payload 必须可序列化： Job.Payload 是 any，但写入 ledger 时会 json.Marshal。
//     不可序列化的 Payload（如 chan / func）会在 SaveJob 时报错。
//
//  3. 幂等： Restore 多次调用不会重复执行已完成的 job（ledger.MarkDone 已标记）。
//
//  4. 不替代 ctx.Checkpoint： ledger 只记录"job 是否完成"，job 内部进度
//     （如"已处理 3000/10000 文件"）由 svc 自己 ctx.Checkpoint 保存。

// JobStatus job 在 ledger 中的状态
type JobStatus string

const (
	JobStatusSubmitted JobStatus = "submitted" // 已提交，未开始
	JobStatusRunning   JobStatus = "running"   // 执行中
	JobStatusDone      JobStatus = "done"      // 成功完成
	JobStatusFailed    JobStatus = "failed"    // 最终失败（重试耗尽）
	JobStatusCancelled JobStatus = "cancelled" // 被 cancel
)

// IsTerminal 是否终态（不可恢复）
func (s JobStatus) IsTerminal() bool {
	return s == JobStatusDone || s == JobStatusFailed || s == JobStatusCancelled
}

// IsPending 是否可恢复（未到终态）
func (s JobStatus) IsPending() bool {
	return s == JobStatusSubmitted || s == JobStatusRunning
}

// StoredJob ledger 中存储的 job 记录
type StoredJob struct {
	TraceID  string    `json:"traceID"`  // 原始 ctx 的 TraceID（resume 时复用）
	PoolName string    `json:"poolName"` // 所属 pool
	Job      Job       `json:"job"`      // job spec（ID/Service/Method/Payload/MaxRetry）
	Status   JobStatus `json:"status"`   // 当前状态
	SavedAt  time.Time `json:"savedAt"`  // 最后更新时间
	Attempts int       `json:"attempts"` // 已重试次数（resume 时延续）
}

// JobLedger job 持久化账本（WorkManager 风格断点续跑）
//
// 实现方：
//   - FileJobLedger：磁盘版（生产用，跨进程恢复）
//   - MemoryJobLedger：内存版（测试用，进程内恢复）
type JobLedger interface {
	// SaveJob 写入/更新一个 job 记录（按 traceID 索引）
	SaveJob(traceID string, job StoredJob) error

	// MarkStatus 仅更新状态（比 SaveJob 轻）
	MarkStatus(traceID string, status JobStatus) error

	// LoadPendingJobs 加载某 pool 的所有未完成 job（status.IsPending）
	LoadPendingJobs(poolName string) ([]StoredJob, error)

	// Delete 删除一个 job 记录（终态后清理）
	Delete(traceID string) error
}

// ─── FileJobLedger：磁盘版 ───────────────────────────────────────────────
//
// 目录结构：
//
//	<root>/
//	  <poolName>/
//	    <traceID>.json   ← StoredJob 序列化
//
// 文件名用 sanitizeID(traceID)，避免路径分隔符注入。
// 写入用 tmp + rename 原子替换，避免半写文件被读到。
type FileJobLedger struct {
	root string
	mu   sync.Mutex
}

// NewFileJobLedger 构造磁盘版 ledger。root 不存在会自动创建。
func NewFileJobLedger(root string) (*FileJobLedger, error) {
	if err := os.MkdirAll(root, 0755); err != nil {
		return nil, err
	}
	return &FileJobLedger{root: root}, nil
}

func (l *FileJobLedger) poolDir(poolName string) string {
	return filepath.Join(l.root, sanitizeID(poolName))
}

func (l *FileJobLedger) jobFile(poolName, traceID string) string {
	return filepath.Join(l.poolDir(poolName), sanitizeID(traceID)+".json")
}

// SaveJob 写入 job 记录
func (l *FileJobLedger) SaveJob(traceID string, job StoredJob) error {
	if traceID == "" {
		return errors.New("kernel: SaveJob with empty traceID")
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	dir := l.poolDir(job.PoolName)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	finalPath := l.jobFile(job.PoolName, traceID)
	tmpPath := finalPath + ".tmp"

	data, err := json.MarshalIndent(job, "", "  ")
	if err != nil {
		return fmt.Errorf("kernel: marshal job: %w", err)
	}
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmpPath, finalPath)
}

// MarkStatus 更新状态
func (l *FileJobLedger) MarkStatus(traceID string, status JobStatus) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	// 不知道 poolName，遍历所有子目录找 traceID 文件
	entries, err := os.ReadDir(l.root)
	if err != nil {
		return err
	}
	target := sanitizeID(traceID) + ".json"
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		p := filepath.Join(l.root, e.Name(), target)
		data, err := os.ReadFile(p)
		if err != nil {
			continue // 这个 poolDir 没有，继续找
		}
		var job StoredJob
		if err := json.Unmarshal(data, &job); err != nil {
			return fmt.Errorf("kernel: unmarshal job %s: %w", p, err)
		}
		job.Status = status
		job.SavedAt = time.Now()
		out, err := json.MarshalIndent(job, "", "  ")
		if err != nil {
			return err
		}
		tmp := p + ".tmp"
		if err := os.WriteFile(tmp, out, 0644); err != nil {
			return err
		}
		return os.Rename(tmp, p)
	}
	return fmt.Errorf("kernel: job %q not found in ledger", traceID)
}

// LoadPendingJobs 加载某 pool 的未完成 job
func (l *FileJobLedger) LoadPendingJobs(poolName string) ([]StoredJob, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	dir := l.poolDir(poolName)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []StoredJob
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		var job StoredJob
		if err := json.Unmarshal(data, &job); err != nil {
			return nil, fmt.Errorf("kernel: unmarshal %s: %w", e.Name(), err)
		}
		if job.Status.IsPending() {
			out = append(out, job)
		}
	}
	return out, nil
}

// Delete 删除 job 记录
func (l *FileJobLedger) Delete(traceID string) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	entries, err := os.ReadDir(l.root)
	if err != nil {
		return err
	}
	target := sanitizeID(traceID) + ".json"
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		p := filepath.Join(l.root, e.Name(), target)
		if _, err := os.Stat(p); err == nil {
			return os.Remove(p)
		}
	}
	return nil // not found = noop
}

// ─── MemoryJobLedger：内存版（测试用） ────────────────────────────────────
type MemoryJobLedger struct {
	mu   sync.Mutex
	jobs map[string]StoredJob // key: traceID
}

// NewMemoryJobLedger 内存版 ledger
func NewMemoryJobLedger() *MemoryJobLedger {
	return &MemoryJobLedger{jobs: map[string]StoredJob{}}
}

func (m *MemoryJobLedger) SaveJob(traceID string, job StoredJob) error {
	if traceID == "" {
		return errors.New("kernel: SaveJob with empty traceID")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	job.SavedAt = time.Now()
	m.jobs[traceID] = job
	return nil
}

func (m *MemoryJobLedger) MarkStatus(traceID string, status JobStatus) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	job, ok := m.jobs[traceID]
	if !ok {
		return fmt.Errorf("kernel: job %q not found", traceID)
	}
	job.Status = status
	job.SavedAt = time.Now()
	m.jobs[traceID] = job
	return nil
}

func (m *MemoryJobLedger) LoadPendingJobs(poolName string) ([]StoredJob, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []StoredJob
	for _, job := range m.jobs {
		if job.PoolName == poolName && job.Status.IsPending() {
			out = append(out, job)
		}
	}
	return out, nil
}

func (m *MemoryJobLedger) Delete(traceID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.jobs, traceID)
	return nil
}

// Snapshot 返回所有记录（测试 debug 用）
func (m *MemoryJobLedger) Snapshot() map[string]StoredJob {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make(map[string]StoredJob, len(m.jobs))
	for k, v := range m.jobs {
		out[k] = v
	}
	return out
}

// ─── Pool 集成 ledger ────────────────────────────────────────────────────

// Restore 从 ledger 加载未完成的 job 并重投。
//
// 用法（pool 重启时）：
//
//	pool := NewPool("encrypt", 4, PoolConfig{Ledger: fileLedger})
//	pool.Start(ctx)
//	count, err := pool.Restore(ctx)
//	if err != nil {
//	    slog.Warn("pool restore failed", "err", err, "restored", count)
//	}
//
// 关键：
//   - baseCtx 必须带 CheckpointStore（resumed job 的 ctx 会继承）
//   - resumed job 的 TraceID 复用 stored TraceID（保证 svc 内 Restore 能拿到上次 checkpoint）
//   - 已终态（done/failed/cancelled）的 job 不恢复
//   - restored job 的 attempts 延续（避免重试次数重置导致死循环）
//
// 返回成功恢复的 job 数。
func (p *Pool) Restore(baseCtx ServiceContext) (int, error) {
	if p.cfg.Ledger == nil {
		return 0, errors.New("kernel: pool has no ledger configured")
	}
	if baseCtx == nil {
		return 0, errors.New("kernel: Restore with nil ctx")
	}

	pending, err := p.cfg.Ledger.LoadPendingJobs(p.name)
	if err != nil {
		return 0, fmt.Errorf("kernel: load pending jobs for %q: %w", p.name, err)
	}

	store := checkpointStoreFrom(baseCtx)
	restored := 0
	for _, sj := range pending {
		// 构造 resumed ctx：复用 stored TraceID，继承 CheckpointStore
		// 这样 svc 内 ctx.Restore("progress") 能拿到上次 checkpoint
		resumedCtx := &serviceCtx{
			parent:    baseCtx,
			service:   p.name,
			requestID: nextID(),    // 新 RequestID（每次恢复都是新 request）
			traceID:   sj.TraceID, // TraceID 复用！
			created:   time.Now(),
			store:     store,
		}

		// 构造 job：保留原 ID/Service/Method/Payload/MaxRetry，attempts 延续
		job := sj.Job
		envelope := jobEnvelope{
			ctx:      resumedCtx,
			job:      job,
			attempts: sj.Attempts,
		}

		// 标记回 "running"（被 restore 触发）
		if err := p.cfg.Ledger.MarkStatus(sj.TraceID, JobStatusRunning); err != nil {
			return restored, fmt.Errorf("kernel: mark %s running: %w", sj.TraceID, err)
		}

		// 投递到 jobCh（非阻塞，失败说明 pool 已满或关闭）
		select {
		case p.jobCh <- envelope:
			restored++
		case <-p.ctx.Done():
			return restored, fmt.Errorf("kernel: pool closed during restore")
		default:
			return restored, fmt.Errorf("kernel: pool queue full, %d jobs not restored", len(pending)-restored)
		}
	}
	p.SetLastRestoreInfo(restored, time.Now())
	return restored, nil
}

// ─── Pool 内部：ledger 钩子（execute 调用） ──────────────────────────────
//
// 这些钩子在 pool.go 的 execute() 中调用，但为了不修改 pool.go 主体逻辑，
// 提供为独立函数。execute 通过 p.cfg.Ledger != nil 判断是否启用。
//
// 注意：ledger 写入失败只 log，不影响 job 执行（ledger 是 best-effort 持久化）。

// ledgerOnSubmit Submit 时调用（如果配置了 ledger）
func (p *Pool) ledgerOnSubmit(ctx ServiceContext, job Job) {
	if p.cfg.Ledger == nil {
		return
	}
	traceID := ctx.TraceID()
	if traceID == "" {
		return
	}
	stored := StoredJob{
		TraceID:  traceID,
		PoolName: p.name,
		Job:      job,
		Status:   JobStatusSubmitted,
		SavedAt:  time.Now(),
	}
	if err := p.cfg.Ledger.SaveJob(traceID, stored); err != nil {
		fmt.Printf("[kernel] ledger.SaveJob failed: traceID=%s err=%v\n", traceID, err)
	}
}

// ledgerOnStart execute 拿到 job 时调用
func (p *Pool) ledgerOnStart(ctx ServiceContext) {
	if p.cfg.Ledger == nil {
		return
	}
	if err := p.cfg.Ledger.MarkStatus(ctx.TraceID(), JobStatusRunning); err != nil {
		fmt.Printf("[kernel] ledger.MarkStatus(running) failed: traceID=%s err=%v\n", ctx.TraceID(), err)
	}
}

// ledgerOnDone job 成功完成时调用
func (p *Pool) ledgerOnDone(ctx ServiceContext) {
	if p.cfg.Ledger == nil {
		return
	}
	if err := p.cfg.Ledger.MarkStatus(ctx.TraceID(), JobStatusDone); err != nil {
		fmt.Printf("[kernel] ledger.MarkStatus(done) failed: traceID=%s err=%v\n", ctx.TraceID(), err)
	}
}

// ledgerOnFinalFailure job 最终失败（重试耗尽）时调用
func (p *Pool) ledgerOnFinalFailure(ctx ServiceContext) {
	if p.cfg.Ledger == nil {
		return
	}
	if err := p.cfg.Ledger.MarkStatus(ctx.TraceID(), JobStatusFailed); err != nil {
		fmt.Printf("[kernel] ledger.MarkStatus(failed) failed: traceID=%s err=%v\n", ctx.TraceID(), err)
	}
}

// ledgerOnCancel job 被 cancel 时调用
func (p *Pool) ledgerOnCancel(ctx ServiceContext) {
	if p.cfg.Ledger == nil {
		return
	}
	if err := p.cfg.Ledger.MarkStatus(ctx.TraceID(), JobStatusCancelled); err != nil {
		fmt.Printf("[kernel] ledger.MarkStatus(cancelled) failed: traceID=%s err=%v\n", ctx.TraceID(), err)
	}
}

// ledgerOnDelegate pool 关闭时把 in-flight job 委托给 Ledger（status=submitted，可 Restore）。
//
// 与 ledgerOnCancel 的区别：
//   - ledgerOnCancel: 用户主动 cancel → status=cancelled（terminal，不 Restore）
//   - ledgerOnDelegate: pool 关闭 → status=submitted（pending，下次 Start 时 Restore 续跑）
//
// 保存完整 job spec（含 attempts），让 Restore 后的续跑能延续重试计数。
func (p *Pool) ledgerOnDelegate(ctx ServiceContext, env jobEnvelope) {
	if p.cfg.Ledger == nil {
		return
	}
	traceID := ctx.TraceID()
	if traceID == "" {
		return
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
		fmt.Printf("[kernel] ledgerOnDelegate: SaveJob failed: traceID=%s err=%v\n", traceID, err)
	}
}
