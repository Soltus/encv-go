package simverse

import (
	"container/list"
	"sync"
)

type EntityCache[T any] struct {
	mu       sync.RWMutex
	capacity int
	items    map[uint64]*list.Element
	lru      *list.List
}

type genericCacheEntry[T any] struct {
	key uint64
	val T
}

func NewEntityCache[T any](capacity int) *EntityCache[T] {
	return &EntityCache[T]{
		capacity: capacity,
		items:    make(map[uint64]*list.Element, capacity),
		lru:      list.New(),
	}
}

func (c *EntityCache[T]) Get(id uint64) (T, bool) {
	c.mu.RLock()
	elem, ok := c.items[id]
	c.mu.RUnlock()
	if !ok {
		var zero T
		return zero, false
	}
	c.mu.Lock()
	c.lru.MoveToFront(elem)
	c.mu.Unlock()
	return elem.Value.(*genericCacheEntry[T]).val, true
}

func (c *EntityCache[T]) Put(id uint64, val T) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if elem, ok := c.items[id]; ok {
		elem.Value.(*genericCacheEntry[T]).val = val
		c.lru.MoveToFront(elem)
		return
	}

	elem := c.lru.PushFront(&genericCacheEntry[T]{key: id, val: val})
	c.items[id] = elem

	if c.lru.Len() > c.capacity {
		last := c.lru.Back()
		if last != nil {
			c.lru.Remove(last)
			delete(c.items, last.Value.(*genericCacheEntry[T]).key)
		}
	}
}

func (c *EntityCache[T]) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lru.Len()
}

func (c *EntityCache[T]) Resize(newCapacity int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.capacity = newCapacity
	for c.lru.Len() > c.capacity {
		last := c.lru.Back()
		if last != nil {
			c.lru.Remove(last)
			delete(c.items, last.Value.(*genericCacheEntry[T]).key)
		}
	}
}

func (c *EntityCache[T]) All() []T {
	c.mu.RLock()
	defer c.mu.RUnlock()
	result := make([]T, 0, c.lru.Len())
	for e := c.lru.Front(); e != nil; e = e.Next() {
		result = append(result, e.Value.(*genericCacheEntry[T]).val)
	}
	return result
}
