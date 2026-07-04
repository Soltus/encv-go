# CI构建修复：Kotlin编译错误

## 错误总览

CI 构建失败，两个模块的 `compileDebugKotlin` 任务均报错：

1. **`:plugin-mpv-player:compileDebugKotlin`** — 缺少依赖 + 代码错误
2. **`:app:compileDebugKotlin`** — `PlayerBridgeModule`/`PlayerEntry`/`PlayerOverlayManager` 编译错误
3. **`:plugin-mpv-player:buildDebugPluginApk`** — 任务不存在（aar2apk 插件未应用到 plugin 模块）

---

## 修复步骤

### 步骤 1：修复 `plugin-mpv-player/build.gradle.kts` — 添加缺失依赖

**问题**：插件模块缺少多个关键依赖，导致大量 unresolved reference。

**修改**：

```kotlin
dependencies {
    compileOnly(libs.combolite.core)

    implementation(platform(libs.compose.bom))
    implementation(libs.compose.ui)
    implementation("androidx.compose.ui:ui-tooling")
    implementation(libs.compose.ui.tooling.preview)
    implementation(libs.compose.material3)
    implementation(libs.androidx.activity.compose)

    // 新增缺失依赖
    implementation("androidx.appcompat:appcompat:1.7.1")           // AppCompatActivity
    implementation("androidx.compose.material:material-icons-extended")  // Fullscreen, Pause, LockOpen 等
    implementation("androidx.core:core-ktx:1.17.0")               // WindowCompat, context 扩展
    implementation("androidx.activity:activity-ktx:1.11.0")       // ComponentActivity 扩展
    implementation("org.jetbrains.kotlinx:kotlinx-coroutines-android:1.8.1")  // async, coroutineScope
}
```

### 步骤 2：修复 `MpvControls.kt` — Material Icons 导入

**问题**：`Fullscreen`, `FullscreenExit`, `LockOpen`, `Pause` 是 `material-icons-extended` 中的图标，不在默认 `material-icons-filled` 中。

**修改**：添加 `material-icons-extended` 依赖后，这些导入应自动解析。无需改代码，只需步骤 1 的依赖。

### 步骤 3：修复 `MpvPlayerActivity.kt` — AppCompatActivity 依赖

**问题**：`AppCompatActivity`、`enableEdgeToEdge()`、`setContent`、`intent`、`finish()` 等均来自 appcompat/activity 库。

**修改**：添加 `appcompat` 和 `activity-ktx` 依赖后自动解析。无需改代码。

### 步骤 4：修复 `MpvPlayerScreen.kt` — 代码级错误

**问题**：
1. `const val SPEED_OPTIONS = listOf(...)` — `const val` 只允许原始类型和 String
2. `startPlayback()` 是 suspend 函数，在非协程上下文中调用
3. `context` 在 Composable 中用法错误（`context as? Activity`）
4. `async` 未导入
5. `intent` 在 Composable 中不可用
6. `WindowCompat` 未导入

**修改**：

```kotlin
// 1. const val → 顶层 val
private val SPEED_OPTIONS = listOf(0.5f, 0.75f, 1f, 1.25f, 1.5f, 2f)

// 2. startPlayback 在 LaunchedEffect 中调用（已是协程上下文，OK）
//    但在 onToggleFullscreen 和 onRetry 回调中不是协程上下文
//    解决：使用 rememberCoroutineScope()

// 3. context → LocalContext.current
val context = LocalContext.current

// 4. 添加 import
import kotlinx.coroutines.async
import androidx.compose.ui.platform.LocalContext
import androidx.core.view.WindowCompat

// 5. intent 引用需要通过 Activity 获取
val backendUrl = (context as? android.app.Activity)?.intent?.getStringExtra("backend_url") ?: ""

// 6. async 需要协程作用域
```

具体代码修改：

- `SPEED_OPTIONS` 从 `const val` 改为 `val`
- 在 `MpvPlayerScreen` 中添加 `val scope = rememberCoroutineScope()`
- `onToggleFullscreen` 回调中的 `context` 改为 `LocalContext.current`
- `onRetry` 回调中的 `startPlayback` 用 `scope.launch { ... }` 包裹
- `resolveStreamUrl` 中的 `intent` 改为通过参数传入 `backendUrl`
- 添加缺失的 import

### 步骤 5：修复 `MpvPluginEntry.kt` — ComboLite IPluginEntry

**问题**：`import io.github.combolite.core.IPluginEntry` — `Unresolved reference 'io'`。

