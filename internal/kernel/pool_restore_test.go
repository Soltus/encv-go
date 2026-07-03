package kernel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ─── JobLedger 基础测试 ────────────────────────────────────────────────

func TestJobStatus_Terminal(t *testing.T) {
	cases := []struct {
		status   JobStatus
		terminal bool
		pending  bool
	}{
		{JobStatusSubmitted, false, true},
		{JobStatusRunning, false, true},
		{JobStatusDone, true, false},
		{JobStatusFailed, true, false},
		{JobStatusCancelled, true, false},
	}
	for _, tc := range cases {
		if tc.status.IsTerminal() != tc.terminal {
			t.Errorf("%s: terminal=%v want %v", tc.status, tc.status.IsTerminal(), tc.terminal)
		}
		if tc.status.IsPending() != tc.pending {
			t.Errorf("%s: pending=%v want %v", tc.status, tc.status.IsPending(), tc.pending)
		}
	}
}

func TestFileJobLedger_SaveAndLoadPending(t *testing.T) {
	dir := t.TempDir()
	ledger, err := NewFileJobLedger(dir)
	if err != nil {
		t.Fatal(err)
	}

	job := StoredJob{
		TraceID:  "trace-1",
		PoolName: "test-pool",
		Job: Job{
			ID:      "j1",
			Service: "svc",
			Method:  "do",
			Payload: map[string]int{"x": 1},
		},
		Status: JobStatusSubmitted,
	}
	if err := ledger.SaveJob("trace-1", job); err != nil {
		t.Fatal(err)
	}

	pending, err := ledger.LoadPendingJobs("test-pool")
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 {
		t.Fatalf("expected 1 pending, got %d", len(pending))
	}
	if pending[0].Job.ID != "j1" {
		t.Errorf("Job.ID = %q, want j1", pending[0].Job.ID)
	}
	if pending[0].TraceID != "trace-1" {
		t.Errorf("TraceID = %q, want trace-1", pending[0].TraceID)
	}
	// Payload 反序列化后是 map[string]interface{}
	pm, ok := pending[0].Job.Payload.(map[string]any)
	if !ok {
		t.Fatalf("Payload type = %T, want map[string]any", pending[0].Job.Payload)
	}
	if pm["x"] != float64(1) {
		t.Errorf("Payload.x = %v, want 1", pm["x"])
	}
}

func TestFileJobLedger_MarkStatus_DoneExcludedFromPending(t *testing.T) {
	dir := t.TempDir()
	ledger, err := NewFileJobLedger(dir)
	if err != nil {
		t.Fatal(err)
	}

	job := StoredJob{
		TraceID:  "trace-done",
		PoolName: "p",
		Job:      Job{ID: "jd", Service: "s", Method: "m"},
		Status:   JobStatusSubmitted,
	}
	ledger.SaveJob("trace-done", job)

	if err := ledger.MarkStatus("trace-done", JobStatusDone); err != nil {
		t.Fatal(err)
	}

	pending, err := ledger.LoadPendingJobs("p")
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 0 {
		t.Errorf("expected 0 pending after done, got %d", len(pending))
	}
}

func TestFileJobLedger_MarkStatus_NotFound(t *testing.T) {
	dir := t.TempDir()
	ledger, err := NewFileJobLedger(dir)
	if err != nil {
		t.Fatal(err)
	}
	err = ledger.MarkStatus("nonexistent", JobStatusDone)
	if err == nil {
		t.Error("expected error for nonexistent traceID")
	}
}

func TestFileJobLedger_Delete(t *testing.T) {
	dir := t.TempDir()
	ledger, err := NewFileJobLedger(dir)
	if err != nil {
		t.Fatal(err)
	}

	job := StoredJob{
		TraceID: "trace-del", PoolName: "p",
		Job: Job{ID: "jd", Service: "s", Method: "m"},
	}
	ledger.SaveJob("trace-del", job)

	if err := ledger.Delete("trace-del"); err != nil {
		t.Fatal(err)
	}

	pending, _ := ledger.LoadPendingJobs("p")
	if len(pending) != 0 {
		t.Errorf("expected 0 pending after delete, got %d", len(pending))
	}
}

