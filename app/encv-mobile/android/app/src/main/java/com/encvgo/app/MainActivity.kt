package com.encvgo.app

import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import android.content.IntentFilter
import android.net.Uri
import android.os.Build
import android.os.Bundle
import android.os.PowerManager
import android.util.Log
import android.webkit.WebSettings
import androidx.core.content.ContextCompat
import androidx.lifecycle.lifecycleScope
import com.getcapacitor.BridgeActivity
import kotlinx.coroutines.launch
import org.json.JSONObject

import com.encvgo.app.GoProcessPlugin
import com.encvgo.app.ApiProxyPlugin
import com.masterpedidos.highrefreshrate.HighRefreshRatePlugin

class MainActivity : BridgeActivity() {
    companion object {
        private const val TAG = "ENCV-go"
        private const val MPV_PLUGIN_ID = "com.encvgo.plugin.mpv"
    }

    private var backendReceiverRegistered = false

    private val backendReceiver = object : BroadcastReceiver() {
        override fun onReceive(context: Context?, intent: Intent?) {
            when (intent?.action) {
                EncvGoService.BROADCAST_BACKEND_READY,
                EncvGoService.BROADCAST_BACKEND_STATUS -> {
                    val port = intent.getIntExtra(EncvGoService.EXTRA_PORT, 0)
                    val error = intent.getStringExtra(EncvGoService.EXTRA_ERROR)
                    val running = intent.getBooleanExtra(EncvGoService.EXTRA_RUNNING, false)
                    val source = intent.getStringExtra(EncvGoService.EXTRA_SOURCE)
                    val command = intent.getStringExtra(EncvGoService.EXTRA_COMMAND)
                    notifyFrontend(port, running, error, source, command)
                }
            }
        }
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        try {
            registerPlugin(GoProcessPlugin::class.java)
            registerPlugin(ApiProxyPlugin::class.java)
            registerPlugin(HighRefreshRatePlugin::class.java)
            registerPlugin(SimVersePlugin::class.java)
        } catch (e: Exception) {
            Log.e(TAG, "registerPlugin failed", e)
        }
        super.onCreate(savedInstanceState)
        // 初始化 Kotlin 层 DevLog 桥接器（必须在 super.onCreate 之后，context 已可用）
        KotlinDevLogBridge.init(this)
        KotlinDevLogBridge.logInfo(TAG, "MainActivity created, KotlinDevLogBridge initialized")
        bridge.webView.settings.mixedContentMode = WebSettings.MIXED_CONTENT_ALWAYS_ALLOW
        Log.i(TAG, "WebView mixedContentMode set to MIXED_CONTENT_ALWAYS_ALLOW")
        registerBackendReceiver()
        requestBatteryOptimizationExemption()
        val handled = handleIntent(intent)
        if (!handled) {
            startBackendService(EncvGoService.ACTION_START, "app", null)
        }
        loadPlugins()
    }

    override fun onNewIntent(intent: Intent) {
        super.onNewIntent(intent)
        setIntent(intent)
        handleIntent(intent)
    }

    private fun loadPlugins() {
        lifecycleScope.launch {
            Log.i(TAG, "Plugin loading deferred to frontend-driven flow")
        }
    }

    override fun onDestroy() {
        if (backendReceiverRegistered) {
            unregisterReceiver(backendReceiver)
            backendReceiverRegistered = false
        }
        super.onDestroy()
    }

    override fun onBackPressed() {
        @Suppress("DEPRECATION")
        super.onBackPressed()
    }

    private fun registerBackendReceiver() {
        if (backendReceiverRegistered) return
        val filter = IntentFilter().apply {
            addAction(EncvGoService.BROADCAST_BACKEND_READY)
            addAction(EncvGoService.BROADCAST_BACKEND_STATUS)
        }
        if (Build.VERSION.SDK_INT >= 33) {
            registerReceiver(backendReceiver, filter, RECEIVER_NOT_EXPORTED)
        } else {
            @Suppress("DEPRECATION")
            registerReceiver(backendReceiver, filter)
        }
        backendReceiverRegistered = true
    }

    private fun handleIntent(intent: Intent?): Boolean {
        if (intent == null) return false
        val source = when {
            intent.data != null -> "scheme"
            intent.action?.startsWith("com.encvgo.action.") == true -> "intent"
            else -> "app"
        }

        val command = resolveExternalCommand(intent)
        when (command) {
            EncvGoService.ACTION_EXTERNAL_RESTART -> startBackendService(command, source, "restart")
            EncvGoService.ACTION_STOP -> startBackendService(command, source, "stop")
            EncvGoService.ACTION_STATUS -> startBackendService(command, source, "status")
            EncvGoService.ACTION_EXTERNAL_START -> {
                if (source == "app") {
                    return false
                }
                startBackendService(command, source, "start")
            }
        }
        return true
    }

    private fun resolveExternalCommand(intent: Intent): String {
        val action = intent.action
        if (action == EncvGoService.ACTION_RESTART) return EncvGoService.ACTION_EXTERNAL_RESTART
        if (action == EncvGoService.ACTION_STOP) return EncvGoService.ACTION_STOP
        if (action == EncvGoService.ACTION_STATUS) return EncvGoService.ACTION_STATUS
        if (action == EncvGoService.ACTION_START) return EncvGoService.ACTION_EXTERNAL_START

        val uri: Uri = intent.data ?: return EncvGoService.ACTION_EXTERNAL_START
        return when (uri.host?.lowercase()) {
            "restart" -> EncvGoService.ACTION_EXTERNAL_RESTART
            "stop" -> EncvGoService.ACTION_STOP
            "status" -> EncvGoService.ACTION_STATUS
            else -> EncvGoService.ACTION_EXTERNAL_START
        }
    }

    private fun startBackendService(action: String, source: String, command: String?) {
        val serviceIntent = EncvGoService.createIntent(this, action, source).apply {
            if (!command.isNullOrEmpty()) {
                putExtra(EncvGoService.EXTRA_COMMAND, command)
            }
        }
        ContextCompat.startForegroundService(this, serviceIntent)
    }

    private fun notifyFrontend(port: Int, running: Boolean, error: String?, source: String?, command: String?) {
        runOnUiThread {
            try {
                val detail = JSONObject().apply {
                    put("port", port)
                    put("running", running)
                    if (error != null) put("error", error)
                    if (source != null) put("source", source)
                    if (command != null) put("command", command)
                }
                val readyEvent = "window.dispatchEvent(new CustomEvent('encv:backend-ready',{detail:${detail}}))"
                val statusEvent = "window.dispatchEvent(new CustomEvent('encv:backend-status',{detail:${detail}}))"
                bridge.webView.evaluateJavascript(readyEvent, null)
                bridge.webView.evaluateJavascript(statusEvent, null)
            } catch (e: Exception) {
                Log.w(TAG, "Failed to notify frontend", e)
            }
        }
    }

    private fun requestBatteryOptimizationExemption() {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.M) {
            val pm = getSystemService(Context.POWER_SERVICE) as PowerManager
            if (!pm.isIgnoringBatteryOptimizations(packageName)) {
                try {
                    val intent = Intent(
                        android.provider.Settings.ACTION_REQUEST_IGNORE_BATTERY_OPTIMIZATIONS
                    ).apply {
                        data = Uri.parse("package:$packageName")
                    }
                    startActivity(intent)
                } catch (e: Exception) {
                    Log.w(TAG, "Failed to request battery optimization exemption", e)
                }
            }
        }
    }
}
