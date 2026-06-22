package service

import (
	"strings"
	"testing"

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
