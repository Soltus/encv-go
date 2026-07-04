package service

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/Soltus/encv-go/internal/config"
	"github.com/Soltus/encv-go/pkg/tasksystem/store/sqlite"
	"github.com/Soltus/encv-go/pkg/tasksystem/store/tursogo"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// BenchmarkCreateBatch 对比不同存储引擎下 CreateBatch 的性能
//
// 测试场景：
//   - SQLite 引擎（modernc.org/sqlite，单写者）
//   - Turso 引擎（tursogo，MVCC 并发写）
//
// 每组测试：
//   - 批量创建 100 / 500 / 1000 个任务
//   - 记录总耗时和单任务平均耗时

func BenchmarkCreateBatch_SQLite_100(b *testing.B) {
	tm, cleanup := newBenchTaskManagerSQLite(b)
	defer cleanup()
	specs := genBatchSpecs(100)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tm.CreateBatch(specs, fmt.Sprintf("run-sqlite-100-%d", i), "automation")
	}
}

func BenchmarkCreateBatch_SQLite_500(b *testing.B) {
	tm, cleanup := newBenchTaskManagerSQLite(b)
	defer cleanup()
	specs := genBatchSpecs(500)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tm.CreateBatch(specs, fmt.Sprintf("run-sqlite-500-%d", i), "automation")
	}
}

func BenchmarkCreateBatch_SQLite_1000(b *testing.B) {
	tm, cleanup := newBenchTaskManagerSQLite(b)
	defer cleanup()
	specs := genBatchSpecs(1000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tm.CreateBatch(specs, fmt.Sprintf("run-sqlite-1000-%d", i), "automation")
	}
}

func BenchmarkCreateBatch_Turso_100(b *testing.B) {
	tm, cleanup := newBenchTaskManagerTurso(b)
	defer cleanup()
	specs := genBatchSpecs(100)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tm.CreateBatch(specs, fmt.Sprintf("run-turso-100-%d", i), "automation")
	}
}

func BenchmarkCreateBatch_Turso_500(b *testing.B) {
	tm, cleanup := newBenchTaskManagerTurso(b)
	defer cleanup()
	specs := genBatchSpecs(500)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tm.CreateBatch(specs, fmt.Sprintf("run-turso-500-%d", i), "automation")
	}
}

func BenchmarkCreateBatch_Turso_1000(b *testing.B) {
	tm, cleanup := newBenchTaskManagerTurso(b)
	defer cleanup()
	specs := genBatchSpecs(1000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tm.CreateBatch(specs, fmt.Sprintf("run-turso-1000-%d", i), "automation")
	}
}

// --- helpers ---

func newBenchTaskManagerSQLite(b *testing.B) (*TaskManager, func()) {
	b.Helper()
	tmpDir := b.TempDir()
	dbPath := filepath.Join(tmpDir, "bench.db")
	store, err := sqlite.New(dbPath)
	require.NoError(b, err)

	cfg := &config.Config{Password: "bench-pass"}
	mb := new(MockBroadcaster)
	mb.On("Broadcast", mock.Anything, mock.Anything).Return()

	servingDir := b.TempDir()
	tm := NewTaskManagerWithStore(servingDir, cfg, mb, store)

	cleanup := func() {
		tm.Stop()
		store.Close()
	}
	return tm, cleanup
}

func newBenchTaskManagerTurso(b *testing.B) (*TaskManager, func()) {
	b.Helper()
	tmpDir := b.TempDir()
	dbPath := filepath.Join(tmpDir, "bench.db")
	store, err := tursogo.NewLocal(dbPath)
	require.NoError(b, err)

	cfg := &config.Config{Password: "bench-pass"}
	mb := new(MockBroadcaster)
	mb.On("Broadcast", mock.Anything, mock.Anything).Return()

	servingDir := b.TempDir()
	tm := NewTaskManagerWithStore(servingDir, cfg, mb, store)

	cleanup := func() {
		tm.Stop()
		store.Close()
	}
	return tm, cleanup
}

func genBatchSpecs(n int) []BatchTaskSpec {
	specs := make([]BatchTaskSpec, n)
	for i := 0; i < n; i++ {
		specs[i] = BatchTaskSpec{
			Type:       "encrypt",
			SourcePath: fmt.Sprintf("/data/source-%d.mp4", i),
			TargetPath: fmt.Sprintf("/data/target-%d.encv", i),
			Password:   "test-password",
			PluginName: "ffmpeg-encryption",
			ExtraFields: map[string]string{
				"cipherMode":      "aes-256-gcm",
				"compressionMode": "zstd",
			},
			CipherMode:        0,
			CompressionMode:   "zstd",
			Version:           4,
		}
	}
	return specs
}
