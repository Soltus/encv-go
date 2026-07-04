# 播放器升级为独立 Activity — 根因修复计划

## 问题诊断

### 用户反馈
"播放还是跳转到了MainActivity" — 点击视频后没有进入独立播放器，仍然在主界面。

### 根因分析（三层问题）

#### 第一层：`navigateToPlayer()` 时序竞态（核心 Bug）

[PlayerActivity.kt L211-L219](file:///workspace/app/encv-mobile/android-overlay/app/src/main/java/com/encvgo/app/PlayerActivity.kt#L211-L219) 中使用 `postDelayed(500ms)` + `window.location.hash='#/standalone/player'` 导航：

```
PlayerActivity.onCreate()
  ├─ registerPlugin(GoProcessPlugin::class.java)
  ├─ super.onCreate(savedInstanceState)  ← BridgeActivity 开始异步加载 WebView（加载整个 SPA）
  ├─ registerBackendReceiver()
  ├─ resolveFileInfo(intent)
  ├─ checkBackendStatus()...
  └─ postDelayed(500ms) { navigateToPlayer() }  ← ❌ 500ms 是猜的，WebView 可能还没开始加载！
```

**问题链**：
1. `BridgeActivity.super.onCreate()` 触发 WebView 加载，但这是**异步的**
2. `postDelayed(500ms)` 在 `onCreate` 阶段就注册了，此时 WebView 可能：
   - 还没创建完成
   - 正在加载 HTML
   - SPA 正在初始化，Vue Router 还没 ready
3. 即使 500ms 后执行了 `window.location.hash='#/standalone/player'`：
   - 如果 Vue Router 没初始化完，hash 变更会被忽略或覆盖
   - 用户会先看到 `/tabs/files` 页面闪一下，再跳转到播放器
   - 在慢设备上 500ms 完全不够，hash 设置太早被 Vue Router 初始导航覆盖

#### 第二层：缺少 `taskAffinity` 配置

[AndroidManifest.xml](file:///workspace/app/encv-mobile/android-overlay/app/src/main/AndroidManifest.xml#L64) 的 PlayerActivity 没有设置独立的 `taskAffinity`。

Sillot-KMP 的做法（参考 [MainPro](file:///workspace/Sillot-KMP/androidApp/src/main/AndroidManifest.xml#L291)）：
- MainPro: `android:taskAffinity="sc.hwd.sillot.kmp.producer.MainPro.task"`
- AppActivity: `android:taskAffinity="sc.hwd.sillot.kmp.sillot.AppActivity"`

每个 Activity 有独立 taskAffinity，确保运行在**独立任务栈**中。我们的 PlayerActivity 继承默认 affinity，与 MainActivity 共享同一任务栈，可能导致 Android 任务管理异常。

#### 第三层：post-cap-sync 脚本已修复但需验证

[post-cap-sync.mjs](file:///workspace/app/encv-mobile/scripts/post-cap-sync.mjs#L259) 已添加 `PlayerActivity.kt` 到复制列表（上一轮修复），合并逻辑正确。

---

## 从 Sillot-KMP 学到的关键模式

### 1. Trampoline Launcher 模式
[AppActivity](file:///workspace/Sillot-KMP/androidApp/src/main/java/sc/hwd/sillot/kmp/sillot/AppActivity.kt#L49-L53):
```kotlin
private fun init(in2intent: Intent?) {
    app.startTargetActivity()  // 启动真正的目标 Activity
    finishAfterTransition()    // 自身立即销毁，不留在任务栈中
}
```
AppActivity 是纯入口跳板，`noHistory=true`，启动目标后立刻 finish。

### 2. 独立 taskAffinity
每个 Activity 都有唯一的 `taskAffinity` + `launchMode="singleInstancePerTask"`，确保独立任务栈。

### 3. 不依赖 Capacitor BridgeActivity
Sillot-KMP 使用原生 `ComponentActivity` + Compose UI + 自定义 WebView（`WebPoolsPro`），完全控制生命周期。
我们仍用 Capacitor BridgeActivity（需要插件能力），但需要正确处理其 WebView 加载时序。

---

## 修复方案

### Fix 1: 用 WebViewClient.onPageFinished 替代 postDelayed（必须）

**原理**：给 PlayerActivity 的 WebView 设置自定义 `WebViewClient`，在 `onPageFinished` 回调中确认页面加载完成后才执行路由导航。这是事件驱动的，不依赖固定延时。

**修改文件**: [PlayerActivity.kt](file:///workspace/app/encv-mobile/android-overlay/app/src/main/java/com/encvgo/app/PlayerActivity.kt)

```kotlin
// 替换 navigateToPlayer() 方法
private fun setupWebViewNavigation() {
    val originalClient = bridge.webView.webViewClient
    bridge.webView.webViewClient = object : WebViewClient() {
        override fun onPageFinished(view: WebView?, url: String?) {
            super.onPageFinished(view, url)
            if (!navigatedToPlayer) {
                navigatedToPlayer = true
                runOnUiThread {
                    bridge.webView.evaluateJavascript(
                        "window.location.hash='#/standalone/player'", null
                    )
                    Log.i(TAG, "Navigated to #/standalone/player")
                }
            }
        }
    }
}

private var navigatedToPlayer = false
```

关键点：
- 在 `super.onCreate()` **之后**、`resolveFileInfo()` **之前**调用 `setupWebViewNavigation()`
- `onPageFinished` 保证 HTML+JS 已全部加载完毕，Vue Router 已初始化
- `navigatedToPlayer` 标志防止重复导航（`onPageFinished` 可能触发多次）

### Fix 2: 给 PlayerActivity 添加独立 taskAffinity（必须）

**修改文件**: [AndroidManifest.xml](file:///workspace/app/encv-mobile/android-overlay/app/src/main/AndroidManifest.xml)

```xml
<activity
    android:name=".PlayerActivity"
    android:exported="true"
    android:launchMode="singleTop"
    android:taskAffinity="com.encvgo.app.player.task"
    ...>
```

确保 PlayerActivity 运行在独立任务栈中，返回键行为正确（finish 回到 MainActivity）。

### Fix 3: 增加超时保护（增强健壮性）

如果 `onPageFinished` 因某些原因未触发（如网络错误），增加一个兜底超时：

```kotlin
private fun setupNavigationTimeout() {
    Handler(Looper.getMainLooper()).postDelayed({
        if (!navigatedToPlayer && !isFinishing && !isDestroyed) {
            Log.w(TAG, "onPageFinished timeout, forcing navigation")
            navigatedToPlayer = true
            try {
                bridge.webView.evaluateJavascript(
                    "window.location.hash='#/standalone/player'", null
                )
            } catch (e: Exception) {
                Log.e(TAG, "Force navigation failed", e)
            }
        }
    }, 10000) // 10秒超时保护
}
```

---

## 实施步骤

### Step 1: 修改 PlayerActivity.kt — 重写导航机制
- [ ] 新增 `navigatedToPlayer: Boolean` 成员变量
- [ ] 将 `navigateToPlayer()` 改为 `setupWebViewNavigation()`，基于 `WebViewClient.onPageFinished`
- [ ] 新增 `setupNavigationTimeout()` 兜底超时
- [ ] 在 `onCreate` 中调整调用顺序：registerPlugin → super.onCreate → **setupWebViewNavigation** → setupNavigationTimeout → registerReceiver → resolveFileInfo → checkBackend
- [ ] 移除旧的 `postDelayed(500ms)` 导航方式

### Step 2: 修改 AndroidManifest.xml — 添加 taskAffinity
- [ ] PlayerActivity 添加 `android:taskAffinity="com.encvgo.app.player.task"`
- [ ] 确认 launchMode 为 `singleTop`
- [ ] 确认 intent-filter 配置不变

### Step 3: 本地模拟合并验证
- [ ] 手动运行 `post-cap-sync.mjs` 逻辑模拟，确认 4 个 kt 文件都会被正确复制
- [ ] 验证合并后的 AndroidManifest.xml 包含 PlayerActivity 声明和所有 intent-filter
- [ ] 验证 GoProcessPlugin.kt 中对 PlayerActivity 的引用可解析

### Step 4: 构建验证
- [ ] `cd /workspace/app/encv-mobile && npx vue-tsc --noEmit` 通过
- [ ] `cd /workspace && go build ./internal/...` 通过

### Step 5: 更新 .gitignore（参考项目要求）
- [ ] 确保 Sillot-KMP 的 `.gitignore` 模式不影响本项目
- [ ] 本项目已有 `.gitignore`，检查是否需要补充

---

## 风险评估

| 风险 | 概率 | 影响 | 缓解 |
|------|------|------|------|
| onPageFinished 在子资源加载时多次触发 | 高 | 低 | navigatedToPlayer 标志去重 |
| BridgeActivity 的 WebView 可能在 onCreate 时还未创建 | 中 | 中 | 在 super.onCreate() 之后才设置 WebViewClient |
| Capacitor 内部可能有自己的 WebViewClient 逻辑 | 中 | 中 | 包装原始 WebViewClient 并调用 super |
