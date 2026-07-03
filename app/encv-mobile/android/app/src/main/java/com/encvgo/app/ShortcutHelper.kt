package com.encvgo.app

import android.content.Context
import android.content.Intent
import android.content.pm.ShortcutInfo
import android.content.pm.ShortcutManager
import android.graphics.drawable.Icon
import android.os.Build
import android.util.Log

object ShortcutHelper {
    private const val TAG = "ShortcutHelper"
    private const val SHORTCUT_ID_WORLD = "simverse_world"

    fun isSupported(context: Context): Boolean {
        return Build.VERSION.SDK_INT >= Build.VERSION_CODES.N_MR1
    }

    fun addWorldShortcut(context: Context): Boolean {
        if (Build.VERSION.SDK_INT < Build.VERSION_CODES.O) {
            Log.w(TAG, "Pinned shortcuts not supported on this API level")
            return false
        }

        val shortcutManager = context.getSystemService(ShortcutManager::class.java)
        if (shortcutManager == null) {
            Log.w(TAG, "ShortcutManager not available")
            return false
        }

        if (!shortcutManager.isRequestPinShortcutSupported) {
            Log.w(TAG, "Pinned shortcuts not supported by launcher")
            return false
        }

        val intent = Intent(context, WorldActivity::class.java).apply {
            action = Intent.ACTION_VIEW
            addFlags(Intent.FLAG_ACTIVITY_NEW_DOCUMENT or Intent.FLAG_ACTIVITY_RETAIN_IN_RECENTS)
        }

        val shortcutInfo = ShortcutInfo.Builder(context, SHORTCUT_ID_WORLD)
            .setShortLabel("SimVerse World")
            .setLongLabel("进入 SimVerse 世界")
            .setIcon(Icon.createWithResource(context, android.R.drawable.ic_menu_compass))
            .setIntent(intent)
            .build()

        return try {
            shortcutManager.requestPinShortcut(shortcutInfo, null)
            Log.i(TAG, "Pinned shortcut requested")
            true
        } catch (e: Exception) {
            Log.e(TAG, "Failed to request pinned shortcut", e)
            false
        }
    }

    fun updateWorldShortcut(context: Context, tick: Int, npcCount: Int) {
        if (Build.VERSION.SDK_INT < Build.VERSION_CODES.N_MR1) return

        val shortcutManager = context.getSystemService(ShortcutManager::class.java) ?: return

        val intent = Intent(context, WorldActivity::class.java).apply {
            action = Intent.ACTION_VIEW
        }

        val label = "世界 T${tick} · ${npcCount}NPC"
        val shortcutInfo = ShortcutInfo.Builder(context, SHORTCUT_ID_WORLD)
            .setShortLabel(label)
            .setLongLabel("SimVerse 世界 - Tick $tick, $npcCount NPCs")
            .setIcon(Icon.createWithResource(context, android.R.drawable.ic_menu_compass))
            .setIntent(intent)
            .build()

        try {
            shortcutManager.updateShortcuts(listOf(shortcutInfo))
        } catch (e: Exception) {
            Log.w(TAG, "Failed to update shortcut", e)
        }
    }
}
