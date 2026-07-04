package com.encvgo.app

import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import android.content.IntentFilter
import android.content.pm.ActivityInfo
import android.os.Build
import android.os.Bundle
import android.util.Log
import android.view.View
import android.webkit.WebResourceRequest
import android.webkit.WebResourceResponse
import android.webkit.WebView
import android.webkit.WebSettings
import android.webkit.WebViewClient
import androidx.core.content.ContextCompat
import androidx.core.view.WindowCompat
import androidx.core.view.WindowInsetsCompat
import androidx.core.view.WindowInsetsControllerCompat
import com.getcapacitor.BridgeActivity

class WorldActivity : BridgeActivity() {
    companion object {
        private const val TAG = "WorldActivity"
        private const val WORLD_URL = "https://localhost/simverse-world.html"
        private const val EXTRA_WORLD_ID = "world_id"
        private const val EXTRA_WORLD_NAME = "world_name"

        fun createIntent(context: Context, worldId: String = "default", worldName: String = "Default"): Intent {
            return Intent(context, WorldActivity::class.java).apply {
                putExtra(EXTRA_WORLD_ID, worldId)
                putExtra(EXTRA_WORLD_NAME, worldName)
            }
        }
    }

    private var backendReceiverRegistered = false
    private var immersiveMode = true

    private val backendReceiver = object : BroadcastReceiver() {
        override fun onReceive(context: Context?, intent: Intent?) {
            when (intent?.action) {
                EncvGoService.BROADCAST_BACKEND_READY,
                EncvGoService.BROADCAST_BACKEND_STATUS -> {
                    val port = intent.getIntExtra(EncvGoService.EXTRA_PORT, 0)
                    val running = intent.getBooleanExtra(EncvGoService.EXTRA_RUNNING, false)
                    val error = intent.getStringExtra(EncvGoService.EXTRA_ERROR)
                    Log.d(TAG, "backendReceiver: port=$port, running=$running, error=$error")
                    notifyFrontend(port, running, error)
                }
            }
        }
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        Log.i(TAG, "onCreate")
        super.onCreate(savedInstanceState)

        setupImmersiveMode()
        setupLandscape()

        try {
            registerPlugin(GoProcessPlugin::class.java)
            registerPlugin(SimVersePlugin::class.java)
            Log.d(TAG, "plugins registered")
        } catch (e: Exception) {
            Log.e(TAG, "registerPlugin failed", e)
        }

        registerBackendReceiver()
        loadWorldPage()
    }

    private fun setupLandscape() {
        requestedOrientation = ActivityInfo.SCREEN_ORIENTATION_SENSOR_LANDSCAPE
        Log.i(TAG, "setupLandscape: sensorLandscape")
    }

    private fun setupImmersiveMode() {
        WindowCompat.setDecorFitsSystemWindows(window, false)
        window.statusBarColor = android.graphics.Color.TRANSPARENT
        window.navigationBarColor = android.graphics.Color.TRANSPARENT

        hideSystemBars()
    }

    private fun hideSystemBars() {
        val controller = WindowInsetsControllerCompat(window, window.decorView)
        controller.hide(WindowInsetsCompat.Type.systemBars())
        controller.systemBarsBehavior =
            WindowInsetsControllerCompat.BEHAVIOR_SHOW_TRANSIENT_BARS_BY_SWIPE
        immersiveMode = true
    }

    private fun showSystemBars() {
        val controller = WindowInsetsControllerCompat(window, window.decorView)
        controller.show(WindowInsetsCompat.Type.systemBars())
        immersiveMode = false
    }

    private fun toggleSystemBars() {
        if (immersiveMode) {
            showSystemBars()
        } else {
            hideSystemBars()
        }
    }

    private fun registerBackendReceiver() {
        if (backendReceiverRegistered) return
        val filter = IntentFilter().apply {
            addAction(EncvGoService.BROADCAST_BACKEND_READY)
            addAction(EncvGoService.BROADCAST_BACKEND_STATUS)
        }
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU) {
            registerReceiver(backendReceiver, filter, Context.RECEIVER_NOT_EXPORTED)
        } else {
            @Suppress("UnspecifiedRegisterReceiverFlag")
            registerReceiver(backendReceiver, filter)
        }
        backendReceiverRegistered = true
        Log.i(TAG, "backend receiver registered")
    }

    private fun loadWorldPage() {
        val webView = bridge?.webView
        if (webView == null) {
            Log.e(TAG, "loadWorldPage: webView is null!")
            return
        }

        webView.settings.javaScriptEnabled = true
        webView.settings.domStorageEnabled = true
        webView.settings.databaseEnabled = true
        webView.settings.allowFileAccess = true
        webView.settings.mixedContentMode = WebSettings.MIXED_CONTENT_ALWAYS_ALLOW

        val originalClient = webView.webViewClient
        webView.webViewClient = object : WebViewClient() {
            override fun onPageStarted(view: WebView?, url: String?, favicon: android.graphics.Bitmap?) {
                Log.i(TAG, "WVC.onPageStarted: $url")
                originalClient?.onPageStarted(view, url, favicon)
            }

            override fun onPageFinished(view: WebView?, url: String?) {
                Log.i(TAG, "WVC.onPageFinished: $url")
                originalClient?.onPageFinished(view, url)
            }

            override fun onReceivedError(
                view: WebView?,
                request: WebResourceRequest?,
                error: android.webkit.WebResourceError?
            ) {
                Log.e(TAG, "WVC.onReceivedError: url=${request?.url}, error=${error?.description}")
                originalClient?.onReceivedError(view, request, error)
            }

            override fun shouldInterceptRequest(
                view: WebView?,
                request: WebResourceRequest?
            ): WebResourceResponse? {
                return try {
                    originalClient?.shouldInterceptRequest(view, request)
                } catch (e: Exception) {
                    Log.w(TAG, "WVC.shouldInterceptRequest: originalClient failed", e)
                    @Suppress("UNCHECKED_CAST")
                    (bridge as? com.getcapacitor.Bridge)?.localServer?.shouldInterceptRequest(request)
                }
            }
        }

        webView.loadUrl(WORLD_URL)
        Log.i(TAG, "loadWorldPage: loading $WORLD_URL")
    }

    private fun notifyFrontend(port: Int, running: Boolean, error: String?) {
        bridge?.webView?.evaluateJavascript(
            """
            if (typeof window.__onBackendReady === 'function') {
                window.__onBackendReady({ port: $port, running: $running, error: ${if (error != null) "'$error'" else "null"} });
            }
            """.trimIndent(),
            null
        )
    }

    override fun onDestroy() {
        if (backendReceiverRegistered) {
            try {
                unregisterReceiver(backendReceiver)
                backendReceiverRegistered = false
            } catch (e: Exception) {
                Log.w(TAG, "unregisterReceiver failed", e)
            }
        }
        super.onDestroy()
    }

    override fun onBackPressed() {
        val webView = bridge?.webView
        if (webView?.canGoBack() == true) {
            webView.goBack()
        } else {
            super.onBackPressed()
        }
    }

    override fun onWindowFocusChanged(hasFocus: Boolean) {
        super.onWindowFocusChanged(hasFocus)
        if (hasFocus && immersiveMode) {
            hideSystemBars()
        }
    }

    fun exitWorld() {
        finish()
    }
}
