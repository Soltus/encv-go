package com.encvgo.combolite

import android.content.Context
import android.content.Intent
import com.encvgo.combolite.engine.PluginLifecycleEngine
import com.encvgo.combolite.model.OperationResult
import com.encvgo.combolite.model.PluginFullState
import com.encvgo.combolite.model.PluginState

object EncvComboLiteHost {

    val isInitialized: Boolean get() = PluginLifecycleEngine.isInitialized()

    fun getInstalledPlugins(): List<PluginState> = PluginLifecycleEngine.getInstalledPlugins()

    fun getPluginInfo(pluginId: String): PluginState? = PluginLifecycleEngine.getPluginInfo(pluginId)

    fun getLoadedPluginInfo(pluginId: String): com.combo.core.model.LoadedPluginInfo? = PluginLifecycleEngine.getLoadedPluginInfo(pluginId)

    fun getPluginFullState(pluginId: String): PluginFullState {
        if (!PluginLifecycleEngine.isInitialized()) {
            return PluginFullState(id = pluginId, status = "framework_not_ready")
        }
        val allInstalled = getInstalledPlugins()
        android.util.Log.i("EncvComboLiteHost", "getPluginFullState($pluginId): allInstalled.size=${allInstalled.size} ids=${allInstalled.map { it.id }}")
        val installed = allInstalled.find { it.id == pluginId }
        if (installed == null) {
            android.util.Log.w("EncvComboLiteHost", "getPluginFullState($pluginId): NOT FOUND in installed list → not_installed")
            return PluginFullState(id = pluginId, status = "not_installed")
        }
        if (!installed.enabled) {
            return PluginFullState(id = pluginId, status = "disabled", name = installed.name, version = installed.versionName)
        }
        val loaded = PluginLifecycleEngine.isPluginLoaded(pluginId)
        return PluginFullState(
            id = pluginId,
            status = if (loaded) "ready" else "not_loaded",
            name = installed.name,
            version = installed.versionName
        )
    }

    fun isPluginAvailable(pluginId: String): Boolean {
        if (!PluginLifecycleEngine.isInitialized()) return false
        val installed = getInstalledPlugins().find { it.id == pluginId }
        return installed != null && installed.enabled && PluginLifecycleEngine.isPluginLoaded(pluginId)
    }

    fun ensurePluginLoaded(pluginId: String): Boolean = PluginLifecycleEngine.ensurePluginLoaded(pluginId)

    suspend fun installPlugin(apkFile: java.io.File): OperationResult<PluginState> =
        PluginLifecycleEngine.installPlugin(apkFile)

    suspend fun uninstallPlugin(pluginId: String): OperationResult<Unit> =
        PluginLifecycleEngine.uninstallPlugin(pluginId)

    suspend fun setPluginEnabled(pluginId: String, enabled: Boolean): OperationResult<Unit> =
        PluginLifecycleEngine.setPluginEnabled(pluginId, enabled)

    suspend fun launchPlugin(pluginId: String): Boolean =
        PluginLifecycleEngine.launchPlugin(pluginId)

    fun createProxyIntent(
        context: Context,
        pluginId: String,
        targetActivity: String,
        hostActivityClass: Class<*>,
        extras: Map<String, Any> = emptyMap()
    ): Intent = PluginLifecycleEngine.createProxyIntent(context, pluginId, targetActivity, hostActivityClass, extras)

    fun setupFramework(
        hostActivityClass: Class<*>,
        hostServicePool: List<Class<out com.combo.core.component.service.BaseHostService>>,
        hostProviderAuthority: String,
    ) = PluginLifecycleEngine.setupFramework(hostActivityClass, hostServicePool, hostProviderAuthority)
}
