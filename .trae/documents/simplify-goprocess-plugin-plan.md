# 计划：GoProcessPlugin.kt 精简为纯 Capacitor 入口胶水层

## 问题分析：当前 1004 行的职责分布

对 GoProcessPlugin.kt 逐行审计，按"是否必须留在 Capacitor Plugin 中"分类：

### 必须保留（Capacitor 胶水层专属）~ 250 行

| 方法/区域 | 行数 | 原因 |
|-----------|------|------|
| `@PluginMethod` + 参数提取 + JSObject 结果转换 | ~150 | 这是 Capacitor Bridge 的唯一职责 |
| ActivityResult 处理（pendingCalls/handleOnActivityResult） | ~80 | 依赖 `PluginCall.save()/call.getString()` |
| BroadcastReceiver（Go 后端状态） | ~40 | 依赖 Plugin 生命周期 |
| File picker Intent 启动 | ~15 | 依赖 `activity.startActivityForResult()` |
| InstallConfirmActivity 协调 | ~30 | 依赖 pendingCalls 机制 |

**理想状态下，GoProcessPlugin 应该只包含上述内容。**

### 应当提取的业务逻辑 ~ 750 行

| 区域 | 当前行数 | 提取目标 | 理由 |
|------|---------|---------|------|
| **AppLogger** (appLogBuffer/appLog/getAppLogs/clearAppLogs) | L43-60 (~18行) | → 新建 `AppLogger` 工具类 | 被多个组件使用（GoProcessPlugin、DiagnosticKit、exportLogs），不应耦合在 Plugin 中 |
| **日志导出子系统** (exportLogs/clearLogs/openLogViewer/saveDevLogs) | L848-1003 (~155行) | → 新建 `LogExporter` 工具类 | 纯文件操作+Intent分享，与 Capacitor 无关。内部还引用 `getAppLogs()` 和 `EncvGoService.getOutputSnapshot()` |
| **权限管理器** (requestNotificationPermission/requestStoragePermission/requestBatteryOptimization/checkPermissions) | L119-183, L311-331 (~85行) | → 扩展到 `PermissionHelper` 或新建 | 权限检查逻辑是通用 Android 能力，`checkPermissions` 被 Vue 多处调用 |
| **诊断结果组装** (debugKotlinReflect/debugApkValidation/debugValidationStrategy 中的后处理逻辑) | L646-789 (~143行) | → 移入 `DiagnosticKit` | 当前 DiagnosticKit 返回 `List<String>`，但 GoProcessPlugin 还要拼接 @Metadata 检查、host signatures、log 文件读取等——这些属于诊断逻辑不是胶水逻辑 |
| **播放器路由** (openPlayer/openExternal/openInPlayer/openPlayerHome/closePlayer) | L207-309 (~103行) | → 部分可委托 PlayerEntry | `openInPlayer`/`openPlayerHome` 包含 Intent 构造细节，可下沉 |
| **插件安装工作流** (installPlugin/pickAndInstallPlugin/handlePickResult/startInstallConfirm/handleInstallConfirmResult/executeComboLiteInstall) | L392-846 (~454行) | → 核心已委托 EncvComboLiteHost，但 URI→File 复制等可提取 | `handlePickResult` 的 URI→File 复制逻辑是通用工具 |

---

## 目标架构

```
GoProcessPlugin (目标: ~250行, 纯入口)
├── 每个 @PluginMethod = 5-15行
│   ├── 提取参数 (call.getString/Boolean)
│   ├── 调用库方法 (单行委托)
│   └── call.resolve/reject (JSObject 转换)
├── ActivityResult 分发 (必须保留)
└── BroadcastReceiver (必须保留)

:combolite-host 库 (扩展)
├── EncvComboLiteHost (已有)
├── PluginLifecycleEngine (已有)
├── DiagnosticKit (扩展: 吸收后处理逻辑)
└── model/ (已有)

:app 内工具类 (新建或从 GoProcessPlugin 提取)
├── AppLogger          — 应用内日志缓冲
├── LogExporter        — 日志导出/清除/查看
├── PermissionHelper   — Android 权限查询
└── UriUtils          — URI→File 复制工具
```

---

## 实施步骤

### Step 1：提取 AppLogger 工具类

**新建**: `app/src/main/java/com/encvgo/app/AppLogger.kt`

