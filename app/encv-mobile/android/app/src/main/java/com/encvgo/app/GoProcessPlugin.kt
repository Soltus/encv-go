package com.encvgo.app

import android.Manifest
import android.app.Activity
import android.content.Context
import android.content.Intent
import android.content.BroadcastReceiver
import android.content.IntentFilter
import android.net.Uri
import android.util.Log
import android.os.Build
import android.os.Handler
import android.os.Looper
import android.content.pm.ActivityInfo
import androidx.core.content.ContextCompat
import com.getcapacitor.JSObject
import com.getcapacitor.Plugin
import com.getcapacitor.PluginCall
import com.getcapacitor.PluginMethod
import com.getcapacitor.annotation.CapacitorPlugin
import com.encvgo.app.workers.EncvTaskCancelWorker
import com.encvgo.combolite.EncvComboLiteHost
import com.encvgo.combolite.OpenListStatusBridge
import com.encvgo.combolite.diagnostic.DiagnosticKit
import com.encvgo.combolite.model.OperationResult
import com.encvgo.combolite.model.PluginFullState
import java.io.File
import java.util.concurrent.ConcurrentHashMap
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.GlobalScope
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

private const val REQUEST_CODE_PLUGIN_PICK = 9001
private const val REQUEST_CODE_INSTALL_CONFIRM = 9002
private const val REQUEST_CODE_MPV_PLAYER = 9003
private const val REQUEST_CODE_PICK_FOLDER = 9010

/**
 * Phase 26: in-process 状态推送契约（替代 Phase 22 跨进程 broadcast）。
 * plugin-openlist 通过 [com.encvgo.plugin.openlist.OpenListNativeService.statusListener]
 * （host 启动时反射注册）调用此 lambda → [notifyListeners] 推到 Capacitor。
 */
private const val EVENT_OPENLIST_STATUS = "openlist:status"

@CapacitorPlugin(
    name = "GoProcess",
    requestCodes = [REQUEST_CODE_PLUGIN_PICK, REQUEST_CODE_INSTALL_CONFIRM, REQUEST_CODE_MPV_PLAYER, REQUEST_CODE_PICK_FOLDER]
)
class GoProcessPlugin : Plugin() {

    companion object {
        private const val TAG = "ENCV-go"
        private const val EVENT_KOTLIN_LOG = "kotlin:log"

        /** plugin 实例引用，供 LogBridge 等静态上下文推送实时日志到前端 DevLogs */
        @Volatile
        private var pluginInstance: GoProcessPlugin? = null

        /**
         * 🆕 2026-07-04：静态方法，从任意 Kotlin 代码（包括 LogBridge）向 JS 端推送实时日日志。
         * 通过 Capacitor notifyListeners 直接推送，不依赖文件 IO。
         */
        fun pushKotlinLog(level: String, tag: String, message: String, stack: String? = null) {
            val inst = pluginInstance ?: return
            try {
                val js = JSObject().apply {
                    put("timestamp", java.text.SimpleDateFormat("yyyy-MM-dd HH:mm:ss.SSS", java.util.Locale.US).format(java.util.Date()))
                    put("level", level.lowercase(java.util.Locale.ROOT))
                    put("message", message)
                    put("source", tag)
                    val tagsArr = org.json.JSONArray()
                    listOf("kotlin", "android", tag.lowercase(java.util.Locale.ROOT)).forEach { tagsArr.put(it) }
                    put("tags", tagsArr)
                    stack?.let { put("stack", it) }
                }
                inst.notifyListeners(EVENT_KOTLIN_LOG, js, true)
            } catch (e: Throwable) {
                Log.w(TAG, "pushKotlinLog failed", e)
            }
        }
    }

    private val pendingCalls = ConcurrentHashMap<String, PluginCall>()
    private var receiverRegistered = false
    private var pluginListenerRegistered = false

    private val statusReceiver = object : BroadcastReceiver() {
        override fun onReceive(context: Context?, intent: Intent?) {
            if (intent == null) return
            when (intent.action) {
                EncvGoService.BROADCAST_BACKEND_READY,
                EncvGoService.BROADCAST_BACKEND_STATUS -> resolvePendingCall(intent)
            }
        }
    }

