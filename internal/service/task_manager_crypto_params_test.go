package service

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// 🆕 2026-06-22 Q1A：原 TestCreateWithCryptoParams 引用了不存在的 API
//
// 历史：
// - 2026-06-18 Task 16 设计时假设有 `CreateWithCryptoParams(taskType, source, target,
//   password, secondary, version, plugin, extras, cipherMode, compressionMode)` 方法
// - 实际 2026-06-18 落地的 API 是 `CreateWithRunMeta`，参数顺序完全一致
//   （10 参，多了 runId + triggeredBy）
// - 因此测试一直跑不过，CI 红×8
//
// 修复（Q1A 改名映射）：
// - `CreateWithCryptoParams(...)` → `CreateWithRunMeta(..., '', 'user')`
// - 末尾补 runId="" + triggeredBy="user"（默认值）
// - cipherMode/compressionMode 位置不变
//
// 覆盖：
// - 显式传 cipherMode=1 + compressionMode='zstd' → 字段持久化
// - cipherMode=0 + compressionMode='none'（默认值）→ 字段仍持久化（用户主动选了默认）
// - 兼容 CreateWithExtras（不传 crypto 参数时 CipherMode=0, CompressionMode=""）
// - List() 返回的任务包含 cipherMode / compressionMode 字段
func TestCreateWithRunMeta_PreservesCryptoFields(t *testing.T) {
	mb := new(MockBroadcaster)
	mb.On("Broadcast", "task:created", mock.Anything).Return()
	tm := newTestTaskManager(mb)

	extras := map[string]string{"plugin_password": "test123"}
	task := tm.CreateWithRunMeta(
		"encrypt", "/test/file.mp4", "", "pw", "secondary-pw",
		4, "mp4-plugin", extras,
		1, "zstd",
		"", "user",
	)

	require.NotNil(t, task)
	assert.Equal(t, 1, task.CipherMode, "cipherMode should be persisted as 1 (AES-256-GCM)")
	assert.Equal(t, "zstd", task.CompressionMode, "compressionMode should be persisted as 'zstd'")
	assert.Equal(t, "test123", task.ExtraFields["plugin_password"], "extras should still be preserved")
	assert.Equal(t, "secondary-pw", task.SecondaryPassword, "secondary password should be preserved")
	assert.Equal(t, 4, task.ContainerVersion, "version should be preserved")
	assert.Equal(t, "mp4-plugin", task.PluginName, "pluginName should be preserved")
}

func TestCreateWithRunMeta_DefaultValuesPersisted(t *testing.T) {
	mb := new(MockBroadcaster)
	mb.On("Broadcast", "task:created", mock.Anything).Return()
	tm := newTestTaskManager(mb)

	// 用户主动选了默认值（cipherMode=0=AES-128, compressionMode='none'）
	// 后端仍应持久化（前端回显时需要知道用户选了什么）
	task := tm.CreateWithRunMeta(
		"encrypt", "/test/file.mp4", "", "pw", "",
		4, "", nil,
		0, "none",
		"", "user",
	)

	require.NotNil(t, task)
	assert.Equal(t, 0, task.CipherMode, "cipherMode=0 should be persisted (user explicitly chose default)")
	assert.Equal(t, "none", task.CompressionMode, "compressionMode='none' should be persisted")
}

func TestCreateWithRunMeta_ListReturnsCryptoFields(t *testing.T) {
	mb := new(MockBroadcaster)
	mb.On("Broadcast", "task:created", mock.Anything).Return()
	tm := newTestTaskManager(mb)

	tm.CreateWithRunMeta(
		"encrypt", "/test/file1.mp4", "", "pw", "",
		4, "", nil,
		1, "zstd",
		"", "user",
	)
	tm.CreateWithRunMeta(
		"encrypt", "/test/file2.mp4", "", "pw", "",
		4, "", nil,
		0, "none",
		"", "user",
	)

	list := tm.List()
	require.Len(t, list, 2)

	// 找到 cipherMode=1 的任务
	var taskWithZstd *MobileTask
	for _, t := range list {
		if t.CipherMode == 1 {
			taskWithZstd = t
			break
		}
	}
	require.NotNil(t, taskWithZstd, "should find task with cipherMode=1")
	assert.Equal(t, "zstd", taskWithZstd.CompressionMode, "task with cipherMode=1 should have compressionMode='zstd'")
}

