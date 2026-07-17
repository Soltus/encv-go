package com.encvgo.app

import android.content.Context
import android.content.Intent
import android.util.Log
import com.getcapacitor.JSObject
import com.getcapacitor.Plugin
import com.getcapacitor.PluginCall
import com.getcapacitor.PluginMethod
import com.getcapacitor.annotation.CapacitorPlugin
import com.encvgo.app.workers.SimverseHeartbeatWorker
import com.encvgo.combolite.EncvComboLiteHost
import com.combo.core.api.IPluginEntryClass
import com.combo.core.runtime.PluginManager
import com.combo.core.runtime.loader.PluginClassLoader
import java.io.File

@CapacitorPlugin(name = "SimVerse")
class SimVersePlugin : Plugin() {

    companion object {
        private const val TAG = "SimVersePlugin"
        private const val SAT_TAG = "SimVerse-SAT"
        private const val PLUGIN_ID = "com.encvgo.plugin.simverse"
        private const val TARGET_ACTIVITY = "com.encvgo.plugin.simverse.SimVerseActivity"
    }

    private fun satLog(step: String, message: String) {
        Log.e(SAT_TAG, "[$step] $message")
    }

    private fun satWarn(step: String, message: String) {
        Log.w(SAT_TAG, "[$step] $message")
    }

    private fun satError(step: String, message: String, throwable: Throwable? = null) {
        Log.e(SAT_TAG, "[$step] $message", throwable)
    }

