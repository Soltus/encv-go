package service

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/Soltus/encv-go/internal/config"
	"github.com/Soltus/encv-go/pkg/tasksystem"
	"github.com/Soltus/encv-go/pkg/tasksystem/store/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// 🆕 2026-06-23 Task 13.3：service 层 SQL 路径测试
//
// 验证 TaskManager 在 store != nil 时走 SQL（不是内存过滤）：
//   - GetRunSummary：store.CountByRunId 返回的 status counts 正确汇总
//   - ListRuns：store.ListRuns 返回所有 run（去重 runId + 倒序）
//   - ListPaginated：store.ListTasks + CountByRunId 走 SQL 分页
//
// 同时验证 store == nil 时降级为内存遍历（向后兼容）。

// newTestTaskManagerWithStore 创建接入真实 SQLite store 的 TaskManager（用于 SQL 路径测试）。
// 返回 tm + 底层 store（测试可直接操作 store 验证 SQL 行为）。
func newTestTaskManagerWithStore(t *testing.T) (*TaskManager, *sqlite.Store) {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	store, err := sqlite.New(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { store.Close() })

	cfg := &config.Config{Password: "test-password-123"}
	mb := new(MockBroadcaster)
	mb.On("Broadcast", mock.Anything, mock.Anything).Return()

	// 用 NewTaskManagerWithStore 创建（启动 worker goroutine + loadTasks）
	// 注意：NewTaskManagerWithStore 会启动 worker goroutine，需要 defer tm.Stop()
	servingDir := t.TempDir()
	tm := NewTaskManagerWithStore(servingDir, cfg, mb, store)
	t.Cleanup(func() { tm.Stop() })

	return tm, store
}

// makeMobileTask 构造一个最小可用的 MobileTask（用于直接插入 tm.tasks 内存 map）
func makeMobileTask(id, runId, status string, createdAt time.Time) *MobileTask {
	return &MobileTask{
		ID:          id,
		Type:        "encrypt",
		SourcePath:  "/test/" + id + ".mp4",
		Status:      status,
		Progress:    0,
		RunId:       runId,
		TriggeredBy: "automation",
		CreatedAt:   createdAt,
	}
}

// ============ GetRunSummary SQL 路径 ============

func TestGetRunSummary_SQLPath(t *testing.T) {
	tm, store := newTestTaskManagerWithStore(t)

	runId := "run-summary-sql"
	baseTime := time.Date(2026, 6, 23, 10, 0, 0, 0, time.UTC)

	// 直接写 store（绕过 tm.Create，避免触发 worker/broadcaster）
	tasks := []tasksystem.TaskData{
		{ID: "s1", Type: tasksystem.TaskTypeEncrypt, Status: tasksystem.StatusCompleted, RunID: runId, TriggeredBy: "automation", CreatedAt: baseTime},
		{ID: "s2", Type: tasksystem.TaskTypeEncrypt, Status: tasksystem.StatusCompleted, RunID: runId, TriggeredBy: "automation", CreatedAt: baseTime.Add(1 * time.Second)},
		{ID: "s3", Type: tasksystem.TaskTypeEncrypt, Status: tasksystem.StatusCompleted, RunID: runId, TriggeredBy: "automation", CreatedAt: baseTime.Add(2 * time.Second)},
		{ID: "s4", Type: tasksystem.TaskTypeEncrypt, Status: tasksystem.StatusFailed, RunID: runId, TriggeredBy: "automation", CreatedAt: baseTime.Add(3 * time.Second)},
		{ID: "s5", Type: tasksystem.TaskTypeEncrypt, Status: tasksystem.StatusRunning, RunID: runId, TriggeredBy: "automation", CreatedAt: baseTime.Add(4 * time.Second)},
		{ID: "s6", Type: tasksystem.TaskTypeEncrypt, Status: tasksystem.StatusQueued, RunID: runId, TriggeredBy: "automation", CreatedAt: baseTime.Add(5 * time.Second)},
		{ID: "s7", Type: tasksystem.TaskTypeEncrypt, Status: tasksystem.StatusCancelled, RunID: runId, TriggeredBy: "automation", CreatedAt: baseTime.Add(6 * time.Second)},
	}
	for _, task := range tasks {
		require.NoError(t, store.CreateTask(task))
	}

	// 调 GetRunSummary（应走 store.CountByRunId SQL 路径）
	summary := tm.GetRunSummary(runId)

	assert.Equal(t, runId, summary.RunID)
	assert.Equal(t, 7, summary.Total, "Total = 3 completed + 1 failed + 1 running + 1 queued + 1 cancelled")
	assert.Equal(t, 3, summary.Passed, "Passed = 3 completed")
	assert.Equal(t, 1, summary.Failed, "Failed = 1")
	assert.Equal(t, 1, summary.Running, "Running = 1")
	assert.Equal(t, 1, summary.Pending, "Pending = 1 queued")
	assert.Equal(t, 1, summary.Cancelled, "Cancelled = 1")
	// 完成百分比 = (passed + failed + cancelled) / total * 100 = (3+1+1)/7*100 = 71
	assert.Equal(t, 71, summary.Percent, "Percent = (3+1+1)/7*100 = 71")
}

