package com.encvgo.app

import android.content.Intent
import android.net.Uri
import android.os.Bundle
import android.util.Log
import android.webkit.ValueCallback
import android.webkit.WebChromeClient
import android.webkit.WebSettings
import android.webkit.WebView
import android.webkit.WebViewClient
import androidx.activity.result.contract.ActivityResultContracts
import androidx.appcompat.app.AppCompatActivity

/**
 * PreviewAssetsActivity —— 容器预览页（encv-preview）的独立全屏 WebView。
 *
 * **为什么要单独一个 Activity**：预览页不是主应用的 dist 资源，它由**内嵌 Go 后端**
 * 从可写数据目录提供（路由是 /preview-assets/ 加文件路径，见 internal/server/preview_assets.go）。
 *
 * ⚠️ 注释里**不能出现块注释起始符**（斜杠加星号）：Kotlin 的块注释是可嵌套的，
 * 注释里再写一个就会开一层没人闭合的嵌套注释 —— 表现为文件末尾
 * "Syntax error: Unclosed comment"（2026-09-30 的 CI 就是这样红的）。
 * 上面那行路由因此写成 "/preview-assets/ 加文件路径"，而不是把通配符原样抄进来。
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

        // 预览页地址的唯一来源：Activity 自己与「用浏览器打开」都走它，
        // 否则端口/路径的兜底逻辑会在两处各写一遍、然后慢慢漂移。
        fun previewAssetsUrl(): String {
            val port = if (EncvGoService.lastKnownPort > 0) EncvGoService.lastKnownPort else DEFAULT_PORT
            return "http://127.0.0.1:$port$ASSETS_PATH"
        }
    }

    private var webView: WebView? = null

    // ─── 文件选择（页面的 <input type="file">）───────────────────────────
    //
    // ⚠️ 没有 WebChromeClient.onShowFileChooser，点击 input 就是**彻底没反应**：
    // 既不弹选择器、也不报错（2026-09-30 真机实测）。加了之后还有两个必踩的坑：
    //   ① 用户取消/失败时**必须**回调 onReceiveValue(null)，否则页面一直停在
    //      "等待选择"状态，之后点同一个 input 再也收不到回调 —— 表现为偶发失灵；
    //   ② 不能照搬页面的 accept：那是自定义容器扩展名（.sccgv 之类），
    //      Android 映射不到 MIME，照搬会让选择器里一个文件都选不到。
    private var fileChooserCallback: ValueCallback<Array<Uri>>? = null

    private val fileChooserLauncher = registerForActivityResult(
        ActivityResultContracts.StartActivityForResult()
    ) { result ->
        val cb = fileChooserCallback ?: return@registerForActivityResult
        fileChooserCallback = null
        val uris = if (result.resultCode == RESULT_OK) {
            WebChromeClient.FileChooserParams.parseResult(result.resultCode, result.data)
        } else {
            null
        }
        cb.onReceiveValue(uris)
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)

        val url = previewAssetsUrl()
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
            webChromeClient = object : WebChromeClient() {
                override fun onShowFileChooser(
                    webView: WebView,
                    filePathCallback: ValueCallback<Array<Uri>>,
                    fileChooserParams: FileChooserParams
                ): Boolean {
                    // 上一个还没回调就先取消掉，避免 callback 泄漏、页面卡在等待态
                    fileChooserCallback?.onReceiveValue(null)
                    fileChooserCallback = filePathCallback
                    val pickIntent = Intent(Intent.ACTION_GET_CONTENT).apply {
                        type = "*/*"
                        addCategory(Intent.CATEGORY_OPENABLE)
                        putExtra(
                            Intent.EXTRA_ALLOW_MULTIPLE,
                            fileChooserParams.mode == FileChooserParams.MODE_OPEN_MULTIPLE
                        )
                    }
                    return try {
                        fileChooserLauncher.launch(Intent.createChooser(pickIntent, "选择文件"))
                        true
                    } catch (e: Exception) {
                        Log.e(TAG, "file chooser 启动失败：${e.message}")
                        fileChooserCallback = null
                        filePathCallback.onReceiveValue(null)
                        false
                    }
                }
            }
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
        // 页面还在等文件结果时 Activity 被销毁：必须回调 null，
        // 否则 WebView 那边会一直挂着一个永远不返回的 Promise。
        fileChooserCallback?.onReceiveValue(null)
        fileChooserCallback = null
        webView?.destroy()
        webView = null
        super.onDestroy()
    }
}
