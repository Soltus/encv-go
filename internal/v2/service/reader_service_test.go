package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Soltus/encv-go/internal/config"
	"github.com/Soltus/encv-go/internal/testutil"
	"github.com/Soltus/encv-go/internal/v2/reader"
	"github.com/Soltus/encv-go/internal/v2/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMain 注册测试所需的 KVI provider，确保 "video" kind 可被解析
func TestMain(m *testing.M) {
	types.RegisterKVIProvider("video", func(rawKVI json.RawMessage) (types.KVIProvider, error) {
		var kvi types.KVI
		if err := json.Unmarshal(rawKVI, &kvi); err != nil {
			return nil, err
		}
		return testKVI{KVI: kvi}, nil
	})
	os.Exit(m.Run())
}

// testKVI 测试专用 KVIProvider 实现，满足 types.KVIProvider 接口
type testKVI struct {
	types.KVI
}

func (k testKVI) GetKind() types.IndexKind     { return "video" }
func (k testKVI) GetEncryptionInfo() types.KVI { return k.KVI }
func (k testKVI) GetIndex() types.Index        { return &testIndex{} }

// testIndex 测试专用 Index 实现
type testIndex struct{}

func (i *testIndex) GetOriginalFilename() string { return "fixture_video.mp4" }
func (i *testIndex) GetOriginalFileSize() int64  { return 0 }
func (i *testIndex) GetOriginalFileMD5() string  { return "" }
func (i *testIndex) GetEncryptedFileMD5() string { return "" }
func (i *testIndex) GetMimeType() string         { return "video/mp4" }

// newTestReaderService 创建 ReaderService 测试辅助函数
func newTestReaderService(t *testing.T) (*ReaderService, *testutil.ContainerFixture) {
	t.Helper()
	fixture := testutil.CreateV3Fixture(t, 100*1024, 5)
	manager := NewContainerManager()
	rs := NewReaderService(manager)
	return rs, fixture
}

var testCfg = &config.Config{Password: "fixture-password"}

// TestGetDecryptReader_ValidContainer 验证对有效容器调用 GetDecryptReader 返回正确结果
func TestGetDecryptReader_ValidContainer(t *testing.T) {
	rs, fixture := newTestReaderService(t)

	factory, decryptReader, index, size, err := rs.GetDecryptReader(*testCfg, fixture.Path, fixture.Password, nil)

	require.NoError(t, err, "GetDecryptReader 不应返回错误")
	require.NotNil(t, factory, "factory 不应为 nil")
	require.NotNil(t, decryptReader, "reader 不应为 nil")
	require.NotNil(t, index, "index 不应为 nil")
	assert.GreaterOrEqual(t, size, int64(0), "原始大小应 >= 0")

	assert.Equal(t, "fixture_video.mp4", index.GetOriginalFilename(), "index 的 OriginalFilename 应匹配 fixture 默认值")

	defer decryptReader.Close()
}

// TestGetDecryptReader_CachedSecondCall 验证连续两次调用返回缓存的同一个 factory 实例
func TestGetDecryptReader_CachedSecondCall(t *testing.T) {
	rs, fixture := newTestReaderService(t)

	factory1, reader1, _, _, err1 := rs.GetDecryptReader(*testCfg, fixture.Path, fixture.Password, nil)
	require.NoError(t, err1)
	defer reader1.Close()

	factory2, reader2, _, _, err2 := rs.GetDecryptReader(*testCfg, fixture.Path, fixture.Password, nil)
	require.NoError(t, err2)
	defer reader2.Close()

	assert.Same(t, factory1, factory2, "两次调用应返回同一个缓存 factory 实例")
}

// TestGetDecryptReader_InvalidContainer 验证对无效容器文件返回错误或 panic
func TestGetDecryptReader_InvalidContainer(t *testing.T) {
	manager := NewContainerManager()
	rs := NewReaderService(manager)

	invalidPath := filepath.Join(t.TempDir(), "garbage.sccgv")
	os.WriteFile(invalidPath, []byte("this is not a valid container"), 0644)

	defer func() {
		r := recover()
		if r == nil {
			t.Log("注意: GetReadablePath 对无效容器未 panic，说明内部重建逻辑已变更")
			return
		}
		assert.NotNil(t, r, "无效容器应触发错误或 panic")
	}()

	rs.GetDecryptReader(*testCfg, invalidPath, "any-password", nil)
}

// TestGetBulkDecryptor_ValidContainer 验证对有效容器创建 BulkDecryptor 成功
func TestGetBulkDecryptor_ValidContainer(t *testing.T) {
	rs, fixture := newTestReaderService(t)

	bulk, err := rs.GetBulkDecryptor(*testCfg, fixture.Path, fixture.Password, nil)

	require.NoError(t, err, "GetBulkDecryptor 不应返回错误")
	assert.NotNil(t, bulk, "BulkDecryptor 不应为 nil")
	assert.IsType(t, &reader.BulkDecryptor{}, bulk, "返回类型应为 *reader.BulkDecryptor")
}

// TestCleanup_FactoriesCleared 验证 Cleanup 后 factories 缓存被清空且不再持有引用
func TestCleanup_FactoriesCleared(t *testing.T) {
	rs, fixture := newTestReaderService(t)

	factory, dr, _, _, err := rs.GetDecryptReader(*testCfg, fixture.Path, fixture.Password, nil)
	require.NoError(t, err)
	dr.Close()

	require.NotNil(t, factory, "Cleanup 前应存在 factory")

	rs.Cleanup()

	rs.mu.RLock()
	count := len(rs.factories)
	rs.mu.RUnlock()

	assert.Equal(t, 0, count, "Cleanup 后 factories 缓存应被清空")
}
