package com.encvgo.plugin.simverse

import android.app.Activity
import android.content.Context
import android.content.pm.ActivityInfo
import android.os.Build
import android.util.Log
import android.view.WindowManager
import android.webkit.JavascriptInterface
import androidx.core.view.WindowCompat
import androidx.core.view.WindowInsetsCompat
import androidx.core.view.WindowInsetsControllerCompat

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

    @JavascriptInterface
    fun hideSystemUI() {
        Log.i(tag, "hideSystemUI called from JS")
        val activity = activityProvider?.invoke() ?: run {
            Log.w(tag, "hideSystemUI: no activity")
            return
        }
        activity.runOnUiThread {
            try {
                val window = activity.window
                WindowCompat.setDecorFitsSystemWindows(window, false)
                val controller = WindowInsetsControllerCompat(window, window.decorView)
                controller.hide(WindowInsetsCompat.Type.systemBars())
                controller.systemBarsBehavior =
                    WindowInsetsControllerCompat.BEHAVIOR_SHOW_TRANSIENT_BARS_BY_SWIPE
                if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.P) {
                    window.attributes.layoutInDisplayCutoutMode =
                        WindowManager.LayoutParams.LAYOUT_IN_DISPLAY_CUTOUT_MODE_SHORT_EDGES
                }
                Log.i(tag, "hideSystemUI: immersive mode enabled")
            } catch (e: Exception) {
                Log.e(tag, "hideSystemUI failed", e)
            }
        }
    }

    @JavascriptInterface
    fun showSystemUI() {
        Log.i(tag, "showSystemUI called from JS")
        val activity = activityProvider?.invoke() ?: run {
            Log.w(tag, "showSystemUI: no activity")
            return
        }
        activity.runOnUiThread {
            try {
                val window = activity.window
                WindowCompat.setDecorFitsSystemWindows(window, true)
                val controller = WindowInsetsControllerCompat(window, window.decorView)
                controller.show(WindowInsetsCompat.Type.systemBars())
                if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.P) {
                    window.attributes.layoutInDisplayCutoutMode =
                        WindowManager.LayoutParams.LAYOUT_IN_DISPLAY_CUTOUT_MODE_DEFAULT
                }
                Log.i(tag, "showSystemUI: immersive mode disabled")
            } catch (e: Exception) {
                Log.e(tag, "showSystemUI failed", e)
            }
        }
    }

    private fun getGoServerPort(): Int {
        val prefs = context.getSharedPreferences("encv_go_prefs", Context.MODE_PRIVATE)
        return prefs.getInt("server_port", 8780)
    }
}
