# MPV 播放器代理启动修复计划

## 根因分析

通过克隆 ComboLite 官方源码（`/tmp/ComboLite`）分析，发现 **两个根本性错误** 导致三种 MPV 模式全部只显示半透明遮罩：

### 根因 1：`createProxyIntent()` 使用了错误的 Intent Extra Key

**ComboLite 官方机制**（源码证据）：

```
Extensions.kt:
  ExtConstant.PLUGIN_ACTIVITY_CLASS_NAME = "plugin_activity_class_name"
  
  fun Intent.getPluginActivity(): IPluginActivity? =
      getStringExtra(PLUGIN_ACTIVITY_CLASS_NAME)?.let {
          PluginManager.getInterface(IPluginActivity::class.java, it)
      }

  fun Context.startPluginActivity(cls: Class<out IPluginActivity>, ...) {
      val intent = Intent(this, hostActivityClass).apply {
          putExtra(PLUGIN_ACTIVITY_CLASS_NAME, cls.name)  // ← 正确的 key
      }
      startActivity(intent)
  }
```

**我们的错误实现**（`PluginLifecycleEngine.createProxyIntent()`）：

```kotlin
putExtra("_combo_plugin_id", pluginId)           // ← 错误！ComboLite 不认这个 key
putExtra("_combo_target_activity", targetActivity) // ← 错误！ComboLite 不认这个 key
```

**后果**：`BaseHostActivity.onCreate()` → `initPluginActivity()` → `intent.getPluginActivity()` 读 `"plugin_activity_class_name"` → 返回 null → `pluginActivity == null` → 插件 Activity 从未被加载 → 只显示空白半透明遮罩。

### 根因 2：`MpvPlayerActivity` 继承了 `AppCompatActivity` 而非 `BasePluginActivity`

**ComboLite 官方机制**（源码证据）：

```
BasePluginActivity — 实现 IPluginActivity 接口的普通类（不是真正的 Activity）
  → 通过 proxyActivity?.setContent { } 设置 UI
  → 通过 proxyActivity 引用获取 Context/资源

官方 demo ComposeActivity:
  class ComposeActivity : BasePluginActivity() {
      override fun onCreate(savedInstanceState: Bundle?) {
          super.onCreate(savedInstanceState)
          proxyActivity?.setContent { ComposeContent() }  // ← 通过代理设置内容
      }
  }
```

**我们的错误实现**：

```kotlin
class MpvPlayerActivity : AppCompatActivity() {  // ← 错误！不是 IPluginActivity
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContent { ... }  // ← 直接设置，但这个 Activity 从未被真正启动
    }
}
```

**后果**：即使 Intent extra 正确，`PluginManager.getInterface(IPluginActivity::class.java, className)` 也找不到 `MpvPlayerActivity`（因为它没实现 `IPluginActivity`），返回 null。

### 根因 3：`EncvHostActivity` 没有按官方模式处理 `pluginActivity == null`

**官方 demo**（`HostActivity.kt`）：

```kotlin
override fun onCreate(savedInstanceState: Bundle?) {
    super.onCreate(savedInstanceState)  // 内部调用 initPluginActivity()
    if (super.pluginActivity == null) {
        // 没有插件 → 显示 Loading/错误页面
        setContent { LoadingScreen() }
    }
    // 有插件 → 插件自己会在 onCreate 中调用 proxyActivity?.setContent
}
```

**我们的错误实现**：自定义了 `proxyStarted` 标记和超时逻辑，完全绕过了 `pluginActivity` 检查。

---

## 修复方案

### Step 1：修复 `MpvPlayerActivity` — 改为继承 `BasePluginActivity`

**文件**：`plugin-mpv-player/src/main/java/com/encvgo/plugin/mpv/MpvPlayerActivity.kt`

