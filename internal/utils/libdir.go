// 平台无关的 libDir 状态（Android + 沙箱都可见）。
//
// 之前 getLibDir/GetLibDir 在 ffmpeg_dlopen.go（带 //go:build android）→
// 沙箱编译时不可见 → ffmpeg 包 import utils 拿不到 GetLibDir → build 失败。
// 修法：把 getLibDir/GetLibDir 抽到本文件（无 build tag）。
//
// 历史：2026-06-15 拆分——背景：
//   - Android Java 端不保证把 env 注入到 Go 进程
//   - 父进程读 os.Getenv("ENCV_LIB_DIR") 为空时，worker subprocess 也会读空
//   - 修法：父进程显式 cmd.Env 注入 ENCV_LIB_DIR=libDir 给 subprocess
//   - libDir 优先 os.Getenv，空时兜底 GetLibDir()（包级缓存，sync.Once）
package utils

import (
	"os"
	"sync"
)

var (
	libDirOnce  sync.Once
	libDirCache string
)

// getLibDir 是包内用的私有版本（仅同步一次性读取 os.Getenv，sync.Once 保护）。
func getLibDir() string {
	libDirOnce.Do(func() {
		libDirCache = os.Getenv("ENCV_LIB_DIR")
	})
	return libDirCache
}

// GetLibDir 是 getLibDir 的公开版本，供 ffmpeg 包（encode.go）显式注入 worker subprocess env 用。
//
// 2026-06-15 改造：之前 ffmpeg 包用 os.Getenv("ENCV_LIB_DIR") 读父进程 env，但 Android Java 端不保证
// 把 env 注入到 Go 进程 → 父进程读空时 worker subprocess 也会读空 → dlopen /libffmpeg.so 失败。
// 修法：ffmpeg 包显式 import utils 并调 GetLibDir() 兜底。
func GetLibDir() string {
	return getLibDir()
}
