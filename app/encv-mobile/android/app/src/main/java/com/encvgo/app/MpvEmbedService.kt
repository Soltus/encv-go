package com.encvgo.app

import android.app.Activity
import android.content.Context
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.ComposeView
import com.encvgo.combolite.EncvComboLiteHost
import java.io.File

object MpvEmbedService {
    private const val TAG = "MpvEmbed"
    private const val MPV_PLUGIN_ID = "com.encvgo.plugin.mpv"
    private var composeView: ComposeView? = null
    private var isEmbedActive = false

    fun startEmbed(
        activity: Activity,
        containerId: String,
        filePath: String,
        fileName: String,
        mimeType: String = "",
        isExternal: Boolean = false
    ): PlayResult {
        LogBridge.i(TAG, "[ModeA-Compose] startEmbed: filePath=$filePath fileName=$fileName containerId=$containerId")

        if (isEmbedActive) {
            LogBridge.w(TAG, "[ModeA-Compose] embed already active, stopping first")
            stopEmbed()
        }

        if (!EncvComboLiteHost.isInitialized) {
            LogBridge.w(TAG, "[ModeA-Compose] ComboLite not initialized")
            return PlayResult(false, "播放器框架未初始化", "PluginManager.isInitialized=false")
        }

        val state = EncvComboLiteHost.getPluginFullState(MPV_PLUGIN_ID)
        if (state.status == "not_installed") {
            return PlayResult(false, "MPV 播放器未安装", "请前往扩展管理安装")
        }
        if (state.status == "disabled") {
            return PlayResult(false, "MPV 播放器已禁用", "请前往扩展管理启用")
        }
        if (state.status == "not_loaded" || state.status == "load_failed") {
            val loaded = EncvComboLiteHost.ensurePluginLoaded(MPV_PLUGIN_ID)
            if (!loaded) return PlayResult(false, "MPV 加载失败", "请重启应用或重新启用扩展")
        }

        try {
            LogBridge.i(TAG, "[ModeA-Compose] creating ComposeView and attaching to container")
            composeView = ComposeView(activity).apply {
                setContent {
                    MpvEmbedPlaceholder(filePath = filePath, fileName = fileName)
                }
            }
            isEmbedActive = true
            LogBridge.i(TAG, "[ModeA-Compose] embed started ✓ (placeholder mode)")
            return PlayResult(true)
        } catch (e: Exception) {
            LogBridge.e(TAG, "[ModeA-Compose] startEmbed failed: ${e.message}", e)
            return PlayResult(false, "嵌入播放器启动失败", e.message ?: "Unknown error")
        }
    }

    fun stopEmbed(): Boolean {
        LogBridge.i(TAG, "[ModeA-Compose] stopEmbed: isEmbedActive=$isEmbedActive")
        if (!isEmbedActive) return true
        try {
            composeView?.let { view ->
                (view.parent as? android.view.ViewGroup)?.removeView(view)
                composeView = null
            }
            isEmbedActive = false
            LogBridge.i(TAG, "[ModeA-Compose] stopped ✓")
            return true
        } catch (e: Exception) {
            LogBridge.e(TAG, "[ModeA-Compose] stopEmbed failed: ${e.message}", e)
            isEmbedActive = false
            return false
        }
    }

    fun isEmbedded(): Boolean = isEmbedActive

    fun getComposeView(): ComposeView? = composeView
}

@Composable
private fun MpvEmbedPlaceholder(filePath: String, fileName: String) {
    androidx.compose.foundation.layout.Box(
        modifier = Modifier.fillMaxSize(),
        contentAlignment = androidx.compose.ui.Alignment.Center
    ) {
        androidx.compose.material3.Text(
            text = "MPV Compose Embed\n$fileName\n(experimental placeholder)",
            color = androidx.compose.ui.graphics.Color.White.copy(alpha = 0.6f)
        )
    }
}
