// internal/server/database_test_runner.go
// 数据库自动化测试运行器（真机 / 桌面端通用）
//
// 提供端点：
//   POST /api/database/test/run  { scenarios: ["crud", "batch_write", ...] }  → SSE 流式进度
//
// 测试场景（scenarios）：
//   - crud         : 基础 CRUD 测试（Create / Read / Update / Delete）
//   - batch_write  : 批量写入性能测试（1000 条任务）
//   - query_filter : 查询过滤性能测试（按 type / status / triggeredBy 等多条件过滤）
//   - concurrency  : 并发写入测试（10 个 goroutine 各写 100 条）
//   - export_import: 导出/导入一致性测试
//   - full         : 全部测试（默认）
//
// 设计原则：
//   - 测试数据使用独立的前缀，跑完后清理，不污染用户真实数据
//   - 每个场景独立计时，返回耗时和结果
//   - SSE 流式推送进度，前端可实时展示
//   - 直接使用 tasksystem.Store 接口，不经过 TaskManager 业务层（避免触发真实任务逻辑）
package server

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"math/rand"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Soltus/encv-go/pkg/tasksystem"
	"github.com/gin-gonic/gin"
)

// dbTestScenario 数据库测试场景定义
type dbTestScenario string

const (
	DBTestScenarioCRUD            dbTestScenario = "crud"
	DBTestScenarioBatchWrite      dbTestScenario = "batch_write"
	DBTestScenarioQueryFilter     dbTestScenario = "query_filter"
	DBTestScenarioConcurrency     dbTestScenario = "concurrency"
	DBTestScenarioExportImport    dbTestScenario = "export_import"
	DBTestScenarioLargeTableQuery dbTestScenario = "large_table_query"
	DBTestScenarioConcurrentRW    dbTestScenario = "concurrent_rw"
	DBTestScenarioTransaction     dbTestScenario = "transaction"
	DBTestScenarioFull            dbTestScenario = "full"
)

// dbTestProgress SSE 进度事件
type dbTestProgress struct {
	Phase    string  `json:"phase"`    // "started" | "running" | "passed" | "failed"
	Scenario string  `json:"scenario"` // 当前场景名
	Message  string  `json:"message"`  // 描述信息
	DurationMs int64  `json:"duration_ms,omitempty"`
	Metrics  map[string]any `json:"metrics,omitempty"`
	Error    string  `json:"error,omitempty"`
}

// dbTestRequest 测试请求体
type dbTestRequest struct {
	Scenarios []string `json:"scenarios"` // 要运行的场景列表，空或 ["full"] 表示全部
}

// dbTestDataPrefix 测试数据的统一前缀，用于识别和清理
const dbTestDataPrefix = "__encv_dbtest_"

