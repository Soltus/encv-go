//go:build js

package logger

import "io"

// jsConsoleWriter 把日志交给 Go 运行时的内建 print（js/wasm 下落到浏览器/Node 的 console）。
//
// ⚠️ **不能写 os.Stderr**：js/wasm 里 os 的写会走 syscall/fs_js 的 fsCall，
// 它需要主 goroutine 响应，而 wasm 内核的主 goroutine 已经永久阻塞在
// `<-make(chan struct{})`（导出函数就是这么挂给 JS 的），于是整只 wasm 直接
//
//	fatal error: all goroutines are asleep - deadlock!
//
// 2026-09-30 实测：`node app/encv-preview/verify-container.mjs <容器> <口令>`
// 一打开容器就死锁在 logger 的这一行（栈里能看到 syscall.fsCall → os.(*File).Write）；
// 浏览器里同样是 js/wasm，只是那边的调用时机没走到这条写路径，所以一直没暴露。
//
// 内建 println 走 runtime 自己的输出通道（不经过 fsCall），因此在任何
// "主线程已阻塞"的时刻都安全。
type jsConsoleWriter struct{}

func (jsConsoleWriter) Write(p []byte) (int, error) {
	println(string(p))
	return len(p), nil
}

func stderrWriter() io.Writer { return jsConsoleWriter{} }
