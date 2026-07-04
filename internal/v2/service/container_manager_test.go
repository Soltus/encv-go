package service

import (
	"testing"

	"github.com/Soltus/encv-go/internal/testutil"
	"github.com/stretchr/testify/assert"

	// 强制激活 test-guard：拦截裸 go test 调用
	_ "github.com/Soltus/encv-go/internal/testguard"
)

// TestGetReadablePath_ValidContainer_ReturnsOriginal 验证有效容器直接返回原始路径，不触发重建
func TestGetReadablePath_ValidContainer_ReturnsOriginal(t *testing.T) {
	cm := NewContainerManager()

	fixture := testutil.CreateV3Fixture(t, 1024, 4)

	path, err := cm.GetReadablePath(fixture.Path, nil)

	assert.NoError(t, err)
	assert.Equal(t, fixture.Path, path, "有效容器应返回原始路径")

	assert.Empty(t, cm.cache, "有效容器不应填充缓存")
}

// TestGetReadablePath_InvalidContainer_NilChunkNamer_Panics 验证损坏容器在无 chunkNamer 时会 panic
// 这是当前实现的已知行为：physical unpacker 内部直接调用 namer.ParseFirstChunkName() 而未做 nil 检查
func TestGetReadablePath_InvalidContainer_NilChunkNamer_Panics(t *testing.T) {
	cm := NewContainerManager()

	original := testutil.CreateV3Fixture(t, 1024, 4)
	corrupted := testutil.CreateCorruptedFixture(t, original)

	assert.Panics(t, func() {
		cm.GetReadablePath(corrupted.Path, nil)
	}, "损坏容器且 chunkNamer=nil 时应 panic（namer 未做 nil 保护）")
}

// TestGetReadablePath_CacheHit_InvalidContainerReturnsCached 验证对无效容器注入缓存后能命中
// 注意：由于 chunkNamer=nil 时重建会 panic，此测试手动注入缓存来验证缓存读取逻辑
func TestGetReadablePath_CacheHit_InvalidContainerReturnsCached(t *testing.T) {
	cm := NewContainerManager()

	fixture := testutil.CreateV3Fixture(t, 1024, 4)
	corrupted := testutil.CreateCorruptedFixture(t, fixture)

	cachedPath := "/tmp/cached_rebuilt_container.sccgv"
	cm.cache[corrupted.Path] = &CachedContainer{
		UnifiedPath: cachedPath,
		CleanupFunc: func() {},
	}

	path, err := cm.GetReadablePath(corrupted.Path, nil)

	assert.NoError(t, err, "缓存命中时不应返回错误")
	assert.Equal(t, cachedPath, path, "应返回缓存中的重建路径")
}

// TestCleanup_ClearsCache 验证 Cleanup 清空缓存状态
func TestCleanup_ClearsCache(t *testing.T) {
	cm := NewContainerManager()

	fixture := testutil.CreateV3Fixture(t, 1024, 4)

	cm.cache[fixture.Path] = &CachedContainer{
		UnifiedPath: "/tmp/test_cleanup.sccgv",
		CleanupFunc: func() {},
	}

	assert.Len(t, cm.cache, 1, "填充后缓存应有 1 项")

	cm.Cleanup()

	assert.Empty(t, cm.cache, "Cleanup 后缓存应清空")
}

// TestGetReadablePath_NilChunkNamer_ValidContainer_NoPanic 验证有效容器在 chunkNamer=nil 时不会 panic
func TestGetReadablePath_NilChunkNamer_ValidContainer_NoPanic(t *testing.T) {
	cm := NewContainerManager()

	fixture := testutil.CreateV3Fixture(t, 1024, 4)

	assert.NotPanics(t, func() {
		path, err := cm.GetReadablePath(fixture.Path, nil)
		assert.NoError(t, err)
		assert.Equal(t, fixture.Path, path)
	}, "有效容器 + chunkNamer=nil 不应触发重建，不会 panic")
}

// TestGetReadablePath_MultiplePaths_IndependentCache 验证不同路径的缓存相互独立
func TestGetReadablePath_MultiplePaths_IndependentCache(t *testing.T) {
	cm := NewContainerManager()

	fixture1 := testutil.CreateV3Fixture(t, 512, 2)
	fixture2 := testutil.CreateV3Fixture(t, 512, 3)

	path1, err1 := cm.GetReadablePath(fixture1.Path, nil)
	path2, err2 := cm.GetReadablePath(fixture2.Path, nil)

	assert.NoError(t, err1)
	assert.NoError(t, err2)
	assert.Equal(t, fixture1.Path, path1)
	assert.Equal(t, fixture2.Path, path2)
	assert.NotEqual(t, path1, path2, "不同容器的路径应不同")
}

// TestNewContainerManager_InitialState 验证新创建的管理器初始状态正确
func TestNewContainerManager_InitialState(t *testing.T) {
	cm := NewContainerManager()

	assert.NotNil(t, cm)
	assert.NotNil(t, cm.cache)
	assert.Empty(t, cm.cache, "新管理器的缓存应为空")
}

// TestGetReadablePath_SameValidPath_Idempotent 验证对同一有效容器多次调用结果一致
func TestGetReadablePath_SameValidPath_Idempotent(t *testing.T) {
	cm := NewContainerManager()

	fixture := testutil.CreateV3Fixture(t, 2048, 6)

	for i := 0; i < 5; i++ {
		path, err := cm.GetReadablePath(fixture.Path, nil)
		assert.NoError(t, err, "第 %d 次调用不应出错", i+1)
		assert.Equal(t, fixture.Path, path, "第 %d 次调用应返回原始路径", i+1)
	}

	assert.Empty(t, cm.cache, "多次调用有效容器后缓存仍为空")
}
