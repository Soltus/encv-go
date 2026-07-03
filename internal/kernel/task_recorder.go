package kernel

import (
	"encoding/json"
	"fmt"
	"time"
)

// ─── 任务中间件（Task Middleware） ───────────────────────────────────────
//
// 所有通过 MicroKernel.Call 的服务调用都会被 TaskRecorder 记录为 TaskRecord。
// 这是一个可选的中间件层，通过 MicroKernelConfig.TaskStore 启用。
//
// 记录内容：
//   - 基础标识（TraceID / RequestID / TaskType）
//   - 生命周期（createdAt / startedAt / completedAt / duration）
//   - 状态流转（pending → running → success/failed/cancelled）
//   - 输入输出（可选，可配置是否记录 payload）
//   - 错误信息（失败时）
//   - 租户信息（多租户场景）
//   - 触发来源（user / automation / ai_agent / system）
//
// 设计原则：
//   - 最佳努力（best-effort）：记录失败不影响业务调用
//   - 可配置：可选择是否记录 payload（避免敏感数据）
//   - 低开销：内存版记录开销 < 1μs

// TaskRecorder 任务记录中间件
type TaskRecorder struct {
	store    TaskStore
	recordIO bool // 是否记录输入输出（默认 false，避免敏感数据）
}

// NewTaskRecorder 创建任务记录器
//   - store: 任务存储（nil 则不记录）
//   - recordIO: 是否记录输入输出 payload（false = 只记录元数据）
func NewTaskRecorder(store TaskStore, recordIO bool) *TaskRecorder {
	return &TaskRecorder{store: store, recordIO: recordIO}
}

// Enabled 是否启用了任务记录
func (tr *TaskRecorder) Enabled() bool {
	return tr != nil && tr.store != nil
}

// Store 获取底层存储
func (tr *TaskRecorder) Store() TaskStore {
	return tr.store
}

// ─── ctx 中的任务相关 value ───────────────────────────────────────────────

type taskRecorderCtxKey struct{}
type taskRecordCtxKey struct{}

// WithTaskRecorder 将 TaskRecorder 存入 ctx（供服务内部使用）
func WithTaskRecorder(ctx ServiceContext, tr *TaskRecorder) ServiceContext {
	return WithValue(ctx, taskRecorderCtxKey{}, tr)
}

// TaskRecorderFromCtx 从 ctx 中获取 TaskRecorder
func TaskRecorderFromCtx(ctx ServiceContext) *TaskRecorder {
	if v := valueFrom(ctx, taskRecorderCtxKey{}); v != nil {
		if tr, ok := v.(*TaskRecorder); ok {
			return tr
		}
	}
	return nil
}

// WithTaskRecord 将当前任务记录存入 ctx（服务内部可更新进度/阶段）
func WithTaskRecord(ctx ServiceContext, taskID string) ServiceContext {
	return WithValue(ctx, taskRecordCtxKey{}, taskID)
}

// TaskIDFromCtx 从 ctx 中获取当前任务 ID
func TaskIDFromCtx(ctx ServiceContext) string {
	if v := valueFrom(ctx, taskRecordCtxKey{}); v != nil {
		if id, ok := v.(string); ok {
			return id
		}
	}
	return ""
}

// ─── WrapCall: 包装服务调用，自动记录任务 ────────────────────────────────
//
// 这是 MicroKernel.Call 的任务记录包装器。
// 调用流程：
//  1. 创建 TaskRecord（status=pending）
//  2. 标记为 running
//  3. 执行实际服务调用
//  4. 标记为 success/failed，记录耗时
//
// 注意：记录失败只 log，不影响业务调用。

