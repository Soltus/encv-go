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

@CapacitorPlugin(name = "SimVerse")
class SimVersePlugin : Plugin() {

    companion object {
        private const val TAG = "SimVersePlugin"
        private const val PLUGIN_ID = "com.encvgo.plugin.simverse"
        private const val TARGET_ACTIVITY = "com.encvgo.plugin.simverse.SimVerseActivity"
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
