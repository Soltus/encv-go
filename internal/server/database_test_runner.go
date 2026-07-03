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
	"time"

	"github.com/Soltus/encv-go/pkg/tasksystem"
	"github.com/gin-gonic/gin"
)

// dbTestScenario 数据库测试场景定义
type dbTestScenario string

const (
	DBTestScenarioCRUD        dbTestScenario = "crud"
	DBTestScenarioBatchWrite  dbTestScenario = "batch_write"
	DBTestScenarioQueryFilter dbTestScenario = "query_filter"
	DBTestScenarioConcurrency dbTestScenario = "concurrency"
	DBTestScenarioExportImport dbTestScenario = "export_import"
	DBTestScenarioFull        dbTestScenario = "full"
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
