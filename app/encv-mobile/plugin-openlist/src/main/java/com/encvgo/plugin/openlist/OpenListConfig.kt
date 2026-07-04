package com.encvgo.plugin.openlist

import android.content.Context
import android.content.SharedPreferences
import android.util.Log
import java.io.File

data class OpenListConfig(
    val port: Int = DEFAULT_PORT,
    val dataDir: String = "",
    val adminPassword: String = "",
) {
    companion object {
        private const val TAG = "OpenList-Config"
        const val DEFAULT_PORT = 5244
        private const val PREFS_NAME = "openlist_config"
        private const val KEY_PORT = "port"
        private const val KEY_DATA_DIR = "data_dir"
        private const val KEY_ADMIN_PASSWORD = "admin_password"

        fun defaultDataDir(context: Context): String =
            File(context.filesDir, "openlist/data").absolutePath

        fun load(context: Context): OpenListConfig {
            val prefs: SharedPreferences = context.getSharedPreferences(PREFS_NAME, Context.MODE_PRIVATE)
            val port = prefs.getInt(KEY_PORT, DEFAULT_PORT)
            val dataDir = prefs.getString(KEY_DATA_DIR, null) ?: defaultDataDir(context)
            val adminPassword = prefs.getString(KEY_ADMIN_PASSWORD, "") ?: ""
            val cfg = OpenListConfig(
                port = port,
                dataDir = dataDir,
                adminPassword = adminPassword
            )
            Log.e(TAG, "[SAT-DBG][OpenList] load() | port=$port dataDir=$dataDir adminPasswordLen=${adminPassword.length}")
            return cfg
        }

        fun save(context: Context, port: Int, dataDir: String, adminPassword: String) {
            Log.e(TAG, "[SAT-DBG][OpenList] save() | port=$port dataDir=$dataDir adminPasswordLen=${adminPassword.length}")
            val prefs = context.getSharedPreferences(PREFS_NAME, Context.MODE_PRIVATE)
            prefs.edit()
                .putInt(KEY_PORT, port)
                .putString(KEY_DATA_DIR, dataDir)
                .putString(KEY_ADMIN_PASSWORD, adminPassword)
                .apply()
        }
    }

    /**
     * Phase 26: 改为空实现（gomobile bind 移除后无 bridge 可 apply）。
     * config 实际生效在 OpenListNativeService.start() —— 它把 port/dataDir 传给
     * libopenlist.so 的 --port / --data 参数。
     */
    fun applyToBridge(@Suppress("UNUSED_PARAMETER") bridge: Any) {
        Log.e(TAG, "[SAT-DBG][OpenList] applyToBridge() is now a no-op (Phase 26 ProcessBuilder mode)")
    }
}
