package vectorsearch

import (
	"context"
	"errors"
	"sync"
	"testing"
)

// mockStore 用于测试的 Store 实现，可控制 Search 返回结果或错误。
// 使用 mu 保护 searchCalled / lastLimit，使并发测试能通过 -race 检测。
type mockStore struct {
	mu            sync.Mutex
	searchResults []SearchResult
	searchErr     error
	searchCalled  int // Search 被调用次数
	lastLimit     int // 最近一次 Search 接收到的 limit
	initErr       error
}

// 编译期保证 mockStore 实现 Store 接口。
var _ Store = (*mockStore)(nil)

func (m *mockStore) Init(ctx context.Context) error                            { return m.initErr }
func (m *mockStore) Upsert(ctx context.Context, item *IndexItem) error         { return nil }
func (m *mockStore) UpsertBatch(ctx context.Context, items []*IndexItem) error { return nil }
func (m *mockStore) Delete(ctx context.Context, indexType IndexType, refID string) error {
	return nil
}
func (m *mockStore) Clear(ctx context.Context, indexType IndexType) error { return nil }
func (m *mockStore) Close() error                                         { return nil }

func (m *mockStore) Search(ctx context.Context, indexType IndexType, query string, limit int) ([]SearchResult, error) {
	m.mu.Lock()
	m.searchCalled++
	m.lastLimit = limit
	err := m.searchErr
	results := m.searchResults
	m.mu.Unlock()
	if err != nil {
		return nil, err
	}
	return results, nil
}

// TestSearchWithFallback_PrimarySuccess 验证主 store 成功时直接返回，不降级。
func TestSearchWithFallback_PrimarySuccess(t *testing.T) {
	cases := []struct {
		name    string
		results []SearchResult
	}{
		{
			name:    "主 store 成功返回结果",
			results: []SearchResult{{RefID: "f1", Title: "file1", Score: 0.9, IndexType: IndexTypeFile}},
		},
		{
			name:    "主 store 返回空结果无错误",
			results: nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			primary := &mockStore{searchResults: tc.results}
			fallback := &mockStore{}

			svc := &SearchService{store: primary, fallbackStore: fallback}

			got, err := svc.searchWithFallback(context.Background(), IndexTypeFile, "query", 10)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != len(tc.results) {
				t.Errorf("expected %d results, got %d", len(tc.results), len(got))
			}
			if len(tc.results) > 0 && got[0].RefID != tc.results[0].RefID {
				t.Errorf("expected ref %s, got %s", tc.results[0].RefID, got[0].RefID)
			}
			if svc.IsDegraded() {
				t.Error("主 store 成功时不应降级")
			}
			if fallback.searchCalled != 0 {
				t.Errorf("fallback 不应被调用，实际调用 %d 次", fallback.searchCalled)
			}
			if primary.searchCalled != 1 {
				t.Errorf("primary 应被调用 1 次，实际 %d 次", primary.searchCalled)
			}
		})
	}
}

// TestSearchWithFallback_PrimaryFail_FallbackSuccess 验证主 store 出错时降级到 fallback。
func TestSearchWithFallback_PrimaryFail_FallbackSuccess(t *testing.T) {
	t.Run("主 store 出错切到 fallback 并标记降级", func(t *testing.T) {
		primary := &mockStore{searchErr: errors.New("vector_distance_cos not available")}
		fallback := &mockStore{searchResults: []SearchResult{{RefID: "f1", Title: "fb", IndexType: IndexTypeFile}}}

		svc := &SearchService{store: primary, fallbackStore: fallback}

		got, err := svc.searchWithFallback(context.Background(), IndexTypeFile, "query", 10)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 1 || got[0].RefID != "f1" {
			t.Errorf("期望返回 fallback 结果 f1，got %+v", got)
		}
		if !svc.IsDegraded() {
			t.Error("主 store 出错后应处于降级状态")
		}
		if primary.searchCalled != 1 {
			t.Errorf("primary 应被调用 1 次，实际 %d 次", primary.searchCalled)
		}
		if fallback.searchCalled != 1 {
			t.Errorf("fallback 应被调用 1 次，实际 %d 次", fallback.searchCalled)
		}
	})

	t.Run("返回 fallback 的结果内容", func(t *testing.T) {
		primary := &mockStore{searchErr: errors.New("boom")}
		fbResults := []SearchResult{
			{RefID: "a", Title: "alpha", Score: 0.8, IndexType: IndexTypeFile},
			{RefID: "b", Title: "beta", Score: 0.7, IndexType: IndexTypeFile},
		}
		fallback := &mockStore{searchResults: fbResults}

		svc := &SearchService{store: primary, fallbackStore: fallback}

		got, err := svc.searchWithFallback(context.Background(), IndexTypeFile, "q", 5)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("期望 2 个结果，got %d", len(got))
		}
		if got[0].RefID != "a" || got[1].RefID != "b" {
			t.Errorf("结果顺序/内容不符: %+v", got)
		}
		if !svc.IsDegraded() {
			t.Error("应处于降级状态")
		}
	})

	t.Run("后续请求短路直接走 fallback", func(t *testing.T) {
		primary := &mockStore{searchErr: errors.New("fail")}
		fallback := &mockStore{searchResults: []SearchResult{{RefID: "x", Title: "x", IndexType: IndexTypeFile}}}

		svc := &SearchService{store: primary, fallbackStore: fallback}

		// 第一次调用：主失败，触发降级
		if _, err := svc.searchWithFallback(context.Background(), IndexTypeFile, "q1", 5); err != nil {
			t.Fatalf("第一次调用 unexpected error: %v", err)
		}
		// 第二次调用：已降级，直接走 fallback，主不再被调用
		if _, err := svc.searchWithFallback(context.Background(), IndexTypeFile, "q2", 5); err != nil {
			t.Fatalf("第二次调用 unexpected error: %v", err)
		}

		if primary.searchCalled != 1 {
			t.Errorf("降级后 primary 不应再被调用，期望 1 次，实际 %d 次", primary.searchCalled)
		}
		if fallback.searchCalled != 2 {
			t.Errorf("fallback 应被调用 2 次，实际 %d 次", fallback.searchCalled)
		}
		if !svc.IsDegraded() {
			t.Error("应保持降级状态")
		}
	})
}

