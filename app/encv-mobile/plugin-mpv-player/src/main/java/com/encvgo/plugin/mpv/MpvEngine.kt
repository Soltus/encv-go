package com.encvgo.plugin.mpv

import android.content.Context
import android.graphics.Bitmap
import android.os.Handler
import android.os.Looper
import android.view.Surface
import android.view.SurfaceHolder
import android.view.SurfaceView
import android.view.View
import android.view.ViewGroup
import `is`.xyz.mpv.MPVLib
import `is`.xyz.mpv.MPVLib.EventObserver
import `is`.xyz.mpv.MPVLib.LogObserver
import `is`.xyz.mpv.MPVLib.MpvEvent
import `is`.xyz.mpv.MPVLib.MpvFormat
import java.io.File
import java.util.concurrent.CopyOnWriteArrayList

class MpvEngine(private val context: Context) {

    companion object {
        private const val TAG = "MpvEngine"
    }

    var eventListener: ((Event) -> Unit)? = null
    var propertyChangeListener: ((PropertyChange) -> Unit)? = null
    var logListener: ((LogMessage) -> Unit)? = null

    sealed class Event {
        data class Seek(val value: Long) : Event()
        data class EndFile(val value: Int) : Event()
        object Pause : Event()
        object Unpause : Event()
        object FileLoaded : Event()
        data class VideoConfig(val width: Int, val height: Int, val dwidth: Int, val dheight: Int) : Event()
        data class AudioConfig(val channels: Int, val sampleRate: Int) : Event()
        data class LogMessage(val prefix: String, val level: Int, val text: String) : Event()
        object Shutdown : Event()
        object PlaybackRestart : Event()
    }

    sealed class PropertyChange {
        data class TimePosition(val value: Long) : PropertyChange()
        data class Duration(val value: Double) : PropertyChange()
        data class Paused(val value: Boolean) : PropertyChange()
        data class Idle(val value: Boolean) : PropertyChange()
        data class EndFileReason(val value: Long) : PropertyChange()
    }

    data class LogMessage(val prefix: String, val level: Int, val text: String)

    sealed class State {
        object MpvReady : State()
        object SurfaceReady : State()
        object Playing : State()
        object Paused : State()
        object Ended : State()
        object AudioOnly : State()
        object WaitingSurface : State()
        data class Error(val message: String) : State()
    }

    var stateListener: ((State) -> Unit)? = null

    private val mainHandler = Handler(Looper.getMainLooper())
    private var mpvSurfaceView: MpvSurfaceView? = null
    private var initialized = false
    private var surfaceReady = false
    private var pendingUrl: String? = null

