# Kotlin 2.3.21 + Android 编码参考（本项目金标准）

> 来源：CI 编译通过代码 + AAR 反编译证据 + Kotlin 官方文档

## 一、变量声明

### 1.1 val vs var 的选择规则

```kotlin
// ✅ 值确定后不再改变 → val
val pluginId = call.getString("pluginId") ?: return
val result = JSObject()

// ✅ 后续有重新赋值 → var
var flags = PackageManager.GET_META_DATA or PackageManager.GET_ACTIVITIES
if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.P) {
    flags = flags or PackageManager.GET_SIGNING_CERTIFICATES
}

// ✅ by 委托的 state（Compose）
var volume by remember { mutableStateOf(1f) }
```

### 1.2 类型推断失败时的处理

```kotlin
// 泛型 lambda 中编译器无法推断 → 显式指定类型参数
val count: Int = runBlocking { engine.loadAllEnabledPlugins() }
// 或
val count = runBlocking<Int> { engine.loadAllEnabledPlugins() }
```

---

## 二、协程与 Suspend 函数

### 2.1 三种调用 suspend 函数的方式

| 场景 | 写法 | 阻塞？ |
|------|------|--------|
| 已在 `suspend` 函数/协程内 | 直接调用 `foo()` | 否 |
| 从普通函数启动异步任务 | `GlobalScope.launch(Dispatchers.IO) { foo() }` | 否 |
| 从普通函数同步等待结果 | `runBlocking { foo() }` | **是** |

### 2.2 Capacitor @PluginMethod 中的标准模式

**本项目已验证通过的唯一正确写法**：

```kotlin
@PluginMethod
fun someAction(call: PluginCall) {
    val param = call.getString("key") ?: run { call.reject("key required"); return }
    GlobalScope.launch(Dispatchers.IO) {
        val result = SomeLibrary.suspendMethod(param)
        when (result) {
            is OperationResult.Success -> withContext(Dispatchers.Main) {
                call.resolve(JSObject().apply { put("data", result.data) })
            }
            is OperationResult.Failure -> withContext(Dispatchers.Main) {
                call.reject(result.reason)
            }
        }
    }
}
```

**关键点**：
- 参数提取在 launch **外部**（同步）
- 业务逻辑在 `Dispatchers.IO`（不阻塞主线程）
- 结果回调在 `Dispatchers.Main`（Capacitor 要求）
- `call.resolve/reject` 必须在主线程
### 2.3 初始化代码中调用 suspend 函数

```kotlin
// Application.onCreate / Framework setup — 一次性阻塞可接受
fun setupFramework(hostActivityClass: Class<*>) {
    try {
        runBlocking { PluginManager.setValidationStrategy(ValidationStrategy.Insecure) }
    } catch (e: Error) { } catch (e: Exception) { }
    // ...
}
```

## 三、模块可见性架构

```
:combolite-host (Android Library)
├── public   EncvComboLiteHost          ← 外部唯一入口 (object)
├── internal PluginLifecycleEngine     ← 引擎实现 (object, 同模块可访问)
├── public   DiagnosticKit              ← 诊断工具 (object)
└── public   model/*                   ← 数据类 (data class)

:app (Android Application)
├── public   GoProcessPlugin           ← Capacitor 插件入口
├── public   AppLogger                 ← 日志工具 (object)
├── public   LogExporter               ← 导出工具 (object)
├── public   PermissionHelper          ← 权限工具 (object)
├── public   UriUtils                  ← URI 工具 (object)
├── public   PlayerEntry               ← 播放器路由 (object)
└── public   EncvApplication           ← Application 类
```

**跨模块调用规则**：
- `:app` → `:combolite-host`：只能调用 **public** API (`EncvComboLiteHost`, `DiagnosticKit`, `model.*`)
- `:app` 中禁止 `import com.combo.core.runtime.PluginManager` 等直接 ComboLite API
- `:combolite-host` 内部：`internal` 对象在同模块不同包**可以**访问，但建议仍走门面

---

## 四、类型系统要点

### 4.1 Class<*> 转换

Java 泛型擦除导致 `Class<*>` 不能直接赋值给 `Class<ConcreteType>`：

