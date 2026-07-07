package com.encvgo.plugin.simverse

import android.content.Context
import android.util.Log
import android.webkit.JavascriptInterface

class SimVersePluginJSInterface(private val context: Context) {

    private val tag = "SimVerse-JSInterface"

    @JavascriptInterface
    fun getApiBaseUrl(): String {
        val port = getGoServerPort()
        val url = "http://127.0.0.1:$port"
        Log.e(tag, "getApiBaseUrl() -> $url")
        return url
    }

    @JavascriptInterface
    fun log(message: String) {
        Log.e(tag, "[JS] $message")
    }

    private fun getGoServerPort(): Int {
        val prefs = context.getSharedPreferences("encv_go_prefs", Context.MODE_PRIVATE)
        return prefs.getInt("server_port", 8780)
    }
}