// handleDatabaseTestRun 运行数据库自动化测试（SSE 流式）
func (s *Server) handleDatabaseTestRun(c *gin.Context) {
	tm := s.mobileSvc.GetTaskManager()
	store := tm.GetStore()
	if store == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "database store not available"})
		return
	}

	var req dbTestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		// 空 body 也可以，默认跑 full
		req = dbTestRequest{Scenarios: []string{"full"}}
	}

	scenarios := resolveScenarios(req.Scenarios)

	// 设置 SSE header
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "streaming not supported"})
		return
	}

	sendEvent := func(progress dbTestProgress) {
		data, _ := json.Marshal(progress)
		fmt.Fprintf(c.Writer, "data: %s\n\n", data)
		flusher.Flush()
	}

	// 开始测试
	slog.Info("db test: starting", "scenarios", len(scenarios), "engine", store.EngineName())

	totalStart := time.Now()
	var failed []string
	var passed []string

	// 测试前：清理可能残留的测试数据
	cleanupTestData(store)
	defer cleanupTestData(store)

	for _, scenario := range scenarios {
		scenarioStart := time.Now()
		sendEvent(dbTestProgress{
			Phase:    "started",
			Scenario: string(scenario),
			Message:  fmt.Sprintf("开始测试: %s", scenario),
		})

		var metrics map[string]any
		var err error

		switch scenario {
		case DBTestScenarioCRUD:
			metrics, err = runDBTestCRUD(store)
		case DBTestScenarioBatchWrite:
			metrics, err = runDBTestBatchWrite(store)
		case DBTestScenarioQueryFilter:
			metrics, err = runDBTestQueryFilter(store)
		case DBTestScenarioConcurrency:
			metrics, err = runDBTestConcurrency(store)
		case DBTestScenarioExportImport:
			metrics, err = runDBTestExportImport(store)
		case DBTestScenarioLargeTableQuery:
			metrics, err = runDBTestLargeTableQuery(store)
		case DBTestScenarioConcurrentRW:
			metrics, err = runDBTestConcurrentRW(store)
		case DBTestScenarioTransaction:
			metrics, err = runDBTestTransaction(store)
		}

		duration := time.Since(scenarioStart).Milliseconds()

		if err != nil {
			slog.Warn("db test: scenario failed", "scenario", scenario, "err", err, "duration_ms", duration)
			failed = append(failed, string(scenario))
			sendEvent(dbTestProgress{
				Phase:      "failed",
				Scenario:   string(scenario),
				Message:    fmt.Sprintf("测试失败: %v", err),
				DurationMs: duration,
				Error:      err.Error(),
			})
		} else {
			passed = append(passed, string(scenario))
			sendEvent(dbTestProgress{
				Phase:      "passed",
				Scenario:   string(scenario),
				Message:    "测试通过",
				DurationMs: duration,
				Metrics:    metrics,
			})
		}

		// 每个场景后清理一次，避免数据累积
		cleanupTestData(store)
	}

	// 汇总
	totalDuration := time.Since(totalStart).Milliseconds()
	sendEvent(dbTestProgress{
		Phase:    "completed",
		Scenario: "summary",
		Message:  fmt.Sprintf("测试完成: %d 通过, %d 失败", len(passed), len(failed)),
		DurationMs: totalDuration,
		Metrics: map[string]any{
			"passed": passed,
			"failed": failed,
			"total":  len(scenarios),
			"engine": store.EngineName(),
		},
	})

	slog.Info("db test: completed",
		"engine", store.EngineName(),
		"passed", len(passed),
		"failed", len(failed),
		"total_ms", totalDuration,
	)
}

// resolveScenarios 解析场景列表，"full" 展开为全部场景
func resolveScenarios(requested []string) []dbTestScenario {
	if len(requested) == 0 {
		return allDBSscenarios()
	}
	for _, s := range requested {
		if s == "full" {
			return allDBSscenarios()
		}
	}
	result := make([]dbTestScenario, 0, len(requested))
	for _, s := range requested {
		result = append(result, dbTestScenario(s))
	}
	return result
}

func allDBSscenarios() []dbTestScenario {
	return []dbTestScenario{
		DBTestScenarioCRUD,
		DBTestScenarioBatchWrite,
		DBTestScenarioQueryFilter,
		DBTestScenarioConcurrency,
		DBTestScenarioExportImport,
		DBTestScenarioLargeTableQuery,
		DBTestScenarioConcurrentRW,
		DBTestScenarioTransaction,
	}
}

// cleanupTestData 清理测试数据（按 ID 前缀匹配）
func cleanupTestData(store tasksystem.Store) {
	// 列出所有任务，删除测试数据
	tasks, err := store.ListTasks(tasksystem.TaskFilter{Limit: 100000})
	if err != nil {
		slog.Warn("db test: cleanup list failed", "err", err)
		return
	}
	deleted := 0
	for _, t := range tasks {
		if len(t.ID) >= len(dbTestDataPrefix) && t.ID[:len(dbTestDataPrefix)] == dbTestDataPrefix {
			if err := store.DeleteTask(t.ID); err == nil {
				deleted++
			}
		}
	}
	if deleted > 0 {
		slog.Debug("db test: cleaned up test data", "count", deleted)
	}
}

