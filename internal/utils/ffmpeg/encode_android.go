//go:build android

package ffmpeg

import (
	"context"
	"fmt"

	"github.com/Soltus/encv-go/internal/utils"
)

// Encode 在真机环境下 spawn worker (cmd/ffmpeg-worker/ffmpeg_worker.c) 子进程，
// 通过 JSON-RPC 协议让 worker 内部 cgo dlopen libffmpeg.so 调 ffmpeg_run。
//
// 为什么真机不直接 CallFFmpegNative？
//   - cgo 阻塞时父进程无法 cancel → 父进程 hang
//   - 子进程可以被 ctx cancel SIGKILL → 父进程 unblock
//   - 父进程仅 wait worker + 解析 JSON 响应
func Encode(ctx context.Context, args ...string) (*EncodeResult, error) {
	bin, err := locateWorker()
	if err != nil {
		return nil, fmt.Errorf("ffmpeg-worker not found: %w", err)
	}
	return runWorkerJSON(ctx, bin, "ffmpeg", args)
}

// Probe 在真机环境下直接 in-process cgo 调 libffprobe.so（utils.CallFFprobeNative）。
//
// 为什么 Probe 不走 worker？
//   - Probe 调用是 read-only + 短时（metadata 提取 < 100ms）→ 阻塞父进程 OK
//   - in-process 比 spawn worker 快 10x（无进程创建开销）
//   - 失败时（libffprobe.so 缺 symbol）立即返回 -1/-2 错误码，metadata_extractor
//     可分类提示（"ffprobe engine unavailable"）
func Probe(ctx context.Context, args ...string) ([]byte, error) {
	res, err := utils.CallFFprobeNative(args)
	if err != nil {
		return nil, err
	}
	if res.ExitCode != 0 {
		return []byte(res.Stdout), &utils.NativeError{
			Type:     utils.NativeErrorExit,
			ExitCode: res.ExitCode,
			Detail:   res.Stderr,
		}
	}
	return []byte(res.Stdout), nil
}

// IsAvailable 探活：委托 utils.CheckFFmpegAvailable（内部用 RTLD_NOW | RTLD_LOCAL 试 dlopen，
// 不会执行 ffmpeg_run → cgo 不会阻塞 OS thread）。
//
// CheckFFmpegAvailable 返回 5 个值（ok, ok, err, ffmpegDetail, ffprobeDetail），
// 这里只取前 3 个（ffmpeg / ffprobe 是否可用 + 合并错误信息），detail 留给调用方按需调 utils 包。
func IsAvailable() (ffmpegOk, ffprobeOk bool, errMsg string) {
	ok, ok2, msg, _, _ := utils.CheckFFmpegAvailable()
	return ok, ok2, msg
}
