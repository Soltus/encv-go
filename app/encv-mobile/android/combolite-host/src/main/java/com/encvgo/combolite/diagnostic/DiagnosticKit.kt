package com.encvgo.combolite.diagnostic

import android.content.Context
import android.os.Build
import android.content.pm.PackageManager
import com.combo.core.runtime.PluginManager
import com.combo.core.security.crash.PluginCrashHandler
import com.combo.core.security.auth.AuthorizationManager
import com.encvgo.combolite.EncvComboLiteHost
import com.encvgo.combolite.engine.PluginLifecycleEngine
import java.io.File
import kotlin.reflect.jvm.javaMethod

object DiagnosticKit {

    fun lifecycleDiagnostic(pluginId: String, context: Context): List<String> {
        val steps = mutableListOf<String>()

        steps.add("=== Plugin Lifecycle Diagnostic ===")
        steps.add("1. PluginManager State:")
        steps.add("   isInitialized = ${PluginManager.isInitialized}")

        if (PluginManager.isInitialized) {
            try {
                val plugins = PluginManager.getAllInstallPlugins()
                steps.add("   installedPlugins(${plugins.size}) = ${plugins.joinToString { "${it.id}(v${it.versionName},enabled=${it.enabled})" }}")
                val target = plugins.find { it.id == pluginId }
                if (target != null) {
                    steps.add("   ✅ target '$pluginId' FOUND: v${target.versionName}, enabled=${target.enabled}")
                } else {
                    steps.add("   ❌ target '$pluginId' NOT found in installed list")
                }
            } catch (e: Exception) {
                steps.add("   getAllInstallPlugins FAILED: ${e.message}")
            }

            steps.add("")
            steps.add("2. ProxyManager State:")
            try {
                val pm = PluginManager.proxyManager
                steps.add("   proxyManager = $pm")
                steps.add("   hostActivity configured = ${pm != null}")
            } catch (e: Exception) {
                steps.add("   proxyManager check FAILED: ${e.message}")
            }

            steps.add("")
            steps.add("3. Activity Resolution Test:")

            steps.add("")
            steps.add("4. setPluginEnabled test (dry run):")
            try {
                val info = PluginManager.getPluginInfo(pluginId)
                if (info != null) {
                    val p = info.pluginInfo
                    steps.add("   getPluginInfo('$pluginId') = ✅ (id=${p.id}, versionName=${p.versionName}, enabled=${p.enabled})")
                } else {
                    steps.add("   getPluginInfo('$pluginId') = ❌ null (not loaded?)")
                }
            } catch (e: Exception) {
                steps.add("   getPluginInfo FAILED: ${e.javaClass.simpleName}: ${e.message}")
            } catch (e: Error) {
                steps.add("   getPluginInfo ERROR: ${e.javaClass.simpleName}: ${e.message}")
            }

            steps.add("")
            steps.add("5. uninstallPlugin test (dry run — will NOT execute):")
            steps.add("   installerManager.available = ${try { PluginManager.installerManager != null } catch (e: Exception) { "ERROR: ${e.message}" }}")
        }

        return steps
    }

