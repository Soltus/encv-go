package service

// task_manager_crud.go — 拆分自 task_manager.go

import (
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"time"

	"github.com/Soltus/encv-go/pkg/tasksystem"
	"github.com/google/uuid"
)

func (tm *TaskManager) Create(taskType, sourcePath, targetPath, password string, version int, pluginName string) *MobileTask {
	task := &MobileTask{
		ID:               uuid.New().String(),
		Type:             taskType,
		SourcePath:       sourcePath,
		TargetPath:       targetPath,
		Password:         password,
		Status:           "queued",
		Progress:         0,
		ContainerVersion: version,
		PluginName:       pluginName,
		CreatedAt:        time.Now(),
	}

	// 🆕 2026-06-15 multi-mount（spec Phase C1+C2）
	//   两种 sourcePath 形式都尝试 mount 解析：
	//   1. /d/<mount>/... 形式（new format，front-end 切到 B4 后默认用这个）
	//      → 显式 mount 解析（longest prefix match）
	//   2. 旧绝对路径（/storage/emulated/0/foo, /foo 等）
	//      → mount.Resolve fallback 到 primary mount（自动映射）
	//   3. 非 / 开头（relative path） → 跳过 mount 解析（旧 SafeResolveToAbsPath 处理）
	//
	//   targetPath 也同样处理（如果非空）。
	if tm.mountResolver != nil {
		tm.tryAttachMount(task, sourcePath, false /* isTarget */)
		if targetPath != "" {
			tm.tryAttachMount(task, targetPath, true /* isTarget */)
		}
	}

	tm.mu.Lock()
	tm.tasks[task.ID] = task
	tm.mu.Unlock()

	// 🆕 2026-06-23 WS 时序修复（spec ws-timing-batch-throughput-100k Task 1）：
	//   - Create 不再内部广播 task:created（避免 admin_handlers / mobile_api / rollback_manager
	//     等直接调 Create 的路径在 runId 未设置时就广播 → 前端收到无 runId 的 task 变孤儿）
	//   - 改由 CreateWithRunMeta（设置 runId 之后）或外部调用方（补 RunId 兜底后）负责广播
	//   - 持久化改用 saveTaskSingle（O(1) 单行写），替代 saveTasks（全表写）
	tm.saveTaskSingle(task)

	slog.Info("Task created", "id", task.ID, "type", taskType, "source", sourcePath,
		"target", targetPath, "version", version,
		"mountId", task.MountID, "mountSubPath", task.MountSubPath,
		"targetMountId", task.TargetMountID)
	return task
}

func (tm *TaskManager) CreateWithExtras(taskType, sourcePath, targetPath, password, secondaryPassword string, version int, pluginName string, extras map[string]string) *MobileTask {
	task := tm.Create(taskType, sourcePath, targetPath, password, version, pluginName)
	task.SecondaryPassword = secondaryPassword
	task.ExtraFields = extras
	return task
}

func (tm *TaskManager) BroadcastCreated(task *MobileTask) {
	if tm.broadcaster != nil {
		tm.broadcaster.Broadcast("task:created", task)
	}
}

func (tm *TaskManager) FinalizeCreatedTask(task *MobileTask) {
	if task.RunId == "" {
		task.RunId = "manual-" + task.ID
	}
	tm.saveTaskSingle(task)
	tm.BroadcastCreated(task)
}

func (tm *TaskManager) CreateWithRunMeta(
	taskType, sourcePath, targetPath, password, secondaryPassword string,
	version int, pluginName string,
	extras map[string]string,
	cipherMode int,
	compressionMode string,
	runId string,
	triggeredBy string,
) *MobileTask {
	task := tm.CreateWithExtras(taskType, sourcePath, targetPath, password, secondaryPassword,
		version, pluginName, extras)
	task.CipherMode = cipherMode
	task.CompressionMode = compressionMode
	// 🆕 2026-06-22 v2 架构重写：runId 永不为空的兜底（根治"任务逃逸"）
	//   历史 bug：前端 createTask 漏传 runId（移动端 Capacitor 调用时偶发丢参）→ task.RunId = ''
	//     → 前端按 runId 分组时这个 task 变孤儿（不入任何 group）
	//   修法：后端兜底。runId 为空 → 用 "manual-" + task.ID 派生稳定 runId
	//     （保证每个 task 都有非空 runId，前端按 runId 分组永远有归属）
	//   triggeredBy 已有兜底（'' → 'user'）
	if runId == "" {
		runId = "manual-" + task.ID
	}
	task.RunId = runId
	if triggeredBy == "" {
		triggeredBy = "user"
	}
	task.TriggeredBy = triggeredBy
	// 🆕 2026-06-22 Q6A：单行写（O(1)），替代 saveTasks() 全表写
	tm.saveTaskSingle(task)
	// 🆕 2026-06-23 WS 时序修复（spec ws-timing-batch-throughput-100k Task 1）：
	//   - Create 不再内部广播，改由 CreateWithRunMeta 在 runId 设置之后广播
	//   - 保证前端收到的 task:created payload 一定带 runId（不会产生孤儿 group）
	if tm.broadcaster != nil {
		tm.broadcaster.Broadcast("task:created", task)
	}
	return task
}

