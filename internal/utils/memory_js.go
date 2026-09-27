//go:build js

package utils

// getAvailableMemoryFromSys 在 js/wasm 下没有系统级内存查询可用
// （golang.org/x/sys/unix 不提供 Sysinfo），返回 0 让 GetAvailableMemory 走降级估算。
//
// 与 memory_unix.go / memory_windows.go 是同一组平台实现，签名必须一致。
func getAvailableMemoryFromSys() uint64 {
	return 0
}
