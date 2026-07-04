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

@CapacitorPlugin(name = "SimVerse")
class SimVersePlugin : Plugin() {

    companion object {
        private const val TAG = "SimVersePlugin"
    }

    @PluginMethod
    fun openWorld(call: PluginCall) {
        try {
            val worldId = call.getString("worldId") ?: "default"
            val worldName = call.getString("worldName") ?: "Default"
            val activity = this.activity ?: return
            val intent = WorldActivity.createIntent(activity, worldId, worldName).apply {
                addFlags(Intent.FLAG_ACTIVITY_NEW_DOCUMENT or Intent.FLAG_ACTIVITY_RETAIN_IN_RECENTS)
            }
            activity.startActivity(intent)
            call.resolve()
        } catch (e: Exception) {
            call.reject(e.message)
        }
    }

    @PluginMethod
    fun closeWorld(call: PluginCall) {
        try {
            val activity = this.activity
            if (activity is WorldActivity) {
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
}