    fun kotlinReflectHealthCheck(): List<String> {
        val steps = mutableListOf<String>()
        steps.add("=== kotlin-reflect Health Check ===")

        steps.add("1. Testing ::function.javaMethod on PluginManager...")
        try {
            val method = PluginManager::setValidationStrategy.javaMethod
            steps.add("   setValidationStrategy.javaMethod = $method")
            steps.add("   declaringClass = ${method?.declaringClass?.name}")
        } catch (e: Error) {
            steps.add("   FAILED with Error: ${e.javaClass.simpleName}: ${e.message}")
        } catch (e: Exception) {
            steps.add("   FAILED with Exception: ${e.javaClass.simpleName}: ${e.message}")
        }

        try {
            val method = PluginManager::loadEnabledPlugins.javaMethod
            steps.add("   loadEnabledPlugins.javaMethod = $method")
        } catch (e: Error) {
            steps.add("   loadEnabledPlugins FAILED: ${e.javaClass.simpleName}: ${e.message}")
        }

        try {
            val method = PluginManager::launchPlugin.javaMethod
            steps.add("   launchPlugin.javaMethod = $method")
        } catch (e: Error) {
            steps.add("   launchPlugin FAILED: ${e.javaClass.simpleName}: ${e.message}")
        }

        steps.add("2. Testing ::function.javaMethod on InstallerManager...")
        try {
            if (PluginManager.isInitialized) {
                val im = PluginManager.installerManager
                val method = im::installPlugin.javaMethod
                steps.add("   installPlugin.javaMethod = $method")
                steps.add("   declaringClass = ${method?.declaringClass?.name}")
            } else {
                steps.add("   SKIPPED: PluginManager not initialized")
            }
        } catch (e: Error) {
            steps.add("   FAILED with Error: ${e.javaClass.simpleName}: ${e.message}")
        }

        steps.add("3. Testing ::function.javaMethod on PluginCrashHandler...")
        try {
            val method = PluginCrashHandler::setGlobalClashCallback.javaMethod
            steps.add("   setGlobalClashCallback.javaMethod = $method")
        } catch (e: Error) {
            steps.add("   FAILED with Error: ${e.javaClass.simpleName}: ${e.message}")
        }

        steps.add("4. Testing ::function.javaMethod on AuthorizationManager...")
        try {
            if (PluginManager.isInitialized) {
                val am = PluginManager.authorizationManager
                val method = am::setAuthorizationHandler.javaMethod
                steps.add("   setAuthorizationHandler.javaMethod = $method")
            } else {
                steps.add("   SKIPPED: PluginManager not initialized")
            }
        } catch (e: Error) {
            steps.add("   FAILED with Error: ${e.javaClass.simpleName}: ${e.message}")
        }

        steps.add("")
        steps.add("5. Checking @Metadata on ComboLite classes...")
        try {
            val pmClass = PluginManager::class.java
            val meta = pmClass.getAnnotation(kotlin.Metadata::class.java)
            steps.add("   PluginManager @Metadata = ${meta != null}")
            if (meta != null) {
                steps.add("   mv = ${meta.metadataVersion.toList()}")
                steps.add("   k = ${meta.kind}")
            }
        } catch (e: Exception) {
            steps.add("   @Metadata check FAILED: ${e.message}")
        }
        try {
            val imClass = Class.forName("com.combo.core.runtime.installer.InstallerManager")
            val meta = imClass.getAnnotation(kotlin.Metadata::class.java)
            steps.add("   InstallerManager @Metadata = ${meta != null}")
            if (meta != null) {
                steps.add("   mv = ${meta.metadataVersion.toList()}")
                steps.add("   k = ${meta.kind}")
            }
        } catch (e: Exception) {
            steps.add("   InstallerManager @Metadata check FAILED: ${e.message}")
        }

        steps.add("")
        steps.add("6. Checking R8 mapping on ComboLite classes...")
        try {
            val pmMethods = PluginManager::class.java.declaredMethods.map { "${it.name}(${it.parameterTypes.map { t -> t.simpleName }})" }
            steps.add("   PluginManager methods (first 10) = ${pmMethods.take(10)}")
        } catch (e: Exception) {
            steps.add("   PluginManager method list FAILED: ${e.message}")
        }
        try {
            val imClass = Class.forName("com.combo.core.runtime.installer.InstallerManager")
            val imMethods = imClass.declaredMethods.map { "${it.name}(${it.parameterTypes.map { t -> t.simpleName }})" }
            steps.add("   InstallerManager methods (first 10) = ${imMethods.take(10)}")
        } catch (e: Exception) {
            steps.add("   InstallerManager method list FAILED: ${e.message}")
        }

        return steps
    }