    private val eventObserver = object : EventObserver {
        override fun eventProperty(property: String) {}
        override fun eventProperty(property: String, value: Long) {
            when (property) {
                "end-file-reason" -> {
                    if (value == 3L) {
                        mainHandler.post {
                            try {
                                val errorText = MPVLib.getPropertyString("error-text") ?: "Unknown playback error"
                                notifyState(State.Error("Playback failed: $errorText"))
                            } catch (_: Exception) {
                                notifyState(State.Error("Playback failed"))
                            }
                        }
                    }
                    mainHandler.post { propertyChangeListener?.invoke(PropertyChange.EndFileReason(value)) }
                }
                "time-pos" ->
                    mainHandler.post { propertyChangeListener?.invoke(PropertyChange.TimePosition(value)) }
            }
        }
        override fun eventProperty(property: String, value: Double) {
            when (property) {
                "duration" ->
                    mainHandler.post { propertyChangeListener?.invoke(PropertyChange.Duration(value)) }
            }
        }
        override fun eventProperty(property: String, value: String) {}
        override fun eventProperty(property: String, value: Boolean) {
            when (property) {
                "pause" -> {
                    mainHandler.post {
                        if (value) {
                            eventListener?.invoke(Event.Pause)
                            notifyState(State.Paused)
                        } else {
                            eventListener?.invoke(Event.Unpause)
                            notifyState(State.Playing)
                        }
                    }
                    mainHandler.post { propertyChangeListener?.invoke(PropertyChange.Paused(value)) }
                }
                "idle" -> {
                    if (value) {
                        mainHandler.post { notifyState(State.Ended) }
                    }
                    mainHandler.post { propertyChangeListener?.invoke(PropertyChange.Idle(value)) }
                }
            }
        }
        override fun event(eventId: Int) {
            when (eventId) {
                MpvEvent.MPV_EVENT_FILE_LOADED -> {
                    mainHandler.postDelayed({
                        try {
                            val videoWidth = try { MPVLib.getPropertyInt("width") ?: 0 } catch (_: Exception) { 0 }
                            val videoHeight = try { MPVLib.getPropertyInt("height") ?: 0 } catch (_: Exception) { 0 }
                            val isAudioOnly = videoWidth == 0 || videoHeight == 0
                            if (isAudioOnly) {
                                mainHandler.post { mpvSurfaceView?.visibility = View.GONE }
                                notifyState(State.AudioOnly)
                            } else {
                                mainHandler.post { mpvSurfaceView?.visibility = View.VISIBLE }
                                notifyState(State.Playing)
                            }
                        } catch (_: Exception) {
                            notifyState(State.Playing)
                        }
                    }, 500)
                    mainHandler.post { eventListener?.invoke(Event.FileLoaded) }
                }
                MpvEvent.MPV_EVENT_END_FILE -> {
                    try {
                        val reason = MPVLib.getPropertyInt("end-file-reason") ?: 0
                        if (reason != 3) {
                            notifyState(State.Ended)
                            mainHandler.post { eventListener?.invoke(Event.EndFile(reason)) }
                        }
                    } catch (_: Exception) {
                        notifyState(State.Ended)
                    }
                }
                MpvEvent.MPV_EVENT_SHUTDOWN -> {
                    notifyState(State.Ended)
                    mainHandler.post { eventListener?.invoke(Event.Shutdown) }
                }
                MpvEvent.MPV_EVENT_SEEK -> {
                    mainHandler.post { eventListener?.invoke(Event.Seek(0L)) }
                }
                MpvEvent.MPV_EVENT_PLAYBACK_RESTART -> {
                    mainHandler.post { eventListener?.invoke(Event.PlaybackRestart) }
                }
            }
        }
    }

    private val logObserver = object : LogObserver {
        override fun logMessage(prefix: String, level: Int, text: String) {
            mainHandler.post {
                logListener?.invoke(LogMessage(prefix, level, text))
                eventListener?.invoke(Event.LogMessage(prefix, level, text))
            }
        }
    }

    fun initialize(): Boolean {
        if (initialized) return true
        return try {
            val configDir = context.filesDir.absolutePath + "/mpv"
            val cacheDir = context.cacheDir.absolutePath + "/mpv"
            File(configDir).mkdirs()
            File(cacheDir).mkdirs()

            MPVLib.create(context)
            MPVLib.setOptionString("config", "yes")
            MPVLib.setOptionString("config-dir", configDir)
            for (opt in arrayOf("gpu-shader-cache-dir", "icc-cache-dir")) {
                MPVLib.setOptionString(opt, cacheDir)
            }
            MPVLib.setOptionString("vo", "gpu")
            MPVLib.setOptionString("hwdec", "auto")
            MPVLib.init()
            MPVLib.setOptionString("force-window", "no")
            MPVLib.setOptionString("idle", "once")

            setupObservers()
            initialized = true
            notifyState(State.MpvReady)
            true
        } catch (e: Exception) {
            notifyState(State.Error("MPV init failed: ${e.message}"))
            false
        }
    }

    fun destroy() {
        if (!initialized) return
        try {
            MPVLib.removeObserver(eventObserver)
            MPVLib.removeLogObserver(logObserver)
        } catch (_: Exception) {}
        try {
            MPVLib.destroy()
        } catch (_: Exception) {}
        initialized = false
        surfaceReady = false
        pendingUrl = null
        mpvSurfaceView = null
    }

    fun attachSurfaceView(rootLayout: ViewGroup) {
        if (mpvSurfaceView != null && mpvSurfaceView?.parent != null) return
        try {
            initialize()
            mpvSurfaceView = MpvSurfaceView(rootLayout.context).apply {
                id = ViewGroup.generateViewId()
                keepScreenOn = true
            }
            val params = ViewGroup.LayoutParams(
                ViewGroup.LayoutParams.MATCH_PARENT,
                ViewGroup.LayoutParams.MATCH_PARENT
            )
            rootLayout.addView(mpvSurfaceView, 0, params)
            mpvSurfaceView?.visibility = View.VISIBLE
        } catch (e: Exception) {
            notifyState(State.Error("Surface attach failed: ${e.message}"))
        }
    }

