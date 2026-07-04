package server

import (
	"fmt"
	"strconv"
	"sync"
	"testing"
	"time"

	mobileservice "github.com/Soltus/encv-go/internal/service"
)

// makeCacheEntry 构造一个简单的缓存项（10 个 fake 文件）。
func makeCacheEntry(query string, n int) *searchResultCacheEntry {
	files := make([]mobileservice.FileInfo, n)
	for i := 0; i < n; i++ {
		files[i] = mobileservice.FileInfo{
			Name: fmt.Sprintf("file-%s-%d.mp4", query, i),
			Path: fmt.Sprintf("/d/%s/file-%d.mp4", query, i),
			Size: int64(1024 * i),
		}
	}
	return &searchResultCacheEntry{
		files:        files,
		vectorSearch: true,
		searchMode:   "combined",
		total:        n,
		cachedAt:     time.Now(),
		fromQuery:    query,
	}
}

// TestSearchCache_BasicHitMiss 验证基本命中/未命中。
func TestSearchCache_BasicHitMiss(t *testing.T) {
	c := newSearchResultCache(10, 30*time.Second)
	defer c.Clear()

	// 第一次 Get → 未命中
	if _, ok := c.Get("k1"); ok {
		t.Error("k1 不应存在，期望未命中")
	}

	// Set 后 Get → 命中
	c.Set("k1", makeCacheEntry("在线", 5))
	if entry, ok := c.Get("k1"); !ok {
		t.Error("k1 应命中")
	} else if entry.total != 5 {
		t.Errorf("entry.total = %d, want 5", entry.total)
	}
}

// TestSearchCache_TTL 验证 TTL 过期后视为未命中。
func TestSearchCache_TTL(t *testing.T) {
	c := newSearchResultCache(10, 50*time.Millisecond) // 50ms TTL 便于测试
	defer c.Clear()

	c.Set("k1", makeCacheEntry("在线", 3))
	if _, ok := c.Get("k1"); !ok {
		t.Error("Set 后立即 Get 应命中")
	}

	time.Sleep(80 * time.Millisecond) // 超过 TTL
	if _, ok := c.Get("k1"); ok {
		t.Error("超过 TTL 后 Get 应未命中（视为过期）")
	}

	hits, misses, _, _ := c.Stats()
	if hits != 1 || misses != 1 {
		t.Errorf("stats: hits=%d misses=%d, want hits=1 misses=1", hits, misses)
	}
}

// TestSearchCache_LRUEviction 验证 LRU 淘汰最旧。
func TestSearchCache_LRUEviction(t *testing.T) {
	c := newSearchResultCache(3, 30*time.Second) // maxSize=3
	defer c.Clear()

	c.Set("k1", makeCacheEntry("q1", 1))
	c.Set("k2", makeCacheEntry("q2", 2))
	c.Set("k3", makeCacheEntry("q3", 3))

	// 满了，再加 k4 → 淘汰最旧 k1
	c.Set("k4", makeCacheEntry("q4", 4))
	if _, ok := c.Get("k1"); ok {
		t.Error("k1 应被 LRU 淘汰")
	}
	if _, ok := c.Get("k2"); !ok {
		t.Error("k2 应保留")
	}
	if _, ok := c.Get("k3"); !ok {
		t.Error("k3 应保留")
	}
	if _, ok := c.Get("k4"); !ok {
		t.Error("k4 应保留")
	}

	hits, misses, evictions, _ := c.Stats()
	if evictions != 1 {
		t.Errorf("evictions = %d, want 1", evictions)
	}
	if misses != 1 { // 访问已淘汰的 k1
		t.Errorf("misses = %d, want 1", misses)
	}
	if hits != 3 { // k2 k3 k4 命中
		t.Errorf("hits = %d, want 3", hits)
	}
}

// TestSearchCache_AccessUpdatesLRUOrder 验证 Get 会更新 LRU 顺序。
func TestSearchCache_AccessUpdatesLRUOrder(t *testing.T) {
	c := newSearchResultCache(3, 30*time.Second)
	defer c.Clear()

	c.Set("k1", makeCacheEntry("q1", 1))
	c.Set("k2", makeCacheEntry("q2", 2))
	c.Set("k3", makeCacheEntry("q3", 3))

	// 访问 k1 → 把它移到最新
	if _, ok := c.Get("k1"); !ok {
		t.Fatal("k1 应命中")
	}

	// 加 k4 → 应淘汰最旧 k2（不是 k1）
	c.Set("k4", makeCacheEntry("q4", 4))
	if _, ok := c.Get("k2"); ok {
		t.Error("k2 应被淘汰（最近没被访问）")
	}
	if _, ok := c.Get("k1"); !ok {
		t.Error("k1 应保留（最近被访问）")
	}
}

