package com.encvgo.plugin.openlist

import android.util.Log
import android.view.ViewGroup
import android.webkit.WebSettings
import android.webkit.WebView
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.runtime.Composable
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.viewinterop.AndroidView

/**
 * 嵌入式 WebView：在 plugin-openlist Content() 内部渲染 Capacitor 风格的 WebView。
 *
 * Phase 1 实施要点：
 *   - 使用 AndroidView 包裹 WebView（保留 minimal Compose 依赖）
 *   - 启用 JS / DOM storage / file access
 *   - 注册 OpenListPluginJSInterface（暴露给 web 端 window.OpenListNative）
 *   - 自定义 WebViewClient（OpenList SPA 链接保持在 WebView 内）
 *
 * URL 来源：
 *   - 开发预览：https://localhost/openlist/  (Vite dev server proxy)
 *   - 生产模式：file:///android_asset/openlist/  (assets 内的 web bundle)
 *
 * 与主 app 共享：通过相同 Vite 配置 + pnpm workspace 共享 web 资源。
 */
@Composable
fun OpenListEmbedWebView(
    containerId: String = "openlist-plugin-embed",
    initialUrl: String = "https://localhost/openlist/",
) {
    val ctx = LocalContext.current
    val webViewRef = remember { mutableStateOf<WebView?>(null) }

    AndroidView(
        modifier = Modifier.fillMaxSize(),
        factory = { c ->
            WebView(c).apply {
                layoutParams = ViewGroup.LayoutParams(
                    ViewGroup.LayoutParams.MATCH_PARENT,
                    ViewGroup.LayoutParams.MATCH_PARENT
                )
                id = containerId.hashCode()
                webViewClient = OpenListWebViewClient()
                settings.apply {
                    javaScriptEnabled = true
                    domStorageEnabled = true
                    allowFileAccess = true
                    allowContentAccess = true
                    databaseEnabled = true
                    cacheMode = WebSettings.LOAD_DEFAULT
                    useWideViewPort = true
                    loadWithOverviewMode = true
                }
                // 注册 JS-Native 桥接
                addJavascriptInterface(
                    OpenListPluginJSInterface(c.applicationContext),
                    "OpenListNative"
                )
                Log.e("OpenList-Embed", "[OpenList] WebView created | containerId=$containerId | initialUrl=$initialUrl")
                loadUrl(initialUrl)
                webViewRef.value = this
            }
        },
        update = { webView ->
            // 状态更新钩子：当前未使用，预留
            Log.d("OpenList-Embed", "[OpenList] WebView update")
        }
    )
}
