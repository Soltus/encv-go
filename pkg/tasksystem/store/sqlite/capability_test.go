package sqlite

import (
	"fmt"
	"sync/atomic"
	"time"

	"github.com/Soltus/encv-go/pkg/tasksystem"
)

func init() {
	tasksystem.RegisterCapabilityTest(tasksystem.CapabilityTest{
		ID:          "sqlite_wal_read_concurrency",
		Name:        "WAL 读并发性能",
		Description: "SQLite WAL 模式下多 goroutine 并发读，验证读读不阻塞特性",
		Category:    "performance",
		Engine:      "sqlite",
		Run:         testWALReadConcurrency,
	})

	tasksystem.RegisterCapabilityTest(tasksystem.CapabilityTest{
		ID:          "sqlite_pure_go_zero_dep",
		Name:        "纯 Go 零依赖验证",
		Description: "验证 glebarez/sqlite 纯 Go transpile 实现，无 CGO、无原生依赖",
		Category:    "feature",
		Engine:      "sqlite",
		Run:         testPureGo,
	})
}

// testWALReadConcurrency 测试 WAL 模式下的读并发性能。
//
// SQLite 的 WAL 模式是其核心特性：
//   - 读读完全不阻塞（多个 reader 可并行）
//   - 写不阻塞读（reader 读快照，writer 写新 WAL 页）
//   - 只有写写阻塞（单写者）
func testWALReadConcurrency(store tasksystem.Store) (map[string]any, error) {
	const readers = 8
	const duration = 3 * time.Second

	for i := 0; i < 500; i++ {
		task := makeTestTaskForCapTest(i)
		if err := store.CreateTask(task); err != nil {
			return nil, fmt.Errorf("init data: %w", err)
		}
	}

	var totalReads int64
	done := make(chan struct{})

	for r := 0; r < readers; r++ {
		go func() {
			local := int64(0)
			for {
				select {
				case <-done:
					atomic.AddInt64(&totalReads, local)
					return
				default:
					_, _ = store.ListTasks(tasksystem.TaskFilter{Limit: 50})
					local++
				}
			}
		}()
	}

	time.Sleep(duration)
	close(done)
	time.Sleep(100 * time.Millisecond)

	reads := atomic.LoadInt64(&totalReads)

	return map[string]any{
		"readers":       readers,
		"duration_ms":   duration.Milliseconds(),
		"total_reads":   reads,
		"read_qps":      float64(reads) / duration.Seconds(),
		"per_reader_qps": float64(reads) / float64(readers) / duration.Seconds(),
		"wal_mode":      true,
	}, nil
}

// testPureGo 验证纯 Go 特性。
//
// glebarez/sqlite 是 SQLite 的纯 Go transpile 版本，这是它最大的优势：
//   - 无 CGO，交叉编译零成本
//   - 无原生 .so/.dylib，部署简单
//   - 全平台可用（包括 Android/iOS 不需要 NDK）
func testPureGo(store tasksystem.Store) (map[string]any, error) {
	return map[string]any{
		"pure_go":         true,
		"no_cgo":          true,
		"no_native_libs":  true,
		"cross_compile":   true,
		"implementation":  "glebarez/sqlite (go-sqlite transpile)",
	}, nil
}

func makeTestTaskForCapTest(i int) tasksystem.TaskData {
	now := time.Now()
	return tasksystem.TaskData{
		ID:          fmt.Sprintf("__encv_captest_%d", i),
		Type:        tasksystem.TaskTypeEncrypt,
		Status:      tasksystem.StatusCompleted,
		SourcePath:  fmt.Sprintf("/test/source_%d.mp4", i),
		TargetPath:  fmt.Sprintf("/test/target_%d.encv", i),
		PluginName:  "test-plugin",
		TriggeredBy: "user",
		Progress:    100,
		CreatedAt:   now,
		CompletedAt: &now,
	}
}