func TestGetRunSummary_EmptyRunId(t *testing.T) {
	tm, _ := newTestTaskManagerWithStore(t)

	summary := tm.GetRunSummary("")
	assert.Equal(t, "", summary.RunID)
	assert.Equal(t, 0, summary.Total)
	assert.Equal(t, 0, summary.Passed)
}

func TestGetRunSummary_NonexistentRunId(t *testing.T) {
	tm, _ := newTestTaskManagerWithStore(t)

	summary := tm.GetRunSummary("nonexistent-run")
	assert.Equal(t, "nonexistent-run", summary.RunID)
	assert.Equal(t, 0, summary.Total)
}

// ============ GetRunSummary 内存降级路径（store == nil） ============

func TestGetRunSummary_InMemoryFallback(t *testing.T) {
	// 用 newTestTaskManager（store == nil）测试内存降级
	mb := new(MockBroadcaster)
	mb.On("Broadcast", mock.Anything, mock.Anything).Return()
	tm := newTestTaskManager(mb)

	runId := "run-summary-mem"
	baseTime := time.Now().UTC()

	// 直接插入内存 map
	tasks := []*MobileTask{
		makeMobileTask("m1", runId, "completed", baseTime),
		makeMobileTask("m2", runId, "completed", baseTime.Add(1*time.Second)),
		makeMobileTask("m3", runId, "failed", baseTime.Add(2*time.Second)),
		makeMobileTask("m4", runId, "running", baseTime.Add(3*time.Second)),
	}
	tm.mu.Lock()
	for _, t := range tasks {
		tm.tasks[t.ID] = t
	}
	tm.mu.Unlock()

	// 调 GetRunSummary（应走内存遍历降级路径）
	summary := tm.GetRunSummary(runId)

	assert.Equal(t, runId, summary.RunID)
	assert.Equal(t, 4, summary.Total)
	assert.Equal(t, 2, summary.Passed)
	assert.Equal(t, 1, summary.Failed)
	assert.Equal(t, 1, summary.Running)
}

// ============ ListRuns SQL 路径 ============

