package com.encvgo.plugin.mpv

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
fun MpvEmbedWebView(
    containerId: String = "mpv-plugin-embed",
    initialUrl: String = "https://mpv-plugin.local/",
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
                webViewClient = MpvWebViewClient(c.applicationContext)
                settings.apply {
                    javaScriptEnabled = true
                    domStorageEnabled = true
                    allowFileAccess = false
                    allowContentAccess = false
                    databaseEnabled = true
                    cacheMode = WebSettings.LOAD_DEFAULT
                    useWideViewPort = true
                    loadWithOverviewMode = true
                    mediaPlaybackRequiresUserGesture = false
                }
                addJavascriptInterface(
                    MpvPluginJSInterface(c.applicationContext),
                    "MpvNative"
                )
                Log.e("Mpv-Embed", "[Mpv] WebView created | containerId=$containerId | initialUrl=$initialUrl")
                loadUrl(initialUrl)
                webViewRef.value = this
            }
        },
        update = { webView ->
            Log.d("Mpv-Embed", "[Mpv] WebView update")
        }
    )
}
