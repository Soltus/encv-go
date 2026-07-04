package com.encvgo.combolite

import android.content.ContentValues
import android.content.Context
import android.database.Cursor
import android.net.Uri
import android.util.Log
import com.combo.core.utils.insertPlugin
import com.combo.core.utils.queryPlugin

/**
 * Host-side bridge to the OpenList extension.
 *
 * Phase 25 A3：完全走 ComboLite ContentProvider 代理层（combolite.md §10.4）。
 * 历史链路（Phase 23-24）：通过 PluginClassLoader 反射调 OpenListBridge 静态方法。
 *   - 失败原因：plugin APK 没系统级 install → Class<?> 可达但体系不正统
 *   - 副作用：每个 host 操作都走 classloader 反射，破坏 ComboLite 抽象
 *
 * 新方案：host 调 contentResolver.queryPlugin / insertPlugin → BaseHostProvider 转发
 *         → plugin OpenListStatusProvider.query / insert。
 *   - authority 链：
 *       host contentResolver.query("content://com.encvgo.app.provider/com.encvgo.plugin.openlist.provider/status")
 *         ↓ ContentResolver 路由到 host manifest 注册的 HostStatusProvider（authority="com.encvgo.app.provider"）
 *         ↓ BaseHostProvider.query(proxyUri) → 解析出 plugin authority="com.encvgo.plugin.openlist.provider"
 *         ↓ PluginManager.proxyManager.findProviderInfoByAuthority("com.encvgo.plugin.openlist.provider")
 *         ↓ PluginManager.getInterface(ContentProvider::class.java, "com.encvgo.plugin.openlist.OpenListStatusProvider")
 *         ↓ clazz.getDeclaredConstructor().newInstance() + attachInfo(context, null)
 *         ↓ OpenListStatusProvider.query(rewrittenUri, ...)  返回 MatrixCursor
 *
 * Plugin 端：
 *   - [com.encvgo.plugin.openlist.OpenListStatusProvider] 已实现 query/insert
 *   - manifest 声明 <provider authorities="com.encvgo.plugin.openlist.provider" exported="true">
 *   - ComboLite 在 install plugin 时解析 manifest，存到 providerRegistry
 *
 * Status 推送方向（host → plugin）保持 Phase 24：
 *   [com.encvgo.plugin.openlist.OpenListNativeService.statusListener] 反射注册 host lambda
 *   plugin broadcastStatus() → lambda.invoke → Capacitor notifyListeners
 *   （A3 不动这块，因为 A3 只换"读 snapshot / 发 action"通道，状态推送方向仍 in-process）
 *
 * 参考 ComboLite:
 *   - com.combo.core.utils.buildProxyUri / queryPlugin / insertPlugin
 *   - com.combo.core.component.provider.BaseHostProvider.withForwardedRequest
 */
object OpenListStatusBridge {

    /** 与 plugin-openlist/src/main/AndroidManifest.xml 的 authorities 对齐 */
    private const val PLUGIN_ID = "com.encvgo.plugin.openlist"
    private const val PLUGIN_AUTHORITY = "com.encvgo.plugin.openlist.provider"
    private const val PATH_STATUS = "status"
    private const val PATH_CONTROL = "control"

    /** 插件端原始 URI（host 调 queryPlugin 时传入，被 buildProxyUri 改写） */
    private val PLUGIN_STATUS_URI: Uri = Uri.parse("content://$PLUGIN_AUTHORITY/$PATH_STATUS")
    private val PLUGIN_CONTROL_URI: Uri = Uri.parse("content://$PLUGIN_AUTHORITY/$PATH_CONTROL")

    private const val TAG = "OpenList-HostBridge"

    data class OpenListRuntime(
        val isInstalled: Boolean,
        val running: Boolean,
        val port: Int,
        val pid: Int,
        val dataSizeBytes: Long,
        val lastError: String,
        val lastUpdateTs: Long,
    ) {
        companion object {
            val NotInstalled = OpenListRuntime(
                isInstalled = false,
                running = false,
                port = 0,
                pid = 0,
                dataSizeBytes = 0L,
                lastError = "openlist extension not installed",
                lastUpdateTs = 0L,
            )
        }
    }