**原因**：`compileOnly(libs.combolite.core)` 的 Group ID 是 `io.github.lnzz123`，不是 `io.github.combolite`。需要确认 ComboLite Core 的实际包名。

**修改**：检查 ComboLite Core 的实际 Maven 坐标和包名。如果包名确实是 `io.github.combolite.core`，则可能是依赖未正确解析。需要确认 `libs.combolite.core` 对应的 Maven 坐标是否正确。

当前 `libs.versions.toml`：
```toml
combolite-core = { group = "io.github.lnzz123", name = "combolite-core", version.ref = "combolite" }
```

如果 Maven 坐标正确但包名不同，需要修改 import。如果包名正确但 Maven 坐标不对，需要修改 `libs.versions.toml`。

**方案**：先检查 ComboLite 的实际包结构。根据 `PlayerEntry.kt` 中的 `io.github.combolite.core.PluginManager` 和 `io.github.combolite.core.model.PluginInfo`，包名应该是 `io.github.combolite.core`。问题可能是 `compileOnly` 在 library 模块中的传递性问题，或者 Maven 仓库中确实没有这个 artifact。

**修改**：将 `compileOnly` 改为 `implementation`，确保编译时可用。

### 步骤 6：修复 `MpvProgressBar.kt` — `isFinite()`

**问题**：`ms.isFinite()` — Kotlin 中 `Long` 没有 `isFinite()` 方法（那是 `Float`/`Double` 的方法）。

**修改**：
```kotlin
// 旧：if (!ms.isFinite() || ms < 0) return "0:00"
// 新：if (ms < 0) return "0:00"
```

`Long` 不可能是 NaN 或 Infinity，所以 `isFinite()` 检查对 `Long` 无意义。

### 步骤 7：修复 `PlayerBridgeModule.kt` — `context(...)` 错误

**问题**：`Function invocation 'context(...)' expected` — LynxModule 的构造函数签名可能不接受 `context` 参数名。

**原因**：Lynx SDK 的 `LynxModule` 基类构造函数可能不是 `(context: Context)`，而是无参或其他签名。但 `MpvPlayerModule`、`GoBackendModule`、`LogBridgeModule` 都使用相同模式 `class XxxModule(context: Context) : LynxModule(context)` 且能编译。

**关键发现**：`PlayerBridgeModule` 在 `:app` 模块中，而其他 LynxModule 也在 `:app` 模块中且能编译。错误只出现在 `PlayerBridgeModule` 上。查看代码：

```kotlin
class PlayerBridgeModule(context: Context) : LynxModule(context) {
    ...
    PlayerEntry.play(context, filePath, fileName, mimeType)  // line 13
    PlayerEntry.play(context, filePath, fileName, mimeType, isExternal = true)  // line 23
    val available = PlayerEntry.isMpvAvailable(context)  // line 33
}
```

错误 `Function invocation 'context(...)' expected` 意味着编译器把 `context` 当作了一个函数而不是属性。这可能是因为 LynxModule 有一个 `context()` 方法而不是 `context` 属性。

**修改**：将 `context` 改为 `getContext()` 或其他 LynxModule 提供的 API。需要检查 LynxModule 的实际 API。根据其他 LynxModule 的用法（如 `GoBackendModule` 中使用 `context.applicationContext`），`context` 应该是可用的。问题可能是 LynxModule 的 `context` 是一个方法而非属性。

**方案**：在构造函数中保存 context 为私有属性：
```kotlin
class PlayerBridgeModule(private val appContext: Context) : LynxModule(appContext) {
    ...
    PlayerEntry.play(appContext, filePath, fileName, mimeType)
    ...
}
```

### 步骤 8：修复 `PlayerEntry.kt` — ComboLite API 错误

**问题**：`PluginManager`、`getInstalledPlugin`、`MpvPlayerActivity`、`createPluginIntent` 等 unresolved。

**原因**：
1. `io.github.combolite.core.PluginManager` — ComboLite Core 的包名/类名可能不对
2. `MpvPlayerActivity` — 在 `:plugin-mpv-player` 模块中，`:app` 模块无法直接引用
3. `mpvPlugin.enabled` — PluginInfo 可能没有 `enabled` 属性

**修改**：
1. 确认 ComboLite Core 的实际 API（需要检查 Maven artifact）
2. `MpvPlayerActivity` 不能从 `:app` 直接引用 `:plugin-mpv-player` 的类（循环依赖）。需要通过反射或 Intent 按类名启动
3. 如果 ComboLite API 不确定，先用反射/Intent 方式启动 MpvPlayerActivity

