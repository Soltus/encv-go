package com.encvgo.app

import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import android.content.IntentFilter
import android.graphics.Bitmap
import android.os.Build
import android.os.Bundle
import android.util.Log
import android.view.View
import android.view.WindowManager
import android.webkit.WebResourceError
import android.webkit.WebResourceRequest
import android.webkit.WebResourceResponse
import android.webkit.WebView
import android.webkit.WebViewClient
import com.getcapacitor.BridgeActivity
import com.encvgo.app.GoProcessPlugin
import com.encvgo.app.ApiProxyPlugin
import com.masterpedidos.highrefreshrate.HighRefreshRatePlugin
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import java.net.HttpURLConnection
import java.net.URL

class WorldActivity : BridgeActivity() {
    companion object {
        private const val TAG = "WorldActivity"
        private const val EXTRA_WORLD_ID = "world_id"
        private const val EXTRA_WORLD_NAME = "world_name"

        fun createIntent(context: Context, worldId: String, worldName: String): Intent {
            return Intent(context, WorldActivity::class.java).apply {
                putExtra(EXTRA_WORLD_ID, worldId)
                putExtra(EXTRA_WORLD_NAME, worldName)
            }
        }
    }

    private var backendReceiverRegistered = false
    private var simverseLoaded = false
    private var currentPort = 0

    private val backendReceiver = object : BroadcastReceiver() {
        override fun onReceive(context: Context?, intent: Intent?) {
            when (intent?.action) {
                EncvGoService.BROADCAST_BACKEND_READY,
                EncvGoService.BROADCAST_BACKEND_STATUS -> {
                    val port = intent.getIntExtra(EncvGoService.EXTRA_PORT, 0)
                    val running = intent.getBooleanExtra(EncvGoService.EXTRA_RUNNING, false)
                    Log.d(TAG, "backendReceiver: action=${intent.action}, port=$port, running=$running")
                    if (running && port > 0) {
                        currentPort = port
                        if (!simverseLoaded) {
                            loadSimverse(port)
                        } else {
                            updateApiBase(port)
                        }
                    }
                }
            }
        }
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        Log.i(TAG, "onCreate: start")
        super.onCreate(savedInstanceState)

        window.statusBarColor = android.graphics.Color.TRANSPARENT
        val decorView = window.decorView
        decorView.systemUiVisibility = (
            View.SYSTEM_UI_FLAG_LAYOUT_STABLE
                or View.SYSTEM_UI_FLAG_LAYOUT_FULLSCREEN
            )
        window.addFlags(WindowManager.LayoutParams.FLAG_DRAWS_SYSTEM_BAR_BACKGROUNDS)

        try {
            registerPlugin(GoProcessPlugin::class.java)
            registerPlugin(ApiProxyPlugin::class.java)
            registerPlugin(HighRefreshRatePlugin::class.java)
            registerPlugin(SimVersePlugin::class.java)
            Log.d(TAG, "onCreate: plugins registered")
        } catch (e: Exception) {
            Log.e(TAG, "onCreate: registerPlugin failed", e)
        }

        bridge.webView.settings.mixedContentMode = android.webkit.WebSettings.MIXED_CONTENT_ALWAYS_ALLOW
        Log.i(TAG, "WebView mixedContentMode set to MIXED_CONTENT_ALWAYS_ALLOW")

        registerBackendReceiver()

        if (EncvGoService.isRunning && EncvGoService.lastKnownPort > 0) {
            Log.i(TAG, "onCreate: backend already running, port=${EncvGoService.lastKnownPort}")
            currentPort = EncvGoService.lastKnownPort
            loadSimverse(EncvGoService.lastKnownPort)
        } else {
            Log.i(TAG, "onCreate: starting backend service")
            startBackendService()
        }
    }

