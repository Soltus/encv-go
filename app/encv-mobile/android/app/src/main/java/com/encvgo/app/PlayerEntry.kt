package com.encvgo.app

import android.content.Context
import android.content.Intent
import androidx.core.content.FileProvider
import com.combo.core.model.LoadedPluginInfo
import com.encvgo.combolite.EncvComboLiteHost
import java.io.File

data class PlayResult(
    val success: Boolean,
    val error: String = "",
    val errorDetail: String = ""
)

object PlayerEntry {
    private const val TAG = "PlayerEntry"
    private const val PLUGIN_ID = "com.encvgo.plugin.mpv"
    private const val TARGET_ACTIVITY = "com.encvgo.plugin.mpv.MpvPlayerActivity"
    private const val PREFS_NAME = "encv_player_prefs"
    private const val PREF_KEY_VIDEO_PLAYER = "video_player"
    const val EXTRA_FILE_PATH = "file_path"
    const val EXTRA_FILE_NAME = "file_name"
    const val EXTRA_MIME_TYPE = "mime_type"
    const val EXTRA_IS_EXTERNAL = "is_external"
    const val EXTRA_BACKEND_URL = "backend_url"
    const val EXTRA_MODE = "player_mode"

    private const val MODE_MPV_ACTIVITY = "mpv-activity"
    private const val MODE_MPV_FRAGMENT = "mpv-fragment"
    private const val MODE_MPV_COMPOSE = "mpv-compose"

    fun play(
        context: Context,
        filePath: String,
        fileName: String,
        mimeType: String = "",
        isExternal: Boolean = false,
        mode: String = ""
    ): PlayResult {
        val effectiveMode = resolveMode(mode, context)
        LogBridge.i(TAG, "play() mode=$effectiveMode (param=$mode) filePath=$filePath fileName=$fileName")

        return when {
            effectiveMode.startsWith("mpv-") -> startMpvPlayer(context, filePath, fileName, mimeType, isExternal, effectiveMode)
            effectiveMode == "external" -> openExternal(context, filePath)
            else -> startArtPlayer(context, filePath, fileName)
        }
    }

    private fun resolveMode(paramMode: String, context: Context): String {
        if (paramMode.isNotEmpty()) {
            return when (paramMode) {
                "mpv", "mpv-plugin" -> MODE_MPV_ACTIVITY
                MODE_MPV_ACTIVITY, MODE_MPV_FRAGMENT, MODE_MPV_COMPOSE -> paramMode
                else -> paramMode
            }
        }
        val prefs = context.getSharedPreferences(PREFS_NAME, Context.MODE_PRIVATE)
        val rawMode = prefs.getString(PREF_KEY_VIDEO_PLAYER, "artplayer") ?: "artplayer"
        return when (rawMode) {
            "mpv", "mpv-plugin" -> MODE_MPV_ACTIVITY
            else -> rawMode
        }
    }

    fun isMpvAvailable(context: Context): Boolean = EncvComboLiteHost.isPluginAvailable(PLUGIN_ID)

    private fun startMpvPlayer(
        context: Context,
        filePath: String,
        fileName: String,
        mimeType: String,
        isExternal: Boolean,
        mode: String
    ): PlayResult {
        LogBridge.i(TAG, "[ModeDispatch] startMpvPlayer mode=$mode filePath=$filePath fileName=$fileName")

        return when (mode) {
            MODE_MPV_ACTIVITY -> {
                LogBridge.i(TAG, "[ModeC-Activity] dispatching to startMpvViaActivity")
                startMpvViaActivity(context, filePath, fileName, mimeType, isExternal)
            }
            MODE_MPV_FRAGMENT -> {
                LogBridge.i(TAG, "[ModeB-Fragment] not yet implemented, falling back to ModeC")
                startMpvViaActivity(context, filePath, fileName, mimeType, isExternal)
            }
            MODE_MPV_COMPOSE -> {
                LogBridge.i(TAG, "[ModeA-Compose] not yet implemented, falling back to ModeC")
                startMpvViaActivity(context, filePath, fileName, mimeType, isExternal)
            }
            else -> {
                LogBridge.i(TAG, "[ModeC-Activity] fallback for unknown mode=$mode")
                startMpvViaActivity(context, filePath, fileName, mimeType, isExternal)
            }
        }
    }