func TestCreateWithRunMeta_CompatWithCreateWithExtras(t *testing.T) {
	mb := new(MockBroadcaster)
	mb.On("Broadcast", "task:created", mock.Anything).Return()
	tm := newTestTaskManager(mb)

	// 旧调用方走 CreateWithExtras → cipherMode/compressionMode 应为零值
	taskViaExtras := tm.CreateWithExtras("encrypt", "/test/file.mp4", "", "pw", "", 3, "", nil)

	assert.Equal(t, 0, taskViaExtras.CipherMode, "CreateWithExtras should leave CipherMode as zero value")
	assert.Equal(t, "", taskViaExtras.CompressionMode, "CreateWithExtras should leave CompressionMode as empty string")
}

// 🆕 2026-06-22：runId 为空时后端兜底派生 "manual-${taskID}"
// 场景：移动端 Capacitor 偶发丢 runId 参数（前端 createTask 调用时漏传）
//   → 后端不能用空 runId 持久化（前端按 runId 分组时 task 变孤儿）
//   → 用 "manual-${id}" 派生稳定 runId → 前端按 runId 分组永远有归属
func TestCreateWithRunMeta_RunIdEmptyFallback(t *testing.T) {
	mb := new(MockBroadcaster)
	mb.On("Broadcast", "task:created", mock.Anything).Return()
	tm := newTestTaskManager(mb)

	task := tm.CreateWithRunMeta(
		"encrypt", "/test/file.mp4", "", "pw", "",
		4, "", nil,
		0, "none",
		"", // 🆕 runId 为空 → 兜底
		"", // triggeredBy 为空 → 兜底 'user'
	)

	require.NotNil(t, task)
	assert.NotEmpty(t, task.RunId, "runId 为空时后端必须兜底派生")
	assert.True(t, strings.HasPrefix(task.RunId, "manual-"), "兜底 runId 必须以 'manual-' 开头，实际: %s", task.RunId)
	assert.Equal(t, task.ID, strings.TrimPrefix(task.RunId, "manual-"), "兜底 runId 必须是 'manual-' + task.ID")
	assert.Equal(t, "user", task.TriggeredBy, "triggeredBy 为空时后端必须兜底 'user'")
}

