# 修复播放器 Surface 未创建导致播放无响应

## 根因确认（基于 logcat.txt + DevLogs 双源交叉验证）

### 完整时间线重建

```
时间          事件                                                            来源
───────────────────────────────────────────────────────────────────────────────
19:54:21.404  createLynxView: START                                            DevLogs ✅
19:54:21.448  onCreate: root layout found                                     DevLogs ✅
19:54:21.467  LynxViewClient registered                                        DevLogs ✅
19:54:21.486  onFirstScreen: first screen rendered                             DevLogs ✅
19:54:21.486  onLoadSuccess: template loaded                                   DevLogs ✅
19:54:22.370  ★ 用户点击 ▶ → handlePlayPause (state=idle)                      DevLogs ✅
19:54:22.387  getStreamUrl → URL 成功                                         DevLogs ✅
19:54:22.422  ❌ play: surfaceReady=false, mpvInitialized=false               logcat ✅
19:54:22.424  ❌ play: surface not ready, queuing url as pending                logcat ✅
19:54:22.429  MPV 初始化完成 (mpv_ready)                                       DevLogs ✅
19:54:22.440  dispatchStateChange(waiting_surface)                              DevLogs ✅
19:54:22.448  dispatchStateChange(playing) ← 但实际没在播！                     DevLogs ⚠️
19:54:27-28   GC + ImeTracker（无任何新播放器日志）                               logcat
19:55:53      onDestroy: cleaning up (用户按返回退出了)                          DevLogs
```

### 致命缺陷：[PlayerActivityLynx.kt:327-337](app/encv-mobile/android-overlay/app/src/main/java/com/encvgo/app/PlayerActivityLynx.kt#L327-L337)

```kotlin
// 当前代码 — attachToLayout 在 lynxView?.post{} 中异步执行
lynxView?.post {                          // ← 投递到 LynxView 的消息队列
    val mpvModule = MpvPlayerModule.getInstance()
    if (mpvModule != null && rootLayout != null) {
        mpvModule.attachToLayout(rootLayout!!) // ← SurfaceView 在这里创建
    }
}
```

**三个问题叠加**：

1. **时机太晚**：`attachToLayout` 在 `lynxView?.post{}` 中异步执行，而用户可能在 LynxView 首屏渲染后立即点击播放（~1秒内），此时 post 还没执行完
2. **日志缺失**：整个流程中 **零条** `attachToLayout: adding MPV surface view` 日志 — 说明要么 post 未执行，要么执行时 `getInstance()` 返回 null
3. **即使执行了也可能无效**：SurfaceView 添加到 index 0（LynxView 下方），但 LynxView 是 MATCH_PARENT × MATCH_PARENT + background #000（不透明），Android 可能认为被遮挡的 SurfaceView 无需创建 Surface → `surfaceCreated` 永远不触发

### 为什么之前说"不需要隔离 SurfaceView"是错的

之前分析认为 DestroyLayoutNode 是 CSS 导致的，修复 CSS 即可。但现在发现 **CSS 修复后 DestroyLayoutNode 已消失**（新 logcat 中完全没有），**真正的根因是 SurfaceView 从未创建**。CSS 修复是有效的（消除了布局抖动），但播放不工作的原因是另一回事。

---

## 修复方案

### Step 1：将 attachToLayout 从异步 post 改为同步立即执行

**文件**：`app/encv-mobile/android-overlay/app/src/main/java/com/encvgo/app/PlayerActivityLynx.kt`

将 L327-337 的 `lynxView?.post { ... }` 块改为 **同步调用**，确保 SurfaceView 在 LynxView addView 之后、renderTemplateUrl 之前就创建：

```kotlin
// 修改前（异步，不可靠）
lynxView?.addView(lynxView, lynxParams)
lynxView?.renderTemplateUrl(...)
lynxView?.post {
    mpvModule.attachToLayout(rootLayout!!)
}

// 修改后（同步，可靠）
rootLayout?.addView(lynxView, lynxParams)

val mpvModule = MpvPlayerModule.getInstance() ?: MpvPlayerModule.init(application)
if (rootLayout != null) {
    mpvModule.attachToLayout(rootLayout!!)
}

lynxView?.renderTemplateUrl("player.lynx.bundle", initData)
lynxView?.post(positionUpdateRunnable)
```

关键变化：
1. `attachToLayout` 移出 `lynxView?.post {}`，改为**直接同步调用**
2. 在 `renderTemplateUrl` **之前**执行（确保 Surface 就绪后再加载 JS）
3. 如果 `getInstance()` 为 null 则主动 `init()`

### Step 2：确保 SurfaceView 可见（z-order 保障）

当前 SurfaceView 通过 `addView(mpvSurfaceView, 0, params)` 添加到 index 0（ LynxView 下方），这是正确的 z-order（视频在底层，UI 在上层）。但要确保：

1. SurfaceView 的 `visibility` 默认为 VISIBLE（MpvSurfaceView 构造函数中已设置 keepScreenOn=true，默认可见）
2. SurfaceView 尺寸为 MATCH_PARENT × MATCH_PARENT（已有）

额外保险：在 attachToLayout 后显式设置 visibility：

```kotlin
fun attachToLayout(rootLayout: ViewGroup) {
    ...
    rootLayout.addView(mpvSurfaceView, 0, params)
    mpvSurfaceView.visibility = View.VISIBLE  // 显式确保可见
    ...
}
```

### Step 3：增加 attachToLayout 结果的日志验证

确保下次能从 DevLogs 看到 attachToLayout 是否成功：

```kotlin
// PlayerActivityLynx.createLynxView() 中
val mpvModule = MpvPlayerModule.getInstance() ?: MpvPlayerModule.init(application)
LogRelay.get().relay(TAG, "info", "createLynxView: mpvModule instance=$mpvModule")
if (rootLayout != null && mpvModule != null) {
    mpvModule.attachToLayout(rootLayout!!)
    LogRelay.get().relay(TAG, "info", "createLynxView: attachToLayout called synchronously")
} else {
    LogRelay.get().relay(TAG, "error", "createLynxView: cannot attachToLayout, rootLayout=$rootLayout, mpvModule=$mpvModule")
}
```

### Step 4：构建验证

```bash
node --check ../scripts/post-cap-sync.mjs
# Kotlin 类型检查
cd app/encv-mobile && node scripts/check-kotlin.mjs
```

---

## 不做的事

- ❌ 不修改 App.css（上一轮已修复，logcat 确认 DestroyLayoutNode 已消失）
- ❌ 不修改 AppComponent.tsx / PlayerControls.tsx（上一轮的打点和重试机制已生效）
- ❌ 不创建独立 FrameLayout 层（先验证同步 attachToLayout 是否足够）
