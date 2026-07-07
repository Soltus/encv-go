package com.encvgo.combolite.engine

import android.content.Context
import android.content.Intent
import android.util.Log
import com.combo.core.runtime.PluginManager
import com.combo.core.runtime.ValidationStrategy
import com.combo.core.security.crash.PluginCrashHandler
import com.combo.core.component.activity.BaseHostActivity
import com.encvgo.combolite.model.OperationResult
import com.encvgo.combolite.model.PluginState
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.withContext
import java.io.File
import java.io.IOException
import java.util.LinkedList

internal object PluginLifecycleEngine {

    private const val TAG = "ComboLiteEngine"

    fun isInitialized(): Boolean = PluginManager.isInitialized

    fun getInstalledPlugins(): List<PluginState> {
        if (!PluginManager.isInitialized) return emptyList()
        return try {
            PluginManager.getAllInstallPlugins().map { plugin ->
                PluginState(
                    id = plugin.id,
                    name = plugin.name,
                    versionName = plugin.versionName,
                    versionCode = plugin.versionCode,
                    enabled = plugin.enabled,
                    installed = true,
                    entryClass = plugin.entryClass,
                    description = plugin.description
                )
            }
        } catch (e: Exception) {
            emptyList()
        }
    }

    fun getPluginInfo(pluginId: String): PluginState? {
        if (!PluginManager.isInitialized) return null
        return try {
            val loadedInfo = PluginManager.getPluginInfo(pluginId)
            if (loadedInfo != null) {
                val p = loadedInfo.pluginInfo
                PluginState(
                    id = p.id, name = p.name, versionName = p.versionName,
                    versionCode = p.versionCode, enabled = p.enabled,
                    installed = true, entryClass = p.entryClass, description = p.description
                )
            } else {
                null
            }
        } catch (e: Exception) {
            null
        }
    }

    fun getLoadedPluginInfo(pluginId: String): com.combo.core.model.LoadedPluginInfo? {
        if (!PluginManager.isInitialized) return null
        return try {
            PluginManager.getPluginInfo(pluginId)
        } catch (e: Exception) {
            Log.e(TAG, "getLoadedPluginInfo($pluginId) failed: ${e.message}", e)
            null
        }
    }

    suspend fun installPlugin(apkFile: File): OperationResult<PluginState> {
        if (!PluginManager.isInitialized) {
            return OperationResult.Failure("PluginManager not initialized")
        }
        if (!apkFile.exists()) {
            return OperationResult.Failure("APK file not found: ${apkFile.absolutePath}")
        }
        return try {
            val result = PluginManager.installerManager.installPlugin(apkFile, true)
            when (result) {
                is com.combo.core.runtime.installer.InstallerManager.InstallResult.Success -> {
                    try { PluginManager.loadEnabledPlugins() } catch (_: Exception) {}
                    val pi = result.pluginInfo
                    val pluginId = pi.id
                    if (!pi.enabled) {
                        Log.i(TAG, "installPlugin: plugin $pluginId installed but disabled, enabling...")
                        try { PluginManager.setPluginEnabled(pluginId, true) } catch (_: Exception) {}
                        try { PluginManager.loadEnabledPlugins() } catch (_: Exception) {}
                    }
                    OperationResult.Success(
                        PluginState(
                            id = pi.id, name = pi.name, versionName = pi.versionName,
                            versionCode = pi.versionCode, enabled = true,
                            installed = true, entryClass = pi.entryClass, description = pi.description
                        )
                    )
                }
                is com.combo.core.runtime.installer.InstallerManager.InstallResult.Failure -> {
                    OperationResult.Failure(result.reason, result.exception)
                }
            }
        } catch (e: Error) {
            OperationResult.Failure("${e.javaClass.simpleName}: ${e.message}", e)
        } catch (e: Exception) {
            OperationResult.Failure(e.message ?: "Unknown install error", e)
        }
    }