func TestListRuns_SQLPath(t *testing.T) {
	tm, store := newTestTaskManagerWithStore(t)

	baseTime := time.Date(2026, 6, 23, 10, 0, 0, 0, time.UTC)
	runOld := "run-old-sql"
	runNew := "run-new-sql"

	// run-old：2 个 task，最早
	require.NoError(t, store.CreateTask(tasksystem.TaskData{
		ID: "o1", Type: tasksystem.TaskTypeEncrypt, Status: tasksystem.StatusCompleted,
		RunID: runOld, TriggeredBy: "automation", CreatedAt: baseTime,
	}))
	require.NoError(t, store.CreateTask(tasksystem.TaskData{
		ID: "o2", Type: tasksystem.TaskTypeEncrypt, Status: tasksystem.StatusCompleted,
		RunID: runOld, TriggeredBy: "automation", CreatedAt: baseTime.Add(1 * time.Second),
	}))

	// run-new：1 个 task，最新
	require.NoError(t, store.CreateTask(tasksystem.TaskData{
		ID: "n1", Type: tasksystem.TaskTypeEncrypt, Status: tasksystem.StatusRunning,
		RunID: runNew, TriggeredBy: "ai_agent", CreatedAt: baseTime.Add(100 * time.Second),
	}))

	// 无 runId 的 task（不应出现）
	require.NoError(t, store.CreateTask(tasksystem.TaskData{
		ID: "manual1", Type: tasksystem.TaskTypeEncrypt, Status: tasksystem.StatusCompleted,
		TriggeredBy: "user", CreatedAt: baseTime.Add(200 * time.Second),
	}))

	runs := tm.ListRuns()

	require.Len(t, runs, 2, "应返回 2 个 run（不含 manual）")
	// 倒序：run-new 在前
	assert.Equal(t, runNew, runs[0].RunID)
	assert.Equal(t, runOld, runs[1].RunID)
	// startedAt 是每个 run 最早 task 的 createdAt
	assert.True(t, runs[0].StartedAt.Equal(baseTime.Add(100*time.Second)),
		"runNew StartedAt = %v, want %v", runs[0].StartedAt, baseTime.Add(100*time.Second))
	assert.True(t, runs[1].StartedAt.Equal(baseTime),
		"runOld StartedAt = %v, want %v", runs[1].StartedAt, baseTime)
	// triggeredBy
	assert.Equal(t, "ai_agent", runs[0].TriggeredBy)
	assert.Equal(t, "automation", runs[1].TriggeredBy)
}

func TestListRuns_Empty(t *testing.T) {
	tm, _ := newTestTaskManagerWithStore(t)

	runs := tm.ListRuns()
	assert.Empty(t, runs)
}

// ============ ListRuns 内存降级路径 ============

func TestListRuns_InMemoryFallback(t *testing.T) {
	mb := new(MockBroadcaster)
	mb.On("Broadcast", mock.Anything, mock.Anything).Return()
	tm := newTestTaskManager(mb)

	baseTime := time.Now().UTC()
	tasks := []*MobileTask{
		makeMobileTask("r1", "run-mem-1", "completed", baseTime),
		makeMobileTask("r2", "run-mem-1", "completed", baseTime.Add(1*time.Second)),
		makeMobileTask("r3", "run-mem-2", "running", baseTime.Add(10*time.Second)),
		// 无 runId
		{ID: "m1", Type: "encrypt", SourcePath: "/m1", Status: "completed", CreatedAt: baseTime.Add(20 * time.Second)},
	}
	tm.mu.Lock()
	for _, t := range tasks {
		tm.tasks[t.ID] = t
	}
	tm.mu.Unlock()

	runs := tm.ListRuns()
	require.Len(t, runs, 2, "应返回 2 个 run（不含无 runId 的 task）")
	// 倒序：run-mem-2 在前（createdAt 更晚）
	assert.Equal(t, "run-mem-2", runs[0].RunID)
	assert.Equal(t, "run-mem-1", runs[1].RunID)
}

// ============ ListPaginated SQL 路径 ============