从 GoProcessPlugin.kt L43-60 提取：
```kotlin
object AppLogger {
    private val buffer = ConcurrentLinkedQueue<String>()
    private const val MAX = 3000

    fun log(level: String, tag: String, msg: String) { /* 同当前实现 */ }
    fun getLogs(): String = buffer.joinToString("\n")
    fun clear() = buffer.clear()
}
```

**影响范围**：
- GoProcessPlugin.companion object 中的 appLog/getAppLogs/clearAppLogs → 改为调用 `AppLogger.*`
- DiagnosticKit 中直接调用 `GoProcessPlugin.appLog()` → 改为调用 `AppLogger.log()`
- LogExporter 中调用 `getAppLogs()` → 改为 `AppLogger.getLogs()`

### Step 2：提取 LogExporter 工具类

**新建**: `app/src/main/java/com/encvgo/app/LogExporter.kt`

从 GoProcessPlugin.kt L848-1003 提取全部 4 个方法的**核心逻辑**（不含 Capacitor 调用）：

```kotlin
object LogExporter {
    data class ExportResult(val success: Boolean, val path: String = "")

    fun export(context: Context): ExportResult { /* 原 exportLogs 的核心 */ }
    fun clear(context: Context): Boolean { /* 原 clearLogs 核心 */ }
    fun openViewer(context: Context): Boolean { /* 原 openLogViewer 核心 */ }
    fun saveDevLogs(context: Context, logsJson: String): String? { /* 原 saveDevLogs 核心 */ }
}
```

**GoProcessPlugin 中的对应 @PluginMethod 变为**：
```kotlin
@PluginMethod
fun exportLogs(call: PluginCall) {
    try {
        val result = LogExporter.export(context)
        if (result.success) call.resolve(JSObject().put("success", true).put("path", result.path))
        else call.reject("Failed to export logs")
    } catch (e: Exception) { call.reject(e.message) }
}
// ← 每个 ~5 行
```

**预计减少**: ~120 行 → ~20 行

### Step 3：提取 PermissionHelper

**新建**: `app/src/main/java/com/encvgo/app/PermissionHelper.kt`

```kotlin
object PermissionHelper {
    data class PermissionState(
        val notifications: Boolean,
        val storage: Boolean,
        val batteryOptimization: Boolean
    )

    fun checkAll(context: Context): PermissionState { /* 原 checkPermissions 逻辑 */ }
    fun isNotificationGranted(context: Context): Boolean { /* ... */ }
    fun isStorageGranted(context: Context): Boolean { /* ... */ }
    fun isBatteryOptimizationIgnored(context: Context): Boolean { /* ... */ }

    fun requestStoragePermission(activity: Activity) { /* Intent to settings */ }
    fun requestBatteryOptimization(activity: Activity) { /* Intent to settings */ }
    fun requestNotificationPermission(activity: Activity, requestCode: Int) { /* activity.requestPermissions */ }
}
```

**GoProcessPlugin 中的权限方法变为**：
```kotlin
@PluginMethod
fun checkPermissions(call: PluginCall) {
    val state = PermissionHelper.checkAll(context)
    call.resolve(JSObject().apply {
        put("notifications", state.notifications)
        put("storage", state.storage)
        put("batteryOptimization", state.batteryOptimization)
    })
}

@PluginMethod
fun requestStoragePermission(call: PluginCall) {
    PermissionHelper.requestStoragePermission(activity)
    call.resolve(JSObject().apply { put("granted", false); put("requiresSettings", true) })
}
// ← 每个 ~5 行
```

**预计减少**: ~85 行 → ~30 行

### Step 4：将诊断后处理逻辑移入 DiagnosticKit

当前问题：DiagnosticKit 返回 `List<String>`，但 GoProcessPlugin 在每个诊断方法中还要拼接额外信息：

| 诊断方法 | GoProcessPlugin 中的额外后处理 | 应移入 DiagnosticKit |
|--------|--------------------------|-------------------|
| `debugKotlinReflect` | @Metadata 检查 (L650-673)、R8 mapping 检查 (L675-688) | ✅ 移入 |
| `debugApkValidation` | Host app signatures (L712-731) | ✅ 移入（增加 context 参数）|
| `debugValidationStrategy` | setValidationStrategy 测试 (L744-769)、loadEnabledPlugins 测试 (L758-769)、onFrameworkSetup log (L772-783) | ✅ 移入 |