```kotlin
// ❌ 类型不匹配
proxyManager.setHostActivity(hostActivityClass)  // hostActivityClass: Class<*>

// ✅ 显式转换
proxyManager.setHostActivity(
    hostActivityClass as Class<com.combo.core.component.activity.BaseHostActivity>
)
```

### 4.2 nullable 处理（AAR 反编译证实）

| API | 返回类型 | 安全用法 |
|-----|---------|---------|
| `getPluginInfo(id)` | `LoadedPluginInfo?` | `if (info != null) { ... }` |
| `uninstallPlugin(id)` | `Boolean` (boxed) | `if (success == true) { ... }` |
| `installPlugin(file, bool)` | `InstallResult` (sealed) | `when (result) { is Success -> ... is Failure -> ... }` |
| `getAllInstallPlugins()` | `List<PluginInfo>` (non-null) | 直接用，无需 null 检查 |
| `PluginInfo.enabled` | `boolean` (primitive) | 直接用 `.enabled`，无需 `?.` |

## 五、import 清单

```kotlin
// 协程
import kotlinx.coroutines.{Dispatchers, GlobalScope, launch, runBlocking, withContext}

// Android 核心
import android.content.Context
import android.content.Intent
import android.net.Uri
import java.io.File

// 并发容器
import java.util.concurrent.ConcurrentHashMap
import java.util.concurrent.ConcurrentLinkedQueue

// JSON（Capacitor Bridge）
import com.getcapacitor.JSObject
import com.getcapacitor.PluginCall

// 项目内部（:app 模块）
import com.encvgo.combolite.{EncvComboLiteHost, DiagnosticKit, model.OperationResult}
import com.encvgo.app.{AppLogger, LogExporter, PermissionHelper, UriUtils}
```

**禁止在 :app 模块中 import**：
- `com.combo.core.runtime.*` （应通过 EncvComboLiteHost）
- `com.combo.core.security.*` （同上）
- `com.combo.core.model.*` （使用 `com.encvgo.combolite.model.*` 包装）

### 5.2 禁止硬编码项目包名

**铁律：代码中不允许出现任何项目绝对包名字符串（`com.encvgo.xxx`）作为调用目标。**

```kotlin
// ❌ 硬编码全限定名（包名变更时全部失效）
com.encvgo.combolite.engine.PluginLifecycleEngine.loadAllEnabledPlugins()
com.encvgo.app.InstallConfirmActivity.EXTRA_APK_PATH
Class<com.encvgo.combolite.SomeClass>

// ✅ 通过 import 引用，包名变更只需改一处
import com.encvgo.combolite.engine.PluginLifecycleEngine
PluginLifecycleEngine.loadAllEnabledPlugins()

import com.encvgo.app.InstallConfirmActivity
InstallConfirmActivity.EXTRA_APK_PATH

import com.encvgo.combolite.SomeClass
Class<SomeClass>()
```

**例外**：第三方 AAR 的包名（如 `com.combo.core.*`）在 import 中出现是正常的，但同样应通过文件顶部 import 声明。

## 六、"金标准"参照

| 文件 | 验证过的模式 |
|------|------------|
| [EncvComboLiteHost.kt](combolite-host/src/main/java/com/encvgo/combolite/EncvComboLiteHost.kt) | public 门面 object，纯委托 |
| [PluginLifecycleEngine.kt](combolite-host/src/main/java/com/encvgo/combolite/engine/PluginLifecycleEngine.kt) | internal 引擎，suspend + runBlocking |
| [GoProcessPlugin.kt](android/app/src/main/java/com/encvgo/app/GoProcessPlugin.kt) | Capacitor @PluginMethod 标准模板 |
| [AppLogger.kt](android/app/src/main/java/com/encvgo/app/AppLogger.kt) | ConcurrentLinkedQueue 日志缓冲 |
| [LogExporter.kt](android/app/src/main/java/com/encvgo/app/LogExporter.kt) | ZipOutputStream 导出 |
| [PermissionHelper.kt](android/app/src/main/java/com/encvgo/app/PermissionHelper.kt) | Android 权限查询封装 |
| [UriUtils.kt](android/app/src/main/java/com/encvgo/app/UriUtils.kt) | ContentResolver URI→File |
