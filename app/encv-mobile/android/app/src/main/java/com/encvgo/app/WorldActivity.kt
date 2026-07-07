package com.encvgo.app

import android.content.Context
import android.content.Intent
import android.os.Bundle
import android.util.Log
import android.view.View
import android.view.WindowManager
import androidx.appcompat.app.AppCompatActivity
import android.webkit.WebSettings
import android.webkit.WebView
import android.webkit.WebViewClient
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import java.net.HttpURLConnection
import java.net.URL

class WorldActivity : AppCompatActivity() {
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

    private lateinit var webView: WebView

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        
        // 初始竖屏（显示 SimVerse 首页）
        requestedOrientation = android.content.pm.ActivityInfo.SCREEN_ORIENTATION_PORTRAIT
        
        // 设置状态栏透明
        window.statusBarColor = android.graphics.Color.TRANSPARENT
        val decorView = window.decorView
        decorView.systemUiVisibility = (
            View.SYSTEM_UI_FLAG_LAYOUT_STABLE
            or View.SYSTEM_UI_FLAG_LAYOUT_FULLSCREEN
        )
        window.addFlags(WindowManager.LayoutParams.FLAG_DRAWS_SYSTEM_BAR_BACKGROUNDS)
        
        // 创建 WebView
        webView = WebView(this)
        setContentView(webView)
        
        // WebView 设置
        val settings = webView.settings
        settings.javaScriptEnabled = true
        settings.domStorageEnabled = true
        settings.mediaPlaybackRequiresUserGesture = false
        settings.mixedContentMode = WebSettings.MIXED_CONTENT_ALWAYS_ALLOW
        
        // 加载 simverse-frontend（路由到 /simverse-home）
        //   - 从 Go 后端 HTTP 服务器加载，确保 API 请求同源
        //   - 端口从 EncvGoService 获取（默认 2025）
        webView.webViewClient = WebViewClient()
        val port = EncvGoService.lastKnownPort.let { if (it > 0) it else 2025 }
        webView.loadUrl("http://127.0.0.1:$port/simverse-home/")
        
        // 桥接：通知 Go 后端世界页面已打开
        notifyBackend("opened")
    }

    override fun onBackPressed() {
        if (webView.canGoBack()) {
            webView.goBack()
        } else {
            // 退出世界，通知 Go 后端 checkpoint
            notifyBackend("closed")
            finish()
        }
    }

    override fun onDestroy() {
        webView.destroy()
        super.onDestroy()
    }

    private fun notifyBackend(event: String) {
        val port = EncvGoService.lastKnownPort
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
