# 修复 MPV 卡住无响应 + 更新 Combolite 规则

## Bug 诊断

### 现象

三种 MPV 打开方式（Activity/Fragment/Compose）均**卡住无响应也不闪退**：
- 用户点击视频播放
- 界面完全无变化（不白屏、不闪退、不报错）
- 触摸事件可能被拦截（无法操作）

### 根因分析

**调用链**：
```
Files.vue → openPlayer(path, name, mime, 'mpv-activity')
  → GoProcessPlugin.openPlayer()
    → PlayerEntry.buildMpvIntent()  ← 前置检查全部通过 ✅
    → activity.startActivityForResult(intent, REQUEST_CODE_MPV_PLAYER)  ← 发出 Intent
      → EncvHostActivity.onCreate()
        → super.onCreate()  ← BaseHostActivity (ComboLite)
          → ProxyManager 代理启动 MpvPlayerActivity
            → ??? （此处行为未知）
```

**三种可能的卡住原因**（按概率排序）：

#### 假设 A（最高概率）：透明 Activity + BaseHostActivity 空白内容

```
EncvHostActivity 主题 = Theme.Translucent.NoTitleBar（透明）
  ↓
BaseHostActivity.onCreate() 执行完成（proxyStarted = true）✅
  ↓
但 ProxyManager 代理启动 MpvPlayerActivity 失败或目标 Activity 未显示
  ↓
BaseHostActivity 显示空白布局（透明 → 用户看不到任何东西）
  ↓
Activity 仍在栈顶，拦截触摸事件 → "卡住"
```

**关键证据**：
- `startMpvViaActivity()` 所有前置检查通过才到 `startActivity()`
- `EncvHostActivity.onResume()` 只检查 `!proxyStarted` — 如果 `super.onCreate()` 成功则跳过
- **没有任何超时或失败兜底机制**

#### 假设 B：`super.onCreate()` 阻塞主线程

`BaseHostActivity.onCreate()` 内部可能在主线程执行：
- ClassLoader 加载插件 DEX
- 解析插件 AndroidManifest
- 初始化 PluginContext

如果这些操作耗时或死锁 → 主线程冻结 → "卡住"

#### 假设 C：`startActivityResult` 的 pending call 永远 pending

GoProcessPlugin 中 `pendingCalls["mpvPlayer"]` 存储了 call 并 `call.save()`。
如果 EncvHostActivity 从未调用 `finish()`（因为没进入 `onResume` 的失败分支也没进入 `onDestroy`）→ Promise 永远不 resolve → 前端 `await openPlayer()` 永远挂起。

但这只影响回调返回，不应影响"界面卡住"。

---

## 修复方案

### Task 1: EncvHostActivity 超时 + 可见性诊断（解决假设 A+C）

#### SubTask 1.1: 改用半透明主题（非全透明）

```xml
<!-- Before: 完全透明，用户看不到任何东西 -->
android:theme="@android:style/Theme.Translucent.NoTitleBar"

<!-- After: 半透明深色背景，用户能看到 Activity 存在 -->
android:theme="@style/Theme.EncvHostTranslucent"
```

新建 `res/values/styles.xml`:
```xml
<style name="Theme.EncvHostTranslucent" parent="@android:style/Theme.Translucent.NoTitleBar">
    <item name="android:windowBackground">#CC000000</item>
    <item name="android:windowIsTranslucent">true</item>
</style>
```

效果：用户能看到一个**半透明深色覆盖层**，至少知道 Activity 启动了。

#### SubTask 1.2: `onPostCreate` / `onResume` 超时检测

```kotlin
private var createTime = 0L
private const val PROXY_TIMEOUT_MS = 5000L

override fun onCreate(savedInstanceState: Bundle?) {
    createTime = System.currentTimeMillis()
    Log.i(TAG, "onCreate: intent=$intent")
    // ... existing code ...
}

override fun onResume() {
    super.onResume()
    val elapsed = System.currentTimeMillis() - createTime
    Log.i(TAG, "onResume: proxyStarted=$proxyStarted elapsed=${elapsed}ms")
    
    if (!proxyStarted) {
        finishWithResult(null, false, "播放器未启动", "proxyStarted=false after ${elapsed}ms")
        return
    }
    
    // 超时检测：如果 onCreate 已过 5 秒但 proxy 看起来异常
    if (elapsed > PROXY_TIMEOUT_MS) {
        Log.w(TAG, "onResume: proxy started but took ${elapsed}ms, may be stuck")
    }
}
```