关键变更：
- `class MpvPlayerActivity : AppCompatActivity()` → `class MpvPlayerActivity : BasePluginActivity()`
- `setContent { ... }` → `proxyActivity?.setContent { ... }`
- `intent.getStringExtra(...)` → `proxyActivity?.intent?.getStringExtra(...)`
- `finish()` → `proxyActivity?.finish()`
- `window.decorView` → `proxyActivity?.window?.decorView`
- `this` 引用（Context）→ `proxyActivity ?: return`
- 移除 `AppCompatActivity` import，添加 `BasePluginActivity` import
- 移除 `enableEdgeToEdge()`（由宿主 Activity 处理）

### Step 2：修复 `PluginLifecycleEngine.createProxyIntent()` — 使用正确的 Extra Key

**文件**：`combolite-host/src/main/java/com/encvgo/combolite/engine/PluginLifecycleEngine.kt`

关键变更：
- `putExtra("_combo_plugin_id", pluginId)` → 删除
- `putExtra("_combo_target_activity", targetActivity)` → `putExtra("plugin_activity_class_name", targetActivity)`
- 或者更好的方案：直接使用 ComboLite 官方 API `Context.startPluginActivity()`

**推荐方案**：使用 `startPluginActivity()` 扩展函数，这是官方推荐的方式。但由于我们需要从 `PluginLifecycleEngine`（非 Activity Context）启动，且需要传递自定义 extras，我们需要：
1. 构建 Intent 时使用正确的 key `"plugin_activity_class_name"`
2. 保留自定义 extras（file_path, file_name 等）

### Step 3：修复 `EncvHostActivity` — 按官方模式处理

**文件**：`android/app/src/main/java/com/encvgo/app/EncvHostActivity.kt`

关键变更：
- `onCreate()` 中先调用 `super.onCreate()`（内部会调用 `initPluginActivity()`）
- 检查 `pluginActivity == null` → 显示错误页面或自动 finish
- 移除自定义的 `proxyStarted` 标记逻辑（`BaseHostActivity.onCreate()` 内部已经处理了 `initPluginActivity()`）
- 保留超时检测作为安全兜底，但基于 `pluginActivity` 而非自定义标记

### Step 4：修复 `GoProcessPlugin.openPlayer()` — 适配新的启动方式

**文件**：`android/app/src/main/java/com/encvgo/app/GoProcessPlugin.kt`

关键变更：
- `mpv-activity` 模式：使用 `startPluginActivity()` 或正确的 Intent 构建
- `mpv-fragment` / `mpv-compose` 模式：暂仍 fallback 到 `mpv-activity`

### Step 5：修复 `PlayerEntry` — 适配新的启动方式

**文件**：`android/app/src/main/java/com/encvgo/app/PlayerEntry.kt`

关键变更：
- `startMpvViaActivity()` 使用正确的 Intent extra key
- 或者使用 `Context.startPluginActivity()` 扩展函数

### Step 6：验证

- `vue-tsc --noEmit` + `vite build`
- 确认 `MpvPlayerActivity` 编译通过（`BasePluginActivity` 来自 `compileOnly(libs.combolite.core)`）

---

## 风险评估

1. **`MpvPlayerActivity` 改为 `BasePluginActivity` 后**：
   - `MpvEngine` 构造函数接收 `Context`，需要改为 `proxyActivity!!`
   - `window.decorView` 需要改为 `proxyActivity?.window?.decorView`
   - `finish()` 需要改为 `proxyActivity?.finish()`
   - `intent` 需要改为 `proxyActivity?.intent`

2. **`BasePluginActivity` 不是 Activity**：
   - 不能使用 `AppCompatActivity` 的方法（如 `getSupportActionBar()`）
   - 所有 Activity 相关操作必须通过 `proxyActivity` 引用

3. **`startPluginActivity()` 需要 `Class<out IPluginActivity>` 参数**：
   - 我们不能直接用 `Class.forName()` 获取插件类（它在不同的 ClassLoader 中）
   - 需要使用 `PluginManager.getInterface(IPluginActivity::class.java, className)` 来获取实例
   - 或者手动构建 Intent，使用正确的 extra key
