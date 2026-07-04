package com.encvgo.app

import android.graphics.Bitmap
import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import android.content.IntentFilter
import android.database.Cursor
import android.net.Uri
import android.os.Build
import android.os.Bundle
import android.provider.OpenableColumns
import android.util.Log
import android.webkit.WebResourceError
import android.webkit.WebResourceRequest
import android.webkit.WebResourceResponse
import android.webkit.WebView
import android.webkit.WebViewClient
import androidx.core.content.ContextCompat
import com.getcapacitor.BridgeActivity
import org.json.JSONObject
import java.io.File
import java.io.FileOutputStream
import java.io.InputStream

class PlayerActivityCapacitor : BridgeActivity() {
    companion object {
        private const val TAG = "ENCV-go"
        var intentFilePath: String = ""
        var intentFileName: String = ""
        var intentFileMimeType: String = ""
    }

    private var backendReceiverRegistered = false

    private val backendReceiver = object : BroadcastReceiver() {
        override fun onReceive(context: Context?, intent: Intent?) {
            when (intent?.action) {
                EncvGoService.BROADCAST_BACKEND_READY,
                EncvGoService.BROADCAST_BACKEND_STATUS -> {
                    val port = intent.getIntExtra(EncvGoService.EXTRA_PORT, 0)
                    val error = intent.getStringExtra(EncvGoService.EXTRA_ERROR)
                    val running = intent.getBooleanExtra(EncvGoService.EXTRA_RUNNING, false)
                    val source = intent.getStringExtra(EncvGoService.EXTRA_SOURCE)
                    val command = intent.getStringExtra(EncvGoService.EXTRA_COMMAND)
                    Log.d(TAG, "backendReceiver: received ${intent.action}, port=$port, running=$running, error=$error")
                    notifyFrontend(port, running, error, source, command)
                }
            }
        }
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        Log.i(TAG, "onCreate: start")
        try {
            registerPlugin(GoProcessPlugin::class.java)
            Log.d(TAG, "onCreate: GoProcessPlugin registered")
        } catch (e: Exception) {
            Log.e(TAG, "onCreate: registerPlugin failed", e)
        }
        super.onCreate(savedInstanceState)
        Log.i(TAG, "onCreate: super done, bridge=${bridge != null}, webView=${bridge?.webView != null}")
        registerBackendReceiver()
        resolveFileInfo(intent)
        navigateToPlayer()
        handleBackend()
    }

    private fun navigateToPlayer() {
        try {
            val playerUrl = "https://localhost/player.html"
            Log.i(TAG, "navigateToPlayer: $playerUrl")
            val webView = bridge?.webView
            if (webView == null) {
                Log.e(TAG, "navigateToPlayer: webView is null!")
                return
            }
            val originalClient = webView.webViewClient
            Log.d(TAG, "navigateToPlayer: originalClient=${originalClient?.javaClass?.simpleName}")
            webView.webViewClient = object : WebViewClient() {
                override fun onPageStarted(view: WebView?, url: String?, favicon: Bitmap?) {
                    Log.i(TAG, "WVC.onPageStarted: url=$url")
                    originalClient?.onPageStarted(view, url, favicon)
                }
                override fun onPageFinished(view: WebView?, url: String?) {
                    Log.i(TAG, "WVC.onPageFinished: url=$url")
                    originalClient?.onPageFinished(view, url)
                }
                override fun onReceivedError(view: WebView?, request: WebResourceRequest?, error: WebResourceError?) {
                    Log.e(TAG, "WVC.onReceivedError: url=${request?.url}, error=${error?.description} (${error?.errorCode}), isMain=${request?.isForMainFrame}")
                    originalClient?.onReceivedError(view, request, error)
                }
                override fun onReceivedHttpError(view: WebView?, request: WebResourceRequest?, response: WebResourceResponse?) {
                    Log.e(TAG, "WVC.onReceivedHttpError: url=${request?.url}, status=${response?.statusCode}, reason=${response?.reasonPhrase}")
                    originalClient?.onReceivedHttpError(view, request, response)
                }
                override fun shouldInterceptRequest(view: WebView?, request: WebResourceRequest?): WebResourceResponse? {
                    val url = request?.url.toString()
                    val ext = url.substringAfterLast('.', "").lowercase()
                    if (ext in listOf("html", "js", "css", "woff", "woff2", "ttf")) {
                        Log.d(TAG, "WVC.shouldInterceptRequest: $url")
                    }
                    return try {
                        originalClient?.shouldInterceptRequest(view, request)
                    } catch (e: Exception) {
                        Log.w(TAG, "WVC.shouldInterceptRequest: originalClient failed, fallback to bridge localServer", e)
                        @Suppress("UNCHECKED_CAST")
                        (bridge as? com.getcapacitor.Bridge)?.localServer?.shouldInterceptRequest(request)
                    }
                }
            }
            webView.loadUrl(playerUrl)
            Log.i(TAG, "navigateToPlayer: loadUrl dispatched with diagnostic WVC")
        } catch (e: Exception) {
            Log.e(TAG, "navigateToPlayer: failed", e)
        }
    }

