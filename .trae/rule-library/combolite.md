# ComboLite 集成规范（详情）

> **本文件为 [combolite.md](../rules/combolite.md) 的详情文档**。包含 R8 重命名机制详细链路、@Metadata 一致性破坏原理、ComboLite 内部反射 API 调用图、完整 isMinifyEnabled 调试经验、实战踩坑历史。

---

## 一、铁律（违反 = 严重错误）

### 1.1 禁止反射调用 ComboLite API

**❌ 错误（幻觉代码）**：
```kotlin
// PluginManager 是 Kotlin object 单例，没有 getInstance(Context) 方法！
val pm = Class.forName("com.combo.core.runtime.PluginManager")
    .getMethod("getInstance", Context::class.java)
    .invoke(null, context)

// installPlugin 定义在 InstallerManager 上，不在 PluginManager 上！
val method = pm.javaClass.methods.find { it.name == "installPlugin" && it.parameterCount == 2 }
method.invoke(pm, apkFile, true)

// getAllInstallPlugins 是直接方法，不需要反射猜测方法名
val pluginsMethod = pm.javaClass.methods.find {
    it.name == "getInstalledPlugins" || it.name == "getLoadedPlugins" || ...
}
```

**✅ 正确（直接引用）**：
```kotlin
import com.combo.core.runtime.PluginManager

// object 单例，直接使用
if (PluginManager.isInitialized) {
    // 通过属性访问子管理器
    val result = PluginManager.installerManager.installPlugin(apkFile, true)
    val plugins = PluginManager.getAllInstallPlugins()
    val info = PluginManager.getPluginInfo("mpv-player")
}
```

**为什么这些反射代码是「幻觉」**：
1. **Kotlin object 单例**是 `INSTANCE` 静态字段，不是 `getInstance(Context)` 工厂方法
2. **`installPlugin` 是 `InstallerManager` 实例方法**，不是 `PluginManager` 的方法
3. **`getAllInstallPlugins` 是真实 API**，但 `getInstalledPlugins` / `getLoadedPlugins` 是 LLM 幻觉出来的错误方法名
4. **ComboLite 公开 API 全部是直接可用的**，反射只会绕过类型检查

### 1.2 禁止 Hook 任何系统服务

ComboLite 的核心价值就是 **0 Hook**。如果发现代码中有：
- `XposedHelpers.findAndHookMethod`
- `Instrumentation` 替换
- AMS/PMS 代理
- ClassLoader `pathList` 修改

**立即删除**，这不是 ComboLite 的用法。

ComboLite 通过 `android:sharedUserId` + `Binder` IPC 实现"伪系统服务"，**不需要 Hook**。

### 1.3 禁止在 release 构建中启用 R8/ProGuard

> **ComboLite 框架内部使用 `::function.javaMethod`（kotlin-reflect API）做 @RequiresPermission 权限检查。**
> **R8 会破坏 ComboLite 类的字节码与 `@Metadata` 注解之间的一致性，导致 kotlin-reflect 无法解析函数签名。**
> **ComboLite 官方 demo (`app/build.gradle.kts`) 明确设置 `isMinifyEnabled = false`。**

#### R8 破坏机制（2026-05-28 实测确认）

**字节码侧**：
```
@Metadata 存在且描述原始名称:
  mv=[2,2,0], k=1

但 R8 将方法重命名:
  setValidationStrategy → a()
  installPlugin → b()
  loadEnabledPlugins → c()
```

**kotlin-reflect 解析时**：
```
PluginManager.class -> lookupMethod "setValidationStrategy" 
  → 在重命名后的字节码里找不到这个名字
  → 抛 NullPointerException 或 IllegalArgumentException
  → @RequiresPermission 权限检查失败
  → installPlugin 永远报 "no suitable plugin"
```