func TestFileJobLedger_MultiPool(t *testing.T) {
	dir := t.TempDir()
	ledger, err := NewFileJobLedger(dir)
	if err != nil {
		t.Fatal(err)
	}

	ledger.SaveJob("t1", StoredJob{TraceID: "t1", PoolName: "poolA", Job: Job{ID: "a"}, Status: JobStatusSubmitted})
	ledger.SaveJob("t2", StoredJob{TraceID: "t2", PoolName: "poolB", Job: Job{ID: "b"}, Status: JobStatusSubmitted})
	ledger.SaveJob("t3", StoredJob{TraceID: "t3", PoolName: "poolA", Job: Job{ID: "c"}, Status: JobStatusSubmitted})

	a, _ := ledger.LoadPendingJobs("poolA")
	b, _ := ledger.LoadPendingJobs("poolB")
	if len(a) != 2 {
		t.Errorf("poolA: expected 2, got %d", len(a))
	}
	if len(b) != 1 {
		t.Errorf("poolB: expected 1, got %d", len(b))
	}
}

func TestMemoryJobLedger_BasicOps(t *testing.T) {
	ledger := NewMemoryJobLedger()

	ledger.SaveJob("t1", StoredJob{TraceID: "t1", PoolName: "p", Job: Job{ID: "j1"}, Status: JobStatusSubmitted})
	ledger.SaveJob("t2", StoredJob{TraceID: "t2", PoolName: "p", Job: Job{ID: "j2"}, Status: JobStatusRunning})
	ledger.SaveJob("t3", StoredJob{TraceID: "t3", PoolName: "p", Job: Job{ID: "j3"}, Status: JobStatusDone})

	pending, _ := ledger.LoadPendingJobs("p")
	if len(pending) != 2 {
		t.Errorf("expected 2 pending, got %d", len(pending))
	}

	ledger.MarkStatus("t1", JobStatusDone)
	pending2, _ := ledger.LoadPendingJobs("p")
	if len(pending2) != 1 {
		t.Errorf("after mark t1 done: expected 1 pending, got %d", len(pending2))
	}

	ledger.Delete("t2")
	pending3, _ := ledger.LoadPendingJobs("p")
	if len(pending3) != 0 {
		t.Errorf("after delete t2: expected 0 pending, got %d", len(pending3))
	}
}

// ─── Pool + ledger 集成测试 ─────────────────────────────────────────────

func TestPool_SubmitWritesLedger(t *testing.T) {
	Unregister("ledger.svc")
	defer Unregister("ledger.svc")
	Register(&mockService{name: "ledger.svc", calls: new(uint64)})

	ledger := NewMemoryJobLedger()
	pool := NewPool("test-ledger-submit", 1, PoolConfig{
		QueueSize:    10,
		RetryBackoff: 10 * time.Millisecond,
		Ledger:       ledger,
	})
	pool.Start(context.Background())
	defer pool.Stop()

	ctx := NewContext(context.Background(), WithTraceID("trace-submit-1"))
	done := make(chan struct{})
	pool.Submit(ctx, Job{
		ID:      "j1",
		Service: "ledger.svc",
		Method:  "m",
		OnDone: func(r JobResult) {
			if r.Err != nil {
				t.Errorf("job failed: %v", r.Err)
			}
			close(done)
		},
	})

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout")
	}

	// 等待 ledger 写入完成（异步 OnDone 可能在 ledgerOnDone 之前/之后）
	// 给一点时间让所有 ledger 操作 settle
	time.Sleep(50 * time.Millisecond)

	snap := ledger.Snapshot()
	stored, ok := snap["trace-submit-1"]
	if !ok {
		t.Fatal("expected job in ledger")
	}
	if stored.Status != JobStatusDone {
		t.Errorf("Status = %q, want done", stored.Status)
	}
	if stored.Job.ID != "j1" {
		t.Errorf("Job.ID = %q, want j1", stored.Job.ID)
	}
	if stored.PoolName != "test-ledger-submit" {
		t.Errorf("PoolName = %q, want test-ledger-submit", stored.PoolName)
	}
}

