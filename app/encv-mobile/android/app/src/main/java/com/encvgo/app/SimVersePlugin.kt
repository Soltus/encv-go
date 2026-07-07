package com.encvgo.app

import android.app.Activity
import android.content.Context
import android.content.Intent
import android.content.pm.PackageManager
import android.util.Log
import com.getcapacitor.JSObject
import com.getcapacitor.Plugin
import com.getcapacitor.PluginCall
import com.getcapacitor.PluginMethod
import com.getcapacitor.annotation.CapacitorPlugin
import com.encvgo.app.workers.SimverseHeartbeatWorker
import com.encvgo.combolite.EncvComboLiteHost
import com.combo.core.runtime.PluginManager

@CapacitorPlugin(name = "SimVerse")
class SimVersePlugin : Plugin() {

    companion object {
        private const val TAG = "SimVersePlugin"
        private const val PLUGIN_ID = "com.encvgo.plugin.simverse"
        private const val TARGET_ACTIVITY = "com.encvgo.plugin.simverse.SimVerseActivity"
    }

    @PluginMethod
    fun debugSimVerseFlow(call: PluginCall) {
        val steps = mutableListOf<String>()

        steps.add("╔══════════════════════════════════════════════════╗")
        steps.add("║     SimVerse 全链路饱和诊断 (v1)                  ║")
        steps.add("╚══════════════════════════════════════════════════╝")
        steps.add("")

        steps.add("═══ 1. ComboLite 框架状态 ═══")
        steps.add("   EncvComboLiteHost.isInitialized = ${EncvComboLiteHost.isInitialized}")
        steps.add("   PluginManager.isInitialized = ${try { PluginManager.isInitialized } catch (e: Exception) { "ERROR: ${e.message}" }}")

        if (!EncvComboLiteHost.isInitialized) {
            steps.add("   ❌ ComboLite 未初始化，后续诊断跳过")
            call.resolve(JSObject().put("debugLog", steps.joinToString("\n")))
            return
        }
        steps.add("   ✅ ComboLite 已初始化")
        steps.add("")

        steps.add("═══ 2. 插件安装/加载状态 ═══")
        try {
            val fullState = EncvComboLiteHost.getPluginFullState(PLUGIN_ID)
            steps.add("   status = ${fullState.status}")
            steps.add("   name = ${fullState.name}")
            steps.add("   version = ${fullState.version}")
            steps.add("   enabled = ${fullState.enabled}")
        } catch (e: Exception) {
            steps.add("   ❌ getPluginFullState FAILED: ${e.javaClass.simpleName}: ${e.message}")
        }
        steps.add("")

        steps.add("═══ 3. 已安装插件列表 ═══")
        try {
            val allPlugins = PluginManager.getAllInstallPlugins()
            steps.add("   total installed = ${allPlugins.size}")
            val target = allPlugins.find { it.id == PLUGIN_ID }
            if (target != null) {
                steps.add("   ✅ 目标插件 '$PLUGIN_ID' 已安装")
                steps.add("      versionName = ${target.versionName}")
                steps.add("      enabled = ${target.enabled}")
                steps.add("      entryClass = ${target.entryClass}")
            } else {
                steps.add("   ❌ 目标插件 '$PLUGIN_ID' 未在已安装列表中")
                steps.add("   已安装插件列表:")
                allPlugins.forEach { p ->
                    steps.add("      - ${p.id} (v${p.versionName}, enabled=${p.enabled})")
                }
            }
        } catch (e: Exception) {
            steps.add("   ❌ getAllInstallPlugins FAILED: ${e.message}")
        }
        steps.add("")

        steps.add("═══ 4. 插件 Info 详细检查 ═══")
        try {
            val info = PluginManager.getPluginInfo(PLUGIN_ID)
            if (info != null) {
                val pi = info.pluginInfo
                steps.add("   ✅ getPluginInfo 成功")
                steps.add("      id = ${pi.id}")
                steps.add("      name = ${pi.name}")
                steps.add("      versionName = ${pi.versionName}")
                steps.add("      versionCode = ${pi.versionCode}")
                steps.add("      enabled = ${pi.enabled}")
                steps.add("      entryClass = ${pi.entryClass}")
                steps.add("      apkPath = ${pi.apkPath}")
            } else {
                steps.add("   ❌ getPluginInfo 返回 null（插件未加载？）")
            }
        } catch (e: Exception) {
            steps.add("   ❌ getPluginInfo FAILED: ${e.javaClass.simpleName}: ${e.message}")
            steps.add("   stack = ${e.stackTraceToString().take(600)}")
        }
        steps.add("")

        steps.add("═══ 5. 插件 APK 文件检查 ═══")
        try {
            val info = PluginManager.getPluginInfo(PLUGIN_ID)
            if (info != null) {
                val apkPath = info.pluginInfo.apkPath
                steps.add("   apkPath = $apkPath")
                val apkFile = java.io.File(apkPath)
                steps.add("   exists = ${apkFile.exists()}")
                steps.add("   canRead = ${apkFile.canRead()}")
                steps.add("   size = ${apkFile.length()} bytes")

                if (apkFile.exists()) {
                    steps.add("")
                    steps.add("   APK 内容检查:")
                    try {
                        val zipFile = java.util.zip.ZipFile(apkFile)
                        val entries = zipFile.entries().toList().map { it.name }
                        steps.add("      entry count = ${entries.size}")
                        steps.add("      has AndroidManifest.xml = ${entries.contains("AndroidManifest.xml")}")
                        steps.add("      has classes.dex = ${entries.any { it.startsWith("classes") && it.endsWith(".dex") }}")
                        steps.add("      has assets/simverse/ = ${entries.any { it.startsWith("assets/simverse/") }}")

                        val simverseAssets = entries.filter { it.startsWith("assets/simverse/") }
                        steps.add("      assets/simverse/ 条目数 = ${simverseAssets.size}")
                        if (simverseAssets.isNotEmpty()) {
                            steps.add("      前 10 个 simverse assets:")
                            simverseAssets.take(10).forEach { steps.add("        - $it") }
                        }

                        steps.add("      has index.html = ${entries.contains("assets/simverse/index.html")}")

                        zipFile.close()
                    } catch (e: Exception) {
                        steps.add("      ❌ ZIP 检查失败: ${e.message}")
                    }
                }
            } else {
                steps.add("   ⏭️  跳过（插件未加载）")
            }
        } catch (e: Exception) {
            steps.add("   ❌ APK 检查失败: ${e.message}")
        }
        steps.add("")

        steps.add("═══ 6. Target Activity 解析测试 ═══")
        try {
            val ctx = context ?: throw IllegalStateException("context is null")
            val intent = Intent()
            intent.setClassName(ctx, TARGET_ACTIVITY)
            val resolveInfo = ctx.packageManager.resolveActivity(intent, 0)
            if (resolveInfo != null) {
                steps.add("   ✅ Target Activity 可解析: $TARGET_ACTIVITY")
                steps.add("      resolveInfo.activityInfo.name = ${resolveInfo.activityInfo.name}")
                steps.add("      resolveInfo.activityInfo.packageName = ${resolveInfo.activityInfo.packageName}")
            } else {
                steps.add("   ❌ Target Activity 不可解析: $TARGET_ACTIVITY")
                steps.add("   这可能意味着:")
                steps.add("     - 插件未正确安装")
                steps.add("     - AndroidManifest 中 Activity 声明有误")
                steps.add("     - ComboLite 代理机制未正确注册")
            }
        } catch (e: Exception) {
            steps.add("   ❌ Activity 解析测试 FAILED: ${e.javaClass.simpleName}: ${e.message}")
        }
        steps.add("")

        steps.add("═══ 7. Host Activity (EncvHostActivity) 检查 ═══")
        try {
            val ctx = context ?: throw IllegalStateException("context is null")
            val intent = Intent(ctx, EncvHostActivity::class.java)
            val resolveInfo = ctx.packageManager.resolveActivity(intent, 0)
            if (resolveInfo != null) {
                steps.add("   ✅ EncvHostActivity 可解析")
                steps.add("      name = ${resolveInfo.activityInfo.name}")
            } else {
                steps.add("   ❌ EncvHostActivity 不可解析")
            }
        } catch (e: Exception) {
            steps.add("   ❌ EncvHostActivity 检查 FAILED: ${e.message}")
        }
        steps.add("")

        steps.add("═══ 8. createProxyIntent 构造测试 ═══")
        try {
            val act = activity
            if (act == null) {
                steps.add("   ⚠️  当前无 Activity 上下文（后台调用？）")
            } else {
                val testExtras = mapOf<String, Any>(
                    "world_id" to "debug-test",
                    "world_name" to "Debug Test World",
                    "api_base_url" to getApiBaseUrl(context ?: act),
                )
                val testIntent = EncvComboLiteHost.createProxyIntent(
                    context = act,
                    pluginId = PLUGIN_ID,
                    targetActivity = TARGET_ACTIVITY,
                    hostActivityClass = EncvHostActivity::class.java,
                    extras = testExtras
                )
                steps.add("   ✅ createProxyIntent 调用成功")
                steps.add("      component = ${testIntent.component}")
                steps.add("      action = ${testIntent.action}")
                steps.add("      extras:")
                testIntent.extras?.keySet()?.forEach { key ->
                    steps.add("        $key = ${testIntent.extras?.get(key)}")
                }
            }
        } catch (e: Exception) {
            steps.add("   ❌ createProxyIntent FAILED: ${e.javaClass.simpleName}: ${e.message}")
            steps.add("   stack = ${e.stackTraceToString().take(600)}")
        }
        steps.add("")

        steps.add("═══ 9. API Base URL 检查 ═══")
        try {
            val url = getApiBaseUrl(context ?: throw IllegalStateException("context is null"))
            steps.add("   apiBaseUrl = $url")
            steps.add("   SharedPreferences server_port 已读取")
        } catch (e: Exception) {
            steps.add("   ❌ API Base URL 检查 FAILED: ${e.message}")
        }
        steps.add("")

        steps.add("═══ 10. ensurePluginLoaded 加载测试 ═══")
        try {
            val state = EncvComboLiteHost.getPluginFullState(PLUGIN_ID)
            if (state.status == "not_loaded" || state.status == "load_failed") {
                steps.add("   当前状态: ${state.status}，尝试加载...")
                val loaded = EncvComboLiteHost.ensurePluginLoaded(PLUGIN_ID)
                steps.add("   ensurePluginLoaded result = $loaded")
                if (loaded) {
                    val afterState = EncvComboLiteHost.getPluginFullState(PLUGIN_ID)
                    steps.add("   加载后状态: ${afterState.status}")
                }
            } else {
                steps.add("   当前状态: ${state.status}（无需加载）")
            }
        } catch (e: Exception) {
            steps.add("   ❌ ensurePluginLoaded 测试 FAILED: ${e.javaClass.simpleName}: ${e.message}")
            steps.add("   stack = ${e.stackTraceToString().take(600)}")
        }
        steps.add("")

        steps.add("═══ 11. 插件 entryClass 反射检查 ═══")
        try {
            val info = PluginManager.getPluginInfo(PLUGIN_ID)
            if (info != null) {
                val entryClass = info.pluginInfo.entryClass
                steps.add("   entryClass = $entryClass")
                try {
                    val clazz = Class.forName(entryClass)
                    steps.add("   ✅ Class.forName 成功")
                    steps.add("      superclass = ${clazz.superclass?.name}")
                    steps.add("      interfaces = ${clazz.interfaces.map { it.name }}")

                    val methods = clazz.declaredMethods.map { it.name }
                    steps.add("      declared methods (first 20) = ${methods.take(20)}")

                    val hasContent = methods.any { it == "Content" || it.contains("Content") }
                    steps.add("      has Content method = $hasContent")
                } catch (e: ClassNotFoundException) {
                    steps.add("   ❌ Class.forName FAILED: 类找不到")
                    steps.add("   这可能意味着:")
                    steps.add("     - 插件 DEX 未正确加载")
                    steps.add("     - entryClass 名称错误")
                    steps.add("     - ClassLoader 隔离问题")
                } catch (e: Exception) {
                    steps.add("   ❌ 反射检查 FAILED: ${e.javaClass.simpleName}: ${e.message}")
                }
            } else {
                steps.add("   ⏭️  跳过（插件未加载）")
            }
        } catch (e: Exception) {
            steps.add("   ❌ entryClass 检查 FAILED: ${e.message}")
        }
        steps.add("")

        steps.add("═══ 12. 设备信息 ═══")
        steps.add("   SDK_INT = ${android.os.Build.VERSION.SDK_INT}")
        steps.add("   RELEASE = ${android.os.Build.VERSION.RELEASE}")
        steps.add("   BRAND = ${android.os.Build.BRAND}")
        steps.add("   MODEL = ${android.os.Build.MODEL}")
        steps.add("   ABIs = ${android.os.Build.SUPPORTED_ABIS.toList()}")
        steps.add("")

        steps.add("╔══════════════════════════════════════════════════╗")
        steps.add("║                  诊断结束                          ║")
        steps.add("╚══════════════════════════════════════════════════╝")

        call.resolve(JSObject().put("debugLog", steps.joinToString("\n")))
    }

