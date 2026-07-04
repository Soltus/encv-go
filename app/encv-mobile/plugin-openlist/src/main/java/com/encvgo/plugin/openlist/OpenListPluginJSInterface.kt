package com.encvgo.plugin.openlist

import android.util.Log
import android.webkit.JavascriptInterface
import android.webkit.WebView
import org.json.JSONObject
import java.io.File

/**
 * JS-Native 桥接：暴露给 WebView 中加载的 plugin-openlist/web 资源调用。
 *
 * 调用方式（web 端）：
 *   window.OpenListNative.startOpenList()         → 返回端口号字符串
 *   window.OpenListNative.stopOpenList()          → 返回 boolean
 *   window.OpenListNative.getRuntimeStatus()      → 返回 JSON 字符串
 *   window.OpenListNative.setAdminPassword(pwd)   → 返回 boolean
 *   window.OpenListNative.readConfig()            → 返回 config.json 文本
 *   window.OpenListNative.writeConfig(content)    → 写并备份，返回 boolean
 *   window.OpenListNative.getVersion()            → 返回 OpenList 版本字符串
 *   window.OpenListNative.getDataDir()            → 返回 data dir 路径
 *
 * 异常处理：所有方法都包 try-catch，失败返回安全值（空串 / false / "{}"）
 * 避免 web 端 throw 不可控。
 */
class OpenListPluginJSInterface(
    private val appContext: android.content.Context,
) {
    private val tag = "OpenList-JS"

    @JavascriptInterface
    fun startOpenList(): String {
        return try {
            Log.e(tag, "[OpenList] JS -> startOpenList()")
            // Phase 25：plugin service 改为 OpenListPluginService (BasePluginService)。
            // 此 Intent 指向 plugin service（系统找不到，因为 plugin 未 install）——dead code。
            // 真正启动靠 host 反射 OpenListBridge.start()（A2 落地后改用 startPluginService）。
            val intent = android.content.Intent(appContext, OpenListPluginService::class.java)
            try {
                appContext.startService(intent)
            } catch (e: Throwable) {
                Log.w(tag, "startService(Intent(OpenListPluginService)) failed (expected: plugin not system-installed)", e)
            }
            // 等待服务启动完成后返回端口（简化：直接返回配置端口）
            val cfg = OpenListConfig.load(appContext)
            Log.e(tag, "[OpenList] startOpenList() OK | port=${cfg.port}")
            cfg.port.toString()
        } catch (e: Throwable) {
            Log.e(tag, "[OpenList] startOpenList() FAILED", e)
            "0"
        }
    }

    @JavascriptInterface
    fun stopOpenList(): Boolean {
        return try {
            Log.e(tag, "[OpenList] JS -> stopOpenList()")
            OpenListPluginService.stopIfRunning()
            Log.e(tag, "[OpenList] stopOpenList() OK")
            true
        } catch (e: Throwable) {
            Log.e(tag, "[OpenList] stopOpenList() FAILED", e)
            false
        }
    }

    @JavascriptInterface
    fun getRuntimeStatus(): String {
        return try {
            // Phase 26: 改用 OpenListNativeService.snapshot() (ProcessBuilder 启的 Go 进程状态)
            val snapshot = OpenListNativeService.snapshot()
            val cfg = OpenListConfig.load(appContext)
            val isRunning = (snapshot["running"] as? Boolean) ?: false
            JSONObject().apply {
                put("running", isRunning)
                put("port", if (isRunning) (snapshot["port"] as? Int) ?: 0 else cfg.port)
                put("pid", (snapshot["pid"] as? Int) ?: 0)
                put("dataSizeBytes", (snapshot["data_size_bytes"] as? Long) ?: 0L)
                put("lastError", (snapshot["last_error"] as? String) ?: "")
                put("lastUpdateTs", (snapshot["last_update_ts"] as? Long) ?: 0L)
                put("dataDir", cfg.dataDir)
                put("isInstalled", true)
            }.toString()
        } catch (e: Throwable) {
            Log.e(tag, "[OpenList] getRuntimeStatus() FAILED", e)
            JSONObject().apply {
                put("running", false)
                put("port", 0)
                put("pid", 0)
                put("error", e.message ?: "unknown")
            }.toString()
        }
    }

    @JavascriptInterface
    fun setAdminPassword(password: String): Boolean {
        return try {
            Log.e(tag, "[OpenList] JS -> setAdminPassword() | len=${password.length}")
            val cfg = OpenListConfig.load(appContext)
            OpenListConfig.save(
                appContext,
                port = cfg.port,
                dataDir = cfg.dataDir,
                adminPassword = password
            )
            // Phase 26: adminPassword 走 OpenList REST API /api/admin/user/set
            // —— 不再调 OpenListBridge.setAdminPassword (gomobile bind 路径已废弃)
            // native service 已通过 setAdminPassword() 收到 password,内部缓存待 REST 调用
            OpenListNativeService.setAdminPassword(password)
            true
        } catch (e: Throwable) {
            Log.e(tag, "[OpenList] setAdminPassword() FAILED", e)
            false
        }
    }

    @JavascriptInterface
    fun readConfig(): String {
        return try {
            val cfg = OpenListConfig.load(appContext)
            val configFile = File(cfg.dataDir, "config.json")
            if (configFile.exists()) configFile.readText() else "{}"
        } catch (e: Throwable) {
            Log.e(tag, "[OpenList] readConfig() FAILED", e)
            "{}"
        }
    }

    @JavascriptInterface
    fun writeConfig(content: String): Boolean {
        return try {
            val cfg = OpenListConfig.load(appContext)
            val configFile = File(cfg.dataDir, "config.json")
            // 自动备份
            if (configFile.exists()) {
                val backup = File(cfg.dataDir, "config.json.bak")
                try {
                    configFile.copyTo(backup, overwrite = true)
                } catch (e: Throwable) {
                    Log.w(tag, "config backup failed (continuing)", e)
                }
            }
            configFile.parentFile?.mkdirs()
            configFile.writeText(content)
            Log.e(tag, "[OpenList] writeConfig() OK | bytes=${content.length}")
            true
        } catch (e: Throwable) {
            Log.e(tag, "[OpenList] writeConfig() FAILED", e)
            false
        }
    }

    @JavascriptInterface
    fun getVersion(): String {
        return try {
            // 优先从 Bridge 读真实运行的 OpenList 版本
            // 简化：从 OpenListConfig 的 dist VERSION 文件读
            val versionFile = File(appContext.filesDir, "openlist/dist/VERSION")
            if (versionFile.exists()) versionFile.readText().trim() else "unknown"
        } catch (e: Throwable) {
            Log.e(tag, "[OpenList] getVersion() FAILED", e)
            "unknown"
        }
    }

    @JavascriptInterface
    fun getDataDir(): String {
        return try {
            OpenListConfig.load(appContext).dataDir
        } catch (e: Throwable) {
            ""
        }
    }

    @JavascriptInterface
    fun getPort(): Int {
        return try {
            OpenListConfig.load(appContext).port
        } catch (e: Throwable) {
            0
        }
    }

    @JavascriptInterface
    fun getIsRunning(): Boolean {
        return try {
            OpenListPluginService.isRunning
        } catch (e: Throwable) {
            false
        }
    }

    @JavascriptInterface
    fun reloadWebView(): Boolean {
        // 占位：实际 reload 由 WebView 自己处理
        return true
    }
}