type BatchTaskSpec struct {
	Type              string
	SourcePath        string
	TargetPath        string
	Password          string
	SecondaryPassword string
	Version           int
	PluginName        string
	ExtraFields       map[string]string
	CipherMode        int
	CompressionMode   string
}

func (tm *TaskManager) CreateBatch(specs []BatchTaskSpec, runId string, triggeredBy string) []*MobileTask {
	if len(specs) == 0 {
		return nil
	}
	// runId / triggeredBy 兜底（跟 CreateWithRunMeta 一致）
	if runId == "" {
		runId = "manual-batch-" + uuid.New().String()[:8]
	}
	if triggeredBy == "" {
		triggeredBy = "user"
	}

	// 并发度：从 store 获取 hint，默认 1（兼容旧 store=nil 的情况）
	concurrency := 1
	if tm.store != nil {
		concurrency = tm.store.ConcurrencyHint()
	}
	if concurrency < 1 {
		concurrency = 1
	}

	// 单并发：走原串行逻辑（简单、零额外开销）
	if concurrency == 1 || len(specs) <= 2 {
		tasks := make([]*MobileTask, 0, len(specs))
		for _, spec := range specs {
			compressionMode := spec.CompressionMode
			if compressionMode == "" {
				compressionMode = "none"
			}
			task := tm.CreateWithRunMeta(
				spec.Type, spec.SourcePath, spec.TargetPath,
				spec.Password, spec.SecondaryPassword, spec.Version, spec.PluginName, spec.ExtraFields,
				spec.CipherMode, compressionMode,
				runId, triggeredBy,
			)
			tasks = append(tasks, task)
		}
		return tasks
	}

	// 多并发：worker pool 模式
	// 使用 channel 分发任务，收集结果
	type result struct {
		index int
		task  *MobileTask
	}

	jobs := make(chan int, len(specs))
	results := make(chan result, len(specs))

	// 启动 worker
	var wg sync.WaitGroup
	for w := 0; w < concurrency; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				spec := specs[i]
				compressionMode := spec.CompressionMode
				if compressionMode == "" {
					compressionMode = "none"
				}
				// 创建单个 task（内部会加锁写内存 map + 持久化 + 广播）
				task := tm.CreateWithRunMeta(
					spec.Type, spec.SourcePath, spec.TargetPath,
					spec.Password, spec.SecondaryPassword, spec.Version, spec.PluginName, spec.ExtraFields,
					spec.CipherMode, compressionMode,
					runId, triggeredBy,
				)
				results <- result{index: i, task: task}
			}
		}()
	}

	// 分发任务
	for i := range specs {
		jobs <- i
	}
	close(jobs)

	// 等待所有 worker 完成
	go func() {
		wg.Wait()
		close(results)
	}()

	// 收集结果（按原顺序排列）
	tasks := make([]*MobileTask, len(specs))
	for r := range results {
		tasks[r.index] = r.task
	}

	return tasks
}

func (tm *TaskManager) List() []*MobileTask {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	result := make([]*MobileTask, 0, len(tm.tasks))
	for _, t := range tm.tasks {
		result = append(result, t)
	}
	return result
}