    /**
     * Phase 26: in-process 状态变更回调（替代 Phase 22 跨进程 broadcast）。
     * 通过 PluginClassLoader 反射写入 [com.encvgo.plugin.openlist.OpenListNativeService.statusListener]
     * 后，plugin 每次 broadcastStatus() 都会调此 lambda → [notifyListeners] 推 Capacitor。
     *
     * 类型选择：Kotlin 函数类型 `(Map<String, Any?>) -> Unit` —— 编译产物是
     * `Function1` interface 的匿名实现，反射 set 字段（raw type `Function1`）
     * 与 plugin 端 OpenListBridge.statusListener 字段（raw type `Function1`）兼容。
     *
     * 反例（CI 实测报错）：
     *   `kotlin.jvm.functions.Function1<Map<...>, Unit> { snap -> ... }`
     *   ❌ `Function1` 是普通 interface（不是 fun interface），没有 SAM 构造器
     *   ❌ lambda 参数 `snap` 无法推断类型
     */
    private val pluginStatusCallback: (Map<String, Any?>) -> Unit = { snap ->
        val running = (snap["running"] as? Boolean) ?: false
        val port = (snap["port"] as? Int) ?: 0
        val pid = (snap["pid"] as? Int) ?: 0
        val dataSize = (snap["data_size_bytes"] as? Long) ?: 0L
        val lastError = (snap["last_error"] as? String) ?: ""
        val lastUpdateTs = (snap["last_update_ts"] as? Long) ?: 0L
        Log.e(TAG, "[SAT-DBG][OpenList][HostReceiver] in-process callback | running=$running port=$port pid=$pid dataSize=$dataSize lastErr='$lastError'")
        val js = JSObject().apply {
            put("isInstalled", true)
            put("running", running)
            put("port", port)
            put("pid", pid)
            put("dataSizeBytes", dataSize)
            put("lastError", lastError)
            put("lastUpdateTs", lastUpdateTs)
        }
        try {
            notifyListeners(EVENT_OPENLIST_STATUS, js, true)
            Log.e(TAG, "[SAT-DBG][OpenList][HostReceiver] notifyListeners('openlist:status') OK")
        } catch (e: Throwable) {
            Log.e(TAG, "[SAT-DBG][OpenList][HostReceiver] notifyListeners FAILED", e)
        }
    }

    override fun load() {
        super.load()
        pluginInstance = this
        registerStatusReceiver()
        registerPluginStatusListener()
    }

    override fun handleOnDestroy() {
        pluginInstance = null
        if (receiverRegistered) { context.unregisterReceiver(statusReceiver); receiverRegistered = false }
        unregisterPluginStatusListener()
        pendingCalls.clear()
        super.handleOnDestroy()
    }

    @PluginMethod
    fun restart(call: PluginCall) {
        pendingCalls["restart"] = call
        startService(EncvGoService.ACTION_RESTART, "manual", "restart")
    }

    @PluginMethod
    fun stop(call: PluginCall) {
        startService(EncvGoService.ACTION_STOP, "manual", "stop")
        call.resolve(JSObject().apply { put("success", true); put("port", 0) })
    }

    @PluginMethod
    fun getStatus(call: PluginCall) {
        call.resolve(JSObject().apply {
            put("running", EncvGoService.isRunning)
            put("port", EncvGoService.lastKnownPort)
            if (!EncvGoService.lastError.isNullOrEmpty()) put("lastError", EncvGoService.lastError)
        })
    }

    @PluginMethod
    fun requestNotificationPermission(call: PluginCall) {
        PermissionHelper.requestNotificationPermission(activity, 1001)
        call.resolve(JSObject().put("granted", PermissionHelper.isNotificationGranted(context)))
    }

    @PluginMethod
    fun requestStoragePermission(call: PluginCall) {
        PermissionHelper.requestStoragePermission(activity)
        call.resolve(JSObject().apply { put("granted", false); put("requiresSettings", true) })
    }

    @PluginMethod
    fun requestBatteryOptimization(call: PluginCall) {
        PermissionHelper.requestBatteryOptimization(activity)
        call.resolve(JSObject().apply {
            put("granted", PermissionHelper.isBatteryOptimizationIgnored(context))
            if (!PermissionHelper.isBatteryOptimizationIgnored(context)) put("requiresSettings", true)
        })
    }

    @PluginMethod
    fun isStandaloneMode(call: PluginCall) {
        call.resolve(JSObject().put("standalone", activity is PlayerActivityCapacitor))
    }

    @PluginMethod
    fun getIntentFileInfo(call: PluginCall) {
        call.resolve(if (activity is PlayerActivityCapacitor) JSObject().apply {
            put("path", PlayerActivityCapacitor.intentFilePath)
            put("name", PlayerActivityCapacitor.intentFileName)
            put("mimeType", PlayerActivityCapacitor.intentFileMimeType)
        } else JSObject().apply { put("path", ""); put("name", ""); put("mimeType", "") })
    }

