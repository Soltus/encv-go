# 修复导航失效 — 重写 load() 方法

## 问题

PlayerActivity 启动后显示的是文件列表页（`/tabs/files`），不是播放器页面（`#/standalone/player`）。无论内部点击视频还是第三方打开都如此。

## 根因分析

### 当前代码执行时序

```kotlin
// PlayerActivity.kt 当前实现
override fun onCreate(savedInstanceState: Bundle?) {
    registerPlugin(GoProcessPlugin::class.java)
    super.onCreate(savedInstanceState)     // ← 内部执行：
    //   1. setContentView(创建 WebView)
    //   2. this.load() {
    //        bridge = builder.create()   ← 创建 bridge，内部 webView.loadUrl("https://localhost/")
    //        this.onNewIntent(getIntent())
    //      }
    navigateToStandalonePlayer()       // ← 我们在这里调用 webView.loadUrl(playerUrl)
    //   ...后续代码
}
```

**问题**：`navigateToStandalonePlayer()` 放在 `onCreate` 中，时机不对。`super.onCreate()` 内部的 `loadWebView()` 已经调用了 `webView.loadUrl(appUrl)` 发起首次加载。我们在 `onCreate` 中紧接着调第二次 `loadUrl`，但此时：

1. **WebView 可能正在处理首次加载的请求队列**，第二次 `loadUrl` 被排队或覆盖行为不确定
2. **Capacitor 的 BridgeWebViewClient 可能在后续回调中重新导航**到初始 URL（比如处理 intent data 时）
3. **`bridge?.webView?.url` 此时可能为空**（首次 loadUrl 是异步的，URL 属性还没更新），导致 fallback 到硬编码的 `"https://localhost"`，可能与实际 hostname 不匹配

### 为什么 MainActivity 没这个问题？

MainActivity **完全不导航**。它让 WebView 加载默认首页 `/tabs/files`，然后通过 JS 事件通知前端。它不需要改变路由。

## 修复方案：重写 `load()` 方法

BridgeActivity 提供了 `load()` 作为扩展点。它在 `bridge` 创建完成后立即执行，是插入自定义逻辑的**精确时机**。

```kotlin
class PlayerActivity : BridgeActivity() {

    override fun load() {
        super.load()
        // super.load() 完成 = bridge 已创建 + webView 已存在 + 首次 loadUrl 已发起
        // 在此精确时机覆盖 URL 为目标路由
        try {
            val wv = bridge?.webView
            if (wv != null) {
                wv.loadUrl("https://localhost/#/standalone/player")
                Log.i(TAG, "PlayerActivity loading standalone player")
            }
        } catch (e: Exception) {
            Log.e(TAG, "Failed to navigate to player", e)
        }
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        registerPlugin(GoProcessPlugin::class.java)
        super.onCreate(savedInstanceState)     // 内部会调用上面的 load()
        // 不再需要 navigateToStandalonePlayer()
        registerBackendReceiver()
        resolveFileInfo(intent)
        if (EncvGoService.isRunning && EncvGoService.lastKnownPort > 0) {
            notifyFrontend(EncvGoService.lastKnownPort, true, null, "player", null)
        } else {
            startBackendService(EncvGoService.ACTION_START, "player", null)
        }
    }
}
```

**为什么这次能行**：

| 对比项 | 旧方案（onCreate 中 loadUrl） | 新方案（重写 load()） |
|--------|---------------------------|---------------------|
| 执行时机 | `super.onCreate()` **返回后** | `super.load()` **内部末尾** |
| bridge 状态 | ✅ 已创建 | ✅ 已创建 |
| webView 状态 | ⚠️ 首次 loadUrl 已发起但可能还在处理中 | ✅ 刚完成首次 loadUrl 调用 |
| 与 Capacitor 的竞态 | ❌ 我们的 loadUrl 和 Capacitor 内部可能有冲突 | ✅ 在 Capacitor 初始化流程**内部**执行 |
| 代码位置 | onCreate（业务逻辑层） | load（框架扩展点） |

### onNewIntent 中的处理

当 PlayerActivity 已在运行、收到新 intent 时（singleTop 复用实例），也需要导航：

```kotlin
override fun onNewIntent(intent: Intent) {
    super.onNewIntent(intent)
    setIntent(intent)
    resolveFileInfo(intent)
    // 直接 loadUrl，不需要重写 load()
    try {
        bridge?.webView?.loadUrl("https://localhost/#/standalone/player")
    } catch (e: Exception) {
        Log.e(TAG, "Failed to navigate on new intent", e)
    }
}
```

---

## 实施步骤

### Step 1: 修改 PlayerActivity.kt
- [ ] 新增 `override fun load()` 方法：调用 `super.load()` 后立即 `webView.loadUrl("https://localhost/#/standalone/player")`
- [ ] 从 `onCreate` 中删除 `navigateToStandalonePlayer()` 调用
- [ ] 删除整个 `navigateToStandalonePlayer()` 方法
- [ ] 修改 `onNewIntent`：直接内联 `webView.loadUrl()` 调用
- [ ] 清理无用 import（Uri 如果不再需要可删除——实际上 resolveFileInfo 还在用所以保留）

### Step 2: 构建验证
- [ ] `go build ./internal/...` 通过
- [ ] `vue-tsc --noEmit` 通过

### Step 3: 本地合并验证
- [ ] 确认无旧 hack 代码残留