// 🆕 2026-06-23 真实架构实现：CreateBatch 批量创建 task
//
// 架构原则（替代 client 预占位野路子）：
//   - 后端是 task ID 的唯一权威源（uuid.New()），前端不传 ID
//   - 批量创建后一次性返回所有 task，前端一次性 push 到 store
//   - 所有 task 共享同一个 runId + triggeredBy
//
// 覆盖：
//   - 批量创建 N 个 task → 返回 N 个 task，每个有后端生成的 UUID
//   - 所有 task 共享同一个 runId
//   - 空 specs → 返回 nil
//   - runId 为空 → 后端兜底派生 "manual-batch-xxx"
func TestCreateBatch_Basic(t *testing.T) {
	mb := new(MockBroadcaster)
	mb.On("Broadcast", "task:created", mock.Anything).Return()
	tm := newTestTaskManager(mb)

	specs := []BatchTaskSpec{
		{Type: "encrypt", SourcePath: "/test/file1.mp4", PluginName: "video", Version: 4, CipherMode: 1, CompressionMode: "zstd"},
		{Type: "encrypt", SourcePath: "/test/file2.mp4", PluginName: "video", Version: 4, CipherMode: 0, CompressionMode: "none"},
		{Type: "encrypt", SourcePath: "/test/file3.mp4", PluginName: "audio", Version: 4, CipherMode: 1, CompressionMode: "none"},
	}

	tasks := tm.CreateBatch(specs, "run-batch-001", "automation")

	require.Len(t, tasks, 3, "批量创建应返回 3 个 task")
	// 1. 每个 task 都有后端生成的 UUID（非空）
	for i, task := range tasks {
		assert.NotEmpty(t, task.ID, "task[%d] 必须有后端生成的 ID", i)
		assert.Equal(t, "run-batch-001", task.RunId, "所有 task 必须共享同一个 runId")
		assert.Equal(t, "automation", task.TriggeredBy, "所有 task 必须共享同一个 triggeredBy")
	}
	// 2. 每个 task 的 ID 互不相同（后端生成 UUID）
	assert.NotEqual(t, tasks[0].ID, tasks[1].ID, "task ID 必须互不相同")
	assert.NotEqual(t, tasks[1].ID, tasks[2].ID, "task ID 必须互不相同")
	// 3. crypto 字段正确持久化
	assert.Equal(t, 1, tasks[0].CipherMode, "task[0] cipherMode=1")
	assert.Equal(t, "zstd", tasks[0].CompressionMode, "task[0] compressionMode='zstd'")
	assert.Equal(t, 0, tasks[1].CipherMode, "task[1] cipherMode=0")
	assert.Equal(t, "none", tasks[1].CompressionMode, "task[1] compressionMode='none'")
	// 4. List() 能查到所有 task
	list := tm.List()
	assert.Len(t, list, 3, "List() 必须返回所有批量创建的 task")
}

func TestCreateBatch_EmptySpecs(t *testing.T) {
	mb := new(MockBroadcaster)
	mb.On("Broadcast", "task:created", mock.Anything).Return()
	tm := newTestTaskManager(mb)

	tasks := tm.CreateBatch(nil, "run-xxx", "automation")
	assert.Nil(t, tasks, "空 specs 应返回 nil")

	tasks = tm.CreateBatch([]BatchTaskSpec{}, "run-xxx", "automation")
	assert.Nil(t, tasks, "空 specs 应返回 nil")
}

func TestCreateBatch_RunIdEmptyFallback(t *testing.T) {
	mb := new(MockBroadcaster)
	mb.On("Broadcast", "task:created", mock.Anything).Return()
	tm := newTestTaskManager(mb)

	specs := []BatchTaskSpec{
		{Type: "encrypt", SourcePath: "/test/file1.mp4", PluginName: "video", Version: 4},
	}

	tasks := tm.CreateBatch(specs, "", "")

	require.Len(t, tasks, 1)
	assert.NotEmpty(t, tasks[0].RunId, "runId 为空时后端必须兜底派生")
	assert.True(t, strings.HasPrefix(tasks[0].RunId, "manual-batch-"), "兜底 runId 必须以 'manual-batch-' 开头，实际: %s", tasks[0].RunId)
	assert.Equal(t, "user", tasks[0].TriggeredBy, "triggeredBy 为空时后端必须兜底 'user'")
}

// 🆕 2026-06-23 WS 时序修复（spec ws-timing-batch-throughput-100k Task 1.7）
//
// Create() 不再内部广播 task:created（避免 admin_handlers / mobile_api / rollback_manager
// 等直接调 Create 的路径在 runId 未设置时就广播 → 前端收到无 runId 的 task 变孤儿）。
// 广播职责改由 CreateWithRunMeta（设置 runId 之后）或外部调用方（FinalizeCreatedTask）负责。
func TestCreate_DoesNotBroadcast(t *testing.T) {
	mb := new(MockBroadcaster)
	// 不注册 mb.On("Broadcast", ...) —— 如果 Create 误调 Broadcast，testify mock 会 fail
	tm := newTestTaskManager(mb)

	task := tm.Create("encrypt", "/test/file.mp4", "", "pw", 0, "")

	require.NotNil(t, task)
	assert.NotEmpty(t, task.ID, "Create 仍应返回有效 task")
	assert.Equal(t, "queued", task.Status)

	// Create 不应触发任何 task:created 广播
	calls := mb.FindCalls("task:created")
	assert.Empty(t, calls, "Create() 不应广播 task:created（WS 时序修复后改由 CreateWithRunMeta / FinalizeCreatedTask 负责）")
}