    @PluginMethod
    fun openPlayer(call: PluginCall) {
        try {
            val mode = call.getString("mode") ?: ""
            val effectiveMode = if (mode.isEmpty() || mode == "mpv" || mode == "mpv-plugin") "mpv-activity" else mode

            if (effectiveMode.startsWith("mpv-")) {
                LogBridge.i(TAG, "openPlayer: using startActivityForResult for mode=$effectiveMode")
                val (intent, result) = PlayerEntry.buildMpvIntent(context ?: activity!!,
                    call.getString("filePath") ?: "",
                    call.getString("name") ?: "",
                    call.getString("mimeType") ?: "",
                    isExternal = false)
                if (intent == null || !result.success) {
                    call.resolve(JSObject().apply {
                        put("success", false)
                        put("error", result.error)
                        put("errorDetail", result.errorDetail)
                    })
                } else {
                    pendingCalls["mpvPlayer"] = call
                    call.save()
                    activity.startActivityForResult(intent, REQUEST_CODE_MPV_PLAYER)
                    LogBridge.i(TAG, "openPlayer: startActivityForResult dispatched for $effectiveMode")
                    Handler(Looper.getMainLooper()).postDelayed({
                        if (pendingCalls.containsKey("mpvPlayer")) {
                            LogBridge.w(TAG, "openPlayer: mpv result timeout (15s), resolving with warning")
                            val staleCall = pendingCalls.remove("mpvPlayer")
                            try { staleCall?.resolve(JSObject().apply {
                                put("success", false)
                                put("error", "播放器响应超时")
                                put("errorDetail", "startActivityForResult dispatched but no result within 15s")
                            }) } catch (_: Exception) {}
                        }
                    }, 15000)
                }
            } else {
                val result = PlayerEntry.play(context ?: activity!!,
                    call.getString("filePath") ?: "",
                    call.getString("name") ?: "",
                    call.getString("mimeType") ?: "",
                    isExternal = false,
                    mode = mode)
                if (result.success) {
                    call.resolve(JSObject().apply { put("success", true) })
                } else {
                    call.resolve(JSObject().apply {
                        put("success", false)
                        put("error", result.error)
                        put("errorDetail", result.errorDetail)
                    })
                }
            }
        } catch (e: Exception) { call.reject(e.message) }
    }

    @PluginMethod
    fun closePlayer(call: PluginCall) = call.resolve()

    @PluginMethod
    fun openExternal(call: PluginCall) {
        try {
            context.startActivity(Intent.createChooser(Intent(Intent.ACTION_VIEW).apply {
                setDataAndType(Uri.parse(call.getString("url") ?: return@openExternal), call.getString("mimeType", "video/*"))
                addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
            }, null))
            call.resolve()
        } catch (e: Exception) { call.reject(e.message) }
    }

    @PluginMethod
    fun openInPlayer(call: PluginCall) {
        try {
            activity.startActivity(Intent(activity, PlayerActivity::class.java).apply {
                addFlags(Intent.FLAG_ACTIVITY_NEW_DOCUMENT or Intent.FLAG_ACTIVITY_MULTIPLE_TASK or Intent.FLAG_ACTIVITY_RETAIN_IN_RECENTS)
                data = Uri.parse("encvgo://player/${System.currentTimeMillis()}")
                putExtra("file_path", call.getString("path") ?: return@openInPlayer)
                putExtra("file_name", call.getString("name") ?: "")
                putExtra("file_mime_type", call.getString("mimeType") ?: "")
                putExtra(PlayerEntry.EXTRA_MODE, call.getString("mode") ?: "")
            })
            call.resolve()
        } catch (e: Exception) { call.reject(e.message) }
    }

    @PluginMethod
    fun openPlayerHome(call: PluginCall) {
        try {
            activity.startActivity(Intent(activity, PlayerActivity::class.java).apply {
                addFlags(Intent.FLAG_ACTIVITY_NEW_DOCUMENT or Intent.FLAG_ACTIVITY_MULTIPLE_TASK or Intent.FLAG_ACTIVITY_RETAIN_IN_RECENTS)
                data = Uri.parse("encvgo://player/home/${System.currentTimeMillis()}")
            })
            call.resolve()
        } catch (e: Exception) { call.reject(e.message) }
    }

    @PluginMethod
    override fun checkPermissions(call: PluginCall) {
        val s = PermissionHelper.checkAll(context)
        call.resolve(JSObject().apply { put("notifications", s.notifications); put("storage", s.storage); put("batteryOptimization", s.batteryOptimization) })
    }

    @PluginMethod
    fun setScreenOrientation(call: PluginCall) {
        try {
            when (call.getString("orientation", "unlocked")) {
                "portrait" -> activity.requestedOrientation = ActivityInfo.SCREEN_ORIENTATION_SENSOR_PORTRAIT
                "landscape" -> activity.requestedOrientation = ActivityInfo.SCREEN_ORIENTATION_SENSOR_LANDSCAPE
                "unlocked" -> activity.requestedOrientation = ActivityInfo.SCREEN_ORIENTATION_UNSPECIFIED
            }
            call.resolve()
        } catch (e: Exception) { call.reject(e.message) }
    }