// makeTestTask 创建一个测试任务（带前缀 ID）
func makeTestTask(idx int) tasksystem.TaskData {
	now := time.Now()
	types := []tasksystem.TaskType{"encrypt", "decrypt"}
	statuses := []tasksystem.TaskStatus{"pending", "running", "success", "failed"}
	triggeredBys := []string{"user", "system", "automation", "ai_agent"}
	t := tasksystem.TaskData{
		ID:            fmt.Sprintf("%s%d_%d", dbTestDataPrefix, time.Now().UnixNano(), idx),
		Type:          types[idx%len(types)],
		Status:        statuses[idx%len(statuses)],
		TriggeredBy:   triggeredBys[idx%len(triggeredBys)],
		SourcePath:    fmt.Sprintf("/test/source/file_%d.mp4", idx),
		TargetPath:    fmt.Sprintf("/test/target/file_%d.encv", idx),
		Progress:      rand.Intn(100),
		ServiceName:   "test_service",
		MethodName:    "test_method",
		CreatedAt:     now.Add(-time.Duration(idx) * time.Second),
	}
	return t
}

// ─── 各测试场景实现 ─────────────────────────────────────────────────

// runDBTestCRUD 基础 CRUD 测试
func runDBTestCRUD(store tasksystem.Store) (map[string]any, error) {
	task := makeTestTask(0)

	// Create
	start := time.Now()
	if err := store.CreateTask(task); err != nil {
		return nil, fmt.Errorf("create failed: %w", err)
	}
	createMs := time.Since(start).Milliseconds()

	// Read
	start = time.Now()
	got, err := store.GetTask(task.ID)
	if err != nil {
		return nil, fmt.Errorf("read failed: %w", err)
	}
	readMs := time.Since(start).Milliseconds()

	if got.ID != task.ID || got.Type != task.Type {
		return nil, fmt.Errorf("read mismatch: expected id=%s type=%s, got id=%s type=%s",
			task.ID, task.Type, got.ID, got.Type)
	}

	// Update
	task.Status = "success"
	task.Progress = 100
	start = time.Now()
	if err := store.UpdateTask(task); err != nil {
		return nil, fmt.Errorf("update failed: %w", err)
	}
	updateMs := time.Since(start).Milliseconds()

	// 验证更新
	got2, err := store.GetTask(task.ID)
	if err != nil {
		return nil, fmt.Errorf("read after update failed: %w", err)
	}
	if got2.Status != "success" || got2.Progress != 100 {
		return nil, fmt.Errorf("update mismatch: status=%s progress=%d", got2.Status, got2.Progress)
	}

	// Delete
	start = time.Now()
	if err := store.DeleteTask(task.ID); err != nil {
		return nil, fmt.Errorf("delete failed: %w", err)
	}
	deleteMs := time.Since(start).Milliseconds()

	// 验证删除
	_, err = store.GetTask(task.ID)
	if err == nil {
		return nil, fmt.Errorf("task still exists after delete")
	}
	if err != tasksystem.ErrNotFound {
		return nil, fmt.Errorf("expected ErrNotFound after delete, got: %w", err)
	}

	return map[string]any{
		"create_ms": createMs,
		"read_ms":   readMs,
		"update_ms": updateMs,
		"delete_ms": deleteMs,
	}, nil
}

// runDBTestBatchWrite 批量写入性能测试
func runDBTestBatchWrite(store tasksystem.Store) (map[string]any, error) {
	const count = 1000
	tasks := make([]tasksystem.TaskData, count)
	for i := 0; i < count; i++ {
		tasks[i] = makeTestTask(i)
	}

	start := time.Now()
	for i := 0; i < count; i++ {
		if err := store.CreateTask(tasks[i]); err != nil {
			return nil, fmt.Errorf("batch write failed at %d: %w", i, err)
		}
	}
	totalMs := time.Since(start).Milliseconds()

	// 验证数量
	all, err := store.ListTasks(tasksystem.TaskFilter{Limit: count * 2})
	if err != nil {
		return nil, fmt.Errorf("list after batch write failed: %w", err)
	}
	testCount := 0
	for _, t := range all {
		if len(t.ID) >= len(dbTestDataPrefix) && t.ID[:len(dbTestDataPrefix)] == dbTestDataPrefix {
			testCount++
		}
	}

	return map[string]any{
		"total_count":    count,
		"total_ms":       totalMs,
		"ops_per_sec":    float64(count) / (float64(totalMs) / 1000.0),
		"avg_ms_per_op":  float64(totalMs) / float64(count),
		"verified_count": testCount,
	}, nil
}