    suspend fun uninstallPlugin(pluginId: String): OperationResult<Unit> {
        if (!PluginManager.isInitialized) {
            return OperationResult.Failure("PluginManager not initialized")
        }
        return try {
            val success = PluginManager.installerManager.uninstallPlugin(pluginId)
            if (success == true) {
                OperationResult.Success(Unit)
            } else {
                OperationResult.Failure("uninstallPlugin returned false (permission denied or plugin not found)")
            }
        } catch (e: Error) {
            OperationResult.Failure("${e.javaClass.simpleName}: ${e.message}", e)
        } catch (e: Exception) {
            OperationResult.Failure(e.message ?: "Unknown uninstall error", e)
        }
    }

    suspend fun setPluginEnabled(pluginId: String, enabled: Boolean): OperationResult<Unit> {
        if (!PluginManager.isInitialized) {
            return OperationResult.Failure("PluginManager not initialized")
        }
        return try {
            PluginManager.setPluginEnabled(pluginId, enabled)
            OperationResult.Success(Unit)
        } catch (e: Error) {
            OperationResult.Failure("${e.javaClass.simpleName}: ${e.message}", e)
        } catch (e: Exception) {
            OperationResult.Failure(e.message ?: "Unknown error", e)
        }
    }

    suspend fun loadAllEnabledPlugins(): Int {
        if (!PluginManager.isInitialized) return 0
        return try {
            PluginManager.loadEnabledPlugins()
        } catch (e: Exception) {
            0
        }
    }

    suspend fun launchPlugin(pluginId: String): Boolean {
        if (!PluginManager.isInitialized) return false
        return try {
            PluginManager.launchPlugin(pluginId) ?: false
        } catch (e: Exception) {
            false
        }
    }

    fun isPluginLoaded(pluginId: String): Boolean {
        if (!PluginManager.isInitialized) {
            Log.w(TAG, "isPluginLoaded($pluginId): PluginManager not initialized")
            return false
        }
        return PluginManager.getPluginInfo(pluginId) != null
    }

    fun ensurePluginLoaded(pluginId: String): Boolean {
        if (!PluginManager.isInitialized) {
            Log.w(TAG, "ensurePluginLoaded($pluginId): PluginManager not initialized")
            return false
        }
        return try {
            if (PluginManager.getPluginInfo(pluginId) != null) {
                Log.i(TAG, "ensurePluginLoaded($pluginId): already loaded")
                true
            } else {
                Log.i(TAG, "ensurePluginLoaded($pluginId): loading...")
                val success = runBlocking { launchPlugin(pluginId) }
                Log.i(TAG, "ensurePluginLoaded($pluginId): load result=$success")
                success
            }
        } catch (e: Exception) {
            Log.e(TAG, "ensurePluginLoaded($pluginId): failed", e)
            false
        }
    }

    suspend fun installBundledPlugins(context: Context, assetsDir: String = "plugins"): Int {
        if (!PluginManager.isInitialized) {
            Log.w(TAG, "installBundledPlugins: PluginManager not initialized")
            return 0
        }
        return withContext(Dispatchers.IO) {
            val installedCount = installFromAssetsRecursive(context, assetsDir, "")
            Log.i(TAG, "installBundledPlugins: installed $installedCount plugin(s) from assets/$assetsDir")
            installedCount
        }
    }

    private suspend fun installFromAssetsRecursive(
        context: Context,
        baseDir: String,
        subPath: String,
    ): Int {
        val assetPath = if (subPath.isEmpty()) baseDir else "$baseDir/$subPath"
        val entries = try {
            context.assets.list(assetPath) ?: return 0
        } catch (e: IOException) {
            Log.w(TAG, "installFromAssets: cannot list $assetPath: ${e.message}")
            return 0
        }

        var count = 0
        val dirs = LinkedList<String>()

        for (entry in entries) {
            val fullPath = if (subPath.isEmpty()) entry else "$subPath/$entry"
            val relativeInBase = fullPath
            val children = try {
                context.assets.list("$baseDir/$relativeInBase")
            } catch (e: IOException) {
                null
            }

            if (children.isNullOrEmpty()) {
                if (entry.endsWith(".apk", ignoreCase = true)) {
                    val apkFile = extractAssetToCache(context, "$baseDir/$relativeInBase")
                    if (apkFile != null) {
                        val result = installPlugin(apkFile)
                        if (result is OperationResult.Success) {
                            Log.i(TAG, "installFromAssets: installed ${result.data.id} (${result.data.versionName}) from $relativeInBase")
                            count++
                        } else {
                            Log.w(TAG, "installFromAssets: failed to install $relativeInBase: ${(result as? OperationResult.Failure)?.reason}")
                        }
                        apkFile.delete()
                    }
                }
            } else {
                dirs.add(relativeInBase)
            }
        }

        for (dir in dirs) {
            count += installFromAssetsRecursive(context, baseDir, dir)
        }

        return count
    }