    @PluginMethod
    fun installPlugin(call: PluginCall) {
        val apkPath = call.getString("apkPath") ?: run { call.reject("apkPath is required"); return }
        startInstallConfirm(call, apkPath, File(apkPath).name)
    }

    @PluginMethod
    fun pickAndInstallPlugin(call: PluginCall) {
        AppLogger.log("I", TAG, "pickAndInstallPlugin: starting file picker")
        pendingCalls["pickPlugin"] = call
        try {
            activity.startActivityForResult(Intent(Intent.ACTION_GET_CONTENT).apply {
                type = "application/vnd.android.package-archive"
                addCategory(Intent.CATEGORY_OPENABLE)
                putExtra(Intent.EXTRA_MIME_TYPES, arrayOf("application/vnd.android.package-archive"))
            }, REQUEST_CODE_PLUGIN_PICK)
        } catch (e: Exception) {
            pendingCalls.remove("pickPlugin")?.reject(e.message)
        }
    }

    @PluginMethod
    fun pickFolder(call: PluginCall) {
        pendingCalls["pickFolder"] = call
        try {
            val intent = Intent(Intent.ACTION_OPEN_DOCUMENT_TREE).apply {
                flags = Intent.FLAG_GRANT_READ_URI_PERMISSION or Intent.FLAG_GRANT_PERSISTABLE_URI_PERMISSION
            }
            activity.startActivityForResult(intent, REQUEST_CODE_PICK_FOLDER)
        } catch (e: Exception) {
            pendingCalls.remove("pickFolder")?.reject(e.message)
        }
    }

    @PluginMethod
    fun checkInstalledPlugins(call: PluginCall) {
        val result = JSObject()
        for (plugin in EncvComboLiteHost.getInstalledPlugins()) {
            result.put(plugin.id, JSObject().apply { put("installed", true); put("enabled", plugin.enabled); put("versionName", plugin.versionName) })
        }
        call.resolve(result)
    }

    @PluginMethod
    fun getPluginFullState(call: PluginCall) {
        val pluginId = call.getString("pluginId") ?: run { call.reject("pluginId required"); return }
        val state = EncvComboLiteHost.getPluginFullState(pluginId)
        call.resolve(JSObject().apply { put("id", state.id); put("status", state.status); put("name", state.name ?: ""); put("version", state.version ?: "") })
    }

    @PluginMethod
    fun getOpenListRuntime(call: PluginCall) {
        Log.e(TAG, "[SAT-DBG][OpenList][Capacitor] getOpenListRuntime() called")
        try {
            val runtime = OpenListStatusBridge.read(context.applicationContext)
            val ret = JSObject().apply {
                put("isInstalled", runtime.isInstalled)
                put("running", runtime.running)
                put("port", runtime.port)
                put("pid", runtime.pid)
                put("dataSizeBytes", runtime.dataSizeBytes)
                put("lastError", runtime.lastError ?: "")
                put("lastUpdateTs", runtime.lastUpdateTs)
            }
            Log.e(TAG, "[SAT-DBG][OpenList][Capacitor] getOpenListRuntime() → $ret")
            call.resolve(ret)
        } catch (e: Throwable) {
            Log.e(TAG, "[SAT-DBG][OpenList][Capacitor] getOpenListRuntime() FAILED", e)
            call.reject("getOpenListRuntime failed: ${e.message}")
        }
    }

    @PluginMethod
    fun controlOpenList(call: PluginCall) {
        val action = call.getString("action", "start") ?: "start"
        Log.e(TAG, "[SAT-DBG][OpenList][Capacitor] controlOpenList() action=$action")
        val args = mutableMapOf<String, Any>()
        call.getInt("port")?.let { args["port"] = it }
        call.getInt("timeout_ms")?.let { args["timeout_ms"] = it.toLong() }
        call.getString("password")?.let { args["password"] = it }

        // Phase 25 A3.2 路由：
        //   start / shutdown → 走 plugin service lifecycle (startPluginService / stopPluginService)
        //   其他 (forceDBSync / setAdminPassword) → 走 ContentProvider insert action dispatch
        val ok = when (action) {
            "start" -> {
                val extras = android.os.Bundle().apply {
                    (args["port"] as? Int)?.let { putInt("port", it) }
                }
                com.encvgo.combolite.OpenListPluginProxy.startMainService(context, extras)
            }
            "shutdown" -> {
                com.encvgo.combolite.OpenListPluginProxy.stopMainService(context)
            }
            else -> {
                // forceDBSync / setAdminPassword → OpenListStatusBridge.control
                OpenListStatusBridge.control(context.applicationContext, action, args)
            }
        }
        Log.e(TAG, "[SAT-DBG][OpenList][Capacitor] controlOpenList() action=$action → ok=$ok")
        val ret = JSObject().apply { put("success", ok) }
        call.resolve(ret)
    }

