package com.encvgo.plugin.mpv

import android.graphics.Bitmap
import android.util.Log
import android.webkit.WebView
import android.webkit.WebViewClient

class MpvWebViewClient : WebViewClient() {

    private val tag = "Mpv-WebClient"

    override fun shouldOverrideUrlLoading(view: WebView, url: String): Boolean {
        Log.e(tag, "[Mpv] shouldOverrideUrlLoading | url=$url")
        return when {
            url.startsWith("file:///android_asset/mpv/") -> {
                false
            }
            url.startsWith("http://127.0.0.1:") || url.startsWith("http://localhost:") -> {
                false
            }
            url.startsWith("http://") || url.startsWith("https://") -> {
                try {
                    val intent = android.content.Intent(android.content.Intent.ACTION_VIEW, android.net.Uri.parse(url))
                    intent.flags = android.content.Intent.FLAG_ACTIVITY_NEW_TASK
                    view.context.startActivity(intent)
                    true
                } catch (e: Throwable) {
                    Log.e(tag, "open external url failed", e)
                    false
                }
            }
            else -> false
        }
    }

    override fun onPageStarted(view: WebView, url: String, favicon: Bitmap?) {
        super.onPageStarted(view, url, favicon)
        Log.e(tag, "[Mpv] onPageStarted | url=$url")
    }

    override fun onPageFinished(view: WebView, url: String) {
        super.onPageFinished(view, url)
        Log.e(tag, "[Mpv] onPageFinished | url=$url")
        view.evaluateJavascript(
            """
            (function() {
                if (window.__mpvBridgeReady) return;
                window.__mpvBridgeReady = true;
                console.log('[Mpv-Bridge] JSInterface ready');
            })();
            """.trimIndent(),
            null
        )
    }

    override fun onReceivedError(
        view: WebView,
        errorCode: Int,
        description: String,
        failingUrl: String
    ) {
        super.onReceivedError(view, errorCode, description, failingUrl)
        Log.e(tag, "[Mpv] onReceivedError | code=$errorCode desc=$description url=$failingUrl")
    }
}
