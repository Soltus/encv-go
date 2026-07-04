package com.encvgo.app

import android.util.Log
import com.getcapacitor.JSObject
import com.getcapacitor.Plugin
import com.getcapacitor.PluginCall
import com.getcapacitor.PluginMethod
import com.getcapacitor.annotation.CapacitorPlugin
import java.io.BufferedReader
import java.io.IOException
import java.io.InputStream
import java.io.InputStreamReader
import java.io.OutputStream
import java.net.HttpURLConnection
import java.net.URL
import java.util.UUID
import java.util.concurrent.ConcurrentHashMap
import java.util.concurrent.Executors
import java.util.concurrent.atomic.AtomicBoolean

/**
 * ApiProxyPlugin — Capacitor 插件，把 WebView 的 fetch 走 native HTTP。
 *
 * 解决 WebView origin (`https://localhost`) 与后端 (`http://127.0.0.1:2025`)
 * 跨源触发的 CORS preflight 问题。所有 API 调用从 JS 走相对路径 `fetch('/api/...')`，
 * plugin 收到后用 HttpURLConnection 调 Go server，把响应原样回传。
 *
 * 端到端时序：
 *   JS: ApiProxy.fetchOnce({url:'/api/...', method:'POST', body:'...'})
 *     ↓
 *   native: HttpURLConnection → 127.0.0.1:2025/api/...
 *     ↓
 *   返回 {status, headers, body, resolvedBaseUrl:'http://127.0.0.1:2025'}
 *     ↓
 *   JS: 包装为 Response 对象供原代码消费
 *
 * SSE 走 streamStart：返回 streamId 后另起线程读流，通过 notifyListeners
 * 把每个 chunk (base64) 推给 JS。streamCancel 用于主动断开（切 tab / 重连）。
 */
@CapacitorPlugin(name = "ApiProxy")
class ApiProxyPlugin : Plugin() {

    companion object {
        private const val TAG = "ApiProxy"
        // 与 EncvGoService.DEFAULT_PORT (private) 对齐；scan 区间 2025-2034
        private const val DEFAULT_PORT = 2025
        private const val CONNECT_TIMEOUT_MS = 5_000
        private const val READ_TIMEOUT_MS = 30_000
        private const val STREAM_CHUNK_MAX = 16 * 1024
    }

    private val ioExecutor = Executors.newCachedThreadPool { r ->
        Thread(r, "ApiProxy-IO").apply { isDaemon = true }
    }

    private val activeStreams = ConcurrentHashMap<String, AtomicBoolean>()

    @PluginMethod
    fun fetchOnce(call: PluginCall) {
        ioExecutor.execute {
            val url = call.getString("url")
            val method = call.getString("method", "GET") ?: "GET"
            val body = call.getString("body")
            val headersJson = call.getObject("headers")

            if (url.isNullOrBlank()) {
                call.reject("url is required")
                return@execute
            }

            val result = JSObject()
            try {
                val resolvedUrl = resolveBackendUrl(url)
                val conn = openConnection(resolvedUrl, method, headersJson, body)
                try {
                    result.put("status", conn.responseCode)
                    result.put("statusText", conn.responseMessage ?: "")
                    val headers = JSObject()
                    conn.headerFields.forEach { (k, v) ->
                        if (k != null && v != null && v.isNotEmpty()) {
                            headers.put(k.lowercase(), v.joinToString(", "))
                        }
                    }
                    result.put("headers", headers)
                    val stream = if (conn.responseCode in 200..299) conn.inputStream else conn.errorStream
                    result.put("body", stream?.readBytes()?.let { String(it, Charsets.UTF_8) } ?: "")
                    result.put("resolvedBaseUrl", backendOrigin())
                    call.resolve(result)
                } finally {
                    conn.disconnect()
                }
            } catch (e: Exception) {
                Log.w(TAG, "fetchOnce failed: ${e.message}")
                call.reject("proxy fetch failed: ${e.message}", e)
            }
        }
    }

