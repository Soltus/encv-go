# PlayerActivity 白屏诊断与修复 Plan

## 当前状态分析

从 logcat 日志确认：

```
✅ onCreate: super done, bridge=true, webView=true     ← 原生层全部就绪
✅ navigateToPlayer: https://localhost/player.html       ← URL 正确
✅ navigateToPlayer: loadUrl dispatched                  ← loadUrl 已调用
✅ handleBackend: already running port=2025              ← 后端正常
❌ 前端白屏，无任何有效日志                              ← 黑洞
```

**结论：原生层没问题，问题在 WebView 内容渲染链路中某个环节静默失败了，但没有任何日志输出告诉我们具体是哪个环节。**

---

## 诊断方案：三层拦截

### 第 1 层：Native — WebViewClient 回调（最关键）

当前 PlayerActivity 完全依赖 Capacitor 默认的 `BridgeWebViewClient`，它的 `onPageStarted`/`onPageFinished`/`onReceivedError` 回调虽然有日志，但**不会打印到我们可控的 logcat tag 下**，而且不包含足够细节。

**方案**：在 `navigateToPlayer()` 之后，给 WebView 设置一个自定义 `WebViewClient` 包装器，在关键回调中输出详细日志：

```kotlin
private fun navigateToPlayer() {
    try {
        val playerUrl = "https://localhost/player.html"
        Log.i(TAG, "navigateToPlayer: $playerUrl")
        
        val originalClient = bridge?.webView?.webViewClient
        bridge?.webView?.webViewClient = object : WebViewClient() {
            override fun onPageStarted(view: WebView?, url: String?, favicon: Bitmap?) {
                Log.i(TAG, "WebViewClient.onPageStarted: url=$url")
                originalClient?.onPageStarted(view, url, favicon)
            }
            override fun onPageFinished(view: WebView?, url: String?) {
                Log.i(TAG, "WebViewClient.onPageFinished: url=$url")
                originalClient?.onPageFinished(view, url, favicon)
            }
            override fun onReceivedError(view: WebView?, request: WebResourceRequest?, error: WebResourceError?) {
                Log.e(TAG, "WebViewClient.onReceivedError: url=${request?.url}, error=${error?.description} (${error?.errorCode})")
                originalClient?.onReceivedError(view, request, error)
            }
            override fun onReceivedHttpError(view: WebView?, request: WebResourceRequest?, response: WebResourceResponse?) {
                Log.e(TAG, "WebViewClient.onReceivedHttpError: url=${request?.url}, status=${response?.statusCode}")
                originalClient?.onReceivedHttpError(view, request, response)
            }
            override fun shouldInterceptRequest(view: WebResourceRequest?): WebResourceResponse? {
                val url = request?.url.toString()
                if (url.endsWith(".html") || url.endsWith(".js") || url.endsWith(".css")) {
                    Log.d(TAG, "shouldInterceptRequest: $url")
                }
                return originalClient?.shouldInterceptRequest(request)
                    ?: (bridge as? com.getcapacitor.Bridge)?.let { b ->
                        b.getLocalServer().shouldInterceptRequest(request)
                    }
            }
        }
        
        bridge?.webView?.loadUrl(playerUrl)
        Log.i(TAG, "navigateToPlayer: loadUrl dispatched with diagnostic WebViewClient")
    } catch (e: Exception) {
        Log.e(TAG, "navigateToPlayer: failed", e)
    }
}
```

**注意**：这里包装而不是替换，确保 Capacitor 的 `BridgeWebViewClient.shouldInterceptRequest`（LocalServer 文件提供）仍然被执行。

### 第 2 层：JS — 全局错误捕获

在 [player-main.ts](src/player-main.ts) 最前面添加错误监听，通过 `console.log` 输出（会被 Logcat 库捕获）：

```typescript
// player-main.ts 顶部添加
window.addEventListener('error', (e) => {
  console.error('[PLAYER-ERROR]', e.message, e.filename, e.lineno, e.colno, e.error)
})
window.addEventListener('unhandledrejection', (e) => {
  console.error('[PLAYER-PROMISE-REJECT]', e.reason)
})
console.log('[PLAYER-INIT] player-main.ts starting to evaluate')
```

