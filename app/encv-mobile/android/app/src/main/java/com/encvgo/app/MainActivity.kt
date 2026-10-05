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
import java.io.File
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
        // 前台时取云控重载指令的间隔（vNext Round 9）
        private const val RELOAD_POLL_INTERVAL_MS = 3000L
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
            // 2026-10-04：扫码改为自建 ZXingLite 插件（去掉 Google Play 服务依赖）
            registerPlugin(BarcodeScannerPlugin::class.java)
        } catch (e: Exception) {
            Log.e(TAG, "registerPlugin failed", e)
        }
        super.onCreate(savedInstanceState)
        bridge.webView.settings.mixedContentMode = WebSettings.MIXED_CONTENT_ALWAYS_ALLOW
        Log.i(TAG, "WebView mixedContentMode set to MIXED_CONTENT_ALWAYS_ALLOW")
        // 🆕 I4（2026-10-05）：主应用 SPA 热更新 —— 有热更包就让 Capacitor 本地服务改指向它
        applyHotWebBundleIfPresent()
        registerBackendReceiver()
        // vNext Round 7：取云控待执行的重载指令（activity/app 级 here，web 级由 Go 广播）
        checkPendingReload()
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

    // ── I4：主应用 SPA 热更新（2026-10-05）──────────────────────────────────
    //
    // 机制：`bridge.setServerBasePath(path)` → Capacitor 的 `WebViewLocalServer.hostFiles(path)`
    //  ⇒ 把**任意应用私有目录**托管在同一个 `https://localhost` 源下。
    //
    // 为什么必须走这条路（而不是 webView.loadUrl("http://127.0.0.1:<port>/web/")）：
    //   · origin 保持 https://localhost ⇒ CORS / baseUrl 判定 / 混合内容策略全部不变；
    //   · Capacitor 的插件桥注入与 WEBVIEW_SERVER_URL 都绑这个本地源 ⇒ 换成别的源会让
    //     GoProcess / BarcodeScanner / Filesystem 等插件直接失效（主应用离不开插件）。
    //   （PreviewAssetsActivity 之所以能用独立 WebView，是因为那一页不需要任何插件。）
    //
    // 安全与回滚：
    //   · 必须同时有 index.html **和** version.json（version.json 由云控通道写入，
    //     说明这个目录是热更放的，不是别的什么东西）；
    //   · 目录里有 .disabled 标记 ⇒ 跳过（给"热更包有问题"留一条不用重新打 APK 的退路）；
    //   · 判定不过 = 什么都不做 ⇒ Capacitor 照旧用 APK 内 assets/public。
    private fun applyHotWebBundleIfPresent() {
        val dir = File(filesDir, ".encv${File.separator}web-bundle")
        val version = File(dir, "version.json")
        if (File(dir, "index.html").exists() && version.exists()) {
            if (File(dir, ".disabled").exists()) {
                Log.i(TAG, "hot web bundle disabled by marker, using bundled assets")
                return
            }
            try {
                bridge.setServerBasePath(dir.absolutePath)
                Log.i(TAG, "hot web bundle active: ${dir.absolutePath} version=${version.readText().take(200)}")
                GoProcessPlugin.pushKotlinLog("info", TAG, "已加载热更新 SPA 包")
            } catch (e: Exception) {
                Log.w(TAG, "failed to switch to hot web bundle, falling back to bundled assets", e)
            }
        } else {
            Log.i(TAG, "no hot web bundle, using bundled assets")
        }
    }

    // ── vNext Round 9（2026-10-06）：重载指令的**即时**轮询 ────────────────
    //
    // ⚠️ 为什么不能只在 onCreate 取一次：
    //    Round 7 只在 onCreate 调 checkPendingReload ⇒ App 停在前台时，
    //    云控下发 reload 后 Kotlin **根本不会去取**，只能等用户自己重开 App
    //    ⇒ "云控重启"退化成"还是要人手重启"，等于没用。
    //
    // 修法：App 在**前台**时按固定间隔轮询 /api/reload/pending（3s），
    //       取到就执行（activity → recreate，app → 重启进程）并 ack。
    //       退到后台就停，避免无谓请求。
    private val reloadPollHandler = android.os.Handler(android.os.Looper.getMainLooper())
    private var reloadPolling = false

    private val reloadPollRunnable = object : Runnable {
        override fun run() {
            checkPendingReload()
            if (reloadPolling) {
                reloadPollHandler.postDelayed(this, RELOAD_POLL_INTERVAL_MS)
            }
        }
    }

    private fun startReloadPolling() {
        if (reloadPolling) return
        reloadPolling = true
        reloadPollHandler.postDelayed(reloadPollRunnable, RELOAD_POLL_INTERVAL_MS)
    }

    private fun stopReloadPolling() {
        reloadPolling = false
        reloadPollHandler.removeCallbacks(reloadPollRunnable)
    }

    override fun onResume() {
        super.onResume()
        // 前台 ⇒ 开始取云控指令（远端发 reload 后这里会立刻执行，不需要用户操作）
        startReloadPolling()
        checkPendingReload()
    }

    override fun onPause() {
        stopReloadPolling()
        super.onPause()
    }

    // ── vNext Round 7（2026-10-06）：云控三级重载 ──────────────────────────
    //
    // 背景：热更包应用后此前**只能手动重启 App** 才生效
    // （applyHotWebBundleIfPresent 只在启动时判定）⇒ 推完还要人手点一次，
    // 既慢又容易被误认为"没生效"。
    //
    // 分工（别搞反）：
    //   · Go 能直接做 web 级 —— 广播 WS 让前端 reload，当场生效
    //   · activity / app 只能由原生做 —— Go 把指令落在 /api/reload/pending，
    //     这里取走执行后 ack（不依赖文件路径约定，避免两端拼路径拼错）
    //
    // 三级：web → WebView.reload()；activity → recreate()；app → 重启进程。
    private fun checkPendingReload() {
        lifecycleScope.launch(kotlinx.coroutines.Dispatchers.IO) {
            try {
                val url = java.net.URL("http://127.0.0.1:2025/api/reload/pending")
                val conn = url.openConnection() as java.net.HttpURLConnection
                conn.connectTimeout = 1500
                conn.readTimeout = 1500
                val body = conn.inputStream.bufferedReader().readText()
                conn.disconnect()
                val level = org.json.JSONObject(body).optString("level", "")
                if (level.isBlank()) return@launch
                Log.i(TAG, "pending reload: level=$level")
                kotlinx.coroutines.withContext(kotlinx.coroutines.Dispatchers.Main) {
                    when (level) {
                        "web" -> bridge.webView.reload()
                        "activity" -> recreate()
                        "app" -> restartApp()
                    }
                }
                ackReload()
            } catch (e: Exception) {
                // 后端还没起来 / 没有 pending：都是正常情况，不刷错误日志
                Log.d(TAG, "checkPendingReload: ${e.message}")
            }
        }
    }

    /** 回执：告诉 Go 指令已执行，清掉 pending（否则会反复重启） */
    private fun ackReload() {
        try {
            val conn = java.net.URL("http://127.0.0.1:2025/api/reload/ack")
                .openConnection() as java.net.HttpURLConnection
            conn.requestMethod = "POST"
            conn.connectTimeout = 1500
            conn.readTimeout = 1500
            conn.responseCode
            conn.disconnect()
        } catch (e: Exception) {
            Log.d(TAG, "ackReload: ${e.message}")
        }
    }

    /** app 级：整个进程重启（换执行体 / 换 Go 二进制时用） */
    private fun restartApp() {
        val intent = packageManager.getLaunchIntentForPackage(packageName)
        intent?.addFlags(Intent.FLAG_ACTIVITY_CLEAR_TOP or Intent.FLAG_ACTIVITY_NEW_TASK)
        if (intent != null) {
            startActivity(intent)
        }
        android.os.Process.killProcess(android.os.Process.myPid())
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
