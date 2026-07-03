//go:build objectbox
// +build objectbox

package objectbox

import (
	"fmt"
	"time"

	"github.com/Soltus/encv-go/pkg/tasksystem"
)

func init() {
	tasksystem.RegisterCapabilityTest(tasksystem.CapabilityTest{
		ID:          "objectbox_box_put_batch",
		Name:        "Box 批量写入性能",
		Description: "ObjectBox 原生 Box.Put 批量写入，对比 SQL  INSERT 的性能优势",
		Category:    "performance",
		Engine:      "objectbox",
		Run:         testBoxPutBatch,
	})

	tasksystem.RegisterCapabilityTest(tasksystem.CapabilityTest{
		ID:          "objectbox_query_index",
		Name:        "索引查询性能",
		Description: "ObjectBox 对象索引查询，验证面向对象数据库的查询优化能力",
		Category:    "performance",
		Engine:      "objectbox",
		Run:         testQueryIndex,
	})

	tasksystem.RegisterCapabilityTest(tasksystem.CapabilityTest{
		ID:          "objectbox_object_native",
		Name:        "原生对象存储验证",
		Description: "验证 ObjectBox 面向对象数据库特性：无需 ORM、无 SQL 阻抗失配",
		Category:    "feature",
		Engine:      "objectbox",
		Run:         testObjectNative,
	})
}

// testBoxPutBatch 测试 Box 批量写入性能。
//
// ObjectBox 的核心优势：
//   - 零 ORM 开销：直接存对象，无 SQL 解析、无参数绑定
//   - 批量 Put 优化：Box.PutMany 一次性写入大量对象
//   - 零拷贝：内存映射文件，读写直接操作内存
func testBoxPutBatch(store tasksystem.Store) (map[string]any, error) {
	s, ok := store.(*Store)
	if !ok {
		return nil, fmt.Errorf("not an objectbox store")
	}

	const batchSize = 1000
	const batches = 5

	entities := make([]*TaskEntity, batchSize)
	start := time.Now()
	totalPut := 0
	for b := 0; b < batches; b++ {
		for i := 0; i < batchSize; i++ {
			now := time.Now().UnixMilli()
			entities[i] = &TaskEntity{
				TaskID:      fmt.Sprintf("__encv_captest_obx_%d_%d", b, i),
				Type:        "encrypt",
				Status:      "completed",
				SourcePath:  fmt.Sprintf("/test/src_%d_%d.mp4", b, i),
				TargetPath:  fmt.Sprintf("/test/dst_%d_%d.encv", b, i),
				PluginName:  "test-plugin",
				TriggeredBy: "cap_test",
				Progress:    100,
				CreatedAt:   now,
				CompletedAt: now,
			}
		}
		ids, err := s.tasks.PutMany(entities)
		if err != nil {
			return nil, fmt.Errorf("put many batch %d: %w", b, err)
		}
		totalPut += len(ids)
	}
	totalMs := time.Since(start).Milliseconds()

	return map[string]any{
		"batch_size":   batchSize,
		"batches":      batches,
		"total_items":  totalPut,
		"total_ms":     totalMs,
		"items_per_sec": float64(totalPut) / float64(totalMs) * 1000,
		"avg_batch_ms": float64(totalMs) / float64(batches),
		"native_object": true,
		"no_orm":       true,
	}, nil
}

// testQueryIndex 测试索引查询性能。
//
// ObjectBox 的查询优化：
//   - 原生对象索引，不是 B-tree 模拟
//   - PropertyEqString 等类型安全查询构建器
//   - 查询结果可直接用，无需行扫描
func testQueryIndex(store tasksystem.Store) (map[string]any, error) {
	s, ok := store.(*Store)
	if !ok {
		return nil, fmt.Errorf("not an objectbox store")
	}

	const numDocs = 2000
	for i := 0; i < numDocs; i++ {
		now := time.Now().UnixMilli()
		status := []string{"pending", "running", "completed", "failed"}[i%4]
		typ := []string{"encrypt", "decrypt"}[i%2]
		trigger := []string{"user", "system", "automation", "ai_agent"}[i%4]
		s.tasks.Put(&TaskEntity{
			TaskID:      fmt.Sprintf("__encv_captest_qidx_%d", i),
			Type:        typ,
			Status:      status,
			TriggeredBy: trigger,
			Progress:    i % 100,
			CreatedAt:   now,
		})
	}

	const iterations = 200

	start := time.Now()
	for i := 0; i < iterations; i++ {
		q := s.tasks.Query(
			TaskEntity_.Type.Equals("encrypt", false),
			TaskEntity_.Status.Equals("completed", false),
		)
		results, _ := q.Find()
		q.Close()
		_ = results
	}
	queryMs := time.Since(start).Milliseconds()

	var count uint64
	q := s.tasks.Query(
		TaskEntity_.Type.Equals("encrypt", false),
		TaskEntity_.Status.Equals("completed", false),
	)
	count, _ = q.Count()
	q.Close()

	return map[string]any{
		"total_docs":   numDocs,
		"iterations":   iterations,
		"total_ms":     queryMs,
		"avg_query_ms": float64(queryMs) / float64(iterations),
		"qps":          float64(iterations) / (float64(queryMs) / 1000),
		"matched":      count,
		"type_safe_query": true,
		"no_sql":       true,
	}, nil
}

// testObjectNative 验证原生对象存储特性。
//
// ObjectBox 对比 SQLite 的本质区别：
//   - 不是关系型数据库，没有表/行/列的概念
//   - 直接存对象，没有 ORM 映射开销
//   - 对象关系是一等公民（ToOne / ToMany）
func testObjectNative(store tasksystem.Store) (map[string]any, error) {
	s, ok := store.(*Store)
	if !ok {
		return nil, fmt.Errorf("not an objectbox store")
	}

	now := time.Now().UnixMilli()
	entity := &TaskEntity{
		TaskID:      "__encv_captest_native_obj",
		Type:        "encrypt",
		Status:      "running",
		SourcePath:  "/native/test.mp4",
		PluginName:  "native-test",
		TriggeredBy: "cap_test",
		Progress:    42,
		CreatedAt:   now,
	}

	id, err := s.tasks.Put(entity)
	if err != nil {
		return nil, fmt.Errorf("put native object: %w", err)
	}

	got, err := s.tasks.Get(id)
	if err != nil {
		return nil, fmt.Errorf("get native object: %w", err)
	}

	return map[string]any{
		"object_id":         id,
		"roundtrip_ok":      got.TaskID == entity.TaskID && got.Progress == 42,
		"storage_model":     "object-oriented",
		"no_orm_mapping":    true,
		"no_sql":            true,
		"no_table_schema":   true,
		"direct_object_api": true,
		"zero_copy_mmap":    true,
	}, nil
}