func TestPool_FinalFailureMarksFailed(t *testing.T) {
	Unregister("ledger.fail")
	defer Unregister("ledger.fail")
	Register(&alwaysFailService{name: "ledger.fail"})

	ledger := NewMemoryJobLedger()
	pool := NewPool("test-ledger-fail", 1, PoolConfig{
		QueueSize:    10,
		RetryBackoff: 5 * time.Millisecond,
		Ledger:       ledger,
	})
	pool.Start(context.Background())
	defer pool.Stop()

	ctx := NewContext(context.Background(), WithTraceID("trace-fail"))
	done := make(chan struct{})
	pool.Submit(ctx, Job{
		ID:       "jf",
		Service:  "ledger.fail",
		Method:   "m",
		MaxRetry: 1, // 2 attempts total (1 original + 1 retry)
		OnDone: func(r JobResult) {
			close(done)
		},
	})

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout")
	}
	time.Sleep(50 * time.Millisecond) // 等 ledger settle

	stored, ok := ledger.Snapshot()["trace-fail"]
	if !ok {
		t.Fatal("expected job in ledger")
	}
	if stored.Status != JobStatusFailed {
		t.Errorf("Status = %q, want failed", stored.Status)
	}
}

type alwaysFailService struct {
	name string
}

func (s *alwaysFailService) Name() string                    { return s.name }
func (s *alwaysFailService) Init(ctx ServiceContext) error   { return nil }
func (s *alwaysFailService) Health(ctx ServiceContext) error { return nil }
func (s *alwaysFailService) Call(ctx ServiceContext, method string, payload json.RawMessage) (json.RawMessage, error) {
	return nil, errors.New("always fails")
}

func TestPool_CancelMarksCancelled(t *testing.T) {
	Unregister("ledger.cancel")
	defer Unregister("ledger.cancel")
	Register(&slowService{name: "ledger.cancel", delay: 30 * time.Second})

	ledger := NewMemoryJobLedger()
	pool := NewPool("test-ledger-cancel", 1, PoolConfig{
		QueueSize:    10,
		Ledger:       ledger,
	})
	pool.Start(context.Background())

	ctx := NewContext(context.Background(), WithTraceID("trace-cancel"))
	done := make(chan struct{})
	pool.Submit(ctx, Job{
		ID:      "jc",
		Service: "ledger.cancel",
		Method:  "m",
		OnDone: func(r JobResult) {
			close(done)
		},
	})

	time.Sleep(100 * time.Millisecond) // 让 worker 拿到 job
	pool.Stop()                        // cancel in-flight

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout")
	}
	time.Sleep(50 * time.Millisecond)

	stored, ok := ledger.Snapshot()["trace-cancel"]
	if !ok {
		t.Fatal("expected job in ledger")
	}
	if stored.Status != JobStatusCancelled {
		t.Errorf("Status = %q, want cancelled", stored.Status)
	}
}

// ─── Pool.Restore 测试 ──────────────────────────────────────────────────

func TestPool_Restore_NoLedger_Error(t *testing.T) {
	pool := NewPool("no-ledger", 1, DefaultPoolConfig())
	pool.Start(context.Background())
	defer pool.Stop()

	_, err := pool.Restore(NewContext(context.Background()))
	if err == nil {
		t.Error("expected error when no ledger configured")
	}
}

