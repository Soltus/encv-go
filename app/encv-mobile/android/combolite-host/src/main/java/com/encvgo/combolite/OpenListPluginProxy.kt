package com.encvgo.combolite

import android.content.Context
import android.os.Bundle
import android.util.Log
import com.combo.core.utils.startPluginService
import com.combo.core.utils.stopPluginService

/**
 * Phase 25 A3.2：Host 端封装 plugin service class 反射。
 *
 * 背景（combolite.md §10.3）：
 *   - ComboLite `Context.startPluginService(cls, instanceId, block)` 第一参必须是
 *     `Class<out IPluginService>` 引用（编译期类型约束）。
 *   - 但 app module 不依赖 plugin-openlist module（plugin 是文件系统 + classloader 加载），
 *     编译期**不能**直接写 `OpenListPluginService::class.java`。
 *   - 唯一合法路径：plugin 装载到 EncvComboLiteHost 后，通过 classloader 反射拿 class。
 *
 * 封装本 helper 后：
 *   - App 模块只调 `OpenListPluginProxy.startMainService(context)` —— 无反射
 *   - 反射只发生在 combolite-host 模块（封装在框架侧）
 *   - 未来添加新 plugin service，host 端复制本 helper 改 PLUGIN_ID 和 SERVICE_CLASS 即可
 *
 * 链路：
 *   OpenListPluginProxy.startMainService(context)
 *     ↓ ensurePluginLoaded(PLUGIN_ID)        // load 进 classloader 池
 *     ↓ loaded.classLoader.loadClass(SERVICE_CLASS)
 *     ↓ context.startPluginService(pluginClass, "main", block)
 *           ↓ ProxyManager.acquireServiceProxy("OpenListPluginService:main")
 *           ↓ Intent(this, HostServiceX) + extras
 *           ↓ Android 系统实例化 HostServiceX
 *           ↓ BaseHostService.onStartCommand → initPluginService → newInstance + onAttach + onCreate
 *           ↓ OpenListPluginService.onStartCommand(intent) → startupSequence
 *           ↓ OpenListBridge.start() 启动 gomobile
 *           ↓ broadcastStatus → statusListener.invoke → Capacitor notifyListeners
 */
object OpenListPluginProxy {

    /** 与 plugin-openlist/AndroidManifest.xml 的 package + class name 对齐 */
    const val PLUGIN_ID = "com.encvgo.plugin.openlist"

    /**
     * plugin service 全限定类名。
     * Plugin APK 内 OpenListPluginService.kt 真实路径：com/encvgo/plugin/openlist/OpenListPluginService.kt
     * 必须与 .class 文件实际位置一致（comboLite 通过 classloader 反射加载）。
     */
    const val SERVICE_CLASS = "com.encvgo.plugin.openlist.OpenListPluginService"

    /**
     * startPluginService 的 instanceId——单个 plugin service 唯一标识。
     * 同 class 可启多个 instance，每个用不同 instanceId 区分。
     * openlist 只跑 1 个 main instance，写死 "main"。
     */
    const val MAIN_INSTANCE_ID = "main"

    private const val TAG = "OpenList-PluginProxy"

    /**
     * 确保 plugin 已加载到 classloader 池，返回 plugin class 引用。
     * 失败返回 null。
     */
    fun loadPluginServiceClassOrNull(): Class<*>? {
        // 先 ensure loaded（如果没装 / 没 load，先装 + load）
        if (EncvComboLiteHost.getLoadedPluginInfo(PLUGIN_ID) == null) {
            val ok = try {
                EncvComboLiteHost.ensurePluginLoaded(PLUGIN_ID)
            } catch (e: Throwable) {
                Log.e(TAG, "loadPluginServiceClassOrNull: ensurePluginLoaded FAILED", e)
                false
            }
            if (!ok) {
                Log.e(TAG, "loadPluginServiceClassOrNull: ensurePluginLoaded returned false")
                return null
            }
        }
        val loaded = EncvComboLiteHost.getLoadedPluginInfo(PLUGIN_ID) ?: run {
            Log.e(TAG, "loadPluginServiceClassOrNull: still not loaded after ensurePluginLoaded")
            return null
        }
        return try {
            loaded.classLoader.loadClass(SERVICE_CLASS)
        } catch (e: ClassNotFoundException) {
            Log.e(TAG, "loadPluginServiceClassOrNull: loadClass('$SERVICE_CLASS') FAILED", e)
            null
        } catch (e: Throwable) {
            Log.e(TAG, "loadPluginServiceClassOrNull: unexpected error", e)
            null
        }
    }

    /**
     * 启动 plugin service (main instance)。
     *
     * @param context 任意 Context（实际用 applicationContext 避免 leak）
     * @param extras 传给 plugin service onStartCommand(intent) 的 extras（plugin 可解析）
     * @return true 表示 Intent 已成功发送（不保证 service 真的启动成功）
     */
    fun startMainService(context: Context, extras: Bundle = Bundle()): Boolean {
        Log.e(TAG, "[SAT-DBG] startMainService() | extras=$extras")
        val cls = loadPluginServiceClassOrNull() ?: run {
            Log.e(TAG, "startMainService: loadPluginServiceClassOrNull returned null")
            return false
        }
        @Suppress("UNCHECKED_CAST")
        val pluginClass = cls as Class<out com.combo.core.api.IPluginService>
        return try {
            context.applicationContext.startPluginService(
                pluginClass,
                MAIN_INSTANCE_ID,
                block = { putExtras(extras) }
            )
            Log.e(TAG, "[SAT-DBG] startMainService: startPluginService() invoked OK")
            true
        } catch (e: Throwable) {
            Log.e(TAG, "startMainService: startPluginService FAILED", e)
            false
        }
    }

    /**
     * 停止 plugin service (main instance)。
     *
     * ComboLite [com.combo.core.utils.stopPluginService] 会查 activeServiceProxies 拿到
     * 之前分配的 host service class，再调 stopService(Intent(this, hostServiceClass))。
     * 链路：BaseHostService.onDestroy → releaseServiceProxy(instanceId) → put back to pool
     */
    fun stopMainService(context: Context): Boolean {
        Log.e(TAG, "[SAT-DBG] stopMainService()")
        val cls = loadPluginServiceClassOrNull() ?: run {
            Log.e(TAG, "stopMainService: loadPluginServiceClassOrNull returned null")
            return false
        }
        @Suppress("UNCHECKED_CAST")
        val pluginClass = cls as Class<out com.combo.core.api.IPluginService>
        return try {
            val ok = context.applicationContext.stopPluginService(pluginClass, MAIN_INSTANCE_ID)
            Log.e(TAG, "[SAT-DBG] stopMainService: stopPluginService() returned $ok")
            ok
        } catch (e: Throwable) {
            Log.e(TAG, "stopMainService: stopPluginService FAILED", e)
            false
        }
    }
}