// TestSearchCache_OverwriteExisting 验证 Set 覆盖已有 key。
func TestSearchCache_OverwriteExisting(t *testing.T) {
	c := newSearchResultCache(10, 30*time.Second)
	defer c.Clear()

	c.Set("k1", makeCacheEntry("old", 5))
	c.Set("k1", makeCacheEntry("new", 99))

	entry, ok := c.Get("k1")
	if !ok {
		t.Fatal("k1 应命中")
	}
	if entry.total != 99 {
		t.Errorf("entry.total = %d, want 99（覆盖后的值）", entry.total)
	}
	if entry.fromQuery != "new" {
		t.Errorf("entry.fromQuery = %q, want %q", entry.fromQuery, "new")
	}
}

// TestSearchCache_ConcurrentSafety 验证并发安全（多个 goroutine 同时 Get/Set）。
func TestSearchCache_ConcurrentSafety(t *testing.T) {
	c := newSearchResultCache(100, 30*time.Second)
	defer c.Clear()

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func(i int) {
			defer wg.Done()
			key := "key-" + strconv.Itoa(i%10)
			c.Set(key, makeCacheEntry(key, i))
		}(i)
		go func(i int) {
			defer wg.Done()
			key := "key-" + strconv.Itoa(i%10)
			c.Get(key)
		}(i)
	}
	wg.Wait()
	// 不 panic 即通过
}

// TestBuildSearchCacheKey_Deterministic 验证 key 一致性。
func TestBuildSearchCacheKey_Deterministic(t *testing.T) {
	k1 := buildSearchCacheKey("/d", "在线", true, 50)
	k2 := buildSearchCacheKey("/d", "在线", true, 50)
	if k1 != k2 {
		t.Errorf("相同输入应产生相同 key: %q vs %q", k1, k2)
	}
}

// TestBuildSearchCacheKey_Unique 验证不同输入产生不同 key。
func TestBuildSearchCacheKey_Unique(t *testing.T) {
	tests := []struct {
		path       string
		query      string
		recursive  bool
		limit      int
	}{
		{"/d", "在线", true, 50},
		{"/d/视频", "在线", true, 50},       // path 不同
		{"/d", "视频", true, 50},             // query 不同
		{"/d", "在线", false, 50},            // recursive 不同
		{"/d", "在线", true, 100},            // limit 不同
		{"/d", "在线 视频", true, 50},        // query 不同
	}
	seen := make(map[string]bool)
	for _, tc := range tests {
		k := buildSearchCacheKey(tc.path, tc.query, tc.recursive, tc.limit)
		if seen[k] {
			t.Errorf("key 重复: %q (path=%q query=%q rec=%v limit=%d)", k, tc.path, tc.query, tc.recursive, tc.limit)
		}
		seen[k] = true
	}
	if len(seen) != len(tests) {
		t.Errorf("去重后 %d 个 key，输入 %d 个", len(seen), len(tests))
	}
}

// TestSearchCache_HitRate 验证命中率计算。
func TestSearchCache_HitRate(t *testing.T) {
	c := newSearchResultCache(10, 30*time.Second)
	defer c.Clear()

	// 空状态：0 / 0 = 0
	if rate := c.HitRate(); rate != 0 {
		t.Errorf("空状态 HitRate = %f, want 0", rate)
	}

	c.Set("k1", makeCacheEntry("q1", 1))
	c.Get("k1") // hit
	c.Get("k1") // hit
	c.Get("k2") // miss

	rate := c.HitRate()
	expected := 2.0 / 3.0
	if rate < expected-0.01 || rate > expected+0.01 {
		t.Errorf("HitRate = %f, want %f (2 hits, 1 miss)", rate, expected)
	}
}

// BenchmarkSearchCache_Get 验证 Get 性能（命中场景下应该 < 1µs）。
func BenchmarkSearchCache_Get(b *testing.B) {
	c := newSearchResultCache(256, 30*time.Second)
	defer c.Clear()
	c.Set("bench-key", makeCacheEntry("在线", 20))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = c.Get("bench-key")
	}
}

// BenchmarkSearchCache_Set 验证 Set 性能（应 < 5µs）。
func BenchmarkSearchCache_Set(b *testing.B) {
	c := newSearchResultCache(256, 30*time.Second)
	defer c.Clear()
	entry := makeCacheEntry("在线", 20)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Set("bench-key-"+strconv.Itoa(i), entry)
	}
}

// BenchmarkSearchCache_GetVsMiss 对比命中 vs 未命中性能。
// 命中应该比未命中快（都很快，但 LRU 链表遍历成本不同）。
func BenchmarkSearchCache_GetVsMiss(b *testing.B) {
	c := newSearchResultCache(256, 30*time.Second)
	defer c.Clear()
	c.Set("hit-key", makeCacheEntry("在线", 20))

	b.Run("Hit", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, _ = c.Get("hit-key")
		}
	})
	b.Run("Miss", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, _ = c.Get("miss-key")
		}
	})
}
