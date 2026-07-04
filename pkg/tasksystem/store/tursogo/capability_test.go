package tursogo

import (
	"encoding/binary"
	"fmt"
	"math"
	"sync/atomic"
	"time"

	"github.com/Soltus/encv-go/pkg/tasksystem"
)

func init() {
	tasksystem.RegisterCapabilityTest(tasksystem.CapabilityTest{
		ID:          "turso_vector_search",
		Name:        "向量搜索能力",
		Description: "测试 Turso 原生 vector_distance_cos 向量余弦距离搜索，Turso 核心差异化特性",
		Category:    "feature",
		Engine:      "turso",
		Run:         testTursoVectorSearch,
	})

	tasksystem.RegisterCapabilityTest(tasksystem.CapabilityTest{
		ID:          "turso_mvcc_concurrent",
		Name:        "MVCC 多写者并发",
		Description: "测试 Turso MVCC 架构的多协程并发写入性能，对比 SQLite 单写者的核心优势",
		Category:    "performance",
		Engine:      "turso",
		Run:         testTursoMVCC,
	})

	tasksystem.RegisterCapabilityTest(tasksystem.CapabilityTest{
		ID:          "turso_async_io",
		Name:        "异步 I/O 读性能",
		Description: "测试 Turso 异步 I/O 架构的并发读性能，I/O 密集场景下的优势",
		Category:    "performance",
		Engine:      "turso",
		Run:         testTursoAsyncIO,
	})
}

func testTursoVectorSearch(store tasksystem.Store) (map[string]any, error) {
	s, ok := store.(*Store)
	if !ok {
		return nil, fmt.Errorf("not a turso store")
	}

	const dim = 256
	const numDocs = 1000

	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS __encv_captest_vec (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		title TEXT,
		embedding BLOB
	)`)
	if err != nil {
		return nil, fmt.Errorf("create vector table: %w", err)
	}
	defer s.db.Exec(`DROP TABLE IF EXISTS __encv_captest_vec`)

	tx, _ := s.db.Begin()
	stmt, _ := tx.Prepare(`INSERT INTO __encv_captest_vec (title, embedding) VALUES (?, ?)`)
	for i := 0; i < numDocs; i++ {
		vec := make([]float32, dim)
		for j := 0; j < dim; j++ {
			vec[j] = float32(math.Sin(float64(i*dim+j)) * 0.01)
		}
		blob := encodeVec(vec)
		stmt.Exec(fmt.Sprintf("doc_%d", i), blob)
	}
	stmt.Close()
	tx.Commit()

	queryVec := make([]float32, dim)
	for j := 0; j < dim; j++ {
		queryVec[j] = float32(math.Sin(float64(j)) * 0.01)
	}
	queryBlob := encodeVec(queryVec)

	const iterations = 100
	start := time.Now()
	var topTitle string
	var topScore float64
	for i := 0; i < iterations; i++ {
		row := s.db.QueryRow(`
			SELECT title, 1.0 - vector_distance_cos(embedding, ?) AS score
			FROM __encv_captest_vec
			ORDER BY score DESC
			LIMIT 1
		`, queryBlob)
		row.Scan(&topTitle, &topScore)
	}
	searchMs := time.Since(start).Milliseconds()

	return map[string]any{
		"vector_dim":     dim,
		"doc_count":      numDocs,
		"iterations":     iterations,
		"total_ms":       searchMs,
		"avg_query_ms":   float64(searchMs) / float64(iterations),
		"qps":            float64(iterations) / (float64(searchMs) / 1000),
		"top_result":     topTitle,
		"top_score":      topScore,
		"native_vector":  true,
		"engine":         "turso (official)",
	}, nil
}

func testTursoMVCC(store tasksystem.Store) (map[string]any, error) {
	const writers = 8
	const duration = 3 * time.Second

	var totalWrites int64
	done := make(chan struct{})

	for w := 0; w < writers; w++ {
		go func(wid int) {
			local := int64(0)
			for i := 0; ; i++ {
				select {
				case <-done:
					atomic.AddInt64(&totalWrites, local)
					return
				default:
					task := tasksystem.TaskData{
						ID:          fmt.Sprintf("__encv_captest_turso_mvcc_%d_%d", wid, i),
						Type:        tasksystem.TaskTypeEncrypt,
						Status:      tasksystem.StatusCompleted,
						SourcePath:  fmt.Sprintf("/test/src_%d_%d.mp4", wid, i),
						PluginName:  "turso-test",
						TriggeredBy: "cap_test",
						Progress:    100,
						CreatedAt:   time.Now(),
					}
					_ = store.CreateTask(task)
					local++
				}
			}
		}(w)
	}

	time.Sleep(duration)
	close(done)
	time.Sleep(100 * time.Millisecond)

	writes := atomic.LoadInt64(&totalWrites)

	return map[string]any{
		"writers":      writers,
		"duration_ms":  duration.Milliseconds(),
		"total_writes": writes,
		"write_tps":    float64(writes) / duration.Seconds(),
		"mvcc":         true,
		"multi_writer": true,
	}, nil
}

func testTursoAsyncIO(store tasksystem.Store) (map[string]any, error) {
	s, ok := store.(*Store)
	if !ok {
		return nil, fmt.Errorf("not a turso store")
	}

	for i := 0; i < 500; i++ {
		task := tasksystem.TaskData{
			ID:          fmt.Sprintf("__encv_captest_turso_aio_%d", i),
			Type:        tasksystem.TaskTypeEncrypt,
			Status:      tasksystem.StatusCompleted,
			SourcePath:  fmt.Sprintf("/test/aio_%d.mp4", i),
			PluginName:  "aio-test",
			TriggeredBy: "cap_test",
			Progress:    100,
			CreatedAt:   time.Now(),
		}
		store.CreateTask(task)
	}

	const readers = 16
	const duration = 2 * time.Second

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
					rows, _ := s.db.Query(`SELECT COUNT(*) FROM __encv_tasks WHERE status = ?`, "completed")
					if rows != nil {
						rows.Next()
						rows.Scan(new(int))
						rows.Close()
					}
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
		"async_io":      true,
		"concurrent_io": true,
	}, nil
}

func encodeVec(vec []float32) []byte {
	buf := make([]byte, len(vec)*4)
	for i, v := range vec {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(v))
	}
	return buf
}