// 🆕 2026-06-23 WS 时序修复（spec ws-timing-batch-throughput-100k Task 1.7）
//
// CreateWithRunMeta() 在 saveTaskSingle 之后广播 task:created，且 payload 的 runId 非空。
// 这保证前端按 runId 分组时不会产生孤儿 group。
func TestCreateWithRunMeta_BroadcastsWithRunId(t *testing.T) {
	mb := new(MockBroadcaster)
	mb.On("Broadcast", "task:created", mock.Anything).Return()
	tm := newTestTaskManager(mb)

	task := tm.CreateWithRunMeta(
		"encrypt", "/test/file.mp4", "", "pw", "",
		4, "", nil,
		1, "zstd",
		"run-explicit-123", "automation",
	)

	require.NotNil(t, task)
	assert.Equal(t, "run-explicit-123", task.RunId, "显式 runId 应被保留")

	// CreateWithRunMeta 应广播 1 次 task:created
	calls := mb.FindCalls("task:created")
	require.Len(t, calls, 1, "CreateWithRunMeta 应广播 1 次 task:created")
	assert.Equal(t, "task:created", calls[0].MsgType)

	// 广播 payload 必须是 *MobileTask 且 runId 非空
	broadcastTask, ok := calls[0].Data.(*MobileTask)
	require.True(t, ok, "广播 payload 应是 *MobileTask")
	assert.NotEmpty(t, broadcastTask.RunId, "广播的 task payload runId 必须非空（根治孤儿 group）")
	assert.Equal(t, "run-explicit-123", broadcastTask.RunId, "广播的 runId 应与 task.RunId 一致")
	assert.Equal(t, task.ID, broadcastTask.ID, "广播的 task ID 应与返回的 task 一致")
}

// 🆕 2026-06-23 WS 时序修复：CreateWithRunMeta runId 为空兜底时广播的 payload runId 也非空
func TestCreateWithRunMeta_BroadcastsWithRunId_EmptyFallback(t *testing.T) {
	mb := new(MockBroadcaster)
	mb.On("Broadcast", "task:created", mock.Anything).Return()
	tm := newTestTaskManager(mb)

	task := tm.CreateWithRunMeta(
		"encrypt", "/test/file.mp4", "", "pw", "",
		4, "", nil,
		0, "none",
		"", "", // runId + triggeredBy 都为空 → 后端兜底
	)

	require.NotNil(t, task)
	assert.True(t, strings.HasPrefix(task.RunId, "manual-"), "兜底 runId 必须以 'manual-' 开头")

	calls := mb.FindCalls("task:created")
	require.Len(t, calls, 1, "CreateWithRunMeta 应广播 1 次 task:created")
	broadcastTask, ok := calls[0].Data.(*MobileTask)
	require.True(t, ok)
	assert.NotEmpty(t, broadcastTask.RunId, "兜底场景下广播的 payload runId 也必须非空")
}

