package com.encvgo.plugin.mpv

import android.util.Log
import android.webkit.JavascriptInterface
import org.json.JSONArray
import org.json.JSONObject

class MpvPluginJSInterface(
    private val appContext: android.content.Context,
) {
    private val tag = "Mpv-JS"

    @JavascriptInterface
    fun getVersion(): String {
        return try {
            Log.d(tag, "JS -> getVersion()")
            "1.0.0"
        } catch (e: Throwable) {
            Log.e(tag, "getVersion() FAILED", e)
            "unknown"
        }
    }

    @JavascriptInterface
    fun getStatus(): String {
        return try {
            Log.d(tag, "JS -> getStatus()")
            JSONObject().apply {
                put("playing", false)
                put("paused", false)
                put("volume", 100)
                put("mute", false)
                put("duration", 0)
                put("position", 0)
                put("title", "")
                put("artist", "")
                put("videoCodec", "")
                put("audioCodec", "")
                put("width", 0)
                put("height", 0)
                put("fps", 0)
                put("lastError", "")
                put("lastUpdateTs", System.currentTimeMillis())
            }.toString()
        } catch (e: Throwable) {
            Log.e(tag, "getStatus() FAILED", e)
            JSONObject().apply {
                put("playing", false)
                put("paused", false)
                put("error", e.message ?: "unknown")
            }.toString()
        }
    }

    @JavascriptInterface
    fun play(url: String): Boolean {
        return try {
            Log.d(tag, "JS -> play() | url=$url")
            true
        } catch (e: Throwable) {
            Log.e(tag, "play() FAILED", e)
            false
        }
    }

    @JavascriptInterface
    fun pause(): Boolean {
        return try {
            Log.d(tag, "JS -> pause()")
            true
        } catch (e: Throwable) {
            Log.e(tag, "pause() FAILED", e)
            false
        }
    }

    @JavascriptInterface
    fun stop(): Boolean {
        return try {
            Log.d(tag, "JS -> stop()")
            true
        } catch (e: Throwable) {
            Log.e(tag, "stop() FAILED", e)
            false
        }
    }

    @JavascriptInterface
    fun seek(seconds: Int): Boolean {
        return try {
            Log.d(tag, "JS -> seek() | seconds=$seconds")
            true
        } catch (e: Throwable) {
            Log.e(tag, "seek() FAILED", e)
            false
        }
    }

    @JavascriptInterface
    fun openPlayer(url: String): Boolean {
        return try {
            Log.d(tag, "JS -> openPlayer() | url=$url")
            val intent = android.content.Intent()
            intent.setClassName(appContext, "com.encvgo.plugin.mpv.MpvPlayerActivity")
            intent.putExtra("file_path", url)
            intent.putExtra("file_name", url.substringAfterLast('/'))
            intent.putExtra("mime_type", "")
            intent.flags = android.content.Intent.FLAG_ACTIVITY_NEW_TASK
            appContext.startActivity(intent)
            true
        } catch (e: Throwable) {
            Log.e(tag, "openPlayer() FAILED", e)
            false
        }
    }

    @JavascriptInterface
    fun openPlayerWithInfo(filePath: String, fileName: String, mimeType: String): Boolean {
        return try {
            Log.d(tag, "JS -> openPlayerWithInfo() | filePath=$filePath fileName=$fileName mimeType=$mimeType")
            val intent = android.content.Intent()
            intent.setClassName(appContext, "com.encvgo.plugin.mpv.MpvPlayerActivity")
            intent.putExtra("file_path", filePath)
            intent.putExtra("file_name", fileName)
            intent.putExtra("mime_type", mimeType)
            intent.flags = android.content.Intent.FLAG_ACTIVITY_NEW_TASK
            appContext.startActivity(intent)
            true
        } catch (e: Throwable) {
            Log.e(tag, "openPlayerWithInfo() FAILED", e)
            false
        }
    }

    @JavascriptInterface
    fun openWebView(): Boolean {
        return try {
            Log.d(tag, "JS -> openWebView()")
            val intent = android.content.Intent()
            intent.setClassName(appContext, "com.encvgo.plugin.mpv.MpvWebViewActivity")
            intent.flags = android.content.Intent.FLAG_ACTIVITY_NEW_TASK
            appContext.startActivity(intent)
            true
        } catch (e: Throwable) {
            Log.e(tag, "openWebView() FAILED", e)
            false
        }
    }

    @JavascriptInterface
    fun getVolume(): Int {
        return try {
            Log.d(tag, "JS -> getVolume()")
            100
        } catch (e: Throwable) {
            Log.e(tag, "getVolume() FAILED", e)
            100
        }
    }

    @JavascriptInterface
    fun setVolume(volume: Int): Boolean {
        return try {
            Log.d(tag, "JS -> setVolume() | volume=$volume")
            true
        } catch (e: Throwable) {
            Log.e(tag, "setVolume() FAILED", e)
            false
        }
    }

    @JavascriptInterface
    fun getMute(): Boolean {
        return try {
            Log.d(tag, "JS -> getMute()")
            false
        } catch (e: Throwable) {
            Log.e(tag, "getMute() FAILED", e)
            false
        }
    }

    @JavascriptInterface
    fun setMute(mute: Boolean): Boolean {
        return try {
            Log.d(tag, "JS -> setMute() | mute=$mute")
            true
        } catch (e: Throwable) {
            Log.e(tag, "setMute() FAILED", e)
            false
        }
    }

    @JavascriptInterface
    fun getDuration(): Long {
        return try {
            Log.d(tag, "JS -> getDuration()")
            0L
        } catch (e: Throwable) {
            Log.e(tag, "getDuration() FAILED", e)
            0L
        }
    }

    @JavascriptInterface
    fun getPosition(): Long {
        return try {
            Log.d(tag, "JS -> getPosition()")
            0L
        } catch (e: Throwable) {
            Log.e(tag, "getPosition() FAILED", e)
            0L
        }
    }

    @JavascriptInterface
    fun setPosition(position: Long): Boolean {
        return try {
            Log.d(tag, "JS -> setPosition() | position=$position")
            true
        } catch (e: Throwable) {
            Log.e(tag, "setPosition() FAILED", e)
            false
        }
    }

    @JavascriptInterface
    fun getPlaylist(): String {
        return try {
            Log.d(tag, "JS -> getPlaylist()")
            JSONArray().toString()
        } catch (e: Throwable) {
            Log.e(tag, "getPlaylist() FAILED", e)
            "[]"
        }
    }

    @JavascriptInterface
    fun addToPlaylist(url: String): Boolean {
        return try {
            Log.d(tag, "JS -> addToPlaylist() | url=$url")
            true
        } catch (e: Throwable) {
            Log.e(tag, "addToPlaylist() FAILED", e)
            false
        }
    }

    @JavascriptInterface
    fun removeFromPlaylist(index: Int): Boolean {
        return try {
            Log.d(tag, "JS -> removeFromPlaylist() | index=$index")
            true
        } catch (e: Throwable) {
            Log.e(tag, "removeFromPlaylist() FAILED", e)
            false
        }
    }

    @JavascriptInterface
    fun clearPlaylist(): Boolean {
        return try {
            Log.d(tag, "JS -> clearPlaylist()")
            true
        } catch (e: Throwable) {
            Log.e(tag, "clearPlaylist() FAILED", e)
            false
        }
    }

    @JavascriptInterface
    fun playNext(): Boolean {
        return try {
            Log.d(tag, "JS -> playNext()")
            true
        } catch (e: Throwable) {
            Log.e(tag, "playNext() FAILED", e)
            false
        }
    }

    @JavascriptInterface
    fun playPrev(): Boolean {
        return try {
            Log.d(tag, "JS -> playPrev()")
            true
        } catch (e: Throwable) {
            Log.e(tag, "playPrev() FAILED", e)
            false
        }
    }

    @JavascriptInterface
    fun getProperty(name: String): String {
        return try {
            Log.d(tag, "JS -> getProperty() | name=$name")
            ""
        } catch (e: Throwable) {
            Log.e(tag, "getProperty() FAILED", e)
            ""
        }
    }

    @JavascriptInterface
    fun setProperty(name: String, value: String): Boolean {
        return try {
            Log.d(tag, "JS -> setProperty() | name=$name value=$value")
            true
        } catch (e: Throwable) {
            Log.e(tag, "setProperty() FAILED", e)
            false
        }
    }
}
