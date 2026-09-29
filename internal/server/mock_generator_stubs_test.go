// 🆕 2026-09-30：这里**不再是 stub** —— 直接跑真实的 ffmpeg 生成。
//
// 历史：
//   - commit 249485d (ffmpeg-patch-1) 重构了 mock_generator.go，删了
//     minimalMP4 / minimalMKV / minimalMP3 / minimalFLAC 四个函数，替换为
//     planMP4() / planMKV() / ... 返回 mockFileSpec。
//   - mock_generator_test.go 没有同步更新，仍然引用 minimalMP4() 等。
//     当时本文件提供 no-op stub（返回空 spec）只为让它能编译。
//
// 为什么必须换成真实实现：
//   stub 返回空 spec，而 TestMinimalMedia* 只在**没有 ffmpeg** 时才 SKIP
//   （requireFFmpeg）。2026-09-30 本机装上 ffmpeg 7.1 之后不再 SKIP，
//   测试拿到 data=nil → TestMinimalMediaIsPlayable 直接红（实测）。
//   也就是说这套 stub 把「有没有 ffmpeg」变成了「测试红不红」的开关，
//   而有 ffmpeg 的机器（本地 dev / 真机）恰恰是它本该守住的场景。
//
// ⚠️ 别用 plan*() 顶替：plan*() 只返回**计划**，data 字段是空的
//   （mockFileSpec.data 的注释：plan 阶段为 nil，handler 跑完 ffmpeg 才填）。
//   要拿到真字节必须调 ffmpegGenerate(ext)。

package server

func specFromFFmpeg(ext string) mockFileSpec {
	data, stderr, exitCode, _ := ffmpegGenerate(ext)
	return mockFileSpec{data: data, stderr: stderr, exitCode: exitCode}
}

func minimalMP4() mockFileSpec  { return specFromFFmpeg("mp4") }
func minimalMKV() mockFileSpec  { return specFromFFmpeg("mkv") }
func minimalMP3() mockFileSpec  { return specFromFFmpeg("mp3") }
func minimalFLAC() mockFileSpec { return specFromFFmpeg("flac") }