// 🆕 2026-06-23 Task 5：后端分页 API 的核心逻辑测试
//
// 覆盖 ListPaginated(runId, offset, limit)：
//   - runId 过滤：只返回 task.RunId == runId 的 task
//   - offset/limit 分页：offset=2&limit=3 返回 3 个 task
//   - totalCount = 过滤后、分页前的总数
//   - 空 runId → 不过滤（返回全部）
//   - offset 超出范围 → 返回空切片但 totalCount 仍正确
//
// 注：List() 基于 map 迭代，顺序非确定，因此测试只校验数量 + runId 归属，
// 不校验"具体第几个 task"（map 迭代顺序不稳定）。
func TestList_Pagination(t *testing.T) {
	mb := new(MockBroadcaster)
	mb.On("Broadcast", "task:created", mock.Anything).Return()
	tm := newTestTaskManager(mb)

	// 创建 10 个 task（同一 runId）
	runId := "run-pagination-test"
	for i := 0; i < 10; i++ {
		tm.CreateWithRunMeta(
			"encrypt", fmt.Sprintf("/test/file%d.mp4", i), "", "pw", "",
			4, "", nil,
			0, "none",
			runId, "automation",
		)
	}
	// 再创建 2 个不同 runId 的 task（验证 runId 过滤）
	tm.CreateWithRunMeta("encrypt", "/test/other1.mp4", "", "pw", "", 4, "", nil, 0, "none", "run-other-1", "automation")
	tm.CreateWithRunMeta("encrypt", "/test/other2.mp4", "", "pw", "", 4, "", nil, 0, "none", "run-other-2", "automation")

	// 1. offset=2&limit=3 → 返回 3 个 task，totalCount=10（过滤后）
	tasks, totalCount := tm.ListPaginated(runId, 2, 3)
	assert.Equal(t, 10, totalCount, "totalCount 应为 10（runId 过滤后）")
	require.Len(t, tasks, 3, "offset=2&limit=3 应返回 3 个 task")
	for _, task := range tasks {
		assert.Equal(t, runId, task.RunId, "所有返回的 task 必须属于 runId=%s", runId)
	}

	// 2. offset=0&limit=100 → 返回全部 10 个（runId 过滤后）
	tasks, totalCount = tm.ListPaginated(runId, 0, 100)
	assert.Equal(t, 10, totalCount)
	require.Len(t, tasks, 10, "offset=0&limit=100 应返回全部 10 个 task")
	for _, task := range tasks {
		assert.Equal(t, runId, task.RunId)
	}

	// 3. 空 runId → 不过滤，返回全部 12 个
	tasks, totalCount = tm.ListPaginated("", 0, 100)
	assert.Equal(t, 12, totalCount, "空 runId 应返回全部 12 个 task 的总数")
	require.Len(t, tasks, 12, "空 runId 应返回全部 12 个 task")

	// 4. offset 超出范围 → 返回空切片，totalCount 仍为 10
	tasks, totalCount = tm.ListPaginated(runId, 100, 10)
	assert.Equal(t, 10, totalCount, "offset 超出范围时 totalCount 仍应为 10")
	assert.Empty(t, tasks, "offset 超出范围应返回空切片")

	// 5. limit=0 → 返回空切片，totalCount 仍为 10
	tasks, totalCount = tm.ListPaginated(runId, 0, 0)
	assert.Equal(t, 10, totalCount)
	assert.Empty(t, tasks, "limit=0 应返回空切片")

	// 6. 不存在的 runId → totalCount=0，空切片
	tasks, totalCount = tm.ListPaginated("run-not-exist", 0, 100)
	assert.Equal(t, 0, totalCount)
	assert.Empty(t, tasks)
}