    @PluginMethod
    fun openWorld(call: PluginCall) {
        try {
            val worldId = call.getString("worldId") ?: "default"
            val worldName = call.getString("worldName") ?: "Default"
            val activity = this.activity ?: return
            val ctx = context ?: return

            Log.e(TAG, "openWorld: worldId=$worldId worldName=$worldName")

            if (!EncvComboLiteHost.isInitialized) {
                Log.e(TAG, "openWorld: ComboLite not initialized")
                call.reject("ComboLite framework not initialized")
                return
            }

            val state = EncvComboLiteHost.getPluginFullState(PLUGIN_ID)
            Log.e(TAG, "openWorld: plugin state=${state.status} name=${state.name} version=${state.version}")

            when (state.status) {
                "not_installed" -> {
                    call.reject("SimVerse plugin not installed")
                    return
                }
                "disabled" -> {
                    call.reject("SimVerse plugin is disabled")
                    return
                }
                "framework_not_ready" -> {
                    call.reject("ComboLite framework not ready")
                    return
                }
                "not_loaded", "load_failed" -> {
                    Log.e(TAG, "openWorld: plugin state=${state.status}, attempting load...")
                    val loaded = EncvComboLiteHost.ensurePluginLoaded(PLUGIN_ID)
                    Log.e(TAG, "openWorld: ensurePluginLoaded result=$loaded")
                    if (!loaded) {
                        call.reject("Failed to load SimVerse plugin")
                        return
                    }
                }
            }

            val loadedInfo = EncvComboLiteHost.getLoadedPluginInfo(PLUGIN_ID)
            if (loadedInfo == null) {
                call.reject("SimVerse plugin info not available")
                return
            }
            Log.e(TAG, "openWorld: loadedInfo id=${loadedInfo.pluginInfo.id} name=${loadedInfo.pluginInfo.name}")

            val extras = mapOf<String, Any>(
                "world_id" to worldId,
                "world_name" to worldName,
                "api_base_url" to getApiBaseUrl(ctx),
            )
            val intent = EncvComboLiteHost.createProxyIntent(
                context = activity,
                pluginId = PLUGIN_ID,
                targetActivity = TARGET_ACTIVITY,
                hostActivityClass = EncvHostActivity::class.java,
                extras = extras
            )
            intent.addFlags(Intent.FLAG_ACTIVITY_NEW_DOCUMENT or Intent.FLAG_ACTIVITY_RETAIN_IN_RECENTS)
            activity.startActivity(intent)

            Log.e(TAG, "openWorld: startActivity dispatched")
            call.resolve()
        } catch (e: Exception) {
            Log.e(TAG, "openWorld failed: ${e.message}", e)
            call.reject(e.message)
        }
    }

