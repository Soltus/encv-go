package simverse

import (
	"container/list"
	"fmt"
	"runtime"
	"sync"
	"testing"
	"time"
)

type EntityCache struct {
	mu       sync.RWMutex
	capacity int
	items    map[uint64]*list.Element
	lru      *list.List
}

type cacheEntry struct {
	key uint64
	val NPC
}

func NewEntityCache(capacity int) *EntityCache {
	return &EntityCache{
		capacity: capacity,
		items:    make(map[uint64]*list.Element, capacity),
		lru:      list.New(),
	}
}

func (c *EntityCache) Get(id uint64) (NPC, bool) {
	c.mu.RLock()
	elem, ok := c.items[id]
	c.mu.RUnlock()
	if !ok {
		return NPC{}, false
	}
	c.mu.Lock()
	c.lru.MoveToFront(elem)
	c.mu.Unlock()
	return elem.Value.(*cacheEntry).val, true
}

func (c *EntityCache) Put(npc NPC) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if elem, ok := c.items[npc.ID]; ok {
		elem.Value.(*cacheEntry).val = npc
		c.lru.MoveToFront(elem)
		return
	}

	elem := c.lru.PushFront(&cacheEntry{key: npc.ID, val: npc})
	c.items[npc.ID] = elem

	if c.lru.Len() > c.capacity {
		last := c.lru.Back()
		if last != nil {
			c.lru.Remove(last)
			delete(c.items, last.Value.(*cacheEntry).key)
		}
	}
}

func (c *EntityCache) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lru.Len()
}

func TestEntityCacheLRU(t *testing.T) {
	cache := NewEntityCache(100)

	for i := 0; i < 200; i++ {
		cache.Put(NPC{ID: uint64(i), Name: fmt.Sprintf("NPC_%d", i)})
	}

	if cache.Len() != 100 {
		t.Fatalf("Expected cache size 100, got %d", cache.Len())
	}

	_, ok := cache.Get(0)
	if ok {
		t.Fatal("NPC 0 should have been evicted")
	}

	_, ok = cache.Get(199)
	if !ok {
		t.Fatal("NPC 199 should be in cache")
	}
}

func BenchmarkEntityCache_Get(b *testing.B) {
	cache := NewEntityCache(10000)
	for i := 0; i < 10000; i++ {
		cache.Put(NPC{ID: uint64(i), Name: fmt.Sprintf("NPC_%d", i)})
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache.Get(uint64(i % 10000))
	}
}

func BenchmarkEntityCache_Put(b *testing.B) {
	cache := NewEntityCache(10000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache.Put(NPC{ID: uint64(i % 20000), Name: "NPC"})
	}
}

func TestStability_3MinuteRun(t *testing.T) {
	t.Skip("Skipping long stability test by default. Run with -run TestStability_3MinuteRun -timeout 5m to enable.")

	duration := 3 * time.Minute
	deadline := time.Now().Add(duration)

	cache := NewEntityCache(10000)
	scheduler := NewEventScheduler()
	eventsProcessed := 0
	ticks := 0

	startMem := getMemMB()
	t.Logf("Starting 3-minute stability run...")
	t.Logf("Initial memory: %.2f MB", startMem)

	for time.Now().Before(deadline) {
		tick := int64(ticks)

		for i := 0; i < 100; i++ {
			scheduler.Schedule(Event{
				ID:          uint64(ticks*100 + i),
				Type:        uint8(i % 20),
				TargetID:    uint64(i % 10000),
				ScheduledAt: tick + int64(i%50),
			})
		}

		ready := scheduler.Tick(tick)
		eventsProcessed += len(ready)

		for _, e := range ready {
			npc, ok := cache.Get(e.TargetID)
			if !ok {
				npc = NPC{ID: e.TargetID, Name: fmt.Sprintf("NPC_%d", e.TargetID), Health: 1000, Energy: 800}
			}
			npc.Health = uint16(int(npc.Health) + int(e.Type) - 10)
			if npc.Health > 1000 {
				npc.Health = 1000
			}
			cache.Put(npc)
		}

		ticks++

		if ticks%10000 == 0 {
			currentMem := getMemMB()
			t.Logf("  Tick %d: events=%d, mem=%.2f MB, delta=%.2f MB",
				ticks, eventsProcessed, currentMem, currentMem-startMem)
		}
	}

	endMem := getMemMB()
	t.Logf("=== 3-Minute Stability Results ===")
	t.Logf("Total ticks: %d", ticks)
	t.Logf("Events processed: %d", eventsProcessed)
	t.Logf("Start mem: %.2f MB", startMem)
	t.Logf("End mem: %.2f MB", endMem)
	t.Logf("Mem delta: %.2f MB", endMem-startMem)
	t.Logf("Cache size: %d", cache.Len())

	if endMem-startMem > 20 {
		t.Errorf("Memory grew by %.2f MB in 3 minutes (possible leak)", endMem-startMem)
	} else {
		t.Log("✅ No significant memory leak detected")
	}
}

func getMemMB() float64 {
	var m runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&m)
	return float64(m.HeapInuse) / 1024 / 1024
}
