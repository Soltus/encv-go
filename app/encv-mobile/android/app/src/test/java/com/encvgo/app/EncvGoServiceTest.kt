package com.encvgo.app

import android.content.Context
import android.content.Intent
import java.io.File
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertNotNull
import kotlin.test.assertTrue
import org.json.JSONObject
import org.junit.Before
import org.junit.runner.RunWith
import org.mockito.Mockito.*
import org.mockito.junit.MockitoJUnitRunner

@RunWith(MockitoJUnitRunner::class)
class EncvGoServiceTest {

    private lateinit var mockContext: Context

    @Before
    fun setUp() {
        mockContext = mock(Context::class.java)
        EncvGoService.isRunning = false
        EncvGoService.lastKnownPort = 0
        EncvGoService.lastError = null
    }

    @Test
    fun test_createIntent_containsCorrectAction() {
        val intent = EncvGoService.createIntent(mockContext, EncvGoService.ACTION_START, "test")
        assertEquals(EncvGoService.ACTION_START, intent.action)
    }

    @Test
    fun test_createIntent_containsSource() {
        val intent = EncvGoService.createIntent(mockContext, EncvGoService.ACTION_RESTART, "manual")
        assertEquals("manual", intent.getStringExtra(EncvGoService.EXTRA_SOURCE))
    }

    @Test
    fun test_createIntent_defaultSource() {
        val intent = EncvGoService.createIntent(mockContext, EncvGoService.ACTION_STOP)
        assertEquals("manual", intent.getStringExtra(EncvGoService.EXTRA_SOURCE))
    }

    @Test
    fun test_createIntent_allActionTypes() {
        val actions = listOf(
            EncvGoService.ACTION_START,
            EncvGoService.ACTION_STOP,
            EncvGoService.ACTION_RESTART,
            EncvGoService.ACTION_STATUS,
            EncvGoService.ACTION_EXTERNAL_START,
            EncvGoService.ACTION_EXTERNAL_RESTART
        )
        for (action in actions) {
            val intent = EncvGoService.createIntent(mockContext, action, "test")
            assertEquals(action, intent.action, "Action $action should be set correctly")
        }
    }

    @Test
    fun test_companionProperties_defaultValues() {
        assertEquals(0, EncvGoService.lastKnownPort)
        assertFalse(EncvGoService.isRunning)
    }

    @Test
    fun test_companionProperties_setAndGet() {
        EncvGoService.isRunning = true
        EncvGoService.lastKnownPort = 2025
        assertTrue(EncvGoService.isRunning)
        assertEquals(2025, EncvGoService.lastKnownPort)

        EncvGoService.isRunning = false
        EncvGoService.lastKnownPort = 0
        assertFalse(EncvGoService.isRunning)
        assertEquals(0, EncvGoService.lastKnownPort)
    }

    @Test
    fun test_companionConstants() {
        assertEquals("com.encvgo.action.START", EncvGoService.ACTION_START)
        assertEquals("com.encvgo.action.STOP", EncvGoService.ACTION_STOP)
        assertEquals("com.encvgo.action.RESTART", EncvGoService.ACTION_RESTART)
        assertEquals("com.encvgo.action.STATUS", EncvGoService.ACTION_STATUS)
        assertEquals("com.encvgo.broadcast.BACKEND_READY", EncvGoService.BROADCAST_BACKEND_READY)
        assertEquals("com.encvgo.broadcast.BACKEND_STATUS", EncvGoService.BROADCAST_BACKEND_STATUS)
        assertEquals("com.encvgo.broadcast.EXTERNAL_RESULT", EncvGoService.BROADCAST_EXTERNAL_RESULT)
    }

    @Test
    fun test_extraConstants() {
        assertEquals("port", EncvGoService.EXTRA_PORT)
        assertEquals("error", EncvGoService.EXTRA_ERROR)
        assertEquals("running", EncvGoService.EXTRA_RUNNING)
        assertEquals("source", EncvGoService.EXTRA_SOURCE)
        assertEquals("command", EncvGoService.EXTRA_COMMAND)
    }

    @Test
    fun test_defaultPort() {
        assertEquals(2025, EncvGoService.DEFAULT_PORT)
    }

    @Test
    fun test_readConfigPort_validConfig() {
        val json = JSONObject().apply {
            put("server", JSONObject().put("port", 3000))
        }
        val serverObj = json.optJSONObject("server")
        assertNotNull(serverObj)
        assertEquals(3000, serverObj.optInt("port", EncvGoService.DEFAULT_PORT))
    }

    @Test
    fun test_readConfigPort_missingServer() {
        val json = JSONObject()
        val serverObj = json.optJSONObject("server")
        assertEquals(null, serverObj)
    }

    @Test
    fun test_readConfigPort_missingPort() {
        val json = JSONObject().apply {
            put("server", JSONObject())
        }
        val serverObj = json.optJSONObject("server")
        assertNotNull(serverObj)
        assertEquals(EncvGoService.DEFAULT_PORT, serverObj.optInt("port", EncvGoService.DEFAULT_PORT))
    }

    @Test
    fun test_intentExtras_forBroadcast() {
        val intent = Intent(EncvGoService.BROADCAST_BACKEND_READY).apply {
            putExtra(EncvGoService.EXTRA_PORT, 2025)
            putExtra(EncvGoService.EXTRA_RUNNING, true)
            putExtra(EncvGoService.EXTRA_SOURCE, "manual")
            putExtra(EncvGoService.EXTRA_COMMAND, "restart")
        }
        assertEquals(2025, intent.getIntExtra(EncvGoService.EXTRA_PORT, 0))
        assertTrue(intent.getBooleanExtra(EncvGoService.EXTRA_RUNNING, false))
        assertEquals("manual", intent.getStringExtra(EncvGoService.EXTRA_SOURCE))
        assertEquals("restart", intent.getStringExtra(EncvGoService.EXTRA_COMMAND))
    }

    @Test
    fun test_intentExtras_errorBroadcast() {
        val intent = Intent(EncvGoService.BROADCAST_BACKEND_STATUS).apply {
            putExtra(EncvGoService.EXTRA_PORT, 0)
            putExtra(EncvGoService.EXTRA_RUNNING, false)
            putExtra(EncvGoService.EXTRA_ERROR, "timeout:exit=1")
        }
        assertEquals(0, intent.getIntExtra(EncvGoService.EXTRA_PORT, -1))
        assertFalse(intent.getBooleanExtra(EncvGoService.EXTRA_RUNNING, true))
        assertEquals("timeout:exit=1", intent.getStringExtra(EncvGoService.EXTRA_ERROR))
    }
}
