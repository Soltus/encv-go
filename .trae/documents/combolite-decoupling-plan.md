# 计划：ComboLite 扩展逻辑解耦为独立 Android 库

## 当前问题分析

### 现状：ComboLite 逻辑散落在 3 个文件中，与 Capacitor 深度耦合

| 文件 | 行数 | ComboLite 职责 | 问题 |
|------|------|---------------|------|
| [GoProcessPlugin.kt](app/encv-mobile/android/app/src/main/java/com/encvgo/app/GoProcessPlugin.kt) | ~1350行 | install/uninstall/toggle/check/diagnostic × 6个方法 | **Capacitor Plugin 直接包含全部业务逻辑**，新增扩展必须改此文件 |
| [PlayerEntry.kt](app/encv-mobile/android/app/src/main/java/com/encvgo/app/PlayerEntry.kt) | 137行 | `isMpvAvailable()` + `startMpvPlayer()` 直接调用 `PluginManager` | 播放器入口直接依赖 ComboLite |
| [EncvApplication.kt](app/encv-mobile/android/app/src/main/java/com/encvgo/app/EncvApplication.kt) | 61行 | `onFrameworkSetup()` 初始化 ProxyManager/ValidationStrategy | 初始化逻辑在 Application 中 |

### 核心矛盾

```
当前调用链：
前端(Vue) → Capacitor Bridge → GoProcessPlugin(1350行巨石) → ComboLite API
                                              ↑
                              所有业务逻辑+诊断+Capacitor胶水混在一起

新增一个扩展需要：
1. 在 GoProcessPlugin.kt 加 @PluginMethod（Capacitor 层）
2. 在 web.ts 加接口声明（TS 层）
3. 在 ExtensionsPage.vue 加 UI（Vue 层）
4. 在 GoProcess.ts 加导出函数
→ 4 个文件、3 层耦合
```

---

## 目标架构

```
重构后调用链：
前端(Vue) → Capacitor Bridge → GoProcessPlugin(纯胶水~200行)
                                 ↓ 委托
                          :combolite-host 库（纯业务逻辑）
                          ┌─────────────────────────────┐
                          │ EncvComboLiteHost (Facade)   │ ← 唯一公共 API
                          │  ├─ 插件生命周期管理           │
                          │  ├─ 安装/卸载/启用/禁用        │
                          │  ├─ 播放器启动代理             │
                          │  └─ 诊断工具集                 │
                          └─────────────────────────────┘
                                 ↓ 调用
                          ComboLite AAR (com.combo.core.*)

新增一个扩展只需要：
1. 在 :combolite-host 注册插件元数据（一行配置）
2. 在 GoProcessPlugin 加 @PluginMethod（纯转发，5行）
3. 在 Vue 加 UI
→ 库层自动支持，无需改核心逻辑
```

---

## 新模块结构：`:combolite-host`

### 目录布局

```
app/encv-mobile/
├── combolite-host/                    ← 新建模块
│   ├── build.gradle.kts               ← 依赖 combolite-core + kotlinx-coroutines
│   └── src/main/java/com/encvgo/combolite/
│       ├── EncvComboLiteHost.kt       ← 公共 API 门面 (object 单例)
│       ├── ComboLiteHostConfig.kt     ← 配置数据类（插件注册表等）
│       ├── model/
│       │   ├── PluginState.kt         ← 插件状态数据类
│       │   ├── InstallResult.kt       ← 安装结果 sealed class
│       │   └── OperationResult.kt     ← 通用操作结果 sealed class
│       ├── engine/
│       │   └── PluginLifecycleEngine.kt ← 核心引擎（封装所有 ComboLite 调用）
│       └── diagnostic/
│           └── DiagnosticKit.kt       ← 饱和诊断工具集
│
├── android/settings.gradle.kts        ← 增加 include(":combolite-host")
├── android/app/build.gradle.kts       ← 改 implementation(project(":combolite-host"))
│
├── android/app/src/main/java/com/encvgo/app/
│   ├── GoProcessPlugin.kt             ← 瘦身为纯 Capacitor 胶水
│   ├── PlayerEntry.kt                ← 改为调用 EncvComboLiteHost
│   └── EncvApplication.kt            ← onFrameworkSetup 委托给 EncvComboLiteHost
```

---

## 实施步骤

### Step 1：创建 `:combolite-host` 模块骨架

**新建文件**：