func (tm *TaskManager) ListPaginated(runId string, offset, limit int) ([]*MobileTask, int) {
	// 🆕 优先走 SQLite store SQL
	if tm.store != nil {
		filter := tasksystem.TaskFilter{
			RunID:  runId,
			Limit:  limit,
			Offset: offset,
		}
		datas, err := tm.store.ListTasks(filter)
		if err != nil {
			slog.Warn("ListPaginated: store.ListTasks failed, fallback to in-memory",
				"runId", runId, "offset", offset, "limit", limit, "error", err)
		} else {
			// totalCount：runId 非空时走 CountByRunId（SQL COUNT），runId 为空时走 ListTasks 全量
			totalCount := 0
			if runId != "" {
				counts, err := tm.store.CountByRunId(runId)
				if err != nil {
					slog.Warn("ListPaginated: store.CountByRunId failed", "runId", runId, "error", err)
					// 降级：用当前页长度估算（不精确但避免崩溃）
					totalCount = len(datas)
				} else {
					for _, c := range counts {
						totalCount += c
					}
				}
			} else {
				// runId 为空：totalCount 用 ListTasks(filter{Limit:0}) 拿全量长度
				allDatas, err := tm.store.ListTasks(tasksystem.TaskFilter{})
				if err != nil {
					slog.Warn("ListPaginated: store.ListTasks (count) failed", "error", err)
					totalCount = len(datas)
				} else {
					totalCount = len(allDatas)
				}
			}
			result := make([]*MobileTask, 0, len(datas))
			for _, d := range datas {
				result = append(result, dataToMobileTask(d))
			}
			return result, totalCount
		}
	}

	// 降级：内存过滤（store == nil 或 store 查询失败）
	all := tm.List()

	filtered := all
	if runId != "" {
		filtered = make([]*MobileTask, 0, len(all))
		for _, t := range all {
			if t.RunId == runId {
				filtered = append(filtered, t)
			}
		}
	}
	totalCount := len(filtered)

	if offset < 0 {
		offset = 0
	}
	if offset > totalCount {
		offset = totalCount
	}
	if limit < 0 {
		limit = 0
	}
	end := offset + limit
	if end > totalCount {
		end = totalCount
	}
	return filtered[offset:end], totalCount
}

func (tm *TaskManager) GetRunSummary(runId string) RunSummary {
	summary := RunSummary{RunID: runId}

	if runId == "" {
		return summary
	}

	var counts map[string]int

	// 优先走 SQLite store SQL
	if tm.store != nil {
		c, err := tm.store.CountByRunId(runId)
		if err != nil {
			slog.Warn("GetRunSummary: store.CountByRunId failed, fallback to in-memory",
				"runId", runId, "error", err)
		} else {
			counts = c
		}
	}

	// 降级：内存遍历
	if counts == nil {
		counts = make(map[string]int)
		for _, t := range tm.List() {
			if t.RunId == runId {
				counts[t.Status]++
			}
		}
	}

	// 汇总
	for status, c := range counts {
		summary.Total += c
		switch status {
		case "completed":
			summary.Passed += c
		case "failed":
			summary.Failed += c
		case "running":
			summary.Running += c
		case "queued", "pending", "paused":
			summary.Pending += c
		case "cancelled":
			summary.Cancelled += c
		}
	}

	// 完成百分比（终态 task / total * 100）
	if summary.Total > 0 {
		terminal := summary.Passed + summary.Failed + summary.Cancelled
		summary.Percent = terminal * 100 / summary.Total
	}

	return summary
}

func (tm *TaskManager) ListRuns() []tasksystem.RunInfo {
	// 优先走 SQLite store SQL
	if tm.store != nil {
		runs, err := tm.store.ListRuns()
		if err != nil {
			slog.Warn("ListRuns: store.ListRuns failed, fallback to in-memory", "error", err)
		} else {
			return runs
		}
	}

	// 降级：内存遍历
	all := tm.List()
	runMap := make(map[string]*tasksystem.RunInfo)
	for _, t := range all {
		if t.RunId == "" {
			continue
		}
		if existing, ok := runMap[t.RunId]; ok {
			// 取最早的 created_at 作为 startedAt
			if t.CreatedAt.Before(existing.StartedAt) {
				existing.StartedAt = t.CreatedAt
			}
			// triggeredBy 取第一个非空的
			if existing.TriggeredBy == "" && t.TriggeredBy != "" {
				existing.TriggeredBy = t.TriggeredBy
			}
		} else {
			info := &tasksystem.RunInfo{
				RunID:       t.RunId,
				StartedAt:   t.CreatedAt,
				TriggeredBy: t.TriggeredBy,
			}
			runMap[t.RunId] = info
		}
	}

	result := make([]tasksystem.RunInfo, 0, len(runMap))
	for _, info := range runMap {
		result = append(result, *info)
	}
	// 按 startedAt 倒序
	sort.Slice(result, func(i, j int) bool {
		return result[i].StartedAt.After(result[j].StartedAt)
	})
	return result
}

func (tm *TaskManager) Get(id string) (*MobileTask, error) {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	task, ok := tm.tasks[id]
	if !ok {
		return nil, errors.New("task not found")
	}
	return task, nil
}

func (tm *TaskManager) Cancel(id string) (*MobileTask, error) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	task, ok := tm.tasks[id]
	if !ok {
		return nil, errors.New("task not found")
	}

	if task.Status == "running" {
		task.Status = "cancelling"
		if task.cancelFn != nil {
			task.cancelFn()
		}
	} else {
		task.Status = "cancelled"
		now := time.Now()
		task.CompletedAt = &now
	}
	if tm.broadcaster != nil {
		tm.broadcaster.Broadcast("task:update", map[string]interface{}{
			"id":     id,
			"status": task.Status,
		})
	}
	return task, nil
}