// WrapSyncCall 包装同步服务调用（短任务，直接等结果）
func (tr *TaskRecorder) WrapSyncCall(
	ctx ServiceContext,
	serviceName, method string,
	payload any,
	fn func(ServiceContext) (json.RawMessage, error),
) (json.RawMessage, error) {
	if !tr.Enabled() {
		return fn(ctx)
	}

	taskType := TaskType(fmt.Sprintf("%s.%s", serviceName, method))
	now := time.Now()

	// 1. 创建任务记录
	task := &TaskRecord{
		ID:          nextID(),
		TraceID:     ctx.TraceID(),
		RequestID:   ctx.RequestID(),
		ParentID:    TaskIDFromCtx(ctx), // 嵌套任务的父 ID
		Type:        taskType,
		Service:     serviceName,
		Method:      method,
		CreatedAt:   now,
		Status:      TaskStatusPending,
		Priority:    TaskPriorityNormal,
		TenantID:    TenantFromContext(ctx),
		TriggeredBy: triggeredByFromCtx(ctx),
		RunID:       runIDFromCtx(ctx),
		Tags:        tagsFromCtx(ctx),
	}

	// 输入 payload（可选）
	if tr.recordIO && payload != nil {
		if data, err := json.Marshal(payload); err == nil {
			task.InputJSON = data
		}
	}

	if err := tr.store.Create(task); err != nil {
		// 记录失败不影响业务
		fmt.Printf("[kernel] task create failed: %v\n", err)
		return fn(ctx)
	}

	// 将 task ID 存入 ctx（服务内部可更新）
	callCtx := WithTaskRecord(ctx, task.ID)

	// 2. 标记 running
	_ = tr.store.Update(task.ID, func(r *TaskRecord) {
		r.Status = TaskStatusRunning
		now2 := time.Now()
		r.StartedAt = &now2
	})

	// 3. 执行调用（用带 taskID 的 ctx）
	start := time.Now()
	result, callErr := fn(callCtx)
	duration := time.Since(start)

	// 4. 标记终态
	_ = tr.store.Update(task.ID, func(r *TaskRecord) {
		now3 := time.Now()
		r.CompletedAt = &now3
		r.DurationMs = duration.Milliseconds()
		r.Progress = 100

		if callErr != nil {
			r.Status = TaskStatusFailed
			r.Error = callErr.Error()
			// 不记录完整堆栈到 errorDetail（避免敏感信息）
			// 如需调试，从日志里找
		} else {
			r.Status = TaskStatusSuccess
			if tr.recordIO && result != nil {
				r.OutputJSON = result
			}
		}
	})

	return result, callErr
}

// UpdateTaskProgress 服务内部调用：更新任务进度
//
// 用法（服务内部）：
//
//	if tr := kernel.TaskRecorderFromCtx(ctx); tr != nil {
//	    tr.UpdateProgress(ctx, 50, "读取中", "10 MB/s", "2分钟")
//	}
func (tr *TaskRecorder) UpdateProgress(ctx ServiceContext, progress int, phase, speed, eta string) {
	if !tr.Enabled() {
		return
	}
	taskID := TaskIDFromCtx(ctx)
	if taskID == "" {
		return
	}
	_ = tr.store.Update(taskID, func(r *TaskRecord) {
		if progress > 0 {
			r.Progress = progress
		}
		if phase != "" {
			r.Phase = phase
		}
		if speed != "" {
			r.Speed = speed
		}
		if eta != "" {
			r.Eta = eta
		}
	})
}

// ─── ctx 辅助：触发来源 / runId / tags ───────────────────────────────────

type triggeredByKey struct{}
type runIDKey struct{}
type tagsKey struct{}

// WithTriggeredBy 设置触发来源
func WithTriggeredBy(ctx ServiceContext, by string) ServiceContext {
	return WithValue(ctx, triggeredByKey{}, by)
}

func triggeredByFromCtx(ctx ServiceContext) string {
	if v := valueFrom(ctx, triggeredByKey{}); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// WithRunID 设置 workflow run ID
func WithRunID(ctx ServiceContext, runID string) ServiceContext {
	return WithValue(ctx, runIDKey{}, runID)
}

func runIDFromCtx(ctx ServiceContext) string {
	if v := valueFrom(ctx, runIDKey{}); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// WithTags 设置自定义标签
func WithTags(ctx ServiceContext, tags map[string]string) ServiceContext {
	return WithValue(ctx, tagsKey{}, tags)
}

func tagsFromCtx(ctx ServiceContext) map[string]string {
	if v := valueFrom(ctx, tagsKey{}); v != nil {
		if m, ok := v.(map[string]string); ok {
			return m
		}
	}
	return nil
}
