package com.encvgo.plugin.simverse

import android.content.Context
import android.graphics.Bitmap
import android.util.Log
import android.webkit.MimeTypeMap
import android.webkit.WebResourceError
import android.webkit.WebResourceRequest
import android.webkit.WebResourceResponse
import android.webkit.WebView
import android.webkit.WebViewClient
import java.io.InputStream
import org.json.JSONObject

class SimVerseWebViewClient(
    private val context: Context,
    private val assetBasePath: String = "simverse",
) : WebViewClient() {

    private val tag = "SimVerse-WebViewClient"
    private val satTag = "SimVerse-SAT"
    private val virtualHost = "simverse-plugin.local"

    @Volatile
    var backendPort: Int = 0

    /**
     * 宿主注入的主题 CSS 变量块（由 SimVerseActivity 从 openWorld 的 theme_css extra 传入）。
     * onPageFinished 时注入 window.__ENCV_THEME__ 并写 <style id="encv-host-theme">，
     * 使主应用外观设置在独立 WebView 内生效（插件不共享主应用 window / CSS 变量）。
     */
    @Volatile
    var hostThemeCss: String = ""

    data class WebViewDiagnosticState(
        var pageStartedUrl: String? = null,
        var pageStartedCount: Int = 0,
        var pageFinishedUrl: String? = null,
        var pageFinishedCount: Int = 0,
        var errorCount: Int = 0,
        var lastError: String? = null,
        var httpErrorCount: Int = 0,
        var lastHttpError: String? = null,
        var consoleErrors: MutableList<String> = mutableListOf(),
        var consoleWarnings: MutableList<String> = mutableListOf(),
        var maxConsoleRecords: Int = 20,
        var loadStartedAt: Long = 0,
        var loadFinishedAt: Long = 0,
        var loadDurationMs: Long = 0,
    )

    val diagState = WebViewDiagnosticState()

    private fun satLog(step: String, msg: String) {
        Log.e(satTag, "[$step] $msg")
    }

    private fun satError(step: String, msg: String, t: Throwable? = null) {
        Log.e(satTag, "[$step] $msg", t)
    }

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

    override fun shouldOverrideUrlLoading(view: WebView?, request: WebResourceRequest?): Boolean {
        val url = request?.url
        Log.e(tag, "shouldOverrideUrlLoading: $url")
        if (url == null) return false
        val scheme = url.scheme
        return when {
            scheme == "file" -> {
                false
            }
            scheme == "http" || scheme == "https" -> {
                val host = url.host
                if (host == "127.0.0.1" || host == "localhost" || host == virtualHost) {
                    false
                } else {
                    try {
                        val intent = android.content.Intent(android.content.Intent.ACTION_VIEW, url)
                        intent.addFlags(android.content.Intent.FLAG_ACTIVITY_NEW_TASK)
                        view?.context?.startActivity(intent)
                        true
                    } catch (e: Throwable) {
                        Log.e(tag, "open external url failed", e)
                        false
                    }
                }
            }
            else -> false
        }
    }

    override fun onPageStarted(view: WebView?, url: String?, favicon: Bitmap?) {
        super.onPageStarted(view, url, favicon)
        diagState.pageStartedUrl = url
        diagState.pageStartedCount++
        diagState.loadStartedAt = System.currentTimeMillis()
        Log.e(tag, "onPageStarted: $url (count=${diagState.pageStartedCount})")
        satLog("S14-WV-PAGE-START", "url=$url, count=${diagState.pageStartedCount}")
    }

    override fun onPageFinished(view: WebView?, url: String?) {
        super.onPageFinished(view, url)
        diagState.pageFinishedUrl = url
        diagState.pageFinishedCount++
        diagState.loadFinishedAt = System.currentTimeMillis()
        if (diagState.loadStartedAt > 0) {
            diagState.loadDurationMs = diagState.loadFinishedAt - diagState.loadStartedAt
        }
        Log.e(tag, "onPageFinished: $url (count=${diagState.pageFinishedCount}, duration=${diagState.loadDurationMs}ms)")
        satLog("S14-WV-PAGE-FINISH", "url=$url, count=${diagState.pageFinishedCount}, duration=${diagState.loadDurationMs}ms")

        val port = backendPort
        if (port > 0) {
            Log.i(tag, "onPageFinished: injecting API base, port=$port")
            injectApiBase(view, port)
        }

        val theme = hostThemeCss
        if (theme.isNotBlank()) {
            Log.i(tag, "onPageFinished: injecting host theme (len=${theme.length})")
            injectTheme(view, theme)
        }

        view?.evaluateJavascript(
            """
            (function() {
                try {
                    var msg = 'document.readyState=' + document.readyState;
                    msg += ' | body.children=' + (document.body ? document.body.children.length : 'no body');
                    msg += ' | title=' + (document.title || '(empty)');
                    console.log('[SimVerse-Diag] ' + msg);
                } catch(e) {
                    console.error('[SimVerse-Diag] page check failed: ' + e.message);
                }
            })();
            """.trimIndent(),
            null
        )
    }

    private fun injectApiBase(view: WebView?, port: Int) {
        view ?: return
        val script = """
            (function() {
                window.__ENCV_API_BASE__ = 'http://127.0.0.1:$port';
                window.__ENCV_PORT__ = $port;
                console.log('[SimVerse-WVC] API base set to http://127.0.0.1:$port');
                if (window.dispatchEvent) {
                    window.dispatchEvent(new CustomEvent('encv:api-base-ready', {detail: {port: $port}}));
                }
            })();
        """.trimIndent()
        view.evaluateJavascript(script, null)
    }

    private fun injectTheme(view: WebView?, themeCss: String) {
        view ?: return
        // JSONObject.quote 生成合法 JSON 字符串字面量，安全容纳换行/引号等任意 CSS 内容。
        val cssJson = JSONObject.quote(themeCss)
        val script = """
            (function() {
                try {
                    var css = $cssJson;
                    window.__ENCV_THEME__ = { css: css };
                    var el = document.getElementById('encv-host-theme');
                    if (!el) {
                        el = document.createElement('style');
                        el.id = 'encv-host-theme';
                        document.head.appendChild(el);
                    }
                    el.textContent = css;
                    if (window.dispatchEvent) {
                        window.dispatchEvent(new CustomEvent('encv:theme-change', { detail: { source: 'host' } }));
                    }
                    console.log('[SimVerse-WVC] host theme injected (len=${themeCss.length})');
                } catch (e) {
                    console.error('[SimVerse-WVC] theme inject failed: ' + e.message);
                }
            })();
        """.trimIndent()
        view.evaluateJavascript(script, null)
    }

    override fun onReceivedError(
        view: WebView?,
        errorCode: Int,
        description: String?,
        failingUrl: String?
    ) {
        super.onReceivedError(view, errorCode, description, failingUrl)
        diagState.errorCount++
        diagState.lastError = "code=$errorCode, desc=$description, url=$failingUrl"
        Log.e(tag, "onReceivedError: code=$errorCode desc=$description url=$failingUrl")
        satError("S15-WV-ERROR", "code=$errorCode, desc=$description, url=$failingUrl")
    }

    override fun onReceivedError(
        view: WebView?,
        request: WebResourceRequest?,
        error: WebResourceError?
    ) {
        super.onReceivedError(view, request, error)
        val url = request?.url
        val errorCode = error?.errorCode
        val description = error?.description
        diagState.errorCount++
        diagState.lastError = "code=$errorCode, desc=$description, url=$url, isForMainFrame=${request?.isForMainFrame}"
        Log.e(tag, "onReceivedError (v2): code=$errorCode desc=$description url=$url mainFrame=${request?.isForMainFrame}")
        satError("S15-WV-ERROR", "code=$errorCode, desc=$description, url=$url, mainFrame=${request?.isForMainFrame}")
    }

    override fun onReceivedHttpError(
        view: WebView?,
        request: WebResourceRequest?,
        errorResponse: WebResourceResponse?
    ) {
        super.onReceivedHttpError(view, request, errorResponse)
        val url = request?.url
        val statusCode = errorResponse?.statusCode
        val reasonPhrase = errorResponse?.reasonPhrase
        diagState.httpErrorCount++
        diagState.lastHttpError = "status=$statusCode, reason=$reasonPhrase, url=$url"
        Log.e(tag, "onReceivedHttpError: status=$statusCode reason=$reasonPhrase url=$url")
        satError("S15-WV-HTTP-ERROR", "status=$statusCode, reason=$reasonPhrase, url=$url")
    }

    fun getDiagnosticReport(): String {
        val lines = mutableListOf<String>()
        lines.add("═══ WebView 加载诊断 ═══")
        lines.add("   pageStartedCount = ${diagState.pageStartedCount}")
        lines.add("   pageStartedUrl = ${diagState.pageStartedUrl ?: "(null)"}")
        lines.add("   pageFinishedCount = ${diagState.pageFinishedCount}")
        lines.add("   pageFinishedUrl = ${diagState.pageFinishedUrl ?: "(null)"}")
        lines.add("   loadDurationMs = ${diagState.loadDurationMs}")
        lines.add("   errorCount = ${diagState.errorCount}")
        if (diagState.lastError != null) {
            lines.add("   lastError = ${diagState.lastError}")
        }
        lines.add("   httpErrorCount = ${diagState.httpErrorCount}")
        if (diagState.lastHttpError != null) {
            lines.add("   lastHttpError = ${diagState.lastHttpError}")
        }
        lines.add("   consoleErrorCount = ${diagState.consoleErrors.size}")
        if (diagState.consoleErrors.isNotEmpty()) {
            lines.add("   最近 console errors:")
            diagState.consoleErrors.takeLast(5).forEach {
                lines.add("      - $it")
            }
        }
        return lines.joinToString("\n")
    }
}