    @PluginMethod
    fun ensurePluginLoaded(call: PluginCall) {
        val pluginId = call.getString("pluginId") ?: run { call.reject("pluginId required"); return }
        val success = EncvComboLiteHost.ensurePluginLoaded(pluginId)
        call.resolve(JSObject().apply { put("success", success) })
    }

    @PluginMethod
    fun togglePluginEnabled(call: PluginCall) {
        val pluginId = call.getString("pluginId") ?: run { call.reject("pluginId required"); return }
        val enabled = call.getBoolean("enabled", true) ?: true
        GlobalScope.launch(Dispatchers.IO) {
            when (val r = EncvComboLiteHost.setPluginEnabled(pluginId, enabled)) {
                is OperationResult.Success -> withContext(Dispatchers.Main) {
                    AppLogger.log("I", TAG, "togglePluginEnabled SUCCESS: $pluginId -> ${if (enabled) "ENABLED" else "DISABLED"}")
                    call.resolve(JSObject().apply { put("success", true); put("pluginId", pluginId); put("enabled", enabled) })
                }
                is OperationResult.Failure -> withContext(Dispatchers.Main) {
                    AppLogger.log("E", TAG, "togglePluginEnabled FAILED: ${r.reason}")
                    call.reject(r.reason)
                }
            }
        }
    }

    @PluginMethod
    fun uninstallPlugin(call: PluginCall) {
        val pluginId = call.getString("pluginId") ?: run { call.reject("pluginId required"); return }
        GlobalScope.launch(Dispatchers.IO) {
            when (val r = EncvComboLiteHost.uninstallPlugin(pluginId)) {
                is OperationResult.Success -> withContext(Dispatchers.Main) {
                    AppLogger.log("I", TAG, "uninstallPlugin SUCCESS: $pluginId")
                    call.resolve(JSObject().apply { put("success", true); put("pluginId", pluginId) })
                }
                is OperationResult.Failure -> withContext(Dispatchers.Main) {
                    AppLogger.log("E", TAG, "uninstallPlugin FAILED: ${r.reason}")
                    call.reject(r.reason)
                }
            }
        }
    }

    @PluginMethod
    fun debugLifecycleFlow(call: PluginCall) {
        call.resolve(JSObject().put("debugLog", DiagnosticKit.lifecycleDiagnostic(
            call.getString("pluginId", "com.encvgo.plugin.mpv") ?: "com.encvgo.plugin.mpv", context).joinToString("\n")))
    }

    override fun handleOnActivityResult(requestCode: Int, resultCode: Int, data: Intent?) {
        super.handleOnActivityResult(requestCode, resultCode, data)
        when (requestCode) {
            REQUEST_CODE_PLUGIN_PICK -> handlePickResult(resultCode, data)
            REQUEST_CODE_INSTALL_CONFIRM -> handleInstallConfirmResult(resultCode, data)
            REQUEST_CODE_MPV_PLAYER -> handleMpvPlayerResult(resultCode, data)
            REQUEST_CODE_PICK_FOLDER -> handlePickFolderResult(resultCode, data)
        }
    }

    private fun handleMpvPlayerResult(resultCode: Int, data: Intent?) {
        val call = pendingCalls.remove("mpvPlayer") ?: return
        LogBridge.i(TAG, "handleMpvPlayerResult: resultCode=$resultCode data=$data")
        if (data != null) {
            val success = data.getBooleanExtra("player_success", true)
            val error = data.getStringExtra("player_error") ?: ""
            val errorDetail = data.getStringExtra("player_error_detail") ?: ""
            LogBridge.i(TAG, "handleMpvPlayerResult: success=$success error=$error detail=$errorDetail")
            call.resolve(JSObject().apply {
                put("success", success)
                put("error", error)
                put("errorDetail", errorDetail)
            })
        } else {
            LogBridge.w(TAG, "handleMpvPlayerResult: no intent data, assuming user back-pressed (success)")
            call.resolve(JSObject().apply { put("success", true) })
        }
    }

    private fun handlePickResult(resultCode: Int, data: Intent?) {
        val call = pendingCalls.remove("pickPlugin") ?: return
        if (resultCode != Activity.RESULT_OK || data?.data == null) { call.reject("File picker cancelled"); return }
        val tempFile = UriUtils.copyUriToFile(context, data.data!!, File(context.cacheDir, "plugin_install"))
            ?: run { call.reject("Cannot read selected file"); return }
        startInstallConfirm(call, tempFile.absolutePath, tempFile.name)
    }

