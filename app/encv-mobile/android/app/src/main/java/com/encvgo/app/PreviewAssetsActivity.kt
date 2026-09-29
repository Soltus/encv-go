package com.encvgo.app

import android.Manifest
import android.content.ContentValues
import android.content.Intent
import android.content.pm.PackageManager
import android.net.Uri
import android.os.Build
import android.os.Bundle
import android.os.Environment
import android.provider.MediaStore
import android.util.Base64
import android.util.Log
import android.webkit.JavascriptInterface
import android.webkit.ValueCallback
import android.webkit.WebChromeClient
import android.webkit.WebResourceRequest
import android.webkit.WebSettings
import android.webkit.WebView
import android.webkit.WebViewClient
import androidx.activity.result.contract.ActivityResultContracts
import androidx.appcompat.app.AppCompatActivity
import androidx.core.content.ContextCompat
import java.io.File
import java.io.FileOutputStream
import java.util.concurrent.ConcurrentHashMap

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

        // 预览页地址的唯一来源：Activity 自己与「复制地址」都走它，
        // 否则端口/路径的兜底逻辑会在两处各写一遍、然后慢慢漂移。
        fun previewAssetsUrl(): String {
            val port = if (EncvGoService.lastKnownPort > 0) EncvGoService.lastKnownPort else DEFAULT_PORT
            return "http://127.0.0.1:$port$ASSETS_PATH"
        }

        // 下载兜底脚本：设备上的预览资源包可能是**旧版**（页面里还没有 attachDownload），
        // 所以这里在页面加载完成后注入，直接接管 a[download] 的点击 —— 不依赖资源包版本。
        //
        // 页面新版本会在自己接管的链接上打 dataset.encvSaved 标记，两边不会重复保存。
        // 分块（256KB）同样是硬要求：一次性把整个 blob 转成 base64 会把 WebView 打死。
        private const val DOWNLOAD_FALLBACK_JS = """
(function () {
  if (window.__encvDlPatched) return;
  window.__encvDlPatched = true;
  if (!window.ENCV || typeof window.ENCV.saveBegin !== 'function') return;
  document.addEventListener('click', function (e) {
    var t = e.target;
    var a = t && t.closest ? t.closest('a[download]') : null;
    if (!a || a.dataset.encvSaved === '1') return;
    var href = a.getAttribute('href') || '';
    if (href.indexOf('blob:') !== 0) return;
    e.preventDefault();
    e.stopPropagation();
    var name = a.getAttribute('download') || 'download.bin';
    fetch(href).then(function (r) { return r.arrayBuffer(); }).then(function (buf) {
      var u8 = new Uint8Array(buf);
      var id = window.ENCV.saveBegin(name);
      var CH = 262144;
      for (var off = 0; off < u8.length; off += CH) {
        var end = Math.min(off + CH, u8.length);
        var s = '';
        for (var i = off; i < end; i += 8192) {
          s += String.fromCharCode.apply(null, u8.subarray(i, Math.min(i + 8192, end)));
        }
        window.ENCV.saveChunk(id, btoa(s));
      }
      window.ENCV.saveEnd(id);
    }).catch(function (err) {
      window.ENCV.toast('保存失败：' + (err && err.message ? err.message : err));
    });
  }, true);
})();
"""
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
            // 留在应用内：只放行本机后端的地址。外跳一律拦掉 ——
            // 既避免预览页把用户带到外部浏览器，也顺带收窄下面那个 JS 桥的暴露面。
            webViewClient = object : WebViewClient() {
                override fun shouldOverrideUrlLoading(view: WebView, request: WebResourceRequest): Boolean {
                    val host = request.url.host ?: return true
                    return host != "127.0.0.1" && host != "localhost"
                }

                override fun onPageFinished(view: WebView, url: String) {
                    super.onPageFinished(view, url)
                    // 资源包可能是旧版，页面自己没有保存逻辑 —— 注入兜底脚本接管下载
                    view.evaluateJavascript(DOWNLOAD_FALLBACK_JS, null)
                }
            }
            // 页面下载产物用的桥（见 OutboxBridge 的注释：DownloadListener 收不到 blob:）
            addJavascriptInterface(OutboxBridge(this@PreviewAssetsActivity), "ENCV")
            setDownloadListener { url, _, _, _, _ ->
                // http(s) 的下载（页面里若真有）交给系统去处理
                runCatching { startActivity(Intent(Intent.ACTION_VIEW, Uri.parse(url))) }
                    .onFailure { Log.e(TAG, "把下载交给系统失败：${it.message}") }
            }
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

    // ─── 保存文件（页面的下载链接）───────────────────────────────────────
    //
    // ⚠️ WebView 的 DownloadListener **收不到 blob: 的下载**：blob 只是页面内的对象，
    // 不是网络请求，于是 `<a download href="blob:…">` 点了既没下载也不报错（真机实测）。
    // 所以页面用 window.ENCV 这套桥把字节交过来：**分块**传（1MB 一片）——
    // 一次性把几百 MB 转成 base64 会把 WebView 打死。
    private class OutboxBridge(private val owner: PreviewAssetsActivity) {
        private val open = ConcurrentHashMap<String, File>()

        @JavascriptInterface
        fun saveBegin(name: String): String {
            val safe = name.substringAfterLast('/').substringAfterLast('\\')
                .ifBlank { "encv-preview-out.bin" }
            val id = "${System.nanoTime()}"
            val dir = File(owner.cacheDir, "preview-outbox").apply { mkdirs() }
            open[id] = File(dir, safe)
            Log.i("PreviewAssetsActivity", "outbox begin: $safe")
            return id
        }

        @JavascriptInterface
        fun saveChunk(id: String, base64: String) {
            val f = open[id] ?: return
            runCatching {
                FileOutputStream(f, true).use { it.write(Base64.decode(base64, Base64.DEFAULT)) }
            }.onFailure { Log.e("PreviewAssetsActivity", "outbox 分块写入失败：${it.message}") }
        }

        @JavascriptInterface
        fun saveEnd(id: String): String {
            val f = open.remove(id) ?: return ""
            val where = owner.publishToDownloads(f)
            // 结果要让用户**看得见**：旧版资源包的页面日志里不会出现这条，
            // 而且 WebView 里 alert 默认不显示（没实现 onJsAlert）。
            owner.runOnUiThread {
                android.widget.Toast.makeText(owner, "已保存：$where", android.widget.Toast.LENGTH_LONG).show()
            }
            return where
        }

        @JavascriptInterface
        fun toast(message: String) {
            owner.runOnUiThread {
                android.widget.Toast.makeText(owner, message, android.widget.Toast.LENGTH_LONG).show()
            }
        }
    }

    // 把收完的文件放到用户看得见的地方：Android 10+ 用 MediaStore（无需权限），
    // 旧版本有权限就写公共 Downloads，没有就落应用自己的外部目录 ——
    // 两种情况都**如实返回路径**，不假装成功。
    private fun publishToDownloads(src: File): String {
        return try {
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.Q) {
                val values = ContentValues().apply {
                    put(MediaStore.Downloads.DISPLAY_NAME, src.name)
                    put(MediaStore.Downloads.MIME_TYPE, "application/octet-stream")
                    put(MediaStore.Downloads.RELATIVE_PATH, Environment.DIRECTORY_DOWNLOADS)
                    put(MediaStore.Downloads.IS_PENDING, 1)
                }
                val uri = contentResolver.insert(MediaStore.Downloads.EXTERNAL_CONTENT_URI, values)
                    ?: return "保存失败：MediaStore 插入返回空"
                contentResolver.openOutputStream(uri)?.use { out -> src.inputStream().use { it.copyTo(out) } }
                values.clear()
                values.put(MediaStore.Downloads.IS_PENDING, 0)
                contentResolver.update(uri, values, null, null)
                "Downloads/${src.name}"
            } else {
                val granted = ContextCompat.checkSelfPermission(
                    this, Manifest.permission.WRITE_EXTERNAL_STORAGE
                ) == PackageManager.PERMISSION_GRANTED
                @Suppress("DEPRECATION")
                val target = if (granted) {
                    Environment.getExternalStoragePublicDirectory(Environment.DIRECTORY_DOWNLOADS)
                } else {
                    getExternalFilesDir(Environment.DIRECTORY_DOWNLOADS)
                }
                val out = File(target, src.name)
                src.inputStream().use { it.copyTo(FileOutputStream(out)) }
                out.absolutePath
            }
        } catch (e: Exception) {
            Log.e(TAG, "publishToDownloads 失败：${e.message}")
            "保存失败：${e.message}"
        }
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