    fun apkValidation(apkFile: File, context: Context): List<String> {
        val steps = mutableListOf<String>()
        steps.add("=== APK Validation ===")

        if (!apkFile.exists()) {
            steps.add("1. APK file not found: ${apkFile.absolutePath}")
            return steps
        }

        steps.add("1. APK file = ${apkFile.name} (${apkFile.length()}B)")
        steps.add("   exists = ${apkFile.exists()}")
        steps.add("   canRead = ${apkFile.canRead()}")

        steps.add("2. PackageManager.getPackageArchiveInfo...")
        try {
            var flags = PackageManager.GET_META_DATA or PackageManager.GET_ACTIVITIES
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.P) {
                flags = flags or PackageManager.GET_SIGNING_CERTIFICATES
            } else {
                @Suppress("DEPRECATION") flags = flags or PackageManager.GET_SIGNATURES
            }
            val pkgInfo = context.packageManager.getPackageArchiveInfo(apkFile.absolutePath, flags)
            if (pkgInfo == null) {
                steps.add("   FAILED: getPackageArchiveInfo returned null (invalid APK?)")
            } else {
                steps.add("   packageName = ${pkgInfo.packageName}")
                steps.add("   versionName = ${pkgInfo.versionName}")

                val appInfo = pkgInfo.applicationInfo
                if (appInfo != null) {
                    appInfo.publicSourceDir = apkFile.absolutePath
                    val metaData = appInfo.metaData
                    if (metaData != null) {
                        steps.add("   metaData keys = ${metaData.keySet().toList()}")
                        steps.add("   plugin.entryClass = ${metaData.getString("plugin.entryClass")}")
                    } else {
                        steps.add("   metaData = NULL ← CRITICAL: plugin must have meta-data")
                    }
                }

                val activities = pkgInfo.activities
                steps.add("   activities = ${activities?.map { it.name } ?: "none"}")
            }
        } catch (e: Exception) {
            steps.add("   FAILED: ${e.javaClass.simpleName}: ${e.message}")
        }

        steps.add("3. APK ZIP integrity...")
        try {
            val zipFile = java.util.zip.ZipFile(apkFile)
            val entries = zipFile.entries().toList().map { it.name }
            steps.add("   entry count = ${entries.size}")
            steps.add("   has AndroidManifest.xml = ${entries.contains("AndroidManifest.xml")}")
            steps.add("   has classes.dex = ${entries.any { it.startsWith("classes") && it.endsWith(".dex") }}")
            steps.add("   has resources.arsc = ${entries.contains("resources.arsc")}")
            zipFile.close()
        } catch (e: Exception) {
            steps.add("   FAILED: ${e.message}")
        }