    private fun handlePickFolderResult(resultCode: Int, data: Intent?) {
        val call = pendingCalls.remove("pickFolder") ?: return
        if (resultCode != Activity.RESULT_OK || data?.data == null) { call.reject("Folder picker cancelled"); return }
        val uri = data.data!!
        val path = resolveTreeUriToPath(uri)
        LogBridge.i(TAG, "pickFolder: uri=$uri → path=$path")
        call.resolve(JSObject().apply { put("path", path) })
    }

    private fun resolveTreeUriToPath(uri: Uri): String {
        if (uri.scheme == "file") return uri.path ?: "/"
        if (uri.authority == "com.android.externalstorage.documents") {
            val docId = uri.path?.removePrefix("/tree/") ?: return "/"
            val parts = docId.split(":", limit = 2)
            val storagePath = when (parts[0]) {
                "primary" -> "/storage/emulated/0"
                else -> "/storage/${parts[0]}"
            }
            return if (parts.size > 1 && parts[1].isNotEmpty()) "$storagePath/${parts[1]}" else storagePath
        }
        try {
            val docFile = android.provider.DocumentsContract.buildDocumentUriUsingTree(uri, android.provider.DocumentsContract.getTreeDocumentId(uri))
            val cursor = context.contentResolver.query(docFile, arrayOf(android.provider.OpenableColumns.DISPLAY_NAME), null, null, null)
            cursor?.use {
                if (it.moveToFirst()) {
                    val name = it.getString(0)
                    val treeId = android.provider.DocumentsContract.getTreeDocumentId(uri)
                    val treeParts = treeId.split(":", limit = 2)
                    val basePath = when (treeParts[0]) {
                        "primary" -> "/storage/emulated/0"
                        else -> "/storage/${treeParts[0]}"
                    }
                    return if (treeParts.size > 1 && treeParts[1].isNotEmpty()) "$basePath/${treeParts[1]}" else basePath
                }
            }
        } catch (_: Exception) {}
        return uri.toString()
    }

    private fun handleInstallConfirmResult(resultCode: Int, data: Intent?) {
        val call = pendingCalls.remove("installConfirm") ?: return
        val apkPath = data?.getStringExtra(InstallConfirmActivity.EXTRA_APK_PATH) ?: call.getString("apkPath") ?: ""
        if (resultCode == Activity.RESULT_OK) executeComboLiteInstall(call, File(apkPath)) else call.reject("\u7528\u6237\u53d6\u6d88\u5b89\u88c5")
    }

    private fun startInstallConfirm(call: PluginCall, apkPath: String, name: String) {
        pendingCalls["installConfirm"] = call; call.getData().put("apkPath", apkPath); call.save()
        try {
            activity.startActivityForResult(Intent(activity, InstallConfirmActivity::class.java).apply {
                putExtra(InstallConfirmActivity.EXTRA_APK_PATH, apkPath)
                putExtra(InstallConfirmActivity.EXTRA_FILE_NAME, name)
            }, REQUEST_CODE_INSTALL_CONFIRM)
        } catch (e: Exception) { pendingCalls.remove("installConfirm"); call.reject(e.message) }
    }

    @PluginMethod
    fun debugInstallFlow(call: PluginCall) {
        val testApk = File(context.cacheDir, "plugin_install").listFiles()?.filter { it.extension == "apk" }?.firstOrNull()
        call.resolve(JSObject().put("debugLog",
            if (testApk != null) DiagnosticKit.installTest(testApk, context).joinToString("\n")
            else "=== Actual installPlugin Test ===\nSKIPPED: no APK file found in plugin_install"))
    }

    @PluginMethod
    fun debugKotlinReflect(call: PluginCall) {
        call.resolve(JSObject().put("debugLog", DiagnosticKit.kotlinReflectHealthCheck().joinToString("\n")))
    }

    @PluginMethod
    fun debugApkValidation(call: PluginCall) {
        val testApk = File(context.cacheDir, "plugin_install").listFiles()?.filter { it.extension == "apk" }?.firstOrNull()
        call.resolve(JSObject().put("debugLog",
            if (testApk != null) DiagnosticKit.apkValidation(testApk, context).joinToString("\n")
            else "No APK file found in plugin_install"))
    }

    @PluginMethod
    fun debugValidationStrategy(call: PluginCall) {
        call.resolve(JSObject().put("debugLog", DiagnosticKit.validationStrategyStatus(context).joinToString("\n")))
    }

