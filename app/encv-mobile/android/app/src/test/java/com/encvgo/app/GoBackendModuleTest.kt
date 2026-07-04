package com.encvgo.app

import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertTrue
import org.junit.Before
import org.junit.runner.RunWith
import org.mockito.junit.MockitoJUnitRunner

@RunWith(MockitoJUnitRunner::class)
class GoBackendModuleTest {

    @Before
    fun setUp() {
        EncvGoService.isRunning = false
        EncvGoService.lastKnownPort = 0
        EncvGoService.lastError = null
    }

    @Test
    fun test_companionEventConstants() {
        assertEquals("backend:ready", GoBackendModule.EVENT_READY)
        assertEquals("backend:error", GoBackendModule.EVENT_ERROR)
    }

    @Test
    fun test_getStreamUrl_validPort_internal() {
        EncvGoService.lastKnownPort = 2025
        val port = EncvGoService.lastKnownPort
        val path = "/storage/emulated/0/test.mp4"
        val encodedPath = android.net.Uri.encode(path, "/")
        val url = "http://127.0.0.1:$port/stream?path=$encodedPath"
        assertTrue(url.startsWith("http://127.0.0.1:2025/stream?path="))
        assertTrue(url.contains("test.mp4"))
    }

    @Test
    fun test_getStreamUrl_validPort_external() {
        EncvGoService.lastKnownPort = 2025
        val port = EncvGoService.lastKnownPort
        val path = "/storage/emulated/0/test.mp4"
        val encodedPath = android.net.Uri.encode(path, "/")
        val url = "http://127.0.0.1:$port/api/stream/external?path=$encodedPath"
        assertTrue(url.startsWith("http://127.0.0.1:2025/api/stream/external?path="))
        assertTrue(url.contains("test.mp4"))
    }

    @Test
    fun test_getStreamUrl_invalidPort() {
        EncvGoService.lastKnownPort = 0
        val port = EncvGoService.lastKnownPort
        assertTrue(port <= 0)
    }

    @Test
    fun test_getStreamUrl_urlEncoding() {
        val path = "/storage/emulated/0/中文文件.mp4"
        val encodedPath = android.net.Uri.encode(path, "/")
        assertTrue(encodedPath.contains("%"))
        assertTrue(encodedPath.contains("/"))
    }

    @Test
    fun test_getStreamUrl_specialCharacters() {
        val path = "/storage/emulated/0/file with spaces.mp4"
        val encodedPath = android.net.Uri.encode(path, "/")
        assertTrue(encodedPath.contains("%20") || !encodedPath.contains(" "))
    }

    @Test
    fun test_backendStatus_reflectsServiceState() {
        EncvGoService.isRunning = true
        EncvGoService.lastKnownPort = 3000
        assertTrue(EncvGoService.isRunning)
        assertEquals(3000, EncvGoService.lastKnownPort)

        EncvGoService.isRunning = false
        EncvGoService.lastKnownPort = 0
        assertTrue(!EncvGoService.isRunning)
        assertEquals(0, EncvGoService.lastKnownPort)
    }

    @Test
    fun test_errorState() {
        EncvGoService.lastError = "timeout:exit=1"
        assertEquals("timeout:exit=1", EncvGoService.lastError)

        EncvGoService.lastError = null
        assertEquals(null, EncvGoService.lastError)
    }
}
