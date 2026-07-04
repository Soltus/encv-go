# 修复：mpvModule attachToLayout 线程错误

## 问题分析

### 错误日志

```
WARN  onLoadSuccess: mpvModule still null after load
ERROR attachToLayout: failed: Only the original thread that created a view hierarchy
       can touch its views. Expected: main Calling: Lynx_JS
```

### 两个问题

**问题 1：线程错误**
`MpvPlayerModule.init{}` 在 Lynx JS 线程执行，而 `attachToLayout()` 调用 `rootLayout.addView()` 必须在 Android 主线程执行。

**问题 2：时序问题**
`onLoadSuccess` 回调触发时，`MpvPlayerModule.getInstance()` 仍返回 null。说明模块实例化发生在 `onLoadSuccess` 之后。

### 时序分析

```
主线程:  createLynxView() → renderTemplateUrl() → onLoadSuccess() ← 此时 mpvModule=null
JS线程:                                         → JS 执行 NativeModules.MpvPlayerModule.play()
                                                  → Lynx 创建 MpvPlayerModule 实例
                                                  → init{} 在 JS 线程执行
                                                  → attachToLayout() ← 线程错误！
```

---

## 修复方案

### Step 1：MpvPlayerModule.init{} 中用 mainHandler.post 切换到主线程

```kotlin
init {
    _instance = this
    LogRelay.get().relay(TAG, "info", "init: MpvPlayerModule created")
    mainHandler.post {
        val act = activity
        if (act is PlayerActivityLynx) {
            val root = act.findViewById<android.widget.FrameLayout>(R.id.lynx_player_root)
            if (root != null) {
                attachToLayout(root)
            } else {
                LogRelay.get().relay(TAG, "warn", "init: lynx_player_root not found on main thread")
            }
        }
    }
}
```

### Step 2：PlayerActivityLynx.onLoadSuccess() 中增加兜底 attach

```kotlin
override fun onLoadSuccess() {
    LogRelay.get().relay(CLIENT_TAG, "info", "onLoadSuccess: template loaded")
    // 兜底：如果模块已创建但还没 attach（通常不会，因为 init 中已自动 attach）
    val mpvModule = MpvPlayerModule.getInstance()
    if (mpvModule != null && rootLayout != null) {
        // 模块已创建，检查 surface 是否已 attach
        // init{} 中的 mainHandler.post 可能还没执行，这里再 post 一次确保
        mainHandler.post {
            if (mpvSurfaceViewNotAttached()) {
                mpvModule.attachToLayout(rootLayout!!)
            }
        }
    }
}
```

但更简洁的方式是：在 `onLoadSuccess` 中也 post 一个 attach 尝试，因为此时模块可能刚创建或即将创建。

### Step 3：更稳健的方案 — Activity 驱动 attach

与其让模块自己找 Activity 和 rootLayout，不如让 Activity 在合适的时机主动调用 attach。这是 Android 标准模式：

**MpvPlayerModule**：添加一个 `isAttached()` 方法，让 Activity 可以检查。

**PlayerActivityLynx**：在 `onLoadSuccess()` 和 `onRuntimeReady()` 中都尝试 attach，用 `isAttached()` 避免重复：

```kotlin
override fun onRuntimeReady() {
    tryAttachMpvModule()
}

override fun onLoadSuccess() {
    tryAttachMpvModule()
}

private fun tryAttachMpvModule() {
    val mpvModule = MpvPlayerModule.getInstance() ?: return
    if (mpvModule.isAttached()) return
    val root = findViewById<FrameLayout>(R.id.lynx_player_root) ?: return
    mpvModule.attachToLayout(root)
}
```

同时保留 `MpvPlayerModule.init{}` 中的 `mainHandler.post` attach 作为早期路径，这样无论模块何时创建都能 attach。

---

## 修改文件

1. `/workspace/app/encv-mobile/android/app/src/main/java/com/encvgo/app/MpvPlayerModule.kt`
   - init{} 中 `attachToLayout` 改为 `mainHandler.post { attachToLayout(root) }`
   - 添加 `isAttached(): Boolean` 方法

2. `/workspace/app/encv-mobile/android/app/src/main/java/com/encvgo/app/PlayerActivityLynx.kt`
   - 添加 `tryAttachMpvModule()` 方法
   - 在 `onRuntimeReady()` 和 `onLoadSuccess()` 中调用
