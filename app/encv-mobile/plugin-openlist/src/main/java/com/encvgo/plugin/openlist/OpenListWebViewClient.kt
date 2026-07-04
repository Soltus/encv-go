package com.encvgo.plugin.openlist

import android.graphics.Bitmap
import android.util.Log
import android.webkit.WebView
import android.webkit.WebViewClient

/**
 * WebView 客户端：处理页面加载回调和 URL 拦截。
 *
 * 关键行为：
 *   - OpenList SPA 内部链接（http://127.0.0.1:port/）保持在 WebView 内打开
 *   - 外部链接走系统浏览器
 *   - 错误时回调让 web 端 fallback
 */
class OpenListWebViewClient : WebViewClient() {

    private val tag = "OpenList-WebClient"

    override fun shouldOverrideUrlLoading(view: WebView, url: String): Boolean {
        Log.e(tag, "[OpenList] shouldOverrideUrlLoading | url=$url")
        return when {
            // OpenList 自有 SPA：保持在 WebView 内
            url.startsWith("http://127.0.0.1:") || url.startsWith("http://localhost:") -> {
                false  // 让 WebView 自己处理
            }
            // 外部链接：交给系统浏览器
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
            // 其他 scheme（tel:, mailto:, etc.）：系统处理
            else -> false
        }
    }

    override fun onPageStarted(view: WebView, url: String, favicon: Bitmap?) {
        super.onPageStarted(view, url, favicon)
        Log.e(tag, "[OpenList] onPageStarted | url=$url")
    }

    override fun onPageFinished(view: WebView, url: String) {
        super.onPageFinished(view, url)
        Log.e(tag, "[OpenList] onPageFinished | url=$url")
        // 注入 JSInterface 状态广播
        view.evaluateJavascript(
            """
            (function() {
                if (window.__openListBridgeReady) return;
                window.__openListBridgeReady = true;
                console.log('[OpenList-Bridge] JSInterface ready');
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
        Log.e(tag, "[OpenList] onReceivedError | code=$errorCode desc=$description url=$failingUrl")
    }
}
