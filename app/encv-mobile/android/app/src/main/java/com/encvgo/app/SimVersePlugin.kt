package com.encvgo.app

import android.app.Activity
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
import com.combo.core.runtime.PluginManager

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
        steps.add("║     SimVerse 全链路饱和诊断 (v2)                  ║")
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
        val targetInstalled = try {
            val allPlugins = PluginManager.getAllInstallPlugins()
            steps.add("   total installed = ${allPlugins.size}")
            satLog("S03-INSTALLED", "已安装插件数: ${allPlugins.size}")
            val target = allPlugins.find { it.id == PLUGIN_ID }
            if (target != null) {
                steps.add("   ✅ 目标插件 '$PLUGIN_ID' 已安装")
                steps.add("      versionName = ${target.versionName}")
                steps.add("      enabled = ${target.enabled}")
                steps.add("      entryClass = ${target.entryClass}")
                satLog("S03-INSTALLED", "目标插件已安装: v${target.versionName}, enabled=${target.enabled}")
                true
            } else {
                steps.add("   ❌ 目标插件 '$PLUGIN_ID' 未在已安装列表中")
                steps.add("   已安装插件列表:")
                allPlugins.forEach { p ->
                    steps.add("      - ${p.id} (v${p.versionName}, enabled=${p.enabled})")
                }
                satWarn("S03-INSTALLED", "目标插件未安装，已安装: ${allPlugins.map { it.id }}")
                false
            }
        } catch (e: Exception) {
            steps.add("   ❌ getAllInstallPlugins FAILED: ${e.message}")
            satError("S03-INSTALLED", "getAllInstallPlugins 失败", e)
            false
        }
        steps.add("")

        steps.add("═══ 4. 插件 Info 详细检查 ═══")
        val loadedInfo = try {
            val info = PluginManager.getPluginInfo(PLUGIN_ID)
            if (info != null) {
                val pi = info.pluginInfo
                steps.add("   ✅ getPluginInfo 成功（插件已加载）")
                steps.add("      id = ${pi.id}")
                steps.add("      name = ${pi.name}")
                steps.add("      versionName = ${pi.versionName}")
                steps.add("      versionCode = ${pi.versionCode}")
                steps.add("      enabled = ${pi.enabled}")
                steps.add("      entryClass = ${pi.entryClass}")
                steps.add("      description = ${pi.description}")
                satLog("S04-LOADED", "插件已加载: ${pi.id} v${pi.versionName}")
                info
            } else {
                steps.add("   ⚠️  getPluginInfo 返回 null（插件未加载）")
                satWarn("S04-LOADED", "插件未加载")
                null
            }
        } catch (e: Exception) {
            steps.add("   ❌ getPluginInfo FAILED: ${e.javaClass.simpleName}: ${e.message}")
            steps.add("   stack = ${e.stackTraceToString().take(600)}")
            satError("S04-LOADED", "getPluginInfo 失败", e)
            null
        }
        steps.add("")

        steps.add("═══ 5. 插件 APK / DEX 检查 ═══")
        try {
            if (loadedInfo != null) {
                val pi = loadedInfo.pluginInfo
                steps.add("   插件已加载，检查 ClassLoader:")
                val classLoader = pi.javaClass.classLoader
                steps.add("      classLoader = $classLoader")
                steps.add("      classLoader.parent = ${classLoader?.parent}")
                satLog("S05-APK", "ClassLoader: $classLoader")

                steps.add("")
                steps.add("   尝试加载 plugin entry class:")
                try {
                    val entryClassName = pi.entryClass
                    if (entryClassName != null) {
                        val clazz = Class.forName(entryClassName)
                        steps.add("      ✅ Class.forName('$entryClassName') 成功")
                        steps.add("         superclass = ${clazz.superclass?.name}")
                        steps.add("         interfaces = ${clazz.interfaces.map { it.name }}")
                        steps.add("         classLoader = ${clazz.classLoader}")
                        satLog("S05-APK", "entry class 加载成功: $entryClassName")

                        val methods = clazz.declaredMethods.map { it.name }
                        steps.add("         declared methods (first 20) = ${methods.take(20)}")

                        val hasContent = methods.any { it == "Content" }
                        steps.add("         has Content() = $hasContent")
                    } else {
                        steps.add("      ⚠️  entryClass 为 null")
                        satWarn("S05-APK", "entryClass 为 null")
                    }
                } catch (e: ClassNotFoundException) {
                    steps.add("      ❌ ClassNotFoundException: ${e.message}")
                    satError("S05-APK", "entry class 加载失败: ClassNotFoundException", e)
                } catch (e: Exception) {
                    steps.add("      ❌ ${e.javaClass.simpleName}: ${e.message}")
                    satError("S05-APK", "entry class 检查失败", e)
                }
            } else if (targetInstalled) {
                steps.add("   插件已安装但未加载，跳过 DEX 检查")
                satWarn("S05-APK", "插件已安装但未加载，跳过 DEX 检查")
            } else {
                steps.add("   ⏭️  跳过（插件未安装）")
            }
        } catch (e: Exception) {
            steps.add("   ❌ APK/DEX 检查失败: ${e.message}")
            satError("S05-APK", "APK/DEX 检查失败", e)
        }
        steps.add("")

        steps.add("═══ 6. Target Activity 解析测试 ═══")
        val activityResolved = try {
            val ctx = context ?: throw IllegalStateException("context is null")
            val intent = Intent()
            intent.setClassName(ctx, TARGET_ACTIVITY)
            val resolveInfo = ctx.packageManager.resolveActivity(intent, 0)
            if (resolveInfo != null) {
                steps.add("   ✅ Target Activity 可直接解析: $TARGET_ACTIVITY")
                steps.add("      name = ${resolveInfo.activityInfo.name}")
                steps.add("      packageName = ${resolveInfo.activityInfo.packageName}")
                satLog("S06-ACTIVITY", "Target Activity 可直接解析: $TARGET_ACTIVITY")
                true
            } else {
                steps.add("   ⚠️  Target Activity 不可直接解析（这是正常的，ComboLite 通过代理启动）")
                steps.add("   接下来验证 ComboLite 代理启动流程...")
                satWarn("S06-ACTIVITY", "Target Activity 不可直接解析（ComboLite 代理机制预期行为）")
                false
            }
        } catch (e: Exception) {
            steps.add("   ❌ Activity 解析测试 FAILED: ${e.javaClass.simpleName}: ${e.message}")
            satError("S06-ACTIVITY", "Activity 解析测试失败", e)
            false
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

        steps.add("═══ 8. createProxyIntent 构造测试 ═══")
        val proxyIntentOk = try {
            val act = activity
            if (act == null) {
                steps.add("   ⚠️  当前无 Activity 上下文（后台调用？）")
                satWarn("S08-PROXY", "无 Activity 上下文")
                false
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
                satLog("S08-PROXY", "createProxyIntent 成功: component=${testIntent.component}")
                true
            }
        } catch (e: Exception) {
            steps.add("   ❌ createProxyIntent FAILED: ${e.javaClass.simpleName}: ${e.message}")
            steps.add("   stack = ${e.stackTraceToString().take(600)}")
            satError("S08-PROXY", "createProxyIntent 失败", e)
            false
        }
        steps.add("")

        steps.add("═══ 9. API Base URL 检查 ═══")
        val apiBaseOk = try {
            val ctx = context ?: throw IllegalStateException("context is null")
            val url = getApiBaseUrl(ctx)
            steps.add("   apiBaseUrl = $url")
            steps.add("   ✅ SharedPreferences server_port 已读取")
            satLog("S09-API", "apiBaseUrl = $url")
            true
        } catch (e: Exception) {
            steps.add("   ❌ API Base URL 检查 FAILED: ${e.message}")
            satError("S09-API", "API Base URL 检查失败", e)
            false
        }
        steps.add("")

        steps.add("═══ 10. ensurePluginLoaded 加载测试 ═══")
        try {
            val state = EncvComboLiteHost.getPluginFullState(PLUGIN_ID)
            if (state.status == "not_loaded" || state.status == "load_failed") {
                steps.add("   当前状态: ${state.status}，尝试加载...")
                satLog("S10-LOAD", "插件未加载，尝试 ensurePluginLoaded")
                val loaded = EncvComboLiteHost.ensurePluginLoaded(PLUGIN_ID)
                steps.add("   ensurePluginLoaded result = $loaded")
                satLog("S10-LOAD", "ensurePluginLoaded result = $loaded")
                if (loaded) {
                    val afterState = EncvComboLiteHost.getPluginFullState(PLUGIN_ID)
                    steps.add("   加载后状态: ${afterState.status}")
                    satLog("S10-LOAD", "加载后状态: ${afterState.status}")
                }
            } else {
                steps.add("   当前状态: ${state.status}（无需加载）")
                satLog("S10-LOAD", "当前状态: ${state.status}，无需加载")
            }
        } catch (e: Exception) {
            steps.add("   ❌ ensurePluginLoaded 测试 FAILED: ${e.javaClass.simpleName}: ${e.message}")
            steps.add("   stack = ${e.stackTraceToString().take(600)}")
            satError("S10-LOAD", "ensurePluginLoaded 测试失败", e)
        }
        steps.add("")

        steps.add("═══ 11. 插件 entryClass 反射方法检查 ═══")
        try {
            val info = PluginManager.getPluginInfo(PLUGIN_ID)
            if (info != null) {
                val entryClass = info.pluginInfo.entryClass
                steps.add("   entryClass = $entryClass")
                if (entryClass != null) {
                    try {
                        val clazz = Class.forName(entryClass)
                        steps.add("   ✅ Class.forName 成功")

                        val methods = clazz.methods.map { "${it.name}(${it.parameterTypes.map { t -> t.simpleName }.joinToString(",")})" }
                        steps.add("   public methods (first 25):")
                        methods.take(25).forEach { m ->
                            steps.add("      - $m")
                        }

                        val fields = clazz.fields.map { it.name }
                        if (fields.isNotEmpty()) {
                            steps.add("   public fields (first 10): ${fields.take(10)}")
                        }

                        satLog("S11-REFLECT", "反射检查完成，public methods=${methods.size}")
                    } catch (e: ClassNotFoundException) {
                        steps.add("   ❌ Class.forName FAILED: 类找不到")
                        satError("S11-REFLECT", "Class.forName 失败: ClassNotFoundException", e)
                    } catch (e: Exception) {
                        steps.add("   ❌ 反射检查 FAILED: ${e.javaClass.simpleName}: ${e.message}")
                        satError("S11-REFLECT", "反射检查失败", e)
                    }
                }
            } else {
                steps.add("   ⏭️  跳过（插件未加载）")
            }
        } catch (e: Exception) {
            steps.add("   ❌ entryClass 检查 FAILED: ${e.message}")
            satError("S11-REFLECT", "entryClass 检查失败", e)
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

        steps.add("═══ 13. 诊断小结 ═══")
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
            val steps = mutableListOf<String>()
            steps.add("[自动诊断 - 原因: $reason]")
            steps.add("frameworkInit=${EncvComboLiteHost.isInitialized}")
            val state = EncvComboLiteHost.getPluginFullState(PLUGIN_ID)
            steps.add("pluginState=${state.status}")
            steps.add("pluginName=${state.name}")
            steps.add("pluginVersion=${state.version}")
            steps.joinToString(" | ")
        } catch (e: Exception) {
            "自动诊断失败: ${e.javaClass.simpleName}: ${e.message}"
        }
    }

    @PluginMethod
    fun openWorld(call: PluginCall) {
        try {
            val worldId = call.getString("worldId") ?: "default"
            val worldName = call.getString("worldName") ?: "Default"
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
                    satLog("OPEN-WORLD", "plugin state=${state.status}, attempting load...")
                    val loaded = EncvComboLiteHost.ensurePluginLoaded(PLUGIN_ID)
                    satLog("OPEN-WORLD", "ensurePluginLoaded result=$loaded")
                    if (!loaded) {
                        val diag = runAutoDiagnosis("ensurePluginLoaded failed")
                        satError("OPEN-WORLD", "ensurePluginLoaded failed, $diag")
                        call.reject("Failed to load SimVerse plugin. $diag")
                        return
                    }
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
