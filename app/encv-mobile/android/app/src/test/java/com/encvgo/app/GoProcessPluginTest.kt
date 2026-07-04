package com.encvgo.app

import android.content.Intent
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertTrue
import org.junit.Before
import org.junit.runner.RunWith
import org.mockito.junit.MockitoJUnitRunner

@RunWith(MockitoJUnitRunner::class)
class GoProcessPluginTest {

    @Before
    fun setUp() {
        EncvGoService.isRunning = false
        EncvGoService.lastKnownPort = 0
        EncvGoService.lastError = null
    }

    @Test
    fun test_serviceActions_matchExpectedValues() {
        assertEquals("com.encvgo.action.START", EncvGoService.ACTION_START)
        assertEquals("com.encvgo.action.STOP", EncvGoService.ACTION_STOP)
        assertEquals("com.encvgo.action.RESTART", EncvGoService.ACTION_RESTART)
    }

    @Test
    fun test_broadcastActions_matchExpectedValues() {
        assertEquals("com.encvgo.broadcast.BACKEND_READY", EncvGoService.BROADCAST_BACKEND_READY)
        assertEquals("com.encvgo.broadcast.BACKEND_STATUS", EncvGoService.BROADCAST_BACKEND_STATUS)
        assertEquals("com.encvgo.broadcast.EXTERNAL_RESULT", EncvGoService.BROADCAST_EXTERNAL_RESULT)
    }

    @Test
    fun test_serviceIntent_creation() {
        val mockContext = org.mockito.Mockito.mock(android.content.Context::class.java)
        val intent = EncvGoService.createIntent(mockContext, EncvGoService.ACTION_RESTART, "manual")
        intent.putExtra(EncvGoService.EXTRA_COMMAND, "restart")

        assertEquals(EncvGoService.ACTION_RESTART, intent.action)
        assertEquals("manual", intent.getStringExtra(EncvGoService.EXTRA_SOURCE))
        assertEquals("restart", intent.getStringExtra(EncvGoService.EXTRA_COMMAND))
    }

    @Test
    fun test_stopIntent_creation() {
        val mockContext = org.mockito.Mockito.mock(android.content.Context::class.java)
        val intent = EncvGoService.createIntent(mockContext, EncvGoService.ACTION_STOP, "manual")
        intent.putExtra(EncvGoService.EXTRA_COMMAND, "stop")

        assertEquals(EncvGoService.ACTION_STOP, intent.action)
        assertEquals("stop", intent.getStringExtra(EncvGoService.EXTRA_COMMAND))
    }

    @Test
    fun test_pendingCallResolution_runningBackend() {
        val intent = Intent(EncvGoService.BROADCAST_BACKEND_READY).apply {
            putExtra(EncvGoService.EXTRA_PORT, 2025)
            putExtra(EncvGoService.EXTRA_RUNNING, true)
            putExtra(EncvGoService.EXTRA_COMMAND, "restart")
        }

        val running = intent.getBooleanExtra(EncvGoService.EXTRA_RUNNING, false)
        val port = intent.getIntExtra(EncvGoService.EXTRA_PORT, 0)
        val command = intent.getStringExtra(EncvGoService.EXTRA_COMMAND)

        assertTrue(running)
        assertEquals(2025, port)
        assertEquals("restart", command)
    }

    @Test
    fun test_pendingCallResolution_failedBackend() {
        val intent = Intent(EncvGoService.BROADCAST_BACKEND_STATUS).apply {
            putExtra(EncvGoService.EXTRA_PORT, 0)
            putExtra(EncvGoService.EXTRA_RUNNING, false)
            putExtra(EncvGoService.EXTRA_ERROR, "timeout:exit=1")
            putExtra(EncvGoService.EXTRA_COMMAND, "restart")
        }

        val running = intent.getBooleanExtra(EncvGoService.EXTRA_RUNNING, true)
        val error = intent.getStringExtra(EncvGoService.EXTRA_ERROR)

        assertFalse(running)
        assertEquals("timeout:exit=1", error)
    }

    @Test
    fun test_configPortParsing_validJson() {
        val json = org.json.JSONObject().apply {
            put("server", org.json.JSONObject().put("port", 3000))
        }
        val port = json.optJSONObject("server")?.optInt("port", EncvGoService.DEFAULT_PORT)
            ?: EncvGoService.DEFAULT_PORT
        assertEquals(3000, port)
    }

    @Test
    fun test_configPortParsing_missingPort() {
        val json = org.json.JSONObject().apply {
            put("server", org.json.JSONObject())
        }
        val port = json.optJSONObject("server")?.optInt("port", EncvGoService.DEFAULT_PORT)
            ?: EncvGoService.DEFAULT_PORT
        assertEquals(EncvGoService.DEFAULT_PORT, port)
    }

    @Test
    fun test_configPortParsing_missingServer() {
        val json = org.json.JSONObject()
        val serverObj = json.optJSONObject("server")
        val port = serverObj?.optInt("port", EncvGoService.DEFAULT_PORT)
            ?: EncvGoService.DEFAULT_PORT
        assertEquals(EncvGoService.DEFAULT_PORT, port)
    }

    @Test
    fun test_configPortParsing_invalidJson() {
        val json = org.json.JSONObject()
        val port = json.optJSONObject("server")?.optInt("port", EncvGoService.DEFAULT_PORT)
            ?: EncvGoService.DEFAULT_PORT
        assertEquals(EncvGoService.DEFAULT_PORT, port)
    }

    @Test
    fun test_externalResultIntent_success() {
        val intent = Intent(EncvGoService.BROADCAST_EXTERNAL_RESULT).apply {
            putExtra("success", true)
            putExtra(EncvGoService.EXTRA_PORT, 2025)
            putExtra(EncvGoService.EXTRA_RUNNING, true)
            putExtra(EncvGoService.EXTRA_SOURCE, "external")
            putExtra(EncvGoService.EXTRA_COMMAND, "start")
        }

        assertTrue(intent.getBooleanExtra("success", false))
        assertEquals(2025, intent.getIntExtra(EncvGoService.EXTRA_PORT, 0))
        assertTrue(intent.getBooleanExtra(EncvGoService.EXTRA_RUNNING, false))
        assertEquals("external", intent.getStringExtra(EncvGoService.EXTRA_SOURCE))
    }

    @Test
    fun test_externalResultIntent_failure() {
        val intent = Intent(EncvGoService.BROADCAST_EXTERNAL_RESULT).apply {
            putExtra("success", false)
            putExtra(EncvGoService.EXTRA_PORT, 0)
            putExtra(EncvGoService.EXTRA_RUNNING, false)
            putExtra(EncvGoService.EXTRA_ERROR, "no_binary")
        }

        assertFalse(intent.getBooleanExtra("success", true))
        assertEquals(0, intent.getIntExtra(EncvGoService.EXTRA_PORT, -1))
        assertFalse(intent.getBooleanExtra(EncvGoService.EXTRA_RUNNING, true))
        assertEquals("no_binary", intent.getStringExtra(EncvGoService.EXTRA_ERROR))
    }
}