func TestListPaginated_SQLPath(t *testing.T) {
	tm, store := newTestTaskManagerWithStore(t)

	runId := "run-paged-sql"
	baseTime := time.Date(2026, 6, 23, 10, 0, 0, 0, time.UTC)

	// 创建 10 个 task，全部属于 runId
	for i := 0; i < 10; i++ {
		require.NoError(t, store.CreateTask(tasksystem.TaskData{
			ID:     "p" + string(rune('0'+i)),
			Type:   tasksystem.TaskTypeEncrypt,
			Status: tasksystem.StatusCompleted,
			RunID:  runId,
			TriggeredBy: "automation",
			CreatedAt: baseTime.Add(time.Duration(i) * time.Second),
		}))
	}

	// 第 1 页：offset=0, limit=4 → 4 个 task
	page1, total := tm.ListPaginated(runId, 0, 4)
	assert.Equal(t, 10, total, "total = 全部 10 个 task")
	require.Len(t, page1, 4, "page1 应有 4 个 task")

	// 第 2 页：offset=4, limit=4 → 4 个 task
	page2, total := tm.ListPaginated(runId, 4, 4)
	assert.Equal(t, 10, total)
	require.Len(t, page2, 4)

	// 第 3 页：offset=8, limit=4 → 2 个 task（剩余）
	page3, total := tm.ListPaginated(runId, 8, 4)
	assert.Equal(t, 10, total)
	require.Len(t, page3, 2, "page3 应有 2 个 task（剩余）")

	// 第 4 页：offset=12, limit=4 → 0 个 task（超出）
	page4, total := tm.ListPaginated(runId, 12, 4)
	assert.Equal(t, 10, total)
	assert.Empty(t, page4, "page4 应为空（超出总数）")
}

func TestListPaginated_SQLPath_EmptyRunId(t *testing.T) {
	tm, store := newTestTaskManagerWithStore(t)

	baseTime := time.Date(2026, 6, 23, 10, 0, 0, 0, time.UTC)

	// 创建 5 个 task，部分有 runId 部分没有
	require.NoError(t, store.CreateTask(tasksystem.TaskData{
		ID: "x1", Type: tasksystem.TaskTypeEncrypt, Status: tasksystem.StatusCompleted,
		RunID: "run-x", TriggeredBy: "user", CreatedAt: baseTime,
	}))
	require.NoError(t, store.CreateTask(tasksystem.TaskData{
		ID: "x2", Type: tasksystem.TaskTypeEncrypt, Status: tasksystem.StatusCompleted,
		RunID: "run-x", TriggeredBy: "user", CreatedAt: baseTime.Add(1 * time.Second),
	}))
	require.NoError(t, store.CreateTask(tasksystem.TaskData{
		ID: "y1", Type: tasksystem.TaskTypeEncrypt, Status: tasksystem.StatusCompleted,
		TriggeredBy: "user", CreatedAt: baseTime.Add(2 * time.Second),
	}))

	// runId 为空 → 返回所有 task（不过滤 runId）
	page, total := tm.ListPaginated("", 0, 10)
	assert.Equal(t, 3, total, "runId 为空时 total = 全部 task 数")
	assert.Len(t, page, 3)
}

// ============ ListPaginated 内存降级路径 ============

func TestListPaginated_InMemoryFallback(t *testing.T) {
	mb := new(MockBroadcaster)
	mb.On("Broadcast", mock.Anything, mock.Anything).Return()
	tm := newTestTaskManager(mb)

	runId := "run-paged-mem"
	baseTime := time.Now().UTC()

	// 直接插入内存 map（store == nil）
	for i := 0; i < 5; i++ {
		task := makeMobileTask(
			"m"+string(rune('0'+i)),
			runId,
			"completed",
			baseTime.Add(time.Duration(i)*time.Second),
		)
		tm.mu.Lock()
		tm.tasks[task.ID] = task
		tm.mu.Unlock()
	}

	// 第 1 页：offset=0, limit=2 → 2 个 task
	page1, total := tm.ListPaginated(runId, 0, 2)
	assert.Equal(t, 5, total)
	require.Len(t, page1, 2)

	// 第 3 页：offset=4, limit=2 → 1 个 task（剩余）
	page3, total := tm.ListPaginated(runId, 4, 2)
	assert.Equal(t, 5, total)
	require.Len(t, page3, 1)
}