**改动**：

1. `DiagnosticKit.kotlinReflectHealthCheck()` → 增加返回值包含 @Metadata/R8 检查
2. `DiagnosticKit.apkValidation(apkFile, context)` → 已有 context，增加 host signatures 步骤
3. `DiagnosticKit.validationStrategyStatus()` → 增加 action test + log file reading

**GoProcessPlugin 中的诊断方法变为**：
```kotlin
@PluginMethod
fun debugKotlinReflect(call: PluginCall) {
    val steps = DiagnosticKit.kotlinReflectHealthCheck()  // 完整版，含 @Metadata/R8
    call.resolve(JSObject().apply { put("debugLog", steps.joinToString("\n")) })
}
// ← 每个 ~4 行
```

**预计减少**: ~100 行 → ~25 行

### Step 5：提取 UriUtils + 安装流程简化

**新建**: `app/src/main/java/com/encvgo/app/UriUtils.kt`

```kotlin
object UriUtils {
    fun copyUriToFile(context: Context, uri: Uri, targetDir: File): File? {
        // 从 handlePickResult (L543-563) 提取的 URI→File 复制逻辑
    }
}
```

**`handlePickResult` 精简为**：
```kotlin
private fun handlePickResult(resultCode: Int, data: Intent?) {
    val call = pendingCalls.remove("pickPlugin") ?: return
    if (resultCode != Activity.RESULT_OK || data?.data == null) {
        call.reject("File picker cancelled"); return
    }
    val tempFile = UriUtils.copyUriToFile(context, data.data!!, File(context.cacheDir, "plugin_install"))
        ?: run { call.reject("Cannot read selected file"); return }
    startInstallConfirm(call, tempFile.absolutePath, tempFile.name)
}
```

**预计减少**: ~30 行 → ~12 行

### Step 6：最终精简效果预估

| 区域 | 当前行数 | 精简后 | 减少 |
|------|---------|--------|------|
| 类声明 + import + companion(AppLogger) | ~60 | ~15 | -45 |
| BroadcastReceiver + 生命周期 | ~90 | ~90 | 0（必须保留） |
| Go 后端管理 (restart/stop/getStatus) | ~30 | ~30 | 0 |
| 权限相关 (4个方法) | ~85 | ~30 | -55 |
| 播放器路由 (5个方法) | ~103 | ~70 | -33 |
| 屏幕方向 | ~15 | ~15 | 0 |
| **插件安装全流程** (6个方法+handler) | **~220** | **~130** | **-90** |
| ComboLite 操作 (check/toggle/uninstall/debugLifecycle) | ~110 | ~55 | -55 |
| **诊断方法** (5个) | **~180** | **~35** | **-145** |
| 文件路径工具 | ~25 | ~10 | -15 |
| **日志系统** (export/clear/viewer/save) | **~155** | **~35** | **-120** |
| **总计** | **~1004** | **~520** | **~-48%** |

> **注意**：剩余 ~520 行中约 260 行是**必须保留的 Capacitor 胶水代码**（ActivityResult、BroadcastReceiver、@PluginMethod 参数提取），实际业务逻辑仅 ~260 行。

---

## 文件变更清单

| 操作 | 文件路径 | 说明 |
|------|---------|------|
| **新建** | `app/.../AppLogger.kt` | 从 companion object 提取的日志缓冲 |
| **新建** | `app/.../LogExporter.kt` | 日志导出/清除/查看/保存 |
| **新建** | `app/.../PermissionHelper.kt` | Android 权限查询+请求 |
| **新建** | `app/.../UriUtils.kt` | URI→File 复制工具 |
| **修改** | `combolite-host/.../diagnostic/DiagnosticKit.kt` | 吸收 @Metadata/R8/signatures/log 后处理 |
| **重写** | `GoProcessPlugin.kt` | 1004→~520 行，纯入口胶水 |

## 验证标准

1. 每个 `@PluginMethod` 方法体 ≤ 15 行（参数提取 + 单行库调用 + resolve/reject）
2. GoProcessPlugin 不再包含任何 `ConcurrentLinkedQueue` / `ZipOutputStream` / `Runtime.exec` / `PackageManager.getPackageInfo` 等非 Capacitor API
3. 所有新工具类的 public 方法可独立单元测试（不依赖 Capacitor）
4. 功能不变：前端 TS 接口、Vue handler 无需修改