### 第 3 层：验证构建产物

确认 CI 构建流程中 `dist/player.html` 是否存在并被复制到 Android assets：

1. 检查 `npx cap copy` 或 `npx cap sync` 是否在 `vite build` 之后执行
2. 确认最终 APK 的 `assets/public/` 目录下包含 `player.html`

---

## 可能的根因与对应修复

### 根因 A：LocalServer 不提供 player.html（404 白屏）

**症状**：第 1 层诊断会看到 `onReceivedHttpError: status=404` 或 `shouldInterceptRequest` 对 `/player.html` 返回 null

**原因**：Capacitor 的 `npx cap copy` 只复制 `webDir`（即 `dist/`）内容到 `assets/public/`。如果 CI 流程中 `vite build` 后没有执行 `cap copy/sync`，或者 `dist/player.html` 不存在，则 LocalServer 无法提供该文件。

**验证**：
```bash
# 在 CI 或本地检查
ls -la android/app/src/main/assets/public/player.html
# 或解压 APK 检查
unzip -l app-release.apk | grep player.html
```

**修复**：确保构建脚本按顺序执行：
```bash
npm run build          # vite build → dist/
npx cap copy           # dist/ → android/app/src/main/assets/public/
# 然后 post-cap-sync.mjs
```

### 根因 B：JS 模块加载路径错误

**症状**：第 1 层看到 `onPageFinished: player.html` 成功，但第 2 层看到 `[PLAYER-ERROR] Failed to fetch dynamically imported module`

**原因**：Vite 多入口构建时，`player.html` 的 `<script type="module" src="/src/player-main.ts">` 被 Vite 替换为实际的 chunk URL（如 `/assets/player-xxx.js`）。如果这个 chunk 文件不存在或路径在 Android LocalServer 中无法解析，则白屏。

**修复**：检查 Vite 构建输出确认所有引用的 asset 存在。

### 根因 C：Capacitor Bridge JS 未注入到 player.html

**症状**：页面部分渲染（能看到 HTML 骨架），但 Capacitor 插件调用全部失败

**原因**：Capacitor 的 JSInjector 通过 `DocumentStartScript` 注入 capacitor runtime。当从 index.html 切换到 player.html 时，新页面应该也会被注入。但如果 `WebViewCompat.addDocumentStartJavaScript` 的 allowedOrigin 不匹配...

**不太可能导致完全白屏**，更可能导致插件不可用。

### 根因 D：Ionic Vue 双实例冲突

**症状**：第 2 层看到 Ionic 相关错误

**原因**：如果 MainActivity 和 PlayerActivity 共享同一个 WebView 进程（默认行为），两个 Ionic App 实例可能会冲突（DOM 事件监听器、CSS 变量等）。

**修复**：暂时排除，先通过诊断定位是否真的走到了 JS 执行阶段。

---

## 实施步骤

### Step 1：PlayerActivity.kt — 添加诊断 WebViewClient

修改 `navigateToPlayer()` 方法，添加 WebViewClient 包装器（代码见上方"第 1 层"）

### Step 2：player-main.ts — 添加全局错误监听

文件顶部添加 `window.addEventListener('error')` 和 `console.log('[PLAYER-INIT]')`

### Step 3：验证构建链路

确认 CI/CD 中 `vite build` → `cap copy` → `post-cap-sync.mjs` 的执行顺序

---

## 修改文件清单

| # | 文件 | 操作 |
|---|------|------|
| 1 | `android-overlay/.../PlayerActivity.kt` | `navigateToPlayer()` 添加诊断 WebViewClient 包装 |
| 2 | `src/player-main.ts` | 顶部添加全局错误监听和初始化日志 |

**本次修改的目标不是直接修复白屏，而是让下一次构建运行后能在 logcat 中看到到底哪一步失败了。**