// 🆕 2026-06-23 spec ws-timing-batch-throughput-100k Task 2.4
//
// CancelByRunId 批量取消指定 runId 下所有非终态 task。
//
// 覆盖：
//   - run-test-1：3 个 queued + 2 个 success → 只取消 3 个 queued，2 个 success 不变
//   - run-test-2：2 个 queued → 完全不变（不同 runId 不受影响）
//   - 广播 task:update 恰好 3 次（只对取消的 task）
func TestCancelByRunId(t *testing.T) {
	mb := new(MockBroadcaster)
	mb.On("Broadcast", mock.Anything, mock.Anything).Return()
	tm := newTestTaskManager(mb)

	// run-test-1：3 个 queued
	var run1Queued []*MobileTask
	for i := 0; i < 3; i++ {
		task := tm.CreateWithRunMeta(
			"encrypt", fmt.Sprintf("/test/run1-queued-%d.mp4", i), "", "pw", "",
			4, "", nil, 0, "none",
			"run-test-1", "automation",
		)
		run1Queued = append(run1Queued, task)
	}

	// run-test-1：2 个 success（终态，不应被取消）
	var run1Success []*MobileTask
	for i := 0; i < 2; i++ {
		task := tm.CreateWithRunMeta(
			"encrypt", fmt.Sprintf("/test/run1-success-%d.mp4", i), "", "pw", "",
			4, "", nil, 0, "none",
			"run-test-1", "automation",
		)
		// 手动标记为终态 success
		tm.mu.Lock()
		task.Status = "success"
		now := time.Now()
		task.CompletedAt = &now
		tm.mu.Unlock()
		run1Success = append(run1Success, task)
	}

	// run-test-2：2 个 queued（不同 runId，不应受影响）
	var run2Queued []*MobileTask
	for i := 0; i < 2; i++ {
		task := tm.CreateWithRunMeta(
			"encrypt", fmt.Sprintf("/test/run2-queued-%d.mp4", i), "", "pw", "",
			4, "", nil, 0, "none",
			"run-test-2", "automation",
		)
		run2Queued = append(run2Queued, task)
	}

	// 清空 mock calls，只观察 CancelByRunId 的广播
	mb.calls_ = nil

	// 调 CancelByRunId("run-test-1")
	err := tm.CancelByRunId("run-test-1")
	require.NoError(t, err)

	// 1. 验证 run-test-1 的 3 个 queued task 变成 cancelled
	for _, task := range run1Queued {
		updated, err := tm.Get(task.ID)
		require.NoError(t, err)
		assert.Equal(t, "cancelled", updated.Status,
			"run-test-1 的 queued task 应变成 cancelled, id=%s", task.ID)
	}

	// 2. 验证 run-test-1 的 2 个 success task 不变
	for _, task := range run1Success {
		updated, err := tm.Get(task.ID)
		require.NoError(t, err)
		assert.Equal(t, "success", updated.Status,
			"run-test-1 的 success task 应保持不变, id=%s", task.ID)
	}

	// 3. 验证 run-test-2 的 2 个 task 不变（仍为 queued）
	for _, task := range run2Queued {
		updated, err := tm.Get(task.ID)
		require.NoError(t, err)
		assert.Equal(t, "queued", updated.Status,
			"run-test-2 的 task 不应被取消, id=%s", task.ID)
	}

	// 4. 验证广播：应恰好 3 次 task:update（只对 3 个非终态 task）
	calls := mb.FindCalls("task:update")
	assert.Len(t, calls, 3, "应广播 3 次 task:update（只取消 3 个非终态 task）")
	for _, call := range calls {
		data, ok := call.Data.(map[string]interface{})
		require.True(t, ok, "广播 payload 应是 map[string]interface{}")
		assert.Equal(t, "cancelled", data["status"], "广播的 status 应为 cancelled")
	}
}

// 🆕 2026-06-23 Task 2.4：CancelByRunId 空 runId 应返回 error
func TestCancelByRunId_EmptyRunId(t *testing.T) {
	mb := new(MockBroadcaster)
	tm := newTestTaskManager(mb)

	err := tm.CancelByRunId("")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "runId is required")
}

// 🆕 2026-06-23 Task 2.4：CancelByRunId 不存在的 runId 应成功（无 task 可取消，返回 nil）
func TestCancelByRunId_NonExistentRunId(t *testing.T) {
	mb := new(MockBroadcaster)
	mb.On("Broadcast", mock.Anything, mock.Anything).Return()
	tm := newTestTaskManager(mb)

	err := tm.CancelByRunId("run-not-exist")
	require.NoError(t, err, "不存在的 runId 应返回 nil（无 task 可取消，不算错误）")
}
