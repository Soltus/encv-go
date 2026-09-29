package com.encvgo.app

import android.os.Bundle
import android.util.Log
import android.webkit.WebSettings
import android.webkit.WebView
import android.webkit.WebViewClient
import androidx.appcompat.app.AppCompatActivity

/**
 * PreviewAssetsActivity —— 容器预览页（encv-preview）的独立全屏 WebView。
 *
 * **为什么要单独一个 Activity**：预览页不是主应用的 dist 资源，它由**内嵌 Go 后端**
 * 从可写数据目录提供（`GET /preview-assets/*`，见 internal/server/preview_assets.go）。
 * 那一页的资源可以随时整包替换而**不需要换 APK**，所以入口必须指向后端地址，
 * 而不是打包进 APK 的 assets。
 *
 * 端口从 [EncvGoService.lastKnownPort] 取（后端在 2025..2034 里自选），
 * 拿不到时回退 2025 —— 与 ApiProxyPlugin.backendPort() 的兜底保持一致。
 *
 * 页面与后端**同源**（都从 http://127.0.0.1:<port> 来），所以页面里调后端 API
 * 不需要走 ApiProxy，也不存在 CORS 问题。
 */
class PreviewAssetsActivity : AppCompatActivity() {

    companion object {
        private const val TAG = "PreviewAssetsActivity"
        private const val DEFAULT_PORT = 2025
        private const val ASSETS_PATH = "/preview-assets/"
    }

    private var webView: WebView? = null

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)

        val port = if (EncvGoService.lastKnownPort > 0) EncvGoService.lastKnownPort else DEFAULT_PORT
        val url = "http://127.0.0.1:$port$ASSETS_PATH"
        Log.i(TAG, "open preview assets: $url")

        val wv = WebView(this).apply {
            settings.apply {
                javaScriptEnabled = true
                domStorageEnabled = true
                databaseEnabled = true
                // 预览页刻意不碰设备文件，全部字节由页面内的 wasm 从后端/用户选择里取
                allowFileAccess = false
                allowContentAccess = false
                // 页面与后端都是 http（不是 https），不开放会被拦掉
                mixedContentMode = WebSettings.MIXED_CONTENT_ALWAYS_ALLOW
                // 边下边播（MSE）需要在页面加载后自行起播
                mediaPlaybackRequiresUserGesture = false
                loadWithOverviewMode = true
                useWideViewPort = true
            }
            // 留在应用内：不做 Intent 跳转，否则预览页里的链接会把用户带去外部浏览器
            webViewClient = WebViewClient()
        }
        setContentView(wv)
        webView = wv
        wv.loadUrl(url)
    }

    override fun onBackPressed() {
        val wv = webView
        if (wv != null && wv.canGoBack()) {
            wv.goBack()
        } else {
            super.onBackPressed()
        }
    }

    override fun onDestroy() {
        webView?.destroy()
        webView = null
        super.onDestroy()
    }
}
