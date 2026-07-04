# PlayerActivity 独立窗口 + 白屏修复 Plan

## 问题诊断

### 问题 1：播放器依附于当前窗口，无独立任务栈

**根因**：[GoProcessPlugin.kt](android-overlay/app/src/main/java/com/encvgo/app/GoProcessPlugin.kt#L150-L170) 中 `openInPlayer()` 启动 Intent 时**没有设置任何 Activity Flag**。

```kotlin
// 当前代码（问题）
val intent = Intent(activity, PlayerActivity::class.java).apply {
    putExtra("file_path", path)
    putExtra("file_name", name)
    putExtra("file_mime_type", mimeType)
}
activity.startActivity(intent)  // ← 无 FLAG，默认在同一个 task 栈中启动
```

虽然 [AndroidManifest.xml](android-overlay/app/src/main/AndroidManifest.xml#L64-L100) 声明了 `taskAffinity="com.encvgo.app.player.task"` 和 `launchMode="singleTop"`，但 **taskAffinity 仅在配合 `FLAG_ACTIVITY_NEW_TASK` 时才生效**。从同一应用内部 `startActivity()` 不加 flag 时，系统默认将新 Activity 放入调用者所在的 task 栈。

**参考证据**：Sillot-KMP 项目中 [`addFlagsForMatrixModel()`](Sillot-KMP/androidSofill/src/main/java/sc/hwd/sofill/Us/U_Uri.kt#L97-L106) 使用三个 flag 实现独立窗口：
- `FLAG_ACTIVITY_NEW_TASK` — 创建新任务栈
- `FLAG_ACTIVITY_NEW_DOCUMENT` — 每次启动新建文档记录（独立最近任务条目）
- `FLAG_ACTIVITY_MULTIPLE_TASK` — 允许同 affinity 多个 task 实例

### 问题 2：进入后白屏

**根因**：[PlayerActivity.kt](android-overlay/app/src/main/java/com/encvgo/app/PlayerActivity.kt#L47-L55) 的 `load()` 方法实现有误：

```kotlin
override fun load() {
    super.load()                                              // ① 先加载 index.html（默认）
    bridge?.webView?.loadUrl("https://localhost/player.html")  // ② 再导航到 player.html
}
```

**问题链路分析**：
1. `BridgeActivity.onCreate()` 内部创建 bridge 后调用 `this.load()`（即我们的 override）
2. `super.load()` 执行 → 调用 `bridge.getWebView().loadUrl(默认URL)` 加载 `index.html`
3. 紧接着执行 `bridge?.webView?.loadUrl("https://localhost/player.html")`
4. **问题 A**：步骤②中 `bridge?.webView` 可能为 null（安全调用静默失败）
5. **问题 B**：即使非 null，`loadUrl` 在 `super.load()` 之后立即调用可能与 WebView 初始化时序冲突
6. **问题 C**：`super.load()` 已触发 index.html 的加载和 Capacitor bridge JS 初始化，随后立即导航走会导致 bridge 状态异常

**正确做法**：不调用 `super.load()`，直接加载目标 URL。Capacitor `BridgeActivity.load()` 的唯一职责就是 `bridge.getWebView().loadUrl(appUrl)`，我们可以完全替换它。

---

## 修复方案

### Step 1：修复 `openInPlayer()` Intent Flags（独立窗口）

**文件**：`android-overlay/app/src/main/java/com/encvgo/app/GoProcessPlugin.kt`

在 `openInPlayer()` 方法的 Intent 上添加独立任务栈 flags：

```kotlin
@PluginMethod
fun openInPlayer(call: PluginCall) {
    val path = call.getString("path", "")
    val name = call.getString("name", "")
    val mimeType = call.getString("mimeType", "")
    if (path.isNullOrEmpty()) {
        call.reject("path is required")
        return
    }
    try {
        val intent = Intent(activity, PlayerActivity::class.java).apply {
            addFlags(Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_ACTIVITY_MULTIPLE_TASK)
            putExtra("file_path", path)
            putExtra("file_name", name)
            putExtra("file_mime_type", mimeType)
        }
        activity.startActivity(intent)
        call.resolve()
    } catch (e: Exception) {
        call.reject("Failed to open player: ${e.message}")
    }
}
```

关键变更：
- 添加 `Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_ACTIVITY_MULTIPLE_TASK`
- 配合 manifest 中已有的 `taskAffinity="com.encvgo.app.player.task"`，确保 PlayerActivity 运行在独立任务栈
- 不使用 `FLAG_ACTIVITY_NEW_DOCUMENT`（那是文档类应用用的，播放器场景不需要）

### Step 2：修改 launchMode 为 singleTask

**文件**：`android-overlay/app/src/main/AndroidManifest.xml`

将 PlayerActivity 的 `launchMode` 从 `singleTop` 改为 `singleTask`：

```xml
<activity
    android:name=".PlayerActivity"
    android:exported="true"
    android:launchMode="singleTask"
    android:taskAffinity="com.encvgo.app.player.task"
    ...>
```

原因：`singleTop` 只处理栈顶复用，不保证独立 task。`singleTask` 配合 `taskAffinity` 确保：
- 如果 player task 不存在 → 创建新 task 并启动 PlayerActivity
- 如果 player task 已存在 → 将该 task 带到前台并 `onNewIntent()`

### Step 3：修复 `load()` 白屏问题

**文件**：`android-overlay/app/src/main/java/com/encvgo/app/PlayerActivity.kt`

重写 `load()` 方法，不再调用 `super.load()`，直接加载 player.html：

```kotlin
override fun load() {
    try {
        val url = bridge.getLocalUrl()
        Log.i(TAG, "PlayerActivity base URL: $url")
        val playerUrl = "$url/player.html"
        Log.i(TAG, "PlayerActivity loading: $playerUrl")
        bridge.getWebView().loadUrl(playerUrl)
    } catch (e: Exception) {
        Log.e(TAG, "Failed to load player app", e)
    }
}
```

关键变更：
- 使用 `bridge.getLocalUrl()` 获取 Capacitor LocalServer 的基础 URL（如 `https://localhost/`），而不是硬编码
- 直接调用 `bridge.getWebView().loadUrl()` （非空断言，因为此时 bridge 必定已初始化）
- 不调用 `super.load()`，避免先加载 index.html 再跳转的竞态条件
- 增加 URL 日志输出便于调试验证

> **注意**：需要确认 `getLocalUrl()` 在 Capacitor 8.3.4 中的方法名。如果不存在，备选方案为：
> ```kotlin
> val url = bridge.getConfig().getString("url") ?: "https://localhost/"
> ```

### Step 4：验证 Vite 构建产物包含 player.html

确保 `vite build` 后 `dist/` 目录下同时存在：
- `dist/index.html` — 主应用入口
- `dist/player.html` — 播放器入口
- `dist/assets/xxx.js` — 共享 chunk + 各入口独立 chunk

当前 [vite.config.ts](vite.config.ts#L13-L19) 的多入口配置已正确设置，无需修改。但需确认 CI 构建流程确实产出了 player.html。

---

## 修改文件清单

| # | 文件 | 修改类型 | 说明 |
|---|------|---------|------|
| 1 | `android-overlay/.../GoProcessPlugin.kt` | 修改 | `openInPlayer()` 添加 `FLAG_ACTIVITY_NEW_TASK \| FLAG_ACTIVITY_MULTIPLE_TASK` |
| 2 | `android-overlay/.../AndroidManifest.xml` | 修改 | PlayerActivity `launchMode`: `singleTop` → `singleTask` |
| 3 | `android-overlay/.../PlayerActivity.kt` | 修改 | `load()` 不调用 `super.load()`，直接通过 `bridge.getLocalUrl()` + `/player.html` 加载 |

---

## 验证方式

1. **独立窗口测试**：从主应用 Files 页面点击视频 → 应弹出独立窗口（最近任务列表中有单独条目）→ 切回主应用 → PlayerActivity 应仍在后台独立运行
2. **第三方打开测试**：从文件管理器分享视频到 ENCV Player → 应以独立窗口打开
3. **白屏修复测试**：进入 PlayerActivity → 应显示播放器 UI（标题栏 + 播放器区域或 loading 状态）
4. **logcat 验证**：搜索 `ENCV-go` tag，应看到 `PlayerActivity base URL:` 和 `PlayerActivity loading:` 日志
