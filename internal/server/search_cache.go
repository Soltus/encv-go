package server

import (
	"sync"
	"time"

	mobileservice "github.com/Soltus/encv-go/internal/service"
)

// searchResultCacheEntry 单条缓存项。
type searchResultCacheEntry struct {
	files        []mobileservice.FileInfo
	vectorSearch bool
	searchMode   string
	total        int
	cachedAt     time.Time
	fromQuery    string // 缓存的原始 query（用于诊断和失效判断）
}

// searchResultCache 搜索结果内存 LRU 缓存。
//
// 设计目标（2026-07-02 用户反馈："先搜'在线'，再搜'在线 视频'应即时响应"）：
//   - 连续搜索同一 query → 直接命中，< 1ms 返回
//   - 避免每次搜索都走 DB + 向量搜索 + 混合评分（综合 ~500ms+）
//   - TTL 30s：用户连续搜索的典型时长，过期自动失效
//   - LRU 上限 256：内存可控（每条 ~5KB，峰值 ~1.3MB）
//
// 不是分布式缓存——单进程内有效，进程重启清空。这对前端体验足够。
type searchResultCache struct {
	mu      sync.RWMutex
	entries map[string]*searchResultCacheEntry
	// LRU 链表（最旧在 front，最新在 back）
	order   []string
	maxSize int
	ttl     time.Duration

	// 命中率统计（用于 benchmark 和监控）
	hits      uint64
	misses    uint64
	evictions uint64
}

// newSearchResultCache 构造。
func newSearchResultCache(maxSize int, ttl time.Duration) *searchResultCache {
	if maxSize <= 0 {
		maxSize = 256
	}
	if ttl <= 0 {
		ttl = 30 * time.Second
	}
	return &searchResultCache{
		entries: make(map[string]*searchResultCacheEntry, maxSize),
		order:   make([]string, 0, maxSize),
		maxSize: maxSize,
		ttl:     ttl,
	}
}

// Get 查缓存。命中返回 entry 和 true；未命中返回 nil 和 false。
//
// 命中时把 key 移到 LRU 最新位置。
func (c *searchResultCache) Get(key string) (*searchResultCacheEntry, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[key]
	if !ok {
		c.misses++
		return nil, false
	}
	if time.Since(entry.cachedAt) > c.ttl {
		// 过期 → 视为未命中
		delete(c.entries, key)
		c.removeFromOrderLocked(key)
		c.misses++
		return nil, false
	}
	c.hits++
	c.touchLocked(key)
	return entry, true
}

// Set 写入缓存。满则淘汰最旧。
func (c *searchResultCache) Set(key string, entry *searchResultCacheEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if existing, ok := c.entries[key]; ok {
		// 覆盖：先删旧的
		existing.cachedAt = entry.cachedAt
		existing.files = entry.files
		existing.vectorSearch = entry.vectorSearch
		existing.searchMode = entry.searchMode
		existing.total = entry.total
		existing.fromQuery = entry.fromQuery
		c.touchLocked(key)
		return
	}
	if len(c.entries) >= c.maxSize {
		// 淘汰最旧（order[0]）
		oldest := c.order[0]
		delete(c.entries, oldest)
		c.order = c.order[1:]
		c.evictions++
	}
	c.entries[key] = entry
	c.order = append(c.order, key)
}

// touchLocked 把 key 移到 LRU 最新位置（必须在 mu 已持锁的情况下调用）。
func (c *searchResultCache) touchLocked(key string) {
	for i, k := range c.order {
		if k == key {
			c.order = append(c.order[:i], c.order[i+1:]...)
			c.order = append(c.order, key)
			return
		}
	}
}

func (c *searchResultCache) removeFromOrderLocked(key string) {
	for i, k := range c.order {
		if k == key {
			c.order = append(c.order[:i], c.order[i+1:]...)
			return
		}
	}
}

// Stats 返回命中率统计（snapshot）。
func (c *searchResultCache) Stats() (hits, misses, evictions uint64, size int) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.hits, c.misses, c.evictions, len(c.entries)
}

// Clear 清空（用于测试）。
func (c *searchResultCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[string]*searchResultCacheEntry, c.maxSize)
	c.order = c.order[:0]
}

// HitRate 命中率（0.0~1.0）。
func (c *searchResultCache) HitRate() float64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	total := c.hits + c.misses
	if total == 0 {
		return 0
	}
	return float64(c.hits) / float64(total)
}