#### `combolite-host/build.gradle.kts`
```kotlin
plugins {
    id("com.android.library")
    id("org.jetbrains.kotlin.android")
}

android {
    namespace = "com.encvgo.combolite"
    compileSdk = libs.versions.compileSdk.get().toInt()
    defaultConfig {
        minSdk = libs.versions.minSdk.get().toInt()
    }
    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_21
        targetCompatibility = JavaVersion.VERSION_21
    }
}

dependencies {
    implementation(libs.combolite.core)
    implementation(libs.kotlin.stdlib)
    implementation(libs.kotlin.reflect)
    implementation("org.jetbrains.kotlinx:kotlinx-coroutines-android:1.8.1")
}
```

关键点：
- **只依赖** `combolite-core` + Kotlin 标准库 + coroutines
- **不依赖** Capacitor / Compose /任何 UI 框架
- **不依赖** `:app` 模块

### Step 2：定义数据模型 (`model/`)

#### `model/PluginState.kt`
```kotlin
package com.encvgo.combolite.model

data class PluginState(
    val id: String,
    val name: String,
    val versionName: String,
    val versionCode: Long,
    val enabled: Boolean,
    val installed: Boolean,
    val entryClass: String?,
    val description: String? = null,
)
```

#### `model/OperationResult.kt`
```kotlin
package com.encvgo.combolite.model

sealed class OperationResult<out T> {
    data class Success<T>(val data: T) : OperationResult<T>()
    data class Failure(val reason: String, val exception: Throwable? = null) : OperationResult<Nothing>()
    
    inline fun <R> map(transform: (T) -> R): OperationResult<R> = when (this) {
        is Success -> Success(transform(data))
        is Failure -> this
    }
}
```

AAR 依据：`InstallerManager.InstallResult` 已是 sealed class(Success/Failure)，这里做统一包装。

### Step 3：实现核心引擎 (`engine/PluginLifecycleEngine.kt`) —— **最关键的文件**

这是唯一直接调用 `com.combo.core.*` API 的文件。所有 ComboLite 交互集中在此。

```kotlin
package com.encvgo.combolite.engine

import com.combo.core.runtime.PluginManager
import com.combo.core.runtime.ValidationStrategy
import com.combo.core.security.crash.PluginCrashHandler
import com.encvgo.combolite.model.*

internal object PluginLifecycleEngine {

    fun isInitialized(): Boolean = PluginManager.isInitialized

    fun getInstalledPlugins(): List<PluginState> { /* getAllInstallPlugins → map to PluginState */ }

    fun getPluginInfo(pluginId: String): PluginState? { /* getPluginInfo → map .pluginInfo */ }

    suspend fun installPlugin(apkFile: File): OperationResult<PluginState> { /* installerManager.installPlugin + auto load */ }

    suspend fun uninstallPlugin(pluginId: String): OperationResult<Unit> { /* installerManager.uninstallPlugin */ }

    suspend fun setPluginEnabled(pluginId: String, enabled: Boolean): OperationResult<Unit> { /* PluginManager.setPluginEnabled */ }

    suspend fun loadAllEnabledPlugins(): Int { /* PluginManager.loadEnabledPlugins */ }

    suspend fun launchPlugin(pluginId: String): Boolean { /* PluginManager.launchPlugin */ }

    fun ensurePluginLoaded(pluginId: String) { /* if getPluginInfo==null → launchPlugin */ }

    fun createProxyIntent(
        context: Context,
        pluginId: String,
        targetActivity: String,
        extras: Map<String, Any>
    ): Intent { /* Intent(context, hostActivity).putExtra(_combo_plugin_id, ...) */ }

    // Framework setup (called from Application.onFrameworkSetup)
    fun setupFramework(hostActivityClass: Class<*>) {
        // setValidationStrategy(Insecure)
        // setGlobalClashCallback(null)
        // proxyManager.setHostActivity(hostActivityClass)
    }
}
```

**从 GoProcessPlugin.kt 迁移的方法清单**：

| GoProcessPlugin 方法 | 迁移到 Engine 的方法 | 行数 |
|---------------------|-------------------|------|
| `checkInstalledPlugins()` | `getInstalledPlugins()` | ~20 |
| `togglePluginEnabled()` | `setPluginEnabled()` | ~30 |
| `uninstallPlugin()` | `uninstallPlugin()` | ~30 |
| `executeComboLiteInstall()` | `installPlugin()` | ~35 |
| — (post-install load) | `loadAllEnabledPlugins()` | ~5 |
| — (ensure loaded) | `ensurePluginLoaded()` | ~8 |
| — (create intent) | `createProxyIntent()` | ~15 |
| EncvApplication.setup | `setupFramework()` | ~20 |

