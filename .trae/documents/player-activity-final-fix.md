# PlayerActivity 彻底修复 Plan（正确使用 Capacitor API）

## 核心思路转变

**之前**：hack `load()` → 在 bridge 创建过程中/之后 redirect → 各种时序问题 → 白屏

**现在**：完全不碰 `load()`，让 Capacitor 正常完成全部初始化流程，然后在 `onCreate()` 的 `super.onCreate()` 返回后再导航到 player.html

### 为什么这是正确的

Capacitor 8.x 的 `Bridge.Builder` 公开了 `setServerPath(ServerPath)` 方法：

```java
// Bridge.java 内部类
public static class Builder {
    public Builder setServerPath(ServerPath serverPath);  // 官方 API
    public Bridge create();
}
```

但 `setServerPath` 只能切换 asset base path（如 `public/` → 其他目录），不能指定具体 HTML 文件名。**最终还是要靠 URL 导航**，关键在于**正确的时机**。

### 正确时机 = `super.onCreate()` 之后

```
PlayerActivity.onCreate()
  ① registerPlugin(GoProcessPlugin)     // 注册自定义插件
  ② super.onCreate()                   // [BridgeActivity.onCreate]
       → setContentView(WebView)
       → bridgeBuilder.addPlugins(...)
       → this.load()                    // [我们的: 空的 或 super.load()]
           → bridge = builder.create()   // ★ bridge 在这里创建
           → webView.loadUrl(appUrl)    // 开始加载 index.html
  ③ [我们在这里] bridge 已经就绪 ✅
      → bridge.webView.loadUrl(player.html)  // 取代上面的 index.html
```

在步骤 ③ 时，bridge、WebView、LocalServer 全部就绪。此时的 `loadUrl(player.html)` 与 Capacitor 内部第一次 `loadUrl(appUrl)` 之间有完整的函数调用栈间隔，不存在"同一方法内连续两次 loadUrl"的竞态问题。

---

## 修改方案

### Step 1：PlayerActivity.kt — 删除 `load()` override，改为 `onCreate()` 后导航

```kotlin
class PlayerActivity : BridgeActivity() {

    // ❌ 删除整个 override fun load() { ... }

    override fun onCreate(savedInstanceState: Bundle?) {
        Log.i(TAG, "onCreate: start")
        
        // 1. 注册插件（必须在 super 之前）
        try {
            registerPlugin(GoProcessPlugin::class.java)
            Log.d(TAG, "onCreate: GoProcessPlugin registered")
        } catch (e: Exception) {
            Log.e(TAG, "onCreate: registerPlugin failed", e)
        }
        
        // 2. 让 Capacitor 完成 ALL 初始化（创建 bridge + WebView + LocalServer + load index.html）
        super.onCreate(savedInstanceState)
        Log.d(TAG, "onCreate: super done, bridge=${bridge != null}")
        
        // 3. ★ 在此点 bridge 100% 就绪，安全导航到 player.html
        registerBackendReceiver()
        resolveFileInfo(intent)
        navigateToPlayer()
        handleBackend()
    }

    private fun navigateToPlayer() {
        try {
            val playerUrl = "https://localhost/player.html"
            Log.i(TAG, "navigateToPlayer: $playerUrl, bridge=${bridge != null}, webView=${bridge?.webView != null}")
            bridge?.webView?.loadUrl(playerUrl)
            Log.i(TAG, "navigateToPlayer: loadUrl called successfully")
        } catch (e: Exception) {
            Log.e(TAG, "navigateToPlayer: failed", e)
        }
    }

    private fun handleBackend() {
        when {
            EncvGoService.isRunning && EncvGoService.lastKnownPort > 0 -> {
                notifyFrontend(EncvGoService.lastKnownPort, true, null, "player", null)
            }
            else -> {
                startBackendService(EncvGoService.ACTION_START, "player", null)
            }
        }
    }
    
    // ... 其余方法保持不变 ...
}
```

**关键变化**：
- **删除 `override fun load()`** — 不再 hack Capacitor 的初始化流程
- 新增 `navigateToPlayer()` — 在 `super.onCreate()` 返回后执行，时机完全安全
- `handleBackend()` 提取为独立方法 — 保持 onCreate 整洁