    private fun handleBackend() {
        when {
            EncvGoService.isRunning && EncvGoService.lastKnownPort > 0 -> {
                Log.i(TAG, "handleBackend: already running port=${EncvGoService.lastKnownPort}")
                notifyFrontend(EncvGoService.lastKnownPort, true, null, "player", null)
            }
            else -> {
                Log.i(TAG, "handleBackend: starting service source=player")
                startBackendService(EncvGoService.ACTION_START, "player", null)
            }
        }
    }

    override fun onNewIntent(intent: Intent) {
        Log.i(TAG, "onNewIntent: action=${intent.action}, data=${intent.data}, extras=${intent.extras?.keySet()}")
        super.onNewIntent(intent)
        setIntent(intent)
        resolveFileInfo(intent)
    }

    override fun onDestroy() {
        Log.d(TAG, "onDestroy: cleaning up")
        if (backendReceiverRegistered) {
            unregisterReceiver(backendReceiver)
            backendReceiverRegistered = false
            Log.d(TAG, "onDestroy: receiver unregistered")
        }
        finishAndRemoveTask()
        super.onDestroy()
    }

    private fun registerBackendReceiver() {
        if (backendReceiverRegistered) {
            Log.d(TAG, "registerBackendReceiver: already registered, skipping")
            return
        }
        val filter = IntentFilter().apply {
            addAction(EncvGoService.BROADCAST_BACKEND_READY)
            addAction(EncvGoService.BROADCAST_BACKEND_STATUS)
        }
        if (Build.VERSION.SDK_INT >= 33) {
            registerReceiver(backendReceiver, filter, RECEIVER_NOT_EXPORTED)
        } else {
            @Suppress("DEPRECATION")
            registerReceiver(backendReceiver, filter)
        }
        backendReceiverRegistered = true
        Log.d(TAG, "registerBackendReceiver: receiver registered successfully")
    }

    private fun resolveFileInfo(intent: Intent?) {
        if (intent == null) {
            Log.w(TAG, "resolveFileInfo: intent is null, skipping")
            return
        }

        val internalPath = intent.getStringExtra("file_path")
        if (!internalPath.isNullOrEmpty()) {
            Log.i(TAG, "resolveFileInfo: internal path provided: $internalPath")
            PlayerActivityCapacitor.intentFilePath = internalPath
            PlayerActivityCapacitor.intentFileName = intent.getStringExtra("file_name") ?: File(internalPath).name
            PlayerActivityCapacitor.intentFileMimeType = intent.getStringExtra("file_mime_type") ?: ""
            return
        }

        val uri: Uri? = intent.data ?: if (Build.VERSION.SDK_INT >= 33) {
            intent.getParcelableExtra(Intent.EXTRA_STREAM, Uri::class.java)
        } else {
            @Suppress("DEPRECATION")
            intent.getParcelableExtra<Uri>(Intent.EXTRA_STREAM)
        }
        if (uri == null) {
            Log.w(TAG, "resolveFileInfo: no URI found in intent data or EXTRA_STREAM")
            return
        }

        Log.i(TAG, "resolveFileInfo: processing URI scheme=${uri.scheme}, uri=$uri")
        PlayerActivityCapacitor.intentFileMimeType = intent.type ?: ""

        when (uri.scheme) {
            "content" -> {
                var fileName = ""
                var filePath = ""
                contentResolver.query(uri, null, null, null, null)?.use { cursor ->
                    if (cursor.moveToFirst()) {
                        val nameIndex = cursor.getColumnIndex(OpenableColumns.DISPLAY_NAME)
                        if (nameIndex >= 0) {
                            fileName = cursor.getString(nameIndex)
                        }
                    }
                }
                if (fileName.isEmpty()) {
                    fileName = uri.lastPathSegment ?: "unknown_file"
                }
                val projection = arrayOf(android.provider.MediaStore.MediaColumns.DATA)
                try {
                    contentResolver.query(uri, projection, null, null, null)?.use { cursor ->
                        if (cursor.moveToFirst()) {
                            val dataIndex = cursor.getColumnIndexOrThrow(android.provider.MediaStore.MediaColumns.DATA)
                            filePath = cursor.getString(dataIndex)
                        }
                    }
                } catch (_: Exception) {
                }
                if (filePath.isEmpty() || !File(filePath).exists()) {
                    Log.d(TAG, "resolveFileInfo: content URI file path not found, copying to cache")
                    filePath = copyContentToCache(uri)
                }
                PlayerActivityCapacitor.intentFilePath = filePath
                PlayerActivityCapacitor.intentFileName = fileName
                if (PlayerActivityCapacitor.intentFileMimeType.isEmpty()) {
                    PlayerActivityCapacitor.intentFileMimeType = contentResolver.getType(uri) ?: ""
                }
                Log.i(TAG, "resolveFileInfo: content resolved -> fileName=$fileName, filePath=$filePath, mimeType=${PlayerActivityCapacitor.intentFileMimeType}")
            }
            "file" -> {
                val path = uri.path ?: ""
                PlayerActivityCapacitor.intentFilePath = path
                PlayerActivityCapacitor.intentFileName = uri.lastPathSegment ?: File(path).name
                if (PlayerActivityCapacitor.intentFileMimeType.isEmpty()) {
                    PlayerActivityCapacitor.intentFileMimeType = contentResolver.getType(uri) ?: ""
                }
                Log.i(TAG, "resolveFileInfo: file scheme resolved -> path=$path, name=${PlayerActivityCapacitor.intentFileName}, mimeType=${PlayerActivityCapacitor.intentFileMimeType}")
            }
            else -> {
                Log.w(TAG, "resolveFileInfo: unsupported URI scheme: ${uri.scheme}")
            }
        }
    }