**实测堆栈**：
```
java.lang.NullPointerException: Cannot invoke "java.lang.reflect.Method.invoke(...)"
    because the return value of "kotlin.reflect.jvm.internal.KClassImpl.getDeclaredFunctions(...)" is null
    at com.combo.core.runtime.PluginManager.setValidationStrategy(PluginManager.kt)
    at com.combo.core.internal.AuthorizationManager.checkPermission(AuthorizationManager.kt:42)
    at com.combo.core.runtime.InstallerManager.installPlugin(InstallerManager.kt:78)
    at com.encv.host.bridge.PluginBridge.install(PluginBridge.kt:55)
```

**为什么 @Metadata 救不回来**：
- @Metadata 的 `d2` 字段描述 Kotlin 内部元数据，**包含原始方法名**
- 但 R8 在**重命名字节码**时也**同步重写 @Metadata 里的引用**（否则反射彻底坏）
- ComboLite 在 release 模式下被宿主**单独打包**到 AAR，宿主 R8 不知道 ComboLite 类的**内部调用关系**（kotlin-reflect 走的是 metadata → jvm 名称的间接映射，R8 保留映射但**宿主 shrink 规则不包含 ComboLite 内部**）

**解决方案（按推荐度）**：
1. **最佳**：`isMinifyEnabled = false`（ComboLite 官方 demo 推荐）
2. **次优**：keep 规则 `keep class com.combo.** { *; }`（但要承担 R8 完整 shrink 风险）
3. **不推荐**：在宿主 R8 配置中保留 kotlin-reflect metadata 完整（容易踩别坑）

---

## 二、isMinifyEnabled / isShrinkResources 硬耦合

| 选项 | 作用对象 | 对 ComboLite 的风险 | 约束 |
|------|---------|-------------------|------|
| `isMinifyEnabled` | **DEX 字节码**（重命名方法、删除未用代码、内联） | **致命**：破坏 `@Metadata ↔ 字节码` 一致性 | **必须 `false`** |
| `isShrinkResources` | **resources.arsc + 资源文件**（删除未引用资源） | 理论上安全（只读 DEX 不改字节码），但... | **必须 `false`** |

**为什么 `isShrinkResources=true` 不能单独使用（CI 实测确认）**：

AGP 源码级硬约束（`AndroidResourcesCreationConfigImpl.kt:91`）：
```kotlin
// AGP internal check — 无法绕过
if (!buildType.isMinifyEnabled && androidResources.shrink) {
    issueReporter.reportError(
        "Removing unused resources requires unused code shrinking to be turned on."
    )
}
```

ResourceShrinker 的依赖图分析需要 R8/ProGuard 先生成完整的类→资源映射文件才能工作。当 `isMinifyEnabled=false` 时，AGP 在配置阶段直接抛出 `EvalIssueException`，构建无法继续。

**结论：由于 ComboLite 要求 `isMinifyEnabled=false`，`isShrinkResources` 也必须为 `false`。两者是 AGP 强耦合的。**

**受影响的 4 个类**（全部使用 `::function.javaMethod`）：

| 类 | 使用 javaMethod 的方法 |
|---|------|
| `PluginManager` | `setValidationStrategy`, `loadEnabledPlugins`, `launchPlugin`, `unloadPlugin`, `setPluginEnabled` |
| `InstallerManager` | `installPlugin`, `uninstallPlugin` |
| `PluginCrashHandler` | `setGlobalClashCallback`, `setClashCallback` |
| `AuthorizationManager` | `setAuthorizationHandler` |

---

## 三、build.gradle.kts 标准配置

```kotlin
android {
    buildTypes {
        release {
            isMinifyEnabled = false      // ← ComboLite 强制要求
            isShrinkResources = false    // ← AGP 强制耦合（同上必须 false）
            proguardFiles getDefaultProguardFile("proguard-android-optimize.txt")
            // 注：proguardFiles 即使配了，isMinifyEnabled=false 也不会启用 R8
        }
        debug {
            isMinifyEnabled = false
            isShrinkResources = false
        }
    }
}
```

**为什么 debug 也要 false**：
- debug 模式默认 false，但有些项目为了 debug 包减小体积会开 `isMinifyEnabled=true`
- 一旦开启 → 跑自动化测试时 installPlugin 报 "no suitable plugin" → 浪费调试时间
- **统一关闭**是最安全的策略

