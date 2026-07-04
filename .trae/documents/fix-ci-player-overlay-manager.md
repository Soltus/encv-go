# 修复 CI 构建：创建 PlayerOverlayManager.kt（完整设计）

## CI 日志结论

**FFmpeg 构建已通过** ✅ — P1/P2/P3 全部生效

**唯一阻塞错误**：Kotlin 编译 — `PlayerOverlayManager` 类缺失（5 处引用）

## 完整架构分析

### 两种启动路径 + 三种播放模式

```
Go 后端调用 openPlayer()
    │
    ├── activity 是 MainActivity?
    │       │
    │       ├── YES → PlayerOverlayManager.showOverlay()
    │       │              │
    │       │              ├── 用户选择 artplayer → bridge.webView.loadUrl(#/player) ← 真正的 overlay
    │       │              ├── 用户选择 mpv      → PlayerEntry.startMpvPlayer() (MpvPlayerActivity)
    │       │              └── 用户选择 external  → PlayerEntry.openExternal() (系统播放器)
    │       │
    │       └── NO  → PlayerEntry.play() (独立 Activity)
    │
    Go 后端调用 closePlayer()
        └── PlayerOverlayManager.hideOverlay()
               ├── artplayer: webView.history.back()
               ├── mpv: finish MpvPlayerActivity (如果还在)
               └── external: 标记状态即可
```

### OverlayManager 的职责（不只是"WebView overlay"）

它是一个**播放器会话生命周期管理器**：

| 方法 | artplayer | mpv | external |
|------|-----------|-----|----------|
| `showOverlay()` | WebView 导航 | startActivity(MpvPlayer) | startActivity(外部) |
| `hideOverlay()` | history.back() | 无操作（Activity 自管） | 标记状态 |
| `isOverlayShowing()` | true/false | true/false | true/false |

**为什么 mpv/external 也走 OverlayManager？**
- 统一的 `showing` 状态供 `MainActivity.onBackPressed()` 判断是否拦截返回键
- 统一的 `closePlayer()` 入口供 Go 后端调用
- 未来可扩展：如需在 mpv 播放时显示自定义 UI 浮层

## 实现方案

### 新建文件

**路径**：`android/app/src/main/java/com/encvgo/app/PlayerOverlayManager.kt`

```kotlin
package com.encvgo.app

import android.app.Activity
import android.util.Log
import com.getcapacitor.Bridge

class PlayerOverlayManager private constructor() {
    private var showing = false
        private set
    private var currentMode = ""
    
    fun showOverlay(activity: Activity, filePath: String, name: String, mimeType: String, isExternal: Boolean) {
        val mainActivity = activity as? MainActivity ?: run {
            Log.w(TAG, "showOverlay: not MainActivity, falling back to PlayerEntry")
            PlayerEntry.play(activity, filePath, name, mimeType, isExternal)
            showing = true
            return
        }
        
        val prefs = mainActivity.getSharedPreferences(PREFS_NAME, Activity.MODE_PRIVATE)
        val rawMode = prefs.getString(PREF_KEY_VIDEO_PLAYER, "artplayer") ?: "artplayer"
        currentMode = if (rawMode == "mpv") "mpv-plugin" else rawMode
        
        Log.i(TAG, "showOverlay: mode=$currentMode filePath=$filePath")
        
        when (currentMode) {
            "mpv-plugin" -> {
                PlayerEntry.play(mainActivity, filePath, name, mimeType, isExternal)
                showing = true
            }
            "external" -> {
                PlayerEntry.play(mainActivity, filePath, name, mimeType, true)
                showing = true
            }
            else -> {
                navigateToArtPlayer(mainActivity, filePath, name)
                showing = true
            }
        }
    }
    
    fun hideOverlay() {
        if (!showing) return
        
        when (currentMode) {
            "artplayer", "" -> {
                try {
                    currentActivity?.let { act ->
                        (act as? MainActivity)?.bridge?.webView?.loadUrl(
                            "javascript:window.history.back()"
                        )
                    }
                } catch (_: Exception) {}
            }
            else -> {}
        }
        
        showing = false
        currentActivity = null
        currentMode = ""
    }
    
    fun isOverlayShowing(): Boolean = showing
    
    companion object {
        private const val TAG = "PlayerOverlayMgr"
        private const val PREFS_NAME = "encv_player_prefs"
        private const val PREF_KEY_VIDEO_PLAYER = "video_player"
        @Volatile private var instance: PlayerOverlayManager? = null
        private var currentActivity: Activity? = null
        
        fun getInstance(): PlayerOverlayManager =
            instance ?: synchronized(this) { instance ?: PlayerOverlayManager().also { instance = it } }
    }
    
    private fun navigateToArtPlayer(activity: MainActivity, filePath: String, fileName: String) {
        currentActivity = activity
        val bridge = activity.bridge ?: run {
            Log.e(TAG, "navigateToArtPlayer: bridge is null")
            return
        }
        val playerUrl = buildPlayerUrl(bridge, filePath, fileName)
        Log.i(TAG, "navigateToArtPlayer: $playerUrl")
        try {
            bridge.webView.loadUrl(playerUrl)
        } catch (e: Exception) {
            Log.e(TAG, "navigateToArtPlayer failed", e)
        }
    }
}

private fun buildPlayerUrl(bridge: Bridge, filePath: String, fileName: String): String {
    val localUrl = bridge.localServer?.url()
    if (localUrl != null) {
        val encodedPath = java.net.URLEncoder.encode(filePath, "UTF-8")
        val encodedName = java.net.URLEncoder.encode(fileName, "UTF-8")
        return "${localUrl}#/player?path=${encodedPath}&name=${encodedName}"
    }
    return "#/player"
}
```

## 修改清单

| 操作 | 文件 |
|------|------|
| **新建** | `android/app/src/main/java/com/encvgo/app/PlayerOverlayManager.kt` |

仅新建 1 个文件，零修改现有代码。

## 清理

完成后删除：
- `/workspace/job_logs/` 目录
- `/workspace/job_logs.zip`