**具体方案**：
```kotlin
// 移除对 plugin-mpv-player 类的直接引用
// 使用 Intent 按包名+类名启动 Activity
private fun startMpvPlayer(context: Context, filePath: String, fileName: String, mimeType: String, isExternal: Boolean) {
    val intent = Intent().apply {
        setClassName(context, "com.encvgo.plugin.mpv.MpvPlayerActivity")
        putExtra(EXTRA_FILE_PATH, filePath)
        putExtra(EXTRA_FILE_NAME, fileName)
        putExtra(EXTRA_MIME_TYPE, mimeType)
        putBoolean(EXTRA_IS_EXTERNAL, isExternal)
        putString(EXTRA_BACKEND_URL, getBackendBaseUrl(context))
        if (context !is android.app.Activity) {
            addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
        }
    }
    context.startActivity(intent)
}
```

对于 ComboLite API，如果 `compileOnly(libs.combolite.core)` 在 `:app` 模块中是 `implementation`，则应该可以解析。检查 `:app` 的依赖——已有 `implementation(libs.combolite.core)`，所以 `PluginManager` 应该可用。问题可能是 ComboLite Core 的实际包名与代码中的 import 不匹配。

**方案**：先检查 ComboLite Core 的实际包结构。如果确认包名正确，则问题可能出在 Maven 仓库配置上（jitpack.io 中的 artifact 可能不存在或版本不对）。

### 步骤 9：修复 `PlayerOverlayManager.kt` — 大量 unresolved reference

**问题**：`FrameLayout`、`LynxView`、`MainActivity`、`Handler`、`MpvPlayerModule`、`LogRelay`、`GoBackendModule` 等全部 unresolved。

**原因**：这些类都在 `:app` 模块中存在，但缺少对应的 import 语句。`PlayerOverlayManager.kt` 文件头部没有 import。

**修改**：添加所有缺失的 import 语句。这个文件本身代码逻辑正确，只是缺少 import。

需要添加的 import：
```kotlin
import android.content.pm.ActivityInfo
import android.os.Handler
import android.os.Looper
import android.view.View
import android.view.ViewGroup
import android.widget.FrameLayout
import android.view.WindowManager
import com.lynx.tasm.LynxView
import com.lynx.tasm.LynxViewBuilder
import com.lynx.tasm.behavior.LynxViewClient
import org.json.JSONObject
```

以及项目内部类的 import：
```kotlin
import com.encvgo.app.LogRelay
import com.encvgo.app.MpvPlayerModule
import com.encvgo.app.GoBackendModule
import com.encvgo.app.LogBridgeModule
import com.encvgo.app.PlayerTemplateProvider
import com.encvgo.app.EncvApplication
import com.encvgo.app.R
```

### 步骤 10：修复 `buildDebugPluginApk` 任务不存在

**问题**：CI 步骤 `:plugin-mpv-player:buildDebugPluginApk` 失败，因为 `aar2apk` 插件只应用在 `:app` 模块，不在 `:plugin-mpv-player` 模块。

**原因**：`aar2apk` 插件在根 `build.gradle.kts` 中声明，在 `:app` 中通过 `alias(libs.plugins.combolite.aar2apk)` 应用。`plugin-mpv-player` 没有应用此插件。

**修改**：在 `plugin-mpv-player/build.gradle.kts` 中添加 `aar2apk` 插件：
```kotlin
plugins {
    id("com.android.library")
    id("org.jetbrains.kotlin.android")
    id("org.jetbrains.kotlin.plugin.compose")
    alias(libs.plugins.combolite.aar2apk)
}
```

### 步骤 11：清理日志文件

修复完成后删除 `job_logs/` 目录和 `job_logs.zip`。

---

## 修改文件清单

| 文件 | 修改内容 |
|------|---------|
| `plugin-mpv-player/build.gradle.kts` | 添加 appcompat, material-icons-extended, core-ktx, coroutines 依赖；添加 aar2apk 插件 |
| `plugin-mpv-player/.../MpvPlayerScreen.kt` | `const val` → `val`；添加 coroutineScope；修复 context/intent/WindowCompat 引用 |
| `plugin-mpv-player/.../MpvProgressBar.kt` | 移除 `isFinite()` 对 Long 的调用 |
| `plugin-mpv-player/.../MpvPluginEntry.kt` | 确认 ComboLite 包名，可能需要修改 import |
| `android/app/.../PlayerBridgeModule.kt` | 修复 `context` 引用问题 |
| `android/app/.../PlayerEntry.kt` | 移除对 plugin-mpv-player 类的直接引用，改用 Intent 按类名启动 |
| `android/app/.../PlayerOverlayManager.kt` | 添加所有缺失的 import 语句 |