**保留在 GoProcessPlugin.kt 中的方法（Capacitor 胶水层专属）**：

| 方法 | 保留原因 |
|------|---------|
| `restart/stop/getStatus` | Go 后端管理，与 ComboLite 无关 |
| `request*Permission` | Android 权限，与 ComboLite 无关 |
| `pickAndInstallPlugin` | 文件选择器 UI + 委托给 Engine.installPlugin |
| `openPlayer/openExternal/openInPlayer` | 播放器路由，委托给 Engine |
| `exportLogs/clearLogs/saveDevLogs` | 日志管理，与 ComboLite 无关 |
| `debug*` (×6个诊断方法) | 迁移到 DiagnosticKit |

### Step 4：实现门面 API (`EncvComboLiteHost.kt`)

```kotlin
package com.encvgo.combolite

import com.encvgo.combolite.engine.PluginLifecycleEngine
import com.encvgo.combolite.model.*

object EncvComboLiteHost {

    val isInitialized: Boolean get() = PluginLifecycleEngine.isInitialized()

    fun getInstalledPlugins(): List<PluginState> = PluginLifecycleEngine.getInstalledPlugins()

    fun getPluginInfo(pluginId: String): PluginState? = PluginLifecycleEngine.getPluginInfo(pluginId)

    fun isPluginAvailable(pluginId: String): Boolean =
        getInstalledPlugins().any { it.id == pluginId && it.enabled }

    suspend fun installPlugin(apkFile: File): OperationResult<PluginState> =
        PluginLifecycleEngine.installPlugin(apkFile)

    suspend fun uninstallPlugin(pluginId: String): OperationResult<Unit> =
        PluginLifecycleEngine.uninstallPlugin(pluginId)

    suspend fun setPluginEnabled(pluginId: String, enabled: Boolean): OperationResult<Unit> =
        PluginLifecycleEngine.setPluginEnabled(pluginId, enabled)

    suspend fun launchPlugin(pluginId: String): Boolean =
        PluginLifecycleEngine.launchPlugin(pluginId)

    fun ensurePluginLoaded(pluginId: String) = PluginLifecycleEngine.ensurePluginLoaded(pluginId)

    fun createProxyIntent(
        context: Context, pluginId: String,
        targetActivity: String, extras: Map<String, Any> = emptyMap()
    ): Intent = PluginLifecycleEngine.createProxyIntent(context, pluginId, targetActivity, extras)

    fun setupFramework(hostActivityClass: Class<*>) = PluginLifecycleEngine.setupFramework(hostActivityClass)
}
```

### Step 5：迁移诊断工具集 (`diagnostic/DiagnosticKit.kt`)

将 GoProcessPlugin 中的 6 个诊断方法迁移到独立的 `DiagnosticKit` 类：

```kotlin
package com.encvgo.combolite.diagnostic

object DiagnosticKit {
    fun lifecycleDiagnostic(pluginId: String): String  // 原 debugLifecycleFlow
    fun kotlinReflectHealthCheck(): String              // 原 debugKotlinReflect
    fun apkValidation(apkFile: File): String            // 原 debugApkValidation
    fun validationStrategyStatus(): String              // 原 debugValidationStrategy
    fun installTest(apkFile: File): String              // 原 debugInstallFlow
}
```

每个方法返回格式化的诊断文本字符串（不含 JSObject/Capacitor 依赖），由 GoProcessPlugin 包装成 `call.resolve(JSObject)` 返回。

### Step 6：改造现有文件为薄胶水层

#### `EncvApplication.kt` 精简
```kotlin
class EncvApplication : BaseHostApplication() {
    override fun onFrameworkSetup(): suspend () -> Unit = {
        EncvComboLiteHost.setupFramework(EncvHostActivity::class.java)
    }
    // Bugly init 保持不变
}
```

#### `PlayerEntry.kt` 精简
```kotlin
fun isMpvAvailable(context: Context): Boolean =
    EncvComboLiteHost.isPluginAvailable(PLUGIN_ID)

private fun startMpvPlayer(...) {
    EncvComboLiteHost.ensurePluginLoaded(PLUGIN_ID)
    val intent = EncvComboLiteHost.createProxyIntent(context, PLUGIN_ID, TARGET_ACTIVITY, extras)
    context.startActivity(intent)
}
```

#### `GoProcessPlugin.kt` 瘦身示例

