# 修复 chrome-error 导航错误计划 v3 — 官方方式

## 错误现象

```
Not allowed to load local resource: chrome-error://chromewebdata/#/standalone-player
```

## 根因（最终确认）

### Capacitor BridgeActivity 加载流程

```
BridgeActivity.onCreate()
  └─ super.onCreate(savedInstanceState)
      └─ setContentView(R.layout.capacitor_bridge_layout_main)  ← 创建 WebView
      └─ this.load()
          └─ bridge = bridgeBuilder.create()
              ├─ new Bridge(context, webView, ...)
              │   ├─ initWebView()
              │   ├─ init LocalServer (hostAssets "public"/"dist")
              │   └─ loadWebView()
              │       └─ webView.loadUrl(appUrl)    ← appUrl = "https://localhost/"
              └─ this.onNewIntent(getIntent())
```

**关键发现**：`bridge.create()` 内部会调用 `webView.loadUrl(appUrl)` 来加载 SPA。`appUrl` 由 Capacitor 配置生成（`androidScheme: 'https'` + hostname）。

### 为什么会出现 chrome-error？

上一版代码**替换了 WebViewClient**（`setupWebViewNavigation()`），破坏了 Capacitor 的 `BridgeWebViewClient`（负责 SSL 处理、URL 拦截、localServer 请求代理），导致 WebView 无法正确加载本地 SPA 资源 → 显示 `chrome-error://chromewebdata/` → 我们的 JS 在错误页上执行 hash 导航 → 报错。

### 正确做法：利用 Bridge API 直接控制加载 URL

**不需要** WebViewClient 包装，**不需要** 轮询。Bridge 创建后 WebView 就已就绪，可以直接调用 `loadUrl`：

```kotlin
override fun onCreate(savedInstanceState: Bundle?) {
    registerPlugin(GoProcessPlugin::class.java)
    super.onCreate(savedInstanceState)     // ← 这里内部完成: bridge = bridgeBuilder.create() + webView.loadUrl(appUrl)
    // 此时 bridge != null, webView != null, SPA 已开始加载
    // 直接告诉 WebView 加载带 hash 的 URL
    bridge?.webView?.loadUrl("${getAppUrl()}#/standalone/player")
}
```

**这就是官方方式**：BridgeActivity 暴露了 `bridge` 对象和 `webView`，`super.onCreate()` 完成后它们就已初始化完毕，可以直接调用 `loadUrl` 覆盖默认的首页 URL。

---

## 修复方案

### PlayerActivity.kt 改动

```kotlin
class PlayerActivity : BridgeActivity() {
    private var navigatedToPlayer = false

    override fun onCreate(savedInstanceState: Bundle?) {
        registerPlugin(GoProcessPlugin::class.java)
        super.onCreate(savedInstanceState)
        // super.onCreate() 完成 = bridge 已创建 + WebView 已加载 appUrl
        // 此时直接覆盖 URL 为目标路由，不需要 WebViewClient / 轮询 / 延时
        navigateToStandalonePlayer()
        registerBackendReceiver()
        resolveFileInfo(intent)
        if (EncvGoService.isRunning && EncvGoService.lastKnownPort > 0) {
            notifyFrontend(EncvGoService.lastKnownPort, true, null, "player", null)
        } else {
            startBackendService(EncvGoService.ACTION_START, "player", null)
        }
    }

    private fun navigateToStandalonePlayer() {
        try {
            val url = bridge?.webView?.url ?: ""
            val baseUrl = when {
                url.startsWith("http") -> Uri.parse(url).buildUpon().path(null).fragment(null).clearQuery().build().toString()
                else -> getAppUrl()
            }
            val targetUrl = "$baseUrl#/standalone/player"
            Log.i(TAG, "Loading player URL: $targetUrl")
            bridge?.webView?.loadUrl(targetUrl)
            navigatedToPlayer = true
        } catch (e: Exception) {
            Log.e(TAG, "Failed to navigate to standalone player", e)
        }
    }

    override fun onNewIntent(intent: Intent) {
        super.onNewIntent(intent)
        setIntent(intent)
        resolveFileInfo(intent)
        // 新 intent 时重新导航
        navigateToStandalonePlayer()
    }
    
    // ... 其余方法保持不变（resolveFileInfo, notifyFrontend 等）
    // 删除: setupWebViewNavigation(), setupNavigationTimeout(), forceNavigateToPlayer()
    // 删除 import: android.webkit.WebViewClient, Handler, Looper
    // 删除成员变量: navigationHandler
}
```

### 删除的代码

| 删除项 | 原因 |
|--------|------|
| `setupWebViewNavigation()` | 替换 WebViewClient 破坏 Capacitor |
| `setupNavigationTimeout()` | 不再需要超时兜底 |
| `forceNavigateToPlayer()` | 被 `navigateToStandalonePlayer()` 替代 |
| `navigationHandler` 成员变量 | 不再需要 |
| `import android.webkit.WebViewClient` | 不再包装 WebViewClient |
| `import android.os.Handler` | 不再需要延时/轮询 |
| `import android.os.Looper` | 同上 |

---

## 实施步骤

### Step 1: 重写 PlayerActivity.kt
- [ ] 删除 `setupWebViewNavigation()` 方法
- [ ] 删除 `setupNavigationTimeout()` 方法
- [ ] 删除 `forceNavigateToPlayer()` 方法
- [ ] 删除 `navigatedToPlayer` 改为简单的 boolean flag（不再需要复杂的状态机）
- [ ] 新增 `navigateToStandalonePlayer()`：读取当前 webView URL 提取 base，拼接 `#/standalone/player`，调用 `webView.loadUrl()`
- [ ] 修改 `onCreate`：`super.onCreate()` 后直接调用 `navigateToStandalonePlayer()`
- [ ] 修改 `onNewIntent`：改为调用 `navigateToStandalonePlayer()`
- [ ] 清理无用 import
- [ ] 保留所有其他功能不变（后端交互、intent 解析、broadcast receiver）

### Step 2: 构建验证
- [ ] `go build ./internal/...` 通过
- [ ] `vue-tsc --noEmit` 通过

### Step 3: 本地合并模拟验证
- [ ] 确认无 WebViewClient 相关代码
- [ ] 确认无 postDelayed / Handler / 轮询相关代码
- [ ] 确认 post-cap-sync 文件列表包含 PlayerActivity.kt