    private fun loadSimverse(port: Int) {
        if (simverseLoaded) return
        simverseLoaded = true
        currentPort = port

        try {
            val simverseUrl = "https://localhost/simverse/"
            Log.i(TAG, "loadSimverse: $simverseUrl (apiPort=$port)")

            val webView = bridge?.webView
            if (webView == null) {
                Log.e(TAG, "loadSimverse: webView is null!")
                return
            }

            val originalClient = webView.webViewClient
            webView.webViewClient = object : WebViewClient() {
                override fun onPageStarted(view: WebView?, url: String?, favicon: Bitmap?) {
                    Log.i(TAG, "WVC.onPageStarted: url=$url")
                    originalClient?.onPageStarted(view, url, favicon)
                }

                override fun onPageFinished(view: WebView?, url: String?) {
                    Log.i(TAG, "WVC.onPageFinished: url=$url")
                    originalClient?.onPageFinished(view, url)
                    injectApiBase(port)
                    notifyBackend("opened")
                }

                override fun onReceivedError(view: WebView?, request: WebResourceRequest?, error: WebResourceError?) {
                    Log.e(TAG, "WVC.onReceivedError: url=${request?.url}, error=${error?.description} (${error?.errorCode}), isMain=${request?.isForMainFrame}")
                    originalClient?.onReceivedError(view, request, error)
                }

                override fun onReceivedHttpError(view: WebView?, request: WebResourceRequest?, response: WebResourceResponse?) {
                    Log.e(TAG, "WVC.onReceivedHttpError: url=${request?.url}, status=${response?.statusCode}, reason=${response?.reasonPhrase}")
                    originalClient?.onReceivedHttpError(view, request, response)
                }

                override fun shouldInterceptRequest(view: WebView?, request: WebResourceRequest?): WebResourceResponse? {
                    return try {
                        originalClient?.shouldInterceptRequest(view, request)
                    } catch (e: Exception) {
                        Log.w(TAG, "WVC.shouldInterceptRequest: originalClient failed", e)
                        null
                    }
                }
            }

            val settings = webView.settings
            settings.javaScriptEnabled = true
            settings.domStorageEnabled = true
            settings.mediaPlaybackRequiresUserGesture = false
            settings.mixedContentMode = android.webkit.WebSettings.MIXED_CONTENT_ALWAYS_ALLOW

            webView.loadUrl(simverseUrl)
            Log.i(TAG, "loadSimverse: loadUrl dispatched")
        } catch (e: Exception) {
            Log.e(TAG, "loadSimverse: failed", e)
        }
    }

    private fun injectApiBase(port: Int) {
        val script = """
            (function() {
                window.__ENCV_API_BASE__ = 'http://127.0.0.1:$port';
                window.__ENCV_PORT__ = $port;
                console.log('[WorldActivity] API base set to http://127.0.0.1:$port');
                if (window.dispatchEvent) {
                    window.dispatchEvent(new CustomEvent('encv:api-base-ready', {detail: {port: $port}}));
                }
            })();
        """.trimIndent()
        bridge?.webView?.evaluateJavascript(script, null)
        Log.d(TAG, "injectApiBase: port=$port")
    }

    private fun updateApiBase(port: Int) {
        currentPort = port
        injectApiBase(port)
    }

    private fun startBackendService() {
        val serviceIntent = EncvGoService.createIntent(this, EncvGoService.ACTION_START, "simverse")
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
            startForegroundService(serviceIntent)
        } else {
            startService(serviceIntent)
        }
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
            @Suppress("DEPRECATED")
            registerReceiver(backendReceiver, filter)
        }
        backendReceiverRegistered = true
        Log.d(TAG, "registerBackendReceiver: receiver registered")
    }

    override fun onBackPressed() {
        val webView = bridge?.webView
        if (webView != null && webView.canGoBack()) {
            webView.goBack()
        } else {
            notifyBackend("closed")
            finish()
        }
    }

    override fun onDestroy() {
        Log.d(TAG, "onDestroy: cleaning up")
        if (backendReceiverRegistered) {
            try {
                unregisterReceiver(backendReceiver)
            } catch (e: Exception) {
                Log.w(TAG, "onDestroy: unregisterReceiver failed", e)
            }
            backendReceiverRegistered = false
        }
        super.onDestroy()
    }

    private fun notifyBackend(event: String) {
        val port = currentPort
        if (port <= 0) {
            Log.w(TAG, "notifyBackend: backend not running (port=0), event=$event")
            return
        }
        CoroutineScope(Dispatchers.IO).launch {
            try {
                val url = URL("http://127.0.0.1:$port/api/simverse/world/control")
                val conn = url.openConnection() as HttpURLConnection
                conn.requestMethod = "POST"
                conn.connectTimeout = 2000
                conn.readTimeout = 3000
                conn.doOutput = true
                conn.setRequestProperty("Content-Type", "application/json")
                val body = "{\"event\":\"$event\"}"
                conn.outputStream.write(body.toByteArray())
                val code = conn.responseCode
                Log.d(TAG, "notifyBackend: event=$event, status=$code")
            } catch (e: Exception) {
                Log.w(TAG, "notifyBackend failed: event=$event, ${e.message}")
            }
        }
    }
}