    @PluginMethod
    fun streamStart(call: PluginCall) {
        val url = call.getString("url")
        val method = call.getString("method", "GET") ?: "GET"
        val body = call.getString("body")
        val headersJson = call.getObject("headers")

        if (url.isNullOrBlank()) {
            call.reject("url is required")
            return
        }

        val streamId = UUID.randomUUID().toString()
        val cancelled = AtomicBoolean(false)
        activeStreams[streamId] = cancelled

        // 立即 resolve 头信息，chunks 走 event
        // 但要等 conn.responseCode 才能拿到 status，所以延迟到 stream thread 拿到 header
        ioExecutor.execute {
            try {
                val resolvedUrl = resolveBackendUrl(url)
                val conn = openConnection(resolvedUrl, method, headersJson, body)
                val initialResult = JSObject().apply {
                    put("streamId", streamId)
                    put("status", conn.responseCode)
                    put("statusText", conn.responseMessage ?: "")
                    val headers = JSObject()
                    conn.headerFields.forEach { (k, v) ->
                        if (k != null && v != null && v.isNotEmpty()) {
                            headers.put(k.lowercase(), v.joinToString(", "))
                        }
                    }
                    put("headers", headers)
                    put("resolvedBaseUrl", backendOrigin())
                }
                call.resolve(initialResult)

                // 异步读流 → notifyListeners
                val stream = if (conn.responseCode in 200..299) conn.inputStream else conn.errorStream
                readStreamToEvents(streamId, conn, stream, cancelled)
            } catch (e: Exception) {
                Log.w(TAG, "streamStart failed: ${e.message}")
                if (!cancelled.get()) {
                    val endPayload = JSObject().apply {
                        put("streamId", streamId)
                        put("error", e.message ?: "stream error")
                    }
                    notifyListeners("stream:end", endPayload)
                }
                activeStreams.remove(streamId)
            }
        }
    }

    @PluginMethod
    fun streamCancel(call: PluginCall) {
        val streamId = call.getString("streamId")
        if (streamId != null) {
            activeStreams.remove(streamId)?.set(true)
        }
        call.resolve()
    }

    // --- internal helpers ---

    private fun readStreamToEvents(
        streamId: String,
        conn: HttpURLConnection,
        stream: InputStream?,
        cancelled: AtomicBoolean,
    ) {
        if (stream == null) {
            notifyStreamEnd(streamId, conn.responseCode, null)
            activeStreams.remove(streamId)
            conn.disconnect()
            return
        }
        try {
            val buffer = ByteArray(STREAM_CHUNK_MAX)
            while (!cancelled.get()) {
                val n = stream.read(buffer)
                if (n <= 0) break
                val payload = JSObject().apply {
                    put("streamId", streamId)
                    put("dataBase64", android.util.Base64.encodeToString(buffer, 0, n, android.util.Base64.NO_WRAP))
                }
                notifyListeners("stream:data", payload)
            }
            notifyStreamEnd(streamId, conn.responseCode, null)
        } catch (e: IOException) {
            if (!cancelled.get()) {
                Log.w(TAG, "stream read error: ${e.message}")
                notifyStreamEnd(streamId, null, e.message)
            }
        } finally {
            try { stream.close() } catch (_: Exception) {}
            conn.disconnect()
            activeStreams.remove(streamId)
        }
    }

    private fun notifyStreamEnd(streamId: String, status: Int?, error: String?) {
        val payload = JSObject().apply {
            put("streamId", streamId)
            if (status != null) put("status", status)
            if (error != null) put("error", error)
        }
        notifyListeners("stream:end", payload)
    }

    /**
     * 解析 JS 端发来的 url：
     *   - 绝对 URL（`http://` / `https://`）原样用
     *   - 相对路径（`/api/...` / `api/...`）走 backend 127.0.0.1:2025
     */
    private fun resolveBackendUrl(url: String): String {
        if (url.startsWith("http://") || url.startsWith("https://")) return url
        val path = if (url.startsWith("/")) url else "/$url"
        return "http://127.0.0.1:${backendPort()}$path"
    }

    private fun backendOrigin(): String = "http://127.0.0.1:${backendPort()}"

    /**
     * 优先 EncvGoService.lastKnownPort，fallback 到 DEFAULT_PORT。
     * 实在拿不到就 2025（多扫 9 个端口的总成本小于这里死磕 1 个）。
     */
    private fun backendPort(): Int {
        val known = EncvGoService.lastKnownPort
        return if (known > 0) known else DEFAULT_PORT
    }

    private fun openConnection(
        urlStr: String,
        method: String,
        headersJson: org.json.JSONObject?,
        body: String?,
    ): HttpURLConnection {
        val url = URL(urlStr)
        val conn = url.openConnection() as HttpURLConnection
        conn.requestMethod = method.uppercase()
        conn.connectTimeout = CONNECT_TIMEOUT_MS
        conn.readTimeout = READ_TIMEOUT_MS
        conn.instanceFollowRedirects = false
        // 把所有自定义 header 透传（X-Agent-Protocol, X-Mock-Mode 等）
        if (headersJson != null) {
            val keys = headersJson.keys()
            while (keys.hasNext()) {
                val k = keys.next()
                conn.setRequestProperty(k, headersJson.getString(k))
            }
        }
        if (body != null && method.uppercase() !in listOf("GET", "HEAD")) {
            conn.doOutput = true
            val bytes = body.toByteArray(Charsets.UTF_8)
            conn.setFixedLengthStreamingMode(bytes.size)
            conn.setRequestProperty("Content-Type", conn.getRequestProperty("Content-Type") ?: "application/json")
            val os: OutputStream = conn.outputStream
            os.use { it.write(bytes) }
        }
        return conn
    }
}
