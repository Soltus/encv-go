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

@Composable
fun SimVerseEmbedWebView(
    containerId: String = "simverse-plugin-embed",
    initialUrl: String = "file:///android_asset/simverse/index.html",
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
                webViewClient = SimVerseWebViewClient()
                settings.apply {
                    javaScriptEnabled = true
                    domStorageEnabled = true
                    allowFileAccess = true
                    allowContentAccess = true
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
                loadUrl(initialUrl)
                webViewRef.value = this
            }
        },
        update = { webView ->
            Log.d("SimVerse-Embed", "[SimVerse] WebView update")
        }
    )
}