    @PluginMethod
    fun debugSimVerseFlow(call: PluginCall) {
        val steps = mutableListOf<String>()

        steps.add("╔══════════════════════════════════════════════════╗")
        steps.add("║     SimVerse 全链路饱和诊断 (v5)                  ║")
        steps.add("╚══════════════════════════════════════════════════╝")
        steps.add("")

        satLog("S00-START", "开始全链路饱和诊断")

        steps.add("═══ 1. ComboLite 框架状态 ═══")
        val frameworkOk = try {
            val encvInit = EncvComboLiteHost.isInitialized
            steps.add("   EncvComboLiteHost.isInitialized = $encvInit")
            val pmInit = try { PluginManager.isInitialized } catch (e: Exception) {
                steps.add("   PluginManager.isInitialized = ERROR: ${e.message}")
                false
            }
            steps.add("   PluginManager.isInitialized = $pmInit")
            if (encvInit) {
                steps.add("   ✅ ComboLite 已初始化")
                satLog("S01-FRAMEWORK", "ComboLite 已初始化")
                true
            } else {
                steps.add("   ❌ ComboLite 未初始化，后续诊断跳过")
                satError("S01-FRAMEWORK", "ComboLite 未初始化")
                false
            }
        } catch (e: Exception) {
            steps.add("   ❌ 框架检查 FAILED: ${e.javaClass.simpleName}: ${e.message}")
            satError("S01-FRAMEWORK", "框架检查失败", e)
            false
        }
        steps.add("")

        if (!frameworkOk) {
            steps.add("╔══════════════════════════════════════════════════╗")
            steps.add("║              诊断结束（框架未就绪）                ║")
            steps.add("╚══════════════════════════════════════════════════╝")
            call.resolve(JSObject().put("debugLog", steps.joinToString("\n")))
            return
        }

        steps.add("═══ 2. 插件安装/加载状态 ═══")
        val fullState = try {
            val state = EncvComboLiteHost.getPluginFullState(PLUGIN_ID)
            steps.add("   status = ${state.status}")
            steps.add("   name = ${state.name}")
            steps.add("   version = ${state.version}")
            satLog("S02-STATE", "插件状态: ${state.status}, name=${state.name}, version=${state.version}")
            state
        } catch (e: Exception) {
            steps.add("   ❌ getPluginFullState FAILED: ${e.javaClass.simpleName}: ${e.message}")
            satError("S02-STATE", "getPluginFullState 失败", e)
            null
        }
        steps.add("")

        steps.add("═══ 3. 已安装插件列表 ═══")
        var targetInstalled = false
        var targetPluginInfo: com.combo.core.model.PluginInfo? = null
        try {
            val allPlugins = PluginManager.getAllInstallPlugins()
            steps.add("   total installed = ${allPlugins.size}")
            satLog("S03-INSTALLED", "已安装插件数: ${allPlugins.size}")
            val target = allPlugins.find { it.id == PLUGIN_ID }
            if (target != null) {
                targetInstalled = true
                targetPluginInfo = target
                steps.add("   ✅ 目标插件 '$PLUGIN_ID' 已安装")
                steps.add("      versionName = ${target.versionName}")
                steps.add("      versionCode = ${target.versionCode}")
                steps.add("      enabled = ${target.enabled}")
                steps.add("      entryClass = ${target.entryClass}")
                steps.add("      path = ${target.path}")
                steps.add("      installTime = ${target.installTime}")
                satLog("S03-INSTALLED", "目标插件已安装: v${target.versionName}, enabled=${target.enabled}, path=${target.path}")
            } else {
                steps.add("   ❌ 目标插件 '$PLUGIN_ID' 未在已安装列表中")
                steps.add("   已安装插件列表:")
                allPlugins.forEach { p ->
                    steps.add("      - ${p.id} (v${p.versionName}, enabled=${p.enabled})")
                }
                satWarn("S03-INSTALLED", "目标插件未安装，已安装: ${allPlugins.map { it.id }}")
            }
        } catch (e: Exception) {
            steps.add("   ❌ getAllInstallPlugins FAILED: ${e.message}")
            satError("S03-INSTALLED", "getAllInstallPlugins 失败", e)
        }
        steps.add("")

        steps.add("═══ 4. 插件 APK 文件检查 ═══")
        try {
            if (targetPluginInfo != null) {
                val apkPath = targetPluginInfo.path
                steps.add("   apkPath = $apkPath")
                val apkFile = File(apkPath)
                steps.add("   exists = ${apkFile.exists()}")
                steps.add("   canRead = ${apkFile.canRead()}")
                steps.add("   size = ${apkFile.length()} bytes (${"%.2f".format(apkFile.length() / 1024.0 / 1024.0)} MB)")
                satLog("S04-APK", "APK 文件: exists=${apkFile.exists()}, size=${apkFile.length()}")

                if (apkFile.exists()) {
                    steps.add("")
                    steps.add("   APK ZIP 内容检查:")
                    try {
                        val zipFile = java.util.zip.ZipFile(apkFile)
                        val entries = zipFile.entries().toList().map { it.name }
                        steps.add("      total entries = ${entries.size}")
                        steps.add("      has AndroidManifest.xml = ${entries.contains("AndroidManifest.xml")}")

                        val dexCount = entries.count { it.startsWith("classes") && it.endsWith(".dex") }
                        steps.add("      dex files = $dexCount")

                        val hasSimverseAssets = entries.any { it.startsWith("assets/simverse/") }
                        steps.add("      has assets/simverse/ = $hasSimverseAssets")

                        val simverseAssets = entries.filter { it.startsWith("assets/simverse/") }
                        steps.add("      assets/simverse/ 条目数 = ${simverseAssets.size}")
                        if (simverseAssets.isNotEmpty()) {
                            steps.add("      前 10 个 simverse assets:")
                            simverseAssets.take(10).forEach { steps.add("        - $it") }
                        }

                        steps.add("      has index.html = ${entries.contains("assets/simverse/index.html")}")

                        zipFile.close()
                        satLog("S04-APK", "APK 内容检查通过: entries=${entries.size}, simverseAssets=${simverseAssets.size}")
                    } catch (e: Exception) {
                        steps.add("      ❌ ZIP 检查失败: ${e.message}")
                        satError("S04-APK", "ZIP 检查失败", e)
                    }
                }
            } else {
                steps.add("   ⏭️  跳过（插件未安装）")
            }
        } catch (e: Exception) {
            steps.add("   ❌ APK 文件检查失败: ${e.message}")
            satError("S04-APK", "APK 文件检查失败", e)
        }
        steps.add("")

        steps.add("═══ 5. 插件加载状态 & 实例检查 ═══")
        var loadedInstance: IPluginEntryClass? = null
        try {
            val loadedInfo = EncvComboLiteHost.getLoadedPluginInfo(PLUGIN_ID)
            if (loadedInfo != null) {
                val pi = loadedInfo.pluginInfo
                steps.add("   ✅ 插件已加载")
                steps.add("      id = ${pi.id}")
                steps.add("      name = ${pi.name}")
                steps.add("      versionName = ${pi.versionName}")
                steps.add("      entryClass = ${pi.entryClass}")
                satLog("S05-LOADED", "插件已加载: ${pi.id} v${pi.versionName}")

                val classLoader = loadedInfo.classLoader
                steps.add("")
                steps.add("   ClassLoader 信息:")
                steps.add("      type = ${classLoader.javaClass.simpleName}")
                steps.add("      is PluginClassLoader = ${classLoader is PluginClassLoader}")
                steps.add("      parent = ${classLoader.parent}")
                satLog("S05-LOADED", "ClassLoader: ${classLoader.javaClass.simpleName}")

                steps.add("")
                steps.add("   插件入口类实例检查 (PluginManager.getPluginInstance):")
                val instance = PluginManager.getPluginInstance(PLUGIN_ID)
                if (instance != null) {
                    loadedInstance = instance
                    steps.add("      ✅ getPluginInstance 返回实例")
                    steps.add("      class = ${instance.javaClass.name}")
                    steps.add("      classLoader = ${instance.javaClass.classLoader}")
                    satLog("S05-LOADED", "插件实例获取成功: ${instance.javaClass.name}")
                } else {
                    steps.add("      ❌ getPluginInstance 返回 null")
                    satError("S05-LOADED", "getPluginInstance 返回 null")
                }

                steps.add("")
                steps.add("   所有已加载插件实例:")
                val allInstances = PluginManager.getAllPluginInstances()
                steps.add("      count = ${allInstances.size}")
                allInstances.forEach { (id, inst) ->
                    steps.add("      - $id → ${inst.javaClass.simpleName}")
                }
            } else {
                steps.add("   ⚠️  插件未加载（getLoadedPluginInfo 返回 null）")
                satWarn("S05-LOADED", "插件未加载")
            }
        } catch (e: Exception) {
            steps.add("   ❌ 加载状态检查 FAILED: ${e.javaClass.simpleName}: ${e.message}")
            steps.add("   stack = ${e.stackTraceToString().take(600)}")
            satError("S05-LOADED", "加载状态检查失败", e)
        }
        steps.add("")

        steps.add("═══ 6. Target Activity 类加载检查 ═══")
        try {
            val loadedInfo = EncvComboLiteHost.getLoadedPluginInfo(PLUGIN_ID)
            if (loadedInfo != null) {
                val classLoader = loadedInfo.classLoader
                steps.add("   targetActivity = $TARGET_ACTIVITY")
                try {
                    val activityClass = classLoader.loadClass(TARGET_ACTIVITY)
                    steps.add("   ✅ classLoader.loadClass 成功")
                    steps.add("      class = ${activityClass.name}")
                    steps.add("      superclass = ${activityClass.superclass?.name}")
                    steps.add("      interfaces = ${activityClass.interfaces.map { it.simpleName }}")
                    satLog("S06-ACTIVITY-CLASS", "Activity 类加载成功: ${activityClass.name}")
                } catch (e: ClassNotFoundException) {
                    steps.add("   ❌ ClassNotFoundException: ${e.message}")
                    satError("S06-ACTIVITY-CLASS", "Activity 类加载失败: ClassNotFoundException", e)
                } catch (e: Exception) {
                    steps.add("   ❌ ${e.javaClass.simpleName}: ${e.message}")
                    satError("S06-ACTIVITY-CLASS", "Activity 类检查失败", e)
                }
            } else {
                steps.add("   ⏭️  跳过（插件未加载）")
            }
        } catch (e: Exception) {
            steps.add("   ❌ Activity 类检查 FAILED: ${e.message}")
            satError("S06-ACTIVITY-CLASS", "Activity 类检查失败", e)
        }
        steps.add("")

        steps.add("═══ 7. Host Activity (EncvHostActivity) 检查 ═══")
        val hostActivityOk = try {
            val ctx = context ?: throw IllegalStateException("context is null")
            val intent = Intent(ctx, EncvHostActivity::class.java)
            val resolveInfo = ctx.packageManager.resolveActivity(intent, 0)
            if (resolveInfo != null) {
                steps.add("   ✅ EncvHostActivity 可解析")
                steps.add("      name = ${resolveInfo.activityInfo.name}")
                satLog("S07-HOST", "EncvHostActivity 可解析")
                true
            } else {
                steps.add("   ❌ EncvHostActivity 不可解析")
                satError("S07-HOST", "EncvHostActivity 不可解析")
                false
            }
        } catch (e: Exception) {
            steps.add("   ❌ EncvHostActivity 检查 FAILED: ${e.message}")
            satError("S07-HOST", "EncvHostActivity 检查失败", e)
            false
        }
        steps.add("")

        steps.add("═══ 8. ProxyManager 配置检查 ═══")
        try {
            val proxyManager = PluginManager.proxyManager
            steps.add("   proxyManager = $proxyManager")

            val hostActivity = proxyManager.getHostActivity()
            steps.add("   hostActivityClass = ${hostActivity?.name}")
            satLog("S08-PROXY", "hostActivityClass = ${hostActivity?.name}")

            val hostProvider = proxyManager.getHostProviderAuthority()
            steps.add("   hostProviderAuthority = $hostProvider")
            satLog("S08-PROXY", "hostProviderAuthority = $hostProvider")
        } catch (e: Exception) {
            steps.add("   ❌ ProxyManager 检查 FAILED: ${e.javaClass.simpleName}: ${e.message}")
            satError("S08-PROXY", "ProxyManager 检查失败", e)
        }
        steps.add("")

        steps.add("═══ 9. createProxyIntent 构造测试 ═══")
        try {
            val act = activity
            if (act == null) {
                steps.add("   ⚠️  当前无 Activity 上下文（后台调用？）")
                satWarn("S09-PROXY-INTENT", "无 Activity 上下文")
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
                steps.add("      flags = ${testIntent.flags}")
                steps.add("      extras:")
                testIntent.extras?.keySet()?.forEach { key ->
                    steps.add("        $key = ${testIntent.extras?.get(key)}")
                }
                satLog("S09-PROXY-INTENT", "createProxyIntent 成功: component=${testIntent.component}")
            }
        } catch (e: Exception) {
            steps.add("   ❌ createProxyIntent FAILED: ${e.javaClass.simpleName}: ${e.message}")
            steps.add("   stack = ${e.stackTraceToString().take(600)}")
            satError("S09-PROXY-INTENT", "createProxyIntent 失败", e)
        }
        steps.add("")

        steps.add("═══ 10. API Base URL 检查 ═══")
        try {
            val ctx = context ?: throw IllegalStateException("context is null")
            val url = getApiBaseUrl(ctx)
            steps.add("   apiBaseUrl = $url")
            steps.add("   ✅ SharedPreferences server_port 已读取")
            satLog("S10-API", "apiBaseUrl = $url")
        } catch (e: Exception) {
            steps.add("   ❌ API Base URL 检查 FAILED: ${e.message}")
            satError("S10-API", "API Base URL 检查失败", e)
        }
        steps.add("")

        steps.add("═══ 11. ensurePluginLoaded 加载测试 ═══")
        try {
            val state = EncvComboLiteHost.getPluginFullState(PLUGIN_ID)
            if (state.status == "not_loaded" || state.status == "load_failed") {
                steps.add("   当前状态: ${state.status}，尝试加载...")
                satLog("S11-LOAD", "插件未加载，尝试 ensurePluginLoaded")
                val loaded = EncvComboLiteHost.ensurePluginLoaded(PLUGIN_ID)
                steps.add("   ensurePluginLoaded result = $loaded")
                satLog("S11-LOAD", "ensurePluginLoaded result = $loaded")
                if (loaded) {
                    val afterState = EncvComboLiteHost.getPluginFullState(PLUGIN_ID)
                    steps.add("   加载后状态: ${afterState.status}")
                    satLog("S11-LOAD", "加载后状态: ${afterState.status}")

                    val afterInstance = PluginManager.getPluginInstance(PLUGIN_ID)
                    if (afterInstance != null) {
                        steps.add("   ✅ 加载后插件实例可用")
                        steps.add("      class = ${afterInstance.javaClass.name}")
                        satLog("S11-LOAD", "加载后插件实例可用: ${afterInstance.javaClass.name}")
                    }
                }
            } else {
                steps.add("   当前状态: ${state.status}（无需加载）")
                satLog("S11-LOAD", "当前状态: ${state.status}，无需加载")
            }
        } catch (e: Exception) {
            steps.add("   ❌ ensurePluginLoaded 测试 FAILED: ${e.javaClass.simpleName}: ${e.message}")
            steps.add("   stack = ${e.stackTraceToString().take(600)}")
            satError("S11-LOAD", "ensurePluginLoaded 测试失败", e)
        }
        steps.add("")

        steps.add("═══ 12. 设备信息 ═══")
        steps.add("   SDK_INT = ${android.os.Build.VERSION.SDK_INT}")
        steps.add("   RELEASE = ${android.os.Build.VERSION.RELEASE}")
        steps.add("   BRAND = ${android.os.Build.BRAND}")
        steps.add("   MODEL = ${android.os.Build.MODEL}")
        steps.add("   ABIs = ${android.os.Build.SUPPORTED_ABIS.toList()}")
        satLog("S12-DEVICE", "SDK=${android.os.Build.VERSION.SDK_INT}, BRAND=${android.os.Build.BRAND}, MODEL=${android.os.Build.MODEL}")
        steps.add("")

        steps.add("═══ 14. WebView 加载诊断 ═══")
        try {
            if (loadedInstance != null) {
                try {
                    val debugMethod = loadedInstance.javaClass.getMethod("debugWebView")
                    val webViewReport = debugMethod.invoke(loadedInstance) as? String
                    if (webViewReport != null) {
                        steps.add(webViewReport.prependIndent("   "))
                        satLog("S14-WV-DIAG", "WebView 诊断获取成功")
                    } else {
                        steps.add("   ⚠️  debugWebView 返回 null")
                        satWarn("S14-WV-DIAG", "debugWebView 返回 null")
                    }
                } catch (e: NoSuchMethodException) {
                    steps.add("   ⚠️  插件未实现 debugWebView 方法（可能是旧版本插件）")
                    satWarn("S14-WV-DIAG", "插件无 debugWebView 方法: ${e.message}")
                } catch (e: Exception) {
                    steps.add("   ❌ WebView 诊断调用失败: ${e.javaClass.simpleName}: ${e.message}")
                    satError("S14-WV-DIAG", "调用失败", e)
                }
            } else {
                steps.add("   ⏭️  跳过（插件实例未加载）")
                satWarn("S14-WV-DIAG", "插件未加载，跳过 WebView 诊断")
            }
        } catch (e: Exception) {
            steps.add("   ❌ WebView 诊断 FAILED: ${e.message}")
            satError("S14-WV-DIAG", "失败", e)
        }
        steps.add("")

        steps.add("═══ 15. 诊断小结 ═══")
        var passCount = 0
        var failCount = 0
        var warnCount = 0
        for (line in steps) {
            if (line.contains("✅")) passCount++
            if (line.contains("❌")) failCount++
            if (line.contains("⚠️")) warnCount++
        }
        steps.add("   ✅ 通过: $passCount")
        steps.add("   ⚠️  警告: $warnCount")
        steps.add("   ❌ 失败: $failCount")
        steps.add("")

        steps.add("╔══════════════════════════════════════════════════╗")
        steps.add("║                  诊断结束                          ║")
        steps.add("╚══════════════════════════════════════════════════╝")

        satLog("S00-END", "诊断结束: pass=$passCount, warn=$warnCount, fail=$failCount")

        call.resolve(JSObject().put("debugLog", steps.joinToString("\n")))
    }

