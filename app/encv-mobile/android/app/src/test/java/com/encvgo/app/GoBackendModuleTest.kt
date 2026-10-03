// ⚠️ 2026-10-03 挂起（原来是编译阻断）：本用例依赖的 `GoBackendModule` 类已在某次重构中删除
//   （grep 全 main 源码已无任何引用），只剩这 2 处 Unresolved reference
//   ⇒ `:app:compileDebugUnitTestKotlin` 一直失败 ⇒ **:app:testDebugUnitTest 从来没被跑起来过**
//   （这也是安卓侧没有任何契约锁能拦住 "localhost:443" 事故的原因之一）。
//
// 处理方式：保留文件与历史意图，整体注释停用（与 vitest 侧孤儿用例同款处理），
// 等 owner 决定"补实现"还是"彻底删用例"。
//
// 原文件内容（停用前）：
//
// package com.encvgo.app
//
// import kotlin.test.Test
// import kotlin.test.assertEquals
// import kotlin.test.assertTrue
// import org.junit.Before
// import org.junit.runner.RunWith
// import org.mockito.junit.MockitoJUnitRunner
//
// @RunWith(MockitoJUnitRunner::class)
// class GoBackendModuleTest {
//
//     @Before
//     fun setUp() {
//         EncvGoService.isRunning = false
//         EncvGoService.lastKnownPort = 0
//         EncvGoService.lastError = null
//     }
//
//     @Test
//     fun test_companionEventConstants() {
//         assertEquals("backend:ready", GoBackendModule.EVENT_READY)
//         assertEquals("backend:error", GoBackendModule.EVENT_ERROR)
//     }
//
//     @Test
//     fun test_getStreamUrl_validPort_internal() {
//         EncvGoService.lastKnownPort = 2025
//         val port = EncvGoService.lastKnownPort
//         val path = "/storage/emulated/0/test.mp4"
//         val url = GoBackendModule.getStreamUrl(port, path, external = false)
//         assertTrue(url.startsWith("http://127.0.0.1:2025/api/stream?path="))
//     }
//
//     @Test
//     fun test_getStreamUrl_external() {
//         EncvGoService.lastKnownPort = 2025
//         val port = EncvGoService.lastKnownPort
//         val path = "http://example.com/a.mp4"
//         val url = GoBackendModule.getStreamUrl(port, path, external = true)
//         assertTrue(url.startsWith("http://127.0.0.1:2025/api/stream/external?path="))
//     }
//
//     @Test
//     fun test_getStreamUrl_invalidPort() {
//         EncvGoService.lastKnownPort = 0
//         val port = EncvGoService.lastKnownPort
//         val url = GoBackendModule.getStreamUrl(port, "/tmp/a.mp4", external = false)
//         assertEquals("", url)
//     }
//
//     @Test
//     fun test_portState_defaults() {
//         EncvGoService.lastKnownPort = 3000
//         assertEquals(3000, EncvGoService.lastKnownPort)
//         EncvGoService.lastKnownPort = 0
//         assertEquals(0, EncvGoService.lastKnownPort)
//     }
// }
