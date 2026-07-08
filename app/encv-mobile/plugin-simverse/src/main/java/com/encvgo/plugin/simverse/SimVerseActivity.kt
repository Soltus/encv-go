package com.encvgo.plugin.simverse

import android.app.AlertDialog
import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import android.content.IntentFilter
import android.os.Build
import android.os.Bundle
import android.view.Gravity
import android.view.ViewGroup
import android.webkit.WebView
import android.widget.Button
import android.widget.FrameLayout
import android.widget.ScrollView
import android.widget.TextView
import androidx.activity.compose.setContent
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.viewinterop.AndroidView
import com.combo.core.component.activity.BasePluginActivity
import java.net.HttpURLConnection
import java.net.URL

class SimVerseActivity : BasePluginActivity() {

    companion object {
        private const val TAG = "SimVerseActivity"
        private const val SAT_TAG = "SimVerse-SAT"

        private const val ACTION_BACKEND_READY = "com.encvgo.broadcast.BACKEND_READY"
        private const val ACTION_BACKEND_STATUS = "com.encvgo.broadcast.BACKEND_STATUS"
        private const val EXTRA_PORT = "port"
        private const val EXTRA_RUNNING = "running"
        private const val EXTRA_SOURCE = "source"
        private const val ACTION_START = "com.encvgo.action.START"

        private const val HOST_PACKAGE = "com.encvgo.app"
        private const val HOST_SERVICE_CLASS = "com.encvgo.app.EncvGoService"
    }

    private var webView: WebView? = null
    private var backendPort: Int = 0
    private var backendReady: Boolean = false
    private var receiverRegistered: Boolean = false

    private val backendReceiver = object : BroadcastReceiver() {
        override fun onReceive(context: Context?, intent: Intent?) {
            when (intent?.action) {
                ACTION_BACKEND_READY,
                ACTION_BACKEND_STATUS -> {
                    val port = intent.getIntExtra(EXTRA_PORT, 0)
                    val running = intent.getBooleanExtra(EXTRA_RUNNING, false)
                    android.util.Log.i(TAG, "backendReceiver: action=${intent.action}, port=$port, running=$running")
                    if (running && port > 0) {
                        backendPort = port
                        backendReady = true
                        (webView?.webViewClient as? SimVerseWebViewClient)?.backendPort = port
                        injectApiBase(port)
                    }
                }
            }
        }
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)

        val host = proxyActivity ?: return
        val hostIntent = host.intent ?: return

        val worldId = hostIntent.getStringExtra("world_id") ?: "default"
        val worldName = hostIntent.getStringExtra("world_name") ?: "Default"
        val apiBaseUrl = hostIntent.getStringExtra("api_base_url") ?: ""

        android.util.Log.e(TAG, "onCreate: worldId=$worldId worldName=$worldName apiBaseUrl=$apiBaseUrl")

        registerBackendReceiver()
        ensureBackendRunning()