    @PluginMethod
    fun getLocalFilePath(call: PluginCall) {
        val path = call.getString("path", "") ?: ""
        call.resolve(JSObject().apply {
            put("path", when {
                path.isEmpty() -> ""
                File(path).exists() && File(path).isFile && File(path).canRead() -> File(path).absolutePath
                File(context.filesDir, path.removePrefix("/")).exists() && File(context.filesDir, path.removePrefix("/")).isFile -> File(context.filesDir, path.removePrefix("/")).absolutePath
                else -> ""
            })
        })
    }

    @PluginMethod
    fun startMpvInPlace(call: PluginCall) {
        try {
            val result = MpvEmbedService.startEmbed(
                activity = activity,
                containerId = call.getString("containerId") ?: "mpv-container",
                filePath = call.getString("filePath") ?: "",
                fileName = call.getString("name") ?: "",
                mimeType = call.getString("mimeType") ?: "",
                isExternal = false
            )
            call.resolve(JSObject().apply {
                put("success", result.success)
                put("error", result.error)
                put("errorDetail", result.errorDetail)
            })
        } catch (e: Exception) { call.reject(e.message) }
    }

    @PluginMethod
    fun stopMpvInPlace(call: PluginCall) {
        val success = MpvEmbedService.stopEmbed()
        call.resolve(JSObject().apply { put("success", success); put("embedded", MpvEmbedService.isEmbedded()) })
    }

    private fun executeComboLiteInstall(call: PluginCall, apkFile: File) {
        AppLogger.log("I", TAG, "executeComboLiteInstall: ${apkFile.name} (${apkFile.length()}B)")
        GlobalScope.launch(Dispatchers.IO) {
            when (val r = EncvComboLiteHost.installPlugin(apkFile)) {
                is OperationResult.Success -> withContext(Dispatchers.Main) {
                    AppLogger.log("I", TAG, "install SUCCESS: ${r.data.id}")
                    call.resolve(JSObject().apply { put("success", true); put("method", "combolite"); put("pluginId", r.data.id) })
                }
                is OperationResult.Failure -> withContext(Dispatchers.Main) {
                    AppLogger.log("E", TAG, "install FAILED: ${r.reason}")
                    call.reject(r.reason)
                }
            }
        }
    }

    @PluginMethod
    fun exportLogs(call: PluginCall) {
        try {
            val r = LogExporter.export(context)
            if (r.success) call.resolve(JSObject().apply { put("success", true); put("path", r.path) }) else call.reject("Failed to export logs")
        } catch (e: Exception) { call.reject(e.message) }
    }

    @PluginMethod
    fun clearLogs(call: PluginCall) {
        call.resolve(JSObject().put("success", LogExporter.clear(context)))
    }

    @PluginMethod
    fun openLogViewer(call: PluginCall) {
        call.resolve(JSObject().put("success", LogExporter.openViewer(context)))
    }

    @PluginMethod
    fun saveDevLogs(call: PluginCall) {
        val p = LogExporter.saveDevLogs(context, call.getString("logs") ?: run { call.reject("logs required"); return })
        if (p != null) call.resolve(JSObject().apply { put("success", true); put("path", p) }) else call.reject("Failed to save dev logs")
    }

    // 🆕 2026-06-17：读取 android-deps.json manifest

    // 🆕 2026-06-17：读取 android-deps.json manifest（由 Gradle task generateAndroidDepsManifest 在
    //   :app:preBuild 阶段生成到 app/src/main/assets/android-deps.json）
    //   用途：About 页"Android 库"section 数据源
    /**
     * 2026-07-03 spec android-workmanager-split-start-stop Phase 3.4
     * 前端取消任务时调用，把 cancel 意图持久化到 WorkManager。
     * 即使 Go 进程已死，Worker 也会在 Go 重启后重试 cancel。
     *
     * 参数：taskId (String, required)
     * 返回：{ success: Boolean, workName: String }
     */
    @PluginMethod
    fun enqueueCancelWorker(call: PluginCall) {
        val taskId = call.getString("taskId") ?: run { call.reject("taskId is required"); return }
        val workName = EncvTaskCancelWorker.enqueue(context, taskId)
        call.resolve(JSObject().apply {
            put("success", true)
            put("workName", workName)
        })
    }

    @PluginMethod
    fun getAndroidDeps(call: PluginCall) {
        try {
            val stream = context.assets.open("android-deps.json")
            val bytes = stream.readBytes()
            stream.close()
            val text = String(bytes, Charsets.UTF_8)
            // JSObject 解析 JSON 文本后传给前端
            call.resolve(JSObject(text))
        } catch (e: Exception) {
            Log.w("GoProcessPlugin", "android-deps.json not found or unreadable: ${e.message}")
            call.reject("android-deps.json not found: ${e.message}")
        }
    }

