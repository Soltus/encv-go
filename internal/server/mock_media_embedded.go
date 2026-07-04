// internal/server/mock_media_embedded.go
// 把 ffmpeg 处理的「真输入文件」用 go:embed 编译进二进制。
//
// ════════════════════════════════════════════════════════════════════
// 2026-06-11 重构：mock 生成必须调 ffmpeg（不许绕开）
// ════════════════════════════════════════════════════════════════════
//
// 用户反馈："我辛苦编译集成ffmpeg你不调？" —— 之前一版改用 go:embed 直接
// 写预编码 mp4/mkv/mp3/flac 字节，**完全绕开了 ffmpeg 集成**。这是错的。
//
// 设计修正（按用户提示"参考加密视频任务怎么调用 ffmpeg 的"）：
//   加密视频任务：ffmpeg.Run 调「真输入文件 → 输出文件」（永远不依赖 lavfi）
//   mock 生成：   同样 ffmpeg 调「真输入文件 → 输出文件」
//   区别只在于：
//     1. mock 的「真输入」用 go:embed 字节（写 tmp 文件给 ffmpeg 读）
//     2. ffmpeg 再把它转成 mp4/mkv/mp3/flac 各格式
//
// 历史 bug（旧 lavfi 路径）：
//   - libffmpeg.so 是为「加密视频任务」编的，configure 没加 --enable-indev=lavfi
//   - 旧实现用 `-f lavfi -i sine=...` → "Unknown input format: lavfi"
//   - 失败不报错 → 写 0 字节 → 测试链全挂
//   - cgo 偶发 segfault → Go 进程挂 → 后端崩溃
//
// 新路径（真输入文件）：
//   - 内嵌 source.mp4（h264+aac, 2s, 160x120, ~20KB）+ source.wav（pcm_s16le, 1s, 16KB）
//   - 写 tmp 文件 → ffmpeg 读真文件 → 输出目标格式
//   - 真机永远 OK（不依赖 lavfi）
//
// 真机 ffmpeg build manifest ([app/encv-mobile/scripts/ffmpeg-feature-manifest.json])：
//   encoders: aac, pcm_s16le, pcm_s24le, pcm_s32le, libx264
//   muxers:   mp4, matroska, flac, mp3, adts, null
//   demuxers: mov, matroska, aac, mp3, flac, ogg, wav
//   关键：没编 libmp3lame / flac encoder → 真机 mp3/flac 输出**必失败**
//        沙箱 ffmpeg 6.1 有 libmp3lame + flac → 沙箱 mp3/flac 正常
//        测试在沙箱跑，real device 仅 mp4/mkv 可生成
// ════════════════════════════════════════════════════════════════════
package server

import (
	_ "embed"
)

//go:embed mock_media/source.mp4
var sourceMP4Bytes []byte

//go:embed mock_media/source.wav
var sourceWAVBytes []byte