    /**
     * 一次性读快照。
     *
     * Phase 25 A3 改：用 [com.combo.core.utils.queryPlugin] 走 BaseHostProvider 代理。
     *   旧版 (Phase 24)：classloader 反射 OpenListBridge.snapshot() → Map<*,*>
     *   新版：contentResolver.queryPlugin(STATUS_URI) → Cursor → 转 OpenListRuntime
     *
     * 失败语义：未安装 → NotInstalled；已装未加载 → InstalledNotLoaded；
     * 已加载但 query 失败 → InstalledButQueryFailed。
     */
    fun read(context: Context): OpenListRuntime {
        Log.e(TAG, "[SAT-DBG][OpenList][HostBridge] read() begin | ts=${System.currentTimeMillis()}")

        val installed = EncvComboLiteHost.getInstalledPlugins().any { it.id == PLUGIN_ID }
        if (!installed) {
            Log.e(TAG, "[SAT-DBG][OpenList][HostBridge] read() → NotInstalled")
            return OpenListRuntime.NotInstalled
        }
        Log.e(TAG, "[SAT-DBG][OpenList][HostBridge] read() plugin IS installed")

        // 已装未加载 → 尝试 load
        if (EncvComboLiteHost.getLoadedPluginInfo(PLUGIN_ID) == null) {
            val ok = try {
                EncvComboLiteHost.ensurePluginLoaded(PLUGIN_ID)
            } catch (e: Throwable) {
                Log.e(TAG, "[SAT-DBG][OpenList][HostBridge] ensurePluginLoaded FAILED", e)
                false
            }
            if (!ok) {
                Log.w(TAG, "[SAT-DBG][OpenList][HostBridge] read() → InstalledNotLoaded")
                return OpenListRuntime(
                    isInstalled = true, running = false, port = 0, pid = 0, dataSizeBytes = 0L,
                    lastError = "installed but not loaded yet", lastUpdateTs = 0L,
                )
            }
        }

        // Phase 25 A3：走 queryPlugin → BaseHostProvider.query → plugin OpenListStatusProvider.query
        return try {
            val cursor = context.contentResolver.queryPlugin(
                PLUGIN_STATUS_URI,
                null, null, null, null
            ) ?: run {
                Log.w(TAG, "[SAT-DBG][OpenList][HostBridge] queryPlugin returned null cursor")
                return OpenListRuntime(
                    isInstalled = true, running = false, port = 0, pid = 0, dataSizeBytes = 0L,
                    lastError = "queryPlugin returned null cursor", lastUpdateTs = 0L,
                )
            }
            cursor.use { c ->
                if (!c.moveToFirst()) {
                    Log.w(TAG, "[SAT-DBG][OpenList][HostBridge] cursor empty")
                    return OpenListRuntime(
                        isInstalled = true, running = false, port = 0, pid = 0, dataSizeBytes = 0L,
                        lastError = "cursor empty", lastUpdateTs = 0L,
                    )
                }
                // Provider 端 MatrixCursor 列：
                //   running (Int 0/1), port (Int), pid (Int),
                //   data_size_bytes (Long), last_error (String), last_update_ts (Long)
                val running = (c.getInt(0) != 0)
                val port = c.getInt(1)
                val pid = c.getInt(2)
                val dataSize = c.getLong(3)
                val lastError = c.getString(4) ?: ""
                val lastUpdate = c.getLong(5)
                val runtime = OpenListRuntime(
                    isInstalled = true, running = running, port = port, pid = pid,
                    dataSizeBytes = dataSize, lastError = lastError, lastUpdateTs = lastUpdate
                )
                Log.e(TAG, "[SAT-DBG][OpenList][HostBridge] read() OK | running=$running port=$port dataSize=$dataSize lastErr='$lastError'")
                runtime
            }
        } catch (e: Throwable) {
            Log.e(TAG, "[SAT-DBG][OpenList][HostBridge] read() FAILED", e)
            OpenListRuntime(
                isInstalled = true, running = false, port = 0, pid = 0, dataSizeBytes = 0L,
                lastError = "read failed: ${e.message}", lastUpdateTs = 0L,
            )
        }
    }

    /**
     * 控制 plugin（启动/停止/强制 DB sync/设置 admin 密码）。
     *
     * Phase 25 A3 改：用 [com.combo.core.utils.insertPlugin] 走 BaseHostProvider 代理。
     *   旧版 (Phase 24)：classloader 反射 OpenListBridge.start / shutdown / forceDBSync / setAdminPassword
     *   新版：contentResolver.insertPlugin(CONTROL_URI, ContentValues("action"=...))
     *         → BaseHostProvider.insert → plugin OpenListStatusProvider.insert(control, values)
     *
     * 注意：plugin 端 OpenListStatusProvider.insert() 不返回错误码——返回的 URI 仅是
     * 一个占位 result URI。失败只能从 catch e 推断。
     */
    fun control(context: Context, action: String, args: Map<String, Any> = emptyMap()): Boolean {
        Log.e(TAG, "[SAT-DBG][OpenList][HostBridge] control() action=$action args=$args")
        val installed = EncvComboLiteHost.getInstalledPlugins().any { it.id == PLUGIN_ID }
        if (!installed) {
            Log.e(TAG, "[SAT-DBG][OpenList][HostBridge] control() FAILED: plugin not installed")
            return false
        }
        if (EncvComboLiteHost.getLoadedPluginInfo(PLUGIN_ID) == null) {
            val ok = try { EncvComboLiteHost.ensurePluginLoaded(PLUGIN_ID) } catch (e: Throwable) { false }
            if (!ok) {
                Log.e(TAG, "[SAT-DBG][OpenList][HostBridge] control() FAILED: plugin not loaded")
                return false
            }
        }

        val values = ContentValues().apply {
            put("action", action)
            (args["port"] as? Int)?.let { put("port", it) }
            (args["password"] as? String)?.let { put("password", it) }
            (args["timeout_ms"] as? Long)?.let { put("timeout_ms", it) }
        }

        return try {
            val result = context.contentResolver.insertPlugin(PLUGIN_CONTROL_URI, values)
            Log.e(TAG, "[SAT-DBG][OpenList][HostBridge] control() action=$action → resultUri=$result")
            true
        } catch (e: Throwable) {
            Log.e(TAG, "[SAT-DBG][OpenList][HostBridge] control() FAILED", e)
            false
        }
    }
}