func TestPool_Restore_ResubmitsPending(t *testing.T) {
	Unregister("restore.svc")
	defer Unregister("restore.svc")
	calls := new(uint64)
	Register(&mockService{name: "restore.svc", calls: calls})

	ledger := NewMemoryJobLedger()

	// 模拟"上次崩溃"留下的未完成 job
	storedTraceID := "trace-restore-1"
	stored := StoredJob{
		TraceID:  storedTraceID,
		PoolName: "test-restore",
		Job: Job{
			ID:      "j-restore",
			Service: "restore.svc",
			Method:  "process",
			Payload: map[string]int{"x": 42},
		},
		Status:   JobStatusRunning, // 崩溃时正在跑
		Attempts: 0,
	}
	if err := ledger.SaveJob(storedTraceID, stored); err != nil {
		t.Fatal(err)
	}

	// 构造新 pool（模拟重启），用同一个 ledger
	pool := NewPool("test-restore", 1, PoolConfig{
		QueueSize:    10,
		RetryBackoff: 10 * time.Millisecond,
		Ledger:       ledger,
	})
	pool.Start(context.Background())
	defer pool.Stop()

	// baseCtx 必须带 CheckpointStore（resumed job 继承）
	baseCtx := NewContext(context.Background(),
		WithCheckpointStore(NewMemoryCheckpointStore()))

	restored, err := pool.Restore(baseCtx)
	if err != nil {
		t.Fatalf("Restore failed: %v", err)
	}
	if restored != 1 {
		t.Errorf("restored = %d, want 1", restored)
	}

	// 等 job 执行完
	time.Sleep(200 * time.Millisecond)

	if atomic.LoadUint64(calls) != 1 {
		t.Errorf("calls = %d, want 1", atomic.LoadUint64(calls))
	}

	// ledger 应标记为 done
	stored2, ok := ledger.Snapshot()[storedTraceID]
	if !ok {
		t.Fatal("expected job in ledger after restore")
	}
	if stored2.Status != JobStatusDone {
		t.Errorf("Status = %q, want done", stored2.Status)
	}
}

func TestPool_Restore_SkipsTerminal(t *testing.T) {
	Unregister("restore.terminal")
	defer Unregister("restore.terminal")
	calls := new(uint64)
	Register(&mockService{name: "restore.terminal", calls: calls})

	ledger := NewMemoryJobLedger()

	// 写 3 个 job：1 done、1 failed、1 running
	ledger.SaveJob("t-done", StoredJob{
		TraceID: "t-done", PoolName: "p",
		Job: Job{ID: "jd", Service: "restore.terminal", Method: "m"},
		Status: JobStatusDone,
	})
	ledger.SaveJob("t-failed", StoredJob{
		TraceID: "t-failed", PoolName: "p",
		Job: Job{ID: "jf", Service: "restore.terminal", Method: "m"},
		Status: JobStatusFailed,
	})
	ledger.SaveJob("t-running", StoredJob{
		TraceID: "t-running", PoolName: "p",
		Job: Job{ID: "jr", Service: "restore.terminal", Method: "m"},
		Status: JobStatusRunning,
	})

	pool := NewPool("p", 1, PoolConfig{
		QueueSize:    10,
		Ledger:       ledger,
	})
	pool.Start(context.Background())
	defer pool.Stop()

	baseCtx := NewContext(context.Background())
	restored, err := pool.Restore(baseCtx)
	if err != nil {
		t.Fatalf("Restore failed: %v", err)
	}
	if restored != 1 {
		t.Errorf("restored = %d, want 1 (only running should be restored)", restored)
	}

	time.Sleep(200 * time.Millisecond)
	if atomic.LoadUint64(calls) != 1 {
		t.Errorf("calls = %d, want 1 (only running job should execute)", atomic.LoadUint64(calls))
	}
}