// TestSearchWithFallback_NoFallback_ReturnsError 验证无 fallback 时返回原错误且不降级。
func TestSearchWithFallback_NoFallback_ReturnsError(t *testing.T) {
	cases := []struct {
		name string
		err  error
	}{
		{"sqlite 驱动场景无 fallback 返回原错误", errors.New("no fallback")},
		{"无 fallback 时 IsDegraded 保持 false", errors.New("still no fallback")},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			primary := &mockStore{searchErr: tc.err}
			svc := &SearchService{store: primary, fallbackStore: nil}

			got, err := svc.searchWithFallback(context.Background(), IndexTypeFile, "q", 5)
			if err == nil {
				t.Fatal("期望返回错误，got nil")
			}
			if got != nil {
				t.Errorf("期望 nil 结果，got %+v", got)
			}
			if svc.IsDegraded() {
				t.Error("无 fallback 时不应降级")
			}
		})
	}
}

// TestSearchWithFallback_AlreadyDegraded_ShortCircuit 验证已降级状态的短路逻辑。
func TestSearchWithFallback_AlreadyDegraded_ShortCircuit(t *testing.T) {
	t.Run("已降级且 fallback 可用直接走 fallback", func(t *testing.T) {
		primary := &mockStore{searchResults: []SearchResult{{RefID: "p", Title: "primary"}}}
		fallback := &mockStore{searchResults: []SearchResult{{RefID: "f", Title: "fallback", IndexType: IndexTypeFile}}}

		svc := &SearchService{store: primary, fallbackStore: fallback, degraded: true}

		got, err := svc.searchWithFallback(context.Background(), IndexTypeFile, "q", 5)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 1 || got[0].RefID != "f" {
			t.Errorf("期望返回 fallback 结果，got %+v", got)
		}
		if primary.searchCalled != 0 {
			t.Errorf("已降级时 primary 不应被调用，实际 %d 次", primary.searchCalled)
		}
		if fallback.searchCalled != 1 {
			t.Errorf("fallback 应被调用 1 次，实际 %d 次", fallback.searchCalled)
		}
	})

	t.Run("已降级但 fallback 为 nil 仍调主 store", func(t *testing.T) {
		primary := &mockStore{searchResults: []SearchResult{{RefID: "p", Title: "primary", IndexType: IndexTypeFile}}}
		svc := &SearchService{store: primary, fallbackStore: nil, degraded: true}

		got, err := svc.searchWithFallback(context.Background(), IndexTypeFile, "q", 5)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 1 || got[0].RefID != "p" {
			t.Errorf("期望返回主 store 结果，got %+v", got)
		}
		if primary.searchCalled != 1 {
			t.Errorf("fallback 为 nil 时应调用主 store，期望 1 次，实际 %d 次", primary.searchCalled)
		}
	})
}

// TestIsDegraded_InitialState 验证新建服务未降级。
func TestIsDegraded_InitialState(t *testing.T) {
	svc := &SearchService{store: &mockStore{}}
	if svc.IsDegraded() {
		t.Error("新建服务不应处于降级状态")
	}
}