    private fun startMpvViaActivity(
        context: Context,
        filePath: String,
        fileName: String,
        mimeType: String,
        isExternal: Boolean
    ): PlayResult {
        LogBridge.i(TAG, "[ModeC-Activity] startMpvViaActivity: filePath=$filePath fileName=$fileName mimeType=$mimeType")

        // 1. 检查框架初始化
        if (!EncvComboLiteHost.isInitialized) {
            LogBridge.w(TAG, "[ModeC-Activity] ComboLite not initialized")
            return PlayResult(false, "播放器框架未初始化", "PluginManager.isInitialized=false")
        }
        LogBridge.i(TAG, "[ModeC-Activity] ComboLite initialized ✓")

        // 2. 检查插件完整状态
        val state = EncvComboLiteHost.getPluginFullState(PLUGIN_ID)
        LogBridge.i(TAG, "[ModeC-Activity] plugin state=$state.status name=${state.name} version=${state.version}")

        when (state.status) {
            "not_installed" -> {
                LogBridge.w(TAG, "[ModeC-Activity] MPV plugin not installed")
                return PlayResult(false, "MPV 播放器未安装", "请前往扩展管理安装")
            }
            "disabled" -> {
                LogBridge.w(TAG, "[ModeC-Activity] MPV plugin disabled")
                return PlayResult(false, "MPV 播放器已禁用", "请前往扩展管理启用")
            }
            "framework_not_ready" -> {
                LogBridge.w(TAG, "[ModeC-Activity] framework not ready")
                return PlayResult(false, "播放器框架未就绪", "请重启应用")
            }
            "not_loaded", "load_failed" -> {
                LogBridge.w(TAG, "[ModeC-Activity] plugin state=${state.status}, attempting load...")
                val loaded = EncvComboLiteHost.ensurePluginLoaded(PLUGIN_ID)
                LogBridge.i(TAG, "[ModeC-Activity] ensurePluginLoaded result=$loaded")
                if (!loaded) {
                    return PlayResult(false, "MPV 加载失败", "请重启应用或重新启用扩展")
                }
            }
        }

        // 3. 关键：检查 LoadedPluginInfo 是否包含目标 Activity
        val loadedInfo = EncvComboLiteHost.getLoadedPluginInfo(PLUGIN_ID)
        if (loadedInfo == null) {
            LogBridge.e(TAG, "[ModeC-Activity] getLoadedPluginInfo returned null after successful load!")
            return PlayResult(false, "MPV 插件信息异常", "已加载但 getLoadedPluginInfo 返回 null")
        }
        LogBridge.i(TAG, "[ModeC-Activity] loadedInfo id=${loadedInfo.pluginInfo.id} name=${loadedInfo.pluginInfo.name}")

        // 4. 启动播放 — 此时才真正 startActivity（ProxyManager 内部会验证 Activity 是否存在）
        return try {
            val extras = mapOf<String, Any>(
                EXTRA_FILE_PATH to filePath,
                EXTRA_FILE_NAME to fileName,
                EXTRA_MIME_TYPE to mimeType,
                EXTRA_IS_EXTERNAL to isExternal,
                EXTRA_BACKEND_URL to getBackendBaseUrl(context),
                EXTRA_MODE to "mpv-plugin",
            )
            val intent = EncvComboLiteHost.createProxyIntent(
                context = context,
                pluginId = PLUGIN_ID,
                targetActivity = TARGET_ACTIVITY,
                hostActivityClass = EncvHostActivity::class.java,
                extras = extras
            )
            context.startActivity(intent)
            LogBridge.i(TAG, "[ModeC-Activity] startActivity dispatched ✓ (result=pending, verify in EncvHostActivity)")
            PlayResult(true)
        } catch (e: Exception) {
            LogBridge.e(TAG, "[ModeC-Activity] startActivity failed: ${e.message}", e)
            PlayResult(false, "播放器启动失败", e.message ?: "Unknown error")
        }
    }

