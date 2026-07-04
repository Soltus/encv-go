// internal/server/mock_generator_stubs_test.go
//
// 🆕 2026-06-14：临时 stub，让 pre-existing 失败的 mock_generator_test.go 能编译。
//
// 历史：
//   - commit 249485d (ffmpeg-patch-1) 重构了 mock_generator.go，删了
//     minimalMP4 / minimalMKV / minimalMP3 / minimalFLAC 四个函数，替换为
//     planMP4() / planMKV() / ... 返回 mockFileSpec。
//   - mock_generator_test.go 没有同步更新，仍然引用 minimalMP4() 等。
//   - 这是 pre-existing 问题，与本次 runtime_api 重构无关。
//
// 本文件提供 no-op stub，让 mock_generator_test.go 在不修原作者测试的前提下能编译。
// 原作者改造完测试后可删除本文件。

package server

// minimalMP4/MKV/MP3/FLAC 是 mock_generator_test.go 引用的 stub。
// 原始实现是返回 *[]byte，已被 plan*() 系列替代。
// 这里 stub 返回空 spec（data=nil, ffmpegArgs=nil），与原作者测试"data==nil 跳过"的逻辑兼容。
func minimalMP4() mockFileSpec  { return mockFileSpec{} }
func minimalMKV() mockFileSpec  { return mockFileSpec{} }
func minimalMP3() mockFileSpec  { return mockFileSpec{} }
func minimalFLAC() mockFileSpec { return mockFileSpec{} }