        host.setContent {
            var webViewRef by remember { mutableStateOf<WebView?>(null) }

            androidx.compose.foundation.layout.Box(
                modifier = Modifier
                    .fillMaxSize()
                    .background(Color.Black)
            ) {
                AndroidView(
                    modifier = Modifier.fillMaxSize(),
                    factory = { ctx ->
                        WebView(ctx).apply {
                            layoutParams = ViewGroup.LayoutParams(
                                ViewGroup.LayoutParams.MATCH_PARENT,
                                ViewGroup.LayoutParams.MATCH_PARENT
                            )
                            id = "simverse-activity-embed".hashCode()
                            val client = SimVerseWebViewClient(ctx.applicationContext)
                            if (backendPort > 0) {
                                client.backendPort = backendPort
                            }
                            webViewClient = client
                            webChromeClient = SimVerseWebChromeClient(client.diagState)
                            settings.apply {
                                javaScriptEnabled = true
                                domStorageEnabled = true
                                allowFileAccess = false
                                allowContentAccess = false
                                databaseEnabled = true
                                cacheMode = android.webkit.WebSettings.LOAD_DEFAULT
                                useWideViewPort = true
                                loadWithOverviewMode = true
                                mixedContentMode = android.webkit.WebSettings.MIXED_CONTENT_ALWAYS_ALLOW
                            }
                            addJavascriptInterface(
                                SimVersePluginJSInterface(ctx.applicationContext),
                                "SimVerseNative"
                            )
                            SimVerseWebViewDiagnostic.capture(client)
                            loadUrl("https://simverse-plugin.local/")
                            webViewRef = this
                            webView = this
                        }
                    },
                    update = { wv ->
                        webView = wv
                    }
                )

                AndroidView(
                    modifier = Modifier.fillMaxSize(),
                    factory = { ctx ->
                        FrameLayout(ctx).apply {
                            val diagBtn = Button(ctx).apply {
                                text = "🔍"
                                textSize = 18f
                                setBackgroundColor(0x66000000.toInt())
                                setTextColor(0xFFFFFFFF.toInt())
                                val size = dpToPx(ctx, 48)
                                val params = FrameLayout.LayoutParams(size, size).apply {
                                    gravity = Gravity.TOP or Gravity.END
                                    setMargins(0, dpToPx(ctx, 8), dpToPx(ctx, 8), 0)
                                }
                                layoutParams = params
                                alpha = 0.3f
                                setOnClickListener {
                                    showDiagnosticDialog()
                                }
                                setOnLongClickListener {
                                    showDiagnosticDialog()
                                    true
                                }
                            }
                            addView(diagBtn)
                        }
                    }
                )
            }
        }
    }

    private fun ensureBackendRunning() {
        val ctx = proxyActivity ?: return
        val existingPort = probeBackendPort()
        if (existingPort > 0) {
            android.util.Log.i(TAG, "ensureBackendRunning: backend already running on port $existingPort")
            backendPort = existingPort
            backendReady = true
            return
        }

        android.util.Log.i(TAG, "ensureBackendRunning: backend not running, starting service...")
        try {
            val intent = Intent().apply {
                setClassName(HOST_PACKAGE, HOST_SERVICE_CLASS)
                action = ACTION_START
                putExtra(EXTRA_SOURCE, "simverse")
            }
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
                ctx.startForegroundService(intent)
            } else {
                ctx.startService(intent)
            }
            android.util.Log.i(TAG, "ensureBackendRunning: service start intent sent")
        } catch (e: Exception) {
            android.util.Log.e(TAG, "ensureBackendRunning: failed to start service", e)
        }
    }

    private fun probeBackendPort(): Int {
        val ctx = proxyActivity ?: return 0
        val prefs = ctx.getSharedPreferences("encv_go_prefs", Context.MODE_PRIVATE)
        val savedPort = prefs.getInt("server_port", 0)
        if (savedPort > 0 && isPortAlive(savedPort)) {
            return savedPort
        }
        for (port in 2025..2035) {
            if (isPortAlive(port)) {
                return port
            }
        }
        return 0
    }

    private fun isPortAlive(port: Int): Boolean {
        return try {
            val url = URL("http://127.0.0.1:$port/health")
            val conn = url.openConnection() as HttpURLConnection
            conn.connectTimeout = 500
            conn.readTimeout = 500
            conn.requestMethod = "GET"
            val code = conn.responseCode
            conn.disconnect()
            code in 200..399
        } catch (_: Exception) {
            false
        }
    }

    private fun registerBackendReceiver() {
        if (receiverRegistered) return
        val ctx = proxyActivity ?: return
        try {
            val filter = IntentFilter().apply {
                addAction(ACTION_BACKEND_READY)
                addAction(ACTION_BACKEND_STATUS)
            }
            if (Build.VERSION.SDK_INT >= 33) {
                ctx.registerReceiver(backendReceiver, filter, Context.RECEIVER_EXPORTED)
            } else {
                @Suppress("DEPRECATION")
                ctx.registerReceiver(backendReceiver, filter)
            }
            receiverRegistered = true
            android.util.Log.d(TAG, "registerBackendReceiver: registered")
        } catch (e: Exception) {
            android.util.Log.e(TAG, "registerBackendReceiver: failed", e)
        }
    }

    private fun unregisterBackendReceiver() {
        if (!receiverRegistered) return
        val ctx = proxyActivity ?: return
        try {
            ctx.unregisterReceiver(backendReceiver)
            receiverRegistered = false
            android.util.Log.d(TAG, "unregisterBackendReceiver: unregistered")
        } catch (e: Exception) {
            android.util.Log.w(TAG, "unregisterBackendReceiver: failed", e)
        }
    }

    private fun injectApiBase(port: Int) {
        val wv = webView ?: run {
            android.util.Log.w(TAG, "injectApiBase: webView is null, skip")
            return
        }
        val script = """
            (function() {
                window.__ENCV_API_BASE__ = 'http://127.0.0.1:$port';
                window.__ENCV_PORT__ = $port;
                console.log('[SimVerse-Activity] API base set to http://127.0.0.1:$port');
                if (window.dispatchEvent) {
                    window.dispatchEvent(new CustomEvent('encv:api-base-ready', {detail: {port: $port}}));
                }
            })();
        """.trimIndent()
        wv.post {
            wv.evaluateJavascript(script, null)
        }
        android.util.Log.d(TAG, "injectApiBase: port=$port, script dispatched")
    }

    private fun dpToPx(ctx: android.content.Context, dp: Int): Int {
        val density = ctx.resources.displayMetrics.density
        return (dp * density).toInt()
    }

    private fun showDiagnosticDialog() {
        val ctx = proxyActivity ?: return
        val report = buildFullDiagnosticReport()

        val scrollView = ScrollView(ctx).apply {
            setPadding(dpToPx(ctx, 16), dpToPx(ctx, 16), dpToPx(ctx, 16), dpToPx(ctx, 16))
            val textView = TextView(ctx).apply {
                text = report
                textSize = 11f
                typeface = android.graphics.Typeface.MONOSPACE
                setTextColor(0xFFE0E0E0.toInt())
            }
            addView(textView)
        }

        AlertDialog.Builder(ctx)
            .setTitle("🌍 SimVerse 全链路诊断")
            .setView(scrollView)
            .setPositiveButton("复制") { _, _ ->
                val clipboard = ctx.getSystemService(android.content.Context.CLIPBOARD_SERVICE) as android.content.ClipboardManager
                clipboard.setPrimaryClip(android.content.ClipData.newPlainText("SimVerse Diagnostic", report))
            }
            .setNegativeButton("关闭", null)
            .show()
    }

    private fun buildFullDiagnosticReport(): String {
        val lines = mutableListOf<String>()
        lines.add("═══ SimVerse 全链路诊断 ═══")
        lines.add("")
        lines.add("═══ 后端状态 ═══")
        lines.add("   backendReady = $backendReady")
        lines.add("   backendPort = $backendPort")
        if (backendPort > 0) {
            lines.add("   portAlive = ${isPortAlive(backendPort)}")
        }
        lines.add("")
        lines.add("═══ WebView ═══")
        lines.add("   url = ${webView?.url ?: "(null)"}")
        lines.add("")
        lines.add(SimVerseWebViewDiagnostic.getReport())
        return lines.joinToString("\n")
    }

    override fun onDestroy() {
        android.util.Log.e(TAG, "onDestroy")
        unregisterBackendReceiver()
        webView = null
        super.onDestroy()
    }
}
