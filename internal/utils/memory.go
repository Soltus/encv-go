package utils

import (
	"runtime"
	"sync/atomic"
)

// 获取系统可用内存，单位：字节
// 这是一个跨平台的实现，优先使用 golang.org/x/sys，如果失败则降级
func GetAvailableMemory() uint64 {
	// 1. 优先尝试使用更精确的 golang.org/x/sys 库
	if mem := getAvailableMemoryFromSys(); mem > 0 {
		return mem
	}

	// 2. 降级方案：使用 runtime 包估算一个粗略的值
	// 这非常不准确，但聊胜于无
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	// 一个非常粗略的估算：假设系统总内存是 Go 分配的内存的 10 倍
	// 可用内存 = (估算的系统总内存) - (Go 已分配内存)
	// 这个数字可能非常不准，仅作为最后的备选
	estimatedTotalMem := m.Sys * 10
	if estimatedTotalMem > m.Alloc {
		return estimatedTotalMem - m.Alloc
	}

	// 如果连估算都失败，返回 0
	return 0
}

// 使用 atomic.Value 来缓存结果，避免频繁调用系统 API
var cachedAvailableMemory uint64
var memoryCached int32 // 用作 bool

// GetCachedAvailableMemory 获取缓存的可用内存，只计算一次
func GetCachedAvailableMemory() uint64 {
	if atomic.LoadInt32(&memoryCached) == 1 {
		return cachedAvailableMemory
	}

	mem := GetAvailableMemory()
	atomic.StoreUint64(&cachedAvailableMemory, mem)
	atomic.StoreInt32(&memoryCached, 1)

	return mem
}