// TestIsDegraded_AfterDegradation 验证触发降级后状态变为 true。
func TestIsDegraded_AfterDegradation(t *testing.T) {
	primary := &mockStore{searchErr: errors.New("fail")}
	fallback := &mockStore{}
	svc := &SearchService{store: primary, fallbackStore: fallback}

	if svc.IsDegraded() {
		t.Fatal("初始状态不应降级")
	}

	if _, err := svc.searchWithFallback(context.Background(), IndexTypeFile, "q", 5); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !svc.IsDegraded() {
		t.Error("触发降级后 IsDegraded 应为 true")
	}
}

// TestSearchFiles_DelegatesToSearchWithFallback 验证 SearchFiles 委托给 searchWithFallback，
// 且 limit<=0 时默认 50。
func TestSearchFiles_DelegatesToSearchWithFallback(t *testing.T) {
	primary := &mockStore{searchResults: []SearchResult{{RefID: "f1", Title: "file", IndexType: IndexTypeFile}}}
	svc := &SearchService{store: primary, fallbackStore: &mockStore{}}

	got, err := svc.SearchFiles(context.Background(), "query", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Errorf("期望 1 个结果，got %d", len(got))
	}
	if svc.IsDegraded() {
		t.Error("主 store 成功时不应降级")
	}
	if primary.searchCalled != 1 {
		t.Errorf("primary 应被调用 1 次，实际 %d 次", primary.searchCalled)
	}
	if primary.lastLimit != 50 {
		t.Errorf("limit<=0 应被夹到 50，实际 %d", primary.lastLimit)
	}
}

// TestSearchTasks_DelegatesToSearchWithFallback 验证 SearchTasks 委托给 searchWithFallback。
func TestSearchTasks_DelegatesToSearchWithFallback(t *testing.T) {
	primary := &mockStore{searchResults: []SearchResult{{RefID: "t1", Title: "task", IndexType: IndexTypeTask}}}
	svc := &SearchService{store: primary, fallbackStore: &mockStore{}}

	got, err := svc.SearchTasks(context.Background(), "query", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Errorf("期望 1 个结果，got %d", len(got))
	}
	if got[0].RefID != "t1" {
		t.Errorf("期望 ref t1，got %s", got[0].RefID)
	}
}

// TestSearchFiles_LimitClamping 验证 limit 边界处理。
func TestSearchFiles_LimitClamping(t *testing.T) {
	t.Run("limit 为 -1 被夹到 50 不 panic", func(t *testing.T) {
		primary := &mockStore{searchResults: []SearchResult{{RefID: "f1", Title: "file", IndexType: IndexTypeFile}}}
		svc := &SearchService{store: primary, fallbackStore: &mockStore{}}

		got, err := svc.SearchFiles(context.Background(), "query", -1)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 1 {
			t.Errorf("期望 1 个结果，got %d", len(got))
		}
		if primary.lastLimit != 50 {
			t.Errorf("limit 应被夹到 50，实际 %d", primary.lastLimit)
		}
	})

	t.Run("limit 为 1000 不 panic 返回结果", func(t *testing.T) {
		primary := &mockStore{searchResults: []SearchResult{{RefID: "f1", Title: "file", IndexType: IndexTypeFile}}}
		svc := &SearchService{store: primary, fallbackStore: &mockStore{}}

		got, err := svc.SearchFiles(context.Background(), "query", 1000)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 1 {
			t.Errorf("期望 1 个结果，got %d", len(got))
		}
		if primary.lastLimit != 1000 {
			t.Errorf("limit 应原样传递 1000，实际 %d", primary.lastLimit)
		}
	})
}

// TestConcurrentDegradation 验证并发触发降级的幂等性与无竞态（go test -race 应通过）。
func TestConcurrentDegradation(t *testing.T) {
	primary := &mockStore{searchErr: errors.New("concurrent fail")}
	fallback := &mockStore{searchResults: []SearchResult{{RefID: "fb", Title: "fallback", IndexType: IndexTypeFile}}}
	svc := &SearchService{store: primary, fallbackStore: fallback}

	const n = 10
	var wg sync.WaitGroup
	wg.Add(n)
	errs := make(chan error, n)

	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			got, err := svc.searchWithFallback(context.Background(), IndexTypeFile, "q", 5)
			if err != nil {
				errs <- err
				return
			}
			if len(got) != 1 {
				errs <- errors.New("期望 1 个 fallback 结果")
				return
			}
			errs <- nil
		}()
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("goroutine 返回错误: %v", err)
		}
	}

	if !svc.IsDegraded() {
		t.Error("并发降级后 IsDegraded 应为 true")
	}
}
