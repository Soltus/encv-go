# ComboLite 插件 WebView 架构规范

> **核心原则：file:// 协议不能加载可交互网页。用虚拟 https 域名 + shouldInterceptRequest 才是正确模式。**
>
> **ComboLite 插件 = 独立 ClassLoader + 独立 APK + 独立 assets。所有与主 app 的交互必须通过公开 API（Intent / Broadcast / ContentProvider / AIDL），不能直接引用类。**

---

## 一、为什么不能用 file:// 协议

### 1.1 技术问题

| 问题 | 表现 |
|------|------|
| **CORS 拦截** | origin 是 `null`，CSS/JS/字体等子资源加载被视为跨域，全部 blocked |
| **ES Modules 失效** | `<script type="module">` 和动态 import() 报 CORS 错误 |
| **Service Worker 不可用** | file:// 协议下 Service Worker 注册失败 |
| **Fetch API 异常** | 相对路径 fetch 行为不可预测 |
| **Web Crypto API 受限** | `crypto.subtle` 在非安全上下文可能不可用 |

### 1.2 常见的错误 workaround（不要用）

| Workaround | 为什么不好 |
|-----------|-----------|
| `allowFileAccessFromFileURLs = true` | 禁用安全限制，不是解决问题 |
| `allowUniversalAccessFromFileURLs = true` | 同上，且已被 Android 官方废弃 |
| 把所有 JS 内联到 HTML | 无法使用构建工具，不可维护 |
| 用 data: URI | 体积膨胀，调试困难 |

---

## 二、正确方案：虚拟 https 域名 + shouldInterceptRequest

### 2.1 架构图

```
WebView (https://simverse-plugin.local/)
    │
    ├── 页面请求 (HTML/CSS/JS/图片/字体)
    │       ↓ shouldInterceptRequest()
    │   AssetManager.open("simverse/index.html")
    │       ↓ 返回 WebResourceResponse
    └── API 请求 (http://127.0.0.1:2025/api/...)
            ↓ 正常 HTTP 请求（走 CORS）
        Go 后端服务
```

### 2.2 虚拟域名命名规范

格式：`https://{plugin-name}-plugin.local/`

| 插件 | 虚拟域名 | assets 子目录 |
|------|---------|--------------|
| SimVerse | `https://simverse-plugin.local/` | `simverse/` |
| MPV Player | `https://mpv-plugin.local/` | `mpv/` |
| OpenList | `https://openlist-plugin.local/` | `openlist/` |

> **为什么用 .local 后缀？**：mDNS 保留域名，不会与真实互联网域名冲突。

### 2.3 shouldInterceptRequest 实现要点

```kotlin
override fun shouldInterceptRequest(
    view: WebView?,
    request: WebResourceRequest?
): WebResourceResponse? {
    val url = request?.url ?: return null
    if (url.host != virtualHost) return null  // 只拦截虚拟域名

    val path = url.path?.trimStart('/') ?: "index.html"
    val assetPath = "$assetBasePath/$path"

    return try {
        val inputStream = context.assets.open(assetPath)
        WebResourceResponse(getMimeType(assetPath), getEncoding(assetPath), inputStream).apply {
            setStatusCodeAndReasonPhrase(200, "OK")
            responseHeaders = mapOf(
                "Cache-Control" to "no-cache",
                "Access-Control-Allow-Origin" to "*",
            )
        }
    } catch (e: Exception) {
        WebResourceResponse("text/plain", "UTF-8", null).apply {
            setStatusCodeAndReasonPhrase(404, "Not Found")
        }
    }
}
```

**关键细节：**

1. **只拦截虚拟域名**：其他域名（如后端 API）不拦截，正常走网络
2. **MIME type 正确**：CSS 必须是 `text/css`，JS 必须是 `application/javascript`，错了浏览器不执行
3. **编码正确**：文本文件（html/css/js/json/svg）明确声明 UTF-8
4. **404 处理**：assets 里找不到文件返回 404，不要静默失败

---

## 三、后端 API 连通方案

### 3.1 CORS 配置（后端侧）

后端 CORS `AllowOriginFunc` 必须允许插件虚拟域名：

```go
// 允许 ComboLite 插件的虚拟 https 域名
if strings.HasSuffix(origin, "-plugin.local") &&
    strings.HasPrefix(origin, "https://") {
    return true
}
```

> 后缀匹配优于枚举：未来加新插件自动生效，不用改后端。

### 3.2 API Base 注入（WebView 侧）

**双保险注入策略：**

| 注入时机 | 触发方式 | 用途 |
|---------|---------|------|
| **广播触发** | 后端就绪广播 `BACKEND_READY` | 后端晚于 WebView 启动时注入 |
| **onPageFinished** | 页面加载完成 | 后端早于 WebView 启动时注入 |

```kotlin
// 注入脚本模板
val script = """
    (function() {
        window.__ENCV_API_BASE__ = 'http://127.0.0.1:$port';
        window.__ENCV_PORT__ = $port;
        window.dispatchEvent(new CustomEvent('encv:api-base-ready', {detail: {port: $port}}));
    })();
""".trimIndent()

webView.evaluateJavascript(script, null)
```

**前端侧：** 优先读 `window.__ENCV_API_BASE__`，fallback 到 JSInterface，再 fallback 到默认值。

### 3.3 后端启动保障

插件 Activity `onCreate` 时：
1. 探测后端端口（SharedPreferences + 端口扫描 + `/health` 探活）
2. 如果后端没启动，通过 Intent 启动 `EncvGoService`
3. 注册广播接收器监听 `BACKEND_READY`
4. 收到广播后注入 API base