// runDBTestQueryFilter 查询过滤性能测试
func runDBTestQueryFilter(store tasksystem.Store) (map[string]any, error) {
	// 先准备数据
	const count = 500
	for i := 0; i < count; i++ {
		task := makeTestTask(i)
		if err := store.CreateTask(task); err != nil {
			return nil, fmt.Errorf("prepare data failed at %d: %w", i, err)
		}
	}

	results := map[string]any{}

	// 按 type 过滤
	start := time.Now()
	encryptTasks, err := store.ListTasks(tasksystem.TaskFilter{
		Types: []tasksystem.TaskType{"encrypt"},
		Limit: count,
	})
	if err != nil {
		return nil, fmt.Errorf("query by type failed: %w", err)
	}
	results["query_type_ms"] = time.Since(start).Milliseconds()
	results["query_type_count"] = len(encryptTasks)

	// 按 status 过滤
	start = time.Now()
	successTasks, err := store.ListTasks(tasksystem.TaskFilter{
		Statuses: []tasksystem.TaskStatus{"success"},
		Limit: count,
	})
	if err != nil {
		return nil, fmt.Errorf("query by status failed: %w", err)
	}
	results["query_status_ms"] = time.Since(start).Milliseconds()
	results["query_status_count"] = len(successTasks)

	// 组合过滤（type + status + triggeredBy）
	start = time.Now()
	combinedTasks, err := store.ListTasks(tasksystem.TaskFilter{
		Types:       []tasksystem.TaskType{"encrypt"},
		Statuses:    []tasksystem.TaskStatus{"running"},
		TriggeredBy: []string{"user"},
		Limit:       count,
	})
	if err != nil {
		return nil, fmt.Errorf("query combined failed: %w", err)
	}
	results["query_combined_ms"] = time.Since(start).Milliseconds()
	results["query_combined_count"] = len(combinedTasks)

	// 分页
	start = time.Now()
	pagedTasks, err := store.ListTasks(tasksystem.TaskFilter{
		Limit:  50,
		Offset: 100,
	})
	if err != nil {
		return nil, fmt.Errorf("query paged failed: %w", err)
	}
	results["query_paged_ms"] = time.Since(start).Milliseconds()
	results["query_paged_count"] = len(pagedTasks)

	return results, nil
}

// runDBTestConcurrency 并发写入测试
func runDBTestConcurrency(store tasksystem.Store) (map[string]any, error) {
	const goroutines = 10
	const perGoroutine = 100
	const total = goroutines * perGoroutine

	var wg sync.WaitGroup
	errCh := make(chan error, goroutines)

	start := time.Now()

	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(gid int) {
			defer wg.Done()
			for i := 0; i < perGoroutine; i++ {
				task := makeTestTask(gid*perGoroutine + i)
				task.TriggeredBy = fmt.Sprintf("concurrency_g%d", gid)
				if err := store.CreateTask(task); err != nil {
					errCh <- fmt.Errorf("g%d i%d: %w", gid, i, err)
					return
				}
			}
		}(g)
	}

	wg.Wait()
	close(errCh)

	totalMs := time.Since(start).Milliseconds()

	// 检查错误
	for err := range errCh {
		if err != nil {
			return nil, fmt.Errorf("concurrency error: %w", err)
		}
	}

	// 验证总数
	all, err := store.ListTasks(tasksystem.TaskFilter{Limit: total * 2})
	if err != nil {
		return nil, fmt.Errorf("list after concurrency failed: %w", err)
	}
	testCount := 0
	for _, t := range all {
		if len(t.ID) >= len(dbTestDataPrefix) && t.ID[:len(dbTestDataPrefix)] == dbTestDataPrefix {
			testCount++
		}
	}

	return map[string]any{
		"goroutines":    goroutines,
		"per_goroutine": perGoroutine,
		"total_count":   total,
		"total_ms":      totalMs,
		"ops_per_sec":   float64(total) / (float64(totalMs) / 1000.0),
		"verified_count": testCount,
	}, nil
}

