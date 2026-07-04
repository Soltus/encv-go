package com.encvgo.app

import android.os.Handler
import android.os.Looper
import android.util.Log
import org.json.JSONObject
import java.net.HttpURLConnection
import java.net.URL

class LogRelay private constructor() {
    companion object {
        @Volatile private var instance: LogRelay? = null
        fun get(): LogRelay = instance ?: synchronized(this) { instance ?: LogRelay().also { instance = it } }
    }

    private val handler = Handler(Looper.getMainLooper())

    fun relay(tag: String, level: String, message: String) {
        when (level) {
            "info" -> Log.i(tag, message)
            "error" -> Log.e(tag, message)
            "warn" -> Log.w(tag, message)
            else -> Log.d(tag, message)
        }
        handler.post { sendToBackend(tag, level, message) }
    }

    private fun sendToBackend(tag: String, level: String, message: String) {
        val port = EncvGoService.lastKnownPort
        if (port <= 0) return
        Thread {
            try {
                val url = URL("http://127.0.0.1:$port/api/logs")
                val conn = url.openConnection() as HttpURLConnection
                conn.requestMethod = "POST"
                conn.setRequestProperty("Content-Type", "application/json")
                conn.doOutput = true
                conn.connectTimeout = 1000
                conn.readTimeout = 1000
                val body = JSONObject().apply {
                    put("level", level)
                    put("message", message)
                    put("tag", tag)
                    put("timestamp", System.currentTimeMillis())
                }.toString()
                conn.outputStream.write(body.toByteArray())
                conn.outputStream.flush()
                conn.inputStream.close()
                conn.disconnect()
            } catch (_e: Exception) { }
        }.start()
    }
}
