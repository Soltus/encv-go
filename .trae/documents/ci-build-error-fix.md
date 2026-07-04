# CI 构建报错 + 扩展安装流程修复计划

## 问题分析

### 问题 1：CI 编译错误 — `Unresolved reference 'REQUEST_CODE_PLUGIN_PICK'`

**错误日志**（Step 28: Build Debug APK）：
```
e: GoProcessPlugin.kt:33:21 Unresolved reference 'REQUEST_CODE_PLUGIN_PICK'.
e: GoProcessPlugin.kt:33:47 Unresolved reference 'REQUEST_CODE_INSTALL_CONFIRM'.
```

**根因**：Kotlin 类注解不能引用同类 `companion object` 中的常量——注解在类定义之前求值，此时 companion object 尚不存在。

**修复**：将 `REQUEST_CODE_PLUGIN_PICK` 和 `REQUEST_CODE_INSTALL_CONFIRM` 从 `companion object` 移到文件顶层（top-level `const val`）。

---

### 问题 2：构建主应用时不应触发插件构建任务

**现象**：`./gradlew assembleDebug` 触发了 `:plugin-mpv-player:assembleDebug` 和 `:convert_plugin-mpv-player_debug` 任务。

**根因**：`app/build.gradle.kts` 的 `packagePlugins { enabled.set(true) }` 让 aar2apk 插件在主应用构建时自动执行插件打包。

**为什么错误**：
1. CI 中已有单独步骤（Step 25-26）构建和打包插件 APK
2. 插件 APK 不应打包到主应用中——ComboLite 插件是运行时动态加载的
3. 重复构建浪费时间

**修复**：`packagePlugins { enabled.set(false) }`

---

### 问题 3：installPlugin() 的系统安装兜底逻辑（最严重）

**位置**：`GoProcessPlugin.kt` L397-415

**当前逻辑**：
```
PluginManager.isInitialized?
  ├─ true  → startInstallConfirm() → ComboLite 安装 ✅
  └─ false → Intent.ACTION_INSTALL_PACKAGE → 系统安装器 ❌
```

**为什么错误**：
1. ComboLite 插件 APK **不是普通 Android 应用**，不能用系统安装器安装
2. 系统安装器会把插件当独立应用安装，要么失败，要么装出一个无法运行的空壳
3. `PluginManager.isInitialized == false` 说明框架未初始化，此时根本不应该安装插件

**修复**：当 `PluginManager.isInitialized == false` 时直接 reject，不走系统安装兜底。

---

### 问题 4：checkInstalledPlugins() 的文件扫描兜底

**位置**：`GoProcessPlugin.kt` L456-480

**当前逻辑**：
```
PluginManager.isInitialized?
  ├─ true  → PluginManager.getAllInstallPlugins() ✅
  └─ false → fallbackCheckInstalled() 扫描文件系统 ❌
```

**为什么错误**：文件存在 ≠ 插件已通过 ComboLite 正确安装（需要签名校验、类索引创建、组件解析、XML 持久化等步骤）。文件扫描会产生假阳性。

**修复**：当 `PluginManager.isInitialized == false` 时返回空结果，删除 `fallbackCheckInstalled()` 方法。

---

### 问题 5：前端系统安装提示残留

**位置**：`ExtensionsPage.vue` L189-193

**当前逻辑**：
```typescript
if (result.pending) {
    // 显示 systemInstallerHint 提示
}
```

**为什么错误**：`result.pending` 和 `systemInstallerHint` 是为系统安装兜底设计的 UI 提示，ComboLite 安装流程不需要这些。

**修复**：移除 `result.pending` 分支，统一走成功提示。

---

## 实施步骤

### Step 1：修复 GoProcessPlugin.kt 编译错误

将 `REQUEST_CODE_PLUGIN_PICK` 和 `REQUEST_CODE_INSTALL_CONFIRM` 从 `companion object` 移到文件顶层：

```kotlin
private const val REQUEST_CODE_PLUGIN_PICK = 9001
private const val REQUEST_CODE_INSTALL_CONFIRM = 9002

@CapacitorPlugin(
    name = "GoProcess",
    requestCodes = [REQUEST_CODE_PLUGIN_PICK, REQUEST_CODE_INSTALL_CONFIRM]
)
class GoProcessPlugin : Plugin() {
    companion object {
        private const val TAG = "ENCV-go"
        // appLogBuffer, APP_LOG_MAX, appLog, getAppLogs, clearAppLogs 不变
    }
}
```

### Step 2：移除 installPlugin() 的系统安装兜底

修改 `installPlugin()` 方法（L386-420），当 PluginManager 未初始化时直接返回错误：

```kotlin
@PluginMethod
fun installPlugin(call: PluginCall) {
    val apkPath = call.getString("apkPath") ?: run {
        call.reject("apkPath is required")
        return
    }
    try {
        val apkFile = File(apkPath)
        if (!apkFile.exists()) {
            call.reject("APK file not found: $apkPath")
            return
        }
        if (!PluginManager.isInitialized) {
            call.reject("PluginManager not initialized, cannot install plugin")
            return
        }
        startInstallConfirm(call, apkPath, apkFile.name)
    } catch (e: Exception) {
        Log.e(TAG, "installPlugin failed", e)
        call.reject("Failed to install plugin: ${e.message}")
    }
}
```

### Step 3：移除 checkInstalledPlugins() 的文件扫描兜底

修改 `checkInstalledPlugins()` 方法（L443-462），删除 `fallbackCheckInstalled()` 方法（L464-480）：

```kotlin
@PluginMethod
fun checkInstalledPlugins(call: PluginCall) {
    Log.d(TAG, "checkInstalledPlugins() called")
    val result = JSObject()
    try {
        if (PluginManager.isInitialized) {
            val plugins = PluginManager.getAllInstallPlugins()
            for (plugin in plugins) {
                result.put(plugin.id, true)
            }
            Log.i(TAG, "checkInstalledPlugins via ComboLite: $result")
            call.resolve(result)
            return
        }
        Log.w(TAG, "checkInstalledPlugins: PluginManager not initialized, returning empty")
        call.resolve(result)
    } catch (e: Exception) {
        Log.e(TAG, "checkInstalledPlugins failed", e)
        call.reject("Failed to check installed plugins: ${e.message}")
    }
}

// 删除 fallbackCheckInstalled() 方法
```

### Step 4：移除前端系统安装提示残留

修改 `ExtensionsPage.vue`（L184-203），移除 `result.pending` 分支：

```typescript
const result = await Promise.race([
    pickAndInstallPlugin(),
    new Promise<never>((_, reject) => setTimeout(() => reject(new Error('Installation timeout')), 120000)),
])
if (result.success) {
    const alert = await alertController.create({
        header: t('extensions.installSuccess'),
        message: `${result.fileName || ''}\n${t('extensions.installHint')}`,
        buttons: [t('common.confirm')],
    })
    await alert.present()
    await loadExtensions()
} else {
    installError.value = t('extensions.installFailed')
}
```

### Step 5：禁用主应用构建时的自动插件打包

修改 `app/build.gradle.kts`：

```kotlin
packagePlugins {
    enabled.set(false)
    buildType.set(PackageBuildType.DEBUG)
    pluginsDir.set("debug_plugins")
}
```

### Step 6：清理日志文件

删除 `job_logs/` 目录和 `job_logs.zip`