        steps.add("")
        steps.add("4. Host app signatures...")
        try {
            val sigFlags = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.P) PackageManager.GET_SIGNING_CERTIFICATES else @Suppress("DEPRECATION") PackageManager.GET_SIGNATURES
            val hostPkgInfo = context.packageManager.getPackageInfo(context.packageName, sigFlags)
            val hostSigs = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.P) {
                hostPkgInfo.signingInfo?.apkContentsSigners?.map { it.toCharsString().take(16) + "..." } ?: listOf("(none)")
            } else {
                @Suppress("DEPRECATION") hostPkgInfo.signatures?.map { it.toCharsString().take(16) + "..." } ?: listOf("(none)")
            }
            steps.add("   host signatures = $hostSigs")
        } catch (e: Exception) {
            steps.add("   FAILED: ${e.message}")
        }

        return steps
    }

    fun validationStrategyStatus(context: Context): List<String> {
        val steps = mutableListOf<String>()
        steps.add("=== ValidationStrategy State ===")
        steps.add("1. PluginManager.isInitialized = ${PluginManager.isInitialized}")

        if (!PluginManager.isInitialized) {
            steps.add("2. SKIPPED: PluginManager not initialized")
            return steps
        }

        try {
            val currentStrategy = PluginManager.validationStrategy
            steps.add("2. current validationStrategy = $currentStrategy")
        } catch (e: Exception) {
            steps.add("2. reading validationStrategy FAILED: ${e.javaClass.simpleName}: ${e.message}")
        }

        steps.add("3. Testing loadEnabledPlugins.javaMethod...")
        try {
            val method = PluginManager::loadEnabledPlugins.javaMethod
            steps.add("   loadEnabledPlugins.javaMethod = $method")
        } catch (e: Error) {
            steps.add("   FAILED with Error: ${e.javaClass.simpleName}: ${e.message}")
        }

        steps.add("")
        steps.add("4. Testing setValidationStrategy(Insecure)...")
        try {
            kotlinx.coroutines.runBlocking(kotlinx.coroutines.Dispatchers.IO) {
                PluginManager.setValidationStrategy(com.combo.core.runtime.ValidationStrategy.Insecure)
                com.combo.core.security.crash.PluginCrashHandler.setGlobalClashCallback(null)
            }
            steps.add("   SUCCESS: no error thrown")
        } catch (e: Error) {
            steps.add("   FAILED with Error: ${e.javaClass.simpleName}: ${e.message}")
            steps.add("   stack = ${e.stackTraceToString().take(500)}")
        } catch (e: Exception) {
            steps.add("   FAILED with Exception: ${e.javaClass.simpleName}: ${e.message}")
        }

        steps.add("5. Testing loadEnabledPlugins() call...")
        try {
            val count: Int = kotlinx.coroutines.runBlocking(kotlinx.coroutines.Dispatchers.IO) {
                PluginLifecycleEngine.loadAllEnabledPlugins()
            }
            steps.add("   loadEnabledPlugins() returned $count")
        } catch (e: Error) {
            steps.add("   FAILED with Error: ${e.javaClass.simpleName}: ${e.message}")
            steps.add("   stack = ${e.stackTraceToString().take(500)}")
        } catch (e: Exception) {
            steps.add("   FAILED with Exception: ${e.javaClass.simpleName}: ${e.message}")
        }

        steps.add("6. EncvApplication onFrameworkSetup log...")
        try {
            val logFile = File(context.filesDir, "encv.log")
            if (logFile.exists()) {
                val lines = logFile.readLines().filter { it.contains("onFrameworkSetup") || it.contains("setValidationStrategy") || it.contains("ValidationStrategy") }
                steps.add("   relevant log lines = ${lines.take(10)}")
            } else {
                steps.add("   encv.log not found")
            }
        } catch (e: Exception) {
            steps.add("   FAILED: ${e.message}")
        }

        return steps
    }

    fun installTest(apkFile: File, context: Context): List<String> {
        val steps = mutableListOf<String>()
        steps.add("=== Actual installPlugin Test ===")

        if (!PluginManager.isInitialized) {
            steps.add("SKIPPED: PluginManager not initialized")
            return steps
        }

        if (!apkFile.exists()) {
            steps.add("SKIPPED: no APK file found")
            return steps
        }

        steps.add("1. testApk = ${apkFile.name} (${apkFile.length()}B)")
        steps.add("2. calling installPlugin(testApk, forceOverwrite=true)...")

        try {
            val result = kotlinx.coroutines.runBlocking(kotlinx.coroutines.Dispatchers.IO) {
                PluginManager.installerManager.installPlugin(apkFile, true)
            }
            when (result) {
                is com.combo.core.runtime.installer.InstallerManager.InstallResult.Success -> {
                    steps.add("3. installPlugin result = SUCCESS")
                    steps.add("   pluginId = ${result.pluginInfo.id}")
                    steps.add("   versionName = ${result.pluginInfo.versionName}")
                    steps.add("   entryClass = ${result.pluginInfo.entryClass}")
                }
                is com.combo.core.runtime.installer.InstallerManager.InstallResult.Failure -> {
                    steps.add("3. installPlugin result = FAILURE")
                    steps.add("   reason = ${result.reason}")
                    result.exception?.let { steps.add("   exception = ${it.stackTraceToString().take(800)}") }
                }
            }
        } catch (e: Error) {
            steps.add("3. installPlugin threw Error: ${e.javaClass.simpleName}: ${e.message}")
        } catch (e: Exception) {
            steps.add("3. installPlugin threw Exception: ${e.javaClass.simpleName}: ${e.message}")
        }

        steps.add("")
        steps.add("4. Post-Install State...")
        if (EncvComboLiteHost.isInitialized) {
            try {
                val allPlugins = EncvComboLiteHost.getInstalledPlugins()
                steps.add("installedPlugins after = ${allPlugins.map { "${it.id}(enabled=${it.enabled})" }}")
            } catch (e: Exception) {
                steps.add("getInstalledPlugins FAILED: ${e.message}")
            }
            val pluginsDir = File(context.filesDir, "plugins")
            steps.add("plugins dir exists = ${pluginsDir.exists()}")
            if (pluginsDir.exists()) {
                steps.add("   contents = ${pluginsDir.listFiles()?.map { it.name }}")
            }
        }

        return steps
    }
}