    fun detachSurfaceView(rootLayout: ViewGroup) {
        try {
            mpvSurfaceView?.let { rootLayout.removeView(it) }
        } catch (_: Exception) {}
    }

    fun isAttached(): Boolean = mpvSurfaceView != null && mpvSurfaceView?.parent != null

    fun isInitialized(): Boolean = initialized

    fun play(filePath: String): Boolean {
        return try {
            if (!initialize()) return false
            if (surfaceReady) {
                MPVLib.command(arrayOf("loadfile", filePath))
            } else {
                pendingUrl = filePath
                notifyState(State.WaitingSurface)
            }
            true
        } catch (e: Exception) {
            notifyState(State.Error("Play failed: ${e.message}"))
            false
        }
    }

    fun playAudioOnly(filePath: String): Boolean {
        return try {
            if (!initialize()) return false
            MPVLib.setPropertyString("vo", "null")
            MPVLib.command(arrayOf("loadfile", filePath))
            true
        } catch (e: Exception) {
            notifyState(State.Error("Audio play failed: ${e.message}"))
            false
        }
    }

    fun pause() {
        try {
            if (!initialized) return
            MPVLib.setPropertyBoolean("pause", true)
        } catch (_: Exception) {}
    }

    fun resume() {
        try {
            if (!initialized) return
            MPVLib.setPropertyBoolean("pause", false)
        } catch (_: Exception) {}
    }

    fun stop() {
        try {
            if (!initialized) return
            MPVLib.command(arrayOf("stop"))
        } catch (_: Exception) {}
    }

    fun seek(posMs: Long) {
        try {
            if (!initialized) return
            val posSec = posMs / 1000.0
            MPVLib.command(arrayOf("seek", posSec.toString(), "absolute+keyframes"))
        } catch (_: Exception) {}
    }

    fun seekDelta(deltaMs: Long) {
        try {
            if (!initialized) return
            val deltaSec = deltaMs / 1000.0
            MPVLib.command(arrayOf("seek", deltaSec.toString()))
        } catch (_: Exception) {}
    }

    fun setSpeed(speed: Float) {
        try {
            if (!initialized) return
            MPVLib.setPropertyDouble("speed", speed.toDouble())
        } catch (_: Exception) {}
    }

    fun getSpeed(): Float {
        return try {
            if (!initialized) 1.0f
            else MPVLib.getPropertyDouble("speed")?.toFloat() ?: 1.0f
        } catch (_: Exception) { 1.0f }
    }

    fun getPosition(): Long {
        return try {
            if (!initialized) 0L
            else ((MPVLib.getPropertyDouble("time-pos") ?: 0.0) * 1000).toLong()
        } catch (_: Exception) { 0L }
    }

    fun getDuration(): Long {
        return try {
            if (!initialized) 0L
            else ((MPVLib.getPropertyDouble("duration") ?: 0.0) * 1000).toLong()
        } catch (_: Exception) { 0L }
    }

    fun isPlaying(): Boolean {
        return try {
            if (!initialized) false
            else !(MPVLib.getPropertyBoolean("pause") ?: true)
        } catch (_: Exception) { false }
    }

    fun isPaused(): Boolean {
        return try {
            if (!initialized) true
            else MPVLib.getPropertyBoolean("pause") ?: true
        } catch (_: Exception) { true }
    }

    fun setVolume(volume: Float) {
        try {
            if (!initialized) return
            MPVLib.setPropertyDouble("volume", volume.toDouble())
        } catch (_: Exception) {}
    }

    fun getVolume(): Float {
        return try {
            if (!initialized) 100f
            else MPVLib.getPropertyDouble("volume")?.toFloat() ?: 100f
        } catch (_: Exception) { 100f }
    }

    fun cycleAudioTrack() {
        try {
            if (!initialized) return
            MPVLib.command(arrayOf("cycle", "audio"))
        } catch (_: Exception) {}
    }

    fun cycleSubtitleTrack() {
        try {
            if (!initialized) return
            MPVLib.command(arrayOf("cycle", "sub"))
        } catch (_: Exception) {}
    }