    fun buildMpvIntent(
        context: Context,
        filePath: String,
        fileName: String,
        mimeType: String,
        isExternal: Boolean
    ): Pair<Intent?, PlayResult> {
        LogBridge.i(TAG, "[ModeC-Activity] buildMpvIntent: filePath=$filePath fileName=$fileName")

        if (!EncvComboLiteHost.isInitialized) {
            LogBridge.w(TAG, "[ModeC-Activity] ComboLite not initialized")
            return Pair(null, PlayResult(false, "播放器框架未初始化", "PluginManager.isInitialized=false"))
        }

        val state = EncvComboLiteHost.getPluginFullState(PLUGIN_ID)
        when (state.status) {
            "not_installed" -> return Pair(null, PlayResult(false, "MPV 播放器未安装", "请前往扩展管理安装"))
            "disabled" -> return Pair(null, PlayResult(false, "MPV 播放器已禁用", "请前往扩展管理启用"))
            "framework_not_ready" -> return Pair(null, PlayResult(false, "播放器框架未就绪", "请重启应用"))
            "not_loaded", "load_failed" -> {
                val loaded = EncvComboLiteHost.ensurePluginLoaded(PLUGIN_ID)
                if (!loaded) return Pair(null, PlayResult(false, "MPV 加载失败", "请重启应用或重新启用扩展"))
            }
        }

        val loadedInfo = EncvComboLiteHost.getLoadedPluginInfo(PLUGIN_ID)
        if (loadedInfo == null) return Pair(null, PlayResult(false, "MPV 插件信息异常", "getLoadedPluginInfo 返回 null"))

        return try {
            val extras = mapOf<String, Any>(
                EXTRA_FILE_PATH to filePath,
                EXTRA_FILE_NAME to fileName,
                EXTRA_MIME_TYPE to mimeType,
                EXTRA_IS_EXTERNAL to isExternal,
                EXTRA_BACKEND_URL to getBackendBaseUrl(context),
                EXTRA_MODE to "mpv-plugin",
            )
            val intent = EncvComboLiteHost.createProxyIntent(
                context = context,
                pluginId = PLUGIN_ID,
                targetActivity = TARGET_ACTIVITY,
                hostActivityClass = EncvHostActivity::class.java,
                extras = extras
            )
            LogBridge.i(TAG, "[ModeC-Activity] buildMpvIntent ✓ intent built successfully")
            Pair(intent, PlayResult(true))
        } catch (e: Exception) {
            LogBridge.e(TAG, "[ModeC-Activity] buildMpvIntent failed: ${e.message}", e)
            Pair(null, PlayResult(false, "播放器 Intent 构建失败", e.message ?: "Unknown error"))
        }
    }

    private fun startArtPlayer(context: Context, filePath: String, fileName: String): PlayResult {
        LogBridge.i(TAG, "startArtPlayer: filePath=$filePath fileName=$fileName")
        return try {
            val intent = Intent(context, PlayerActivityCapacitor::class.java).apply {
                putExtra(EXTRA_FILE_PATH, filePath)
                putExtra(EXTRA_FILE_NAME, fileName)
                if (context !is android.app.Activity) addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
            }
            context.startActivity(intent)
            LogBridge.i(TAG, "startArtPlayer: success ✓")
            PlayResult(true)
        } catch (e: Exception) {
            LogBridge.e(TAG, "startArtPlayer: failed: ${e.message}", e)
            PlayResult(false, "内置播放器启动失败", e.message ?: "Unknown error")
        }
    }

    private fun openExternal(context: Context, filePath: String): PlayResult {
        LogBridge.i(TAG, "openExternal: filePath=$filePath")
        val file = File(filePath)
        if (!file.exists()) {
            LogBridge.w(TAG, "openExternal: file does not exist: $filePath")
            return PlayResult(false, "文件不存在", "path=$filePath")
        }
        return try {
            val uri = FileProvider.getUriForFile(context, "${context.packageName}.fileprovider", file)
            val intent = Intent(Intent.ACTION_VIEW).apply {
                setDataAndType(uri, "*/*")
                addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION or Intent.FLAG_ACTIVITY_NEW_TASK)
            }
            if (intent.resolveActivity(context.packageManager) != null) {
                context.startActivity(intent)
                LogBridge.i(TAG, "openExternal: success ✓")
                PlayResult(true)
            } else {
                LogBridge.w(TAG, "openExternal: no app can open this file")
                PlayResult(false, "没有应用可以打开此文件", "resolveActivity=null")
            }
        } catch (e: Exception) {
            LogBridge.e(TAG, "openExternal: failed: ${e.message}", e)
            PlayResult(false, "外部打开失败", e.message ?: "Unknown error")
        }
    }

    private fun getBackendBaseUrl(context: Context): String {
        val port = EncvGoService.lastKnownPort
        return if (port > 0) "http://127.0.0.1:$port" else "http://127.0.0.1:2025"
    }
}