func (tm *TaskManager) CancelByRunId(runId string) error {
	if runId == "" {
		return errors.New("runId is required")
	}

	// 锁内收集需要取消的 task ID（避免长时间持锁，Cancel 会自己加锁）
	tm.mu.RLock()
	var toCancel []string
	for id, task := range tm.tasks {
		if task.RunId != runId {
			continue
		}
		if isTerminalStatus(task.Status) {
			continue
		}
		toCancel = append(toCancel, id)
	}
	tm.mu.RUnlock()

	for _, id := range toCancel {
		// 复用 Cancel 逻辑：状态转换 + cancelFn + 广播 task:update
		cancelledTask, err := tm.Cancel(id)
		if err != nil {
			// 单个取消失败不阻断整体（task 可能已被 worker 并发处理）
			slog.Warn("CancelByRunId: cancel task failed", "taskId", id, "runId", runId, "error", err)
			continue
		}
		// Cancel 不持久化，这里补单行持久化（与 failTask 模式一致）
		tm.saveTaskSingle(cancelledTask)
	}

	return nil
}

func (tm *TaskManager) ResumeTask(id string) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	task, ok := tm.tasks[id]
	if !ok {
		return errors.New("task not found")
	}

	if task.Status != "paused" {
		return fmt.Errorf("cannot resume task with status %q", task.Status)
	}

	task.Status = "queued"
	task.Error = ""

	if tm.broadcaster != nil {
		tm.broadcaster.Broadcast("task:update", map[string]interface{}{
			"id":     id,
			"status": "queued",
		})
	}

	tm.saveTaskSingle(task)
	return nil
}

func (tm *TaskManager) ResumePausedByRunId(runId string) (int, error) {
	if runId == "" {
		return 0, errors.New("runId is required")
	}

	tm.mu.Lock()
	var toResume []*MobileTask
	for _, task := range tm.tasks {
		if task.RunId != runId {
			continue
		}
		if task.Status != "paused" {
			continue
		}
		toResume = append(toResume, task)
	}
	tm.mu.Unlock()

	for _, task := range toResume {
		if err := tm.ResumeTask(task.ID); err != nil {
			slog.Warn("ResumePausedByRunId: resume task failed", "taskId", task.ID, "runId", runId, "error", err)
		}
	}

	return len(toResume), nil
}

func (tm *TaskManager) ResumeAllPaused() (int, error) {
	tm.mu.RLock()
	var toResume []string
	for id, task := range tm.tasks {
		if task.Status == "paused" {
			toResume = append(toResume, id)
		}
	}
	tm.mu.RUnlock()

	for _, id := range toResume {
		if err := tm.ResumeTask(id); err != nil {
			slog.Warn("ResumeAllPaused: resume task failed", "taskId", id, "error", err)
		}
	}

	return len(toResume), nil
}

func (tm *TaskManager) RemoveTask(id string) error {
	tm.mu.Lock()
	if _, ok := tm.tasks[id]; !ok {
		tm.mu.Unlock()
		return errors.New("task not found")
	}

	delete(tm.tasks, id)
	tm.mu.Unlock()

	tm.saveTasks()

	slog.Info("Task removed", "id", id)
	if tm.broadcaster != nil {
		tm.broadcaster.Broadcast("task:removed", map[string]interface{}{
			"id": id,
		})
	}
	return nil
}

func (tm *TaskManager) ClearCompleted() int {
	tm.mu.Lock()
	removed := 0
	for id, task := range tm.tasks {
		if task.Status == "completed" || task.Status == "failed" || task.Status == "cancelled" {
			delete(tm.tasks, id)
			removed++
		}
	}
	tm.mu.Unlock()

	if removed > 0 {
		tm.saveTasks()
		slog.Info("Cleared completed tasks", "count", removed)
		if tm.broadcaster != nil {
			tm.broadcaster.Broadcast("task:cleared", map[string]interface{}{
				"count": removed,
			})
		}
	}
	return removed
}

func (tm *TaskManager) Retry(id string) (*MobileTask, error) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	task, ok := tm.tasks[id]
	if !ok {
		return nil, errors.New("task not found")
	}

	task.Status = "queued"
	task.Error = ""
	task.Progress = 0
	if tm.broadcaster != nil {
		tm.broadcaster.Broadcast("task:update", map[string]interface{}{
			"id":       id,
			"status":   "queued",
			"progress": 0,
		})
	}
	return task, nil
}