    private fun copyContentToCache(uri: Uri): String {
        val fileName = try {
            contentResolver.query(uri, null, null, null, null)?.use { cursor ->
                if (cursor.moveToFirst()) {
                    val nameIndex = cursor.getColumnIndex(OpenableColumns.DISPLAY_NAME)
                    if (nameIndex >= 0) cursor.getString(nameIndex) else null
                } else null
            }
        } catch (_: Exception) {
            null
        } ?: uri.lastPathSegment ?: "cached_file"

        val cacheDir = File(cacheDir, "player_cache")
        cacheDir.mkdirs()
        val destFile = File(cacheDir, fileName)
        if (destFile.exists()) {
            destFile.delete()
        }
        try {
            contentResolver.openInputStream(uri)?.use { input ->
                FileOutputStream(destFile).use { output ->
                    val buffer = ByteArray(8192)
                    var len: Int
                    while (input.read(buffer).also { len = it } != -1) {
                        output.write(buffer, 0, len)
                    }
                }
            }
            Log.i(TAG, "copyContentToCache: copied to ${destFile.absolutePath} (${destFile.length()} bytes)")
        } catch (e: Exception) {
            Log.e(TAG, "copyContentToCache: failed to copy content to cache", e)
            return ""
        }
        return destFile.absolutePath
    }

    private fun startBackendService(action: String, source: String, command: String?) {
        Log.i(TAG, "startBackendService: action=$action, source=$source, command=$command")
        val serviceIntent = EncvGoService.createIntent(this, action, source).apply {
            if (!command.isNullOrEmpty()) {
                putExtra(EncvGoService.EXTRA_COMMAND, command)
            }
        }
        ContextCompat.startForegroundService(this, serviceIntent)
    }

    private fun notifyFrontend(port: Int, running: Boolean, error: String?, source: String?, command: String?) {
        runOnUiThread {
            try {
                val detail = JSONObject().apply {
                    put("port", port)
                    put("running", running)
                    if (error != null) put("error", error)
                    if (source != null) put("source", source)
                    if (command != null) put("command", command)
                }
                val readyEvent = "window.dispatchEvent(new CustomEvent('encv:backend-ready',{detail:${detail}}))"
                val statusEvent = "window.dispatchEvent(new CustomEvent('encv:backend-status',{detail:${detail}}))"
                Log.d(TAG, "notifyFrontend: port=$port, running=$running, error=$error, source=$source, command=$command")
                bridge?.webView?.evaluateJavascript(readyEvent, null)
                bridge?.webView?.evaluateJavascript(statusEvent, null)
            } catch (e: Exception) {
                Log.w(TAG, "notifyFrontend: failed to notify frontend (WebView may not be ready yet)", e)
            }
        }
    }
}