    private fun registerStatusReceiver() {
        if (receiverRegistered) return
        val filter = IntentFilter().apply { addAction(EncvGoService.BROADCAST_BACKEND_READY); addAction(EncvGoService.BROADCAST_BACKEND_STATUS) }
        if (Build.VERSION.SDK_INT >= 33) context.registerReceiver(statusReceiver, filter, Context.RECEIVER_NOT_EXPORTED)
        else @Suppress("DEPRECATION") context.registerReceiver(statusReceiver, filter)
        receiverRegistered = true
    }

    /**
     * Phase 23: 通过 PluginClassLoader 反射注册 statusListener 到 plugin。
     * plugin 在 host 进程里跑（PluginClassLoader），无 IPC 边界；
     * 直接拿 LoadedPluginInfo.classLoader.loadClass("com.encvgo.plugin.openlist.OpenListNativeService")
     * → 拿 statusListener 字段 → set(pluginStatusCallback) 即可。
     *
     * 失败语义：plugin 未安装/未加载 → 静默返回（前端首屏会拿到 NotInstalled），
     * 不阻塞 Capacitor 启动。
     */
    private fun registerPluginStatusListener() {
        if (pluginListenerRegistered) return
        val loaded = try {
            val l = EncvComboLiteHost.getLoadedPluginInfo("com.encvgo.plugin.openlist")
            if (l == null) {
                Log.w(TAG, "[SAT-DBG][OpenList][HostReceiver] plugin not loaded yet; trying ensurePluginLoaded")
                val ok = EncvComboLiteHost.ensurePluginLoaded("com.encvgo.plugin.openlist")
                if (!ok) {
                    Log.w(TAG, "[SAT-DBG][OpenList][HostReceiver] ensurePluginLoaded FAILED; will not register listener (frontend will get NotInstalled)")
                    return
                }
                EncvComboLiteHost.getLoadedPluginInfo("com.encvgo.plugin.openlist")
            } else l
        } catch (e: Throwable) {
            Log.e(TAG, "[SAT-DBG][OpenList][HostReceiver] getLoadedPluginInfo FAILED", e)
            return
        } ?: return
        try {
            // Phase 26: 反射字段从 OpenListBridge 改为 OpenListNativeService
            // —— gomobile bind 移除后, statusListener 字段迁移到 OpenListNativeService
            // (与 OpenListBridge 字段同型: Kotlin @JvmStatic Function1<Map<String, Any?>, Unit>)
            val bridgeClass = loaded.classLoader.loadClass("com.encvgo.plugin.openlist.OpenListNativeService")
            val listenerField = bridgeClass.getDeclaredField("statusListener")
            listenerField.isAccessible = true
            listenerField.set(null, pluginStatusCallback)
            pluginListenerRegistered = true
            Log.e(TAG, "[SAT-DBG][OpenList][HostReceiver] statusListener registered via classloader reflection | class=${bridgeClass.name}")
        } catch (e: Throwable) {
            Log.e(TAG, "[SAT-DBG][OpenList][HostReceiver] statusListener register FAILED", e)
        }
    }

    private fun unregisterPluginStatusListener() {
        if (!pluginListenerRegistered) return
        try {
            val l = EncvComboLiteHost.getLoadedPluginInfo("com.encvgo.plugin.openlist") ?: return
            val bridgeClass = l.classLoader.loadClass("com.encvgo.plugin.openlist.OpenListNativeService")
            val listenerField = bridgeClass.getDeclaredField("statusListener")
            listenerField.isAccessible = true
            listenerField.set(null, null)
            pluginListenerRegistered = false
            Log.e(TAG, "[SAT-DBG][OpenList][HostReceiver] statusListener unregistered")
        } catch (e: Throwable) {
            Log.e(TAG, "[SAT-DBG][OpenList][HostReceiver] statusListener unregister FAILED", e)
        }
    }

    private fun startService(action: String, source: String, command: String) =
        ContextCompat.startForegroundService(context, EncvGoService.createIntent(context, action, source).apply { putExtra(EncvGoService.EXTRA_COMMAND, command) })

    private fun resolvePendingCall(intent: Intent) {
        val cmd = intent.getStringExtra(EncvGoService.EXTRA_COMMAND) ?: return
        if (cmd != "restart") return
        val call = pendingCalls.remove("restart") ?: return
        if (intent.getBooleanExtra(EncvGoService.EXTRA_RUNNING, false) && intent.getIntExtra(EncvGoService.EXTRA_PORT, 0) > 0)
            call.resolve(JSObject().apply { put("success", true); put("port", intent.getIntExtra(EncvGoService.EXTRA_PORT, 0)) })
        else intent.getStringExtra(EncvGoService.EXTRA_ERROR)?.let { call.reject(it) } ?: call.reject("Unknown error")
    }
}