    @PluginMethod
    fun closeWorld(call: PluginCall) {
        try {
            val activity = this.activity
            if (activity is EncvHostActivity) {
                activity.finish()
            }
            call.resolve()
        } catch (e: Exception) {
            call.reject(e.message)
        }
    }

    @PluginMethod
    fun startHeartbeat(call: PluginCall) {
        try {
            val ctx = context ?: return
            SimverseHeartbeatWorker.schedule(ctx)
            call.resolve()
        } catch (e: Exception) {
            call.reject(e.message)
        }
    }

    @PluginMethod
    fun stopHeartbeat(call: PluginCall) {
        try {
            val ctx = context ?: return
            SimverseHeartbeatWorker.cancel(ctx)
            call.resolve()
        } catch (e: Exception) {
            call.reject(e.message)
        }
    }

    @PluginMethod
    fun setWorldRunning(call: PluginCall) {
        try {
            val running = call.getBoolean("running") ?: false
            val ctx = context ?: return
            SimverseHeartbeatWorker.setWorldRunning(ctx, running)
            call.resolve()
        } catch (e: Exception) {
            call.reject(e.message)
        }
    }

    @PluginMethod
    fun addShortcut(call: PluginCall) {
        try {
            val ctx = context ?: return
            ShortcutHelper.addWorldShortcut(ctx)
            call.resolve()
        } catch (e: Exception) {
            call.reject(e.message)
        }
    }

    @PluginMethod
    fun isShortcutSupported(call: PluginCall) {
        try {
            val ctx = context ?: return
            val supported = ShortcutHelper.isSupported(ctx)
            val ret = JSObject()
            ret.put("supported", supported)
            call.resolve(ret)
        } catch (e: Exception) {
            call.reject(e.message)
        }
    }

    private fun getApiBaseUrl(context: Context): String {
        val prefs = context.getSharedPreferences("encv_go_prefs", Context.MODE_PRIVATE)
        val port = prefs.getInt("server_port", 8780)
        return "http://127.0.0.1:$port"
    }
}