// ─── 端到端：checkpoint + restore + resume 续跑 ──────────────────────────
//
// 场景：长任务处理 5 个文件，处理到第 3 个时"崩溃"。
// 重启后 Restore，svc 内 Restore("progress") 拿到 {processed: 3}，跳过前 3 个，
// 继续处理第 4、5 个。
func TestPool_Restore_E2E_ResumeFromCheckpoint(t *testing.T) {
	Unregister("e2e.resume")
	defer Unregister("e2e.resume")

	svc := &resumableService{
		name:        "e2e.resume",
		crashAt:     3, // 处理到第 3 个时"崩溃"（返回特殊 error）
		crashMarker: "E2E_CRASH",
	}
	Register(svc)

	// 用 file-based checkpoint store + file-based ledger（模拟真实场景）
	ckptDir := t.TempDir()
	ledgerDir := t.TempDir()

	store, err := NewFileCheckpointStore(ckptDir)
	if err != nil {
		t.Fatal(err)
	}
	ledger, err := NewFileJobLedger(ledgerDir)
	if err != nil {
		t.Fatal(err)
	}

	traceID := "trace-e2e-resume"

	// === Phase 1：首次提交，处理到第 3 个时"崩溃" ===
	pool1 := NewPool("e2e-pool", 1, PoolConfig{
		QueueSize:    10,
		Ledger:       ledger,
	})
	pool1.Start(context.Background())

	ctx1 := NewContext(context.Background(),
		WithTraceID(traceID),
		WithCheckpointStore(store),
		WithServiceName("e2e.client"),
	)

	phase1Done := make(chan struct{})
	var phase1Err error
	pool1.Submit(ctx1, Job{
		ID:      "e2e-job",
		Service: "e2e.resume",
		Method:  "process",
		Payload: map[string]any{"files": 5},
		OnDone: func(r JobResult) {
			phase1Err = r.Err
			close(phase1Done)
		},
	})

	select {
	case <-phase1Done:
	case <-time.After(2 * time.Second):
		t.Fatal("phase 1 timeout")
	}

	// 验证 phase 1 因 crash 结束（有 error，但不是 cancelled）
	if phase1Err == nil {
		t.Fatal("expected phase 1 to end with crash error")
	}
	t.Logf("phase 1 ended (expected crash): %v", phase1Err)

	// 模拟"进程崩溃"：直接 Stop pool（不优雅退出 in-flight job）
	// 注意：phase1 的 OnDone 已经触发说明 job 已结束（crash 返回 error）
	pool1.Stop()

	// 验证 checkpoint 已写入 "progress" = {processed: 3}
	checkpointData, err := store.Get(traceID, "progress")
	if err != nil {
		t.Fatalf("checkpoint not written: %v", err)
	}
	t.Logf("checkpoint data: %s", string(checkpointData))
	var prog struct {
		Processed int `json:"processed"`
		Total     int `json:"total"`
	}
	if err := json.Unmarshal(checkpointData, &prog); err != nil {
		t.Fatal(err)
	}
	if prog.Processed != 3 {
		t.Errorf("checkpoint Processed = %d, want 3", prog.Processed)
	}

	// 验证 ledger 标记为 failed（crash = 最终失败）
	pending, err := ledger.LoadPendingJobs("e2e-pool")
	if err != nil {
		t.Fatal(err)
	}
	// failed 是终态，不在 pending 里
	if len(pending) != 0 {
		t.Errorf("expected 0 pending (failed is terminal), got %d", len(pending))
	}

	// === 模拟手动改回 "running" 让 Restore 能拾起 ===
	// （真实场景：crash 时进程死了，ledger 停在 "running"；
	//  这里因为我们的 OnDone 触发了 ledgerOnFinalFailure，所以手动改回）
	// 这是为了模拟"进程 crash 在 svc.Call 中间，没机会写 failed"的真实场景
	if err := ledger.MarkStatus(traceID, JobStatusRunning); err != nil {
		t.Fatalf("MarkStatus running failed: %v", err)
	}

	// === Phase 2：重启 pool，Restore 续跑 ===
	svc.crashAt = -1 // 不再 crash，正常完成
	svc.ResetProcessed()

	pool2 := NewPool("e2e-pool", 1, PoolConfig{
		QueueSize:    10,
		Ledger:       ledger,
	})
	pool2.Start(context.Background())
	defer pool2.Stop()

	ctx2 := NewContext(context.Background(),
		WithCheckpointStore(store), // 同一个 store，resumed job 继承
	)

	restored, err := pool2.Restore(ctx2)
	if err != nil {
		t.Fatalf("Restore failed: %v", err)
	}
	if restored != 1 {
		t.Errorf("restored = %d, want 1", restored)
	}

	// 等 resumed job 完成
	time.Sleep(500 * time.Millisecond)

	// 验证 svc 从 checkpoint 恢复，处理了剩余 2 个文件（4、5）
	if svc.Processed() != 2 {
		t.Errorf("resumed processed = %d, want 2 (files 4,5)", svc.Processed())
	}
	t.Logf("resumed svc processed %d files (skipped first 3 via checkpoint)", svc.Processed())

	// 验证 ledger 最终标记为 done
	stored, err := readLedgerJob(ledgerDir, "e2e-pool", traceID)
	if err != nil {
		t.Fatalf("read ledger: %v", err)
	}
	if stored.Status != JobStatusDone {
		t.Errorf("final Status = %q, want done", stored.Status)
	}
}

