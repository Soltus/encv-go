package kernel

import (
	"encoding/json"
	"time"

	"github.com/Soltus/encv-go/pkg/tasksystem"
)

// TaskRecordingConfig 微内核任务记录配置。
// 非 nil 时，所有经过 MicroKernel.Call 的服务调用都会被记录为任务。
type TaskRecordingConfig struct {
	// Store 任务存储（tasksystem.Store 接口，SQLite / libsql / turso 均可）。
	Store tasksystem.Store

	// TriggeredBy 默认触发者（如 "system"、"automation"）。
	// ctx 中通过 WithTriggeredBy 可覆盖。
	DefaultTriggeredBy string

	// RecordErrors 是否记录失败调用（默认 true）。
	RecordErrors bool

	// RecordSuccess 是否记录成功调用（默认 true）。
	RecordSuccess bool
}

// triggeredByKey ctx 值 key
type triggeredByKeyType struct{}

var triggeredByKey = triggeredByKeyType{}

// WithTriggeredBy 设置 ctx 中的任务触发者。
func WithTriggeredBy(ctx ServiceContext, triggeredBy string) ServiceContext {
	if c, ok := ctx.(*serviceCtx); ok {
		c.setValue(triggeredByKey, triggeredBy)
	}
	return ctx
}

// getTriggeredBy 从 ctx 中获取触发者，没有则用默认值。
func getTriggeredBy(ctx ServiceContext, def string) string {
	if c, ok := ctx.(*serviceCtx); ok {
		if v, ok := c.getValue(triggeredByKey).(string); ok {
			return v
		}
	}
	return def
}

// runIdKey ctx 值 key
type runIdKeyType struct{}

var runIdKey = runIdKeyType{}

// WithRunId 设置 ctx 中的 run ID（自动化测试 / workflow 场景）。
func WithRunId(ctx ServiceContext, runId string) ServiceContext {
	if c, ok := ctx.(*serviceCtx); ok {
		c.setValue(runIdKey, runId)
	}
	return ctx
}

// getRunId 从 ctx 中获取 run ID。
func getRunId(ctx ServiceContext) string {
	if c, ok := ctx.(*serviceCtx); ok {
		if v, ok := c.getValue(runIdKey).(string); ok {
			return v
		}
	}
	return ""
}

// recordMicroserviceTask 记录一次微服务调用为任务。
// 在调用前创建 pending 任务，调用后更新为 success/failure。
func (mk *MicroKernel) recordMicroserviceTask(
	ctx ServiceContext,
	serviceName, method string,
	payload any,
	callFn func() (json.RawMessage, error),
) (json.RawMessage, error) {
	if mk.taskRecorder == nil {
		return callFn()
	}
	cfg := mk.taskRecorder
	if !cfg.RecordErrors && !cfg.RecordSuccess {
		return callFn()
	}

	triggeredBy := getTriggeredBy(ctx, cfg.DefaultTriggeredBy)
	if triggeredBy == "" {
		triggeredBy = "system"
	}

	taskType := tasksystem.TaskType(serviceName + "." + method)
	if !taskType.IsMicroservice() {
		taskType = tasksystem.TaskType(serviceName + "." + method)
	}

	task := tasksystem.TaskData{
		ID:            generateTaskID(),
		Type:          taskType,
		Status:        tasksystem.StatusRunning,
		ServiceName:   serviceName,
		MethodName:    method,
		TenantID:      TenantFromContext(ctx),
		TriggeredBy:   triggeredBy,
		RunID:         getRunId(ctx),
		CreatedAt:     time.Now().UTC(),
		Priority:      0,
		Attempts:      1,
	}

	if payload != nil {
		if inputJSON, err := json.Marshal(payload); err == nil {
			task.InputJSON = string(inputJSON)
		}
	}

	_ = cfg.Store.CreateTask(task)

	start := time.Now()
	resp, callErr := callFn()
	durationMs := time.Since(start).Milliseconds()

	task.DurationMs = durationMs
	task.CompletedAt = &[]time.Time{time.Now().UTC()}[0]

	if callErr != nil {
		if cfg.RecordErrors {
			task.Status = tasksystem.StatusFailed
			task.Error = callErr.Error()
			_ = cfg.Store.UpdateTask(task)
		}
		return resp, callErr
	}

	if cfg.RecordSuccess {
		task.Status = tasksystem.StatusCompleted
		if resp != nil {
			task.OutputJSON = string(resp)
		}
		_ = cfg.Store.UpdateTask(task)
	}

	return resp, nil
}

// generateTaskID 生成任务 ID。
func generateTaskID() string {
	return "mk-" + time.Now().UTC().Format("20060102-150405") + "-" + randomHex(6)
}

// randomHex 生成 n 字节的十六进制字符串。
func randomHex(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(time.Now().UnixNano() & 0xff)
		time.Sleep(time.Nanosecond)
	}
	const hex = "0123456789abcdef"
	r := make([]byte, n*2)
	for i, v := range b {
		r[i*2] = hex[v>>4]
		r[i*2+1] = hex[v&0x0f]
	}
	return string(r)
}
