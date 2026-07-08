package com.encvgo.plugin.mpv

import android.content.Context
import android.graphics.Bitmap
import android.util.Log
import android.webkit.MimeTypeMap
import android.webkit.WebResourceRequest
import android.webkit.WebResourceResponse
import android.webkit.WebView
import android.webkit.WebViewClient
import java.io.InputStream

class MpvWebViewClient(
    private val context: Context,
    private val assetBasePath: String = "mpv",
) : WebViewClient() {

    private val tag = "Mpv-WebClient"
    private val virtualHost = "mpv-plugin.local"

    override fun shouldInterceptRequest(
        view: WebView?,
        request: WebResourceRequest?
    ): WebResourceResponse? {
        val url = request?.url ?: return null
        if (url.host != virtualHost) return null

        val path = url.path?.trimStart('/') ?: "index.html"
        val assetPath = if (path.isEmpty()) {
            "$assetBasePath/index.html"
        } else {
            "$assetBasePath/$path"
        }

        return try {
            val inputStream: InputStream = context.assets.open(assetPath)
            val mimeType = getMimeType(assetPath)
            val ext = assetPath.substringAfterLast('.', "")
            val encoding = when (ext) {
                "css", "js", "html", "json", "svg", "xml", "txt" -> "UTF-8"
                else -> null
            }
            WebResourceResponse(mimeType, encoding, inputStream).apply {
                setStatusCodeAndReasonPhrase(200, "OK")
                responseHeaders = mapOf(
                    "Cache-Control" to "no-cache",
                    "Access-Control-Allow-Origin" to "*",
                )
            }
        } catch (e: Exception) {
            Log.w(tag, "shouldInterceptRequest: asset not found: $assetPath", e)
            WebResourceResponse("text/plain", "UTF-8", null).apply {
                setStatusCodeAndReasonPhrase(404, "Not Found")
            }
        }
    }

    private fun getMimeType(path: String): String {
        val ext = path.substringAfterLast('.', "").lowercase()
        return when (ext) {
            "html", "htm" -> "text/html"
            "css" -> "text/css"
            "js", "mjs" -> "application/javascript"
            "json" -> "application/json"
            "png" -> "image/png"
            "jpg", "jpeg" -> "image/jpeg"
            "gif" -> "image/gif"
            "svg" -> "image/svg+xml"
            "webp" -> "image/webp"
            "ico" -> "image/x-icon"
            "woff" -> "font/woff"
            "woff2" -> "font/woff2"
            "ttf" -> "font/ttf"
            "otf" -> "font/otf"
            "mp4" -> "video/mp4"
            "webm" -> "video/webm"
            "mp3" -> "audio/mpeg"
            "wav" -> "audio/wav"
            "ogg" -> "audio/ogg"
            "pdf" -> "application/pdf"
            "zip" -> "application/zip"
            else -> {
                val guess = MimeTypeMap.getSingleton().getMimeTypeFromExtension(ext)
                guess ?: "application/octet-stream"
            }
        }
    }

    override fun shouldOverrideUrlLoading(view: WebView, url: String): Boolean {
        Log.e(tag, "[Mpv] shouldOverrideUrlLoading | url=$url")
        return when {
            url.startsWith("https://$virtualHost/") -> {
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