> **ComboLite 特殊性**：不能直接 `EncvGoService::class.java`，必须用字符串类名：
> ```kotlin
> val intent = Intent().apply {
>     setClassName("com.encvgo.app", "com.encvgo.app.EncvGoService")
>     action = "com.encvgo.action.START"
> }
> if (Build.VERSION.SDK_INT >= 26) {
>     startForegroundService(intent)
> } else {
>     startService(intent)
> }
> ```

---

## 四、ComboLite 跨 ClassLoader 交互规范

### 4.1 允许的交互方式

| 方式 | 用途 | 示例 |
|------|------|------|
| **Intent + ComponentName** | 启动 Activity / Service | 启动 EncvGoService |
| **BroadcastReceiver** | 监听全局事件 | 监听后端就绪广播 |
| **ContentProvider** | 共享数据 | 读取 SharedPreferences（走 provider） |
| **AIDL** | 双向 RPC | 复杂插件 API（未来） |

### 4.2 禁止的交互方式

| 禁止 | 原因 |
|------|------|
| 直接 import 主 app 的类 | ClassLoader 不同，ClassNotFoundException |
| 直接访问主 app 的 SharedPreferences | 进程不同，数据不共享 |
| 直接调用主 app 的单例对象 | 进程不同，实例不共享 |
| 反射调用主 app 的 API | 脆弱，混淆后失效 |

### 4.3 上下文获取

在 `BasePluginActivity` / `BasePluginEntry` 中：
- `proxyActivity` = 宿主 Activity 实例（真实的 Context）
- 所有 Android 系统 API 调用都通过 `proxyActivity` 执行
- 不要用 `this`（插件 Activity 是代理，不是真实 Context）

---

## 五、WebView 配置清单

### 5.1 正确配置

```kotlin
settings.apply {
    javaScriptEnabled = true
    domStorageEnabled = true
    databaseEnabled = true
    cacheMode = WebSettings.LOAD_DEFAULT
    useWideViewPort = true
    loadWithOverviewMode = true
    mixedContentMode = WebSettings.MIXED_CONTENT_ALWAYS_ALLOW
    mediaPlaybackRequiresUserGesture = false  // 视频类插件需要

    // 不需要的（虚拟域名方案下）:
    // allowFileAccess = false
    // allowContentAccess = false
    // allowFileAccessFromFileURLs = false
    // allowUniversalAccessFromFileURLs = false
}
```

### 5.2 混合内容说明

页面是 `https://`，API 是 `http://127.0.0.1`，属于混合内容。必须：
- `mixedContentMode = MIXED_CONTENT_ALWAYS_ALLOW`
- 后端 CORS 允许 `https://*-plugin.local` origin

---

## 六、与 Capacitor WebView 的对比

| 维度 | Capacitor WebView | ComboLite 插件 WebView |
|------|-------------------|----------------------|
| **宿主** | 主 app | 插件 APK |
| **资源加载** | Capacitor LocalServer (`https://localhost/`) | 自定义 `shouldInterceptRequest` |
| **后端 API** | 同进程，直接访问 | 跨进程，走 HTTP |
| **JS Bridge** | Capacitor Bridge | 自定义 JavascriptInterface |
| **生命周期** | 主 app 生命周期 | 插件 Activity 生命周期 |
| **ClassLoader** | 主 app ClassLoader | 插件独立 ClassLoader |
| **CORS** | 同域（localhost → localhost） | 跨域（plugin.local → 127.0.0.1） |

---

## 七、OpenList 插件的特殊情况

OpenList 插件有自己的 Go 后端进程（独立端口 5244），前端直接从 Go 服务加载（`http://127.0.0.1:5244/`），不走 assets。

这种模式下：
- 不需要虚拟域名 + shouldInterceptRequest
- 不存在 CORS 问题（同域）
- 但需要确保 OpenList 后端进程已启动

> OpenList 是"插件自带后端"的模式，SimVerse/MPV 是"复用主 app 后端"的模式，两者架构不同。

---

## 八、饱和调试建议

插件 WebView 白屏/功能异常时，按以下层级排查：

| 层级 | 检查项 | 诊断方法 |
|------|--------|---------|
| L1 | 插件是否安装 | ComboLite 插件列表 |
| L2 | 插件是否加载 | PluginManager 状态 |
| L3 | Activity 能否启动 | createProxyIntent 测试 |
| L4 | WebView 能否加载页面 | `onPageStarted` / `onPageFinished` 计数 + URL |
| L5 | 资源加载是否正常 | `onReceivedError` 统计 + `shouldInterceptRequest` 日志 |
| L6 | 控制台错误 | `onConsoleMessage` 捕获最近 20 条 |
| L7 | 后端是否运行 | `/health` 探活 + 端口扫描 |
| L8 | API 能否访问 | CORS preflight 检查 + 实际请求测试 |

> 调试工具：悬浮诊断按钮（半透明 🔍），点击弹出全链路诊断报告。

---

## 九、相关文件索引

| 文件 | 作用 |
|------|------|
| `plugin-simverse/src/main/java/.../SimVerseWebViewClient.kt` | 虚拟域名 + shouldInterceptRequest 参考实现 |
| `plugin-simverse/src/main/java/.../SimVerseActivity.kt` | 后端启动 + 广播 + API 注入 参考实现 |
| `plugin-mpv-player/src/main/java/.../MpvWebViewClient.kt` | MPV 版本实现 |
| `internal/server/gin_app.go` | 后端 CORS 配置 |
| `docs/saturation-debugging-playbook.md` | 饱和调试方法论 |