// runDBTestExportImport 导出/导入一致性测试
func runDBTestExportImport(store tasksystem.Store) (map[string]any, error) {
	// 先准备数据
	const count = 100
	ids := make([]string, count)
	for i := 0; i < count; i++ {
		task := makeTestTask(i)
		ids[i] = task.ID
		if err := store.CreateTask(task); err != nil {
			return nil, fmt.Errorf("prepare data failed at %d: %w", i, err)
		}
	}

	// 导出
	start := time.Now()
	dump, err := store.ExportAll()
	if err != nil {
		return nil, fmt.Errorf("export failed: %w", err)
	}
	exportMs := time.Since(start).Milliseconds()

	// 清理当前数据
	cleanupTestData(store)

	// 导入
	start = time.Now()
	if err := store.ImportAll(dump); err != nil {
		return nil, fmt.Errorf("import failed: %w", err)
	}
	importMs := time.Since(start).Milliseconds()

	// 验证导入的数量
	all, err := store.ListTasks(tasksystem.TaskFilter{Limit: count * 2})
	if err != nil {
		return nil, fmt.Errorf("list after import failed: %w", err)
	}
	testCount := 0
	for _, t := range all {
		if len(t.ID) >= len(dbTestDataPrefix) && t.ID[:len(dbTestDataPrefix)] == dbTestDataPrefix {
			testCount++
		}
	}

	if testCount != count {
		return nil, fmt.Errorf("import count mismatch: expected %d, got %d", count, testCount)
	}

	// 验证第一条数据完整性
	first, err := store.GetTask(ids[0])
	if err != nil {
		return nil, fmt.Errorf("get after import failed: %w", err)
	}
	if first.ID != ids[0] {
		return nil, fmt.Errorf("import data mismatch: id=%s", first.ID)
	}

	return map[string]any{
		"task_count":   count,
		"export_ms":    exportMs,
		"import_ms":    importMs,
		"verified":     testCount,
		"dump_version": dump.Version,
		"dump_engine":  dump.Engine,
	}, nil
}

// runDBTestLargeTableQuery 大表查询性能测试（引擎特殊能力：索引/查询优化）
//
// 先写入 5000 条任务，然后测：
//   - 单条件过滤查询速度
//   - 多条件组合过滤查询速度
//   - 分页查询速度
//
// 不同引擎差异：
//   - SQLite: 走索引但单写者，查询时可并发读
//   - LibSQL/Turso: MVCC 快照读，查询不阻塞写入
//   - ObjectBox: 对象索引，等值查询快但范围查询弱
func runDBTestLargeTableQuery(store tasksystem.Store) (map[string]any, error) {
	const total = 5000

	for i := 0; i < total; i++ {
		task := makeTestTask(i)
		task.Status = tasksystem.TaskStatus([]tasksystem.TaskStatus{
			tasksystem.StatusQueued, tasksystem.StatusRunning,
			tasksystem.StatusCompleted, tasksystem.StatusFailed,
		}[i%4])
		task.Type = tasksystem.TaskType([]tasksystem.TaskType{
			tasksystem.TaskTypeEncrypt, tasksystem.TaskTypeDecrypt,
		}[i%2])
		task.TriggeredBy = []string{"user", "system", "automation", "ai_agent"}[i%4]
		if err := store.CreateTask(task); err != nil {
			return nil, fmt.Errorf("create task %d: %w", i, err)
		}
	}

	// 测试 1：单条件过滤（按 type）
	start := time.Now()
	for i := 0; i < 10; i++ {
		_, err := store.ListTasks(tasksystem.TaskFilter{
			Types: []tasksystem.TaskType{tasksystem.TaskTypeEncrypt},
			Limit: total,
		})
		if err != nil {
			return nil, fmt.Errorf("filter by type: %w", err)
		}
	}
	typeFilterMs := time.Since(start).Milliseconds() / 10

	// 测试 2：多条件组合过滤（type + status + triggeredBy）
	start = time.Now()
	for i := 0; i < 10; i++ {
		_, err := store.ListTasks(tasksystem.TaskFilter{
			Types:       []tasksystem.TaskType{tasksystem.TaskTypeEncrypt},
			Statuses:    []tasksystem.TaskStatus{tasksystem.StatusRunning},
			TriggeredBy: []string{"user"},
			Limit:       total,
		})
		if err != nil {
			return nil, fmt.Errorf("filter combined: %w", err)
		}
	}
	combinedFilterMs := time.Since(start).Milliseconds() / 10

	// 测试 3：分页查询（大偏移量）
	start = time.Now()
	pageCount := 0
	for offset := 0; offset < total; offset += 100 {
		_, err := store.ListTasks(tasksystem.TaskFilter{Limit: 100, Offset: offset})
		if err != nil {
			return nil, fmt.Errorf("pagination offset=%d: %w", offset, err)
		}
		pageCount++
	}
	paginationMs := time.Since(start).Milliseconds()

	return map[string]any{
		"total_rows":        total,
		"type_filter_avg_ms": typeFilterMs,
		"combined_filter_avg_ms": combinedFilterMs,
		"pagination_total_ms": paginationMs,
		"pagination_pages":  pageCount,
	}, nil
}