    fun toggleSubtitleVisibility() {
        try {
            if (!initialized) return
            val current = MPVLib.getPropertyBoolean("sub-visibility") ?: false
            MPVLib.setPropertyBoolean("sub-visibility", !current)
        } catch (_: Exception) {}
    }

    fun takeThumbnail(dimension: Int = 256): Bitmap? {
        return try {
            if (!initialized) null
            else MPVLib.grabThumbnail(dimension)
        } catch (_: Exception) { null }
    }

    data class TrackInfo(val id: Int, val type: String, val lang: String?, val title: String?, val codec: String?, val default: Boolean)

    fun getTrackList(type: String): List<TrackInfo> {
        if (!initialized) return emptyList()
        return try {
            val count = MPVLib.getPropertyInt("track-list/count") ?: 0
            (0 until count).mapNotNull { i ->
                val trackType = MPVLib.getPropertyString("track-list/$i/type") ?: return@mapNotNull null
                if (trackType != type) return@mapNotNull null
                val id = MPVLib.getPropertyInt("track-list/$i/id") ?: return@mapNotNull null
                val lang = MPVLib.getPropertyString("track-list/$i/lang")
                val title = MPVLib.getPropertyString("track-list/$i/title")
                val codec = MPVLib.getPropertyString("track-list/$i/codec")
                val default = MPVLib.getPropertyBoolean("track-list/$i/default") ?: false
                TrackInfo(id, trackType, lang, title, codec, default)
            }
        } catch (_: Exception) {
            emptyList()
        }
    }

    fun setTrack(type: String, id: Int) {
        try {
            if (!initialized) return
            MPVLib.setPropertyString(type, id.toString())
        } catch (_: Exception) {}
    }

    fun addSubtitleFile(path: String): Boolean {
        return try {
            if (!initialized) false
            else { MPVLib.command(arrayOf("sub-add", path)); true }
        } catch (_: Exception) { false }
    }

    fun setProperty(key: String, value: String): Boolean {
        return try {
            if (!initialized) false
            else { MPVLib.setPropertyString(key, value); true }
        } catch (_: Exception) { false }
    }

    fun getProperty(key: String): String? {
        return try {
            if (!initialized) null
            else MPVLib.getPropertyString(key)
        } catch (_: Exception) { null }
    }

    private fun setupObservers() {
        MPVLib.addObserver(eventObserver)
        MPVLib.addLogObserver(logObserver)
        MPVLib.observeProperty("pause", MpvFormat.MPV_FORMAT_FLAG)
        MPVLib.observeProperty("idle", MpvFormat.MPV_FORMAT_FLAG)
        MPVLib.observeProperty("end-file-reason", MpvFormat.MPV_FORMAT_INT64)
        MPVLib.observeProperty("time-pos", MpvFormat.MPV_FORMAT_DOUBLE)
        MPVLib.observeProperty("duration", MpvFormat.MPV_FORMAT_DOUBLE)
    }

    private fun notifyState(state: State) {
        stateListener?.invoke(state)
    }

    private inner class MpvSurfaceView(ctx: Context) :
        SurfaceView(ctx), SurfaceHolder.Callback {

        init {
            holder.addCallback(this)
        }

        override fun surfaceCreated(holder: SurfaceHolder) {
            try {
                MPVLib.attachSurface(holder.surface)
                MPVLib.setOptionString("force-window", "yes")
                MPVLib.setPropertyString("vo", "gpu")
                surfaceReady = true
                notifyState(State.SurfaceReady)
                pendingUrl?.let { url ->
                    MPVLib.command(arrayOf("loadfile", url))
                    pendingUrl = null
                }
            } catch (e: Exception) {
                surfaceReady = false
                notifyState(State.Error("Surface create failed: ${e.message}"))
            }
        }

        override fun surfaceChanged(holder: SurfaceHolder, format: Int, width: Int, height: Int) {
            try {
                MPVLib.setPropertyString("android-surface-size", "${width}x$height")
            } catch (_: Exception) {}
        }

        override fun surfaceDestroyed(holder: SurfaceHolder) {
            try {
                MPVLib.setPropertyString("vo", "null")
                MPVLib.setPropertyString("force-window", "no")
                MPVLib.detachSurface()
            } catch (_: Exception) {}
            surfaceReady = false
        }
    }
}
