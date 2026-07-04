package com.encvgo.plugin.mpv

import android.os.Bundle
import android.view.ViewGroup
import androidx.activity.compose.setContent
import androidx.compose.ui.graphics.Color
import com.combo.core.component.activity.BasePluginActivity
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.ui.Modifier
import com.encvgo.plugin.mpv.theme.EncvMpVPlayerTheme

private val AUDIO_EXTENSIONS = setOf(
    "mp3", "flac", "wav", "ogg", "aac", "m4a", "wma", "opus", "alac", "ape", "aiff", "mid", "midi"
)

private fun isAudioFile(mimeType: String, fileName: String): Boolean {
    if (mimeType.startsWith("audio/", ignoreCase = true)) return true
    val ext = fileName.substringAfterLast('.', "").lowercase()
    return ext in AUDIO_EXTENSIONS
}

class MpvPlayerActivity : BasePluginActivity() {

    companion object {
        private const val TAG = "MpvPlayerActivity"
    }

    private lateinit var engine: MpvEngine

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)

        val host = proxyActivity ?: return
        val hostIntent = host.intent ?: return

        val filePath = hostIntent.getStringExtra("file_path") ?: ""
        val fileName = hostIntent.getStringExtra("file_name") ?: ""
        val mimeType = hostIntent.getStringExtra("mime_type") ?: ""
        val isExternal = hostIntent.getBooleanExtra("is_external", false)
        val backendUrl = hostIntent.getStringExtra("backend_url") ?: ""
        val audioMode = isAudioFile(mimeType, fileName)

        android.util.Log.i(TAG, "onCreate: filePath=$filePath backendUrl=$backendUrl audioMode=$audioMode")

        engine = createMpvEngine(host)
        engine.initialize()

        host.setContent {
            EncvMpVPlayerTheme {
                Surface(
                    modifier = Modifier.fillMaxSize(),
                    color = if (audioMode) MaterialTheme.colorScheme.background else Color.Transparent
                ) {
                    if (audioMode) {
                        MpvAudioPlayerScreen(
                            filePath = filePath,
                            fileName = fileName,
                            mimeType = mimeType,
                            isExternal = isExternal,
                            backendUrl = backendUrl,
                            engine = engine,
                            onBack = { host.finish() }
                        )
                    } else {
                        MpvPlayerScreen(
                            filePath = filePath,
                            fileName = fileName,
                            mimeType = mimeType,
                            isExternal = isExternal,
                            backendUrl = backendUrl,
                            engine = engine,
                            onBack = { host.finish() }
                        )
                    }
                }
            }
        }

        if (!audioMode) {
            val decorView = host.window?.decorView as? ViewGroup
            if (decorView != null) {
                val contentRoot = decorView.findViewById<ViewGroup>(android.R.id.content)
                if (contentRoot != null) {
                    engine.attachSurfaceView(contentRoot)
                }
            }
        }
    }

    override fun onDestroy() {
        super.onDestroy()
        try {
            engine.destroy()
        } catch (_: Exception) {}
    }

    private fun createMpvEngine(host: androidx.activity.ComponentActivity): MpvEngine {
        return MpvEngine(host).also { engine ->
            engine.eventListener = { event ->
                when (event) {
                    is MpvEngine.Event.Pause -> { }
                    is MpvEngine.Event.Unpause -> { }
                    is MpvEngine.Event.EndFile -> host.finish()
                    is MpvEngine.Event.Shutdown -> host.finish()
                    is MpvEngine.Event.PlaybackRestart -> { }
                    else -> { }
                }
            }
            engine.logListener = { msg ->
                android.util.Log.d("MpvEngine", "[${msg.prefix}] ${msg.text}")
            }
        }
    }
}