    private fun extractAssetToCache(context: Context, assetPath: String): File? {
        return try {
            val outFile = File(context.cacheDir, "bundled_plugins/${assetPath.replace('/', '_')}")
            outFile.parentFile?.mkdirs()
            context.assets.open(assetPath).use { input ->
                outFile.outputStream().use { output ->
                    input.copyTo(output)
                }
            }
            if (outFile.length() > 0) outFile else null
        } catch (e: IOException) {
            Log.e(TAG, "extractAssetToCache: failed to extract $assetPath: ${e.message}", e)
            null
        }
    }

    fun createProxyIntent(
        context: Context,
        pluginId: String,
        targetActivity: String,
        hostActivityClass: Class<*>,
        extras: Map<String, Any> = emptyMap()
    ): Intent {
        return Intent(context, hostActivityClass).apply {
            putExtra("plugin_activity_class_name", targetActivity)
            for ((key, value) in extras) {
                when (value) {
                    is String -> putExtra(key, value)
                    is Int -> putExtra(key, value)
                    is Long -> putExtra(key, value)
                    is Float -> putExtra(key, value)
                    is Double -> putExtra(key, value)
                    is Boolean -> putExtra(key, value)
                }
            }
            if (context !is android.app.Activity) {
                addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
            }
        }
    }

    /**
     * Phase 25 A2：Host 端 ComboLite 框架初始化。
     *
     * @param hostActivityClass 单一 Host Activity（comboLite 标准只能有 1 个）
     * @param hostServicePool 16 个 Host Service 代理（com.encvgo.combolite.proxy.HostService1..16）
     *   ComboLite ProxyManager 池大小 = servicePool.size，最多支持 servicePool.size - 1
     *   个 plugin service 并发（+ 1 buffer 防 race）。
     * @param hostProviderAuthority Host Provider 的 authority 字符串。
     *   必须与 AndroidManifest.xml 中 <provider> 的 android:authorities 完全一致。
     *   Plugin ContentResolver.queryPlugin() 会被改写路由到 content://<hostAuthority>/<encoded_plugin_authority>/path
     */
    fun setupFramework(
        hostActivityClass: Class<*>,
        hostServicePool: List<Class<out com.combo.core.component.service.BaseHostService>>,
        hostProviderAuthority: String,
    ) {
        try {
            runBlocking { PluginManager.setValidationStrategy(ValidationStrategy.Insecure) }
        } catch (e: Error) {
        } catch (e: Exception) {
        }
        try {
            runBlocking { PluginCrashHandler.setGlobalClashCallback(null) }
        } catch (e: Error) {
        } catch (e: Exception) {
        }
        try {
            PluginManager.proxyManager.setHostActivity(hostActivityClass as Class<BaseHostActivity>)
        } catch (e: Exception) {
            Log.e(TAG, "setupFramework: setHostActivity failed", e)
        }
        try {
            PluginManager.proxyManager.setServicePool(hostServicePool)
        } catch (e: Exception) {
            Log.e(TAG, "setupFramework: setServicePool failed (size=${hostServicePool.size})", e)
        }
        try {
            PluginManager.proxyManager.setHostProviderAuthority(hostProviderAuthority)
        } catch (e: Exception) {
            Log.e(TAG, "setupFramework: setHostProviderAuthority('$hostProviderAuthority') failed", e)
        }
        Log.i(TAG, "setupFramework: complete | servicePoolSize=${hostServicePool.size} providerAuthority='$hostProviderAuthority'")
    }
}