// readLedgerJob 辅助：从 FileJobLedger 目录直接读 job 文件
func readLedgerJob(root, poolName, traceID string) (StoredJob, error) {
	p := filepath.Join(root, sanitizeID(poolName), sanitizeID(traceID)+".json")
	data, err := os.ReadFile(p)
	if err != nil {
		return StoredJob{}, err
	}
	var sj StoredJob
	if err := json.Unmarshal(data, &sj); err != nil {
		return StoredJob{}, err
	}
	return sj, nil
}

// resumableService 可恢复的服务：处理 N 个文件，每个 Checkpoint 一次。
// crashAt > 0 时，处理到第 crashAt 个时返回特殊 error（模拟 crash）
type resumableService struct {
	name        string
	crashAt     int  // -1 = 不 crash
	crashMarker string
	mu          sync.Mutex
	processed   int
}

func (s *resumableService) Name() string                    { return s.name }
func (s *resumableService) Init(ctx ServiceContext) error   { return nil }
func (s *resumableService) Health(ctx ServiceContext) error { return nil }

func (s *resumableService) Processed() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.processed
}

func (s *resumableService) ResetProcessed() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.processed = 0
}

func (s *resumableService) Call(ctx ServiceContext, method string, payload json.RawMessage) (json.RawMessage, error) {
	// 解析 payload
	var req struct {
		Files int `json:"files"`
	}
	if len(payload) > 0 {
		if err := json.Unmarshal(payload, &req); err != nil {
			return nil, err
		}
	}
	if req.Files == 0 {
		req.Files = 5
	}

	// 尝试 Restore 上次的 checkpoint
	var prog struct {
		Processed int `json:"processed"`
		Total     int `json:"total"`
	}
	if err := ctx.Restore("progress", &prog); err == nil {
		// 恢复成功：跳过已处理的部分
		s.mu.Lock()
		s.processed = 0 // 重置计数，只统计本次处理的
		s.mu.Unlock()
		fmt.Printf("[resumableService] restored from checkpoint: processed=%d total=%d\n", prog.Processed, prog.Total)
	} else {
		// 首次运行
		prog.Processed = 0
		prog.Total = req.Files
	}

	// 继续处理
	for i := prog.Processed + 1; i <= prog.Total; i++ {
		// 检查 cancel
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		// crash 模拟
		if s.crashAt > 0 && i == s.crashAt+1 {
			// 在处理第 (crashAt+1) 个之前 crash
			// 但要先 checkpoint 已完成的 crashAt 个
			prog.Processed = i - 1
			if err := ctx.Checkpoint("progress", prog); err != nil {
				return nil, fmt.Errorf("checkpoint at %d: %w", prog.Processed, err)
			}
			return nil, errors.New(s.crashMarker)
		}

		// 模拟处理一个文件
		time.Sleep(10 * time.Millisecond)
		s.mu.Lock()
		s.processed++
		s.mu.Unlock()

		// checkpoint 进度
		prog.Processed = i
		if err := ctx.Checkpoint("progress", prog); err != nil {
			// checkpoint 失败不致命（继续跑，最坏情况是重做）
			fmt.Printf("[resumableService] checkpoint failed: %v\n", err)
		}
	}

	return json.Marshal(map[string]any{
		"processed": prog.Processed,
		"total":     prog.Total,
	})
}