    private fun runAutoDiagnosis(reason: String): String {
        satError("AUTO-DIAG", "自动触发饱和诊断，原因: $reason")
        return try {
            val parts = mutableListOf<String>()
            parts.add("[自动诊断 - 原因: $reason]")
            parts.add("frameworkInit=${EncvComboLiteHost.isInitialized}")
            val state = EncvComboLiteHost.getPluginFullState(PLUGIN_ID)
            parts.add("pluginState=${state.status}")
            parts.add("pluginName=${state.name}")
            parts.add("pluginVersion=${state.version}")

            if (state.status == "ready") {
                try {
                    val instance = PluginManager.getPluginInstance(PLUGIN_ID)
                    parts.add("instanceAvailable=${instance != null}")
                } catch (e: Exception) {
                    parts.add("instanceCheckFailed=${e.javaClass.simpleName}")
                }
            }

            parts.joinToString(" | ")
        } catch (e: Exception) {
            "自动诊断失败: ${e.javaClass.simpleName}: ${e.message}"
        }
    }

    @PluginMethod
    fun openWorld(call: PluginCall) {
        try {
            val worldId = call.getString("worldId") ?: "default"
            val worldName = call.getString("worldName") ?: "Default"
            val themeCss = call.getString("themeCss") ?: ""
            val activity = this.activity ?: run {
                val diag = runAutoDiagnosis("activity is null")
                satError("OPEN-WORLD", "activity is null, $diag")
                call.reject("Activity context not available. $diag")
                return
            }
            val ctx = context ?: run {
                val diag = runAutoDiagnosis("context is null")
                satError("OPEN-WORLD", "context is null, $diag")
                call.reject("Context not available. $diag")
                return
            }

            satLog("OPEN-WORLD", "worldId=$worldId worldName=$worldName")

            if (!EncvComboLiteHost.isInitialized) {
                val diag = runAutoDiagnosis("ComboLite not initialized")
                satError("OPEN-WORLD", "ComboLite not initialized, $diag")
                call.reject("ComboLite framework not initialized. $diag")
                return
            }

            val state = EncvComboLiteHost.getPluginFullState(PLUGIN_ID)
            satLog("OPEN-WORLD", "plugin state=${state.status} name=${state.name} version=${state.version}")

            when (state.status) {
                "not_installed" -> {
                    val diag = runAutoDiagnosis("plugin not installed")
                    satError("OPEN-WORLD", "plugin not installed, $diag")
                    call.reject("SimVerse plugin not installed. $diag")
                    return
                }
                "disabled" -> {
                    val diag = runAutoDiagnosis("plugin disabled")
                    satError("OPEN-WORLD", "plugin disabled, $diag")
                    call.reject("SimVerse plugin is disabled. $diag")
                    return
                }
                "framework_not_ready" -> {
                    val diag = runAutoDiagnosis("framework not ready")
                    satError("OPEN-WORLD", "framework not ready, $diag")
                    call.reject("ComboLite framework not ready. $diag")
                    return
                }
                "not_loaded", "load_failed" -> {
                    satLog("OPEN-WORLD", "plugin state=${state.status}, attempting load via launchPlugin...")
                    val loaded = EncvComboLiteHost.ensurePluginLoaded(PLUGIN_ID)
                    satLog("OPEN-WORLD", "ensurePluginLoaded result=$loaded")
                    if (!loaded) {
                        val diag = runAutoDiagnosis("ensurePluginLoaded failed")
                        satError("OPEN-WORLD", "ensurePluginLoaded failed, $diag")
                        call.reject("Failed to load SimVerse plugin. $diag")
                        return
                    }
                }
                "ready" -> {
                    satLog("OPEN-WORLD", "plugin already loaded and ready")
                }
            }

            val loadedInfo = EncvComboLiteHost.getLoadedPluginInfo(PLUGIN_ID)
            if (loadedInfo == null) {
                val diag = runAutoDiagnosis("loadedInfo is null after ensurePluginLoaded")
                satError("OPEN-WORLD", "loadedInfo is null, $diag")
                call.reject("SimVerse plugin info not available. $diag")
                return
            }
            satLog("OPEN-WORLD", "loadedInfo id=${loadedInfo.pluginInfo.id} name=${loadedInfo.pluginInfo.name}")

            val instance = PluginManager.getPluginInstance(PLUGIN_ID)
            if (instance == null) {
                val diag = runAutoDiagnosis("pluginInstance is null after load")
                satError("OPEN-WORLD", "pluginInstance is null, $diag")
                call.reject("SimVerse plugin instance not available. $diag")
                return
            }
            satLog("OPEN-WORLD", "pluginInstance class=${instance.javaClass.name}")

            val extras = mapOf<String, Any>(
                "world_id" to worldId,
                "world_name" to worldName,
                "api_base_url" to getApiBaseUrl(ctx),
                "theme_css" to themeCss,
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

            satLog("OPEN-WORLD", "startActivity dispatched")
            call.resolve()
        } catch (e: Exception) {
            val diag = runAutoDiagnosis("exception: ${e.javaClass.simpleName}")
            satError("OPEN-WORLD", "openWorld failed: ${e.message}, $diag", e)
            call.reject("${e.message}. $diag")
        }
    }

    @PluginMethod
    fun closeWorld(call: PluginCall) {
        try {
            val activity = this.activity
            if (activity is EncvHostActivity) {
                activity.finish()
                satLog("CLOSE-WORLD", "EncvHostActivity finished")
            } else {
                satLog("CLOSE-WORLD", "current activity is not EncvHostActivity, cannot close")
            }
            call.resolve()
        } catch (e: Exception) {
            satError("CLOSE-WORLD", "closeWorld failed: ${e.message}", e)
            call.reject(e.message)
        }
    }

    @PluginMethod
    fun unloadPlugin(call: PluginCall) {
        try {
            if (!EncvComboLiteHost.isInitialized) {
                call.reject("ComboLite framework not initialized")
                return
            }
            val state = EncvComboLiteHost.getPluginFullState(PLUGIN_ID)
            if (state.status != "ready") {
                call.reject("Plugin is not loaded, current state: ${state.status}")
                return
            }
            kotlinx.coroutines.runBlocking {
                PluginManager.unloadPlugin(PLUGIN_ID)
            }
            satLog("UNLOAD-PLUGIN", "plugin unloaded successfully")
            call.resolve()
        } catch (e: Exception) {
            satError("UNLOAD-PLUGIN", "unloadPlugin failed: ${e.message}", e)
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