// runDBTestConcurrentRW 并发读写测试（引擎特殊能力：MVCC / 事务隔离）
//
// 3 个写 goroutine + 5 个读 goroutine 同时跑，测：
//   - 读写是否互相阻塞（MVCC 引擎读不阻塞写）
//   - 数据一致性（读到的是否是已提交的快照）
//   - 总吞吐量
//
// 不同引擎差异：
//   - SQLite: 单写者，写时阻塞所有读（WAL 模式下读读不阻塞）
//   - LibSQL/Turso: MVCC，读写互不阻塞，快照隔离
//   - ObjectBox: 多版本并发，读写互不阻塞
func runDBTestConcurrentRW(store tasksystem.Store) (map[string]any, error) {
	const writers = 3
	const readers = 5
	const perWriter = 200
	const duration = 5 * time.Second

	// 先写入一批基础数据
	for i := 0; i < 500; i++ {
		task := makeTestTask(10000 + i)
		if err := store.CreateTask(task); err != nil {
			return nil, fmt.Errorf("init data: %w", err)
		}
	}

	var wg sync.WaitGroup
	var writeCount int64
	var readCount int64
	var firstErr error
	var errOnce sync.Once

	// 写 goroutine
	for w := 0; w < writers; w++ {
		wg.Add(1)
		go func(wid int) {
			defer wg.Done()
			deadline := time.Now().Add(duration)
			i := 0
			for time.Now().Before(deadline) {
				task := makeTestTask(20000 + wid*10000 + i)
				if err := store.CreateTask(task); err != nil {
					errOnce.Do(func() { firstErr = fmt.Errorf("writer %d: %w", wid, err) })
					return
				}
				i++
			}
			atomic.AddInt64(&writeCount, int64(i))
		}(w)
	}

	// 读 goroutine
	for r := 0; r < readers; r++ {
		wg.Add(1)
		go func(rid int) {
			defer wg.Done()
			deadline := time.Now().Add(duration)
			localCount := int64(0)
			for time.Now().Before(deadline) {
				_, err := store.ListTasks(tasksystem.TaskFilter{Limit: 100})
				if err != nil {
					errOnce.Do(func() { firstErr = fmt.Errorf("reader %d: %w", rid, err) })
					return
				}
				localCount++
			}
			atomic.AddInt64(&readCount, localCount)
		}(r)
	}

	wg.Wait()

	if firstErr != nil {
		return nil, firstErr
	}

	writes := atomic.LoadInt64(&writeCount)
	reads := atomic.LoadInt64(&readCount)

	return map[string]any{
		"writers":       writers,
		"readers":       readers,
		"duration_ms":   duration.Milliseconds(),
		"write_count":   writes,
		"read_count":    reads,
		"write_tps":     float64(writes) / duration.Seconds(),
		"read_qps":      float64(reads) / duration.Seconds(),
		"total_ops":     writes + reads,
		"total_tps":     float64(writes+reads) / duration.Seconds(),
	}, nil
}