**何时可临时开启 isMinifyEnabled**：
- ✅ 单元测试环境（不调 ComboLite）
- ✅ 不涉及 ComboLite 的纯 Java 模块
- ❌ 主宿主 App 任何包含 `com.combo.**` import 的代码
- ❌ CI 跑 release build 准备上架前

---

## 四、API 使用陷阱（实战）

### 4.1 误用：把 installPlugin 放在 PluginManager 上

**❌ 错误**：
```kotlin
val pm = PluginManager  // Kotlin object
pm.installPlugin(apkFile, true)  // ← 编译失败！installPlugin 不在 PluginManager 上
```

**✅ 正确**：
```kotlin
val pm = PluginManager
pm.installerManager.installPlugin(apkFile, true)  // ← 通过子管理器
```

### 4.2 误用：getInstance(Context) 反射调用

**❌ 错误**：
```kotlin
val pm = Class.forName("com.combo.core.runtime.PluginManager")
    .getMethod("getInstance", Context::class.java)
    .invoke(null, context)
```

**✅ 正确**：
```kotlin
// ComboLite 初始化在 Application.onCreate 由 aar2apk 插件自动完成
// 业务代码只检查是否初始化 + 使用 API
if (PluginManager.isInitialized) {
    val result = PluginManager.installerManager.installPlugin(apkFile, true)
}
```

### 4.3 误用：自己处理权限检查

**❌ 错误**：
```kotlin
if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.M) {
    if (ContextCompat.checkSelfPermission(context, Manifest.permission.INSTALL_PACKAGES)
        != PackageManager.PERMISSION_GRANTED) {
        // 请求权限...
    }
}
PluginManager.installerManager.installPlugin(apkFile, true)  // ← ComboLite 自己会做这事
```

**✅ 正确**：
```kotlin
// ComboLite 内部的 AuthorizationManager 走 @RequiresPermission 自动处理
// 你只需要：确保 AndroidManifest.xml 声明了权限 + 在合适时机调 installPlugin
PluginManager.installerManager.installPlugin(apkFile, true)
```

### 4.4 误用：混淆 isInitialized 和 isReady

```kotlin
// 启动时序:
Application.onCreate() -> ComboLite.init() 完成 -> PluginManager.isInitialized = true
// 但插件列表加载是异步的，可能 PluginManager.getAllInstallPlugins() 返回空列表
```

**解决方案**：监听 `PluginManager.OnPluginsLoadedCallback` 回调再读列表。

---

## 五、实战踩坑历史

### 2026-05-28 事故：release build 报 "no suitable plugin"

**症状**：debug 包一切正常，release 包 `installPlugin` 报 "no suitable plugin found"。

**排查**：
1. 抓 logcat 发现 `kotlin.reflect.jvm.internal.KClassImpl.getDeclaredFunctions` 抛 NPE
2. 检查 APK：`apkanalyzer dex packages` 发现 `setValidationStrategy` 被重命名为 `a()`
3. 比对 debug APK：方法名完整保留

**根因**：`isMinifyEnabled = true` 启用 R8，重命名 ComboLite 内部方法名。

**修复**：`isMinifyEnabled = false`，release 重新构建，安装成功。

**教训**：
- ComboLite 项目 = 永远 `isMinifyEnabled = false`
- 不要为减小 APK 体积强行开 R8（ComboLite 约 1.2MB，开了 R8 省不了多少）
- CI 流程必须包含 release 冒烟测试（debug 通过不代表 release 通过）

### 2026-04-12 事故：isShrinkResources 单独开启 EvalException

**症状**：CI 配置 `isShrinkResources = true` 但保留 `isMinifyEnabled = false`，构建直接失败。

**修复**：两者都设为 `false`（参考 ComboLite 官方 demo）。

**教训**：单独尝试省资源体积的方案在 AGP 层面就被禁止了，不要浪费时间配置。
