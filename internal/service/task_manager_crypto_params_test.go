package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// 🆕 2026-06-18 Task 16：CreateWithCryptoParams 持久化 cipherMode / compressionMode
//
// 覆盖：
// - 显式传 cipherMode=1 + compressionMode='zstd' → 字段持久化
// - cipherMode=0 + compressionMode='none'（默认值）→ 字段仍持久化（用户主动选了默认）
// - 兼容 CreateWithExtras（不传 crypto 参数时 CipherMode=0, CompressionMode=""）
// - List() 返回的任务包含 cipherMode / compressionMode 字段
func TestCreateWithCryptoParams_PreservesCryptoFields(t *testing.T) {
	mb := new(MockBroadcaster)
	mb.On("Broadcast", "task:created", mock.Anything).Return()
	tm := newTestTaskManager(mb)

	extras := map[string]string{"plugin_password": "test123"}
	task := tm.CreateWithCryptoParams(
		"encrypt", "/test/file.mp4", "", "pw", "secondary-pw",
		4, "mp4-plugin", extras,
		1, "zstd",
	)

	require.NotNil(t, task)
	assert.Equal(t, 1, task.CipherMode, "cipherMode should be persisted as 1 (AES-256-GCM)")
	assert.Equal(t, "zstd", task.CompressionMode, "compressionMode should be persisted as 'zstd'")
	assert.Equal(t, "test123", task.ExtraFields["plugin_password"], "extras should still be preserved")
	assert.Equal(t, "secondary-pw", task.SecondaryPassword, "secondary password should be preserved")
	assert.Equal(t, 4, task.ContainerVersion, "version should be preserved")
	assert.Equal(t, "mp4-plugin", task.PluginName, "pluginName should be preserved")
}

func TestCreateWithCryptoParams_DefaultValuesPersisted(t *testing.T) {
	mb := new(MockBroadcaster)
	mb.On("Broadcast", "task:created", mock.Anything).Return()
	tm := newTestTaskManager(mb)

	// 用户主动选了默认值（cipherMode=0=AES-128, compressionMode='none'）
	// 后端仍应持久化（前端回显时需要知道用户选了什么）
	task := tm.CreateWithCryptoParams(
		"encrypt", "/test/file.mp4", "", "pw", "",
		4, "", nil,
		0, "none",
	)

	require.NotNil(t, task)
	assert.Equal(t, 0, task.CipherMode, "cipherMode=0 should be persisted (user explicitly chose default)")
	assert.Equal(t, "none", task.CompressionMode, "compressionMode='none' should be persisted")
}

func TestCreateWithCryptoParams_ListReturnsCryptoFields(t *testing.T) {
	mb := new(MockBroadcaster)
	mb.On("Broadcast", "task:created", mock.Anything).Return()
	tm := newTestTaskManager(mb)

	tm.CreateWithCryptoParams(
		"encrypt", "/test/file1.mp4", "", "pw", "",
		4, "", nil,
		1, "zstd",
	)
	tm.CreateWithCryptoParams(
		"encrypt", "/test/file2.mp4", "", "pw", "",
		4, "", nil,
		0, "none",
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

func TestCreateWithCryptoParams_CompatWithCreateWithExtras(t *testing.T) {
	mb := new(MockBroadcaster)
	mb.On("Broadcast", "task:created", mock.Anything).Return()
	tm := newTestTaskManager(mb)

	// 旧调用方走 CreateWithExtras → cipherMode/compressionMode 应为零值
	taskViaExtras := tm.CreateWithExtras("encrypt", "/test/file.mp4", "", "pw", "", 3, "", nil)

	assert.Equal(t, 0, taskViaExtras.CipherMode, "CreateWithExtras should leave CipherMode as zero value")
	assert.Equal(t, "", taskViaExtras.CompressionMode, "CreateWithExtras should leave CompressionMode as empty string")
}