### Step 2：AndroidManifest.xml — Document-Centric 模型

```xml
<activity
    android:name=".PlayerActivity"
    android:exported="true"
    android:launchMode="standard"
    android:documentLaunchMode="always"
    android:maxRecents="16"
    android:taskAffinity="com.encvgo.app.player.task"
    android:theme="@style/AppTheme.NoActionBar"
    android:configChanges="orientation|keyboardHidden|keyboard|screenSize|locale|smallestScreenSize|screenLayout|uiMode"
    android:label="ENCV Player">
    <!-- intent-filter 不变 -->
</activity>
```

变更：
- `launchMode`: `singleTask` → `standard`
- **新增** `android:documentLaunchMode="always"`（文档中心模型核心）
- **新增** `android:maxRecents="16"`

### Step 3：GoProcessPlugin.kt — Intent Flags 补全

```kotlin
@PluginMethod
fun openInPlayer(call: PluginCall) {
    val path = call.getString("path", "")
    val name = call.getString("name", "")
    val mimeType = call.getString("mimeType", "")
    if (path.isNullOrEmpty()) {
        Log.w(TAG, "openInPlayer rejected: path is empty")
        call.reject("path is required")
        return
    }
    try {
        val uniqueId = System.currentTimeMillis().toString()
        val intent = Intent(activity, PlayerActivity::class.java).apply {
            addFlags(
                Intent.FLAG_ACTIVITY_NEW_DOCUMENT
                    or Intent.FLAG_ACTIVITY_MULTIPLE_TASK
                    or Intent.FLAG_ACTIVITY_RETAIN_IN_RECENTS
            )
            data = Uri.parse("encvgo://player/$uniqueId")
            putExtra("file_path", path)
            putExtra("file_name", name)
            putExtra("file_mime_type", mimeType)
        }
        Log.d(TAG, "openInPlayer: NEW_DOCUMENT+MULTIPLE_TASK+RETAIN_IN_RECENTS, data=${intent.data}")
        activity.startActivity(intent)
        call.resolve()
    } catch (e: Exception) {
        Log.e(TAG, "openInPlayer failed", e)
        call.reject("Failed to open player: ${e.message}")
    }
}
```

变更：
- **新增** `FLAG_ACTIVITY_NEW_DOCUMENT`（核心）
- **新增** `FLAG_ACTIVITY_RETAIN_IN_RECENTS`
- **新增** 唯一 `data` URI 区分不同播放器实例

### Step 4：PlayerActivity.kt — onDestroy 清理卡片

```kotlin
override fun onDestroy() {
    Log.d(TAG, "onDestroy: cleaning up")
    if (backendReceiverRegistered) {
        unregisterReceiver(backendReceiver)
        backendReceiverRegistered = false
    }
    finishAndRemoveTask()
    super.onDestroy()
}
```

---

## 修改文件清单

| # | 文件 | 操作 |
|---|------|------|
| 1 | `.../PlayerActivity.kt` | **删除** `load()` override；`onCreate()` 末尾添加 `navigateToPlayer()`；`onDestroy()` 添加 `finishAndRemoveTask()` |
| 2 | `.../AndroidManifest.xml` | `launchMode`→`standard`；添加 `documentLaunchMode="always"` + `maxRecents="16"` |
| 3 | `.../GoProcessPlugin.kt` | 添加 `FLAG_ACTIVITY_NEW_DOCUMENT` + `RETAIN_IN_RECENTS` + 唯一 data URI |

---

## 白屏诊断检查清单

如果修复后仍有问题，按顺序排查：

1. **logcat `ENCV-go`** — 必须看到 `navigateToPlayer: loadUrl called successfully`
2. **logcat `Console`** — 检查 WebView 中是否有 JS 错误
3. **确认构建产物** — `dist/player.html` 存在且非空，引用的 JS/CSS 路径正确
4. **Chrome DevTools 远程调试** — `chrome://inspect` 连接 WebView 查看 Network 面板确认 `player.html` 是否返回 200 及其依赖资源是否加载成功
