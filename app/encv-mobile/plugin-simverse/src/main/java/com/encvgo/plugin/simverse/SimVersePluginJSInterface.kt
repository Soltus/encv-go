package com.encvgo.plugin.simverse

import android.app.Activity
import android.content.Context
import android.content.pm.ActivityInfo
import android.util.Log
import android.webkit.JavascriptInterface

class SimVersePluginJSInterface(
    private val context: Context,
    private val activityProvider: (() -> Activity?)? = null,
    private val onShowDiagnostic: (() -> Unit)? = null,
    private val onCloseWorld: (() -> Unit)? = null,
) {

    private val tag = "SimVerse-JSInterface"

    @JavascriptInterface
    fun getApiBaseUrl(): String {
        val port = getGoServerPort()
        val url = "http://127.0.0.1:$port"
        Log.i(tag, "getApiBaseUrl() -> $url")
        return url
    }

    @JavascriptInterface
    fun log(message: String) {
        Log.d(tag, "[JS] $message")
    }

    @JavascriptInterface
    fun lockOrientation(orientation: String) {
        val activity = activityProvider?.invoke() ?: run {
            Log.w(tag, "lockOrientation: no activity")
            return
        }
        val requested = when (orientation) {
            "landscape-primary" -> ActivityInfo.SCREEN_ORIENTATION_LANDSCAPE
            "portrait-primary" -> ActivityInfo.SCREEN_ORIENTATION_PORTRAIT
            else -> {
                Log.w(tag, "lockOrientation: unknown orientation=$orientation")
                return
            }
        }
        activity.runOnUiThread {
            try {
                activity.requestedOrientation = requested
                Log.i(tag, "lockOrientation: $orientation")
            } catch (e: Exception) {
                Log.e(tag, "lockOrientation failed", e)
            }
        }
    }

    @JavascriptInterface
    fun unlockOrientation() {
        val activity = activityProvider?.invoke() ?: run {
            Log.w(tag, "unlockOrientation: no activity")
            return
        }
        activity.runOnUiThread {
            try {
                activity.requestedOrientation = ActivityInfo.SCREEN_ORIENTATION_UNSPECIFIED
                Log.i(tag, "unlockOrientation")
            } catch (e: Exception) {
                Log.e(tag, "unlockOrientation failed", e)
            }
        }
    }

    @JavascriptInterface
    fun showDiagnostic() {
        Log.i(tag, "showDiagnostic called from JS")
        onShowDiagnostic?.invoke()
    }

    @JavascriptInterface
    fun closeWorld() {
        Log.i(tag, "closeWorld called from JS")
        onCloseWorld?.invoke()
    }

    private fun getGoServerPort(): Int {
        val prefs = context.getSharedPreferences("encv_go_prefs", Context.MODE_PRIVATE)
        return prefs.getInt("server_port", 8780)
    }
}
