package com.encvgo.plugin.simverse

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

object SimVerseWebViewDiagnostic {
    var lastWebViewClient: SimVerseWebViewClient? = null
        private set

    fun capture(client: SimVerseWebViewClient) {
        lastWebViewClient = client
    }

    fun getReport(): String {
        return lastWebViewClient?.getDiagnosticReport() ?: "⚠️  WebView 尚未初始化（无诊断数据）"
    }
}

@Composable
fun SimVerseEmbedWebView(
    containerId: String = "simverse-plugin-embed",
    initialUrl: String = "https://simverse-plugin.local/",
) {
    val ctx = LocalContext.current
    val webViewRef = remember { mutableStateOf<WebView?>(null) }
    val client = remember { SimVerseWebViewClient(ctx.applicationContext) }
    val chromeClient = remember { SimVerseWebChromeClient(client.diagState) }

    AndroidView(
        modifier = Modifier.fillMaxSize(),
        factory = { c ->
            WebView(c).apply {
                layoutParams = ViewGroup.LayoutParams(
                    ViewGroup.LayoutParams.MATCH_PARENT,
                    ViewGroup.LayoutParams.MATCH_PARENT
                )
                id = containerId.hashCode()
                webViewClient = client
                webChromeClient = chromeClient
                settings.apply {
                    javaScriptEnabled = true
                    domStorageEnabled = true
                    allowFileAccess = false
                    allowContentAccess = false
                    databaseEnabled = true
                    cacheMode = WebSettings.LOAD_DEFAULT
                    useWideViewPort = true
                    loadWithOverviewMode = true
                    mixedContentMode = WebSettings.MIXED_CONTENT_ALWAYS_ALLOW
                }
                addJavascriptInterface(
                    SimVersePluginJSInterface(c.applicationContext),
                    "SimVerseNative"
                )
                Log.e("SimVerse-Embed", "[SimVerse] WebView created | containerId=$containerId | initialUrl=$initialUrl")
                SimVerseWebViewDiagnostic.capture(client)
                loadUrl(initialUrl)
                webViewRef.value = this
            }
        },
        update = { webView ->
            Log.d("SimVerse-Embed", "[SimVerse] WebView update")
        },
    )
}