**Before (当前)**: `togglePluginEnabled()` ~35行，直接调用 PluginManager + 构建 JSObject
**After (重构后)**:
```kotlin
@PluginMethod
fun togglePluginEnabled(call: PluginCall) {
    val pluginId = call.getString("pluginId") ?: run { call.reject("pluginId required"); return }
    val enabled = call.getBoolean("enabled", true) ?: true
    GlobalScope.launch(Dispatchers.IO) {
        val result = EncvComboLiteHost.setPluginEnabled(pluginId, enabled)
        when (result) {
            is OperationResult.Success -> withContext(Dispatchers.Main) {
                call.resolve(JSObject().apply { put("success", true); put("pluginId", pluginId); put("enabled", enabled) })
            }
            is OperationResult.Failure -> withContext(Dispatchers.Main) {
                call.reject(result.reason)
            }
        }
    }
}
```

每个 @PluginMethod 从 ~30-40行缩减到 ~15行（纯参数提取 + 结果转换）。

### Step 7：注册新模块到 Gradle

#### `settings.gradle.kts` 增加
```kotlin
include(":combolite-host")
```

#### `app/build.gradle.kts` 修改依赖
```kotlin
// 新增
implementation(project(":combolite-host"))

// combolite-core 不再被 app 直接依赖（通过 :combolite-host 传递）
// 但如果其他代码仍需直接引用，可保留
```

---

## 文件变更清单

| 操作 | 文件路径 | 说明 |
|------|---------|------|
| **新建** | `combolite-host/build.gradle.kts` | 库模块构建配置 |
| **新建** | `combolite-host/src/.../EncvComboLiteHost.kt` | 公共 API 门面 |
| **新建** | `combolite-host/src/.../model/PluginState.kt` | 插件状态数据类 |
| **新建** | `combolite-host/src/.../model/OperationResult.kt` | 统一结果类型 |
| **新建** | `combolite-host/src/.../engine/PluginLifecycleEngine.kt` | 核心 ComboLite 引擎 |
| **新建** | `combolite-host/src/.../diagnostic/DiagnosticKit.kt` | 诊断工具集 |
| **修改** | `settings.gradle.kts` | include 新模块 |
| **修改** | `app/build.gradle.kts` | 添加库依赖 |
| **重写** | `GoProcessPlugin.kt` | 瘦身为纯 Capacitor 胶水 |
| **精简** | `PlayerEntry.kt` | 委托给 EncvComboLiteHost |
| **精简** | `EncvApplication.kt` | onFrameworkSetup 委托给 Engine |

---

## 新增扩展的开发流程对比

### 当前流程（重构前）
```
1. 创建 :plugin-xxx 模块（已有良好结构 ✅）
2. 在 GoProcessPlugin.kt 加 @PluginMethod（install/toggle/uninstall 各一个）← 改业务文件！
3. 在 web.ts 加接口声明
4. 在 GoProcess.ts 加导出函数
5. 在 ExtensionsPage.vue 加 UI 卡片 + handler
6. 在 useI18n.ts 加 i18n key
```

### 重构后流程
```
1. 创建 :plugin-xxx 模步（同上 ✅）
2. 在 ComboLiteHostConfig.kt 注册插件元数据（一行配置）← 新！库层自动支持
3. 在 GoProcessPlugin.kt 加 @PluginMethod（纯转发，每个~15行）← 只改胶水层
4. 在 web.ts 加接口声明（同上）
5. 在 GoProcess.ts 加导出函数（同上）
6. 在 ExtensionsPage.vue 加 UI 卡片 + handler（同上）
7. 在 useI18n.ts 加 i18n key（同上）
```

**关键改进**：步骤 2 的"一行注册"意味着新插件的 **安装/卸载/启用/禁用/状态查询** 全部由 `PluginLifecycleEngine` 自动处理——不需要为新插件写任何 ComboLite 业务代码。

---

## 依赖关系图

```
:app (Android Application)
├── :capacitor-android          (Capacitor runtime)
├── :capacitor-cordova-android-plugins
├── :combolite-host             ← 新增！纯业务逻辑库
│   └── combolite-core:2.0.2    (ComboLite AAR)
├── :plugin-mpv-player          (MPV 插件模块)
│   └── combolite-core:2.0.2    (compileOnly)
└── [其他 UI/网络依赖]

:combolite-host (Android Library)  ← 可独立编译测试
├── combolite-core:2.0.2
├── kotlin-stdlib
├── kotlin-reflect
└── kotlinx-coroutines-android
```

`:combolite-host` **零 UI 依赖**，可在纯 JVM 单元测试中验证所有 ComboLite 交互逻辑。