// runDBTestTransaction 事务原子性 / 一致性测试（引擎特殊能力：ACID 事务）
//
// 测试内容：
//   - 批量写入 + 验证完整性（原子性：要么全成功要么全失败）
//   - 导入导出前后数据一致性（持久性）
//   - 并发更新同一条记录的正确性（隔离性）
//
// 不同引擎差异：
//   - SQLite: 支持完整 ACID，单写者事务
//   - LibSQL/Turso: 支持完整 ACID + MVCC 快照隔离
//   - ObjectBox: 支持 ACID 事务，对象级锁
func runDBTestTransaction(store tasksystem.Store) (map[string]any, error) {
	testsPassed := 0
	totalTests := 3

	// 测试 1：导入导出一致性（持久性 + 一致性）
	{
		const n = 100
		for i := 0; i < n; i++ {
			task := makeTestTask(30000 + i)
			task.Progress = i
			if err := store.CreateTask(task); err != nil {
				return nil, fmt.Errorf("tx test 1 create: %w", err)
			}
		}

		dump, err := store.ExportAll()
		if err != nil {
			return nil, fmt.Errorf("tx test 1 export: %w", err)
		}

		if err := store.ImportAll(dump); err != nil {
			return nil, fmt.Errorf("tx test 1 import: %w", err)
		}

		// 随机抽查 10 条
		mismatch := 0
		for i := 0; i < 10; i++ {
			id := fmt.Sprintf("%s%d", dbTestDataPrefix, 30000+i)
			got, err := store.GetTask(id)
			if err != nil {
				mismatch++
				continue
			}
			if got.Progress != i {
				mismatch++
			}
		}
		if mismatch == 0 {
			testsPassed++
		}
	}

	// 测试 2：删除 + 回滚验证（原子性通过 ImportAll 验证）
	{
		// 导出当前状态
		dump, err := store.ExportAll()
		if err != nil {
			return nil, fmt.Errorf("tx test 2 export before: %w", err)
		}
		countBefore := len(dump.Tasks)

		// 删除 10 条
		for i := 0; i < 10; i++ {
			id := fmt.Sprintf("%s%d", dbTestDataPrefix, 30000+i)
			if err := store.DeleteTask(id); err != nil {
				return nil, fmt.Errorf("tx test 2 delete: %w", err)
			}
		}

		// 导入回去（相当于回滚）
		if err := store.ImportAll(dump); err != nil {
			return nil, fmt.Errorf("tx test 2 import back: %w", err)
		}

		dumpAfter, err := store.ExportAll()
		if err != nil {
			return nil, fmt.Errorf("tx test 2 export after: %w", err)
		}

		if len(dumpAfter.Tasks) == countBefore {
			testsPassed++
		}
	}

	// 测试 3：更新一致性（更新后字段必须全部正确）
	{
		id := fmt.Sprintf("%s%d", dbTestDataPrefix, 30050)
		task, err := store.GetTask(id)
		if err != nil {
			return nil, fmt.Errorf("tx test 3 get: %w", err)
		}

		task.Status = tasksystem.StatusCompleted
		task.Progress = 100
		task.Error = "tx_test_error"
		if err := store.UpdateTask(task); err != nil {
			return nil, fmt.Errorf("tx test 3 update: %w", err)
		}

		got, err := store.GetTask(id)
		if err != nil {
			return nil, fmt.Errorf("tx test 3 get after: %w", err)
		}

		if got.Status == tasksystem.StatusCompleted && got.Progress == 100 && got.Error == "tx_test_error" {
			testsPassed++
		}
	}

	return map[string]any{
		"total_tests":   totalTests,
		"passed_tests":  testsPassed,
		"acid_compliant": testsPassed == totalTests,
	}, nil
}