#### SubTask 1.3: `onPostCreate` 延迟检查

```kotlin
override fun onPostCreate(savedInstanceState: Bundle?) {
    super.onPostCreate(savedInstanceState)
    if (!proxyStarted) {
        Log.w(TAG, "onPostCreate: proxy still not started after onPostCreate, scheduling timeout check")
        android.os.Handler(android.os.Looper.getMainLooper()).postDelayed({
            if (!proxyStarted && !isFinishing) {
                Log.e(TAG, "Timeout: proxy never started, finishing with error")
                finishWithResult(null, false, "播放器启动超时", "proxyStarted=false after onPostCreate+delay")
            }
        }, PROXY_TIMEOUT_MS)
    }
}
```

---

### Task 2: GoProcessPlugin 超时兜底（解决假设 C）

#### SubTask 2.1: `openPlayer` 添加超时自动 resolve

```kotlin
if (effectiveMode == "mpv-activity") {
    pendingCalls["mpvPlayer"] = call
    call.save()
    activity.startActivityForResult(intent, REQUEST_CODE_MPV_PLAYER)
    
    // 超时兜底：如果 15 秒内没有 onActivityResult 回调，自动 resolve
    Handler(Looper.getMainLooper()).postDelayed({
        if (pendingCalls.containsKey("mpvPlayer")) {
            Log.w(TAG, "openPlayer: mpv-activity result timeout (15s), resolving with warning")
            val staleCall = pendingCalls.remove("mpvPlayer")
            try { staleCall?.resolve(JSObject().apply {
                put("success", false)
                put("error", "播放器响应超时")
                put("errorDetail", "startActivityForResult dispatched but no result within 15s")
            }) } catch (_: Exception) {}
        }
    }, 15000)
}
```

---

### Task 3: 更新 combolite.md 规则

新增章节记录此踩坑经验：

```markdown
### 1.N EncvHostActivity 透明主题陷阱

> **使用 Theme.Translucent.NoTitleBar 的 HostActivity 如果 ProxyManager 代理失败，
> 用户会看到一个不可见的透明 Activity 覆盖在 WebView 上，表现为"卡住"。**

**症状**：startActivity 成功、无崩溃日志、无错误回调、触摸无响应

**必须的防御措施**：
1. 使用半透明主题而非全透明（让用户能看到 Activity 存在）
2. onPostCreate/onResume 添加超时检测（5s 无响应自动 finish）
3. GoProcessPlugin 端 startActivityForResult 添加超时兜底 resolve（15s）
4. 日志输出 onCreate→onResume 时间戳差值

**反模式**：
- ❌ 全透明主题 + 无超时检测 = 用户以为 app 死了
- ❌ 依赖 onActivityResult 回调而不做超时兜底 = Promise 永远 pending
```

---

## 改动文件清单

| 文件 | 改动 |
|------|------|
| [EncvHostActivity.kt](app/encv-mobile/android/app/src/main/java/com/encvgo/app/EncvHostActivity.kt) | 半透明主题适配 + onPostCreate 超时 + onResume 诊断增强 |
| [AndroidManifest.xml](app/encv-mobile/android/app/src/main/AndroidManifest.xml) | 主题改为自定义半透明 |
| `res/values/styles.xml` (新建) | Theme.EncvHostTranslucent 定义 |
| [GoProcessPlugin.kt](app/encv-mobile/android/app/src/main/java/com/encvgo/app/GoProcessPlugin.kt) | startActivityForResult 15s 超时兜底 resolve |
| [combolite.md](.trae/rules/combolite.md) | 新增 §1.N 透明主题陷阱规则 |

## 验证清单

- [ ] 安装 MPV → Settings 显示 ✓ ready → 打开视频 → 能看到半透明覆盖层出现
- [ ] 如果 MPV 正常播放 → 覆盖层消失/被替代为播放画面
- [ ] 如果 MPV 失败 → 5 秒后自动关闭覆盖层 + Files.vue 显示红色 error banner
- [ ] 如果完全卡住 → 15 秒后前端收到超时错误（不再永久挂起）
- [ ] logcat 过滤 `EncvHostActivity` 能看到完整生命周期时间戳
